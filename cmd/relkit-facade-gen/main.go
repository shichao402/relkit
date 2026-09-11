package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// relkit-facade-gen writes the locked method/result signature table.
// Every language facade must contain every token in this table.
func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	out := filepath.Join(root, "conformance", "updater", "facade-signatures.txt")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, []byte(signatures), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	files := []string{
		filepath.Join(root, "sdk", "updaterfacade", "facade.go"),
		filepath.Join(root, "sdk", "dart", "lib", "src", "updater_facade.dart"),
		filepath.Join(root, "sdk", "node", "src", "updater_facade.ts"),
		filepath.Join(root, "sdk", "rust", "src", "lib.rs"),
	}
	required := []string{
		"check", "skip", "download", "apply", "status", "cleanup", "cancel",
		"capabilities", "open",
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		body := strings.ToLower(string(raw))
		for _, tok := range required {
			if !strings.Contains(body, strings.ToLower(tok)) {
				fmt.Fprintf(os.Stderr, "%s missing %s\n", f, tok)
				os.Exit(1)
			}
		}
	}
	fmt.Println("wrote", out)
}

const signatures = `methods: open capabilities check skip download apply status cleanup cancel scheduler
results: upToDate updateAvailable fallbackRequired throttled failed
errors: network signature rollbackRejected selectorNoMatch disk permissionDenied occupied protocolMismatch updaterTooOld updaterTooNew planTampered planExpired planUnknown planNotDownloaded skipDenied profileInvalid channelNotAllowed canceled sidecarNotFound layoutUnsupported
ipc: 1
`
