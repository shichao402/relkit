# updater / apply conformance

Black-box fixtures for `relkit-updater`. Root `conformance/` RUP fixtures remain SSOT for chain/signature; this tree covers engine lifecycle.

## updater/

| Fixture | Asserts |
|---------|---------|
| `facade-signatures.txt` | generated facades contain the locked method set |
| `legacy-lastResult.json` | Go + Dart disk strings map to LastResult enum |
| `policy-clamp.json` | 0.5h / 0 duration clamp to minCheckInterval |
| `check-result-empty-notes.json` | empty notes / `mandatory:false` still emit keys |

Run: `go test ./internal/updater ./cmd/relkit-facade-gen`；Rust 投影：`cargo test --manifest-path sdk/rust/Cargo.toml`

## apply/

Layout success / rollback are Go tests in `internal/updater`
(`TestFileSetRollback`, `TestVersionedDirAtomicActive`). `TestHandleApplyStartsWorkerBeforeAccepting`
locks the handoff boundary: an apply request cannot return `accepted` until an independent staging
worker has started. Native Windows file locks and macOS `.app` swap must run on those OS runners;
Linux tests do not substitute.
