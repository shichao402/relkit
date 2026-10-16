# ADR 0021: ArtifactSet、Runtime Profile 与 Component Registry

- 状态：Accepted（ArtifactSet、Runtime Profile、registry 与 facade 物化已实现）
- 日期：2026-10-16
- 依赖：[ADR 0019](0019-vnext-breaking-and-protocol-contracts.md)、[ADR 0020](0020-vnext-catalog-operator-resolution.md)
- 取代：[ADR 0012](0012-single-component-registry.md) 的 Python `facets.py` 载体与宿主特判式门禁写法
- 关联：[ADR 0010](0010-single-updater-engine.md)、[ADR 0013](0013-internal-update-payload.md)、[ADR 0014](0014-placement-and-version-library.md)、[ADR 0022](0022-vnext-certification-ops-skill.md)

## 背景

发布制品声明、跨仓订阅配置、组件注册表目前分散在 release-artifacts schema、手写 URL/公钥、Python facets 与 Go registry 之间。Loom→Launcher 依赖手抄 `EDITOR_*`；Dec 维护多份配置副本。vNext 需要单一 ArtifactSet 合同、受信 Runtime Profile，以及声明式 component registry。

## 决策

### 1. `relkit.artifact-set/1`

vNext 不读取旧 `/1`、`/2`，保留有效语义：

- 文件、kind、platform、selectors
- install / payload 关系
- 产品生成的单调 `releaseCode`（Operator 不得从 tag 或仓库规则二次猜测）
- changelog、展示元数据
- multi-platform drop merge identity

规则：

1. core 把 resolution digest、product version、channel、target、输出目录传给 producer
2. producer **只返回候选制品**；不得写 staged / index / manifest、签名或 backend
3. merge 拒绝 resolution、版本、releaseCode、selector、payload 关系不一致的 drop
4. `stage → simulate → fake → publish` 使用同一 channel 与同一 resolution
5. 输出必须符合 resolution 声明的 RUP Schema Set
6. 平台不对称合法：例如 Windows 有 installer+payload，macOS 仅 installer；不得强迫全平台同构
7. 产品元数据（如版本树所需 JSON）由宿主 producer 声明字段，**不进入 core 特判**

Schema 草案：[`../design/vnext/schemas/relkit.artifact-set.1.schema.json`](../design/vnext/schemas/relkit.artifact-set.1.schema.json)

### 2. `relkit.runtime-profile/1`

发布侧 core 根据 project 与签名 directory 生成 Runtime Profile：

- product、允许 channels、directory bootstrap、recovery
- RUP Schema Set
- profile version、摘要与发布者签名

#### 信任与交付（闭合默认）

1. Profile **不能**用自己携带的 key 自证
2. publisher project 定义 Product RUP Root；Runtime Profile Root 默认复用该 root
3. subscriber project 只提交 `productRef + rootFingerprint`，不复制 URL、公钥列表或 recovery
4. resolver 验证 profile 并把 digest/contract 放入 resolution
5. **物理交付**：作为与 directory 同事务发布的 **RUP 签名对象**（内容寻址路径由 directory/index 指向）；不是 distribution catalog 条目，也不是手抄静态文件
6. **发布顺序**：被订阅产品的 Runtime Profile 必须先 publish 且 `verify released` 通过；依赖方（如 Launcher）本窗口才可进入 publish。失败则依赖方硬失败，不允许“先发后补”
7. **freshness**：subscriber resolution 记录所消费 profile digest；离线复用时 digest 不一致则失败
8. **首次自订阅自举**：首个 vNext release 尚不存在可供自身 resolution 消费的 Profile 时，由 `relkit-ops runtime-profile publish` 使用 product-scoped token 请求 agent 在签名机上签发并原子更新 directory；私钥不离开 agent，Operator 只取得公开 Profile。首发之后仍由正常 release 事务更新。
8. root 轮换必须显式跨仓变更（subscriber 更新 fingerprint）

Loom 发布 `loom` profile；Launcher 订阅 `loom-launcher` 与 `loom`，禁止手写另一仓 URL/公钥。

Schema 草案：[`../design/vnext/schemas/relkit.runtime-profile.1.schema.json`](../design/vnext/schemas/relkit.runtime-profile.1.schema.json)

### 3. Projection 与 Facade

每门语言区分：

- **updater projection**：官方 DTO/protobuf，供 UI 解析事件（例：Loom TS）
- **updater facade**：`open/check/download/apply/status` 等正式宿主调用面

规则：

- 仅 projection 不能声明 updater-host
- Node 只有存在正式 `sdk-node` facade 时才能声明 updater-host
- Rust / Dart / Go / Node 每个 facade 声明 Updater IPC `requires/provides` 并通过行为 conformance
- legacy `sidecar.packScript` 不得让迁移器误判“仅 projection 产品”为 updater host

### 4. 声明式 Component Registry

vNext 只有一份声明式 registry；Go 代码、schema、打包与文档由它生成或验证。

- 取代 Python `facets.py` 与多份平行数组
- 每个 component 行声明 role、build、install、transport（module vs Release asset）、合同向量、目标平台
- Go 宿主通过 resolver 的临时 overlay 消费 facade；不把历史 module 版本留作项目长期 pin
- 物化路径命名（脱离 `third_party` 语义）可在本 ADR 实现窗口搭车，但不得引入双路径长期共存

### 5. `relkit.extension/1`（产品面）

产品面只允许：

- artifact producer
- 可选 version provider

统一约束：argv 启动、版本化 JSON stdin/stdout、stderr 日志、超时、工作目录、环境变量白名单、声明输入输出与权限。

禁止：任意 pre/post hook、进程内插件、extension 修改 resolution / staged / index / 签名数据。

运维面 extension（mirror/CI/deploy）见 ADR 0022。

Schema 草案：[`../design/vnext/schemas/relkit.extension.1.schema.json`](../design/vnext/schemas/relkit.extension.1.schema.json)

### 6. 能力维度（禁止仓库身份）

core 与 registry **不得**包含 Loom、Launcher、SvnMergeTool、Dec 等仓库名分支。差异只能来自声明的能力维度：

- 关系：自更新 / 管理器更新其他产品
- 订阅：单产品 / 多产品
- 平台：单平台 / 多平台合并
- 应用方式：installer-only、原地替换、版本库存与切换
- 制品选择：platform / component / audience
- 语言接入：projection / facade

代表性场景映射见 [`../design/vnext/representative-scenarios.md`](../design/vnext/representative-scenarios.md)。

## 否决

- 宿主手抄被订阅产品的 URL/公钥作为正式配置
- producer 写签名或 backend
- core 按仓库名特判元数据或布局
- projection 冒充 facade / updater-host
- 双份 registry（Python + Go）长期并存

## 后果

- 跨仓订阅变为受信 profile 管道
- 制品与语言面差异靠声明与 conformance，不靠隐藏分支
- 新宿主组合已有能力即可接入，无需改 core

## 完成标准（本 ADR）

能唯一回答：

1. Launcher 为何信任 Loom profile 又无需复制 URL/公钥？
2. 为何新宿主只需组合已有能力、不给 core 加仓库特判？
3. ArtifactSet 如何允许平台不对称且仍同一 resolution？
