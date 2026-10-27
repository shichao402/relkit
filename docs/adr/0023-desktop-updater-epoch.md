# ADR 0023: 桌面更新断代——单引擎、单根信任、服务器单一事实源

- 状态：Accepted（2026-10-22 用户裁决；代码与宿主迁移按任务清单推进）
- 日期：2026-10-22
- 取代：[ADR 0019](0019-vnext-breaking-and-protocol-contracts.md)、[ADR 0020](0020-vnext-catalog-operator-resolution.md)、[ADR 0021](0021-vnext-artifact-runtime-registry.md)、[ADR 0022](0022-vnext-certification-ops-skill.md) 与本次决策冲突的部分（逐条驳斥见 [驳斥文档](0019-0022-refutation.md)）
- 关联：[ADR 0010](0010-single-updater-engine.md)（单引擎决策的前身，本 ADR 将其推到底）、[ADR 0013](0013-internal-update-payload.md)（payload 形态继续有效）、[ADR 0016](0016-serve-split-store-console.md)（console 只读定位由本 ADR 更新）、[`docs/architecture.md`](../architecture.md)（现行架构，与本 ADR 一致）
- 依据：本 ADR 正文即为完整决策依据；现行实现以 [`docs/architecture.md`](../architecture.md) 为准

## 背景

上一代架构（ADR 0019–0022）解决的是真问题：跨仓版本不对齐、一次 CI 内重复解析漂移、宿主各自 pin Relkit 版本。它的解法是在控制面里建一套完整的"版本选择"体系：宿主仓库持有 resolution（用哪个版本的 relkit、每个文件的摘要）、离线 bundle、`relkitw` 包装脚本、`selectionPolicy`、`requires`/`provides` 能力向量、candidate/certified 两级认证、fleet 认证登记，以及一套自建的 Go 源码审计工具（production escape）来检查有没有人绕过这些规则。

这套体系的每一层单独看都有理由，加起来却构成一个持续自我膨胀的结构：

- **四把信任根**：Distribution Root、每产品一把 Product RUP Root、Runtime Profile Root、Candidate Root。它们的私钥全部放在同一台 agent 服务器上，服务器被攻破时一起泄露，分开放不产生任何隔离效果，只增加管理面。
- **两套签名层**：根签 Profile/Catalog，产品密钥再签 directory/index/manifest。第二层签名验证的是"这份文档属于这个产品"，而这件事 index 路径本身已经表达了。
- **宿主挑版本**：因为宿主挑版本，才需要 resolution 把选择结果冻结下来；因为要冻结，才需要 bundle 把资产离线带过去；因为要展开，才需要 materialize；因为要验证据，才需要 certification。删掉"宿主挑版本"这一个前提，后面整条链失去存在理由。

同时实际操作面已经收窄：现役客户端只有 Windows 与 macOS 桌面。移动端、沙箱、Linux 从未有真实使用者，只为它们准备的 selector 取值与设计余量长期空转。

本 ADR 是把这套结构一次性拆掉，并正式声明新的边界。

## 决策

### 1. 范围：只做 Windows/macOS 桌面更新

relkit 的服务对象是桌面应用的检查更新、下载、安装、重启。

范围外平台清单（明确不支持，不保留设计余量）：

| 平台 | 状态 | 说明 |
| --- | --- | --- |
| Linux | 范围外 | 从未有现役宿主；安装布局、提权模型、包管理器集成均未验证 |
| Android | 范围外 | 曾经的 server/client 共用版本号场景已消失 |
| iOS | 范围外 | 同上，且应用商店规则与自更新模型不兼容 |
| 移动端沙箱环境 | 范围外 | 无独立安装目录与重启控制权 |
| Web / 浏览器内自更新 | 范围外 | 由浏览器自身负责 |

范围内平台：`windows`、`macos`。selector 与 target 维度中只为范围外平台存在的取值一并删除。

### 2. `relkit-updater` 是唯一引擎，各语言只有薄 IPC 客户端

