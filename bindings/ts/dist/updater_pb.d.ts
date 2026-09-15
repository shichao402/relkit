import type { GenEnum, GenFile, GenMessage } from "@bufbuild/protobuf/codegenv2";
import type { Duration, Timestamp } from "@bufbuild/protobuf/wkt";
import type { Message } from "@bufbuild/protobuf";
/**
 * Describes the file updater/v1/updater.proto.
 */
export declare const file_updater_v1_updater: GenFile;
/**
 * @generated from message relkit.updater.v1.TrustedKey
 */
export type TrustedKey = Message<"relkit.updater.v1.TrustedKey"> & {
    /**
     * @generated from field: string key_id = 1;
     */
    keyId: string;
    /**
     * raw 32-byte ed25519
     *
     * @generated from field: bytes public_key = 2;
     */
    publicKey: Uint8Array;
};
/**
 * Describes the message relkit.updater.v1.TrustedKey.
 * Use `create(TrustedKeySchema)` to create a new message.
 */
export declare const TrustedKeySchema: GenMessage<TrustedKey>;
/**
 * @generated from message relkit.updater.v1.RecoveryLink
 */
export type RecoveryLink = Message<"relkit.updater.v1.RecoveryLink"> & {
    /**
     * @generated from field: string label = 1;
     */
    label: string;
    /**
     * @generated from field: string url = 2;
     */
    url: string;
};
/**
 * Describes the message relkit.updater.v1.RecoveryLink.
 * Use `create(RecoveryLinkSchema)` to create a new message.
 */
export declare const RecoveryLinkSchema: GenMessage<RecoveryLink>;
/**
 * @generated from message relkit.updater.v1.RecoveryHelp
 */
export type RecoveryHelp = Message<"relkit.updater.v1.RecoveryHelp"> & {
    /**
     * @generated from field: string message = 1;
     */
    message: string;
    /**
     * @generated from field: repeated relkit.updater.v1.RecoveryLink links = 2;
     */
    links: RecoveryLink[];
};
/**
 * Describes the message relkit.updater.v1.RecoveryHelp.
 * Use `create(RecoveryHelpSchema)` to create a new message.
 */
export declare const RecoveryHelpSchema: GenMessage<RecoveryHelp>;
/**
 * @generated from message relkit.updater.v1.ClientProfile
 */
export type ClientProfile = Message<"relkit.updater.v1.ClientProfile"> & {
    /**
     * @generated from field: string product = 1;
     */
    product: string;
    /**
     * @generated from field: repeated string allowed_channels = 2;
     */
    allowedChannels: string[];
    /**
     * @generated from field: repeated string entry_urls = 3;
     */
    entryUrls: string[];
    /**
     * @generated from field: repeated string index_urls = 4;
     */
    indexUrls: string[];
    /**
     * @generated from field: repeated string fallback_urls = 5;
     */
    fallbackUrls: string[];
    /**
     * @generated from field: repeated relkit.updater.v1.TrustedKey trusted_keys = 6;
     */
    trustedKeys: TrustedKey[];
    /**
     * @generated from field: relkit.updater.v1.RecoveryHelp recovery = 7;
     */
    recovery?: RecoveryHelp | undefined;
};
/**
 * Describes the message relkit.updater.v1.ClientProfile.
 * Use `create(ClientProfileSchema)` to create a new message.
 */
export declare const ClientProfileSchema: GenMessage<ClientProfile>;
/**
 * @generated from message relkit.updater.v1.FileSetEntry
 */
export type FileSetEntry = Message<"relkit.updater.v1.FileSetEntry"> & {
    /**
     * @generated from field: string dest_relpath = 1;
     */
    destRelpath: string;
    /**
     * @generated from field: string artifact_name = 2;
     */
    artifactName: string;
};
/**
 * Describes the message relkit.updater.v1.FileSetEntry.
 * Use `create(FileSetEntrySchema)` to create a new message.
 */
export declare const FileSetEntrySchema: GenMessage<FileSetEntry>;
/**
 * @generated from message relkit.updater.v1.InstallSpec
 */
export type InstallSpec = Message<"relkit.updater.v1.InstallSpec"> & {
    /**
     * @generated from field: relkit.updater.v1.Layout layout = 1;
     */
    layout: Layout;
    /**
     * @generated from field: string install_root = 2;
     */
    installRoot: string;
    /**
     * @generated from field: string executable_relpath = 3;
     */
    executableRelpath: string;
    /**
     * @generated from field: string sidecar_relpath = 4;
     */
    sidecarRelpath: string;
    /**
     * @generated from field: repeated string preserve = 5;
     */
    preserve: string[];
    /**
     * 0 means current + previous
     *
     * @generated from field: int32 retain = 6;
     */
    retain: number;
    /**
     * @generated from field: bool relaunch = 7;
     */
    relaunch: boolean;
    /**
     * @generated from field: repeated relkit.updater.v1.FileSetEntry file_set = 8;
     */
    fileSet: FileSetEntry[];
};
/**
 * Describes the message relkit.updater.v1.InstallSpec.
 * Use `create(InstallSpecSchema)` to create a new message.
 */
export declare const InstallSpecSchema: GenMessage<InstallSpec>;
/**
 * @generated from message relkit.updater.v1.Runtime
 */
export type Runtime = Message<"relkit.updater.v1.Runtime"> & {
    /**
     * @generated from field: string channel = 1;
     */
    channel: string;
    /**
     * @generated from field: int64 current_code = 2;
     */
    currentCode: bigint;
    /**
     * @generated from field: map<string, string> client_selectors = 3;
     */
    clientSelectors: {
        [key: string]: string;
    };
    /**
     * @generated from field: string data_dir = 4;
     */
    dataDir: string;
    /**
     * @generated from field: relkit.updater.v1.InstallSpec install = 5;
     */
    install?: InstallSpec | undefined;
    /**
     * empty = search order
     *
     * @generated from field: string sidecar_path = 6;
     */
    sidecarPath: string;
};
/**
 * Describes the message relkit.updater.v1.Runtime.
 * Use `create(RuntimeSchema)` to create a new message.
 */
