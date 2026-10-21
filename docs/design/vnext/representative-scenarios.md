# vNext 代表性能力与场景

本文把当前四仓**映射为能力与匿名场景**，供认证与使用者验收使用。  
**禁止**把仓库名写入 Relkit core、registry 或合同分支。

## 能力维度（可独立组合）

| 维度 | 取值示例 |
| --- | --- |
| 关系 | 自更新；管理器更新其他产品 |
| 订阅 | 单产品；多产品订阅 |
| 平台 | 单平台；多平台一次发布合并 |
| 应用方式 | installer-only；原地替换；版本库存 + 切换 |
| 制品选择 | platform；component；audience |
| 语言接入 | projection（只展示）；facade（驱动 updater） |

## 三个匿名端到端场景

### S1. 独立应用自更新

- 自己发布、自己检查与应用更新
- 可组合：跨平台、installer/payload 不对称、多版本库存与切换
- 当前真实样本：SvnMergeTool（仅样本，非合同身份）

### S2. 管理器与被管理产品

- 被管理产品发布制品 + 受信 Runtime Profile
- 管理器自更新，并安装/切换/更新另一产品
- 当前真实样本：Loom + Loom-Launcher（共同构成一例；Loom 单独不是第四种形态）

### S3. 可选择的产品套件

- 同一 product release 内 component/audience 可独立选择
- 可组合多语言 facade，仍共用同一 updater 与发布合同
- 当前真实样本：Dec

## 测试分层

1. **独立能力测试** — 每项能力单独证明合同  
2. **组合测试** — 共享状态的能力显式组合（非全笛卡尔积）  
3. **S1–S3 端到端** — 完整发布→检查→下载→应用  
4. **真实宿主** — 迁移期四仓联合验收；长期按受影响能力选 canary  

新增能力时必须：定义合同 → 独立测试 → 声明交互面 → 组合测试 → 至少一个端到端 → 首宿主临时 canary。

## 组合测试重点（非穷举）

组合测试只覆盖共享状态和真实交互，不跑无关能力的全笛卡尔积。当前至少包括：

- 多平台 × installer/payload 不对称，证明缺少 payload 是显式能力而非偶然漏产物
- `in-place` / `library` × 多版本库存、切换、回滚，证明状态机与保留策略一致
- 管理器自更新 × 单/多产品订阅，证明不同产品的 root、Profile 和 active version 不串线
- component × audience × selector，证明可选组件不会改变同版本其他受众的制品集合
- candidate × production lane，证明身份、Catalog、凭据、namespace 与 stable 写权限隔离
- facade/projection × updater IPC 窗口，证明各语言只消费同一 sidecar 合同
- 公网、mirror、完全离线供给 × 同一 resolution，证明供给方式不改变选版与摘要

新增能力先声明它读写的共享状态，再把受影响交互加入本清单和自动化；不能只让首个宿主
端到端变绿，也不因为一个新维度出现就重跑所有无关组合。

## 保证边界

S1–S3 全绿不是“任何项目、任何环境都不会出问题”的证明。可承诺的是：

1. 宿主只依赖已声明能力和合同
2. 宿主使用的共享状态组合已有组合或非法组合测试
3. 端到端场景覆盖发布、发现、下载与应用主链
4. 目标 Operator 的真实 canary 补充发现合同外环境依赖

只组合已有能力的新项目通常不改 core；若出现尚未覆盖的交互，只补该组合及其非法边界，
不重写所有测试。真正新增能力时，测试与合同同时变化；没有独立、组合和至少一个端到端
证据，该能力不算完成。

真实宿主验证按认证域进行。某次公网 Operator 只选 S3/Dec canary，可以证明该公网域，
但不能据此宣称内网 S1/S2 已完成生产切换；后者保留自己的 Catalog、fleet 和发布窗口。

## 四仓现实验收映射（迁移窗口）

| 仓 | 证明的场景/能力 | 不得落入 core 的特判 |
| --- | --- | --- |
| Loom | S2 的被管理侧；projection；Runtime Profile 产出；版本树元数据由 producer 声明 | 不得硬编码 loom 元数据字段 |
| Loom-Launcher | S2 管理器侧；多订阅；LIBRARY / IN_PLACE；双 apply 流 | 不得手抄 EDITOR URL/公钥 |
| SvnMergeTool | S1；多平台不对称；版本库存与切换 | 不得强迫 macOS 必须有 payload |
| Dec | S3；component×audience；Go/Rust facade | 不得双份手工 runtime 配置 |

cronkit、novoard-agent、SvnAutoMerge 等**不纳入**本轮设计与门禁。

其中 cronkit 于 2026-10-21 由用户裁决**退出接入范围**，按“从未接入”处理：
vNext 的接入面就是上表四仓，没有第五个待迁移宿主。它仓内的 legacy 控制面
（`relkit.json` 控制面、`scripts/host`、`relkit ci release --execute` 发版链）
随该仓自行归档，Relkit 不为其保留任何兼容分支或按产品特判的路径。
