package updater

import "time"

const (
	IPCMin     uint32 = 1
	IPCMax     uint32 = 1
	IPCCurrent uint32 = 1

	MinCheckInterval = 5 * time.Minute
	DefaultSuccess   = 24 * time.Hour
	DefaultFailure   = time.Hour
	PlanTTL          = 24 * time.Hour

	StateFileName    = "state.pb"
	LegacyStateJSON  = "state.json"
	PlansDirName     = "plans"
	SessionsDirName  = "sessions"
	PlanKeyFileName  = "plan.key"
	JSONSessionName  = "update_apply.json"
	ActivePointer    = "active.json"
	RetainRecordName = "retain.json"
)

var DefaultOperations = []int32{1, 2, 3, 4, 5, 6, 7, 8} // Operation enum values