export declare const RuntimeSchema: GenMessage<Runtime>;
/**
 * @generated from message relkit.updater.v1.CheckPolicy
 */
export type CheckPolicy = Message<"relkit.updater.v1.CheckPolicy"> & {
    /**
     * @generated from field: google.protobuf.Duration after_success = 1;
     */
    afterSuccess?: Duration | undefined;
    /**
     * @generated from field: google.protobuf.Duration after_failure = 2;
     */
    afterFailure?: Duration | undefined;
};
/**
 * Describes the message relkit.updater.v1.CheckPolicy.
 * Use `create(CheckPolicySchema)` to create a new message.
 */
export declare const CheckPolicySchema: GenMessage<CheckPolicy>;
/**
 * @generated from message relkit.updater.v1.SchedulerConfig
 */
export type SchedulerConfig = Message<"relkit.updater.v1.SchedulerConfig"> & {
    /**
     * @generated from field: bool check_on_start = 1;
     */
    checkOnStart: boolean;
    /**
     * @generated from field: bool force_on_start = 2;
     */
    forceOnStart: boolean;
    /**
     * @generated from field: relkit.updater.v1.CheckPolicy policy = 3;
     */
    policy?: CheckPolicy | undefined;
};
/**
 * Describes the message relkit.updater.v1.SchedulerConfig.
 * Use `create(SchedulerConfigSchema)` to create a new message.
 */
export declare const SchedulerConfigSchema: GenMessage<SchedulerConfig>;
/**
 * @generated from message relkit.updater.v1.ClientHello
 */
export type ClientHello = Message<"relkit.updater.v1.ClientHello"> & {
    /**
     * @generated from field: uint32 ipc_min = 1;
     */
    ipcMin: number;
    /**
     * @generated from field: uint32 ipc_max = 2;
     */
    ipcMax: number;
};
/**
 * Describes the message relkit.updater.v1.ClientHello.
 * Use `create(ClientHelloSchema)` to create a new message.
 */
export declare const ClientHelloSchema: GenMessage<ClientHello>;
/**
 * @generated from message relkit.updater.v1.Capabilities
 */
export type Capabilities = Message<"relkit.updater.v1.Capabilities"> & {
    /**
     * @generated from field: uint32 ipc = 1;
     */
    ipc: number;
    /**
     * @generated from field: repeated relkit.updater.v1.Operation operations = 2;
     */
    operations: Operation[];
    /**
     * @generated from field: repeated relkit.updater.v1.Layout layouts = 3;
     */
    layouts: Layout[];
    /**
     * @generated from field: google.protobuf.Duration min_check_interval = 4;
     */
    minCheckInterval?: Duration | undefined;
    /**
     * @generated from field: google.protobuf.Duration plan_ttl = 5;
     */
    planTtl?: Duration | undefined;
    /**
     * @generated from field: string engine_version = 6;
     */
    engineVersion: string;
};
/**
 * Describes the message relkit.updater.v1.Capabilities.
 * Use `create(CapabilitiesSchema)` to create a new message.
 */
export declare const CapabilitiesSchema: GenMessage<Capabilities>;
/**
 * @generated from message relkit.updater.v1.CheckOp
 */
export type CheckOp = Message<"relkit.updater.v1.CheckOp"> & {
    /**
     * @generated from field: bool force = 1;
     */
    force: boolean;
    /**
     * @generated from field: int64 exact_code = 2;
     */
    exactCode: bigint;
    /**
     * @generated from field: relkit.updater.v1.CheckPolicy policy = 3;
     */
    policy?: CheckPolicy | undefined;
};
/**
 * Describes the message relkit.updater.v1.CheckOp.
 * Use `create(CheckOpSchema)` to create a new message.
 */
export declare const CheckOpSchema: GenMessage<CheckOp>;
/**
 * @generated from message relkit.updater.v1.SkipOp
 */
export type SkipOp = Message<"relkit.updater.v1.SkipOp"> & {
    /**
     * @generated from field: int64 code = 1;
     */
    code: bigint;
};
/**
 * Describes the message relkit.updater.v1.SkipOp.
 * Use `create(SkipOpSchema)` to create a new message.
 */
export declare const SkipOpSchema: GenMessage<SkipOp>;
/**
 * @generated from message relkit.updater.v1.DownloadOp
 */
export type DownloadOp = Message<"relkit.updater.v1.DownloadOp"> & {
    /**
     * @generated from field: string plan_id = 1;
     */
    planId: string;
};
/**
 * Describes the message relkit.updater.v1.DownloadOp.
 * Use `create(DownloadOpSchema)` to create a new message.
 */
export declare const DownloadOpSchema: GenMessage<DownloadOp>;
/**
 * @generated from message relkit.updater.v1.ApplyOp
 */
export type ApplyOp = Message<"relkit.updater.v1.ApplyOp"> & {
    /**
     * @generated from field: string plan_id = 1;
     */
    planId: string;
};
/**
 * Describes the message relkit.updater.v1.ApplyOp.
 * Use `create(ApplyOpSchema)` to create a new message.
 */
export declare const ApplyOpSchema: GenMessage<ApplyOp>;
/**
 * @generated from message relkit.updater.v1.StatusOp
 */
export type StatusOp = Message<"relkit.updater.v1.StatusOp"> & {};
/**
 * Describes the message relkit.updater.v1.StatusOp.
 * Use `create(StatusOpSchema)` to create a new message.
 */
export declare const StatusOpSchema: GenMessage<StatusOp>;
/**
 * @generated from message relkit.updater.v1.CleanupOp
 */
export type CleanupOp = Message<"relkit.updater.v1.CleanupOp"> & {};
/**
 * Describes the message relkit.updater.v1.CleanupOp.
 * Use `create(CleanupOpSchema)` to create a new message.
 */
export declare const CleanupOpSchema: GenMessage<CleanupOp>;
/**
 * @generated from message relkit.updater.v1.CancelOp
 */
export type CancelOp = Message<"relkit.updater.v1.CancelOp"> & {};
/**
 * Describes the message relkit.updater.v1.CancelOp.
 * Use `create(CancelOpSchema)` to create a new message.
 */
