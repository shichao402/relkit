# vNext 协调切换、生产晋升与回滚

配套 [ADR 0022](../../adr/0022-vnext-certification-ops-skill.md) §8。使用者摘要见
[USER-ACCEPTANCE.md](USER-ACCEPTANCE.md) §I。

本文是 Candidate → Production 的唯一逐步 runbook；Skill 只编排这里对应的结构化命令，
不得复制另一套顺序或完成账本。

## 两层切换不能混为一谈

1. **架构迁移验收**：用代表性宿主证明合同、能力组合和 S1–S3。四仓只是迁移样本。
2. **生产认证域切换**：按目标 Operator 的 Catalog、fleet、CI 路径和现场分别晋升。

第一层通过不要求所有生产环境同夜切换。公网、内网、特殊构建环境若属于不同 Operator
或责任边界，可以分窗口推进；延期域保持原状态，不阻塞其他域，也不能借用其他域的
认证报告。

每个生产认证域独立持有 Catalog 快照、Operator Profile / `deploymentId`、fleet 与
`requiredCi`、candidate/production policy、bundle URL/SHA、回滚备份及健康证据。
“当前只验证公网 GHA”是某次窗口的范围事实，不是永久 fleet 定义；以后纳入内网蓝盾或
其他 Operator 时新增该域登记并重新认证，不改 Relkit core。

## 证据新鲜度

调查、认证和签发前必须：

1. 更新每个参与仓的远端引用，并把可安全快进的工作副本更新到最新
2. 有本地改动时保留用户工作，不以 reset/checkout 覆盖；另用干净 worktree 认证
3. 记录每个输入仓的精确 commit、分支/tag 与 dirty 状态
4. impact、回归源码、Catalog 晋升必须回到同一干净 Candidate commit

基于旧 checkout 得出的“宿主不支持”“CI 缺失”或“合同冲突”不算证据。

Issue 是真实使用者痛点的重要输入，但标题、归因和建议方案不是设计真相。必须在最新
commit 上复现，核对现有 ADR/合同，并横向检查代表性能力组合；不得直接把某个 issue 的
项目专用修法写进 core。

## Candidate → Production 顺序

1. 发布完整 immutable candidate distribution，签发独立 Candidate Catalog/Profile。
2. 为目标认证域生成 candidate resolution 与**完整**离线 bundle；bundle 必须包含
   distribution 的全部 component 和 release assets。
   生成的 Runtime Bootstrap、resolution 与 bundle manifest 必须以落盘字节送入公开
   SDK/wrapper 验证；控制面内部类型测试或手写 fixture 不能替代这一步。
3. Agent 先切 candidate policy：`plan → execute + 唯一备份 → restart/reload → health`。
   只升级 agent/store 二进制不等于切换 lane；只改 policy 不重启也不算生效。
4. 宿主 dev CI 使用同一 candidate resolution 生成、合并并发布 ArtifactSet。
5. 从消费者可见端执行 `verify released`，同时固定：
   - immutable resolution
   - Candidate Root（仅 candidate）
   - 与 `runtimeProfilePublication.rootFingerprint` 匹配的 Product RUP Root
   - 正式场景证据必须从 bundle 安装后的 bootstrap 发现 Product Root；显式
     `--product-root` 只用于诊断，不能代替这条链路
6. 在干净 Candidate commit 上执行 impact、目标域 fleet certification 与 production
   escape audit。
7. 用同一 fleet/report 把该域的 Catalog 条目从 candidate 晋升为 certified。
8. 使用 production Catalog/Profile 重新生成 production resolution 和完整 bundle；
   production bundle 不携带或依赖 Candidate Root。
9. 将 bundle 发布到不可变 URL，CI 只保存 URL + SHA-256 对；先验证下载摘要，再改变量。
   若使用 GitHub auxiliary Release，必须标为 prerelease/非 Latest；正式 release baseline
   只接受 `vX.Y.Z`，不能让 control-plane tag 冒充产品版本。
10. 原子切换 Agent production policy，移除 Candidate Root，并保留可直接恢复的旧配置。
11. stable CI 必须使用 production bundle；打 tag 或启动 workflow 只表示开始发布，
    不等于 certified 或 stable 成功。CI 与资产读回通过后，必须从 production bundle
    签发的 Runtime Bootstrap 启动真实 updater，执行 `check → download → apply → --version`，
    再做回滚和安全验收。