更新状态机只实现一次：`cmd/relkit-updater` + `internal/updater`。宿主与 relkit 之间唯一的客户端契约是 `api/updater/v1` protobuf 帧 IPC（4 字节大端长度前缀 + protobuf 载荷）。

各语言只保留薄客户端，职责限于：找到引擎可执行文件、启动子进程、编解码帧、把事件交给 UI。薄客户端不做：解析 bootstrap、解析 index/manifest、验签、算摘要、决定版本、管理安装目录。

| 宿主 | 语言 | 薄客户端位置 |
| --- | --- | --- |
| SvnMergeTool | Dart | `sdk/dart/lib/src/updater_facade.dart` |
| Loom-Launcher | Rust | `sdk/rust/src/lib.rs` |
| Loom | TypeScript | `bindings/ts`（仅生成类型，不承载调用面） |

Go 侧 facade 在 `internal/updaterfacade`，属仓内实现，不对外发布。`sdk/node` 删除：没有现役 Node 宿主。

IPC 窗口从 `[3,3]` 抬到 `[4,4]`。断代不迁移旧宿主，窗口不兼容时明确报 `UPDATER_TOO_OLD` / `UPDATER_TOO_NEW`，不做兼容分支。

### 3. 不考虑存量版本：断代，不迁移

删除所有兼容路径、双写、过渡分支与旧状态迁移。判据：如果一段代码存在的唯一理由是"让旧版本还能用"，它就被删除。

具体包括：`relkit.lock.json`、`.relkit/` 下的指纹与 resolution 文件、宿主仓内的 `relkitw`、`relkit-ops bundle install`、`control-plane/*-candidate*.zip`、`legacy-lastResult.json`、`update_apply.json` 双写、facade 调度器、旧 field 客户端兼容 reader。

同时删除维护这套结构的控制面元数据：`selectionPolicy`（含 `latest-compatible`）、`requires`/`provides` 能力向量、`certificationSequence`、Catalog 生命周期状态机、Catalog/Profile 的 watermark、fleet 认证登记、production escape 审计。

### 4. 每台服务器一把根，只证明"这是这台服务器发出的、没被改过"

每台 agent 服务器各有一把 Ed25519 根密钥对。私钥只在这台服务器上（加离线备份），开发机与 CI 不持有，不同服务器之间不共用。

- 内网服务器（tencent-internal）沿用 `relkit-candidate-2026b` 的密钥材料，改名为内网服务器根，去掉"候选"含义（指纹不变，仍是 `sha256:3dc1ccb4…bf33`）。
- 外网服务器（firoyang.com）在断代部署时生成一把新根。这是新服务器身份，不是"私钥找不到后换根"。
- 每把根的位置登记在凭据清单（见决策 6）。

根私钥签服务器发出的全部文档：Runtime Profile（服务器是谁、端点在哪）、Catalog（提供哪些 relkit 组件和摘要）、各产品的 index 与 manifest、工具清单与工具文件的 SSH 签名。

根密钥的作用只有一个：证明这份文档/文件是合法服务器发出的、没被改过。它不是任何操作的鉴权凭据（鉴权是 token 的事）。

删除项：Distribution Root `relkit-distribution-2026`、每产品一把的签名密钥（agent 配置的 `privateKeyPath`/`signing` 段、产品密钥生成与轮换、`api/` 里按产品挑公钥的逻辑）、Runtime Profile Root、candidate 与 certified 的二分。

下载的文件本身只做摘要校验，不另外签名。一个应用只认一台服务器的根，不同时信任多把。

**不设计换根流程，不做新旧根重叠窗口。** 只要不丢根私钥就没有换根需求。保管要求：所在服务器上一份、离线备份至少一份且在所有 git 仓库之外，位置登记在凭据清单；自检脚本逐把做签名往返确认归属；agent 启动时校验所载私钥与登记的本服务器根公钥一致，不一致拒绝启动。

已接受的代价：换根意味着需要换根的服务器上所有已装应用都失去信任链，必须重装才能继续接收更新。缓解手段只有妥善保管，没有技术兜底。

