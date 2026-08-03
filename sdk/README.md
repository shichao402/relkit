# Client SDKs

This directory holds **language SDKs** for consuming RUP updates. Publishing /
serving stay in `cmd/relkit` and `cmd/relkit-serve`.

| Path | Language | Module / package | Agent onboarding |
|------|----------|------------------|------------------|
| [`./`](.) (Go sources at this level) | Go | `github.com/shichao402/relkit/sdk` | [`AGENT-QUICKSTART.md`](AGENT-QUICKSTART.md) |
| [`dart/`](dart/) | Dart | package `rup_client` | [`dart/AGENT-QUICKSTART.md`](dart/AGENT-QUICKSTART.md) |

Go files stay at `sdk/*.go` so the existing module path does not break
(`go get github.com/shichao402/relkit/sdk`). Dart is nested under `sdk/dart/` so
it is obviously an SDK peer, not a host-app vendor tree.

Host products that cannot `git` from GitHub on CI (e.g. internal builders) may
keep a **mirrored copy**, but the source of truth for Dart is **`sdk/dart/` in
this repo**. Sync procedure: see that product’s `VENDORED.md` / sync script.

Greenfield Agent entry: [`../docs/agent/README.md`](../docs/agent/README.md).
