# Relkit vNext — 使用者验收（唯一推荐审查入口）

> **给审查者**：你不需要阅读 ADR 0019–0022 的实现细节。  
> 本文件从两个角度写清「做成什么样算对」：  
> 1）Relkit 作为产品本身；2）接入 Relkit 的宿主产品。  
> 实现完成后，每项用真实命令/CI/运行结果打勾；**只有设计文档不算通过**。

状态约定：`通过` / `不通过` / `不适用`。  
当前阶段（2026-10-16 实现验收）：A–I 全部通过。这里的「通过」指实现、合同测试和本机宿主验收已闭合；生产 Operator 切换、远端发布和目标平台正式发布仍按正常发布窗口执行，不属于本次本地验收的写操作。

---

## 怎么审查（建议 30–60 分钟）

1. 只读本文件 + 文末「一票否决」。  
2. 对每个阻断项问一句：*换成我的产品，流程是否说得通？*  
3. 细节 ADR 仅在某条承诺让你不信时再点开链接。  
4. 实现落地后，按「验收证据」列复跑，把状态改成通过/不通过。

架构文档索引（可选）：[README](README.md) · [ADR 0019](../../adr/0019-vnext-breaking-and-protocol-contracts.md) · [0020](../../adr/0020-vnext-catalog-operator-resolution.md) · [0021](../../adr/0021-vnext-artifact-runtime-registry.md) · [0022](../../adr/0022-vnext-certification-ops-skill.md)

---

## 2026-10-16 最终验收记录

- 同一候选套件：`v0.6.0-candidate.6+1ee9a9917d66`，distribution `sha256:76d408a5f5be3c6583be24eba79d7c31d4caaf415c9b681074487332577f19ca`，Candidate Root `sha256:0f9057f51369d07d6e23b7b25963d3857deeec9077fd7aa4cb37237010488585`。
- 四仓不可变 resolution 均经 `doctor --json` 验证并完成物化：Loom `sha256:9de6d5cbfb076dcfced3f5c28e6bef5d71878a35c419eac2c6dbbba1026da01f`；Loom-Launcher `sha256:429c1816da931df915eb2c28585321e82645eb8344ec6ffd80696d035937882b`；SvnMergeTool `sha256:2b0640510ee26964ae72dd36e143801aaea3c523a8745a6aff6dadf20050d6dd`；Dec `sha256:3bb050238b9c3ca7619f332bd82655fa6ac1d1ddea84aff047b54b4ab409a09d`。
- Relkit：`go test ./... -count=1 -timeout=20m` 全绿；Rust SDK 16 项、Dart 85 项（含真实 Relkit publish → Dart consume 互操作）、Node SDK 46 项、TS bindings 5 项及 Python 旧边界测试全绿。
- 发布事务负面证据随 Go 全量通过：directory / Runtime Profile / released verification 失败不返回成功；多 backend 字节不一致、partial commit、candidate 写 stable、回放和摘要篡改均被拒绝；旧 field updater 可读取 vNext 发布数据。
- 宿主证据：Loom 全量 package 测试通过并实产 2 个 Windows ArtifactSet 条目；Loom-Launcher Node 全量、Rust 70 项与 Candidate doctor 通过；SvnMergeTool 4026 项测试、release preflight 及真实 macOS DMG 通过；Dec Go 全量及 7 个 component × audience ArtifactSet 条目通过。
- 迁移审查：四仓只保留 `relkit.project.json`、canonical `relkitw/relkitw.cmd` 和宿主 producer；旧 consume lock、复制 bootstrap、onboarding 与宿主 mirror 运维入口已退出活动路径。历史 ADR/changelog 中的旧路径文字不构成可执行入口。
- 安全边界：真实 Runtime Profile 私钥不在本机，本次以仓外本机合成 key 验证签名、合同、解析、物化和 producer；未改 production Operator、stable 指针或远端 backend。macOS 无法实跑 Loom-Launcher 的 Windows NSIS Job，其静态发布链、Node/Rust 行为和 Windows resolution 已通过；正式部署前仍须由 Windows Runner 执行常规平台构建。

---

## A. 接入体验（宿主视角）

| # | 类型 | 产品侧承诺 | 你应看到的证据（实现后） | 状态 |
| --- | --- | --- | --- | --- |
| A1 | **阻断** | 新项目只需声明：产品、目标服务、所需能力、自己的构建入口；**不复制** Relkit 脚本 | 新仓只有 `relkit.project.json` + 生成的 `relkitw`；无 `scripts/host` / 复制 bootstrap | 通过 |
| A2 | | 项目文件**不写** Relkit 版本、服务器地址、Token、SSH、mirror 凭据 | `relkit.project.json` 审查无上述字段 | 通过 |
| A3 | | 首次 clone：公网 / 仅 mirror / 完全离线三条路径说得清 | `relkitw` 文档 + 离线 bundle 一次成功演示 | 通过 |
| A4 | | 一个只读命令能回答：连哪台服务、用了哪个 Relkit、为何选它、配置来自哪 | `relkit inspect --json` / `doctor` 输出 | 通过 |
| A5 | | 失败直接指出缺能力/资产/权限，不靠翻 Skill 日志猜 | 故意缺资产的失败演示，含稳定 `code` | 通过 |

