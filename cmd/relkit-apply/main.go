// Command relkit-apply copies a staged payload into versions/<id>/ and
// switches active.json. It exists so a Flutter host does not have to start a
// second copy of itself to apply a versionedDir update.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	req, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := apply(req); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if req.sessionPath != "" {
			_ = writeSession(req, "failed", err.Error())
		}
		return 1
	}
	return 0
}

type request struct {
	installDir  string
	stagedRoot  string
	executable  string
	layout      string
	version     string
	code        int
	retain      int
	sessionPath string
	logPath     string
	relaunch    bool
}

func parseArgs(args []string) (*request, error) {
	req := &request{layout: "wholeRoot", retain: 2, relaunch: true}
	value := func(name string) string {
		for i, a := range args {
			if a == name && i+1 < len(args) {
				return args[i+1]
			}
		}
		return ""
	}
	has := func(name string) bool {
		for _, a := range args {
			if a == name {
				return true
			}
		}
		return false
	}
	if !has("--rup-apply") && !has("apply") {
		// Allow both `relkit-apply --rup-apply ...` and `relkit-apply apply ...`.
	}
	req.installDir = value("--install-dir")
	req.stagedRoot = value("--staged-root")
	req.executable = value("--executable")
	if layout := value("--layout"); layout != "" {
		req.layout = layout
	}
	req.version = value("--target-version")
	if code := value("--target-code"); code != "" {
		n, err := strconv.Atoi(code)
		if err != nil {
			return nil, fmt.Errorf("--target-code: %w", err)
		}
		req.code = n
	}
	if retain := value("--retain-versions"); retain != "" {
		n, err := strconv.Atoi(retain)
		if err != nil {
			return nil, fmt.Errorf("--retain-versions: %w", err)
		}
		req.retain = n
	}
	req.sessionPath = value("--apply-session")
	req.logPath = value("--apply-log")
	if has("--no-relaunch") {
		req.relaunch = false
	}
	if req.installDir == "" || req.stagedRoot == "" || req.executable == "" {
		return nil, fmt.Errorf("need --install-dir, --staged-root, --executable")
	}
	if req.layout == "versionedDir" && (req.version == "" || req.code == 0) {
		return nil, fmt.Errorf("versionedDir needs --target-version and --target-code")
	}
	if req.layout == "versionedDir" && strings.Contains(strings.ToLower(req.installDir), ".app") {
		return nil, fmt.Errorf("versionedDir is not supported inside a .app bundle")
	}
	return req, nil
}

