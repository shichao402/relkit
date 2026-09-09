package updater

import updaterv1 "firoyang.com/relkit/api/updater/v1"

// MapLegacyLastResult maps on-disk SDK strings onto LastResult.
//
// Go SDK wrote: available, fallback, success, failure
// Dart SDK wrote: up-to-date, error, no-artifact, update-available
// Unknown strings become LAST_RESULT_UNSPECIFIED.
func MapLegacyLastResult(s string) updaterv1.LastResult {
	switch s {
	case "available", "update-available":
		return updaterv1.LastResult_LAST_RESULT_UPDATE_AVAILABLE
	case "fallback":
		return updaterv1.LastResult_LAST_RESULT_FALLBACK_REQUIRED
	case "success", "up-to-date":
		return updaterv1.LastResult_LAST_RESULT_UP_TO_DATE
	case "failure", "error", "no-artifact":
		return updaterv1.LastResult_LAST_RESULT_FAILED
	case "throttled":
		return updaterv1.LastResult_LAST_RESULT_THROTTLED
	case "applied":
		return updaterv1.LastResult_LAST_RESULT_APPLIED
	default:
		return updaterv1.LastResult_LAST_RESULT_UNSPECIFIED
	}
}

func isFailureResult(r updaterv1.LastResult) bool {
	return r == updaterv1.LastResult_LAST_RESULT_FAILED
}
