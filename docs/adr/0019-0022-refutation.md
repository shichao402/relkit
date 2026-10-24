# ADR 0019–0022 驳斥

本文逐项说明上一代 vNext 设计（[ADR 0019](0019-vnext-breaking-and-protocol-contracts.md)、[0020](0020-vnext-catalog-operator-resolution.md)、[0021](0021-vnext-artifact-runtime-registry.md)、[0022](0022-vnext-certification-ops-skill.md)）哪些决策被现实否定、证据是什么、现在结论是什么。

四份 ADR 已全部标为 Superseded，指向 [ADR 0023](0023-desktop-updater-epoch.md) 与 [`docs/architecture.md`](../architecture.md)。

写这份文档的目的不是清算，是防止后来者（包括 AI）读到旧 ADR 以为那还是现行设计，照着把东西加回来。过去"修了又坏"的来源之一就是文档与实现打架而没人说清哪边对。

格式：每项写「原决策 / 现实证据 / 结论」。仍然成立的部分单独列在最后一节「保留项」，不连带作废。

## 一、被判否的决策

### 1. 四把信任根分权

原决策（0020 §3）：设四个信任域。Distribution Root 签 Catalog 与 Operator Profile，每产品一把 Product RUP Root 签 directory/index/manifest，Runtime Profile Root 签 runtime-profile（默认复用产品根），Candidate Root 签 candidate-only 文档。理由是两个默认闭合：Catalog 与 Profile 同属一个运维信任域但与产品密钥分离；candidate 不能签 certified。

现实证据：四个域的私钥全部物理存放在同一台 agent 服务器上，同一份备份策略、同一个运维账号可读。服务器被攻破时四把私钥一起泄露，分域不产生任何隔离效果。实际发生的两次"私钥丢失"恐慌也都出自这套多根结构：`relkit-candidate-2026` 与 `relkit-candidate-2026b` 被判定丢失，两次都不是真丢，是没人记得放在哪——多一把根就多一条"钥匙在哪"的记账负担。

结论：一台服务器一把根。根的职责收窄为"证明这是这台服务器发出的、没被改过"。Distribution Root、Candidate Root、Runtime Profile Root、每产品签名密钥全部删除。candidate 与 certified 的区分随之消失，Catalog 只表达"服务器当前提供哪个版本"。

### 2. 第二层签名：根签 Catalog，产品密钥再签 index/manifest

原决策（0020 §3、0021 §2）：根签 Catalog 与 Profile；产品 RUP Root 另签 directory/index/manifest。理由是"产品内容由产品自己的根背书"，不同产品可独立轮换。

现实证据：第二层签名验证的是"这份文档属于这个产品"，而这件事 index 的路径与 manifest 的产品字段本身已经表达。产品密钥与服务器根在同一台机器上，攻破服务器两者同时失效，第二层不增加攻击者需要克服的难度。反过来它带来实际成本：每个产品要生成、轮换、备份一把密钥；发布时要多签一次；unpublish 要重签；`api/` 里要按产品挑公钥。这些成本在每一次新宿主接入时都要再付一遍。

结论：删除产品级签名密钥与第二层签名。文档只由服务器根签，文件只校验摘要。两层结构（文档签名 + 文件摘要）足以覆盖全部威胁模型。

### 3. 宿主持有 resolution，冻结一次 CI 的版本选择

原决策（0020 §5）：每次 CI 开始时 Resolver 生成一份 immutable resolution，固定 project digest、Profile digest、Catalog snapshot、selected distribution、module H1、各平台 component 摘要。CI 多 Job 传递同一文件，build 中途不得重新读 catalog。

现实证据：resolution 存在的前提是"宿主需要挑一个版本并把它冻结"。断代后宿主不挑版本：服务器 Catalog 的 `current` 就是唯一答案，一次构建里本来就是确定的。为了支撑 resolution，又长出了离线 bundle（把资产带过去）、materialize（把清单展开到本地目录）、`relkitw`（读 resolution 再展开）、`selectionPolicy`（决定挑哪个）、`requires`/`provides`（判断能不能升）、certification（判断能不能给宿主用）。这一整条链的上游只有一个假设：宿主自己决定用哪个 relkit 版本。假设消失，链失去存在理由。

结论：删除 `internal/vnext/resolution`、`offlinebundle`、`wrapper`、`materialize` 与 `relkit.resolution/1` 文档。宿主不再持有 resolution。CI 变成"下载工具、验签、执行"三步。

### 4. candidate / certified 两级认证与 fleet 认证域

