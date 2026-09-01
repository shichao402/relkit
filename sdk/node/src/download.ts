/**
 * Downloading and verifying (SPEC.md section 12.3).
 *
 * Orchestrates mirror fallback, per-URL retries, optional multi-connection Range
 * downloads, and resume via `.part` + `.part.meta`.
 */

import { open, mkdir, readFile, rename, rm, stat, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { pathToFileURL } from "node:url";

import {
  FetchError,
  ThroughputMeter,
  sha256OfFile,
  type Fetcher,
  type ProgressCallback,
} from "./fetch.js";
import { checkArtifactFilename } from "./filename.js";
import { num, type Artifact } from "./models.js";
import { resolvePolicy, type UpdatePolicy } from "./state.js";

/** A file that has been fetched and whose bytes match the signed manifest. */
export interface VerifiedFile {
  path: string;
  artifact: Artifact;
  /**
   * Which mirror it actually came from. Worth logging: when one mirror is stale,
   * this is the only thing that says which.
   */
  sourceUrl: string;
}

export class VerificationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "VerificationError";
  }
}

interface PartMeta {
  sha256: string;
  size: number;
  url: string;
  mode: "single" | "parallel";
  completed: [number, number][];
}

/**
 * Downloads an artifact, trying each mirror in turn, and returns it only if the
 * bytes match.
 *
 * Mirrors are tried strictly one at a time. Racing them would finish sooner and
 * is explicitly forbidden: it multiplies load on every mirror for every client,
 * and the winner is whichever server is fastest rather than whichever is
 * correct.
 *
 * Within a single URL, multiple Range connections may run in parallel when the
 * server advertises byte ranges.
 */
export async function downloadArtifact(
  artifact: Artifact,
  options: {
    fetcher: Fetcher;
    destinationDir: string;
    policy?: Partial<UpdatePolicy>;
    onProgress?: ProgressCallback;
    log?: (message: string) => void;
  },
): Promise<VerifiedFile> {
  const policy = resolvePolicy(options.policy);
  const { fetcher, destinationDir, onProgress, log } = options;

  if (typeof destinationDir !== "string" || destinationDir.length === 0) {
    // Without this the failure surfaces from node:path as "path must be of type
    // string", which points at this library rather than at the caller's typo.
    throw new TypeError(
      "downloadArtifact: options.destinationDir must be a non-empty path",
    );
  }

  const unsafe = checkArtifactFilename(artifact.filename);
  if (unsafe !== null) {
    throw new VerificationError(`refusing artifact ${artifact.id}: ${unsafe}`);
  }

  const target = join(destinationDir, artifact.filename);
  const partial = `${target}.part`;
  const metaFile = `${target}.part.meta`;
  const expectedSize = num(artifact.size);

  // A complete copy from an earlier attempt is worth more than a fresh download:
  // it is the same bytes, and the check below is the same check that would accept
  // them at the end anyway. This is what makes retrying a failed *install* free.
  if (await exists(target)) {
    const problem = await verifyFile(target, artifact);
    if (problem === null) {
      log?.(`reusing verified ${target}`);
      await deleteQuietly(partial);
      await deleteQuietly(metaFile);
      onProgress?.({
        received: expectedSize,
        total: expectedSize,
        bytesPerSecond: 0,
        etaMs: 0,
        fraction: 1,
      });
      return {
        path: target,
        artifact,
        sourceUrl: pathToFileURL(target).toString(),
      };
    }
  }

  const failures: string[] = [];

  for (const rawUrl of artifact.urls) {
    let url: URL;
    try {
      url = new URL(rawUrl);
    } catch {
      failures.push(`${rawUrl}: not a valid URL`);
      continue;
    }

    let attempt = 0;
    let backoff = policy.downloadRetryBackoffMs;

    for (;;) {
      attempt++;
      try {
        await downloadFromUrl({
          fetcher,
          url,
          rawUrl,
          artifact,
          expectedSize,
          partial,
          metaFile,
          policy,
          ...(onProgress ? { onProgress } : {}),
          ...(log ? { log } : {}),
        });

        const problem = await verifyFile(partial, artifact);
        if (problem !== null) {
          await deleteQuietly(partial);
          await deleteQuietly(metaFile);
          failures.push(`${rawUrl}: ${problem}`);
          log?.(`rejected ${rawUrl}: ${problem}`);
          break; // next mirror; hash mismatch is not a transient retry
        }

        await deleteQuietly(target);
        await rename(partial, target);
        await deleteQuietly(metaFile);
        return { path: target, artifact, sourceUrl: rawUrl };
      } catch (error) {
        const message =
          error instanceof FetchError ? error.detail : String(error);
        failures.push(`${rawUrl}: ${message}`);
        log?.(`failed ${rawUrl} (attempt ${attempt}): ${message}`);

        const retryable =
          error instanceof FetchError ? error.isRetryable : true;
        const canRetry =
          retryable && attempt < Math.max(1, policy.downloadRetries);
        if (!canRetry) {
          await cleanupAbandonedPartial(partial, metaFile);
          break;
        }
        await delay(backoff);
        backoff *= 2;
      }
    }
  }

  throw new VerificationError(
    `could not obtain ${artifact.filename} from any of ${artifact.urls.length} ` +
      `URL(s): ${failures.join("; ")}`,
  );
}

