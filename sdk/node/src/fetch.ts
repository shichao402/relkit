/**
 * The network boundary (SPEC.md section 3.2).
 *
 * An interface rather than direct `fetch` calls, so the orchestration in
 * `updater.ts` can be tested without a server, and so a host that must route
 * through its own proxy or client certificate can substitute its own.
 */

import { createHash } from "node:crypto";
import { createReadStream, createWriteStream } from "node:fs";
import { mkdir } from "node:fs/promises";
import { dirname } from "node:path";
import { Readable } from "node:stream";
import { pipeline } from "node:stream/promises";

export class FetchError extends Error {
  readonly url: string;
  readonly statusCode?: number;
  private readonly retryable?: boolean;

  constructor(
    url: string | URL,
    message: string,
    options: { statusCode?: number; retryable?: boolean } = {},
  ) {
    super(`FetchError(${String(url)}): ${message}`);
    this.name = "FetchError";
    this.url = String(url);
    this.detail = message;
    this.statusCode = options.statusCode;
    this.retryable = options.retryable;
  }

  readonly detail: string;

  get isRetryable(): boolean {
    if (this.retryable !== undefined) return this.retryable;
    const code = this.statusCode;
    if (code === undefined) return true; // connection / timeout
    if (code === 408 || code === 429) return true;
    return code >= 500 && code <= 599;
  }
}

/** Progress of an artifact download, including a smoothed throughput estimate. */
export interface DownloadProgress {
  received: number;
  total: number;
  bytesPerSecond: number;
  etaMs?: number;
  fraction?: number;
}

export type ProgressCallback = (progress: DownloadProgress) => void;

/** Result of probing a URL for size and Range support. */
export interface ResourceProbe {
  acceptsRanges: boolean;
  contentLength?: number;
}

/** Sliding-window bytes/sec from incremental byte reports. */
export class ThroughputMeter {
  private readonly samples: { at: number; bytes: number }[] = [];
  private receivedBytes = 0;

  constructor(private readonly windowMs = 1000) {}

  get received(): number {
    return this.receivedBytes;
  }

  seed(alreadyReceived: number): void {
    this.receivedBytes = alreadyReceived;
  }

  observe(total: number, delta?: number): DownloadProgress {
    if (delta !== undefined && delta > 0) {
      this.receivedBytes += delta;
      const now = Date.now();
      this.samples.push({ at: now, bytes: delta });
      const cutoff = now - this.windowMs;
      while (this.samples.length > 0 && this.samples[0]!.at < cutoff) {
        this.samples.shift();
      }
    }

    let bps = 0;
    if (this.samples.length === 1) {
      const elapsed = Math.max(1, Date.now() - this.samples[0]!.at);
      bps = (this.samples[0]!.bytes / elapsed) * 1000;
    } else if (this.samples.length > 1) {
      const spanMs = Math.max(
        1,
        this.samples[this.samples.length - 1]!.at - this.samples[0]!.at,
      );
      const windowBytes = this.samples.reduce((sum, s) => sum + s.bytes, 0);
      bps = (windowBytes / spanMs) * 1000;
    }

    const progress: DownloadProgress = {
      received: this.receivedBytes,
      total,
      bytesPerSecond: bps,
    };
    if (total > 0) progress.fraction = this.receivedBytes / total;
    if (bps > 1 && total > this.receivedBytes) {
      progress.etaMs = Math.round(((total - this.receivedBytes) / bps) * 1000);
    }
    return progress;
  }
}

export interface Fetcher {
  /** Fetches a small document whole. */
  getBytes(url: URL, timeoutMs: number): Promise<Uint8Array>;

  /** Probes whether the resource supports byte ranges (and optional length). */
  probe(url: URL, timeoutMs: number): Promise<ResourceProbe>;

  /**
   * Streams a large file to disk (single connection). When `startOffset > 0`,
   * sends `Range: bytes=startOffset-` and appends. A `200` response means the
   * server ignored Range: this throws `FetchError` with detail
   * `range not honored` so the caller can truncate and restart.
   */
  download(
    url: URL,
    destination: string,
    options: {
      idleTimeoutMs: number;
      startOffset?: number;
      knownTotal?: number;
      onProgress?: ProgressCallback;
    },
  ): Promise<void>;

  /**
   * Fetches inclusive byte range `[start, endInclusive]` into `destination` as a
   * contiguous file starting at offset 0. Requires HTTP 206.
   */
  downloadRange(
    url: URL,
    options: {
      destination: string;
      start: number;
      endInclusive: number;
      idleTimeoutMs: number;
      onBytes?: (bytesJustReceived: number) => void;
    },
  ): Promise<void>;

  close(): void;
}

const REDIRECT_STATUSES = new Set([301, 302, 303, 307, 308]);

/**
 * Redirect limit from SPEC.md section 3.2. Object storage and release hosts
 * redirect routinely, so refusing to follow any would break real deployments;
 * allowing unbounded ones invites a loop.
 */
export const MAX_REDIRECTS = 5;

