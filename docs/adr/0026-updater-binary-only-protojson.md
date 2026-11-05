# ADR 0026: 只交付 relkit-updater 二进制，契约用 ProtoJSON 收敛

- 状态：Proposed（草案，待用户裁决）
- 日期：2026-11-04
- 关联：修订 [ADR 0023](0023-desktop-updater-epoch.md) 决策 2（各语言薄 IPC 客户端）；延续 [ADR 0010](0010-single-updater-engine.md) 单引擎原则与 [ADR 0003](0003-protobuf-v2-wire-format.md) 的 protobuf 契约

## 背景

ADR 0023 把更新逻辑全部收进 `relkit-updater`，宿主只留“薄 IPC 客户端”：Dart 版 `sdk/dart/lib/src/updater_facade.dart` + `frame.dart`（约 290 行），Rust 版 `sdk/rust/src/lib.rs` + `json.rs`（约 1100 行），TS 只有 `bindings/ts` 生成类型。为防手写 facade 与契约漂移，仓库另有 `cmd/relkit-facade-gen` 和 `conformance/updater/facade-signatures.txt` 门禁。

三个问题：

1. 手写层是风险源。类型本身由 proto 生成，是强类型的；真正会漂的是每种语言手写的进程管理、帧编解码和调用面。要用专门的签名门禁兜它，说明它本不该存在。本项目由 agent 主导，一次契约改动要同步碰的地方越多，漏改的概率越大。
2. 协议形态早已是“一进程一请求”。`cmd/relkit-updater/main.go` 从 stdin 读一帧 `UpdaterRequest`，处理完写出若干 `UpdaterEvent` 帧后退出，没有长会话。保留二进制长度前缀帧，只是让每种宿主多写一份拆包代码。
3. 自更新阶段对用户是静默的。apply 被接受后，引擎从 staging 拉起 `--worker` 进程，宿主退出；worker 等宿主退出（最长 5 分钟）、拷贝或切换版本、重启宿主，进度只写进 dataDir 的 session 文件。此时宿主与 SDK 都已不在，任何客户端都画不出这段进度。SvnMergeTool 为此吃过一次真实事故：点“立即安装”后程序退出、屏幕上什么都没有，用户手动重开，新进程锁住安装目录，更新永远失败。宿主只能补一个 `update_handoff_screen.dart` 让路界面来兜底。

强类型必须保留：契约只有一个来源，两侧都从它生成，不一致在编译期暴露。要去掉的是“relkit 维护的各语言客户端”，不是类型。

## 决策

### 1. 交付物只剩引擎二进制

relkit 对宿主只交付 `relkit-updater`（随工具分发，验签规则不变，见 ADR 0023 §5）。删除：

- `sdk/`（`dart`、`rust`、`README.md`）
- `bindings/ts`
- `cmd/relkit-facade-gen`、`conformance/updater/facade-signatures.txt`
- `internal/updaterfacade`（仓内无调用方）
- `internal/ipc` 的长度前缀帧

`internal/sdk/doc.go`、`internal/inprocess/updater.go` 中指向 `sdk/updaterfacade` 的注释一并改正。

### 2. 契约唯一来源：proto，引擎自带

`proto/updater/v1` 升为 `proto/updater/v2`（package `relkit.updater.v2`）。宿主可见的 proto 编译进引擎，`relkit-updater proto` 原样打印。宿主 CI 用当次下载并验签过的二进制导出 proto，再 `buf generate` 生成本语言类型。

proto 与引擎版本因此天然一致：宿主拿到的类型永远对应它要调用的那份二进制，不存在“SDK 版本”与“引擎版本”两条线。

### 3. 传输：ProtoJSON，一进程一请求

```text
stdin : 一个 ProtoJSON 编码的 UpdaterRequest（读到 EOF）
stdout: 每行一个 ProtoJSON 编码的 UpdaterEvent（JSON Lines）
stderr: 仅人读诊断，不属于契约
exit  : 0 = 已输出终态事件；非 0 = 引擎自身故障，终态事件可能缺失
```

只有一个请求消息、一个事件消息。不为每个操作另设命令行参数：命令行参数是第二套无类型接口面，与收敛目标相反。命令行只保留引擎内部用途（`--worker`、`proto`、`--version`）。

宿主侧所需代码只剩：启动进程、写一个 JSON、按行读、用生成类型反序列化。每种语言的标准 protobuf 运行时都有 ProtoJSON 实现（Go `protojson`、Dart `mergeFromProto3Json`、Rust `pbjson`、TS `@bufbuild/protobuf`），不需要 relkit 写任何东西。

### 4. 严格性：宁可拒绝，不可宽容

- 两侧反序列化一律拒绝未知字段（各运行时默认行为；宿主不得开 ignore unknown）。字段对不上是断代信号，必须当场失败，不能“多出来的忽略”。
- `UpdaterRequest.protocol` 必填，取值 5。引擎不等于 5 时只输出一行 `Failed{UPDATER_TOO_OLD | UPDATER_TOO_NEW}` 后退出。无协商、无兼容窗口，延续 ADR 0023。
- 每次运行的第一行事件固定是 `Capabilities`，带 `protocol` 与 `engine_version`，宿主先校验再读后续事件。
- 枚举以名字编码（ProtoJSON 默认），日志与 conformance 样例人可读，不会出现魔法数字。

### 5. 收窄宿主可见面

v2 只放宿主真正读写的消息。引擎内部落盘结构移到 `proto/updater/internal/v1`（package `relkit.updater.internal.v1`），不随 `relkit-updater proto` 导出：`UpdatePlan`、`PlannedFile`、`ArtifactTarget`、`PersistedState`、`ApplySessionRecord`、`JournalEntry`、`ApplyJournal`，以及整个 `payload.proto`。