export declare const CancelOpSchema: GenMessage<CancelOp>;
/**
 * @generated from message relkit.updater.v1.UpdaterRequest
 */
export type UpdaterRequest = Message<"relkit.updater.v1.UpdaterRequest"> & {
    /**
     * @generated from field: relkit.updater.v1.ClientHello hello = 1;
     */
    hello?: ClientHello | undefined;
    /**
     * @generated from field: relkit.updater.v1.ClientProfile profile = 2;
     */
    profile?: ClientProfile | undefined;
    /**
     * @generated from field: relkit.updater.v1.Runtime runtime = 3;
     */
    runtime?: Runtime | undefined;
    /**
     * @generated from oneof relkit.updater.v1.UpdaterRequest.op
     */
    op: {
        /**
         * @generated from field: relkit.updater.v1.CheckOp check = 10;
         */
        value: CheckOp;
        case: "check";
    } | {
        /**
         * @generated from field: relkit.updater.v1.SkipOp skip = 11;
         */
        value: SkipOp;
        case: "skip";
    } | {
        /**
         * @generated from field: relkit.updater.v1.DownloadOp download = 12;
         */
        value: DownloadOp;
        case: "download";
    } | {
        /**
         * @generated from field: relkit.updater.v1.ApplyOp apply = 13;
         */
        value: ApplyOp;
        case: "apply";
    } | {
        /**
         * @generated from field: relkit.updater.v1.StatusOp status = 14;
         */
        value: StatusOp;
        case: "status";
    } | {
        /**
         * @generated from field: relkit.updater.v1.CleanupOp cleanup = 15;
         */
        value: CleanupOp;
        case: "cleanup";
    } | {
        /**
         * @generated from field: relkit.updater.v1.CancelOp cancel = 16;
         */
        value: CancelOp;
        case: "cancel";
    } | {
        case: undefined;
        value?: undefined;
    };
};
/**
 * Describes the message relkit.updater.v1.UpdaterRequest.
 * Use `create(UpdaterRequestSchema)` to create a new message.
 */
export declare const UpdaterRequestSchema: GenMessage<UpdaterRequest>;
/**
 * @generated from message relkit.updater.v1.Error
 */
export type Error = Message<"relkit.updater.v1.Error"> & {
    /**
     * @generated from field: relkit.updater.v1.ErrorCode code = 1;
     */
    code: ErrorCode;
    /**
     * @generated from field: bool retryable = 2;
     */
    retryable: boolean;
    /**
     * @generated from field: string message = 3;
     */
    message: string;
    /**
     * @generated from field: repeated string attempts = 4;
     */
    attempts: string[];
    /**
     * @generated from field: relkit.updater.v1.RecoveryHelp recovery = 5;
     */
    recovery?: RecoveryHelp | undefined;
};
/**
 * Describes the message relkit.updater.v1.Error.
 * Use `create(ErrorSchema)` to create a new message.
 */
export declare const ErrorSchema: GenMessage<Error>;
/**
 * @generated from message relkit.updater.v1.PriorReleaseNotes
 */
export type PriorReleaseNotes = Message<"relkit.updater.v1.PriorReleaseNotes"> & {
    /**
     * @generated from field: string version = 1;
     */
    version: string;
    /**
     * @generated from field: int64 code = 2;
     */
    code: bigint;
    /**
     * @generated from field: string notes = 3;
     */
    notes: string;
    /**
     * @generated from field: string notes_url = 4;
     */
    notesUrl: string;
};
/**
 * Describes the message relkit.updater.v1.PriorReleaseNotes.
 * Use `create(PriorReleaseNotesSchema)` to create a new message.
 */
export declare const PriorReleaseNotesSchema: GenMessage<PriorReleaseNotes>;
/**
 * @generated from message relkit.updater.v1.ArtifactView
 */
export type ArtifactView = Message<"relkit.updater.v1.ArtifactView"> & {
    /**
     * @generated from field: string name = 1;
     */
    name: string;
    /**
     * @generated from field: int64 size = 2;
     */
    size: bigint;
    /**
     * @generated from field: bytes sha256 = 3;
     */
    sha256: Uint8Array;
};
/**
 * Describes the message relkit.updater.v1.ArtifactView.
 * Use `create(ArtifactViewSchema)` to create a new message.
 */
export declare const ArtifactViewSchema: GenMessage<ArtifactView>;
/**
 * @generated from message relkit.updater.v1.UpToDate
 */
export type UpToDate = Message<"relkit.updater.v1.UpToDate"> & {
    /**
     * @generated from field: int64 sequence = 1;
     */
    sequence: bigint;
    /**
     * @generated from field: bool current_is_yanked = 2;
     */
    currentIsYanked: boolean;
};
/**
 * Describes the message relkit.updater.v1.UpToDate.
 * Use `create(UpToDateSchema)` to create a new message.
 */
export declare const UpToDateSchema: GenMessage<UpToDate>;
/**
 * @generated from message relkit.updater.v1.UpdateAvailable
 */
export type UpdateAvailable = Message<"relkit.updater.v1.UpdateAvailable"> & {
    /**
     * @generated from field: string plan_id = 1;
     */
    planId: string;
    /**
     * @generated from field: string prompt_key = 2;
     */
    promptKey: string;
    /**
     * @generated from field: string version = 3;
     */
    version: string;
    /**
     * @generated from field: int64 code = 4;
     */
    code: bigint;
    /**
     * @generated from field: bool mandatory = 5;
     */
    mandatory: boolean;
    /**
     * @generated from field: int32 remaining_hops = 6;
     */
    remainingHops: number;
    /**
     * @generated from field: int64 sequence = 7;
     */
    sequence: bigint;
    /**
     * @generated from field: string release_notes_markdown = 8;
     */
    releaseNotesMarkdown: string;
    /**
     * @generated from field: string release_notes_url = 9;
     */
    releaseNotesUrl: string;
    /**
     * @generated from field: repeated relkit.updater.v1.PriorReleaseNotes prior_release_notes = 10;
     */
    priorReleaseNotes: PriorReleaseNotes[];
    /**
     * @generated from field: repeated relkit.updater.v1.ArtifactView artifacts = 11;
     */
    artifacts: ArtifactView[];
};
/**
 * Describes the message relkit.updater.v1.UpdateAvailable.
 * Use `create(UpdateAvailableSchema)` to create a new message.
 */
