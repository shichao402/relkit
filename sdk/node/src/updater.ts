/**
 * The check-for-update flow (SPEC.md section 12.1).
 *
 * The ordering here is the specification, not a preference. Each step either
 * establishes something the next one relies on, or rejects a source outright,
 * and reordering them creates gaps that are invisible in a working deployment:
 * verification before parsing, product and channel before anything is acted on,
 * sequence before the index is adopted.
 */

import { clone } from "@bufbuild/protobuf";

import { isMandatory, resolveUpgradePath, selectNextTarget } from "./chain.js";
import { downloadArtifact, type VerifiedFile } from "./download.js";
import {
  openEnvelope,
  describeEnvelopeResult,
  toTrustedKeys,
  type TrustedKeys,
  type TrustedKeysInput,
} from "./envelope.js";
import {
  FetchError,
  HttpFetcher,
  cacheBust,
  sha256OfBytes,
  type Fetcher,
  type ProgressCallback,
} from "./fetch.js";
import {
  ArtifactSchema,
  RupFormatError,
  num,
  parseDirectory,
  parseFallback,
  parseIndex,
  parseManifest,
  selectorsToMap,
  type Artifact,
  type DirectoryService,
  type Fallback,
  type FallbackRule,
  type Index,
  type Manifest,
  type UpdateDirectory,
  type VersionNode,
} from "./models.js";
import { rankByLearning, rankUrlStrings } from "./preference.js";
import { collectPriorReleaseNotes, type PriorReleaseNotes } from "./release-notes.js";
import { matchesSelectors, selectArtifact } from "./selectors.js";
import {
  acceptsSequence,
  resolvePolicy,
  shouldCheck,
  type UpdatePolicy,
  type UpdateState,
  type UpdateStateStore,
} from "./state.js";

/** Preference key for a directory service id (SPEC §12.7 / §16). */
export const directoryServiceKey = (id: string): string => `service:${id}`;

/** Nothing to do: the client is already on the newest reachable version. */
export interface UpToDate {
  kind: "up-to-date";
  sequence: number;
  /**
   * The running version has been withdrawn but nothing newer is reachable.
   * Worth telling the user, since the fix is out of their hands.
   */
  currentIsYanked: boolean;
}

/** A newer version is available and its artifact has been located. */
export interface UpdateAvailable {
  kind: "update-available";
  target: VersionNode;
  artifact: Artifact;
  manifest: Manifest;
  /** The client is below `minSupported`; the host must not offer to postpone. */
  mandatory: boolean;
  /**
   * How many versions still lie between here and the newest one, this one
   * included. Greater than one means the chain forces an intermediate hop.
   */
  remainingHops: number;
  sequence: number;
  priorReleaseNotes: PriorReleaseNotes[];
  isFinalHop: boolean;
  releaseNotesMarkdown: string;
  releaseNotesUrl: string;
}

/** No source could be used. */
export interface CheckFailed {
  kind: "check-failed";
  reason: string;
  attempts: string[];
  /** Compile-time last-resort copy. Not a signed remote document. */
  recovery?: RecoveryHelp;
}

export interface RecoveryHelp {
  message: string;
  links: RecoveryLink[];
}

export interface RecoveryLink {
  label: string;
  url: string;
}

/** Skipped because not enough time has passed (SPEC.md section 12.2). */
export interface CheckThrottled {
  kind: "check-throttled";
  nextAllowedAt: Date;
}

/**
 * A signed emergency notice matches this build (SPEC.md section 12.6).
 *
 * The host should show `message` and open `manualUrl` in a browser. This never
 * carries an auto-download; that is the point of the escape hatch.
 */
export interface FallbackRequired {
  kind: "fallback-required";
  manualUrl: string;
  message: string;
  mandatory: boolean;
  sequence: number;
  minCode: number;
  maxCode: number;
}

export type UpdateCheckResult =
  | UpToDate
  | UpdateAvailable
  | CheckFailed
  | CheckThrottled
  | FallbackRequired;