宿主可见面同时删掉：

- `ClientProfile` 里与 bootstrap 重复的 `product`、`allowed_channels`、`entry_urls`、`index_urls`、`fallback_urls`、`trusted_keys`：bootstrap 改为必填，引擎是唯一解析者（ADR 0023 §5 已如此要求，v2 把“可以不传 bootstrap”的口子关掉）。
- `ClientHello`（由 `protocol` 单字段取代）。
- `CancelOp`：现实现只回 `Ok`，什么也不取消。一进程一请求下，取消就是给进程发中断，引擎已用 `signal.NotifyContext` 处理。

保留的操作：`check`、`skip`、`download`、`apply`、`status`、`cleanup`、`listInstalled`、`deleteInstalled`、`switchActive`、`rollback`。后两个现役宿主暂未调用，保留给“回到上一版本 / 切换已装版本”的用户入口（2026-11-05 用户裁决）。

### 6. 交接阶段的进度窗口归引擎

`ApplyOp` 新增 `show_progress`（默认 true）。worker 进程在宿主退出后自己显示一个小窗口，覆盖宿主看不到的全部阶段：

| 阶段 | 窗口内容 |
| --- | --- |
| `WAITING_FOR_EXIT` | 正在等待 <产品名> 退出 |
| （plan 未下载时）下载 | 进度条，字节数与速率 |
| `COPYING` | 正在安装 <版本> |
| `RELAUNCHING` | 正在启动 |
| `NEEDS_ATTENTION` / `ROLLED_BACK` | 失败原因与 `RecoveryHelp` 链接，带关闭按钮 |

规则：

- `ApplyOp` 允许 plan 尚未下载：worker 先下载再交接。宿主最简流程因此只剩两步：`check` 得到 `UpdateAvailable`，用户确认后发 `apply` 并立刻退出。需要后台预下载的宿主（如 Loom-Launcher 暂存启动器包）照旧先发 `download`。
- 窗口只覆盖宿主不在的阶段。宿主仍在运行时的前台下载进度由宿主自己画，引擎不弹窗（2026-11-05 用户裁决）。
- 窗口只显示、不提供“取消”以外的操作，与 SvnMergeTool 让路界面“不给留下来的机会”的原则一致。安装开始拷贝后不可取消。
- 窗口期间仍有宿主实例被手动打开：宿主启动时发 `status`，看到 `active_session` 处于非终态就自行退出。这是宿主唯一必须实现的交接行为，契约里写死。
- full-install（安装包交接）：窗口只覆盖等待宿主退出这一段，之后交给安装程序自身的界面。
- 产品显示名取自 bootstrap，界面文案随系统语言在 zh/en 之间选择。

平台实现：

- Windows：`golang.org/x/sys/windows` 直接调 Win32（窗口、进度条控件），无 cgo，交叉编译不变。
- macOS：AppKit 需要 cgo，`relkit-updater` 的 darwin 产物改由 GitHub Actions macOS runner 编译（2026-11-05 用户裁决），产物仍走现有工具发布与验签流程上传服务器。macOS 是整个 `.app` 当 payload 替换，耗时短，第一版只做等待与安装两个阶段的面板。

### 7. conformance

`conformance/updater/*.json` 已是 ProtoJSON 样例，改为两条门禁：

1. 引擎对每个样例场景的实际 stdout 逐行与样例一致（逐字节比较规范化后的 JSON）。
2. 每个样例都能用 v2 生成类型在严格模式下反序列化，再序列化回来不变。

宿主仓库可以直接复用这批样例测试自己的解析，不再有 facade 签名门禁。

## 迁移

断代一次，不留兼容分支（ADR 0023 原则 2）：

1. relkit：proto v2 与 internal 拆分、引擎改 ProtoJSON 传输与 `protocol` 校验、`relkit-updater proto` 子命令、conformance 改写、删除第 1 节所列目录。发布新引擎。
2. 进度窗口：Windows 先行，macOS 随 macOS 构建机就绪跟进；在此之前 `show_progress` 在 macOS 上是无操作。
3. SvnMergeTool（Dart）：删 `rup_client` 依赖与 `.relkit/components/sdk/dart` 物化；CI 加“导出 proto → buf generate”；`update_service.dart` 改成启动进程 + 按行解码；启动时 `status` 检查替代现有让路逻辑的判定来源（界面可保留）。
4. Loom-Launcher（Rust）：删 `relkit-updater` crate 路径依赖与 `.relkit/components/sdk/rust`；`build.rs` 用导出的 proto 跑 `prost-build` + `pbjson-build`；`updater.rs` 改调用方式。
5. Loom：无变化（不持有 updater 客户端）。
6. relkit-ops skill 中的宿主接入合同（`host-update.md`）按本 ADR 重写。

步骤 3、4 必须在新引擎发布后同批完成：新旧两侧会以 `UPDATER_TOO_OLD/NEW` 互相拒绝，不会半工作。

## 代价

- 宿主每种语言要自己写几十行“启动进程 + 读行”的代码。这部分不涉及契约，生成类型覆盖了所有会出错的字段层面。
- ProtoJSON 比二进制帧大，`Progress` 事件频繁时略多 CPU。事件量很小（每次运行几十到几百行），可忽略。
- macOS 产物需要 macOS 构建机。
- 再断代一次，两个现役宿主各改一次。

## 已裁决（2026-11-05）

1. `SwitchActiveOp`、`RollbackOp` 保留。
2. 宿主在运行时的前台下载进度不归引擎窗口。
3. darwin 产物由 GitHub Actions macOS runner 构建。