func apply(req *request) error {
	note := func(msg string) {
		line := time.Now().Format(time.RFC3339) + " " + msg + "\n"
		if req.logPath == "" {
			return
		}
		_ = os.MkdirAll(filepath.Dir(req.logPath), 0o755)
		f, err := os.OpenFile(req.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		_, _ = f.WriteString(line)
		_ = f.Close()
	}
	if req.sessionPath != "" {
		_ = writeSession(req, "running", "")
	}
	if req.layout != "versionedDir" {
		return fmt.Errorf("this binary only implements versionedDir; use the Dart host apply for wholeRoot")
	}
	payload := filepath.Join(req.stagedRoot, "versions", req.version)
	if st, err := os.Stat(payload); err != nil || !st.IsDir() {
		payload = req.stagedRoot
	}
	dest := filepath.Join(req.installDir, "versions", req.version)
	note("copying " + payload + " -> " + dest)
	if err := os.RemoveAll(dest); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := copyDir(payload, dest); err != nil {
		_ = os.RemoveAll(dest)
		return err
	}
	pointer := map[string]any{
		"code":    req.code,
		"version": req.version,
		"path":    "versions/" + req.version,
		"executable": "versions/" + req.version + "/" +
			strings.ReplaceAll(req.executable, "\\", "/"),
	}
	if err := writeActive(req.installDir, pointer); err != nil {
		_ = os.RemoveAll(dest)
		return err
	}
	if nested, err := os.Stat(filepath.Join(req.stagedRoot, "versions", req.version)); err == nil && nested.IsDir() {
		refreshRootFiles(req.installDir, req.stagedRoot, req.executable, req.code, note)
	}
	pruneVersions(filepath.Join(req.installDir, "versions"), req.version, req.retain, note)
	if req.sessionPath != "" {
		_ = writeSession(req, "succeeded", "")
	}
	if req.relaunch {
		launcher := filepath.Join(req.installDir, filepath.Base(req.executable))
		note("starting " + launcher)
		cmd := exec.Command(launcher)
		cmd.Dir = req.installDir
		if err := cmd.Start(); err != nil {
			return err
		}
	}
	return nil
}

func refreshRootFiles(installDir, stagedRoot, executable string, code int, note func(string)) {
	applyName := "relkit-apply"
	if runtime.GOOS == "windows" {
		applyName = "relkit-apply.exe"
	}
	names := []string{filepath.Base(executable), applyName}
	seen := map[string]bool{}
	for _, name := range names {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		source := filepath.Join(stagedRoot, name)
		info, err := os.Stat(source)
		if err != nil || info.IsDir() {
			continue
		}
		dest := filepath.Join(installDir, name)
		pruneAsideFiles(installDir, name, note)
		if sameFileBytes(source, dest) {
			note("root file " + name + " unchanged")
			continue
		}
		aside := dest + ".old-" + strconv.Itoa(code)
		_ = os.Remove(aside)
		if _, err := os.Stat(dest); err == nil {
			if err := os.Rename(dest, aside); err != nil {
				note("could not refresh " + name + ": " + err.Error())
				continue
			}
		}
		if err := copyFile(source, dest); err != nil {
			if _, destErr := os.Stat(dest); os.IsNotExist(destErr) {
				_ = os.Rename(aside, dest)
			}
			note("could not refresh " + name + ": " + err.Error())
			continue
		}
		note("refreshed " + name)
		if err := os.Remove(aside); err != nil && !os.IsNotExist(err) {
			note("left aside " + aside)
		}
	}
}

func pruneAsideFiles(installDir, fileName string, note func(string)) {
	prefix := fileName + ".old-"
	entries, err := os.ReadDir(installDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		path := filepath.Join(installDir, e.Name())
		if err := os.Remove(path); err != nil {
			continue
		}
		note("removed leftover " + e.Name())
	}
}

func sameFileBytes(a, b string) bool {
	left, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	right, err := os.ReadFile(b)
	if err != nil {
		return false
	}
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}

func writeActive(installDir string, pointer map[string]any) error {
	path := filepath.Join(installDir, "active.json")
	tmp := path + ".tmp"
	raw, err := json.Marshal(pointer)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

type sessionFile struct {
	State         string `json:"state"`
	Pid           int    `json:"pid"`
	StartedAt     string `json:"startedAt"`
	UpdatedAt     string `json:"updatedAt"`
	InstallDir    string `json:"installDir"`
	StagedRoot    string `json:"stagedRoot"`
	TargetCode    int    `json:"targetCode,omitempty"`
	TargetVersion string `json:"targetVersion,omitempty"`
	Message       string `json:"message,omitempty"`
}

func writeSession(req *request, state, message string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	doc := sessionFile{
		State:         state,
		Pid:           os.Getpid(),
		StartedAt:     now,
		UpdatedAt:     now,
		InstallDir:    req.installDir,
		StagedRoot:    req.stagedRoot,
		TargetCode:    req.code,
		TargetVersion: req.version,
		Message:       message,
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(req.sessionPath), 0o755)
	return os.WriteFile(req.sessionPath, raw, 0o644)
}

func pruneVersions(root, current string, retain int, note func(string)) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	keep := map[string]bool{current: true}
	for _, e := range entries {
		if e.IsDir() && e.Name() != current && len(keep) < retain {
			keep[e.Name()] = true
		}
	}
	for _, e := range entries {
		if !e.IsDir() || keep[e.Name()] {
			continue
		}
		note("removing retired version " + e.Name())
		_ = os.RemoveAll(filepath.Join(root, e.Name()))
	}
}

func copyDir(from, to string) error {
	return filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(to, rel)
		if info.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(target, dest)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		cerr := out.Close()
		if err != nil {
			return err
		}
		return cerr
	})
}
