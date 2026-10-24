# ADR 0019: vNext 断代边界与协议合同

- 状态：**Superseded**（2026-10-22 由 [ADR 0023](0023-desktop-updater-epoch.md) 取代；逐条驳斥与保留项见 [ADR 0019-0022 驳斥](0019-0022-refutation.md)，现行架构见 [`docs/architecture.md`](../architecture.md)）
- 历史状态：Accepted（vNext core、发布事务与 conformance 已实现；§1 控制面删除已于 2026-10-21 在 relkit 仓内执行完毕）
- 日期：2026-10-16
- 取代：[ADR 0017](0017-cli-owns-consume-source-channel.md) 的 consume/host/onboarding 控制面（决策 2、4 中产品仓入口与 lock `relkit.consume/3` follow-latest 语义）；不取代其 Go module + sumdb 传输经验
- 修订：[ADR 0011](0011-immutable-release-consume.md) 的 lock/consume 形态；[ADR 0012](0012-single-component-registry.md) 的 Python `facets.py` 载体（声明式 registry 归属 ADR 0021）
- 关联：[ADR 0003](0003-protobuf-v2-wire-format.md)、[ADR 0005](0005-signed-bootstrap-directory.md)、[ADR 0009](0009-publisher-protocol-negotiation.md)、[ADR 0010](0010-single-updater-engine.md)、[ADR 0020](0020-vnext-catalog-operator-resolution.md)、[ADR 0021](0021-vnext-artifact-runtime-registry.md)、[ADR 0022](0022-vnext-certification-ops-skill.md)

> **Superseded 说明（2026-10-22）**：本 ADR 的协议合同框架仍然有效（§2 四类合同、§5 发布事务、§8 命名隔离、§7 module 传输经验），但签名与信任模型、控制面形态、兼容窗口策略已被 [ADR 0023](0023-desktop-updater-epoch.md) 取代。本文件保留为历史记录，不作为现行设计依据；现行设计见 [`docs/architecture.md`](../architecture.md)，逐条差异见 [驳斥文档](0019-0022-refutation.md)。
- 取代：[ADR 0017](0017-cli-owns-consume-source-channel.md) 的 consume/host/onboarding 控制面（决策 2、4 中产品仓入口与 lock `relkit.consume/3` follow-latest 语义）；不取代其 Go module + sumdb 传输经验
- 修订：[ADR 0011](0011-immutable-release-consume.md) 的 lock/consume 形态；[ADR 0012](0012-single-component-registry.md) 的 Python `facets.py` 载体（声明式 registry 归属 ADR 0021）
- 关联：[ADR 0003](0003-protobuf-v2-wire-format.md)、[ADR 0005](0005-signed-bootstrap-directory.md)、[ADR 0009](0009-publisher-protocol-negotiation.md)、[ADR 0010](0010-single-updater-engine.md)、[ADR 0020](0020-vnext-catalog-operator-resolution.md)、[ADR 0021](0021-vnext-artifact-runtime-registry.md)、[ADR 0022](0022-vnext-certification-ops-skill.md)

## 背景

v0.5 控制面同时存在 consume lock、`scripts/host`、Python gates、Go releasegate、Skill/onboarding 多份真相。四宿主写 `release: latest`，却没有目标 Operator 作用域与单次 immutable resolution。修一个宿主会被动改变其他宿主，根因是控制面耦合具体项目，而不是协议合同缺失。

跨 Relkit release 兼容已经是现场事实：RUP wire、Publish Protocol 窗口、Updater IPC 窗口、旧 field updater 读取新 directory/index。这些隐含假设必须正式化。本 ADR 定义断代边界与四类协议合同；Catalog/Operator/Resolution 见 ADR 0020，ArtifactSet/Runtime Profile/Registry 见 ADR 0021，认证与 ops 见 ADR 0022。

## 决策

### 1. 控制面直接断代

vNext 不提供兼容 reader，协调迁移窗口一次性删除：

- `relkit.consume/1`、`/2`、`/3`
- 旧 `relkit.json` 作为 Relkit 控制面入口
- `.relkit/onboarding.json` 与“步骤全绿”门禁
- 产品仓物化的 `scripts/host`
- Python `install` / `upgrade` / `status` / `ci release` 产品面
- `release-artifacts/1`、`/2`
- Python `facets.py` registry
- 宿主复制的 `scripts/relkit_cli.py`
- 以源码 grep 充当 facade/channel 证明的门禁

旧 major（v0.5 及更早控制面）冻结：只接受安全与正确性最小修复，不接受新控制面特性。

### 2. 必须稳定的是协议合同，不是文件与命令

保留并正式化四类独立合同。不做 Relkit release 两两兼容矩阵，不做“v1.2 CLI 兼容 v1.5 server”清单，不默认混用不同 release 的组件。

| 合同 | Peer | 现有锚点 | 兼容规则 |
| --- | --- | --- | --- |
| **RUP Schema Set** | updater/client ↔ directory/index/manifest/artifact | `proto/rup/v2`、签名与行为 conformance | 冻结 schema set + 旧 SDK 行为 conformance；**不是** consume lock 里的 `protocol` 整数 |
| **Publish Protocol** | CLI/publisher ↔ agent/store | `internal/publishproto`，当前选择 `3`、兼容窗口 `[2,3]` | inclusive `[min,max]` 交集；无交集 hard fail（426 语义保留） |
| **Updater IPC** | language facade ↔ updater sidecar | `proto/updater/v1`，现网窗口已演进到 lock 记录的 `[3,3]` | inclusive `[min,max]`；无交集 `updaterTooOld` / `updaterTooNew` |
| **ArtifactSet schema** | artifact producer ↔ release core | 发布侧制品声明 | 版本化 schema；producer 只返回候选制品，不写 staged/index/签名 |

