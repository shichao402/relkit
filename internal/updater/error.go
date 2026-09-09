package updater

import (
	updaterv1 "go.firoyang.com/relkit/api/updater/v1"
)

func newError(code updaterv1.ErrorCode, retryable bool, message string, attempts []string) *updaterv1.Error {
	return &updaterv1.Error{
		Code:      code,
		Retryable: retryable,
		Message:   message,
		Attempts:  attempts,
	}
}

func failedEvent(code updaterv1.ErrorCode, retryable bool, message string, attempts []string) *updaterv1.UpdaterEvent {
	return &updaterv1.UpdaterEvent{
		Kind: &updaterv1.UpdaterEvent_Failed{
			Failed: &updaterv1.Failed{Error: newError(code, retryable, message, attempts)},
		},
	}
}

func classifyFetch(err error) (updaterv1.ErrorCode, bool) {
	if err == nil {
		return updaterv1.ErrorCode_ERROR_CODE_UNSPECIFIED, false
	}
	return updaterv1.ErrorCode_ERROR_CODE_NETWORK, true
}
