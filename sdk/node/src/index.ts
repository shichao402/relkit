/**
 * Host-facing Node surface: protocol helpers plus the generated updater facade.
 *
 * Check/download/apply for products goes through `updater_facade.ts` and the
 * sidecar. The in-process class in `updater.ts` is not part of this barrel.
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
  type UpdateRuntimeConfig,
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
export { directoryServiceKey } from "./preference.js";
export { Updater, ipcMin, ipcMax, ipcCurrent, defaultGlue } from "./updater_facade.js";
