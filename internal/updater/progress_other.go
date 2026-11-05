//go:build !windows

package updater

// The macOS AppKit window lands with the macOS build runner (ADR 0026 §6);
// until then hide_progress is a no-op off Windows.
func platformProgressUI(progressSpec) (progressUI, error) { return noopProgress{}, nil }

func uiLanguage() string { return langFromLocale(envLocale()) }
