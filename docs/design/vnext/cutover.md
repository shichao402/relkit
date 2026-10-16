# vNext 协调切换与回滚

配套 [ADR 0022](../../adr/0022-vnext-certification-ops-skill.md) §8。使用者摘要见 [USER-ACCEPTANCE.md](USER-ACCEPTANCE.md) §I。

## 切换顺序

1. 冻结 v0.5 控制面新特性（正确性最小修复除外）  
2. 实现 contracts、wrapper、resolver、conformance  
3. 四仓 vNext 分支：删除 consume lock、host scripts、onboarding、复制 wrapper  
4. 签发真实 Operator Profile（内网 + Dec 等）  
5. **同一** candidate distribution 跑四仓验收  
6. dev 发布：证明旧 field updater 仍可读 RUP  
7. 升级 Operator → 原子新 Profile  
8. Loom + Launcher **同一窗口**；SvnMergeTool、Dec 可并行  
9. stable 单独窗口  

## 回滚矩阵

| 阶段 | 动作 | 禁止 |
| --- | --- | --- |
| 未升级 Operator | 回退宿主分支 | — |
| Operator 升级失败 | 恢复旧服务 + 旧 Profile | 暴露半升级 |
| 已发 dev | fix-forward 新索引版本 | 改写已发 index 内容冒充未发 |
| 已进 stable | fix-forward；不改历史 | 为单仓加旧 reader |

## 迁移期验收样本

四仓仅作真实样本，证明 S1–S3；通过标准是 [USER-ACCEPTANCE.md](USER-ACCEPTANCE.md)，不是「某仓特判绿了」。
