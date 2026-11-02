# 宿主接入

产品仓接入合同（两轨 `--install` / `--payload`、`InstallSpec.placement`、IPC、最小步骤）的 SSOT 是运维 skill `relkit-ops` 里的 `host-update.md`。

该 skill 只存在于 Dec 云端 `relkit` bundle，装好 Dec 后落在各宿主的用户级 skill 目录（如 `~/.claude/skills/relkit-ops/host-update.md`）。要修改就改云端，不要在本文件、本仓或产品仓 docs 再复制一份。

层对照见仓库 [`README.md`](../../README.md)：`relkit.version/1`（项目版本文件）· `rup.v2`（线格式）· `relkit.updater.v1`（本机 IPC）。

协议实现（payload 结构、基线差分、引擎 Placement）仍以 [ADR 0013](../adr/0013-internal-update-payload.md)、[ADR 0014](../adr/0014-placement-and-version-library.md)、[`SPEC.md`](../../SPEC.md) §6.2 / §11 / §12.5 / 附录 B、[`CLI.md`](../../CLI.md) `relkit stage` 为准。旧 Layout 背景见 [`docs/design/install-layouts.md`](../design/install-layouts.md)。
