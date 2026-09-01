/**
 * Persisted client state and the throttling rules around it (SPEC.md sections
 * 12.2, 12.4, 12.7, and 16.2).
 */

import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";

/**
 * Whether an index with this sequence may be adopted (SPEC.md section 12.4).
 *
 * Equal is accepted: re-fetching the same index is the ordinary case, not an
 * attack. Strictly smaller is refused, because a valid signature proves only
 * that the publisher issued the document at some point, never that it is the
 * newest one, so a network attacker could otherwise replay an old index to
 * steer clients onto a version with known holes.
 *
 * A refusal here is not an error to report. Mirrors legitimately lag behind one
 * another, and surfacing that to the user as a failure would train them to
 * ignore a message that usually means nothing.
 */
export function acceptsSequence(
  sequence: number,
  lastSeenSequence: number | null | undefined,
): boolean {
  return (
    lastSeenSequence === null ||
    lastSeenSequence === undefined ||
    sequence >= lastSeenSequence
  );
}

/** Stats for one candidate source key (URL or `service:<id>`). */
export interface SourceStat {
  successes: number;
  failures: number;
  consecutiveFailures: number;
  lastBytesPerSecond?: number;
  lastSuccessAt?: string;
  lastFailureAt?: string;
}

function emptyStat(): SourceStat {
  return { successes: 0, failures: 0, consecutiveFailures: 0 };
}

/** What the client remembers between runs, per (product, channel). */
export class UpdateState {
  lastCheckAt: Date | null = null;
  lastResult: string | null = null;

  /** The highest index sequence ever accepted. Never lower it. */
  lastSeenSequence: number | null = null;

  /** The highest fallback sequence ever accepted (product-scoped). */
  lastSeenFallbackSequence: number | null = null;

  /** The highest directory sequence ever accepted (product-scoped). */
  lastSeenDirectorySequence: number | null = null;

  /** Key of the last candidate that fully succeeded (URL or `service:<id>`). */
  lastSuccessfulSourceKey: string | null = null;

  /** Per-source outcomes from real attempts only (SPEC section 12.7). */
  readonly sourceStats = new Map<string, SourceStat>();

  /** Codes the user chose to skip. Ignored when an update is mandatory. */
  readonly skipped = new Set<number>();

  static fromJson(raw: unknown): UpdateState {
    const state = new UpdateState();
    if (typeof raw !== "object" || raw === null) return state;
    const json = raw as Record<string, unknown>;

    if (typeof json.lastCheckAt === "string") {
      const parsed = new Date(json.lastCheckAt);
      state.lastCheckAt = Number.isNaN(parsed.getTime()) ? null : parsed;
    }
    if (typeof json.lastResult === "string") state.lastResult = json.lastResult;
    if (typeof json.lastSeenSequence === "number") {
      state.lastSeenSequence = json.lastSeenSequence;
    }
    if (typeof json.lastSeenFallbackSequence === "number") {
      state.lastSeenFallbackSequence = json.lastSeenFallbackSequence;
    }
    if (typeof json.lastSeenDirectorySequence === "number") {
      state.lastSeenDirectorySequence = json.lastSeenDirectorySequence;
    }
    if (typeof json.lastSuccessfulSourceKey === "string") {
      state.lastSuccessfulSourceKey = json.lastSuccessfulSourceKey;
    }
    if (typeof json.sourceStats === "object" && json.sourceStats !== null) {
      for (const [key, value] of Object.entries(
        json.sourceStats as Record<string, unknown>,
      )) {
        if (typeof value !== "object" || value === null) continue;
        const entry = value as Record<string, unknown>;
        const stat = emptyStat();
        if (typeof entry.successes === "number") stat.successes = entry.successes;
        if (typeof entry.failures === "number") stat.failures = entry.failures;
        if (typeof entry.consecutiveFailures === "number") {
          stat.consecutiveFailures = entry.consecutiveFailures;
        }
        if (typeof entry.lastBytesPerSecond === "number") {
          stat.lastBytesPerSecond = entry.lastBytesPerSecond;
        }
        if (typeof entry.lastSuccessAt === "string") {
          stat.lastSuccessAt = entry.lastSuccessAt;
        }
        if (typeof entry.lastFailureAt === "string") {
          stat.lastFailureAt = entry.lastFailureAt;
        }
        state.sourceStats.set(key, stat);
      }
    }
    if (Array.isArray(json.skipped)) {
      for (const code of json.skipped) {
        if (typeof code === "number") state.skipped.add(code);
      }
    }
    return state;
  }

  toJson(): Record<string, unknown> {
    const out: Record<string, unknown> = {};
    if (this.lastCheckAt !== null) {
      out.lastCheckAt = this.lastCheckAt.toISOString();
    }
    if (this.lastResult !== null) out.lastResult = this.lastResult;
    if (this.lastSeenSequence !== null) {
      out.lastSeenSequence = this.lastSeenSequence;
    }
    if (this.lastSeenFallbackSequence !== null) {
      out.lastSeenFallbackSequence = this.lastSeenFallbackSequence;
    }
    if (this.lastSeenDirectorySequence !== null) {
      out.lastSeenDirectorySequence = this.lastSeenDirectorySequence;
    }
    if (this.lastSuccessfulSourceKey !== null) {
      out.lastSuccessfulSourceKey = this.lastSuccessfulSourceKey;
    }
    if (this.sourceStats.size > 0) {
      out.sourceStats = Object.fromEntries(this.sourceStats);
    }
    out.skipped = [...this.skipped].sort((a, b) => a - b);
    return out;
  }

