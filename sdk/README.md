# Client SDKs

This directory holds **language SDKs** for consuming RUP updates. Publishing /
serving stay in `cmd/relkit`, `cmd/relkit-serve`, and `cmd/relkit-agent`.

| Path | Language | Module / package | Agent onboarding |
|------|----------|------------------|------------------|
| [`./`](.) (Go sources at this level) | Go | `github.com/shichao402/relkit/sdk` | [`AGENT-QUICKSTART.md`](AGENT-QUICKSTART.md) |
| [`dart/`](dart/) | Dart | package `rup_client` | [`dart/AGENT-QUICKSTART.md`](dart/AGENT-QUICKSTART.md) |

## Go ↔ Dart alignment

| Capability | Dart | Go |
|---|---|---|
| Index check + envelope verify | yes | yes |
| Fallback (§12.6) | yes | yes |
| `entryUrls` → directory bootstrap (§16) | yes | yes |
| State store + throttle (§12.2/12.4) | yes | yes |
| Source learning (§12.7) | yes | yes |
| Range resume / parallel chunks | yes | yes |
| Progress callback | yes | yes |
| Scheduler | yes | yes |
| Apply: single-binary replace | n/a (dir swap) | `sdk/apply` |
| Apply: portable directory swap / DMG | yes | **not aligned** (host or Dart) |

Go files stay at `sdk/*.go` so the existing module path does not break
(`go get github.com/shichao402/relkit/sdk`). Dart is nested under `sdk/dart/`.

Greenfield Agent entry: [`../docs/agent/README.md`](../docs/agent/README.md).
Publish topology: [`../docs/design/update-ingress-cos.md`](../docs/design/update-ingress-cos.md),
[`../docs/design/publish-agent.md`](../docs/design/publish-agent.md).