export interface RupUpdaterOptions {
  product: string;
  channel: string;
  /**
   * This build's code. Never pass 0 for "unknown" (SPEC.md section 8.1): a
   * development build that reports 0 will happily replace itself with a release.
   * Use a value above every published code instead.
   */
  currentCode: number;
  /**
   * Trusted Ed25519 public keys by key id. A plain object, a `Map`, or a
   * prebuilt {@link TrustedKeys} all work; string values are read as base64.
   */
  trustedKeys: TrustedKeysInput;
  clientSelectors: Record<string, string>;
  stateStore: UpdateStateStore;
  /** Signed directory entry URLs (primary → backups). Preferred bootstrap path. */
  entryUrls?: string[];
  /** Direct index mirrors. Ignored for the check path when entryUrls is set. */
  indexUrls?: string[];
  /** Optional signed fallback notice URLs (SPEC section 12.6). */
  fallbackUrls?: string[];
  fetcher?: Fetcher;
  policy?: Partial<UpdatePolicy>;
  log?: (message: string) => void;
  /** Host-embedded last-resort copy shown when every remote check fails. */
  recovery?: RecoveryHelp;
}

type SourceOutcome<T> = { ok: true; value: T } | { ok: false; why: string };

interface IndexCandidate {
  url: URL;
  preferenceKey: string;
}

/** Checks for, and downloads, updates for one (product, channel). */
export class RupUpdater {
  readonly product: string;
  readonly channel: string;
  readonly currentCode: number;
  readonly trustedKeys: TrustedKeys;
  readonly clientSelectors: Record<string, string>;
  readonly stateStore: UpdateStateStore;
  readonly entryUrls: readonly string[];
  readonly indexUrls: readonly string[];
  readonly fallbackUrls: readonly string[];
  readonly fetcher: Fetcher;
  readonly policy: UpdatePolicy;
  private readonly log?: (message: string) => void;
  readonly recovery?: RecoveryHelp;

  /** Last directory adopted during check / checkFallback in this process. */
  private lastDirectory: UpdateDirectory | null = null;

  constructor(options: RupUpdaterOptions) {
    this.product = options.product;
    this.channel = options.channel;
    this.currentCode = options.currentCode;
    this.trustedKeys = toTrustedKeys(options.trustedKeys);
    this.clientSelectors = options.clientSelectors;
    this.stateStore = options.stateStore;
    this.entryUrls = [...(options.entryUrls ?? [])];
    this.indexUrls = [...(options.indexUrls ?? [])];
    this.fallbackUrls = [...(options.fallbackUrls ?? [])];
    this.fetcher = options.fetcher ?? new HttpFetcher();
    this.policy = resolvePolicy(options.policy);
    this.recovery = options.recovery;
    if (options.log) this.log = options.log;

    if (this.indexUrls.length === 0 && this.entryUrls.length === 0) {
      throw new TypeError("indexUrls or entryUrls must contain at least one URL");
    }
    if (this.trustedKeys.isEmpty) {
      // Without a key the signature check can only ever fail, and a client built
      // that way would report "update server broken" forever. Failing at
      // construction puts the error where someone can act on it.
      throw new TypeError("trustedKeys must contain at least one public key");
    }
  }

  /**
   * Runs a check.
   *
   * Pass `force` for a user-initiated check, which ignores throttling. When
   * fallback sources are available, also evaluates the fallback document and
   * merges: UpdateAvailable > FallbackRequired > UpToDate / CheckFailed.
   */
  async check(options: { force?: boolean } = {}): Promise<UpdateCheckResult> {
    const normal = await this.checkIndex(options.force ?? false);
    if (normal.kind === "update-available") return normal;
    const fallback = await this.checkFallback();
    if (fallback !== null) return fallback;
    if (normal.kind === "check-failed" && this.recovery) {
      return { ...normal, recovery: this.recovery };
    }
    return normal;
  }

  /**
   * Evaluates only the signed fallback document (SPEC section 12.6).
   *
   * Returns null when no rule matches or no source can be used. Hosts should call
   * this after a download/apply failure to urge a manual update.
   */
  async checkFallback(): Promise<FallbackRequired | null> {
    const state = await this.stateStore.load();
    const urls = await this.resolveFallbackUrls(state);
    if (urls.length === 0) return null;

    const attempts: string[] = [];

    for (const rawUrl of urls) {
      let url: URL;
      try {
        url = new URL(rawUrl);
      } catch {
        attempts.push(`${rawUrl}: not a valid URL`);
        continue;
      }

      const outcome = await this.loadFallbackFrom(url, state);
      if (outcome.ok) {
        const doc = outcome.value;
        state.recordSourceSuccess(rawUrl);
        state.observeFallbackSequence(num(doc.sequence));
        await this.stateStore.save(state);
        const rule = this.matchFallbackRule(doc);
        if (rule === null) return null;
        return {
          kind: "fallback-required",
          manualUrl: rule.manualUrl,
          message: rule.message,
          mandatory: rule.mandatory,
          sequence: num(doc.sequence),
          minCode: num(rule.minCode),
          maxCode: num(rule.maxCode),
        };
      }
      state.recordSourceFailure(rawUrl);
      attempts.push(`${rawUrl}: ${outcome.why}`);
    }

    await this.stateStore.save(state);
    this.log?.(`fallback check failed: ${attempts.join("; ")}`);
    return null;
  }