  /** Records a newly accepted sequence, never moving the high-water mark down. */
  observeSequence(sequence: number): void {
    if (this.lastSeenSequence === null || sequence > this.lastSeenSequence) {
      this.lastSeenSequence = sequence;
    }
  }

  observeFallbackSequence(sequence: number): void {
    if (
      this.lastSeenFallbackSequence === null ||
      sequence > this.lastSeenFallbackSequence
    ) {
      this.lastSeenFallbackSequence = sequence;
    }
  }

  observeDirectorySequence(sequence: number): void {
    if (
      this.lastSeenDirectorySequence === null ||
      sequence > this.lastSeenDirectorySequence
    ) {
      this.lastSeenDirectorySequence = sequence;
    }
  }

  private statFor(key: string): SourceStat {
    let stat = this.sourceStats.get(key);
    if (stat === undefined) {
      stat = emptyStat();
      this.sourceStats.set(key, stat);
    }
    return stat;
  }

  recordSourceSuccess(key: string, bytesPerSecond?: number): void {
    if (key.length === 0) return;
    const stat = this.statFor(key);
    stat.successes += 1;
    stat.consecutiveFailures = 0;
    stat.lastSuccessAt = new Date().toISOString();
    if (bytesPerSecond !== undefined && bytesPerSecond > 0) {
      stat.lastBytesPerSecond = bytesPerSecond;
    }
    this.lastSuccessfulSourceKey = key;
  }

  recordSourceFailure(key: string): void {
    if (key.length === 0) return;
    const stat = this.statFor(key);
    stat.failures += 1;
    stat.consecutiveFailures += 1;
    stat.lastFailureAt = new Date().toISOString();
  }
}

export interface UpdateStateStore {
  load(): Promise<UpdateState>;
  save(state: UpdateState): Promise<void>;
}

const sanitize = (value: string): string =>
  value.replace(/[^A-Za-z0-9._-]/g, "_");

/**
 * Keeps state in a JSON file, one per (product, channel).
 *
 * Separate files rather than one shared document: two channels of the same app
 * have independent sequences, and mixing them would let a beta index hold back
 * a stable one.
 */
export class FileUpdateStateStore implements UpdateStateStore {
  readonly path: string;

  constructor(options: {
    directory: string;
    product: string;
    channel: string;
  }) {
    this.path = join(
      options.directory,
      `rup-state-${sanitize(options.product)}-${sanitize(options.channel)}.json`,
    );
  }

  async load(): Promise<UpdateState> {
    try {
      const raw = await readFile(this.path, "utf8");
      return UpdateState.fromJson(JSON.parse(raw));
    } catch {
      // Corrupt or missing state must not block updating. The only thing lost is
      // the rollback high-water mark, which the next successful check restores;
      // refusing to run would be the worse failure, since it disables updates
      // permanently until someone deletes a file they do not know about.
      return new UpdateState();
    }
  }

  async save(state: UpdateState): Promise<void> {
    await mkdir(dirname(this.path), { recursive: true });
    const temp = `${this.path}.tmp`;
    await writeFile(temp, JSON.stringify(state.toJson()), "utf8");
    await rename(temp, this.path);
  }
}

/** In-memory store, for tests and for hosts that do not want a file. */
export class MemoryUpdateStateStore implements UpdateStateStore {
  private state: UpdateState;

  constructor(initial?: UpdateState) {
    this.state = initial ?? new UpdateState();
  }

  async load(): Promise<UpdateState> {
    return this.state;
  }

  async save(state: UpdateState): Promise<void> {
    this.state = state;
  }
}

/** The intervals from SPEC.md section 12.2. Hosts may override them. */
export interface UpdatePolicy {
  /** Wait after a successful check. Default 24h. */
  afterSuccessMs: number;
  /** Wait after a failed check. Default 1h. */
  afterFailureMs: number;
  /** Applies to index and manifest fetches, which are small. Default 10s. */
  documentTimeoutMs: number;
  /**
   * Applies between chunks of an artifact download, never to the whole
   * transfer: a large package on a slow link legitimately takes minutes, so any
   * total timeout is either useless or actively harmful. Default 60s.
   */
  downloadIdleTimeoutMs: number;
  /** Attempts per mirror URL for transient failures. Default 3. */
  downloadRetries: number;
  /** Initial backoff between retries; doubles after each failure. Default 500ms. */
  downloadRetryBackoffMs: number;
  /** Parallel Range workers for one URL. `1` forces single-connection mode. */
  downloadConcurrency: number;
  /** Preferred Range slice size in bytes (last slice truncated to EOF). */
  downloadChunkSize: number;
}

export const defaultUpdatePolicy: UpdatePolicy = {
  afterSuccessMs: 24 * 60 * 60 * 1000,
  afterFailureMs: 60 * 60 * 1000,
  documentTimeoutMs: 10_000,
  downloadIdleTimeoutMs: 60_000,
  downloadRetries: 3,
  downloadRetryBackoffMs: 500,
  downloadConcurrency: 8,
  downloadChunkSize: 4 * 1024 * 1024,
};

export function resolvePolicy(policy?: Partial<UpdatePolicy>): UpdatePolicy {
  return { ...defaultUpdatePolicy, ...policy };
}

/** Whether enough time has passed since the last check. */
export function shouldCheck(
  policy: UpdatePolicy,
  state: UpdateState,
  now: Date = new Date(),
): boolean {
  const last = state.lastCheckAt;
  if (last === null) return true;
  const wait =
    state.lastResult === "error" ? policy.afterFailureMs : policy.afterSuccessMs;
  return now.getTime() - last.getTime() >= wait;
}