Catalog、Profile、resolution、bundle 和 Agent policy 是一组事务证据，不能只替换其中一项。
已经推送但失败或取消的 dev/stable tag 不移动、不覆写；修复后提升产品版本并创建新 tag，
使失败输入和后续 fix-forward 都可追溯。

## 已提交但立即读回失败

`release --execute` 包含“写入”与“从消费者入口读回验证”。网络、缓存或投影延迟可能导致：

- CAS、manifest、index 或 directory 已提交
- 命令在最终 consumer readback 报错

此时结果是**提交状态待确认**，不是“肯定没发布”。必须先用同一 version/channel、
resolution 和 Product RUP Root 做有界重试的 `verify released`：

- 读回通过：保留原发布，不得重发同一 immutable version
- 读回仍失败且能证明未提交：才允许重新执行
- 已部分提交：按 `partial-commit` / fix-forward 处理，不覆盖历史

发布器应输出结构化 commit receipt 和稳定错误 code；错误信息中的空白 fix-forward、
仅凭 CI 红灯判断“未发布”，都不满足运维合同。实现跟踪见
[issue #52](https://github.com/shichao402/relkit/issues/52)。

## URL 与信任根的常见误判

- Operator 的 `consumer` / `directory` 是 **base URL**；拼接 `directory/<product>.pb`
  必须使用统一 URL 解析合同，不能依赖字符串末尾恰好有 `/`
- 解析测试必须覆盖根路径、有 path prefix、带/不带尾斜杠
- Candidate Root 证明 resolution 的候选授权；Product RUP Root 验证产品更新数据，
  两者不能互换
- `trustedKeys[].key` 等跨语言字节字段必须使用 Schema 规定的 canonical 编码；至少一条
  门禁要把正式生成 JSON 交给公开 SDK，而不是分别测试生成器与手写 fixture
- 本机残留环境变量或旧 bootstrap mapping 可以产生“authority/policy 不匹配”，
  诊断时先显式传入当前 resolution 和 root，不先归咎远端 Agent

## 2026-10-17 首次生产窗口范围

- 架构迁移仍以 Loom、Loom-Launcher、SvnMergeTool、Dec 映射 S1–S3。
- 本次生产晋升只覆盖 `firoyang-public` Operator 与 Dec（S3）公网链路。
- Loom 系列与 SvnMergeTool 的内网环境特殊、现场噪声高，退出本窗口；其内网 Catalog
  不因公网认证自动晋升，后续另开窗口。
- 这项收窄是**认证域隔离**，不是删除 S1/S2 能力，也不是把 Dec 写进 core 合同。

## 回滚矩阵

| 阶段 | 动作 | 禁止 |
| --- | --- | --- |
| 尚未晋升 Catalog | 保持 candidate；修复后重新认证 | 直接让 production 信任 candidate |
| 已晋升、未切 Agent | 恢复旧 bundle/CI 变量 | 把“Catalog 已签”说成生产已切 |
| Agent policy 切换失败 | 恢复唯一备份并重启、健康检查 | 暴露半切换配置 |
| 已发 dev | 先 verify 判断是否已提交；已提交则 fix-forward | 盲目重发同一版本 |
| 已进 stable | fix-forward；不改历史索引 | 为单仓加旧 reader |
| 某认证域延期 | 保持该域旧 Catalog/Profile | 用其他域报告越域晋升 |

## 最终验收

生产完成至少需要：

1. stable CI 与发布资产成功
2. 消费者入口 `verify released` 通过
3. 公开 SDK 能解析实际签发的 Runtime Bootstrap，真实已安装客户端完成
   检查/下载/应用并报告目标版本，或有可证明的不适用项
4. Agent、Catalog、Profile、bundle URL/SHA 与 resolution provenance 可追溯
5. 回滚快照存在且恢复流程已验证
6. 日志、仓库和 release asset 不含 Token、私钥或带签名查询串

通过标准是 [USER-ACCEPTANCE.md](USER-ACCEPTANCE.md)，不是“某仓特判绿了”。
