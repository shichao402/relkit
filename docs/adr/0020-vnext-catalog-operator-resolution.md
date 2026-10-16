# ADR 0020: Catalog、Operator 与 Immutable Resolution

- 状态：Accepted（Catalog、Operator、resolution、wrapper 与离线 bundle 已实现）
- 日期：2026-10-16
- 依赖：[ADR 0019](0019-vnext-breaking-and-protocol-contracts.md)
- 关联：[ADR 0021](0021-vnext-artifact-runtime-registry.md)、[ADR 0022](0022-vnext-certification-ops-skill.md)、[ADR 0007](0007-entry-mirror-must-be-reachable-and-cacheable.md)、[ADR 0018](0018-offline-env-pack-in-ci.md)

## 背景

项目当前直接写 `release: latest`，没有目标服务器作用域。固定自建 Operator 与滚动托管服务需要不同选择策略；一次 CI 内重复解析会导致 install/build/publish 漂移。Catalog、Operator Profile 与 resolution 必须成为唯一版本选择真相。

## 决策

### 1. `relkit.project/1`

建议路径：仓库根 `relkit.project.json`。

包含：

- product、channels、版本策略、selector 维度
- 角色：publisher、updater-host、subscriber、updater-projection
- 逻辑 `operatorRef` + Distribution Root `trustFingerprint`
- artifact producer 与可选 version provider
- Runtime Profile 订阅（product ref + root fingerprint，不复制 URL/公钥列表）
- capability fingerprint：OS/arch、facade/projection、发布/drop 拓扑、subscriptions、Operator 类型与所需合同

不包含：

- Relkit version 或项目级 pin
- agent/backend/upload URL
- Token、SSH、机器目录、mirror 凭据、CI secret
- onboarding step 或 verified 状态

Schema 草案：[`../design/vnext/schemas/relkit.project.1.schema.json`](../design/vnext/schemas/relkit.project.1.schema.json)

### 2. `relkit.operator-profile/1`

Operator Profile 是服务器现场权威，由 relkit-ops 从实际部署生成并签名；项目不得手写期望版本冒充现场。

包含：

- `deploymentId`
- 实际 server release（精确 distribution）
- `selectionPolicy: exact | latest-compatible`
- `provides`：RUP Schema Set、Publish Protocol、发布事务能力
- endpoint、backend、service identity
- secret **引用**（不是明文）
- distribution sources、mirror、bundle
- SSH inventory、部署参数、CI adapter 引用
- 单调 `sequence` 与签名

定位：用户私有配置、CI 注入或 relkit-ops inventory。与 agent 的 per-product `products/*.json` **永久分离**；store operator token 也不是 Operator Profile。

#### selectionPolicy

| 策略 | 适用 | Resolver 行为 |
| --- | --- | --- |
| `exact` | 固定自建、版本钉死的服务器 | 项目必须使用 Profile 声明的精确 server distribution |
| `latest-compatible` | 愿意滚动且声明合同的托管/滚动服务 | 从 certified catalog 选最新且 `requires/provides` 相交的完整 distribution |

这里拒绝 v0.5 的反模式：各宿主仓分别保存一个 Relkit 版本，升级时才发现每个 pin 都已
依赖不同的宿主特判。共享同一 Operator 的项目必须共享该 Operator 的选择策略；升级风险
通过独立 candidate lane 持续暴露，不能靠长期分仓 pin 推迟到下一次人工升级。

无作用域裸 `latest` 不存在。运维命令 `relkit-ops upgrade <operator> --to latest` 中的 latest 仅表示“选最新 certified release 作为服务器升级目标”；升级完成后 Profile 仍记录精确 server version 与合同向量。

也不存在 `fleet-latest` 这类全局产品版本。所有版本选择都以目标 Operator 为作用域：

- 固定自建部署的 `exact` 是部署事实；连接该部署的项目得到 Profile 声明的同一**完整**
  distribution，包括 CLI、updater、facade 与服务组件；resolver 不自动抬升其中某个
  客户端组件。服务器升级时先原子签发新的 Profile，再重新 resolve
- `latest-compatible` 是 `latest-compatible(operator)`，不是“全世界最新”；它可以在
  Operator 公布的合同窗口内选择比当前 server release 更新的 certified **完整**
  distribution，但仍禁止逐组件混版
- 无论哪种策略，解析结果都立即冻结为 immutable resolution；`latest` 不能在流水线中再次求值

