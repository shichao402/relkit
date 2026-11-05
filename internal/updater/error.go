package updater

import (
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
)

func newError(code updaterv2.ErrorCode, retryable bool, message string, attempts []string) *updaterv2.Error {
	return &updaterv2.Error{
		Code:      code,
		Retryable: retryable,
		Message:   message,
		Attempts:  attempts,
	}
}

func failedEvent(code updaterv2.ErrorCode, retryable bool, message string, attempts []string) *updaterv2.UpdaterEvent {
	return &updaterv2.UpdaterEvent{
		Kind: &updaterv2.UpdaterEvent_Failed{
			Failed: &updaterv2.Failed{Error: newError(code, retryable, message, attempts)},
		},
	}
}

func classifyFetch(err error) (updaterv2.ErrorCode, bool) {
	if err == nil {
		return updaterv2.ErrorCode_ERROR_CODE_UNSPECIFIED, false
	}
	return updaterv2.ErrorCode_ERROR_CODE_NETWORK, true
}
