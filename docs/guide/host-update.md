# 宿主接入

产品仓接入合同（两轨 `--install` / `--payload`、`InstallSpec.placement`、IPC、最小步骤）的 SSOT 是运维 skill：

[`DecAssets/skills/relkit-ops/host-update.md`](../../DecAssets/skills/relkit-ops/host-update.md)

产品仓经 Dec 拉取 `relkit-ops` 后，同文件在 skill 目录里。不要在本文件或产品仓 docs 再复制一份。

协议实现（payload 结构、基线差分、引擎 Placement）仍以 [ADR 0013](../adr/0013-internal-update-payload.md)、[ADR 0014](../adr/0014-placement-and-version-library.md)、[`SPEC.md`](../../SPEC.md) §6.2 / §11 / §12.5 / 附录 B、[`CLI.md`](../../CLI.md) `relkit stage` 为准。旧 Layout 背景见 [`docs/design/install-layouts.md`](../design/install-layouts.md)。