**白话**：接入 = 填项目声明 + 指向你们的更新服务器；版本与密钥不由业务仓保管。

---

## B. 版本行为（Relkit + 宿主）

| # | 类型 | 产品侧承诺 | 验收证据 | 状态 |
| --- | --- | --- | --- | --- |
| B1 | **阻断** | 连**固定自建**服务器 → 用该服务器**精确** Relkit 套件，不自己追别的 latest | `exact` Operator 上 inspect 显示 server 版本 = 所选 distribution | 通过 |
| B2 | | 连**滚动**服务 → 只用已认证且合同匹配的最新套件 | `latest-compatible` 选择记录 + catalog certified 证明 | 通过 |
| B3 | **阻断** | **一次 CI 内版本钉死**；install/build/各平台/publish 同一份 resolution | 多 Job 共用同一 resolution 文件；digest 全程一致 | 通过 |
| B4 | | 记录能回答「这次到底用了哪版、哪些摘要」，不能只写 latest | 发布 provenance / resolution 存档 | 通过 |
| B5 | | 生产 pin 住的项目仍能在 **candidate 车道**提前试下一次升级 | 同仓 production 绿 + candidate 红/绿独立报告 | 通过 |

**白话**：版本跟「你连的那台服务器」走；流水线中间不会偷偷换成另一版；想提前踩坑走 candidate，不污染生产。

---

## C. 谁说了算（客制化边界）

| # | 产品侧承诺 | 验收证据 | 状态 |
| --- | --- | --- | --- |
| C1 | 宿主可定：产品版本、业务构建、制品内容、UI、检查时机、允许的安装动态参数 | 宿主 producer / 产品代码仍归宿主仓 | 通过 |
| C2 | 宿主**不能**改：resolution、签名数据、directory/index/manifest、发布事务、updater 状态机 | 故意篡改在 publish/apply 前失败 | 通过 |
| C3 | 扩展用有版本的 stdin/stdout 合同；失败/超时/权限可观察 | extension 合同测试 | 通过 |
| C4 | 只组合已有能力的新项目**不改 Relkit core**、不加仓库名特判 | 新仓仅 project 声明即过 conformance | 通过 |
| C5 | 真·新能力 = 合同 + 独立测试 + 组合测试 + 至少一个端到端 | 能力变更检查单 | 通过 |

---

## D. 三种用法（代表性场景）

详细映射：[representative-scenarios.md](representative-scenarios.md)

| # | 场景 | 产品侧承诺 | 验收证据 | 状态 |
| --- | --- | --- | --- | --- |
| D1 | 独立应用自更新 | 跨平台、平台制品可不对称、installer/payload、多版本库存与切换按**声明**工作 | S1 端到端 + SvnMergeTool 迁移验收（样本） | 通过 |
| D2 | 管理器 ↔ 被管理产品 | 管理器更新自己并订阅另一产品；**不手抄**对方 URL/公钥 | S2 + Loom/Launcher：无 `EDITOR_*` 手抄 | 通过 |
| D3 | 可选产品套件 | 同一 release 内 component/audience 可选；不另搞一套更新协议 | S3 + Dec 迁移验收 | 通过 |
| D4 | | 三场景共用同一 core/updater 合同；差异来自声明，不是隐藏分支 | 无仓库名 `if` 的代码审查门 + 场景测试 | 通过 |

**白话**：不是「四种产品类型」，而是同一更新模型上的能力组合。

---

## E. 发布可信（Relkit 产品核心）

| # | 类型 | 产品侧承诺 | 验收证据 | 状态 |
| --- | --- | --- | --- | --- |
| E1 | **阻断** | stage→simulate→fake→publish 同一 channel/版本/resolution | 中途改 latest 被拒 | 通过 |
| E2 | **阻断** | artifact/manifest/index/directory/Runtime Profile **任一关键失败 → 整体不得成功** | 注入 directory 失败：最终非 ok | 通过 |
| E3 | | 多 backend 必须内容一致，不能只比 sequence 大小 | 不一致 backend 演示失败 | 通过 |
| E4 | | 部分提交 → 明确 partial-commit + 修复路径 | 错误 code + repair 文档 | 通过 |
| E5 | | 结束后有独立验证：从消费者可见位置再读一遍 | `relkit verify released` 通过/失败演示 | 通过 |
| E6 | | 浏览页/HTML 失败 ≠ 核心发布失败，且可重建 | site 挂了仍可标核心成功 + rebuild | 通过 |

---

## F. 客户端更新行为

| # | 产品侧承诺 | 验收证据 | 状态 |
| --- | --- | --- | --- |
| F1 | 各语言正式 facade 上 check/download/apply/status/错误一致 | facade conformance | 通过 |
| F2 | 声明兼容窗口内，**旧 updater 仍能读**服务升级后的新数据 | 旧 sidecar + 新 directory 实机 | 通过 |
| F3 | 非法 selector、合同不匹配、摘要/签名错误、回放旧配置 → 改本机或发布前失败 | 负面用例 | 通过 |
| F4 | 多版本切换 / 原地更新 / installer-only 仅在声明支持时出现 | 未声明则不可达 | 通过 |

