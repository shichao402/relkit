# Client SDKs

This directory holds **language SDKs** for consuming RUP updates.

**Canonical updater path (ADR 0010):** spawn `relkit-updater` through the generated facade.

| Language | Facade | Sidecar IPC |
|----------|--------|-------------|
| Go | [`updaterfacade`](updaterfacade/) | `relkit.updater.v1` |
| Dart | [`dart/lib/src/updater_facade.dart`](dart/lib/src/updater_facade.dart) | same |
| Node | [`node/src/updater_facade.ts`](node/src/updater_facade.ts) | same |

Legacy `RupUpdater` / `sdk.Updater` implementations are **frozen** and will be removed after host migration. Do not add features there.

Signatures: [`../conformance/updater/facade-signatures.txt`](../conformance/updater/facade-signatures.txt)

Open:

```
opened = Updater.open(profile, runtime)  // failed -> close update UI
u.check(force: userInitiated)
u.download(planId)
u.apply(planId)  // if requiresHostExit, exit
```

IPC window is a compile-time constant on the facade (`ipcMin`/`ipcMax` = 1), not a product config field.
