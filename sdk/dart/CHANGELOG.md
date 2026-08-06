## 0.4.0

Everything here comes from one field failure: an apply waited a minute for the
installation to be released, the user saw nothing at all, started the
application again, and that second process locked the directory the update was
waiting for. Nothing in the system could see that an update was in flight, and
nothing told anyone it had failed.

- Add `ApplySession` (`readApplySession` / `writeApplySession` /
  `clearApplySession`): a heartbeat-backed record of an in-flight apply,
  written outside the installation and staging area so any process can read it.
  `launchApply` takes `sessionFile` and writes it before starting the staged
  copy; `runApplyMode` heartbeats through the wait and records success or the
  failure reason. Hosts use it to step aside instead of starting into the
  middle of an update, and to explain a failed update afterwards.
- `launchApply` takes `renameTimeout`, `targetCode` and `targetVersion`; how
  long an update may take is a product decision, not an SDK constant.
- `swapInstallation` takes `onWaiting`, called on every blocked attempt, and
  now logs the wait periodically instead of only when it gives up. A wait that
  leaves no trace cannot be diagnosed later.
- `unpackUpdatePackage` / `stageUpdate` retry clearing the staging directory
  (`clearTimeout`) and report failure as something a user can act on, rather
  than passing through `Deletion failed, errno = 5`.
- `cleanStagingArea` takes `keep`, so a host can hold on to a verified package
  until the update it belongs to has actually landed.
- `downloadArtifact` reuses an existing file whose sha256 matches the manifest,
  making a retry after a failed *install* free.

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
