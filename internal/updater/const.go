package updater

import "time"

// Window 4 is the desktop-epoch baseline: OPERATION_SCHEDULER is gone,
// DeleteInstalled exists, and the engine owns Runtime Bootstrap parsing.
// Older windows are rejected outright — this epoch does not migrate hosts.
const (
	IPCMin     uint32 = 4
	IPCMax     uint32 = 4
	IPCCurrent uint32 = 4

	MinCheckInterval = 5 * time.Minute
	DefaultSuccess   = 24 * time.Hour
	DefaultFailure   = time.Hour
	PlanTTL          = 24 * time.Hour

	StateFileName    = "state.pb"
	PlansDirName     = "plans"
	SessionsDirName  = "sessions"
	PlanKeyFileName  = "plan.key"
	ActivePointer    = "active.json"
	RetainRecordName = "retain.json"
	VersionMetaName  = "relkit-version.json"
	ApplyLockName    = ".relkit-apply.lock"
	VersionsDirName  = "versions"
	TrashDirName     = ".trash"
)

var DefaultOperations = []int32{1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 12} // Operation enum values
