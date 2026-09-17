// Package payload builds and validates structured internal-update artifacts.
package payload

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	updaterv1 "go.firoyang.com/relkit/api/updater/v1"
	"go.firoyang.com/relkit/internal/model"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	Schema        = "relkit.payload/1"
	TableName     = "files.pb"
	FilesPrefix   = "files/"
	ScriptsPrefix = "scripts/"
	ConfigDir     = ".relkit-payload"
	ConfigName    = "manifest.json"
)

type Script struct {
	Phase       updaterv1.ScriptPhase
	Path        string
	Interpreter updaterv1.ScriptInterpreter
	Timeout     time.Duration
	Args        []string
}

type BuildOptions struct {
	Tree       string
	ScriptsDir string
	Scripts    []Script
	Preserve   []string
}

type Package struct {
	Table       *updaterv1.FileTable
	FilesRoot   string
	ScriptsRoot string
}

func Build(outPath string, options BuildOptions) (*updaterv1.FileTable, error) {
	info, err := os.Stat(options.Tree)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("payload tree must be a directory: %s", options.Tree)
	}
	if err := applyDescriptor(&options); err != nil {
		return nil, err
	}
	table := &updaterv1.FileTable{Schema: Schema}
	type source struct {
		name string
		path string
		mode os.FileMode
	}
	var sources []source
	err = filepath.Walk(options.Tree, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == options.Tree {
			return nil
		}
		relFromRoot, _ := filepath.Rel(options.Tree, path)
		if relFromRoot == ConfigDir || strings.HasPrefix(filepath.ToSlash(relFromRoot), ConfigDir+"/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("payload does not support symlink %s", path)
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(options.Tree, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if err := CheckRelpath(rel); err != nil {
			return err
		}
		digest, size, err := model.Sha256File(path)
		if err != nil {
			return err
		}
		table.Files = append(table.Files, &updaterv1.FileEntry{
			Relpath: rel,
			Size:    size,
			Sha256:  digest,
			Mode:    uint32(info.Mode().Perm()),
		})
		sources = append(sources, source{name: FilesPrefix + rel, path: path, mode: info.Mode()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(table.Files, func(i, j int) bool { return table.Files[i].Relpath < table.Files[j].Relpath })
	sort.Slice(sources, func(i, j int) bool { return sources[i].name < sources[j].name })

	seenPreserve := map[string]struct{}{}
	for _, rel := range options.Preserve {
		rel = filepath.ToSlash(strings.TrimSpace(rel))
		if err := CheckRelpath(rel); err != nil {
			return nil, fmt.Errorf("preserve: %w", err)
		}
		if _, exists := seenPreserve[rel]; exists {
			continue
		}
		seenPreserve[rel] = struct{}{}
		table.Preserve = append(table.Preserve, rel)
	}
	sort.Strings(table.Preserve)

	for _, script := range options.Scripts {
		if script.Phase == updaterv1.ScriptPhase_SCRIPT_PHASE_UNSPECIFIED {
			return nil, fmt.Errorf("script %q has no phase", script.Path)
		}
		if script.Interpreter == updaterv1.ScriptInterpreter_SCRIPT_INTERPRETER_UNSPECIFIED {
			return nil, fmt.Errorf("script %q has no interpreter", script.Path)
		}
		rel := filepath.ToSlash(script.Path)
		if err := CheckRelpath(rel); err != nil {
			return nil, fmt.Errorf("script: %w", err)
		}
		sourcePath := filepath.Join(options.ScriptsDir, filepath.FromSlash(rel))
		info, err := os.Stat(sourcePath)
		if err != nil || info.IsDir() {
			return nil, fmt.Errorf("script not found: %s", sourcePath)
		}
		digest, size, err := model.Sha256File(sourcePath)
		if err != nil {
			return nil, err
		}
		timeout := script.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		table.Scripts = append(table.Scripts, &updaterv1.ScriptEntry{
			Phase:       script.Phase,
			Relpath:     rel,
			Size:        size,
			Sha256:      digest,
			Timeout:     durationpb.New(timeout),
			Interpreter: script.Interpreter,
			Args:        append([]string(nil), script.Args...),
		})
		sources = append(sources, source{name: ScriptsPrefix + rel, path: sourcePath, mode: info.Mode()})
	}
	sort.Slice(table.Scripts, func(i, j int) bool {
		if table.Scripts[i].Phase != table.Scripts[j].Phase {
			return table.Scripts[i].Phase < table.Scripts[j].Phase
		}
		return table.Scripts[i].Relpath < table.Scripts[j].Relpath
	})
	if err := ensurePortableDistinct(table); err != nil {
		return nil, err
	}

	rawTable, err := proto.Marshal(table)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return nil, err
	}
	out, err := os.Create(outPath)
	if err != nil {
		return nil, err
	}
	zw := zip.NewWriter(out)
	if err := writeBytes(zw, TableName, rawTable, 0o644); err != nil {
		zw.Close()
		out.Close()
		return nil, err
	}
	for _, item := range sources {
		if err := writeSource(zw, item.name, item.path, item.mode); err != nil {
			zw.Close()
			out.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		out.Close()
		return nil, err
	}
	if err := out.Close(); err != nil {
		return nil, err
	}
	return table, nil
}

func applyDescriptor(options *BuildOptions) error {
	configPath := filepath.Join(options.Tree, ConfigDir, ConfigName)
	raw, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var descriptor struct {
		Preserve []string `json:"preserve"`
		Scripts  []struct {
			Phase       string   `json:"phase"`
			Path        string   `json:"path"`
			Interpreter string   `json:"interpreter"`
			Timeout     string   `json:"timeout"`
			Args        []string `json:"args"`
		} `json:"scripts"`
	}
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		return fmt.Errorf("%s: %w", configPath, err)
	}
	if len(options.Preserve) == 0 {
		options.Preserve = descriptor.Preserve
	}
	if options.ScriptsDir == "" {
		options.ScriptsDir = filepath.Join(options.Tree, ConfigDir, "scripts")
	}
	if len(options.Scripts) != 0 {
		return nil
	}
	for _, item := range descriptor.Scripts {
		script := Script{Path: item.Path, Args: item.Args}
		switch item.Phase {
		case "pre":
			script.Phase = updaterv1.ScriptPhase_SCRIPT_PHASE_PRE_APPLY
		case "post":
			script.Phase = updaterv1.ScriptPhase_SCRIPT_PHASE_POST_APPLY
		default:
			return fmt.Errorf("%s: script %q has invalid phase %q", configPath, item.Path, item.Phase)
		}
		switch item.Interpreter {
		case "direct":
			script.Interpreter = updaterv1.ScriptInterpreter_SCRIPT_INTERPRETER_DIRECT
		case "sh":
			script.Interpreter = updaterv1.ScriptInterpreter_SCRIPT_INTERPRETER_SH
		case "powershell":
			script.Interpreter = updaterv1.ScriptInterpreter_SCRIPT_INTERPRETER_POWERSHELL
		default:
			return fmt.Errorf("%s: script %q has invalid interpreter %q", configPath, item.Path, item.Interpreter)
		}
		if item.Timeout != "" {
			timeout, err := time.ParseDuration(item.Timeout)
			if err != nil {
				return fmt.Errorf("%s: script %q timeout: %w", configPath, item.Path, err)
			}
			script.Timeout = timeout
		}
		options.Scripts = append(options.Scripts, script)
	}
	return nil
}

func Validate(path string) (*updaterv1.FileTable, error) {
	return inspect(path, "")
}

func Extract(path, dest string) (*Package, error) {
	table, err := inspect(path, dest)
	if err != nil {
		return nil, err
	}
	return &Package{
		Table:       table,
		FilesRoot:   filepath.Join(dest, "files"),
		ScriptsRoot: filepath.Join(dest, "scripts"),
	}, nil
}

func inspect(path, dest string) (*updaterv1.FileTable, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open payload: %w", err)
	}
	defer zr.Close()
	entries := make(map[string]*zip.File, len(zr.File))
	for _, item := range zr.File {
		name := strings.TrimSuffix(item.Name, "/")
		if name == "" || item.FileInfo().IsDir() {
			continue
		}
		if _, exists := entries[name]; exists {
			return nil, fmt.Errorf("duplicate payload entry %q", name)
		}
		if name != TableName {
			prefix := FilesPrefix
			if strings.HasPrefix(name, ScriptsPrefix) {
				prefix = ScriptsPrefix
			} else if !strings.HasPrefix(name, FilesPrefix) {
				return nil, fmt.Errorf("unexpected payload entry %q", name)
			}
			if err := CheckRelpath(strings.TrimPrefix(name, prefix)); err != nil {
				return nil, err
			}
		}
		entries[name] = item
	}
	tableFile := entries[TableName]
	if tableFile == nil {
		return nil, fmt.Errorf("payload is missing %s", TableName)
	}
	raw, err := readZipFile(tableFile)
	if err != nil {
		return nil, err
	}
	table := &updaterv1.FileTable{}
	if err := proto.Unmarshal(raw, table); err != nil {
		return nil, fmt.Errorf("decode %s: %w", TableName, err)
	}
	if table.Schema != Schema {
		return nil, fmt.Errorf("unexpected payload schema %q", table.Schema)
	}
	if err := ensurePortableDistinct(table); err != nil {
		return nil, err
	}
	expected := map[string]struct{}{TableName: {}}
	for _, file := range table.Files {
		if file == nil {
			return nil, fmt.Errorf("payload contains nil file entry")
		}
		if err := CheckRelpath(file.Relpath); err != nil {
			return nil, err
		}
		name := FilesPrefix + file.Relpath
		if _, duplicate := expected[name]; duplicate {
			return nil, fmt.Errorf("duplicate file table path %q", file.Relpath)
		}
		expected[name] = struct{}{}
		if err := verifyZipEntry(entries[name], file.Size, file.Sha256, name); err != nil {
			return nil, err
		}
	}
	for _, script := range table.Scripts {
		if script == nil || script.Phase == updaterv1.ScriptPhase_SCRIPT_PHASE_UNSPECIFIED ||
			script.Interpreter == updaterv1.ScriptInterpreter_SCRIPT_INTERPRETER_UNSPECIFIED {
			return nil, fmt.Errorf("invalid script entry")
		}
		if script.Timeout == nil || script.Timeout.AsDuration() <= 0 {
			return nil, fmt.Errorf("script %q has invalid timeout", script.Relpath)
		}
		if err := CheckRelpath(script.Relpath); err != nil {
			return nil, err
		}
		name := ScriptsPrefix + script.Relpath
		if _, duplicate := expected[name]; duplicate {
			return nil, fmt.Errorf("duplicate script path %q", script.Relpath)
		}
		expected[name] = struct{}{}
		if err := verifyZipEntry(entries[name], script.Size, script.Sha256, name); err != nil {
			return nil, err
		}
	}
	for name := range entries {
		if _, ok := expected[name]; !ok {
			return nil, fmt.Errorf("payload entry %q is not declared", name)
		}
	}
	if dest != "" {
		if err := os.RemoveAll(dest); err != nil {
			return nil, err
		}
		for name, item := range entries {
			if name == TableName {
				continue
			}
			target := filepath.Join(dest, filepath.FromSlash(name))
			if err := extractZipFile(item, target); err != nil {
				return nil, err
			}
		}
	}
	return table, nil
}

func ensurePortableDistinct(table *updaterv1.FileTable) error {
	seen := map[string]string{}
	check := func(kind, rel string) error {
		key := kind + ":" + strings.ToLower(rel)
		if previous, exists := seen[key]; exists {
			return fmt.Errorf("payload paths collide on case-insensitive filesystems: %q and %q", previous, rel)
		}
		seen[key] = rel
		return nil
	}
	for _, file := range table.Files {
		if file != nil {
			if err := check("file", file.Relpath); err != nil {
				return err
			}
		}
	}
	for _, script := range table.Scripts {
		if script != nil {
			if err := check("script", script.Relpath); err != nil {
				return err
			}
		}
	}
	return nil
}

func CheckRelpath(rel string) error {
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, `\`) {
		return fmt.Errorf("invalid relative path %q", rel)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if clean != rel || clean == "." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("invalid relative path %q", rel)
	}
	for _, segment := range strings.Split(rel, "/") {
		if err := model.CheckFilename(segment); err != nil {
			return err
		}
	}
	return nil
}

func verifyZipEntry(item *zip.File, size int64, digest, name string) error {
	if item == nil {
		return fmt.Errorf("payload is missing %q", name)
	}
	if int64(item.UncompressedSize64) != size {
		return fmt.Errorf("payload entry %q size mismatch", name)
	}
	rc, err := item.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, rc)
	if err != nil {
		return err
	}
	if n != size || hex.EncodeToString(hash.Sum(nil)) != digest {
		return fmt.Errorf("payload entry %q sha256 mismatch", name)
	}
	return nil
}

func writeBytes(zw *zip.Writer, name string, data []byte, mode os.FileMode) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(mode)
	header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func writeSource(zw *zip.Writer, name, path string, mode os.FileMode) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(mode)
	header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, in)
	return err
}

func readZipFile(item *zip.File) ([]byte, error) {
	rc, err := item.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func extractZipFile(item *zip.File, target string) error {
	rc, err := item.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	mode := item.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, rc)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
