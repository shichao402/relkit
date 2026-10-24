# ADR 0022: 认证模型、relkit-ops 与 Skill

- 状态：**Superseded**（2026-10-22 由 [ADR 0023](0023-desktop-updater-epoch.md) 取代；逐条驳斥与保留项见 [ADR 0019-0022 驳斥](0019-0022-refutation.md)，现行架构见 [`docs/architecture.md`](../architecture.md)）
- 历史状态：Accepted（分层认证、relkit-ops 与薄 Skill 已实现）
- 日期：2026-10-16
- 依赖：[ADR 0019](0019-vnext-breaking-and-protocol-contracts.md)、[ADR 0020](0020-vnext-catalog-operator-resolution.md)、[ADR 0021](0021-vnext-artifact-runtime-registry.md)
- 关联：原使用者验收入口 `../design/vnext/USER-ACCEPTANCE.md` 已随该目录删除；现行设计见 [`docs/architecture.md`](../architecture.md)

> **Superseded 说明（2026-10-22）**：certification / impact / fleet / production escape 全套体系删除，`relkit-ops` 收敛为 5 个服务器运维子命令，Catalog 与 Profile 由服务器在部署与发布时自动签发（不再有 `certify`/`operator`/`catalog` 人工入口）。合同 conformance（§6.1）与结构化诊断 envelope（§7）继续有效。Token 模型改为"只表示能操作哪些产品"，签发/轮换/吊销走管理面板，见 [`docs/architecture.md`](../architecture.md) §三。本文件对 `docs/design/vnext/` 的链接已失效，仅作历史参考。

## 背景

当前 Python gates、Go releasegate、Skill、onboarding note 多份真相；relkit-ops Skill 曾承载 schema 与复杂门禁。上百宿主不可能每次全量真实仓验证。需要：合同 conformance、能力 cohort、代表性场景、反向 candidate CI，以及独立于产品 core 的 ops 工具。

## 决策

### 1. 单一 Go core 与职责切分

| 面 | 职责 | 不负责 |
| --- | --- | --- |
| **relkit core** | resolve、integrity、发布图、签名、协议、updater 状态机、正式 facade | 机器 SSH、Token 明文、mirror 凭据、CI secret、仓库名单 |
| **relkit-ops** | Operator Profile、资产供给、远端部署、Token、服务健康、CI adapter、fleet 认证登记 | 产品业务构建、UI、制品内容 |
| **relkit-deploy** | Relkit 自身 agent/store/console 二进制部署 | 产品到 Operator 的日常运维 |
| **Skill** | 生成声明、解释诊断、执行 plan 的薄编排 | 第二份 schema、门禁实现、“已完成步骤”账本 |

### 2. relkit-ops 交付（闭合默认）

1. `relkit-ops` 是**有版本的可执行工具**，随 Relkit distribution 一起发布
2. Dec Skill **只分发 Skill 说明与编排**；同时可索引 ops binary 的获取方式（来自同一 distribution），但不在 Skill 内嵌第二份逻辑
3. Skill 唯一 SSOT：`DecAssets/skills/relkit-ops`；仓内副本只允许 `dec pull` 生成并校验 digest
4. 删除 onboarding 账本；替代命令：`relkit inspect`、`relkit doctor --resolution`、`relkit release verify --resolution`、`relkit-ops plan/status`（均 `--json`）
5. `DecAssets` 变更必须提升 `VERSION.json` build 并由 `publish-provides` 产生不可变 registry ref；
   发布验收比较 registry commit/tree 中的 Skill blob 与源码 blob，不只看 workflow 退出码
6. 提供方仓若同时是 Dec `Home` 项目，`dec pull` 可能解析作者 vault/home 资产而非刚发布的
   registry ref，因此不能作为消费者读回证据。应使用声明该 ref 的独立 consumer，或直接
   校验 registry tree；若 self-pull 令生成副本回退，不得提交该差异

### 3. 运维 extension

允许：mirror/bundle producer、CI adapter、deploy/rotate adapter。约束同 ADR 0021 extension/1（argv + 版本化 JSON IO）。不得修改 resolution / staged / 签名数据。

### 4. 凭据与 Token rotate（闭合默认）

三类凭据：store operator credential、product-scoped upload token、CI secret/reference。产品仓不得存放明文。

Token rotate **完成条件**（缺一不可，强制 immediate restart/reload）：

