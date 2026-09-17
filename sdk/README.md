# Client SDKs

This directory holds **language SDKs** for consuming RUP updates.

**Canonical updater path (ADR 0010):** spawn `relkit-updater` through the generated facade.

| Language | Facade | Sidecar IPC |
|----------|--------|-------------|
| Go | [`updaterfacade`](updaterfacade/) | `relkit.updater.v1` |
| Dart | [`dart/lib/src/updater_facade.dart`](dart/lib/src/updater_facade.dart) | same |
| Node | [`node/src/updater_facade.ts`](node/src/updater_facade.ts) | same |

Legacy in-process `RupUpdater` / `internal/inprocess.Updater` implementations are
**engine-internal**. They are not host APIs and are not packed into consume zips.

Signatures: [`../conformance/updater/facade-signatures.txt`](../conformance/updater/facade-signatures.txt)

Open:

```
opened = Updater.open(profile, runtime)  // failed -> close update UI
u.check(force: userInitiated)
u.download(planId)
u.apply(planId)  // if requiresHostExit, exit
```

IPC window is a compile-time constant on the facade (`ipcMin`/`ipcMax` = 3), not a product config field.
