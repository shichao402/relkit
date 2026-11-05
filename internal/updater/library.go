package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
)

func (e *Engine) handleListInstalled(req *updaterv2.UpdaterRequest) error {
	root := req.GetRuntime().GetInstall().GetInstallRoot()
	if root == "" {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "installRoot required", nil))
	}
	list := listInstalled(root)
	return e.emit(&updaterv2.UpdaterEvent{
		Kind: &updaterv2.UpdaterEvent_Installed{Installed: list},
	})
}

func (e *Engine) handleSwitchActive(req *updaterv2.UpdaterRequest, op *updaterv2.SwitchActiveOp) error {
	root := req.GetRuntime().GetInstall().GetInstallRoot()
	if root == "" {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "installRoot required", nil))
	}
	if op == nil || op.Code == 0 {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "code required", nil))
	}
	if err := acquireApplyLock(root, "switch"); err != nil {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_OCCUPIED, true, err.Error(), nil))
	}
	defer releaseApplyLock(root)
	if err := switchActive(root, op.Code); err != nil {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_SELECTOR_NO_MATCH, false, err.Error(), nil))
	}
	return e.emit(&updaterv2.UpdaterEvent{
		Kind: &updaterv2.UpdaterEvent_Result{Result: &updaterv2.Result{Kind: &updaterv2.Result_Ok{Ok: &updaterv2.Ok{}}}},
	})
}

func (e *Engine) handleRollback(req *updaterv2.UpdaterRequest) error {
	root := req.GetRuntime().GetInstall().GetInstallRoot()
	if root == "" {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "installRoot required", nil))
	}
	if err := acquireApplyLock(root, "rollback"); err != nil {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_OCCUPIED, true, err.Error(), nil))
	}
	defer releaseApplyLock(root)
	if err := rollbackActive(root); err != nil {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_ROLLBACK_REJECTED, false, err.Error(), nil))
	}
	return e.emit(&updaterv2.UpdaterEvent{
		Kind: &updaterv2.UpdaterEvent_Result{Result: &updaterv2.Result{Kind: &updaterv2.Result_Ok{Ok: &updaterv2.Ok{}}}},
	})
}

// handleDeleteInstalled removes one installed version (requirement 3.1).
//
// Three guards, all of them deliberate:
//   - the version active.json points at is never deletable, otherwise the
//     next launch would have nothing to start;
//   - versions the host declares in use (open windows) are never deletable;
//   - the delete holds .relkit-apply.lock, so it can never interleave with an
//     apply that is writing versions/.
func (e *Engine) handleDeleteInstalled(req *updaterv2.UpdaterRequest, op *updaterv2.DeleteInstalledOp) error {
	root := req.GetRuntime().GetInstall().GetInstallRoot()
	if root == "" {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "installRoot required", nil))
	}
	if req.GetRuntime().GetInstall().GetPlacement() != updaterv2.Placement_PLACEMENT_LIBRARY {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED, false,
			"deleteInstalled only applies to LIBRARY installs", nil))
	}
	if op == nil || op.Code <= 0 {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "code required", nil))
	}
	if err := acquireApplyLock(root, "delete"); err != nil {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_OCCUPIED, true, err.Error(), nil))
	}
	defer releaseApplyLock(root)
	if err := deleteInstalled(root, op.Code, op.InUseCodes); err != nil {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_OCCUPIED, false, err.Error(), nil))
	}
	return e.emit(&updaterv2.UpdaterEvent{
		Kind: &updaterv2.UpdaterEvent_Result{Result: &updaterv2.Result{Kind: &updaterv2.Result_Ok{Ok: &updaterv2.Ok{}}}},
	})
}

// deleteInstalled renames the version directory out of versions/ before
// removing it. Removing in place would leave a half-deleted tree visible to a
// concurrent listInstalled; a rename is atomic and takes it out of view at once.
func deleteInstalled(root string, code int64, inUse []int64) error {
	if active := readActive(root); active != nil && active.Code == code {
		return fmt.Errorf("refusing to delete active version %d", code)
	}
	for _, used := range inUse {
		if used == code {
			return fmt.Errorf("refusing to delete version %d: host reports it in use", code)
		}
	}
	found := ""
	for _, v := range listInstalled(root).GetVersions() {
		if v.Code == code {
			found = v.Path
			break
		}
	}
	if found == "" {
		return fmt.Errorf("code %d is not installed", code)
	}
	return moveToTrash(root, filepath.Join(root, filepath.FromSlash(found)))
}

func listInstalled(root string) *updaterv2.InstalledList {
	out := &updaterv2.InstalledList{}
	active := readActive(root)
	if active != nil {
		out.ActiveCode = active.Code
	}
	base := filepath.Join(root, "versions")
	ents, err := os.ReadDir(base)
	if err != nil {
		return out
	}
	for _, e := range ents {
		// Hidden entries are partial fresh writes, never installed versions.
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("versions", e.Name()))
		abs := filepath.Join(root, filepath.FromSlash(rel))
		meta := readVersionMeta(abs)
		item := &updaterv2.InstalledVersion{Path: rel}
		if meta != nil {
			item.Code = meta.Code
			item.Version = meta.Version
			item.Executable = meta.Executable
		} else if active != nil && filepath.ToSlash(active.Path) == rel {
			item.Code = active.Code
			item.Version = active.Version
			item.Executable = active.Executable
		} else if n, err := strconv.ParseInt(strings.TrimPrefix(e.Name(), "code-"), 10, 64); err == nil && strings.HasPrefix(e.Name(), "code-") {
			item.Code = n
		}
		if active != nil && item.Code == active.Code {
			item.Active = true
		}
		out.Versions = append(out.Versions, item)
	}
	sort.Slice(out.Versions, func(i, j int) bool {
		return out.Versions[i].Code < out.Versions[j].Code
	})
	return out
}

func switchActive(root string, code int64) error {
	list := listInstalled(root)
	var found *updaterv2.InstalledVersion
	for _, v := range list.Versions {
		if v.Code == code {
			found = v
			break
		}
	}
	if found == nil {
		return fmt.Errorf("code %d is not installed", code)
	}
	prev := readActive(root)
	ptr := activePointer{
		Code:       found.Code,
		Version:    found.Version,
		Path:       found.Path,
		Executable: found.Executable,
	}
	if prev != nil {
		ptr.Previous = prev.Path
	}
	if meta := readVersionMeta(filepath.Join(root, filepath.FromSlash(found.Path))); meta != nil {
		ptr.Bundle = meta.Bundle
	}
	return writeActive(root, ptr)
}

func rollbackActive(root string) error {
	cur := readActive(root)
	if cur == nil || cur.Previous == "" {
		return fmt.Errorf("no previous version to roll back to")
	}
	prevPath := filepath.FromSlash(cur.Previous)
	abs := filepath.Join(root, prevPath)
	meta := readVersionMeta(abs)
	ptr := activePointer{
		Path:     filepath.ToSlash(cur.Previous),
		Previous: cur.Path,
	}
	if meta != nil {
		ptr.Code = meta.Code
		ptr.Version = meta.Version
		ptr.Executable = meta.Executable
		ptr.Bundle = meta.Bundle
	} else {
		ptr.Executable = filepath.ToSlash(filepath.Join(cur.Previous, filepath.Base(cur.Executable)))
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("previous version directory missing")
	}
	return writeActive(root, ptr)
}
