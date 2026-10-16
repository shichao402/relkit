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
9. Dec `dev/v1.13.137` 已完成远端提交，但 CI 的最终读回暴露：离线 bundle 只把 Product
   Root 放在 Runtime Profile mapping，而 `verify released` 只查可选 `productRoots`。
   已提交的 tag 没有移动或重跑，修复按 fix-forward 进入 Relkit v0.6.15 与 Dec v1.13.138。
10. v0.6.15 新增真实 bundle install → bootstrap → released-state 回归；公网 Dec-only
    fleet 在干净候选 commit 上重新 full certification，并将同一 distribution 晋升 certified。
11. 公网 agent/store 原子升级到 v0.6.15，既有 Dec production policy 保持不变；
    v1.13.138 的 dev/stable 流水线均从同一 production resolution 安装工具链。
12. 流水线成功不等于客户端更新成功。最终三组件 `check → download → apply` smoke 在
    updater 打开阶段发现：实际生成的 `relkit.runtime-bootstrap/1` 把
    `trustedKeys[].key` 编码成数字数组，而正式 Schema 与公开 Go/Rust SDK 要求
    canonical base64 字符串。v1.13.138 tag 与已发布记录保持不动，不能算最终生产验收通过。
13. 既有门禁分别验证了控制面内部模型和手写 SDK fixture，却没有把**实际生成的 JSON**
    送入公开 SDK。这是测试边界分裂，不是某个宿主的特殊问题。
14. 修复按 immutable fix-forward 进入 Relkit v0.6.16 与 Dec v1.13.139：生成器改用
    canonical base64，控制面在验证时 decode 后检查长度、fingerprint 与签名，并新增
    `TestGeneratedRuntimeBootstrapMatchesPublicSDK`。同一 Candidate commit 重新执行
    production escape audit、公网 Dec-only full certification、Catalog 晋升、production
    resolution/bundle、服务升级、dev/stable 发布与真实三组件更新。

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

### 9. Product Root 必须从真实安装结果取证

`Runtime Profile Root` 默认复用对应的 `Product RUP Root`。离线 bundle 安装后，
这个公钥可能以 Runtime Profile mapping 的形式存在，而不一定同时复制为另一份
`productRoots` mapping。`verify released` 必须优先使用显式 Product Root；publisher
声明复用且显式 mapping 缺失时，才回退到同产品 Runtime Profile roots，并继续按
`runtimeProfilePublication.rootFingerprint` 校验。

门禁不能只在测试中直接传 `--product-root`，否则会绕过 bundle install → bootstrap →
wrapper → released-state verify 的真实链路。至少保留一条不带显式 root override 的场景回归。

### 10. 生成物才是跨组件合同的最终证据

控制面类型能 marshal、SDK 能解析手写 fixture，两项同时通过仍不能证明互操作。
凡是公开给 wrapper、SDK、updater 或宿主 CI 的 JSON/Protobuf，都必须保留一条
“正式生成器 → 落盘字节 → 公开消费者”的 conformance。Schema 中表示字节的字段必须明确
规定 canonical 编码；不能依赖 Go 默认 JSON 编码、内部类型或另一份手写样例恰好一致。

发布流水线绿只能证明 ArtifactSet 与发布事务完成；最终生产门禁仍须从签发的 Runtime
Bootstrap 启动真实 updater，至少执行一次 `check → download → apply → --version`。

## 首次推进暴露的通用缺口

| 现象 | 最早应拦截的位置 | 持久处理 |
| --- | --- | --- |
| Base URL 末尾斜杠改变 Directory 地址 | Operator/URL 合同测试 | 统一 URL join；覆盖 root、path prefix、尾斜杠 |
| CI 最终读回失败但版本稍后可见 | release commit/verify 边界 | [issue #52](https://github.com/shichao402/relkit/issues/52)：结构化 commit receipt、稳定 code、有界 readback |
| 本机旧 root 造成误导性 policy 错误 | inspect/doctor | 输出 resolution authority、实际 root 来源与 Product RUP mapping |
| 内网噪声阻塞公网推进 | certification domain 选择 | Catalog/fleet/report 按 Operator 域隔离，延期不越域 |
| auxiliary Release 抢占正式 baseline | tag release gate | auxiliary 标 prerelease；基线只接受 `vX.Y.Z` |
| macOS Bash 3.2 在 `set -u` 下展开空可选参数数组失败 | 宿主 CI adapter 的跨平台门禁 | 构造始终非空的完整 argv；Candidate Root 存在/缺失两条路径都在实际 Linux/macOS shell 执行 |
| 测试直接传 Product Root，掩盖离线 bundle 的真实 bootstrap mapping | released-state 场景门禁 | 安装真实 bundle 后不带 root override 验证；显式 Product Root 优先，publisher 才允许复用同产品 Runtime Profile Root |
| 生成的 Runtime Bootstrap key 是数字数组，但公开 SDK 只接受 base64 | 生成器→公开 SDK conformance | Schema 固定 canonical base64；实际生成 JSON 必须被公开 Go SDK 解析并完成签名/fingerprint 校验 |
| dev/stable CI 全绿，却未执行真实 updater apply | 最终产品验收 | CI 后从签发 bootstrap 对所有 runtime component 执行 `check → download → apply → --version` |
| 已推送的 dev tag 失败后想原地补跑 | immutable release 输入 | 不移动、不覆写旧 tag；修复后提升产品版本并创建新 tag |
| 提供方仓 `dec pull` 读到作者 home/vault 旧 Skill，误判 registry 发布失败 | Dec 资产发布读回 | 比对 registry commit/tree 与 `DecAssets` blob；用独立 consumer 验 ref，不提交 self-pull 回退 |
| 复杂问题全部抛给使用者 | Skill questions | 先取证并采用安全默认，只展示可观察结果与回滚 |

## 状态

公网 `firoyang-public` + Dec 认证域已以 Relkit v0.6.16 / Dec v1.13.139 完成
production Catalog、Operator、bundle、服务二进制、dev/stable 发布和三个 runtime
component 的真实更新验收；证据见 [使用者验收](USER-ACCEPTANCE.md)。v0.6.15 /
v1.13.138 保留为不可变逃逸证据，不是最终通过版本。Loom 系列与 SvnMergeTool 的内网
生产验证仍延期，且不由本次公网结论代验。