1. mint
2. 更新 secret store
3. 校验服务用户可读
4. **restart/reload**
5. 健康与写权限验证
6. 吊销旧 token

任何一步失败 → 不把 Operator Profile 标为 active。不允许“已 rotate 但未 reload”算完成。

### 5. Operator 原子升级

```text
relkit-ops upgrade <operator> --to <certified-version|latest>
```

1. latest → 精确 certified release  
2. 对目标 Operator cohort 跑 candidate lane  
3. plan 展示服务、Token、mirror、合同变化  
4. 同步完整 immutable distribution  
5. 部署服务并验证 Profile `provides`  
6. 原子签发新 Operator Profile  
7. 失败 → 恢复旧服务/Profile，不暴露半升级  

### 6. 分层认证（闭合默认）

#### 6.1 合同 conformance

每个 RUP Schema Set、Publish Protocol、Updater IPC、ArtifactSet 版本一套规范测试。

#### 6.2 独立能力 + 组合测试

- 每项能力单独测输入/输出/错误/状态迁移
- 对共享状态的能力做组合测试（两两、已知冲突、非法组合）；不做无关笛卡尔积
- 触及发布事务/签名/resolution/updater 状态机的三项以上组合，由端到端场景覆盖
- 重点交互矩阵与保证边界见 [ADR 0019-0022 驳斥](0019-0022-refutation.md)（原场景文档已随 `docs/design/vnext/` 删除）

#### 6.3 三个匿名代表性场景（distribution 级长期 mandatory）

见 [ADR 0019-0022 驳斥](0019-0022-refutation.md)：场景文档已随 `docs/design/vnext/` 删除，三个场景本身仍作为协议层 conformance 的覆盖目标（`conformance/`）。

1. 独立应用自更新  
2. 管理器与被管理产品  
3. 可选择的产品套件  

完整 Relkit distribution 在进入任何生产认证域前必须通过可移植的 S1–S3；仓库名不得
写入场景合同。单个 Operator 认证域只登记其实际提供产品涉及的场景和真实 canary：
例如只承载 S3 产品的公网域不需要借用内网 S1/S2 仓库作为现场门禁，但不能跳过
distribution 级 S1/S2 conformance。

#### 6.4 认证域登记不是全世界宿主清单

- `fleet` 不是服务器集群，也不是 Relkit 已知的所有客户仓库；它是**一个认证域内**
  受版本控制的私有认证登记，列出 mandatory 场景/真实 canary、project 来源与
  `requiredCi`
- 登记载体是 `relkit.ops-fleet/1`：每项包含稳定 `id`、`active`、`projectPath`、
  `canary`（none/fixed/rotating）、`mandatoryCapabilities`、`requiredCi` 与对应
  `ciAdapters`；认证时重新计算 project digest 与整份 fleet digest，报告不能替换条目
- 认证域由待晋升的 Catalog、目标 Operator/发布责任边界和 fleet 共同确定；公网与内网
  不共享“已认证”结论
- 只组合现有能力的新宿主不会自动进入永久 fleet；真实首用者只在引入新能力时临时 mandatory
- 与该域产品相关的匿名场景可作为稳定 fleet 条目；真实仓用于发现未声明依赖和现场差异，
  不成为 core 合同身份
- candidate 生成 impact manifest  
- 按 project capability fingerprint 选 cohort  
- 每 cohort：固定 canary + 轮换样本  
- 首个使用**新能力**的真实宿主临时 mandatory canary；稳定后不永久绑定  

fleet 是某次认证域的**当前声明快照**，不是未来所有 Operator、CI 系统或用户仓的闭集。
扩展到新 Operator 或新 CI 路径时更新该域登记并重新认证，不修改 core。必须区分：

- 尚未纳入该认证域的环境可以延期，其 Catalog 保持未晋升
- 已在 fleet 的 active 条目若缺任一 `requiredCi`，结果必须是 `skipped-unverified`
  并阻止 certified，不能再解释为“本轮不测”

#### 6.5 反向 candidate CI

生产按真实 Operator；candidate lane 用 override；观察窗口内失败阻止 certified。

#### 6.6 风险分级与全量

