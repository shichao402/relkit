# updater / apply conformance

Black-box fixtures for `relkit-updater`. Root `conformance/` RUP fixtures remain SSOT for chain/signature; this tree covers engine lifecycle.

## updater/

| Fixture | Asserts |
|---------|---------|
| `facade-signatures.txt` | generated facades contain the locked method set |
| `runtime-bootstrap.json` | the generator's real bootstrap is accepted and reported back in the handshake |
| `runtime-bootstrap-numeric-key.json` | `rootPublicKey` written as a number array is rejected (2026-10-17 accident) |
| `policy-clamp.json` | 0.5h / 0 duration clamp to minCheckInterval |
| `check-result-empty-notes.json` | empty notes / `mandatory:false` still emit keys |

`runtime-bootstrap*.json` are golden files produced by the generator itself
(`runtimebootstrap.New` over a profile signed with the fixed conformance seed),
never typed by hand. Regenerate after any change to the bootstrap or profile
documents:

```
go test ./internal/updater/ -run RuntimeBootstrap -args -update
```

Run: `go test ./internal/updater ./cmd/relkit-facade-gen`；Rust 投影：`cargo test --manifest-path sdk/rust/Cargo.toml`

## apply/

Layout success / rollback are Go tests in `internal/updater`
(`TestFileSetRollback`, `TestVersionedDirAtomicActive`). `TestHandleApplyStartsWorkerBeforeAccepting`
locks the handoff boundary: an apply request cannot return `accepted` until an independent staging
worker has started. Native Windows file locks and macOS `.app` swap must run on those OS runners;
Linux tests do not substitute.

The full-install handoff is covered by the three exit paths in
`internal/updater/fullinstall_test.go`: zero exit completes, a non-zero exit is
`INSTALLER_FAILED` and not retryable, and a wizard cancel (exit 1 or 2) is
`CANCELED` and retryable. macOS refuses the whole disposition (requirement 3.11).