不存在额外的项目级 “emergency pin”。事故回退由 Operator 恢复旧服务/Profile，或把
部署策略切为 `exact`；运维 inventory 记录原因、责任人与复核时间。固定自建部署可以
长期保持 `exact`，临时事故 pin 则在复核后由 Operator 升级流程解除，产品仓始终不新增
Relkit 版本字段。

#### 发现与多环境

- `operatorRef` 解析为逻辑名 → relkit-ops / CI 注入的已签名 Profile 路径或 URL（带 trustFingerprint 校验）
- **一份 Profile = 一个 deployment**。dev/stable、内网/公网、多机若部署事实不同，则多份 Profile、多个 `deploymentId`
- project 只引用逻辑 Operator；具体 deployment 由 CI/环境选择注入，不把机器地址写入产品仓
- 签名 endpoint 是服务身份与默认路由的权威事实；受限网络中的 CI adapter
  可以建立本机隧道，但不得重写 Profile 或 resolution。写路径仅在
  `RELKIT_RELEASE_VIA_CI=1` 的 execute 模式接受独立的
  `RELKIT_AGENT_TRANSPORT_URL`，且目标必须是 loopback IP；它不复用旧控制面的
  `RELKIT_AGENT_URL`，因此不会把签名 endpoint 与兼容配置混为一谈，产品 Token 也不会被
  环境变量导向任意远端。发布后只读验证可显式传入 `--consumer` 与 `--directory` 的
  隧道路由，验证结果仍绑定原 resolution digest、deployment identity 与产品签名根

Schema 草案：[`../design/vnext/schemas/relkit.operator-profile.1.schema.json`](../design/vnext/schemas/relkit.operator-profile.1.schema.json)

### 3. 信任根

四个信任域：

| 根 | 签发对象 | 默认 |
| --- | --- | --- |
| **Distribution Root** | production-authority Catalog（含其生命周期状态）、production Operator Profile | 与 product RUP key **分离** |
| **Product RUP Root** | directory/index/manifest/fallback | 每产品独立 |
| **Runtime Profile Root** | runtime-profile | 默认复用对应 product 的 RUP root |
| **Candidate Root / Identity** | candidate-only Catalog/Profile/resolution bundle | 不能签 certified 或写 production 指针 |

闭合默认：

1. Catalog 与 Operator Profile **共用 Distribution Root**（同一运维信任域），但与各产品 RUP key 分离。
2. `relkitw` 内置 Distribution Root fingerprint；project 保存 `operatorRef + trustFingerprint`。
3. Catalog / Profile 带单调 sequence；旧 sequence 默认拒绝；显式灾备回滚必须留审计记录。
4. Root 轮换支持旧/新 key 重叠窗口。
5. 离线备份与轮换责任人：Relkit 运维（Distribution Root）；各产品发布负责人（Product RUP Root）。责任写入 ops inventory，不进产品仓。

### 4. Distribution Catalog

状态机：

```text
candidate → certified → superseded
                 ↘ revoked
```

规则：

- production resolver 只读 certified
- candidate 仅在显式 candidate lane 可见
- 无合同交集 hard fail，不自动回退更旧或未认证版本
- “最新”按 SemVer precedence；pre-release 不进 production latest
- certification sequence 用于同版本索引、防回滚与审计
- Catalog 是对 immutable GitHub Release / module / mirror 资产的**签名索引**，不替代内容地址

`certified` 是**某个已签名 Catalog 快照中的状态**，不是 distribution 脱离部署环境后的
全局属性。生产晋升以 `Catalog + Operator Profile + certification fleet/report` 共同形成
认证域：

- 公网与内网、不同 Operator 或不同发布责任边界可以持有不同 Catalog 快照
- 同一 immutable distribution 可以先在公网 Catalog certified，而在内网 Catalog 仍是
  candidate 或尚未出现
- 一个认证域的报告不得用于晋升另一个认证域；后者必须使用自己的 fleet、CI 路径和现场证据
- 产品仓只看目标 Operator 注入的签名快照，不能因为其他环境已 certified 就越域采用

#### 物理载体（闭合默认）