### 5. token 只表示身份和权限

token 回答一个问题：这个宿主项目能在 agent 上操作哪些产品。

- 服务器只存 token 的 `hex(sha256)`，明文只在签发时显示一次。
- 每条凭据绑定唯一产品，`Credential.Allows(product)` 没有放大规则，越权返回明确错误并同时点名两个产品名。
- token 不承担任何验证服务器或验证内容的作用。验证服务器只靠根公钥。
- 宿主 CI 只需要注入这一个 token。下载 relkit 组件不需要 token，也不需要 mirrors 凭据（内容由根签名保证）。

签发、轮换、吊销都在管理面板完成。`relkit-agent init -product` / `-token-only` / `-share-with` / `-list-products` / `-remove` 这套命令行入口删除；`init` 只剩 `-config`、`-bootstrap`、`-reset`。

### 6. 凭据清单是代码取钥匙的唯一入口

代码与脚本不再知道任何钥匙放在哪，只知道 keyId。要用钥匙时拿 keyId 问清单，清单告诉它从哪个文件或哪个环境变量读。

清单 schema `relkit.custody-manifest/1`，每条登记 keyId、种类（`root` / `token` / `mirror-credential`）、用途、载体（`file` / `env`）、路径、公钥指纹。清单本身不存任何秘密。

三个 scope，格式相同、读取代码相同：`server`（服务器上一份，根私钥与 token 存储位置）、`host-ci`（随项目配置提交，只写 keyId 与环境变量名）、`archive`（离线备份处）。

启动自检：清单里有登记、载体可读、私钥与登记的公钥指纹一致（签名往返）。任一不满足立即退出，报出 keyId 与原因，不降级、不跳过。

Go 程序用共享包 `internal/custody`；Python/shell 脚本调用 `relkit custody get` / `check` / `public` 取值，不各写一份解析。代码与配置中不再出现密钥路径或凭据环境变量名。

token 的具体实例以管理面板数据为准，清单只登记 token 存储的位置，不逐个登记 token。

### 7. 组件从服务器取，服务器是唯一事实源

宿主项目只持有四样东西：服务器地址、根公钥（公钥本身，不只是指纹）、产品名、token（配在 CI 密钥里）。

宿主 CI 需要 relkit 工具时：向服务器下载工具与工具的根签名 → 用项目里的根公钥验签 → 验签通过才执行。验签失败立即失败，不降级到 GitHub、mirrors 或其他来源。

验签用构建机自带程序：服务器为每个工具额外产出一份 SSH 签名（namespace 固定 `relkit-tool`），CI 用 `ssh-keygen -Y verify` 对照 `allowed_signers` 文件校验。这一步固定为"下载、验签、执行"三条命令，脚本内不含版本号、文件名、摘要或 bundle 地址，因此可以写进 yaml 并冻结。

工具启动后再从同一服务器取 Profile 与 Catalog，同样用根公钥验签。哪个 relkit 版本是当前版本由服务器 Catalog 的 `current` 字段决定，宿主不挑版本。

他语言不再需要"下载 relkit SDK"这一步。SDK 就是薄客户端源码，随宿主仓库走；不再有 `.relkit/components/` 物化目录、`third_party/relkit` 树、module H1 或 Release asset 摘要。

已接受的代价：宿主 CI 构建机不保留任何状态，理论上人可以重放一份旧的、签名有效的 Catalog，让 CI 用上旧版工具。最终用户不受影响（updater 对产品 index 的防回滚保留）。这个代价被接受，因为引入 watermark 会让 CI 持有状态，而 CI 持有状态正是上一代"修了又坏"的来源之一。

### 8. agent 管理面板

`relkit-console` 提供用户、角色、宿主产品三类管理功能。