原决策（0020 §4、0022 §6）：Catalog 有 `candidate → certified → superseded/revoked` 状态机，配 `certificationSequence`；认证域由 Catalog、Operator Profile 与 fleet 共同确定；fleet 是 `relkit.ops-fleet/1` 登记表，列出 mandatory 场景、真实 canary、`requiredCi`；横切变更要求该域全部 mandatory 条目通过。

现实证据：这套体系的目的是回答"某个 relkit 版本能不能给宿主用"。断代后版本由服务器标记，服务器只有两台（内网、外网），且部署由 CI 完成、上传前校验摘要与正式 Release 一致。"能不能给宿主用"这件事退化为"部署流水线是否成功"。为此维护一套状态机、一份 fleet 登记表、一个 certification report 格式与相应的 impact 计算，成本远超收益。更现实的问题是：fleet 条目需要持续维护，而实际在役宿主只有三个，认证登记表的维护成本比它保护的发布面还大。

结论：删除 Catalog 生命周期状态机、`certificationSequence`、fleet 登记、impact 与 certification-report 体系（`internal/vnext/ops/certification.go`）。保留协议层 conformance（`conformance/`）：它验证的是 schema 与签名行为，与"哪个版本能不能用"无关，仍然有效。

### 5. 宿主仓内的 wrapper 与锁文件

原决策（0020 §6）：删除宿主复制的 `scripts/relkit_cli.py`，改为宿主保留 relkit 生成的不可客制化 `relkitw` / `relkitw.cmd`；wrapper 固定 Distribution Root fingerprint、读 project 的 Operator 引用、取签名 Profile、解析完整 distribution、物化组件到 `.relkit/components/`。

现实证据：`relkitw` 存在是因为要按 resolution 把一组组件物化出来。断代后只需要三个工具文件（cli/updater/ops 按需），且每个都有服务器签名，CI 直接下载验签即可，不需要一个中间脚本代为解析。另一个实际后果：`relkitw` 一旦进入宿主仓库就要跟着 relkit 的 wrapper 协议演进，五个宿主的 wrapper 同步成为发布窗口的一部分。

结论：删除 `relkitw`、`relkit.lock.json`、`.relkit/` 下的指纹与 resolution 文件。宿主 CI 用三条固定命令代替。

### 6. `selectionPolicy: exact | latest-compatible`

原决策（0020 §2）：固定自建用 `exact`，滚动服务用 `latest-compatible`，由 Operator Profile 声明；运维命令 `relkit-ops upgrade <operator> --to latest` 的 latest 仅在操作层表示"选最新 certified"。

现实证据：`latest-compatible` 需要 `requires`/`provides` 合同向量来判断"兼容"，而合同向量又是为"宿主挑版本"服务的。服务器自己标记当前版本时，选择策略没有对象：服务器说哪个就是哪个。这个字段还带来一个隐性风险：它让"项目的行为取决于另一份文档的内容"，而断代后的目标是让项目行为只取决于自己的四个字段。

结论：删除 `selectionPolicy`、`requires`/`provides` 能力向量与 Operator Profile 的选择策略字段。

### 7. Operator Profile 作为独立签名文档

原决策（0020 §2）：Operator Profile 是"服务器现场权威，由 relkit-ops 从实际部署生成并签名"，包含 deploymentId、精确 server release、selectionPolicy、provides、endpoint/backend/service identity、secret 引用、distribution sources、mirror、bundle、SSH inventory、部署参数、CI adapter 引用、单调 sequence。

现实证据：这份文档承载的大部分字段都是被删层的东西（selectionPolicy、提供的能力、distribution sources、mirror、bundle、SSH inventory）。剩下真正有用的只有三样：服务器是谁（product）、端点在哪、允许哪些渠道。而这三样恰好就是 Runtime Profile 的内容。

结论：Operator Profile 与 Runtime Profile 合并为一份 Runtime Profile（`relkit.runtime-profile/1`），由服务器根在部署与发布时自动签发。不再有人工签发入口，不再有 `relkit-ops operator` 子命令。

### 8. production escape：自建 Go 源码静态审计

原决策（0022 §7.1）：用 `internal/vnext/ops/production_escape.go` 与 `conformance/production-escapes/*.json` 登记表，检查代码里有没有绕过发布门禁；`relkit-ops escape audit` 是 review gate，严格校验描述、tracking 与真实测试定义，拒绝 zero-match/skip；`impact/2` 固定 commit 与 coverage digest。

现实证据：这套工具本身长成了一个新的维护面：它要求每条 escape 都有可执行的 regression、稳定的 code、tracking issue、精确的测试 symbol，还要按语言区分判定方式（Go 要求 package 内唯一声明、Python 看 `unittest.TestResult`、Node 只看 `kind=test` 事件、Rust 只接受 `tests/*.rs` 顶层唯一的 `#[test]`）。它保护的是发布门禁，而发布门禁的一半内容（certification、fleet）已经删除。审计工具比它保护的对象更复杂，说明层次搞错了。

