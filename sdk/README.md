# Client SDKs

This directory holds **language SDKs** for consuming RUP updates. Publishing /
serving stay in `cmd/relkit`, `cmd/relkit-serve`, and `cmd/relkit-agent`.

| Path | Language | Module / package | Agent onboarding |
|------|----------|------------------|------------------|
| [`./`](.) (Go sources at this level) | Go | `cnb.cool/shichao402/relkit/sdk` | [`AGENT-QUICKSTART.md`](AGENT-QUICKSTART.md) |
| [`dart/`](dart/) | Dart | package `rup_client` | [`dart/AGENT-QUICKSTART.md`](dart/AGENT-QUICKSTART.md) |
| [`node/`](node/) | Node / TypeScript | package `rup-client` | [`node/AGENT-QUICKSTART.md`](node/AGENT-QUICKSTART.md) |

## Go ↔ Dart ↔ Node alignment

| Capability | Dart | Go | Node |
|---|---|---|---|
| Index check + envelope verify | yes | yes | yes |
| Fallback (§12.6) | yes | yes | yes |
| `entryUrls` → directory bootstrap (§16) | yes | yes | yes |
| State store + throttle (§12.2/12.4) | yes | yes | yes |
| Source learning (§12.7) | yes | yes | yes |
| Range resume / parallel chunks | yes | yes | yes |
| Progress callback | yes | yes | yes |
| Scheduler | yes | yes | yes |
| Apply: single-binary replace | n/a (dir swap) | `sdk/apply` | n/a (host) |
| Apply: portable directory swap / DMG | yes | **not aligned** (host or Dart) | n/a (host) |

Go files stay at `sdk/*.go` so the existing module path does not break
(`go get cnb.cool/shichao402/relkit/sdk`). Dart is nested under `sdk/dart/`,
Node under `sdk/node/`.

Greenfield Agent entry: [`../docs/agent/README.md`](../docs/agent/README.md).
Publish topology: [`../docs/design/update-ingress-cos.md`](../docs/design/update-ingress-cos.md),
[`../docs/design/publish-agent.md`](../docs/design/publish-agent.md).