/**
 * Appends a cache-busting query parameter for mutable documents (SPEC §3.2).
 *
 * Immutable objects (manifest, artifact) must NOT be passed through this: they
 * are content-addressed, and busting their cache only removes the CDN benefit
 * that makes a release survive its own launch day.
 */
export function cacheBust(url: URL): URL {
  const out = new URL(url.toString());
  out.searchParams.set("t", String(Math.floor(Date.now() / 1000)));
  return out;
}

/** The default `Fetcher`, on global `fetch` (undici). */
export class HttpFetcher implements Fetcher {
  private readonly userAgent: string;

  constructor(options: { userAgent?: string } = {}) {
    this.userAgent = options.userAgent ?? "rup-client/2";
  }

  async getBytes(url: URL, timeoutMs: number): Promise<Uint8Array> {
    const response = await this.open(url, {
      timeoutMs,
      headers: { "cache-control": "no-cache" },
    });
    try {
      const buffer = await withTimeout(
        response.arrayBuffer(),
        timeoutMs,
        () => new FetchError(url, `timed out after ${timeoutMs}ms`),
      );
      return new Uint8Array(buffer);
    } catch (error) {
      if (error instanceof FetchError) throw error;
      throw new FetchError(url, `read failed: ${String(error)}`);
    }
  }

  async probe(url: URL, timeoutMs: number): Promise<ResourceProbe> {
    // Prefer HEAD. Some CDNs mishandle HEAD; fall back to a one-byte Range GET.
    try {
      const response = await this.open(url, { timeoutMs, method: "HEAD" });
      await drain(response);
      const length = parseLength(response.headers.get("content-length"));
      const result: ResourceProbe = {
        acceptsRanges: acceptsBytesRanges(response.headers.get("accept-ranges")),
      };
      if (length !== undefined) result.contentLength = length;
      return result;
    } catch (error) {
      if (!(error instanceof FetchError)) throw error;
      const status = error.statusCode;
      if (status !== 405 && status !== 501) {
        if (status !== undefined && status >= 400 && status < 500) throw error;
      }
    }

    const response = await this.open(url, {
      timeoutMs,
      headers: { range: "bytes=0-0" },
    });
    if (response.status === 206) {
      const total = totalFromContentRange(response.headers.get("content-range"));
      await drain(response);
      const result: ResourceProbe = { acceptsRanges: true };
      if (total !== undefined) result.contentLength = total;
      return result;
    }
    if (response.status === 200) {
      const length = parseLength(response.headers.get("content-length"));
      await drain(response);
      const result: ResourceProbe = { acceptsRanges: false };
      if (length !== undefined) result.contentLength = length;
      return result;
    }
    await drain(response);
    throw new FetchError(url, `HTTP ${response.status}`, {
      statusCode: response.status,
    });
  }

  async download(
    url: URL,
    destination: string,
    options: {
      idleTimeoutMs: number;
      startOffset?: number;
      knownTotal?: number;
      onProgress?: ProgressCallback;
    },
  ): Promise<void> {
    const startOffset = options.startOffset ?? 0;
    const headers: Record<string, string> = {};
    if (startOffset > 0) headers.range = `bytes=${startOffset}-`;

    const response = await this.open(url, {
      timeoutMs: options.idleTimeoutMs,
      headers,
    });

    if (startOffset > 0) {
      if (response.status === 200) {
        await drain(response);
        throw new FetchError(url, "range not honored", {
          statusCode: 200,
          retryable: false,
        });
      }
      if (response.status !== 206) {
        await drain(response);
        throw new FetchError(url, `HTTP ${response.status}`, {
          statusCode: response.status,
        });
      }
    }

    const contentLength = parseLength(response.headers.get("content-length"));
    const total =
      options.knownTotal ??
      (startOffset > 0 && contentLength !== undefined
        ? startOffset + contentLength
        : contentLength) ??
      0;

    await mkdir(dirname(destination), { recursive: true });
    const meter = new ThroughputMeter();
    meter.seed(startOffset);

    const sink = createWriteStream(destination, {
      flags: startOffset > 0 ? "a" : "w",
    });
    const source = toNodeStream(url, response, options.idleTimeoutMs, (n) => {
      const progress = meter.observe(total, n);
      options.onProgress?.(progress);
    });

    await pipeline(source, sink);
  }

  async downloadRange(
    url: URL,
    options: {
      destination: string;
      start: number;
      endInclusive: number;
      idleTimeoutMs: number;
      onBytes?: (bytesJustReceived: number) => void;
    },
  ): Promise<void> {
    const { start, endInclusive } = options;
    if (endInclusive < start) throw new RangeError("endInclusive < start");
    const expected = endInclusive - start + 1;

    const response = await this.open(url, {
      timeoutMs: options.idleTimeoutMs,
      headers: { range: `bytes=${start}-${endInclusive}` },
    });

    if (response.status === 200) {
      await drain(response);
      throw new FetchError(url, "range not honored", {
        statusCode: 200,
        retryable: false,
      });
    }
    if (response.status !== 206) {
      await drain(response);
      throw new FetchError(url, `HTTP ${response.status}`, {
        statusCode: response.status,
      });
    }

    await mkdir(dirname(options.destination), { recursive: true });
    let got = 0;
    const sink = createWriteStream(options.destination, { flags: "w" });
    const source = toNodeStream(url, response, options.idleTimeoutMs, (n) => {
      got += n;
      options.onBytes?.(n);
    });
    await pipeline(source, sink);

    if (got !== expected) {
      throw new FetchError(url, `range size mismatch: got ${got} want ${expected}`, {
        retryable: true,
      });
    }
  }

