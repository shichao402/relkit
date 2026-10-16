# ADR 0022: 认证模型、relkit-ops 与 Skill

- 状态：Accepted（分层认证、relkit-ops 与薄 Skill 已实现）
- 日期：2026-10-16
- 依赖：[ADR 0019](0019-vnext-breaking-and-protocol-contracts.md)、[ADR 0020](0020-vnext-catalog-operator-resolution.md)、[ADR 0021](0021-vnext-artifact-runtime-registry.md)
- 关联：使用者验收入口 [`../design/vnext/USER-ACCEPTANCE.md`](../design/vnext/USER-ACCEPTANCE.md)

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

#### 6.3 三个匿名代表性场景（长期 mandatory）

见 [`../design/vnext/representative-scenarios.md`](../design/vnext/representative-scenarios.md)：

1. 独立应用自更新  
2. 管理器与被管理产品  
3. 可选择的产品套件  

仓库名不得写入场景合同。

#### 6.4 impact / cohort / canary

- candidate 生成 impact manifest  
- 按 project capability fingerprint 选 cohort  
- 每 cohort：固定 canary + 轮换样本  
- 首个使用**新能力**的真实宿主临时 mandatory canary；稳定后不永久绑定  

#### 6.5 反向 candidate CI

生产按真实 Operator；candidate lane 用 override；观察窗口内失败阻止 certified。

#### 6.6 风险分级与全量

| 变更类型 | 验证范围 | 观察窗口（默认） |
| --- | --- | --- |
| 文档/非运行时 | conformance 子集 | 无 |
| 单能力实现 | 该能力 + 声明的组合 | patch：1 个宿主 CI 周期 |
| 普通 contract bump | 受影响 cohort | minor：受影响 canary 全部绿 + 1 周期 |
| 横切（签名、resolution、发布事务、协议、registry、IPC） | **全部已登记活跃宿主** | major：全部绿 + 延长窗口 |

- 内网蓝盾与外网 GHA：**凡声明为该 Operator/宿主认证路径的，都必须通过**才可 certified；缺一不可把缺口标为 `skipped-unverified`
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

### 8. 协调切换与回滚

vNext 首次切换：

1. 冻结 v0.5 控制面新特性（正确性最小修复除外，ADR 0019）
2. 实现 contracts、wrapper、resolver、conformance
3. 四仓建立 vNext 分支，删除旧 config/lock/wrapper/host/onboarding
4. 为真实 Operator 生成 Profile
5. 同一 candidate distribution 验证四仓
6. dev 发布证明旧 field 客户端仍可读 RUP
7. 升级 Operator 并签发新 Profile
8. Loom 与 Launcher 同一窗口；SvnMergeTool、Dec 可并行
9. stable 单独窗口

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
- 使用者最终只通过 [`USER-ACCEPTANCE.md`](../design/vnext/USER-ACCEPTANCE.md) 验收

## 完成标准（本 ADR）

能唯一回答：

1. 何种 change 只跑 cohort，何种必须全部已登记宿主？
2. Operator 半升级为何不暴露给项目？
3. Token rotate 何时才算完成？
4. 迁移四阶段如何回退或 fix-forward？