  /** Downloads the artifact of an available update and verifies its hash. */
  async download(
    update: UpdateAvailable,
    options: { destinationDir: string; onProgress?: ProgressCallback },
  ): Promise<VerifiedFile> {
    const state = await this.stateStore.load();
    const ranked = clone(ArtifactSchema, update.artifact);
    ranked.urls = rankUrlStrings(update.artifact.urls, state);

    let lastBps = 0;
    const verified = await downloadArtifact(ranked, {
      fetcher: this.fetcher,
      destinationDir: options.destinationDir,
      policy: this.policy,
      onProgress: (progress) => {
        if (progress.bytesPerSecond > 0) {
          lastBps = Math.round(progress.bytesPerSecond);
        }
        options.onProgress?.(progress);
      },
      ...(this.log ? { log: this.log } : {}),
    });

    if (!verified.sourceUrl.startsWith("file:")) {
      state.recordSourceSuccess(
        verified.sourceUrl,
        lastBps > 0 ? lastBps : undefined,
      );
      await this.stateStore.save(state);
    }
    return verified;
  }

  /**
   * Marks a version as skipped so later checks stay quiet about it.
   *
   * Has no effect on a mandatory update: the host must not offer the choice, and
   * honouring a stale skip would defeat the floor the publisher set.
   */
  async skip(version: VersionNode): Promise<void> {
    const state = await this.stateStore.load();
    state.skipped.add(num(version.code));
    await this.stateStore.save(state);
  }

  async isSkipped(version: VersionNode): Promise<boolean> {
    const state = await this.stateStore.load();
    return state.skipped.has(num(version.code));
  }

  close(): void {
    this.fetcher.close();
  }

  private async checkIndex(force: boolean): Promise<UpdateCheckResult> {
    const state = await this.stateStore.load();

    if (!force && !shouldCheck(this.policy, state)) {
      const wait =
        state.lastResult === "error"
          ? this.policy.afterFailureMs
          : this.policy.afterSuccessMs;
      return {
        kind: "check-throttled",
        nextAllowedAt: new Date(state.lastCheckAt!.getTime() + wait),
      };
    }

    const attempts: string[] = [];
    const plan = await this.resolveIndexPlan(state, attempts);
    if (plan === null || plan.length === 0) {
      state.lastCheckAt = new Date();
      state.lastResult = "error";
      await this.stateStore.save(state);
      return {
        kind: "check-failed",
        reason: "no usable directory or index source",
        attempts,
      };
    }

    let adopted: Index | null = null;
    let adoptedKey: string | null = null;

    for (const candidate of plan) {
      const outcome = await this.loadIndexFrom(candidate.url, state);
      if (outcome.ok) {
        adopted = outcome.value;
        adoptedKey = candidate.preferenceKey;
        state.recordSourceSuccess(candidate.preferenceKey);
        break;
      }
      state.recordSourceFailure(candidate.preferenceKey);
      attempts.push(`${candidate.url}: ${outcome.why}`);
    }

    if (adopted === null) {
      state.lastCheckAt = new Date();
      state.lastResult = "error";
      await this.stateStore.save(state);
      return { kind: "check-failed", reason: "no usable index source", attempts };
    }

    state.observeSequence(num(adopted.sequence));

    const target = selectNextTarget(adopted, this.currentCode);
    if (target === null) {
      state.lastCheckAt = new Date();
      state.lastResult = "up-to-date";
      await this.stateStore.save(state);
      return {
        kind: "up-to-date",
        sequence: num(adopted.sequence),
        currentIsYanked: adopted.versions.some(
          (node) => num(node.code) === this.currentCode && node.yanked,
        ),
      };
    }

    let manifest: Manifest;
    try {
      manifest = await this.loadManifest(target, state);
    } catch (error) {
      attempts.push(
        `manifest for ${target.version}: ${
          error instanceof Error ? error.message : String(error)
        }`,
      );
      state.lastCheckAt = new Date();
      state.lastResult = "error";
      await this.stateStore.save(state);
      return {
        kind: "check-failed",
        reason: "could not obtain a valid manifest",
        attempts,
      };
    }

    const artifact = selectArtifact(manifest, this.clientSelectors);
    if (artifact === null) {
      state.lastCheckAt = new Date();
      state.lastResult = "no-artifact";
      await this.stateStore.save(state);
      return {
        kind: "check-failed",
        reason: `version ${target.version} has no artifact for this platform`,
        attempts: [
          ...attempts,
          `client selectors: ${JSON.stringify(this.clientSelectors)}`,
          `offered: ${manifest.artifacts
            .map((a) => `${a.id} ${JSON.stringify(selectorsToMap(a.selectors))}`)
            .join(", ")}`,
        ],
      };
    }

    const ranked = clone(ArtifactSchema, artifact);
    ranked.urls = rankUrlStrings(artifact.urls, state);

    state.lastCheckAt = new Date();
    state.lastResult = "update-available";
    await this.stateStore.save(state);

    this.log?.(
      `adopted index via ${adoptedKey} (sequence ${num(adopted.sequence)})`,
    );

    const remainingHops = resolveUpgradePath(adopted, this.currentCode).length;
    return {
      kind: "update-available",
      target,
      artifact: ranked,
      manifest,
      mandatory: isMandatory(adopted, this.currentCode),
      remainingHops,
      sequence: num(adopted.sequence),
      priorReleaseNotes: collectPriorReleaseNotes(adopted, num(target.code)),
      isFinalHop: remainingHops === 1,
      releaseNotesMarkdown:
        target.notes.trim().length > 0
          ? target.notes.trim()
          : manifest.notes.trim(),
      releaseNotesUrl: target.notesUrl.trim(),
    };
  }