export declare const UpdateAvailableSchema: GenMessage<UpdateAvailable>;
/**
 * @generated from message relkit.updater.v1.FallbackRequired
 */
export type FallbackRequired = Message<"relkit.updater.v1.FallbackRequired"> & {
    /**
     * @generated from field: string prompt_key = 1;
     */
    promptKey: string;
    /**
     * @generated from field: string manual_url = 2;
     */
    manualUrl: string;
    /**
     * @generated from field: string message = 3;
     */
    message: string;
    /**
     * @generated from field: bool mandatory = 4;
     */
    mandatory: boolean;
    /**
     * @generated from field: int64 sequence = 5;
     */
    sequence: bigint;
    /**
     * @generated from field: int64 min_code = 6;
     */
    minCode: bigint;
    /**
     * @generated from field: int64 max_code = 7;
     */
    maxCode: bigint;
};
/**
 * Describes the message relkit.updater.v1.FallbackRequired.
 * Use `create(FallbackRequiredSchema)` to create a new message.
 */
export declare const FallbackRequiredSchema: GenMessage<FallbackRequired>;
/**
 * @generated from message relkit.updater.v1.Throttled
 */
export type Throttled = Message<"relkit.updater.v1.Throttled"> & {
    /**
     * @generated from field: google.protobuf.Timestamp next_allowed_at = 1;
     */
    nextAllowedAt?: Timestamp | undefined;
};
/**
 * Describes the message relkit.updater.v1.Throttled.
 * Use `create(ThrottledSchema)` to create a new message.
 */
export declare const ThrottledSchema: GenMessage<Throttled>;
/**
 * @generated from message relkit.updater.v1.Failed
 */
export type Failed = Message<"relkit.updater.v1.Failed"> & {
    /**
     * @generated from field: relkit.updater.v1.Error error = 1;
     */
    error?: Error | undefined;
};
/**
 * Describes the message relkit.updater.v1.Failed.
 * Use `create(FailedSchema)` to create a new message.
 */
export declare const FailedSchema: GenMessage<Failed>;
/**
 * @generated from message relkit.updater.v1.CheckResult
 */
export type CheckResult = Message<"relkit.updater.v1.CheckResult"> & {
    /**
     * @generated from oneof relkit.updater.v1.CheckResult.kind
     */
    kind: {
        /**
         * @generated from field: relkit.updater.v1.UpToDate up_to_date = 1;
         */
        value: UpToDate;
        case: "upToDate";
    } | {
        /**
         * @generated from field: relkit.updater.v1.UpdateAvailable update_available = 2;
         */
        value: UpdateAvailable;
        case: "updateAvailable";
    } | {
        /**
         * @generated from field: relkit.updater.v1.FallbackRequired fallback_required = 3;
         */
        value: FallbackRequired;
        case: "fallbackRequired";
    } | {
        /**
         * @generated from field: relkit.updater.v1.Throttled throttled = 4;
         */
        value: Throttled;
        case: "throttled";
    } | {
        /**
         * @generated from field: relkit.updater.v1.Failed failed = 5;
         */
        value: Failed;
        case: "failed";
    } | {
        case: undefined;
        value?: undefined;
    };
};
/**
 * Describes the message relkit.updater.v1.CheckResult.
 * Use `create(CheckResultSchema)` to create a new message.
 */
export declare const CheckResultSchema: GenMessage<CheckResult>;
/**
 * @generated from message relkit.updater.v1.Downloaded
 */
export type Downloaded = Message<"relkit.updater.v1.Downloaded"> & {
    /**
     * @generated from field: string plan_id = 1;
     */
    planId: string;
    /**
     * @generated from field: int64 bytes = 2;
     */
    bytes: bigint;
};
/**
 * Describes the message relkit.updater.v1.Downloaded.
 * Use `create(DownloadedSchema)` to create a new message.
 */
export declare const DownloadedSchema: GenMessage<Downloaded>;
/**
 * @generated from message relkit.updater.v1.DownloadResult
 */
export type DownloadResult = Message<"relkit.updater.v1.DownloadResult"> & {
    /**
     * @generated from oneof relkit.updater.v1.DownloadResult.kind
     */
    kind: {
        /**
         * @generated from field: relkit.updater.v1.Downloaded downloaded = 1;
         */
        value: Downloaded;
        case: "downloaded";
    } | {
        /**
         * @generated from field: relkit.updater.v1.Failed failed = 2;
         */
        value: Failed;
        case: "failed";
    } | {
        case: undefined;
        value?: undefined;
    };
};
/**
 * Describes the message relkit.updater.v1.DownloadResult.
 * Use `create(DownloadResultSchema)` to create a new message.
 */
export declare const DownloadResultSchema: GenMessage<DownloadResult>;
/**
 * @generated from message relkit.updater.v1.ApplyAccepted
 */
export type ApplyAccepted = Message<"relkit.updater.v1.ApplyAccepted"> & {
    /**
     * @generated from field: string session_id = 1;
     */
    sessionId: string;
    /**
     * @generated from field: string plan_id = 2;
     */
    planId: string;
    /**
     * @generated from field: bool requires_host_exit = 3;
     */
    requiresHostExit: boolean;
};
/**
 * Describes the message relkit.updater.v1.ApplyAccepted.
 * Use `create(ApplyAcceptedSchema)` to create a new message.
 */
export declare const ApplyAcceptedSchema: GenMessage<ApplyAccepted>;
/**
 * @generated from message relkit.updater.v1.ApplyResult
 */