| 变更类型 | 验证范围 | 观察窗口（默认） |
| --- | --- | --- |
| 文档/非运行时 | conformance 子集 | 无 |
| 单能力实现 | 该能力 + 声明的组合 | patch：1 个宿主 CI 周期 |
| 普通 contract bump | 受影响 cohort | minor：受影响 canary 全部绿 + 1 周期 |
| 横切（签名、resolution、发布事务、协议、registry、IPC） | **该认证域 fleet 的全部 mandatory 条目** | major：全部绿 + 延长窗口 |

- “全量”不等于未来上百个使用者仓库全部进入中央门禁；它表示当前认证域中正式登记的
  匿名场景、固定 canary 与本次临时 mandatory 首用者全部执行。更广宿主通过反向
  candidate CI 和周期抽样提供信号
- 内网蓝盾与外网 GHA：**凡被同一认证域声明为该 Operator/宿主的 required CI 路径，
  都必须通过**才可 certified；缺一不可把缺口标为 `skipped-unverified`
- 不在本次认证域中的环境可以延期，但结论必须写成“未在该环境晋升”，不能把公网通过
  表述成内网也通过；后续不得复用公网 report 越域签发内网 Catalog
- candidate override 凭据与生产隔离；每次注入留审计；超时/重试策略写在 ops，失败 fail closed
- 夜间/周期全量用于发现 impact 漏判

### 7. 结构化诊断

所有 core/ops 诊断统一 envelope：

```json
{
  "code": "release.directory_partial",
  "severity": "error",
  "stage": "publish",
  "message": "...",
  "evidence": {}
}
```

每个 gate 有稳定 code，禁止默认 `unclassified`。candidate 归因按 code。status/doctor 只读。journal/retrospect 不进产品发布门禁。

### 7.1 Production escape 必须成为可执行回归

旧 `host.py retrospect` 写入 gitignored `.relkit/cache/ops-journal.jsonl`，并通过
`DIGESTED_ISSUE_CODES` 判断“已消化”。这两者既不能随 Relkit release 传播，也不能让
另一个宿主在出错前失败，**不是 vNext 的继承或完成机制**，不得扩展为新账本。

vNext 对现有自动化未提前发现的问题执行以下闭环：

1. 先分类。可跨项目复现的 core/wrapper/ops 问题记为 `generic`，agent 控制面问题记为
   `agent`；产品业务专属问题留在宿主仓，不进入中央 registry。
2. 先增加真实可执行回归，再在 `conformance/production-escapes/*.json` 记录稳定 code、
   最早拦截点、tracking issue/PR（已知 fix PR 可直接充当 tracking）和测试 path/symbol。
   generic 横跨组件时必须有场景证据。
3. `relkit-ops escape audit --baseline-ref <target> --execute ...` 是 review gate：
   严格校验描述、tracking 与真实测试定义，拒绝 zero-match/skip；平台限定证据只在声明
   平台执行并在其他平台显式记为 deferred，由 Linux/Windows matrix 合并覆盖。baseline
   中已有 code 不得删除、改名或回退 closure，但允许本次 merge commit 与正式 release
   尚未知。登记 path 必须拥有实际执行的 symbol：Go 要求 package 内唯一声明就在该文件，
   Python 只收集方法源码也属于该文件的 `TestCase`，Node 事件的 file 必须等于登记文件；
   Rust 只接受直接 `tests/*.rs` 中无条件、顶层、唯一的 `#[test]`，并用独立 `--test`
   target 与 `--exact` 执行。Python 读取 `unittest.TestResult`，Node
   只接受 `kind=test` 的 reporter 事件；不得靠导入文件、同名 suite/测试或测试进程自行
   打印的“通过”文本判定成功。
4. `relkit.impact/2` 固定 Candidate commit，以及包含测试源码摘要、不含 closure 字段的
   coverage digest；`certify` 只接受同一干净 commit，重新审计并执行当前平台适用证据，
   digest 漂移即失败，再通过 `relkit.ops-certification-input/2` 传给宿主 CI，最终写入
   `relkit.certification-report/2`。Impact/report/Catalog/Profile 快照必须写到源码 checkout
   外的临时目录；impact 中的相对引用以该文件所在目录解析，但不可因目录搬迁改变其摘要。
   Catalog 晋升同时校验 impact/report/current registry 三方 digest，且当前干净源码
   commit 必须仍等于认证的 Candidate commit；再按待晋升 Catalog
   重新推导组件/合同差异；有运行时或合同差异时不得标为 documentation。报告绑定原 fleet
   摘要，并使用当前同一 fleet 重新计算 host、项目摘要、cohort、原因与每个 `requiredCi`，
   要求精确覆盖、不可替换、裁剪或增补。真正的 documentation 风险默认可选择零宿主；
   其他风险至少必须有一个实际通过结果。Skill 只调用命令，不复制 schema、gates 或完成账本。