  private async resolveIndexPlan(
    state: UpdateState,
    attempts: string[],
  ): Promise<IndexCandidate[] | null> {
    if (this.entryUrls.length === 0) {
      const plan: IndexCandidate[] = [];
      for (const rawUrl of rankUrlStrings(this.indexUrls, state)) {
        try {
          plan.push({ url: new URL(rawUrl), preferenceKey: rawUrl });
        } catch {
          attempts.push(`${rawUrl}: not a valid URL`);
        }
      }
      return plan;
    }

    for (const rawEntry of rankUrlStrings(this.entryUrls, state)) {
      let entry: URL;
      try {
        entry = new URL(rawEntry);
      } catch {
        attempts.push(`${rawEntry}: not a valid URL`);
        continue;
      }

      const outcome = await this.loadDirectoryFrom(entry, state);
      if (!outcome.ok) {
        state.recordSourceFailure(rawEntry);
        attempts.push(`${rawEntry}: ${outcome.why}`);
        continue;
      }

      const doc = outcome.value;
      this.lastDirectory = doc;
      state.recordSourceSuccess(rawEntry);
      state.observeDirectorySequence(num(doc.directorySequence));
      await this.stateStore.save(state);

      const services = this.servicesForChannel(doc);
      if (services.length === 0) {
        attempts.push(`${rawEntry}: no services for channel "${this.channel}"`);
        continue;
      }

      const ranked = rankByLearning<DirectoryService>({
        items: services,
        keyOf: (service) => directoryServiceKey(service.id),
        state,
      });

      const plan: IndexCandidate[] = [];
      for (const service of ranked) {
        try {
          plan.push({
            url: new URL(service.indexUrl),
            preferenceKey: directoryServiceKey(service.id),
          });
        } catch {
          attempts.push(
            `${rawEntry} service ${service.id}: bad indexUrl ${service.indexUrl}`,
          );
        }
      }
      if (plan.length === 0) {
        attempts.push(`${rawEntry}: no usable indexUrl in directory`);
        continue;
      }
      return plan;
    }
    return null;
  }

  private async resolveFallbackUrls(state: UpdateState): Promise<string[]> {
    if (this.fallbackUrls.length > 0) {
      return rankUrlStrings(this.fallbackUrls, state);
    }
    if (this.entryUrls.length === 0) return [];

    let doc = this.lastDirectory;
    if (doc === null) {
      for (const rawEntry of rankUrlStrings(this.entryUrls, state)) {
        let entry: URL;
        try {
          entry = new URL(rawEntry);
        } catch {
          continue;
        }
        const outcome = await this.loadDirectoryFrom(entry, state);
        if (outcome.ok) {
          this.lastDirectory = outcome.value;
          state.recordSourceSuccess(rawEntry);
          state.observeDirectorySequence(num(outcome.value.directorySequence));
          await this.stateStore.save(state);
          break;
        }
        state.recordSourceFailure(rawEntry);
      }
      doc = this.lastDirectory;
    }
    if (doc === null) return [];

    const urls: string[] = [];
    for (const service of this.servicesForChannel(doc)) {
      if (service.fallbackUrl.length === 0) continue;
      urls.push(service.fallbackUrl);
    }
    return rankUrlStrings(urls, state);
  }

