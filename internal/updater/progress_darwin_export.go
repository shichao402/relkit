//go:build darwin && cgo

package updater

// The AppKit callbacks live in their own file: a cgo file with //export may
// only declare, not define, C functions in its preamble.

import "C"

//export relkitProgressReady
func relkitProgressReady() {
	if w := currentMac(); w != nil {
		w.onReady()
	}
}

//export relkitProgressButton
func relkitProgressButton() {
	if w := currentMac(); w != nil {
		w.onButton()
	}
}

//export relkitProgressHelp
func relkitProgressHelp() {
	if w := currentMac(); w != nil {
		w.openHelp()
	}
}
