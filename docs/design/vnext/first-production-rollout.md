# vNext 首次生产推进记录（2026-10-16～17）

本文记录第一次把 vNext 从四宿主迁移验收推进到真实生产时得到的事实与设计结论。
它不替代 [ADR 0019–0022](README.md)，操作顺序以 [cutover.md](cutover.md) 为准。

## 范围

- 架构输入：Loom、Loom-Launcher、SvnMergeTool、Dec，用于抽象 S1–S3 与能力组合。
- 本次生产认证域：`firoyang-public` + Dec。
- 延期：Loom 系列与 SvnMergeTool 内网生产验证。延期不表示能力删除，也不由公网报告代验。
- 不做：为任何宿主增加仓库名特判、恢复旧 consume/host/onboarding 控制面。

## 经过

1. 在最初调查后重新同步 Relkit 与四个宿主，确认旧 checkout 会让 issue、CI 和接入结论失真。
2. 用能力和合同重写设计：控制面允许断代，RUP/Publish Protocol/Updater IPC 等现场协议继续兼容。
3. 四宿主迁移只作为 S1–S3 样本；长期改用独立能力、交互组合、匿名场景和 capability cohort。
4. 发布 v0.6.13 后，真实 control-plane bundle 暴露 Directory base URL 拼接缺陷；修复进入
   v0.6.14，并重新发布 distribution、升级 agent/store。
5. Dec candidate dev CI 完成测试、五平台 ArtifactSet、合并及远端提交；最终 consumer
   readback 一度失败。后续独立验证确认该版本已完整可见，因此没有重复发布同一版本。
6. 公网 Dec-only fleet 在干净 v0.6.14 Candidate commit 上通过 full certification。
7. v0.6.14 在公网 production Catalog 中晋升 certified，生成并验证 production resolution
   与完整 bundle；内网 Catalog 未随之晋升。
8. control-plane auxiliary Release 一度成为 GitHub Latest，可能污染下一次正式 release
   的 baseline；已改为 prerelease，并把正式基线筛选收紧为 `vX.Y.Z`。

## 设计结论

### 1. Relkit 面向能力，不面向仓库

宿主拥有产品版本、业务构建、制品内容、UI 和检查时机；Relkit 拥有 resolution、
签名、发布事务、协议和 updater 状态机；ops 拥有 Operator、凭据、机器和 CI adapter。
任何修复若需要在 core 判断仓库名，说明缺少能力合同或组合测试。

### 2. `latest` 必须有 Operator 作用域

固定自建服务使用 Operator `exact`；这是部署 pin，不是项目偏好。滚动服务使用
`latest-compatible(operator)`。项目不保存 Relkit 版本；每次 CI 只解析一次并冻结
resolution。独立 candidate lane 持续暴露下一次升级问题，避免 exact 把问题藏到未来。

### 3. 兼容成本落在少量协议合同

不维护 release 两两矩阵，不允许宿主任意混搭组件。默认选择完整 distribution，只维护：

- RUP Schema Set
- Publish Protocol
- Updater IPC
- ArtifactSet schema

控制面可以断代；已部署客户端依赖的数据和 IPC 合同不能因断代被顺手破坏。

### 4. 三个场景不是全部测试

S1–S3 证明完整链路；独立能力测试证明局部合同；组合测试覆盖共享状态、已知冲突和非法组合；
真实 canary 发现合同外环境依赖。新增能力必须同步新增合同、独立测试、受影响组合、
至少一个端到端和临时首宿主 canary。

### 5. `fleet` 是认证登记，不是所有用户名单

fleet 属于一个 Catalog/Operator 认证域，登记匿名场景、固定 canary、临时首用者和
required CI。横切变更全量执行该域的 mandatory 登记，不把未来上百个用户仓库永久耦合
进中央门禁。公网与内网分别认证，结论不可跨域复用。

### 6. 发布错误不等于“什么都没写”

release 的写入与 consumer readback 是两个阶段。最终读回失败时必须先按同一
resolution/version/channel 验证真实可见状态，再决定重试或 fix-forward。盲目重发会把
一次可恢复的观察延迟变成 immutable version 冲突。

### 7. Root 必须按职责显式传递

Candidate Root 授权候选 resolution；Distribution Root 信任 Catalog/Profile；
Product RUP Root 验证产品数据。旧环境变量或 bootstrap mapping 可能比远端 Agent 更早
制造“不匹配”，因此诊断必须显示实际输入和 provenance，不能只输出模糊的 policy 错误。

### 8. Skill 应降低理解成本，不应复制复杂度

用户只需从产品和接入产品角度验收。Skill 先从结构化命令读取事实，只询问真实业务选择，
不把信任根、fleet、schema 等内部默认逐项交给用户拍板；同时也不保存第二套门禁账本。

## 首次推进暴露的通用缺口

| 现象 | 最早应拦截的位置 | 持久处理 |
| --- | --- | --- |
| Base URL 末尾斜杠改变 Directory 地址 | Operator/URL 合同测试 | 统一 URL join；覆盖 root、path prefix、尾斜杠 |
| CI 最终读回失败但版本稍后可见 | release commit/verify 边界 | [issue #52](https://github.com/shichao402/relkit/issues/52)：结构化 commit receipt、稳定 code、有界 readback |
| 本机旧 root 造成误导性 policy 错误 | inspect/doctor | 输出 resolution authority、实际 root 来源与 Product RUP mapping |
| 内网噪声阻塞公网推进 | certification domain 选择 | Catalog/fleet/report 按 Operator 域隔离，延期不越域 |
| auxiliary Release 抢占正式 baseline | tag release gate | auxiliary 标 prerelease；基线只接受 `vX.Y.Z` |
| 复杂问题全部抛给使用者 | Skill questions | 先取证并采用安全默认，只展示可观察结果与回滚 |

## 状态

截至本文首次提交，公网 production Catalog 与 bundle 已就绪；Agent production policy、
Dec stable 发布、真实更新/回滚仍需按 [使用者验收](USER-ACCEPTANCE.md) 闭合。完成后只更新
真实证据，不改写上述设计边界。
