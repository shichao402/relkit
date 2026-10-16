// Package diag defines the stable diagnostic envelope shared by vNext core
// and relkit-ops.
package diag

import "fmt"

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type Envelope struct {
	Code     string         `json:"code"`
	Severity Severity       `json:"severity"`
	Stage    string         `json:"stage"`
	Message  string         `json:"message"`
	Evidence map[string]any `json:"evidence,omitempty"`
}

func (d Envelope) Validate() error {
	if d.Code == "" || d.Code == "unclassified" {
		return fmt.Errorf("diagnostic code must be stable and classified")
	}
	switch d.Severity {
	case SeverityInfo, SeverityWarning, SeverityError:
	default:
		return fmt.Errorf("diagnostic %q has invalid severity %q", d.Code, d.Severity)
	}
	if d.Stage == "" {
		return fmt.Errorf("diagnostic %q has no stage", d.Code)
	}
	if d.Message == "" {
		return fmt.Errorf("diagnostic %q has no message", d.Code)
	}
	return nil
}

// Error carries a stable code while preserving a conventional Go error.
type Error struct {
	Diagnostic Envelope
	Cause      error
}

func (e Error) Error() string {
	if e.Cause == nil {
		return e.Diagnostic.Message
	}
	return e.Diagnostic.Message + ": " + e.Cause.Error()
}

func (e Error) Unwrap() error {
	return e.Cause
}
