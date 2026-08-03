## 0.3.1

- Add `UpdateScheduler`: start + periodic throttled `check(force: false)` with
  injectable clock/timer for hosts that want background update checks without
  re-implementing `afterSuccess` / `afterFailure`.

## 0.3.0

- Artifact download: multi-connection HTTP Range slices (default 8 workers / 4 MiB),
  single-URL retries with backoff, and resume via `.part` + `.part.meta`.
- Falls back to single-connection GET when the server does not honor Range.
- `ProgressCallback` now reports `DownloadProgress` (received / total / bytesPerSecond / eta).
- `UpdatePolicy` adds `downloadRetries`, `downloadRetryBackoff`, `downloadConcurrency`,
  `downloadChunkSize`.
- `Fetcher` gains `probe` and `downloadRange` (still `dart:io` only; no third-party downloader).

## 0.2.0

- RUP **protobuf v2** wire format: documents are `.pb`, envelope payload is Index protobuf bytes.
- Document types come from generated protobuf code (`lib/src/gen`); hand-written JSON models removed.
- Breaking: clients must consume v2 indexes republished by `relkit` ≥ 0.2.0.

## 0.1.0

- Initial JSON v1 client (superseded).