- 主载体：与 GitHub Release / module 发布同流水线产出的 **signed catalog index**（独立对象，内容寻址；可由 mirror 同步）
- relkit-ops inventory 可缓存副本，但不是信任 SSOT
- mirror 同步顺序：先同步 immutable assets 并验摘要 → 再原子切换 catalog/Operator 指针
- mirrors 若 PUT 不可覆盖：错误资产 **只发新版本**；禁止 delete/re-upload 覆盖同一内容地址。纠错 = 发 superseded/revoked + 新 certified 条目

browse 的 `catalog.json` 不在此信任链（ADR 0019）。

### 5. `relkit.resolution/1`

每次 CI 开始时，Resolver 根据 project、已签名 Operator Profile、certified catalog 或 candidate override、Runtime Profiles 生成一份 immutable resolution。

固定内容至少包括：

- project digest
- Operator Profile digest / sequence / deploymentId
- Catalog snapshot digest
- Runtime Profile digests
- selected distribution release/commit
- module H1、Release asset sha256
- 所有目标平台的 component rows
- production/candidate identity 与允许 channels
- 合同证明（RUP Schema Set、Publish Protocol、Updater IPC、ArtifactSet）

规则：

1. 默认不提交；同一次 install/build/drop/stage/release plan/execute/verify 只使用这一份
2. CI 多 Job 传递同一文件；build 中途不得重新读 catalog/Operator/profile
3. 多平台 drop **共用一个矩阵 resolution**
4. staged document 与 agent publish request 携带 resolution digest；agent 校验身份、Operator、channel、candidate/production 作用域
5. candidate resolution **禁止写 stable**
6. 离线复用时 Operator/Profile digest 不一致直接失败
7. extension 不能生成或修改 resolution
8. 定义 canonical JSON 与 digest 算法（字段排序、无无关空白、固定编码）；同一输入在不同平台/镜像得到相同 digest

Schema 草案：[`../design/vnext/schemas/relkit.resolution.1.schema.json`](../design/vnext/schemas/relkit.resolution.1.schema.json)

### 6. Canonical wrapper

删除宿主复制的 `scripts/relkit_cli.py`。宿主只保留 Relkit 生成、不可客制化的：

- `relkitw` / `relkitw.cmd`

wrapper 固定 Distribution Root fingerprint、Operator/Catalog 协议、project 文件位置。

首次 clone：

1. 读 project 的 Operator 引用
2. 取得已签名 Operator Profile
3. `exact` → 精确 CLI；`latest-compatible` → 解析完整 distribution
4. 优先 Operator 预置资产，否则公开 source
5. 缺资产、摘要或合同不匹配 → hard fail
6. 启动 CLI 并生成完整 resolution

#### 离线 / 无 Go（闭合默认）

- 有公网：wrapper 拉取所需 CLI（及后续 component）
- 仅 mirror：同一 Profile/catalog/assets，经 mirror
- 完全离线：**预置完整 distribution bundle**（签名格式与 catalog 条目一致），不只预置 CLI；bundle 由 relkit-ops 唯一供给路径签发，wrapper 验 Distribution Root 后展开

物化组件进入 `.relkit/components/`。Go facade 经临时 modfile/workspace overlay 接入。

### 7. Production lane 与 Candidate lane

- Production：`exact` 用部署 pin；`latest-compatible` 用最新合同相交的 certified distribution
- Candidate：每个 Relkit change 生成不可变 candidate；宿主 CI 注入 candidate Profile/Catalog override，不改生产选择
- 固定生产服务器也跑 candidate lane，因此 pin 不掩盖未来升级问题
- candidate 与 production 使用不同 namespace 与写权限；production job 默认拒绝 candidate override

## 否决

- 项目文件填写 Relkit version、URL、Token、SSH
- 未签名或可手改的“现场版本”
- build 中重复 resolve
- candidate 写 stable 或改 production certified 指针
- 把 browse `catalog.json` 当作 Distribution Catalog

## 后果

- Pin 属于部署事实（Operator），不属于项目偏好
- 一次 CI 版本固定且可审计
- 滚动服务持续暴露兼容问题，而不靠每仓私自 pin 掩盖

## 完成标准（本 ADR）

能唯一回答：

1. 固定自建为何 exact，滚动服务如何 latest-compatible？
2. 一次 CI 最终选了哪个完整 distribution？
3. 固定 pin 为何不掩盖 candidate 问题？
4. 无 Go/无公网首次 clone 如何启动正确 CLI？
5. Wrapper / Catalog / Operator / Runtime Profile / RUP keys 分别信任谁？
6. 同一 resolution 在不同平台/镜像为何 digest 相同？
