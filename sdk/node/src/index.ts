/**
 * A RUP client for Node: check for an update, verify it, download it.
 *
 * The protocol contract lives in relkit's `SPEC.md`; this package implements the
 * client half of it. Everything up to and including "here is a file whose sha256
 * matches a signed manifest" is protocol, and is what `updater.ts` provides.
 *
 * Applying that file is deliberately NOT in this package. An Electron host that
 * replaces its own directory has to solve locked files, a stable launcher path,
 * single-instance locks and login-item registration, none of which are protocol
 * and all of which differ per host. Use the verified file directly.
 */

export {
  selectNextTarget,
  resolveUpgradePath,
  isMandatory,
} from "./chain.js";
export {
  downloadArtifact,
  VerificationError,
  type VerifiedFile,
} from "./download.js";
export {
  openEnvelope,
  describeEnvelopeResult,
  toTrustedKeys,
  TrustedKeys,
  type EnvelopeRejection,
  type EnvelopeResult,
  type TrustedKeysInput,
} from "./envelope.js";
export {
  cacheBust,
  FetchError,
  HttpFetcher,
  MAX_REDIRECTS,
  sha256OfBytes,
  sha256OfFile,
  ThroughputMeter,
  type DownloadProgress,
  type Fetcher,
  type ProgressCallback,
  type ResourceProbe,
} from "./fetch.js";
export { checkArtifactFilename } from "./filename.js";
export * from "./models.js";
export { rankByLearning, rankUrlStrings } from "./preference.js";
export {
  collectPriorReleaseNotes,
  resolveReleaseNotesMarkdown,
  type PriorReleaseNotes,
} from "./release-notes.js";
export {
  defaultRuntimeConfig,
  runtimeConfigFromJson,
  UpdateScheduler,
  type UpdateRuntimeConfig,
  type UpdateSchedulerOptions,
} from "./scheduler.js";
export {
  matchesSelectors,
  matchingArtifacts,
  selectArtifact,
} from "./selectors.js";
export {
  acceptsSequence,
  defaultUpdatePolicy,
  FileUpdateStateStore,
  MemoryUpdateStateStore,
  resolvePolicy,
  shouldCheck,
  UpdateState,
  type SourceStat,
  type UpdatePolicy,
  type UpdateStateStore,
} from "./state.js";
export {
  directoryServiceKey,
  RupUpdater,
  type CheckFailed,
  type CheckThrottled,
  type FallbackRequired,
  type RecoveryHelp,
  type RecoveryLink,
  type RupUpdaterOptions,
  type UpdateAvailable,
  type UpdateCheckResult,
  type UpToDate,
} from "./updater.js";
