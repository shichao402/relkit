# relkit sim-hosts

relkit 的真实消费者模拟分支。把四个已接入 relkit 的真实仓（Dec、cronkit、
SvnMergeTool、Loom、Loom-Launcher）的接入架构抽象成三个 e2e 场景，用
hello-world 程序复刻各仓的接入形态，走真实 SDK facade + 真实发布链 +
公网 Operator（firoyang-public），覆盖实际接入 relkit 可能踩到的坑。

cronkit 未迁移 vnext（仍是旧 consume 形态），不纳入本轮模拟（用户裁决 1A）。

## 场景映射

| 场景 | 产品 id | 样板仓 | 模拟的架构要素 |
| --- | --- | --- | --- |
| S1 独立应用自更新 | `sim-s1` | SvnMergeTool | dart facade；windows library(retain=2) / darwin in-place 不对称；installer+payload |
| S2 管理器+被管理 | `sim-s2-manager` | Loom-Launcher | rust facade；双订阅（自身 in-place + managed library）；两段式 apply |
| S2 被管理端 | `sim-s2-managed` | Loom | node/TS projection（publisher-only，不驱动 updater） |
| S3 可选套件 | `sim-s3` | Dec | go facade；component×audience 选择器；多平台合并发布；in-place |

精简原则：相同能力只在最先用到它的场景作为主链（S1 承担发布链主链；
S2/S3 复用同一 producer 与发布 workflow，不重复）。

## 布局

```
products/
  sim-s1/           # Dart facade（AOT 编译单文件）
  sim-s2-manager/   # Rust facade（cargo release）
  sim-s2-managed/   # Node blob（脚本制品）
  sim-s3/           # Go facade（3 components × installer+payload）
scripts/
  producer.mjs      # 共用 ArtifactSet producer（按 product 分派）
  e2e_driver.mjs    # 更新链路验证 driver
relkit.project.json # 每产品一份（products/<id>/relkit.project.json）
```

## 发布与验证

发布走与真实仓相同的链路（Dec release.yml 同构）：

1. `relkitw artifacts produce --version v... --channel dev --target ...`
2. 多平台 ArtifactSet merge
3. `relkitw stage --artifact-set ...` → `relkitw release --version ... --execute`
4. `relkitw verify released --version ... --channel ...`
5. e2e driver 拉起客户端验证 check → apply → 版本断言

公网 Operator: `firoyang-public`（publish.firoyang.com）。

## 边界

- 本分支只覆盖公网认证域；内网（tencent-internal / update.devcloud.woa.com）
  不在本分支范围（用户裁决：先只考虑公网版本）。
- 模拟程序是 hello-world，业务逻辑极简；壳层（Tauri/Flutter/NSIS）自身的
  风险属宿主风险，不属 relkit 合同层，不模拟（用户裁决 2A）。