  private servicesForChannel(doc: UpdateDirectory): DirectoryService[] {
    return doc.services
      .filter(
        (service) =>
          service.channel.length === 0 || service.channel === this.channel,
      )
      .sort((a, b) => {
        if (a.priority !== b.priority) return a.priority - b.priority;
        return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
      });
  }

  private async loadDirectoryFrom(
    url: URL,
    state: UpdateState,
  ): Promise<SourceOutcome<UpdateDirectory>> {
    let bytes: Uint8Array;
    try {
      bytes = await this.fetcher.getBytes(
        cacheBust(url),
        this.policy.documentTimeoutMs,
      );
    } catch (error) {
      return { ok: false, why: describeFetchError(error) };
    }

    const verified = openEnvelope(bytes, this.trustedKeys);
    if (!verified.accepted) {
      return {
        ok: false,
        why: `signature check failed (${describeEnvelopeResult(verified)})`,
      };
    }

    let doc: UpdateDirectory;
    try {
      doc = parseDirectory(verified.payload);
    } catch (error) {
      if (error instanceof RupFormatError) {
        return { ok: false, why: `malformed directory: ${error.message}` };
      }
      throw error;
    }

    if (doc.product !== this.product) {
      return {
        ok: false,
        why: `directory is for product "${doc.product}", expected "${this.product}"`,
      };
    }

    const sequence = num(doc.directorySequence);
    if (!acceptsSequence(sequence, state.lastSeenDirectorySequence)) {
      this.log?.(
        `${url} directory sequence ${sequence} < ` +
          `${state.lastSeenDirectorySequence}, trying the next entry`,
      );
      return {
        ok: false,
        why:
          `directory_sequence ${sequence} is older than the last accepted ` +
          `${state.lastSeenDirectorySequence}`,
      };
    }

    return { ok: true, value: doc };
  }

  private async loadIndexFrom(
    url: URL,
    state: UpdateState,
  ): Promise<SourceOutcome<Index>> {
    let bytes: Uint8Array;
    try {
      bytes = await this.fetcher.getBytes(
        cacheBust(url),
        this.policy.documentTimeoutMs,
      );
    } catch (error) {
      return { ok: false, why: describeFetchError(error) };
    }

    const verified = openEnvelope(bytes, this.trustedKeys);
    if (!verified.accepted) {
      // A source that fails verification is unusable, full stop. There is no
      // fallback to reading it unsigned: that fallback is exactly what an
      // attacker who can serve bytes would aim for.
      return {
        ok: false,
        why: `signature check failed (${describeEnvelopeResult(verified)})`,
      };
    }

    let index: Index;
    try {
      index = parseIndex(verified.payload);
    } catch (error) {
      if (error instanceof RupFormatError) {
        return { ok: false, why: `malformed index: ${error.message}` };
      }
      throw error;
    }

    if (index.product !== this.product) {
      return {
        ok: false,
        why: `index is for product "${index.product}", expected "${this.product}"`,
      };
    }
    if (index.channel !== this.channel) {
      return {
        ok: false,
        why: `index is for channel "${index.channel}", expected "${this.channel}"`,
      };
    }

    const sequence = num(index.sequence);
    if (!acceptsSequence(sequence, state.lastSeenSequence)) {
      // Not an error the user should see. A mirror lagging behind another is
      // ordinary, and it resolves itself once replication catches up.
      this.log?.(
        `${url} is behind (sequence ${sequence} < ${state.lastSeenSequence}), ` +
          "trying the next source",
      );
      return {
        ok: false,
        why:
          `sequence ${sequence} is older than the last accepted ` +
          `${state.lastSeenSequence}`,
      };
    }

    return { ok: true, value: index };
  }