export type ApplyResult = Message<"relkit.updater.v1.ApplyResult"> & {
    /**
     * @generated from oneof relkit.updater.v1.ApplyResult.kind
     */
    kind: {
        /**
         * @generated from field: relkit.updater.v1.ApplyAccepted accepted = 1;
         */
        value: ApplyAccepted;
        case: "accepted";
    } | {
        /**
         * @generated from field: relkit.updater.v1.Failed failed = 2;
         */
        value: Failed;
        case: "failed";
    } | {
        case: undefined;
        value?: undefined;
    };
};
/**
 * Describes the message relkit.updater.v1.ApplyResult.
 * Use `create(ApplyResultSchema)` to create a new message.
 */
export declare const ApplyResultSchema: GenMessage<ApplyResult>;
/**
 * @generated from message relkit.updater.v1.Ok
 */
export type Ok = Message<"relkit.updater.v1.Ok"> & {};
/**
 * Describes the message relkit.updater.v1.Ok.
 * Use `create(OkSchema)` to create a new message.
 */
export declare const OkSchema: GenMessage<Ok>;
/**
 * @generated from message relkit.updater.v1.Result
 */
export type Result = Message<"relkit.updater.v1.Result"> & {
    /**
     * @generated from oneof relkit.updater.v1.Result.kind
     */
    kind: {
        /**
         * @generated from field: relkit.updater.v1.Ok ok = 1;
         */
        value: Ok;
        case: "ok";
    } | {
        /**
         * @generated from field: relkit.updater.v1.Failed failed = 2;
         */
        value: Failed;
        case: "failed";
    } | {
        case: undefined;
        value?: undefined;
    };
};
/**
 * Describes the message relkit.updater.v1.Result.
 * Use `create(ResultSchema)` to create a new message.
 */
export declare const ResultSchema: GenMessage<Result>;
/**
 * @generated from message relkit.updater.v1.SidecarInfo
 */
export type SidecarInfo = Message<"relkit.updater.v1.SidecarInfo"> & {
    /**
     * @generated from field: string path = 1;
     */
    path: string;
    /**
     * @generated from field: uint32 ipc = 2;
     */
    ipc: number;
    /**
     * @generated from field: string version = 3;
     */
    version: string;
};
/**
 * Describes the message relkit.updater.v1.SidecarInfo.
 * Use `create(SidecarInfoSchema)` to create a new message.
 */
export declare const SidecarInfoSchema: GenMessage<SidecarInfo>;
/**
 * @generated from message relkit.updater.v1.SessionView
 */
export type SessionView = Message<"relkit.updater.v1.SessionView"> & {
    /**
     * @generated from field: string session_id = 1;
     */
    sessionId: string;
    /**
     * @generated from field: string plan_id = 2;
     */
    planId: string;
    /**
     * @generated from field: relkit.updater.v1.SessionPhase phase = 3;
     */
    phase: SessionPhase;
    /**
     * @generated from field: google.protobuf.Timestamp started_at = 4;
     */
    startedAt?: Timestamp | undefined;
    /**
     * @generated from field: relkit.updater.v1.Error error = 5;
     */
    error?: Error | undefined;
};
/**
 * Describes the message relkit.updater.v1.SessionView.
 * Use `create(SessionViewSchema)` to create a new message.
 */
export declare const SessionViewSchema: GenMessage<SessionView>;
/**
 * @generated from message relkit.updater.v1.StatusSnapshot
 */
export type StatusSnapshot = Message<"relkit.updater.v1.StatusSnapshot"> & {
    /**
     * @generated from field: google.protobuf.Timestamp last_check_at = 1;
     */
    lastCheckAt?: Timestamp | undefined;
    /**
     * @generated from field: relkit.updater.v1.LastResult last_result = 2;
     */
    lastResult: LastResult;
    /**
     * @generated from field: google.protobuf.Timestamp next_allowed_at = 3;
     */
    nextAllowedAt?: Timestamp | undefined;
    /**
     * @generated from field: int64 last_seen_sequence = 4;
     */
    lastSeenSequence: bigint;
    /**
     * @generated from field: repeated int64 skipped_codes = 5;
     */
    skippedCodes: bigint[];
    /**
     * @generated from field: relkit.updater.v1.SessionView active_session = 6;
     */
    activeSession?: SessionView | undefined;
    /**
     * @generated from field: relkit.updater.v1.SidecarInfo sidecar = 7;
     */
    sidecar?: SidecarInfo | undefined;
};
/**
 * Describes the message relkit.updater.v1.StatusSnapshot.
 * Use `create(StatusSnapshotSchema)` to create a new message.
 */
export declare const StatusSnapshotSchema: GenMessage<StatusSnapshot>;
/**
 * @generated from message relkit.updater.v1.Progress
 */
export type Progress = Message<"relkit.updater.v1.Progress"> & {
    /**
     * @generated from field: int64 bytes_received = 1;
     */
    bytesReceived: bigint;
    /**
     * @generated from field: int64 bytes_total = 2;
     */
    bytesTotal: bigint;
    /**
     * @generated from field: int64 bytes_per_second = 3;
     */
    bytesPerSecond: bigint;
};
/**
 * Describes the message relkit.updater.v1.Progress.
 * Use `create(ProgressSchema)` to create a new message.
 */
export declare const ProgressSchema: GenMessage<Progress>;
/**
 * @generated from message relkit.updater.v1.ApplyProgress
 */
export type ApplyProgress = Message<"relkit.updater.v1.ApplyProgress"> & {
    /**
     * @generated from field: string session_id = 1;
     */
    sessionId: string;
    /**
     * @generated from field: relkit.updater.v1.SessionPhase phase = 2;
     */
    phase: SessionPhase;
};
/**
 * Describes the message relkit.updater.v1.ApplyProgress.
 * Use `create(ApplyProgressSchema)` to create a new message.
 */
export declare const ApplyProgressSchema: GenMessage<ApplyProgress>;
/**
 * @generated from message relkit.updater.v1.Log
 */
export type Log = Message<"relkit.updater.v1.Log"> & {
    /**
     * @generated from field: string message = 1;
     */
    message: string;
};
/**
 * Describes the message relkit.updater.v1.Log.
 * Use `create(LogSchema)` to create a new message.
 */