状态不手填，由字段推导，避免状态与证据漂移：

- `covered`：描述、tracking 和 regression evidence 均有效；只有 code 或文案不属于 covered。
- `open`：已经 covered，但尚缺 fix PR、完整 40 字符 commit 或首次正式 release 中任一项。
- `closed`：covered 且三个 closure 字段齐全。

修复 PR merge 后，由 release prep PR 补 closure；tag release job 在构建制品前以
GitHub 上一项已发布且非 prerelease/draft 的正式 Release 为 baseline，运行
`relkit-ops escape audit ... --release <当前 tag>`。
该模式要求 `open=0`，并验证当前/声明 release tag 与完整 fix commit 都存在、fix commit
确实属于声明 release 历史，声明 release/baseline 也都属于当前 tag 历史，且工作树就是
干净的当前 tag 源码；同时拒绝 `fix.release` 晚于当前 tag。因此普通 PR 不会被未知 merge
commit/release 阻塞，stable 也不能从旁支、失败 tag 或脏工作树洗掉历史记录。

闭环是 `escape → executable test → candidate certification → release gate`。只有同类缺陷会在
下一个无关宿主到达生产路径之前被中央自动化拦住，才算完成沉淀；只有文案、code 或本地
journal 条目均不算。

### 8. 协调切换与回滚

vNext 首次切换：

1. 冻结 v0.5 控制面新特性（正确性最小修复除外，ADR 0019）
2. 实现 contracts、wrapper、resolver、conformance
3. 代表性宿主建立 vNext 分支，删除旧 config/lock/wrapper/host/onboarding
4. 为每个真实 Operator 生成独立 Profile、Catalog 与 fleet
5. 同一 candidate distribution 通过合同、组合、S1–S3 与迁移样本验证
6. 每个认证域分别以 dev 发布证明真实发布链和旧 field 客户端仍可读 RUP
7. 在该认证域内完成 impact → fleet certification → Catalog 晋升
8. 原子切换该 Operator 的 production Profile/policy；强耦合产品在同一窗口，独立域可分批
9. 每个认证域单独进入 stable 窗口

架构迁移样本通过不等于所有生产环境必须同夜切换。公网、内网或特殊构建环境若属于
不同认证域，可以分别推进；延期域保持旧 production/candidate 状态，不阻塞已闭合域，
也不能借用已闭合域的认证结果。

回滚：

| 阶段 | 动作 |
| --- | --- |
| 未升级 Operator | 回退宿主分支 |
| Operator 升级失败 | 恢复旧服务/Profile |
| 已发布 dev | 索引版本 fix-forward |
| 已进 stable | 不改写历史索引 |
| 单仓问题 | **不为单仓**加旧 reader |

### 9. 灾备顺序（闭合默认）

1. 恢复 Distribution Root / Candidate Identity 可用性（离线备份）  
2. 恢复 certified catalog 与 Operator Profile（按 sequence）  
3. 恢复服务二进制与 Token  
4. 最后恢复 product RUP 发布能力  

冻结攻击（`expiresAt`）按 ADR 0019：vNext 首轮接受残余风险，单独后续 ADR。

## 否决

- Skill 保存 verified step 或第二份 schema/gates
- 永久以四仓库名作为唯一 canary 名单
- rotate 未 reload 即标完成
- Operator 半升级对项目可见
- 为单宿主加长期兼容分支掩盖失败

## 后果

- 验证成本随能力增长，不随宿主数量平方增长
- 运维与产品仓隔离
- 使用者最终通过 [`docs/architecture.md`](../architecture.md) 与协议 conformance 验收（原 `USER-ACCEPTANCE.md` 已随 `docs/design/vnext/` 删除）

## 完成标准（本 ADR）

能唯一回答：

1. 何种 change 只跑 cohort，何种必须全部已登记宿主？
2. Operator 半升级为何不暴露给项目？
3. Token rotate 何时才算完成？
4. 迁移四阶段如何回退或 fix-forward？
