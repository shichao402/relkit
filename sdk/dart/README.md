# rup_client

Thin Dart client for the `relkit-updater` engine. The package spawns the engine
and speaks the `relkit.updater.v1` IPC protocol with it; it contains no update
logic of its own.

`relkit/sdk/dart` is the source of truth. Host wiring: SvnMergeTool consumes this
package directly (requirement 4.1); there is no client-side materialization step
any more.

Pure Dart, no Flutter SDK dependency.

## Install

From this monorepo path (local):

```yaml
dependencies:
  rup_client:
    path: /path/to/relkit/sdk/dart
```

From Git:

```yaml
dependencies:
  rup_client:
    git:
      url: https://github.com/shichao402/relkit.git
      path: sdk/dart
      ref: main   # or a release tag
```

Internal CI that cannot reach GitHub should mirror this tree into the host repo
and document the sync command (do not treat the mirror as upstream).

## What this package contains

- `lib/src/updater_facade.dart` — the hand-written facade: spawn the engine,
  frame protobuf messages on stdio, expose `check`, `download`, `apply`,
  `status`, `cancel`, `cleanup`, `listInstalled`, `deleteInstalled`,
  `switchActive`, `rollback`.
- `lib/src/gen/updater/v1/` — generated protobuf types for the IPC protocol.
- `lib/rup_client.dart` and `lib/updater_v1.dart` — export surfaces.

Nothing else. Signature verification, index/manifest/envelope parsing, selector
matching, version comparison, artifact download and file replacement all live in
the engine (requirement 4.3). If a host needs a decision, it asks the engine.

## Opening the engine

```dart
final opened = await Updater.open(bootstrapJson, runtime: UpdateRuntime.detect(product: 'svnmergetool'));
final updater = switch (opened) {
  OpenResult.open(:final updater) => updater,
  OpenResult.failed(:final reason) => throw StateError('updates unavailable: $reason'),
};
```

`bootstrapJson` is the runtime bootstrap document, passed through verbatim. The
engine validates it, checks the product matches, and reports back the allowed
channels, the default channel, and the root key it trusts. A bootstrap the engine
rejects is a hard startup failure, so the host should close its update UI rather
than fall back to a degraded mode.

Manual checks from a settings screen call `updater.check(force: true)`.

## IPC baseline

`ipcMin`, `ipcMax` and `ipcCurrent` are compile-time constants and all equal `4`.
An engine and a client from different sides of the boundary refuse each other
with an explicit `UPDATER_TOO_OLD` / `UPDATER_TOO_NEW`. There is no negotiation
window and no compatibility shim: this is the epoch boundary.

## Generated code

`lib/src/gen/updater/v1/` is produced by `proto/buf.gen.dart.yaml`, which
generates the updater surface only. RUP wire types are Go-side and are not
generated for Dart, because a thin client never parses them.

Regenerate from the repository root:

```
pwsh scripts/gen-proto.ps1
```
