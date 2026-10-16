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

## 四仓现实验收映射（迁移窗口）

| 仓 | 证明的场景/能力 | 不得落入 core 的特判 |
| --- | --- | --- |
| Loom | S2 的被管理侧；projection；Runtime Profile 产出；版本树元数据由 producer 声明 | 不得硬编码 loom 元数据字段 |
| Loom-Launcher | S2 管理器侧；多订阅；LIBRARY / IN_PLACE；双 apply 流 | 不得手抄 EDITOR URL/公钥 |
| SvnMergeTool | S1；多平台不对称；版本库存与切换 | 不得强迫 macOS 必须有 payload |
| Dec | S3；component×audience；Go/Rust facade | 不得双份手工 runtime 配置 |

cronkit、novoard-agent、SvnAutoMerge 等**不纳入**本轮设计与门禁。