### 3. `requires` / `provides` 合同向量

每个 component 与完整 distribution 声明：

- `provides`：实现的合同版本或 capability
- `requires`：对端必须具备的合同版本或 capability

每条 requirement 必须带明确 peer/domain，例如：

- `cli.requires.publishProtocol`
- `facade.requires.updaterIpc`
- `updater.requires.rupSchemaSet`
- `publisher.requires.artifactSet`

禁止无作用域 capability。Resolver（ADR 0020）只检查合同集合相交与 distribution 完整性，不比较 Relkit tag 字符串。

### 4. 默认选择完整 distribution

一个 Relkit release 发布完整 distribution：CLI、agent/store/console、updater、各语言 facade、TS projection、schema 与 component registry。

默认 resolver 选择一个完整 distribution。若未来某组件需独立发布，必须成为拥有自己版本、合同与 conformance 的正式发行单元；宿主不得任意混搭。

### 5. 发布事务：关键路径与非关键投影

**关键事务**（失败则 publish 失败）：

- artifact / manifest 可读
- index 提交
- directory 可读且跨 backend 字节一致
- runtime-profile（若本 release 产出）可读且一致

**非关键投影**（可异步重建，失败不得伪装成核心发布成功）：

- browse site、HTML、latest 下载页、`catalog.json`（browse 用，见决策 8）

硬规则：

1. directory / runtime-profile 失败时，最终结果不得报告成功。
2. index 与 directory 验证所有目标 backend 的旧文档字节一致；禁止只取最大 sequence。
3. 正常发布禁用 `allow-partial`；部分 backend 已提交时返回明确 `partial-commit` 并进入 fix-forward/repair。
4. `relkit verify released`（或等价）必须验证 index、directory、manifest、resolution digest 与 sequence 单调。
5. RUP 发布端支持多 signer；轮换流程为：客户端信任新 key → 双签窗口 → 切主 key → 退旧 key。

### 6. 旧 field 客户端与冻结风险边界

- 服务升级后，已安装且仍在声明 RUP Schema Set 兼容窗口内的旧 updater，必须能读取新的 directory/index/manifest。
- 本 ADR **明确接受** vNext 首轮不对 `expiresAt` / 冻结攻击做完整缓解；残留风险写入发布说明，后续单独 ADR。
- 防回滚仍依赖既有 sequence / 水位规则（ADR 0005 / 0010）。

### 7. 与 ADR 0017 / module 通道的关系

**废止** 0017 控制面：consume/3、follow-latest lock、产品仓 host scripts/onboarding、Python/Go 双入口。

**保留** 0017 transport：

- Go module + sumdb 可作为 CLI / updater / Go facade 资产来源
- Dart / Rust / TS 可继续使用 Release assets
- 两种 transport 均由同一 selected distribution manifest 与 `relkit.resolution/1` 固定
- module 是传输与完整性机制，不是独立版本 SSOT
- Go 宿主使用 resolver 生成的临时 modfile / workspace overlay；项目 `go.mod` 不长期 pin 另一 Relkit 版本

### 8. 命名隔离

browse 站点的 `catalog.json` **永不**进入信任链，也不得与 Distribution Catalog 混用同一名称或同一解析路径。

### 9. v0.5 严重事务债

在 vNext 切换前，v0.5 **允许且应当**做不改变控制面形状的最小正确性修复：

- directory 失败仍返回 `ok: true`
- 多 backend directory 只取最大 sequence、不比字节
- 缺少 `verify released` 收尾门

多 signer / LoadSigners 单钥限制归 vNext 实现，不在 v0.5 大改发布端密钥模型。迁移前风险必须在发布说明与 ops 清单中显式列出。

## 否决

- Relkit release 两两兼容矩阵或任意组件混搭
- 为单宿主保留旧 consume/host reader 作为长期备用真相
- 把 consume lock 的 `protocol` 误当作 RUP Schema Set
- 无作用域 `latest` 或项目级任意 pin Relkit 版本
- 关键发布失败仍报告成功
- 在 core 中按 Loom / Launcher / SvnMergeTool / Dec 仓库身份分支

## 后果

- 宿主协调切换到 `relkit.project/1` + Operator Profile + resolution（ADR 0020）
- 协议演进成本落在合同版本与 conformance，不落在 release 数量平方
- 旧 field 客户端在声明窗口内继续可用；控制面命令与文件一次性作废
- 实现顺序：本 ADR 与 0020–0022 全部 Accepted 后再改代码与宿主，不先修单仓倒推

## 完成标准（本 ADR）

能唯一回答：

1. 哪些控制面删除、哪些协议保留？
2. Resolver 为何不需要 release pair matrix？
3. directory/runtime-profile 失败为何不能报告发布成功？
4. 服务升级后旧 field 客户端为何仍能读 RUP？
