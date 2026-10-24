# Thin client SDKs

`relkit-updater` is the only update engine (requirement 2). Every host talks to
it through a thin IPC client: spawn the engine, exchange length-prefixed
protobuf frames on stdio, render the events the engine emits. No host parses an
index, a manifest, an envelope, or an artifact list, and no host compares
versions or replaces files.

| Host | Language | Client |
|------|----------|--------|
| SvnMergeTool | Dart | [`dart/lib/src/updater_facade.dart`](dart/lib/src/updater_facade.dart) + `lib/src/gen/updater/v1` |
| Loom-Launcher | Rust | [`rust/src/lib.rs`](rust/src/lib.rs) |
| Loom | TypeScript | [`../bindings/ts`](../bindings/ts) generated types only |

The Go facade lives in `internal/updaterfacade` and is not published: no host is
written in Go, so publishing it would only invite a second engine.

## What a host has to do

```
open   = Updater.open(bootstrap, runtime)   // failed -> close the update UI
u.check(force: userInitiated)
u.download(planId)
u.apply(planId)                             // if requiresHostExit, exit
```

`open` hands the engine the runtime bootstrap document verbatim plus the product
the host expects. The engine is the only parser: it validates the bootstrap,
checks the product matches, and reports back the allowed channels, the default
channel, and the root key it trusts. A bootstrap the engine cannot validate is a
startup failure, not a degraded mode.

Host-specific extras the engine exposes: `listInstalled`, `deleteInstalled`
(refuses the active version and any version the host declares in use),
`switchActive`, `rollback`, `status`, `cancel`, `cleanup`.

## IPC baseline

`ipcMin` / `ipcMax` / `ipcCurrent` are compile-time constants on each client and
are `4` everywhere. This is the epoch upgrade: an engine and a host from
different sides of the boundary refuse each other with an explicit
`UPDATER_TOO_NEW` / `UPDATER_TOO_OLD` instead of half-working. There is no
negotiation and no compatibility window.

Signatures: [`../conformance/updater/facade-signatures.txt`](../conformance/updater/facade-signatures.txt)

## Generation

`scripts/gen-proto.ps1` runs `buf generate` twice: the default template produces
the full Go surface under `api/`, and `proto/buf.gen.dart.yaml` produces only
`updater/v1` for Dart. RUP wire types stay Go-side because a thin client never
sees them. `cmd/relkit-facade-gen` then locks the two hand-written facades
(Dart, Rust) against drift.
