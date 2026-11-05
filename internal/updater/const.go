package updater

import "time"

// Protocol 5 (ADR 0026): relkit.updater.v2 over ProtoJSON lines. The engine
// speaks exactly one protocol; hosts on any other value are rejected with
// UPDATER_TOO_OLD / UPDATER_TOO_NEW. This epoch does not migrate hosts.
const (
	Protocol uint32 = 5

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

var DefaultOperations = []int32{1, 2, 3, 4, 5, 6, 9, 10, 11, 12} // Operation enum values