export declare const LogSchema: GenMessage<Log>;
/**
 * @generated from message relkit.updater.v1.UpdaterEvent
 */
export type UpdaterEvent = Message<"relkit.updater.v1.UpdaterEvent"> & {
    /**
     * @generated from oneof relkit.updater.v1.UpdaterEvent.kind
     */
    kind: {
        /**
         * @generated from field: relkit.updater.v1.Capabilities capabilities = 1;
         */
        value: Capabilities;
        case: "capabilities";
    } | {
        /**
         * @generated from field: relkit.updater.v1.Progress progress = 2;
         */
        value: Progress;
        case: "progress";
    } | {
        /**
         * @generated from field: relkit.updater.v1.ApplyProgress apply_progress = 3;
         */
        value: ApplyProgress;
        case: "applyProgress";
    } | {
        /**
         * @generated from field: relkit.updater.v1.Log log = 4;
         */
        value: Log;
        case: "log";
    } | {
        /**
         * @generated from field: relkit.updater.v1.CheckResult check = 10;
         */
        value: CheckResult;
        case: "check";
    } | {
        /**
         * @generated from field: relkit.updater.v1.DownloadResult download = 11;
         */
        value: DownloadResult;
        case: "download";
    } | {
        /**
         * @generated from field: relkit.updater.v1.ApplyResult apply = 12;
         */
        value: ApplyResult;
        case: "apply";
    } | {
        /**
         * @generated from field: relkit.updater.v1.Result result = 13;
         */
        value: Result;
        case: "result";
    } | {
        /**
         * @generated from field: relkit.updater.v1.StatusSnapshot status = 14;
         */
        value: StatusSnapshot;
        case: "status";
    } | {
        /**
         * @generated from field: relkit.updater.v1.Failed failed = 15;
         */
        value: Failed;
        case: "failed";
    } | {
        case: undefined;
        value?: undefined;
    };
};
/**
 * Describes the message relkit.updater.v1.UpdaterEvent.
 * Use `create(UpdaterEventSchema)` to create a new message.
 */
export declare const UpdaterEventSchema: GenMessage<UpdaterEvent>;
/**
 * @generated from message relkit.updater.v1.ArtifactTarget
 */
export type ArtifactTarget = Message<"relkit.updater.v1.ArtifactTarget"> & {
    /**
     * @generated from field: string name = 1;
     */
    name: string;
    /**
     * @generated from field: map<string, string> selectors = 2;
     */
    selectors: {
        [key: string]: string;
    };
};
/**
 * Describes the message relkit.updater.v1.ArtifactTarget.
 * Use `create(ArtifactTargetSchema)` to create a new message.
 */
export declare const ArtifactTargetSchema: GenMessage<ArtifactTarget>;
/**
 * @generated from message relkit.updater.v1.PlannedFile
 */
export type PlannedFile = Message<"relkit.updater.v1.PlannedFile"> & {
    /**
     * @generated from field: string name = 1;
     */
    name: string;
    /**
     * @generated from field: int64 size = 2;
     */
    size: bigint;
    /**
     * @generated from field: string sha256_hex = 3;
     */
    sha256Hex: string;
    /**
     * @generated from field: repeated string urls = 4;
     */
    urls: string[];
    /**
     * @generated from field: string dest_relpath = 5;
     */
    destRelpath: string;
    /**
     * @generated from field: string local_path = 6;
     */
    localPath: string;
    /**
     * @generated from field: bool downloaded = 7;
     */
    downloaded: boolean;
};
/**
 * Describes the message relkit.updater.v1.PlannedFile.
 * Use `create(PlannedFileSchema)` to create a new message.
 */
export declare const PlannedFileSchema: GenMessage<PlannedFile>;
/**
 * @generated from message relkit.updater.v1.UpdatePlan
 */
export type UpdatePlan = Message<"relkit.updater.v1.UpdatePlan"> & {
    /**
     * @generated from field: string plan_id = 1;
     */
    planId: string;
    /**
     * @generated from field: string product = 2;
     */
    product: string;
    /**
     * @generated from field: string channel = 3;
     */
    channel: string;
    /**
     * @generated from field: string version = 4;
     */
    version: string;
    /**
     * @generated from field: int64 code = 5;
     */
    code: bigint;
    /**
     * @generated from field: int64 sequence = 6;
     */
    sequence: bigint;
    /**
     * @generated from field: bool mandatory = 7;
     */
    mandatory: boolean;
    /**
     * @generated from field: int32 remaining_hops = 8;
     */
    remainingHops: number;
    /**
     * @generated from field: string release_notes_markdown = 9;
     */
    releaseNotesMarkdown: string;
    /**
     * @generated from field: string release_notes_url = 10;
     */
    releaseNotesUrl: string;
    /**
     * @generated from field: repeated relkit.updater.v1.PriorReleaseNotes prior_release_notes = 11;
     */
    priorReleaseNotes: PriorReleaseNotes[];
    /**
     * @generated from field: repeated relkit.updater.v1.PlannedFile files = 12;
     */
    files: PlannedFile[];
    /**
     * @generated from field: google.protobuf.Timestamp created_at = 13;
     */
    createdAt?: Timestamp | undefined;
    /**
     * @generated from field: google.protobuf.Timestamp expires_at = 14;
     */
    expiresAt?: Timestamp | undefined;
    /**
     * @generated from field: bytes plan_hmac = 15;
     */
    planHmac: Uint8Array;
};
/**
 * Describes the message relkit.updater.v1.UpdatePlan.
 * Use `create(UpdatePlanSchema)` to create a new message.
 */
export declare const UpdatePlanSchema: GenMessage<UpdatePlan>;
/**
 * @generated from message relkit.updater.v1.PersistedState
 */