async function downloadFromUrl(options: {
  fetcher: Fetcher;
  url: URL;
  rawUrl: string;
  artifact: Artifact;
  expectedSize: number;
  partial: string;
  metaFile: string;
  policy: UpdatePolicy;
  onProgress?: ProgressCallback;
  log?: (message: string) => void;
}): Promise<void> {
  const { fetcher, url, policy, log } = options;

  await ensurePartialCompatible(options);

  const concurrency = Math.max(1, policy.downloadConcurrency);
  let useParallel = concurrency > 1;

  if (useParallel) {
    try {
      const probe = await fetcher.probe(url, policy.documentTimeoutMs);
      useParallel = probe.acceptsRanges;
      if (
        probe.contentLength !== undefined &&
        probe.contentLength !== options.expectedSize
      ) {
        log?.(
          `probe Content-Length ${probe.contentLength} != manifest ` +
            `${options.expectedSize} for ${url} (continuing with manifest size)`,
        );
      }
    } catch (error) {
      log?.(`probe failed (${String(error)}); falling back to single connection`);
      useParallel = false;
    }
  }

  if (useParallel) {
    try {
      await downloadParallel(options);
      return;
    } catch (error) {
      if (error instanceof FetchError && error.detail.includes("range not honored")) {
        log?.("server ignored Range; falling back to single connection");
        await deleteQuietly(options.partial);
        await deleteQuietly(options.metaFile);
      } else {
        throw error;
      }
    }
  }

  await downloadSingle(options);
}

async function ensurePartialCompatible(options: {
  partial: string;
  metaFile: string;
  artifact: Artifact;
  rawUrl: string;
  expectedSize: number;
}): Promise<void> {
  const { partial, metaFile, artifact, rawUrl, expectedSize } = options;

  if (!(await exists(metaFile))) {
    if (await exists(partial)) {
      const len = await fileLength(partial);
      if (len <= 0 || len >= expectedSize) await deleteQuietly(partial);
      // Contiguous prefix without meta is fine for single-connection resume.
    }
    return;
  }

  try {
    const meta = await loadMeta(metaFile);
    const ok =
      meta.sha256 === artifact.sha256 &&
      meta.size === expectedSize &&
      meta.url === rawUrl;
    if (!ok) {
      await deleteQuietly(partial);
      await deleteQuietly(metaFile);
    }
  } catch {
    await deleteQuietly(partial);
    await deleteQuietly(metaFile);
  }
}

async function downloadSingle(options: {
  fetcher: Fetcher;
  url: URL;
  rawUrl: string;
  artifact: Artifact;
  expectedSize: number;
  partial: string;
  metaFile: string;
  policy: UpdatePolicy;
  onProgress?: ProgressCallback;
}): Promise<void> {
  const { fetcher, url, rawUrl, artifact, expectedSize, partial, metaFile, policy } =
    options;

  let startOffset = 0;
  if (await exists(partial)) {
    startOffset = await fileLength(partial);
    if (startOffset >= expectedSize) {
      await deleteQuietly(partial);
      startOffset = 0;
    }
  }

  await saveMeta(metaFile, {
    sha256: artifact.sha256,
    size: expectedSize,
    url: rawUrl,
    mode: "single",
    completed: startOffset > 0 ? [[0, startOffset - 1]] : [],
  });

  try {
    await fetcher.download(url, partial, {
      idleTimeoutMs: policy.downloadIdleTimeoutMs,
      startOffset,
      knownTotal: expectedSize,
      ...(options.onProgress ? { onProgress: options.onProgress } : {}),
    });
  } catch (error) {
    if (
      error instanceof FetchError &&
      error.detail.includes("range not honored") &&
      startOffset > 0
    ) {
      await deleteQuietly(partial);
      await saveMeta(metaFile, {
        sha256: artifact.sha256,
        size: expectedSize,
        url: rawUrl,
        mode: "single",
        completed: [],
      });
      await fetcher.download(url, partial, {
        idleTimeoutMs: policy.downloadIdleTimeoutMs,
        startOffset: 0,
        knownTotal: expectedSize,
        ...(options.onProgress ? { onProgress: options.onProgress } : {}),
      });
    } else {
      throw error;
    }
  }
}