  close(): void {
    // Global fetch keeps its own pooled agent; nothing to release.
  }

  private async open(
    url: URL,
    options: {
      timeoutMs: number;
      method?: string;
      headers?: Record<string, string>;
    },
  ): Promise<Response> {
    if (url.protocol !== "http:" && url.protocol !== "https:") {
      throw new FetchError(url, `unsupported scheme "${url.protocol}"`);
    }

    const send = async (target: URL): Promise<Response> => {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), options.timeoutMs);
      try {
        return await fetch(target, {
          method: options.method ?? "GET",
          headers: { "user-agent": this.userAgent, ...options.headers },
          redirect: "manual",
          signal: controller.signal,
        });
      } catch (error) {
        if (controller.signal.aborted) {
          throw new FetchError(target, `timed out after ${options.timeoutMs}ms`);
        }
        throw new FetchError(target, `connection failed: ${String(error)}`);
      } finally {
        clearTimeout(timer);
      }
    };

    let current = url;
    let response = await send(current);

    for (let hop = 0; hop < MAX_REDIRECTS; hop++) {
      if (!REDIRECT_STATUSES.has(response.status)) break;

      const location = response.headers.get("location");
      if (location === null) {
        throw new FetchError(current, "redirect without a Location header", {
          statusCode: response.status,
        });
      }
      const next = new URL(location, current);
      if (current.protocol === "https:" && next.protocol !== "https:") {
        throw new FetchError(
          current,
          `refusing redirect from https to ${next.protocol}`,
        );
      }
      await drain(response);
      response = await send(next);
      current = next;
    }

    if (REDIRECT_STATUSES.has(response.status)) {
      throw new FetchError(current, `more than ${MAX_REDIRECTS} redirects`, {
        statusCode: response.status,
      });
    }

    if (response.status >= 400) {
      const status = response.status;
      await drain(response);
      throw new FetchError(current, `HTTP ${status}`, { statusCode: status });
    }
    return response;
  }
}

function toNodeStream(
  url: URL,
  response: Response,
  idleTimeoutMs: number,
  onBytes: (n: number) => void,
): Readable {
  const body = response.body;
  if (body === null) return Readable.from([]);

  const reader = body.getReader();
  return new Readable({
    async read() {
      try {
        const { done, value } = await withTimeout(
          reader.read(),
          idleTimeoutMs,
          () =>
            new FetchError(url, `stalled for ${idleTimeoutMs}ms`, {
              retryable: true,
            }),
        );
        if (done) {
          this.push(null);
          return;
        }
        onBytes(value.byteLength);
        this.push(Buffer.from(value));
      } catch (error) {
        this.destroy(
          error instanceof Error
            ? error
            : new FetchError(url, `read failed: ${String(error)}`),
        );
      }
    },
  });
}

async function withTimeout<T>(
  promise: Promise<T>,
  timeoutMs: number,
  onTimeout: () => Error,
): Promise<T> {
  let timer: NodeJS.Timeout | undefined;
  try {
    return await Promise.race([
      promise,
      new Promise<never>((_, reject) => {
        timer = setTimeout(() => reject(onTimeout()), timeoutMs);
      }),
    ]);
  } finally {
    if (timer !== undefined) clearTimeout(timer);
  }
}

async function drain(response: Response): Promise<void> {
  try {
    await response.body?.cancel();
  } catch {
    // Best effort.
  }
}

function acceptsBytesRanges(raw: string | null): boolean {
  if (raw === null) return false;
  return raw
    .toLowerCase()
    .split(",")
    .some((part) => part.trim() === "bytes");
}

function parseLength(raw: string | null): number | undefined {
  if (raw === null) return undefined;
  const value = Number.parseInt(raw, 10);
  return Number.isFinite(value) && value >= 0 ? value : undefined;
}

function totalFromContentRange(raw: string | null): number | undefined {
  if (raw === null) return undefined;
  const slash = raw.lastIndexOf("/");
  if (slash < 0 || slash + 1 >= raw.length) return undefined;
  const value = Number.parseInt(raw.slice(slash + 1), 10);
  return Number.isFinite(value) ? value : undefined;
}

/** Hashes bytes already in memory. For manifests, which are small by design. */
export function sha256OfBytes(bytes: Uint8Array): string {
  return createHash("sha256").update(bytes).digest("hex");
}

/** Streams the file through the hash rather than reading it whole. */
export async function sha256OfFile(path: string): Promise<string> {
  const hash = createHash("sha256");
  await pipeline(createReadStream(path), hash);
  return hash.digest("hex");
}
