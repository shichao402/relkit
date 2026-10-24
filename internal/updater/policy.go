package updater

import (
	"time"

	updaterv1 "github.com/shichao402/relkit/api/updater/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

func clampPolicy(p *updaterv1.CheckPolicy) (afterSuccess, afterFailure time.Duration) {
	afterSuccess, afterFailure = DefaultSuccess, DefaultFailure
	if p != nil {
		if p.AfterSuccess != nil {
			afterSuccess = p.AfterSuccess.AsDuration()
		}
		if p.AfterFailure != nil {
			afterFailure = p.AfterFailure.AsDuration()
		}
	}
	if afterSuccess < MinCheckInterval {
		afterSuccess = MinCheckInterval
	}
	if afterFailure < MinCheckInterval {
		afterFailure = MinCheckInterval
	}
	return afterSuccess, afterFailure
}

func shouldCheck(lastCheck *time.Time, last updaterv1.LastResult, policy *updaterv1.CheckPolicy, force bool, now time.Time) (ok bool, next time.Time) {
	afterSuccess, afterFailure := clampPolicy(policy)
	wait := afterSuccess
	if isFailureResult(last) {
		wait = afterFailure
	}
	if lastCheck == nil || lastCheck.IsZero() {
		return true, time.Time{}
	}
	next = lastCheck.Add(wait)
	if force {
		return true, next
	}
	return !now.Before(next), next
}

func minCheckDurationPB() *durationpb.Duration {
	return durationpb.New(MinCheckInterval)
}

func planTTLDurationPB() *durationpb.Duration {
	return durationpb.New(PlanTTL)
}

// isFailureResult lives here (it used to sit next to the legacy state reader)
// because the throttle policy is its only consumer.
func isFailureResult(r updaterv1.LastResult) bool {
	return r == updaterv1.LastResult_LAST_RESULT_FAILED
}