export type PersistedState = Message<"relkit.updater.v1.PersistedState"> & {
    /**
     * @generated from field: google.protobuf.Timestamp last_check_at = 1;
     */
    lastCheckAt?: Timestamp | undefined;
    /**
     * @generated from field: relkit.updater.v1.LastResult last_result = 2;
     */
    lastResult: LastResult;
    /**
     * @generated from field: int64 last_seen_sequence = 3;
     */
    lastSeenSequence: bigint;
    /**
     * @generated from field: int64 last_seen_directory_sequence = 4;
     */
    lastSeenDirectorySequence: bigint;
    /**
     * @generated from field: int64 last_seen_fallback_sequence = 5;
     */
    lastSeenFallbackSequence: bigint;
    /**
     * @generated from field: repeated int64 skipped_codes = 6;
     */
    skippedCodes: bigint[];
};
/**
 * Describes the message relkit.updater.v1.PersistedState.
 * Use `create(PersistedStateSchema)` to create a new message.
 */
export declare const PersistedStateSchema: GenMessage<PersistedState>;
/**
 * @generated from message relkit.updater.v1.ApplySessionRecord
 */
export type ApplySessionRecord = Message<"relkit.updater.v1.ApplySessionRecord"> & {
    /**
     * @generated from field: string session_id = 1;
     */
    sessionId: string;
    /**
     * @generated from field: string plan_id = 2;
     */
    planId: string;
    /**
     * @generated from field: relkit.updater.v1.SessionPhase phase = 3;
     */
    phase: SessionPhase;
    /**
     * @generated from field: google.protobuf.Timestamp started_at = 4;
     */
    startedAt?: Timestamp | undefined;
    /**
     * @generated from field: google.protobuf.Timestamp heartbeat_at = 5;
     */
    heartbeatAt?: Timestamp | undefined;
    /**
     * @generated from field: string install_root = 6;
     */
    installRoot: string;
    /**
     * @generated from field: string staged_root = 7;
     */
    stagedRoot: string;
    /**
     * @generated from field: int64 target_code = 8;
     */
    targetCode: bigint;
    /**
     * @generated from field: string target_version = 9;
     */
    targetVersion: string;
    /**
     * @generated from field: relkit.updater.v1.Error error = 10;
     */
    error?: Error | undefined;
    /**
     * @generated from field: int32 pid = 11;
     */
    pid: number;
    /**
     * @generated from field: relkit.updater.v1.Layout layout = 12;
     */
    layout: Layout;
    /**
     * @generated from field: bool relaunch = 13;
     */
    relaunch: boolean;
    /**
     * @generated from field: string executable_relpath = 14;
     */
    executableRelpath: string;
    /**
     * @generated from field: repeated string preserve = 15;
     */
    preserve: string[];
    /**
     * @generated from field: int32 retain = 16;
     */
    retain: number;
    /**
     * @generated from field: repeated relkit.updater.v1.FileSetEntry file_set = 17;
     */
    fileSet: FileSetEntry[];
    /**
     * @generated from field: string sidecar_relpath = 18;
     */
    sidecarRelpath: string;
};
/**
 * Describes the message relkit.updater.v1.ApplySessionRecord.
 * Use `create(ApplySessionRecordSchema)` to create a new message.
 */
export declare const ApplySessionRecordSchema: GenMessage<ApplySessionRecord>;
/**
 * @generated from message relkit.updater.v1.JournalEntry
 */
export type JournalEntry = Message<"relkit.updater.v1.JournalEntry"> & {
    /**
     * @generated from field: string dest_path = 1;
     */
    destPath: string;
    /**
     * @generated from field: string backup_path = 2;
     */
    backupPath: string;
    /**
     * @generated from field: string source_path = 3;
     */
    sourcePath: string;
};
/**
 * Describes the message relkit.updater.v1.JournalEntry.
 * Use `create(JournalEntrySchema)` to create a new message.
 */
export declare const JournalEntrySchema: GenMessage<JournalEntry>;
/**
 * @generated from message relkit.updater.v1.ApplyJournal
 */
export type ApplyJournal = Message<"relkit.updater.v1.ApplyJournal"> & {
    /**
     * @generated from field: string session_id = 1;
     */
    sessionId: string;
    /**
     * @generated from field: repeated relkit.updater.v1.JournalEntry entries = 2;
     */
    entries: JournalEntry[];
    /**
     * @generated from field: bool committed = 3;
     */
    committed: boolean;
};
/**
 * Describes the message relkit.updater.v1.ApplyJournal.
 * Use `create(ApplyJournalSchema)` to create a new message.
 */
export declare const ApplyJournalSchema: GenMessage<ApplyJournal>;
/**
 * @generated from enum relkit.updater.v1.ErrorCode
 */
export declare enum ErrorCode {
    /**
     * @generated from enum value: ERROR_CODE_UNSPECIFIED = 0;
     */
    UNSPECIFIED = 0,
    /**
     * @generated from enum value: ERROR_CODE_NETWORK = 1;
     */
    NETWORK = 1,
    /**
     * @generated from enum value: ERROR_CODE_SIGNATURE = 2;
     */
    SIGNATURE = 2,
    /**
     * @generated from enum value: ERROR_CODE_ROLLBACK_REJECTED = 3;
     */
    ROLLBACK_REJECTED = 3,
    /**
     * @generated from enum value: ERROR_CODE_SELECTOR_NO_MATCH = 4;
     */
    SELECTOR_NO_MATCH = 4,
    /**
     * @generated from enum value: ERROR_CODE_DISK = 5;
     */
    DISK = 5,
    /**
     * @generated from enum value: ERROR_CODE_PERMISSION_DENIED = 6;
     */
    PERMISSION_DENIED = 6,
    /**
     * @generated from enum value: ERROR_CODE_OCCUPIED = 7;
     */
    OCCUPIED = 7,
    /**
     * @generated from enum value: ERROR_CODE_PROTOCOL_MISMATCH = 8;
     */
    PROTOCOL_MISMATCH = 8,
    /**
     * @generated from enum value: ERROR_CODE_UPDATER_TOO_OLD = 9;
     */
    UPDATER_TOO_OLD = 9,
    /**
     * @generated from enum value: ERROR_CODE_UPDATER_TOO_NEW = 10;
     */
    UPDATER_TOO_NEW = 10,
    /**
     * @generated from enum value: ERROR_CODE_PLAN_TAMPERED = 11;
     */
    PLAN_TAMPERED = 11,
    /**
     * @generated from enum value: ERROR_CODE_PLAN_EXPIRED = 12;
     */
    PLAN_EXPIRED = 12,
    /**
     * @generated from enum value: ERROR_CODE_PLAN_UNKNOWN = 13;
     */
    PLAN_UNKNOWN = 13,
    /**
     * @generated from enum value: ERROR_CODE_PLAN_NOT_DOWNLOADED = 14;
     */
    PLAN_NOT_DOWNLOADED = 14,
    /**
     * @generated from enum value: ERROR_CODE_SKIP_DENIED = 15;
     */
    SKIP_DENIED = 15,
    /**
     * @generated from enum value: ERROR_CODE_PROFILE_INVALID = 16;
     */
    PROFILE_INVALID = 16,
    /**
     * @generated from enum value: ERROR_CODE_CHANNEL_NOT_ALLOWED = 17;
     */
    CHANNEL_NOT_ALLOWED = 17,
    /**
     * @generated from enum value: ERROR_CODE_CANCELED = 18;
     */
    CANCELED = 18,
    /**
     * @generated from enum value: ERROR_CODE_SIDECAR_NOT_FOUND = 19;
     */
    SIDECAR_NOT_FOUND = 19,
    /**
     * @generated from enum value: ERROR_CODE_LAYOUT_UNSUPPORTED = 20;
     */
    LAYOUT_UNSUPPORTED = 20
}
/**
 * Describes the enum relkit.updater.v1.ErrorCode.
 */