结论：删除 `production_escape.go`、`conformance/production-escapes/` 登记表与 `impact`/`certification-report` 体系。架构规矩改为写进 [`docs/architecture.md`](../architecture.md)，靠评审遵守。

### 9. 能力维度包含 audience，为 Dec 的存在服务

原决策（0021 §6）：能力维度列举"关系、订阅、平台、应用方式、制品选择（platform / component / audience）、语言接入"，其中 `audience` 用于区分 Dec 与桌面产品的更新对象。

现实证据：Dec 改为在线服务，退出 relkit 更新链。`audience` 维度失去唯一使用者，`internal/webmeta` 里按 audience 过滤的规则成为只为已退出产品存在的分支。

结论：删除 `audience` 维度与 `internal/webmeta` 的 audience 过滤规则。制品选择维度只保留 platform 与 component。

### 10. 范围不设限，selector 保留移动/沙箱/Linux 取值

原决策（0021 §6、selector conformance 夹具）：`target-dimension.json` 专门覆盖"桌面 client 与 Android server 共用版本号"的场景；`os-arch.json` 用 `linux/x64`；`apply-track.json` 用 `linux-installer` 与 `setup.deb`。

现实证据：这三个取值从未有真实使用者。`target-dimension.json` 的存在理由（桌面与 Android 共用版本号）随移动端退出而消失。Linux 安装布局、提权模型、包管理器集成都未验证过，保留取值等于给一个未验证路径发合格证。

结论：范围明确为 Windows/macOS。删除 `target-dimension.json`，把 `linux/x64`、`linux-installer`、`setup.deb` 改为桌面在役平台取值。selector 维度仍由项目自由声明，代码里没有平台白名单。

### 11. Publish Protocol 作为需要长期维护的合同窗口

原决策（0019 §2）：Publish Protocol 是四类稳定合同之一，`internal/publishproto` 当前选 `3`、兼容窗口 `[2,3]`，无交集 hard fail（426 语义保留）。

现实证据：兼容窗口的前提是有旧发布者还在跑。断代后发布者只有部署流水线与三个宿主 CI，全部随 relkit 发布同步升级。窗口的 `min` 侧永远不会有真实对端。

结论：Publish Protocol 的窗口收敛为单值，不再维护向后兼容区间。Updater IPC 同理抬到 `[4,4]`：不兼容时明确报 `UPDATER_TOO_OLD` / `UPDATER_TOO_NEW`，不做兼容分支。

### 12. `relkit-ops` 作为完整运维工具

原决策（0022 §1、§2、§5）：`relkit-ops` 负责 Operator Profile、资产供给、远端部署、Token、服务健康、CI adapter、fleet 认证登记；子命令含 `certify`、`impact`、`escape`、`bundle`、`distribution`、`artifact-set`、`runtime-profile`、`catalog`、`operator`、`upgrade`、`rotate`、`status`、`plan`、`key`。

现实证据：这些子命令中有一半操作的对象已被删除（certify/impact/escape/bundle/distribution/artifact-set/runtime-profile/catalog/operator）。剩下的是每台 agent 服务器的常驻运维面：`status`、`plan`、`upgrade`、`rotate`、`key`。

结论：`relkit-ops` 保留 5 个子命令，其余删除。Catalog 与 Profile 的签名改由服务器在部署与发布时自动完成，不再有人工签发入口。

### 13. 把"工具链来源"当作需要多通道兜底的问题

原决策（0019 §7、0020 §6）：保留 Go module + sumdb 作为 CLI/updater/Go facade 的资产来源，Dart/Rust/TS 继续用 Release assets；两种 transport 由同一 selected distribution manifest 与 resolution 固定；mirror 作为受限网络路径；完全离线用预置完整 distribution bundle。

现实证据：多通道的本意是让宿主的获取不依赖单一源。但它同时意味着"同一份工具可能从三个地方来"，每一处都要能验摘要、要能对齐版本。断代后服务器本身就是唯一事实源，工具由服务器签名提供，CI 从服务器下载。mirror 退化为 CI 中转文件的用途（凭据清单里只剩这一类），bundle 整条删除。

结论：工具一律从服务器取。删除 module H1 / Release asset / bundle 三条通道与 `relkitw` 的资产选择逻辑。宿主不再需要"下载 relkit SDK"，SDK 就是薄客户端源码。

### 14. watermark 与单调 sequence 作为防回滚手段

原决策（0020 §3 闭合默认 3、§4）：Catalog 与 Profile 带单调 sequence，旧 sequence 默认拒绝；`certificationSequence` 用于防回滚与审计。