  /**
   * Fetches the manifest, trying mirrors in order.
   *
   * The digest comes from the signed index, so a manifest that matches it is as
   * trustworthy as the index itself. That is why the manifest carries no
   * signature of its own: one signature over a document that pins the hash of
   * everything else is both cheaper and harder to get wrong.
   */
  private async loadManifest(
    target: VersionNode,
    state: UpdateState,
  ): Promise<Manifest> {
    const failures: string[] = [];

    for (const rawUrl of rankUrlStrings(target.manifest!.urls, state)) {
      let url: URL;
      try {
        url = new URL(rawUrl);
      } catch {
        failures.push(`${rawUrl}: not a valid URL`);
        continue;
      }

      let bytes: Uint8Array;
      try {
        // Immutable, content-addressed: never cache-busted.
        bytes = await this.fetcher.getBytes(url, this.policy.documentTimeoutMs);
      } catch (error) {
        state.recordSourceFailure(rawUrl);
        failures.push(`${rawUrl}: ${describeFetchError(error)}`);
        continue;
      }

      const expectedSize = num(target.manifest!.size);
      if (bytes.length !== expectedSize) {
        state.recordSourceFailure(rawUrl);
        failures.push(
          `${rawUrl}: expected ${expectedSize} bytes, got ${bytes.length}`,
        );
        continue;
      }
      if (sha256OfBytes(bytes) !== target.manifest!.sha256) {
        state.recordSourceFailure(rawUrl);
        failures.push(`${rawUrl}: sha256 mismatch`);
        continue;
      }

      let manifest: Manifest;
      try {
        manifest = parseManifest(bytes);
      } catch (error) {
        state.recordSourceFailure(rawUrl);
        failures.push(
          `${rawUrl}: ${error instanceof Error ? error.message : String(error)}`,
        );
        continue;
      }

      // The hash already proves these bytes are what the index pinned, so a
      // mismatch here means the publisher assembled two documents that disagree.
      // Refusing is the only safe reading: acting on it would install a
      // different version than the chain says.
      if (manifest.product !== this.product) {
        state.recordSourceFailure(rawUrl);
        failures.push(`${rawUrl}: manifest names product "${manifest.product}"`);
        continue;
      }
      if (num(manifest.code) !== num(target.code)) {
        state.recordSourceFailure(rawUrl);
        failures.push(
          `${rawUrl}: manifest code ${num(manifest.code)} does not match ` +
            `index node ${num(target.code)}`,
        );
        continue;
      }
      if (manifest.version !== target.version) {
        state.recordSourceFailure(rawUrl);
        failures.push(
          `${rawUrl}: manifest version "${manifest.version}" does not match ` +
            `index node "${target.version}"`,
        );
        continue;
      }

      state.recordSourceSuccess(rawUrl);
      return manifest;
    }

    throw new Error(
      `no usable manifest for ${target.version}: ${failures.join("; ")}`,
    );
  }

  private async loadFallbackFrom(
    url: URL,
    state: UpdateState,
  ): Promise<SourceOutcome<Fallback>> {
    let bytes: Uint8Array;
    try {
      bytes = await this.fetcher.getBytes(
        cacheBust(url),
        this.policy.documentTimeoutMs,
      );
    } catch (error) {
      return { ok: false, why: describeFetchError(error) };
    }

    const verified = openEnvelope(bytes, this.trustedKeys);
    if (!verified.accepted) {
      return {
        ok: false,
        why: `signature check failed (${describeEnvelopeResult(verified)})`,
      };
    }

    let doc: Fallback;
    try {
      doc = parseFallback(verified.payload);
    } catch (error) {
      if (error instanceof RupFormatError) {
        return { ok: false, why: `malformed fallback: ${error.message}` };
      }
      throw error;
    }

    if (doc.product !== this.product) {
      return {
        ok: false,
        why: `fallback is for product "${doc.product}", expected "${this.product}"`,
      };
    }

    const sequence = num(doc.sequence);
    if (!acceptsSequence(sequence, state.lastSeenFallbackSequence)) {
      this.log?.(
        `${url} fallback sequence ${sequence} < ` +
          `${state.lastSeenFallbackSequence}, trying the next source`,
      );
      return {
        ok: false,
        why:
          `sequence ${sequence} is older than the last accepted ` +
          `${state.lastSeenFallbackSequence}`,
      };
    }

    return { ok: true, value: doc };
  }

  private matchFallbackRule(doc: Fallback): FallbackRule | null {
    for (const rule of doc.rules) {
      if (this.currentCode < num(rule.minCode)) continue;
      if (this.currentCode > num(rule.maxCode)) continue;
      if (!matchesSelectors(selectorsToMap(rule.selectors), this.clientSelectors)) {
        continue;
      }
      return rule;
    }
    return null;
  }
}

function describeFetchError(error: unknown): string {
  if (error instanceof FetchError) return error.detail;
  return error instanceof Error ? error.message : String(error);
}