async function downloadParallel(options: {
  fetcher: Fetcher;
  url: URL;
  rawUrl: string;
  artifact: Artifact;
  expectedSize: number;
  partial: string;
  metaFile: string;
  policy: UpdatePolicy;
  onProgress?: ProgressCallback;
  log?: (message: string) => void;
}): Promise<void> {
  const { fetcher, url, rawUrl, artifact, expectedSize, partial, metaFile, policy } =
    options;

  const chunkSize = Math.max(1024, policy.downloadChunkSize);
  const concurrency = Math.max(1, policy.downloadConcurrency);

  let meta: PartMeta = (await exists(metaFile))
    ? await loadMeta(metaFile)
    : {
        sha256: artifact.sha256,
        size: expectedSize,
        url: rawUrl,
        mode: "parallel",
        completed: [],
      };

  // Promote a contiguous single-mode prefix into completed ranges.
  if (meta.mode === "single" && (await exists(partial))) {
    const len = await fileLength(partial);
    if (len > 0 && len < expectedSize) {
      meta = { ...meta, mode: "parallel", completed: [[0, len - 1]] };
    } else {
      if (len >= expectedSize) await deleteQuietly(partial);
      meta = { ...meta, mode: "parallel", completed: [] };
    }
  } else {
    meta = { ...meta, mode: "parallel" };
  }

  // Ensure the file exists at the final size without wiping resume data.
  await mkdir(dirname(partial), { recursive: true });
  // Two opens, not one: on Windows a handle opened in append mode carries only
  // FILE_APPEND_DATA, so truncating through it fails with EPERM. "a" just
  // creates the file when missing, and "r+" is the handle allowed to resize it.
  const create = await open(partial, "a");
  await create.close();
  const handle = await open(partial, "r+");
  try {
    await handle.truncate(expectedSize);
  } finally {
    await handle.close();
  }

  const slices = planSlices(expectedSize, chunkSize);
  const pending = slices.filter(
    (slice) => !rangeCovered(meta.completed, slice[0], slice[1]),
  );

  const meter = new ThroughputMeter();
  meter.seed(completedBytes(meta.completed, expectedSize));
  const emit = (delta?: number): void => {
    options.onProgress?.(meter.observe(expectedSize, delta));
  };
  emit();

  let nextIndex = 0;
  let failure: unknown = null;
  let writeChain: Promise<void> = Promise.resolve();

  const worker = async (): Promise<void> => {
    for (;;) {
      if (failure !== null) return;
      const i = nextIndex++;
      if (i >= pending.length) return;
      const slice = pending[i]!;
      const [start, end] = slice;

      // Stage each slice into a side file, then copy under a serialised write so
      // a failed worker cannot leave a torn region in `.part`.
      const chunkFile = `${partial}.chunk.${start}-${end}`;
      try {
        await fetcher.downloadRange(url, {
          destination: chunkFile,
          start,
          endInclusive: end,
          idleTimeoutMs: policy.downloadIdleTimeoutMs,
          onBytes: (n) => emit(n),
        });

        const bytes = await readFile(chunkFile);
        const want = end - start + 1;
        if (bytes.byteLength !== want) {
          throw new FetchError(
            url,
            `chunk size mismatch: got ${bytes.byteLength} want ${want}`,
            { retryable: true },
          );
        }

        writeChain = writeChain.then(async () => {
          const out = await open(partial, "r+");
          try {
            await out.write(bytes, 0, bytes.byteLength, start);
          } finally {
            await out.close();
          }
          meta = {
            ...meta,
            completed: mergeRanges([...meta.completed, [start, end]]),
          };
          await saveMeta(metaFile, meta);
        });
        await writeChain;
      } catch (error) {
        failure ??= error;
        throw error;
      } finally {
        await deleteQuietly(chunkFile);
      }
    }
  };

  const workers = Array.from(
    { length: Math.min(concurrency, Math.max(1, pending.length)) },
    () => worker(),
  );

  const results = await Promise.allSettled(workers);
  await writeChain.catch(() => undefined);

  const rejected = results.find((r) => r.status === "rejected");
  if (rejected !== undefined && rejected.status === "rejected") {
    const reason: unknown = rejected.reason;
    if (reason instanceof FetchError) throw reason;
    throw new FetchError(url, `parallel download failed: ${String(reason)}`);
  }

  if (pending.length === 0) {
    options.log?.(`all ranges already present for ${url}`);
  }
  emit();
}