- 用户：登录、新增、删除、改密。首个管理员沿用一次性 bootstrap 令牌流程。
- 角色：管理员（管理用户与全部产品）、产品负责人（只能管理被指派产品的 token）。
- 宿主产品：登记产品并签发首个 token（不生成产品密钥）；面板给出一段可复制的接入配置（服务器地址、根公钥、产品名），token 单独显示一次；删除产品走整产品下线操作（版本、catalog 条目、人页、token）。
- 面板的写操作全部经 agent 管理接口 `/v1/admin/*` 执行，面板进程不直接写存储。

ADR 0016 中"console 只读"的定位由本 ADR 更新：console 仍是独立二进制、仍不持有存储写权限，但面板的写操作可以经 agent 管理接口完成。这是"读存储"与"写存储"的分离，不是"只读进程"与"可写进程"的分离。

### 9. Dec 退出，cronkit 视同从未接入

Dec 改为在线服务，不再有客户端版本，不参与 relkit 更新链。

- 外网发布服务器上的 Dec 产品及全部在线资产删除。
- 断代后宿主只剩 SvnMergeTool、Loom、Loom-Launcher 三个。
- relkit 仓库内的 `DecAssets` 是 Skill 资产，与 Dec 更新产品无关，保留；`dec-publish.yml` 发布的是这些资产，同样保留。

cronkit 只在历史文档与测试样例中出现，从未进入生产接入面。按"从未接入"处理：样例改名或删除，不为它保留任何兼容分支。

### 10. 文档定档方式：架构文档 + ADR，不做自动检查

本次定下的信任与凭据模型写进 [`docs/architecture.md`](../architecture.md)。这份文档记的是"现在长什么样、哪些规矩不能破"；ADR 记的是"当时为什么这么定"。

不新增任何检查这些规矩的 CI 步骤或守卫脚本。理由：规则写得严，正常改动也被拦；写得松，又拦不住；而检查本身会变成新的拦路虎。规矩靠评审遵守。

要破规矩（引入新的签名层、新的密钥种类、新的工具来源）时，先写 ADR 废止或修订本 ADR，再改架构文档。两者不得矛盾。

## 否决

- 保留移动、沙箱、Linux 的设计余量
- 保留每产品一把签名密钥或第二层签名文档
- 保留 candidate/certified 二分与 fleet 认证
- 保留 window 化的换根流程与新旧根重叠窗口
- 让宿主仓库持有 relkit 版本、组件摘要或 resolution
- 让 CI 持有 watermark 或任何跨构建状态
- 让 token 承担验证服务器或验证内容的职责
- 用 CI 检查脚本约束架构规矩
- 为单宿主保留兼容分支
- relkit 仓库内按 Loom / Launcher / SvnMergeTool / Dec 仓库身份分支

## 后果

- 控制面只剩一条路径：服务器签 Profile/Catalog/index/manifest/工具，宿主 CI 验签取工具，updater 验签取更新。删除量远大于新增量。
- 密钥管理面从四类根收敛到每服务器一把。加一台服务器 = 生成一把根 + 登记清单 + 离线备份。
- 宿主接入从"读 resolution 再物化"变成"下载、验签、执行"三步，项目配置只剩四个字段。
- 引入新签名层、新密钥种类或新工具来源的成本从"改一段代码"变成"写一份 ADR 并改架构文档"，摩擦从流程后端的 CI 移到决策前端的评审。
- 已接受的代价两条：换根要已装应用重装；CI 可能被重放旧 Catalog（最终用户不受影响）。
- 现役宿主三个；Dec 退出，cronkit 视同从未接入。新增宿主只需按薄客户端规则接入，不改引擎。

## 完成标准（本 ADR）

能唯一回答：

1. relkit 支持哪些平台，为什么 Linux 不在范围内？
2. 更新逻辑写在哪一层，各语言客户端负责什么、不负责什么？
3. 为什么这次不做任何向后兼容？
4. 一台服务器有几把根，它签什么、不签什么，丢了怎么办？
5. token 与根密钥各管什么，能不能互相替代？
6. 宿主 CI 从哪拿 relkit 工具，怎么确认拿到的是对的？
7. 想加一个新的签名层或新的密钥种类，第一步该做什么？
