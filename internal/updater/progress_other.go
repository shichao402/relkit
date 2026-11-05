//go:build !windows && !(darwin && cgo)

package updater

// Linux has no window, and a darwin build without cgo cannot reach AppKit;
// both run the session without one (ADR 0026 §6).
func platformProgressUI(progressSpec) (progressUI, error) { return noopProgress{}, nil }

func uiLanguage() string { return langFromLocale(envLocale()) }