/** Plans inclusive [start, end] slices covering `[0, size)`. */
function planSlices(size: number, chunkSize: number): [number, number][] {
  if (size <= 0) return [];
  const out: [number, number][] = [];
  for (let start = 0; start < size; start += chunkSize) {
    out.push([start, Math.min(size - 1, start + chunkSize - 1)]);
  }
  return out;
}

function rangeCovered(
  completed: readonly [number, number][],
  start: number,
  end: number,
): boolean {
  return completed.some(([a, b]) => a <= start && b >= end);
}

function completedBytes(
  completed: readonly [number, number][],
  size: number,
): number {
  let n = 0;
  for (const [rawA, rawB] of completed) {
    const a = Math.max(0, rawA);
    const b = Math.min(size - 1, rawB);
    if (b >= a) n += b - a + 1;
  }
  return n;
}

function mergeRanges(input: readonly [number, number][]): [number, number][] {
  if (input.length === 0) return [];
  const sorted = [...input].sort((a, b) => a[0] - b[0]);
  const out: [number, number][] = [[sorted[0]![0], sorted[0]![1]]];
  for (let i = 1; i < sorted.length; i++) {
    const cur = sorted[i]!;
    const last = out[out.length - 1]!;
    if (cur[0] <= last[1] + 1) {
      last[1] = Math.max(last[1], cur[1]);
    } else {
      out.push([cur[0], cur[1]]);
    }
  }
  return out;
}

/**
 * Size first, then hash. Size is free and rules out the common case of a
 * truncated transfer or an error page served with status 200, so the expensive
 * check only runs on plausible input.
 */
async function verifyFile(
  path: string,
  artifact: Artifact,
): Promise<string | null> {
  const expectedSize = num(artifact.size);
  const length = await fileLength(path);
  if (length !== expectedSize) {
    return `expected ${expectedSize} bytes, got ${length}`;
  }
  const actual = await sha256OfFile(path);
  if (actual !== artifact.sha256) {
    return `sha256 mismatch (expected ${artifact.sha256}, got ${actual})`;
  }
  return null;
}

async function cleanupAbandonedPartial(
  partial: string,
  metaFile: string,
): Promise<void> {
  const hasBytes = (await exists(partial)) && (await fileLength(partial)) > 0;
  if (!hasBytes) {
    await deleteQuietly(partial);
    await deleteQuietly(metaFile);
  }
}

async function loadMeta(path: string): Promise<PartMeta> {
  const raw = JSON.parse(await readFile(path, "utf8")) as Record<string, unknown>;
  const completed: [number, number][] = [];
  if (Array.isArray(raw.completed)) {
    for (const item of raw.completed) {
      if (Array.isArray(item) && item.length >= 2) {
        completed.push([Number(item[0]), Number(item[1])]);
      }
    }
  }
  return {
    sha256: String(raw.sha256),
    size: Number(raw.size),
    url: String(raw.url),
    mode: raw.mode === "single" ? "single" : "parallel",
    completed,
  };
}

async function saveMeta(path: string, meta: PartMeta): Promise<void> {
  await mkdir(dirname(path), { recursive: true });
  const temp = `${path}.tmp`;
  await writeFile(temp, JSON.stringify(meta), "utf8");
  await rename(temp, path);
}

async function exists(path: string): Promise<boolean> {
  try {
    await stat(path);
    return true;
  } catch {
    return false;
  }
}

async function fileLength(path: string): Promise<number> {
  try {
    return (await stat(path)).size;
  } catch {
    return 0;
  }
}

async function deleteQuietly(path: string): Promise<void> {
  try {
    await rm(path, { force: true });
  } catch {
    // Best effort. A leftover .part file is untidy but harmless, and failing the
    // update because cleanup failed would be worse.
  }
}

const delay = (ms: number): Promise<void> =>
  new Promise((resolve) => setTimeout(resolve, ms));