现实证据：sequence 只在持有状态的读者那里有意义。宿主 CI 构建机不保留状态，所以 CI 侧拿不到"上次见到多少"；agent 侧本来就只有一份当前文档。真正需要防回滚的是最终用户：updater 对产品 index 的防回滚保留，这条没动。

结论：删除 Catalog/Profile 的 watermark 与 `certificationSequence`。Catalog 的 `sequence` 字段保留为信息性字段（服务端递增，便于人排查），但不作为接受/拒绝判据。CI 可能被重放旧 Catalog 的代价被显式接受，写进架构文档。

## 二、保留项

这些决策不冲突，继续有效。删除动作不连带它们。

| 保留内容 | 出处 | 现状 |
| --- | --- | --- |
| 签名端点即服务身份：端点写在签名文档里，客户端按签名文档找服务 | 0020 §2 | 有效。Runtime Profile 的 `directoryBootstrap.entryUrls` 就是签名端点 |
| 服务与域名一一对应，拓扑无关：publish 写面、update-internal 读面、update 人页各有一个域名；服务实例是否与签名端点同机属部署细节，客户端配置不得表达，不得据此 fail closed | 0020 §2 | 有效。任何同机/同 IP 校验都会在分机部署或换机迁移当天误伤 |
| 签名端点 = 消费方实测可达的 scheme + 端口（按可达性签，不按"域名听起来该是 https"签） | 0020 §2 过渡态第 3 条 | 有效。内网三端点统一 http:80 的教训应固化 |
| 控制面直接断代，不提供兼容 reader | 0019 §1 | 有效，且本次推得更彻底 |
| 必须稳定的是协议合同，不是文件与命令 | 0019 §2 | 有效。四类合同收敛为 Updater IPC + RUP Schema Set + ArtifactSet + Publish Protocol，其中窗口收敛为单值 |
| 发布事务：关键路径失败不得报告成功；非关键投影可异步重建 | 0019 §5 | 有效，实现保留 |
| ArtifactSet 只声明制品，producer 不写 staged/index/签名 | 0021 §1 | 有效，`relkit.artifact-set/1` 保留 |
| 平台不对称合法（Windows 有 installer+payload，macOS 仅 installer），不强迫全平台同构 | 0021 §1 规则 6 | 有效 |
| 声明式 Component Registry 是唯一登记表 | 0021 §4 | 有效，`internal/vnext/registry` 保留供 `internal/build` 与 `relkit build` 消费 |
| `relkit.extension/1`：argv 启动、版本化 JSON IO、超时、环境白名单；禁止 hook 与进程内插件；extension 不得修改 staged/index/签名 | 0021 §5 | 有效 |
| core 与 registry 不得按仓库身份分支 | 0021 §6 | 有效，且是本次范围收窄后仍要守住的底线。`registry_test.go` 里有专门的守卫用例 |
| 分层认证中的合同 conformance | 0022 §6.1 | 有效，`conformance/` 保留 |
| 结构化诊断 envelope（稳定 code、severity、stage、evidence），禁止默认 `unclassified` | 0022 §7 | 有效 |
| 单引擎决策：更新状态机只实现一次 | 0010 | 有效，本次把各语言厚实现全部删除 |
| payload 整文件内容寻址，不做包内差分 | 0013 | 有效 |
| 宿主仓不保存 Relkit 版本、服务 URL、Token、SSH 或私钥 | 0019 §1、0020 §1 不包含清单 | 有效，且字段清单进一步收窄到四个字段 |
| 空机部署与断代部署用同一套部署流水线，内外网形态完全一致，只有地址与根密钥实例不同 | 本次需求 12.7 | 新立，来自本次 |

## 三、驳斥后的可检索性

需求 14 M11 要求：检索 `candidate root`、`distribution root`、`resolution`、`production escape`、`certification` 时，只命中本文、历史记录与被标 Superseded 的 ADR 正文，不命中现行代码与现行文档。

本文是这些概念在现行文档里的唯一载体。新增代码或现行文档提到这些词时，只能是在说明"它已被删除、不要再加回来"。

## 相关

- [ADR 0023](0023-desktop-updater-epoch.md)：取代 0019-0022 的断代决策
- [`docs/architecture.md`](../architecture.md)：现行架构
- [ADR 0019](0019-vnext-breaking-and-protocol-contracts.md) · [ADR 0020](0020-vnext-catalog-operator-resolution.md) · [ADR 0021](0021-vnext-artifact-runtime-registry.md) · [ADR 0022](0022-vnext-certification-ops-skill.md)：被驳斥的四份原始文档（保留原样，状态已标 Superseded）