---

## G. Candidate 不污染生产

| # | 类型 | 产品侧承诺 | 验收证据 | 状态 |
| --- | --- | --- | --- | --- |
| G1 | **阻断** | candidate 独立身份与权限；**不能**发到 stable / 改 production certified | 权限拒绝演示 | 通过 |
| G2 | | 独立能力 + 组合 + S1–S3 有明确覆盖 | CI 矩阵可见 | 通过 |
| G3 | | 普通改动只跑受影响 canary；横切才全量已登记宿主 | impact manifest + 选仓记录 | 通过 |
| G4 | | 新能力首宿主临时 canary，稳定后不永久绑仓 | 登记策略文档 | 通过 |
| G5 | | 失败能归因：core 破坏 / 声明错误 / 宿主未声明依赖 / 环境 | 诊断 `code` 分类 | 通过 |

---

## H. 运维与安全（不进产品仓）

| # | 产品侧承诺 | 验收证据 | 状态 |
| --- | --- | --- | --- |
| H1 | 产品仓无真实 Token/私钥/SSH/机器路径 | 仓内扫描 + 示例只有引用名 | 通过 |
| H2 | 一句话分清：core / ops / Skill；Skill 无第二套门禁账本 | 无 `.relkit/onboarding.json` 步骤门 | 通过 |
| H3 | Token/密钥轮换：新凭据生效 + **必须 reload/restart** + 健康通过 + 吊销旧的 → 才算完成 | rotate 中断任一步 → 未 active | 通过 |
| H4 | Operator 升级失败时，项目只看见完整旧版或完整新版 | 半升级注入：Profile 仍旧 | 通过 |
| H5 | mirror/bundle/公网只是供给方式；同一 resolution → 相同摘要 | 三供给源对照 digest | 通过 |

---

## I. 断代迁移可控

| # | 类型 | 产品侧承诺 | 验收证据 | 状态 |
| --- | --- | --- | --- | --- |
| I1 | **阻断** | 迁移后**只有一条**正式接入/发布路径；旧 consume、Python host、onboarding、复制 wrapper 不再备用 | 四仓删除旧入口后仍能发布 | 通过 |
| I2 | | 四仓同一 candidate distribution 验收；不为单仓加长期兼容分支 | 联合 CI 记录 | 通过 |
| I3 | | 未升 Operator / 升级失败 / 已发 dev / 已进 stable 四种回退或 fix-forward说得清 | 演练纪要 | 通过 |
| I4 | | 删控制面不影响声明窗口内 field updater 数据协议 | 旧客户端读新 RUP | 通过 |

---

## 一票否决（出现即整包不接受）

- Relkit core 判断 Loom / Launcher / SvnMergeTool / Dec 仓库身份  
- 同一次流水线不同阶段解析到不同 Relkit 版本  
- 接入新项目仍要复制/改一份宿主专用 bootstrap  
- 关键发布失败仍可能返回成功  
- candidate 能碰 stable 写权限  
- 为救一个宿主改未声明的全局行为，且无新合同与组合测试  
- 只有「应该兼容」的文字，给不出上表流程证据  

---

## Relkit 产品一句话

> Relkit 提供**稳定、可声明、可验证**的发布与更新能力；宿主用能力组合描述自己，而不是让 Relkit 适配某个仓库。版本跟随**目标服务器的部署事实**；一次构建只用一套工具；候选升级单独试，不污染生产。

## 接入产品一句话

> 我声明产品和能力，连接我们的 Operator；我不保管 Relkit 版本号和服务器密钥；我负责业务构建与制品内容；更新是否可信由 Relkit 合同与签名保证。

---

## 架构闭合对照（供实现开工前自检）

计划完成标准 13 问 → 答案位置：

| # | 问题 | 闭合位置 |
| --- | --- | --- |
| 1 | exact vs latest-compatible | ADR 0020 §2 |
| 2 | 为何无 release 两两表 | ADR 0019 §2–3 |
| 3 | 一次 CI 选哪个 distribution | ADR 0020 §5 |
| 4 | pin 为何不掩盖 candidate | ADR 0020 §7 |
| 5 | cohort vs 全量 | ADR 0022 §6.6 |
| 6 | 无 Go/无公网首次 clone | ADR 0020 §6 |
| 7 | Launcher 信 Loom profile | ADR 0021 §2 |
| 8 | 旧 field 客户端读 RUP | ADR 0019 §6 |
| 9 | Operator 半升级不可见 | ADR 0022 §5 |
| 10 | 各信任根 | ADR 0020 §3 |
| 11 | directory 失败≠成功 | ADR 0019 §5 |
| 12 | 新宿主无仓库特判 | ADR 0021 §6、本文件 D/C |
| 13 | 跨平台同 resolution digest | ADR 0020 §5 |

本表已按真实命令、测试和宿主产物完成验收；后续若任一阻断项证据失效，必须把对应状态改回「不通过」，不能用 ADR 文字代替运行证据。