export declare const ErrorCodeSchema: GenEnum<ErrorCode>;
/**
 * @generated from enum relkit.updater.v1.LastResult
 */
export declare enum LastResult {
    /**
     * @generated from enum value: LAST_RESULT_UNSPECIFIED = 0;
     */
    UNSPECIFIED = 0,
    /**
     * @generated from enum value: LAST_RESULT_UP_TO_DATE = 1;
     */
    UP_TO_DATE = 1,
    /**
     * @generated from enum value: LAST_RESULT_UPDATE_AVAILABLE = 2;
     */
    UPDATE_AVAILABLE = 2,
    /**
     * @generated from enum value: LAST_RESULT_FALLBACK_REQUIRED = 3;
     */
    FALLBACK_REQUIRED = 3,
    /**
     * @generated from enum value: LAST_RESULT_THROTTLED = 4;
     */
    THROTTLED = 4,
    /**
     * @generated from enum value: LAST_RESULT_FAILED = 5;
     */
    FAILED = 5,
    /**
     * @generated from enum value: LAST_RESULT_APPLIED = 6;
     */
    APPLIED = 6
}
/**
 * Describes the enum relkit.updater.v1.LastResult.
 */
export declare const LastResultSchema: GenEnum<LastResult>;
/**
 * @generated from enum relkit.updater.v1.Layout
 */
export declare enum Layout {
    /**
     * @generated from enum value: LAYOUT_UNSPECIFIED = 0;
     */
    UNSPECIFIED = 0,
    /**
     * @generated from enum value: LAYOUT_WHOLE_ROOT = 1;
     */
    WHOLE_ROOT = 1,
    /**
     * @generated from enum value: LAYOUT_VERSIONED_DIR = 2;
     */
    VERSIONED_DIR = 2,
    /**
     * @generated from enum value: LAYOUT_FILE_SET = 3;
     */
    FILE_SET = 3
}
/**
 * Describes the enum relkit.updater.v1.Layout.
 */
export declare const LayoutSchema: GenEnum<Layout>;
/**
 * @generated from enum relkit.updater.v1.Operation
 */
export declare enum Operation {
    /**
     * @generated from enum value: OPERATION_UNSPECIFIED = 0;
     */
    UNSPECIFIED = 0,
    /**
     * @generated from enum value: OPERATION_CHECK = 1;
     */
    CHECK = 1,
    /**
     * @generated from enum value: OPERATION_SKIP = 2;
     */
    SKIP = 2,
    /**
     * @generated from enum value: OPERATION_DOWNLOAD = 3;
     */
    DOWNLOAD = 3,
    /**
     * @generated from enum value: OPERATION_APPLY = 4;
     */
    APPLY = 4,
    /**
     * @generated from enum value: OPERATION_STATUS = 5;
     */
    STATUS = 5,
    /**
     * @generated from enum value: OPERATION_CLEANUP = 6;
     */
    CLEANUP = 6,
    /**
     * @generated from enum value: OPERATION_CANCEL = 7;
     */
    CANCEL = 7,
    /**
     * @generated from enum value: OPERATION_SCHEDULER = 8;
     */
    SCHEDULER = 8
}
/**
 * Describes the enum relkit.updater.v1.Operation.
 */
export declare const OperationSchema: GenEnum<Operation>;
/**
 * @generated from enum relkit.updater.v1.SessionPhase
 */
export declare enum SessionPhase {
    /**
     * @generated from enum value: SESSION_PHASE_UNSPECIFIED = 0;
     */
    UNSPECIFIED = 0,
    /**
     * @generated from enum value: SESSION_PHASE_WAITING_FOR_EXIT = 1;
     */
    WAITING_FOR_EXIT = 1,
    /**
     * @generated from enum value: SESSION_PHASE_COPYING = 2;
     */
    COPYING = 2,
    /**
     * @generated from enum value: SESSION_PHASE_COMMITTING = 3;
     */
    COMMITTING = 3,
    /**
     * @generated from enum value: SESSION_PHASE_RELAUNCHING = 4;
     */
    RELAUNCHING = 4,
    /**
     * @generated from enum value: SESSION_PHASE_NEEDS_ATTENTION = 5;
     */
    NEEDS_ATTENTION = 5,
    /**
     * @generated from enum value: SESSION_PHASE_COMPLETED = 6;
     */
    COMPLETED = 6,
    /**
     * @generated from enum value: SESSION_PHASE_ROLLED_BACK = 7;
     */
    ROLLED_BACK = 7
}
/**
 * Describes the enum relkit.updater.v1.SessionPhase.
 */
export declare const SessionPhaseSchema: GenEnum<SessionPhase>;
