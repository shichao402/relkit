# Relkit vNext 设计包

> **审查入口（推荐）**：[USER-ACCEPTANCE.md](USER-ACCEPTANCE.md)  
> 从 Relkit 产品视角与接入宿主视角验收；无需通读 ADR。

## 文档地图

| 文档 | 用途 |
| --- | --- |
| [USER-ACCEPTANCE.md](USER-ACCEPTANCE.md) | **唯一推荐审查入口**：检查项、产品侧承诺、证据、一票否决 |
| [representative-scenarios.md](representative-scenarios.md) | 能力维度与三个匿名场景；四仓仅作样本映射 |
| [cutover.md](cutover.md) | Candidate→Production 顺序、认证域隔离、提交待确认与回滚 |
| [first-production-rollout.md](first-production-rollout.md) | 首次真实生产推进的事实、设计结论与通用缺口 |
| [single-core-ops.md](single-core-ops.md) | core / wrapper / ops / Skill 职责与生产控制面命令 |
| [schemas/](schemas/) | `project` / `operator-profile` / `resolution` / `artifact-set` / `runtime-profile` / `extension` 草案 |
| [contract-vectors.md](contract-vectors.md) | 四类协议合同的 requires/provides 向量 |

## ADR（已接受并实现）

| ADR | 主题 |
| --- | --- |
| [0019](../../adr/0019-vnext-breaking-and-protocol-contracts.md) | 断代边界与协议合同 |
| [0020](../../adr/0020-vnext-catalog-operator-resolution.md) | Catalog、Operator、Resolution、wrapper |
| [0021](../../adr/0021-vnext-artifact-runtime-registry.md) | ArtifactSet、Runtime Profile、Registry |
| [0022](../../adr/0022-vnext-certification-ops-skill.md) | 认证、ops、Skill、迁移 |

## 阶段

1. 架构决策：完成，ADR 0019–0022 Accepted。  
2. core / ops / wrapper：完成，并有合同与负面 conformance。  
3. 代表性迁移：Loom、Loom-Launcher、SvnMergeTool、Dec 共用同一 candidate
   distribution 验证；仓库身份不进入合同。
4. 生产切换：按 Operator 认证域独立推进。2026-10-17 首窗口仅覆盖公网 Dec，
   内网三宿主延期且不复用公网认证。
5. 最终状态以 [USER-ACCEPTANCE.md](USER-ACCEPTANCE.md) 的真实证据为准。
