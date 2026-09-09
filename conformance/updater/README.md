# updater / apply conformance

Black-box fixtures for `relkit-updater`. Root `conformance/` RUP fixtures remain SSOT for chain/signature; this tree covers engine lifecycle.

## updater/

| Fixture | Asserts |
|---------|---------|
| `facade-signatures.txt` | generated facades contain the locked method set |
| `legacy-lastResult.json` | Go + Dart disk strings map to LastResult enum |
| `policy-clamp.json` | 0.5h / 0 duration clamp to minCheckInterval |
| `filename-reject.json` | path escape rejected |

Run: `go test ./internal/updater ./cmd/relkit-facade-gen`

## apply/

Layout success / rollback are Go tests in `internal/updater` (`TestFileSetRollback`, `TestVersionedDirAtomicActive`). Native Windows file locks and macOS `.app` swap must run on those OS runners; Linux tests do not substitute.
