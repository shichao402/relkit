# ADR 0027: 内容轴补上软链与密封包，macOS 同时支持 LIBRARY 与整包 IN_PLACE

- 状态：Accepted，已实施（第五版，2026-11-05：第 0–3 步实现于 `cb03d6d`、`2276d82`，第 4–7 步已完成，2026-11-06 进入 stable；macOS LIBRARY 形态的真机验证尚未做，目前没有产品使用。本版按实现修正 schema 类型、`controlDir` 判定、换名恢复、脚本工作目录、`ExecutableRelpath` 基准与对账写的软链父目录。第四版裁决不变：整包交换 commit 即删旧包、`active.json` 只加 `bundle` 字段、launcher bundle id 用 `<id>.launcher`、Loom 作为托管库特例）
- 日期：2026-11-05
- 关联：修订 [ADR 0013](0013-internal-update-payload.md)（软链禁令）、[ADR 0014](0014-placement-and-version-library.md)（“macOS .app 选 bundle 根”一句的落地方式）；延续 [ADR 0010](0010-single-updater-engine.md) 单引擎、[ADR 0026](0026-updater-binary-only-protojson.md) 强类型契约；同步 `docs/design/install-layouts.md` §7、§8、§11

## 背景

### 现状是死锁

macOS 宿主目前没有任何一条能走通的应用内更新路径：

| 路径 | 卡在哪 |
| --- | --- |
| FULL_INSTALL（交 DMG/安装器） | `fullInstallRefused(goos=="darwin")` 拒绝，提示 “ship the .app as a payload” |
| payload IN_PLACE | `payload.Build` 遇到软链直接报错；Flutter `.app` 内 framework 有 30 个软链（`Versions/Current -> A` 等），打不出包 |
| payload LIBRARY | 宿主侧从未接上：`resolveHostPlacement` 只在 Windows 返回 LIBRARY；引擎侧 `.app` 根识别已删，只剩死代码 |

SvnMergeTool 于是只能发 DMG（其 adr-009 第 5 条“macOS 只发裸 .dmg，只 --install，不做 payload”），而 relkit 又要求 macOS 必须走 payload。

### 来龙去脉

- `cafec53` 一代：引擎曾能识别 `.app` 根（`findAppBundle` / `looksLikeAppBundle`），macOS 走过版本化。
- `79be2be`：所有平台统一成逐文件 payload（ADR 0013）。这次重构顺手做了两件没有写理由的事：`Build` 禁软链；删掉 `.app` 根识别。后者在 `applyLibraryContent` 留下一段永不执行的分支（`content.filesRoot` 恒为 `<StagedRoot>/payload/files`，不可能以 `.app` 结尾）。
- `b275be3`：SvnMergeTool macOS 只出 DMG。
- `b9da050`：relkit 拒绝 darwin FULL_INSTALL，要求 payload。

禁软链没有安全以外的理由。安全上的顾虑是真实的（软链可把写入导向安装根之外），但它要的是“受约束的软链”，不是“没有软链”。framework 的软链是 codesign 要求的结构，压平会让签名失效。

### 两种形态都需要

- 版本化（LIBRARY，Fusion 360 / JetBrains Toolbox / Unity Hub 式）：入口 + `versions/<id>/`，`active.json` 切换。优点是不必等宿主退出即可装好新版本、可多版本并存（SvnMergeTool `retain=2` 正是为多实例）、回滚是改指针。`install-layouts.md` §8 早已为 macOS 设计过这套，只是没接上。
- 整包替换（IN_PLACE 的 bundle 形态，Sparkle 式）：存量用户手里是 `/Applications/X.app` 一个裸包，没有入口也没有 library。它们需要一条不必先手动重装就能升级的路，也是没有 launcher 的产品的常态。

结论：两者是 Placement 轴上的两个取值，不是互相替代。缺的不是新的 Placement，而是 Content 轴表达不了“一个带软链的密封包”。

## 设计原则（本 ADR 不得破坏）

1. Content 与 Placement 两根轴正交（ADR 0014）。新增能力落在 Content 轴，Placement 仍只有 IN_PLACE / LIBRARY；四种组合都要有明确定义，不允许“某平台某轴偷偷特判”。
2. 唯一 Go 引擎（ADR 0010）。软链校验、签名校验、交换、回滚全部在引擎里，宿主不写 post 脚本补结构。
3. 强类型、收敛（ADR 0026）。新概念以 proto 字段/枚举表达，不靠文件扩展名、目录名或约定字符串推断；能复用现有机制（selector 协商、journal、active 指针）就不新造。
4. 老客户端安全。新格式对老引擎不可见，靠 ADR 0013 已有的 selector 隔离，不靠版本号比较。

## 决策

### 1. 软链成为文件表的一等条目（所有平台同一套规则）

`FileEntry` 增加类型与目标：

```proto
enum EntryKind {
  ENTRY_KIND_UNSPECIFIED = 0; // 读作 FILE，兼容 /1 表
  ENTRY_KIND_FILE = 1;
  ENTRY_KIND_SYMLINK = 2;
}

message FileEntry {
  string relpath = 1;
  int64 size = 2;      // SYMLINK 为 0
  string sha256 = 3;   // SYMLINK 为空
  uint32 mode = 4;     // SYMLINK 忽略
  EntryKind kind = 5;
  string link_target = 6; // 仅 SYMLINK；原样记录 readlink 结果
}
```

zip 里不存软链（zip 的软链语义各实现不一致），软链只存在于 `files.pb`，解包和落盘时由引擎创建。`inspect` 的“zip 条目与表一一对应”改为“与表中 FILE 条目一一对应”。

安全规则在 `Build`、`inspect`、落盘三处执行同一个纯函数 `payload.CheckSymlinks(table)`：

1. `link_target` 非空、是相对路径、不含反斜杠、不含 NUL。
2. 以链接所在目录为基准做词法解析（`path.Clean(path.Join(dir(relpath), target))`），结果不得为 `..` 或以 `../` 开头，即不越出内容根。
3. 解析结果必须是表内某个 FILE/SYMLINK 条目，或某个条目的祖先目录（隐含目录）。悬空链接拒绝。
4. 任何条目的 relpath 都不得经过一个 SYMLINK 条目（`Versions/Current/foo` 这类路径不允许出现在表里；真实文件只能以 `Versions/A/foo` 出现）。这保证写入永远不会穿过表内软链。
5. 链式软链允许，但解析深度上限 40，且不得成环。
6. Windows 引擎遇到含 SYMLINK 的表一律拒绝（`LAYOUT_UNSUPPORTED`），stage 阶段对 Windows 选择器的 artifact 同样拒绝。Windows 上创建软链需要特权或开发者模式，且 Windows 产品不需要它。

落盘加固（同时适用于现有 IN_PLACE，算顺手补洞）：写每个条目前，检查它在 `targetRoot` 之下的每一级父目录都是真实目录（`Lstat`，不是软链也不是文件），防止磁盘上已有的软链把写入导出安装根。对账写里，挡路的父路径若是上一版基线拥有且未被改动的条目（例如上一版把 `lib` 发成软链、这一版改成目录），先按 §4.1 挪进备份再写；不属于基线或已被改动的，报 `LAYOUT_UNSUPPORTED`，绝不穿过它写入。全新写的 `P` 是空目录，只会遇到表内条目，表规则（第 4 条）已保证不冲突。

`preserve` 列表中的路径不得是 SYMLINK 条目或位于其下。

### 2. 内容单元：TREE 与 BUNDLE，由发布方显式声明

```proto
enum ContentUnit {
  CONTENT_UNIT_UNSPECIFIED = 0; // 读作 TREE，兼容 /1 表
  CONTENT_UNIT_TREE = 1;        // 开放安装目录：可逐文件增删，可 preserve
  CONTENT_UNIT_BUNDLE = 2;      // 密封包（macOS .app）：只能整体替换
}

message FileTable {
  string schema = 1;           // "relkit.payload/1" 或 "relkit.payload/2"
  repeated FileEntry files = 2;
  repeated ScriptEntry scripts = 3;
  repeated string preserve = 4;
  ContentUnit unit = 5;
  string bundle_name = 6;      // 仅 BUNDLE，如 "SvnAutoMerge.app"；单段、以 .app 结尾
}
```

`FileTable` 属于 `enginestate`（引擎私有），不是宿主契约。本 ADR 不改 `proto/updater/v2`，宿主仓的 proto 副本与逐字节比对（ADR 0026）不受影响。

- 表内 relpath 在 BUNDLE 下相对于 bundle 根（`Contents/...`），不带 `bundle_name` 前缀。这样同一张表在两种 Placement 下都不用改写。
- `relkit payload build --bundle <X.app>`：显式开关，不按扩展名推断。构建时校验 `Contents/Info.plist` 存在、`bundle_name` 合法，并拒绝 `--preserve`（密封包内不允许有可变数据）。
- BUNDLE 只允许在目标系统为 darwin 的 artifact 上出现；stage 闸门对其他目标拒绝。目标系统取 `os` 选择器，未声明时取 `os-arch` 的 GOOS 段（定向构建的规范维度，如 `darwin-universal`）；两者都声明时必须一致（`payload.TargetOS`）。含软链的表不得以 windows 为目标，判定同此。

为什么不复用 TREE + “根目录名以 .app 结尾”：那正是 `79be2be` 之前的推断式做法，会被一次重构顺手删掉而无人察觉。显式枚举让“这是密封包”成为契约的一部分，测试和 stage 闸门都能看见。

### 3. 格式版本与协商：payload/2 + selector 隔离

- 只要表里出现 SYMLINK 条目或 `unit=BUNDLE`，`schema="relkit.payload/2"`（`payload.RequiredSchema` 计算，`Build` 写入、`inspect` 校验两者一致）；否则仍为 `relkit.payload/1`，字节级不变（Windows 产物完全不动）。
- 发布时 artifact 带 selector `apply=relkit-payload/2`。
- 新引擎 `withEngineDefaults` 的 apply 偏好从单值改为有序列表 `[relkit-payload/2, relkit-payload]`，`selectors.prefersApply` 按序挑选。
- 老引擎只认 `apply=relkit-payload`，匹配不到 `/2`，回落到完整安装 artifact：在 macOS 上即提示手动下载 DMG（老引擎本就拒绝 darwin FULL_INSTALL，表现为“需手动更新”，不会装坏）。
- stage 闸门（`internal/stage/stage.go`“去掉 apply 后需有同 selectors 的完整安装 artifact”）原样适用于 `/2`，因此 macOS 发布必须同时有 DMG 与 payload。

这与 ADR 0013 隔离老客户端的机制完全一致，不引入版本比较逻辑。

### 4. 落盘：两种写法 × 两种切换

四格不是四套算法。按“目标目录是否完全归这份内容所有”分成两种写法，按 Placement 分成两种切换：

| Content \ Placement | IN_PLACE | LIBRARY |
| --- | --- | --- |
| TREE | 对账写（4.1） | 全新写（4.2）+ 指针切换（4.3） |
| BUNDLE | 全新写（4.2）+ 换名切换（4.3） | 全新写（4.2）+ 指针切换（4.3） |

写法与切换由同一个纯函数 `layoutFor(unit, placement)` 决定（`UNSPECIFIED` 读作 TREE）：只有 TREE + IN_PLACE 是对账写，其余都是全新写；切换按上表。原因不在 Placement：TREE/IN_PLACE 的安装根是开放目录，里面有 launcher、`active.json`、用户文件和 preserve 条目，不归这份内容独占，只能逐文件对账；另外三格的目标（一个版本目录或一个密封包）完全由这份内容定义，可以整体新建再切过去。

BUNDLE/IN_PLACE 可以看成“容量为 1 的版本库”：指针是 `.app` 路径本身，切换是原子换名，不保留旧版。

#### 4.1 对账写（仅 TREE / IN_PLACE）

保持现有 `applyContentTable` 语义：逐文件把旧文件挪进同卷 `.relkit-backup/<session>` 并记 journal、写新文件、基线差分删除（sha 相同才删）、preserve 跳过、post 脚本失败按 journal 回滚、commit 后存基线。新增：

- SYMLINK 条目：落盘为 `os.Symlink`；备份与回滚用 rename，天然覆盖软链本身（不跟随）。磁盘探测一律 `Lstat`。
- 基线条目增加 `kind` 与 `link_target`；删除判定对软链比较 `readlink` 结果，对文件要求是普通文件且 sha 相同，形态变了（文件变软链、软链变目录）一律视为用户改动而保留。
- 父目录检查（§1）：基线拥有且未改动的挡路条目先挪进备份；删除旧条目时若路径经过软链则跳过，不跟随。
- 回滚时被挪开的条目可能是目录，按目录整体还原。

这条路径只服务开放目录。需要整目录替换的 Windows 产品应改用 LIBRARY，而不是让对账写去换整个目录：Windows 上只要有进程打开目录里的任何文件（资源管理器、杀毒、停在该目录的终端），目录就无法改名。

#### 4.2 全新写（TREE/LIBRARY、BUNDLE/LIBRARY、BUNDLE/IN_PLACE 共用）

设最终目标为 `T`：

| 组合 | `T` |
| --- | --- |
| TREE/LIBRARY | `install_root/versions/<id>` |
| BUNDLE/LIBRARY | `install_root/versions/<id>`（包在 `T/<bundle_name>`） |
| BUNDLE/IN_PLACE | `install_root`（即 `.app` 本身） |

设内容根 `C`：TREE 为 `P`，BUNDLE 为 `P/<bundle_name>`（LIBRARY）或 `P` 本身（IN_PLACE，`P` 就是未来的 `.app`）。表内 relpath、脚本工作目录与 `ExecutableRelpath` 都相对于 `C`，与对账写相对于安装根同一语义。

1. 暂存目录 `P = dir(T)/.<base(T)>.relkit-partial`，与 `T` 同卷、隐藏（以 `.` 开头，prune 与列表跳过）。存在残留先删。
2. pre 脚本（工作目录 `C`）。
3. 按表物化到 `C`：写文件、设 mode、建软链、父目录检查。不建 journal、不做备份：`P` 是全新目录，失败直接删 `P`，已安装内容一个字节都没动。
4. post 脚本（工作目录 `C`）。失败删 `P`。
5. 校验：BUNDLE 对包根跑 `codesign --verify --deep --strict`，失败报 `SIGNATURE` 并删 `P`。放在 post 脚本之后，脚本若篡改包内文件会被拦下。TREE 不额外校验（解包时已逐文件核 sha）。ad-hoc 签名也能通过，它验的是“包完整”，不是身份。
6. 写 `P/relkit-version.json`（仅 LIBRARY）作为完成标记。
7. 交给 4.3 切换。

崩溃恢复统一：下次会话发现残留 `P` 直接删除。换名切换的中间态同样由兄弟路径推导，不需要 journal（见 4.3）。

脚本约束：全新写下 pre/post 脚本的工作目录是内容根 `C`（在 `P` 之下），不是最终路径 `T`，引擎不向脚本暴露 `T`。脚本只能按相对路径改动内容，不得记录绝对路径或向系统注册（文件关联、服务、Shell 扩展）；这类工作属于安装器或 launcher。现有产品的 payload 都不带脚本，此约束不破坏存量。

同版本重装（LIBRARY 下 `T` 已存在）：

- `T/relkit-version.json` 的 code 与计划一致：`T` 只可能由第 7 步的原子 rename 产生，因此完整，跳过 1–6，直接切换指针。幂等，不删除任何东西。
- 不一致或无 `relkit-version.json`（外来或残缺目录）：不在用则挪进 TrashDirName 后照常安装；在用则报 `OCCUPIED`。
- 不再先 `RemoveAll(versionRoot)`（现状），那会删掉正在运行的版本，且中途崩溃会留下坏掉的 active 版本。

不新增错误码，宿主契约不变。

#### 4.3 两种切换

指针切换（LIBRARY）：

1. `rename(P, T)`。Windows 上杀毒或索引服务可能短暂持有 `P` 内刚写的文件句柄，导致目录改名失败；有限退避重试（总时长约 5 秒），仍失败则删 `P` 报错，已安装内容不受影响。
2. 非 `install_only` 时写 `active.json`（原子写），`previous` 记旧路径。
3. `pruneVersions`。
4. 不要求宿主退出：不碰任何正在运行的文件。

换名切换（BUNDLE/IN_PLACE）：

1. 前置检查（在 4.2 之前，受理 apply 时做）：
   - 单元按磁盘判定再与表交叉校验：`install_root` 本身是软链 → `LAYOUT_UNSUPPORTED`；`Contents/Info.plist` 是普通文件即 BUNDLE，否则 TREE。表的 `unit` 与之不符（把树装到包上，或反之）→ `LAYOUT_UNSUPPORTED`。不按扩展名推断，也不信任宿主上报：宿主只给 Placement，单元是磁盘事实。判定结果记入会话（`ApplySessionRecord.unit`），后续 worker、锁路径、拉起方式都只读会话。
   - `InstallSpec.preserve` 非空 → `LAYOUT_UNSUPPORTED`；`dir(T)` 不可写 → `PERMISSION_DENIED`（不提权，提示用 DMG）。表的 `bundle_name` 与 `base(T)` 不同不算错（用户可能改过名），以 `T` 为准。
   - 控制文件不得写进 `T`：`.relkit-apply.lock` 与可写探针若放在 `install_root` 下，对 BUNDLE/IN_PLACE 即 `.app` 内部，会破坏封印并随交换被换走。由纯函数 `controlDirFor(unit, placement, root)` 决定：换名切换为 `dir(T)`，锁名 `.<base(T)>.relkit-apply.lock`；其余写法仍为 `install_root`，锁名不变。
2. 等宿主退出（`planRequiresHostExit` 对换名切换恒为 true）。
3. 先 `recoverSwap`，再 4.2 全新写，然后交换：`renamex_np(P, T, RENAME_SWAP)`；非 APFS 或调用失败时回落两次 rename（`T → O`，`P → T`，`O = dir(T)/.<base(T)>.relkit-old`），第二步失败立即把 `O` 改回 `T`，连还原也失败则报错并注明 INCOMPLETE，留给下次恢复。
   恢复不靠 journal，只看三个兄弟路径：`T` 缺失而 `O` 存在 → `rename(O, T)`（崩在两次 rename 之间）；`T` 存在 → `O` 是已换下的旧包，删除；`P` 一律删除（未完成的新包，或 RENAME_SWAP 后换出来的旧包）。每种中间态都能唯一推出动作，journal 是多余的状态源。
4. commit：删除换下来的旧包（`P` 与 `O`，已裁决：不保留回滚副本，回滚只由 LIBRARY 提供）；删除该产品的陈旧基线（整包即真相）。
5. 不支持跨更新的多实例：这是 IN_PLACE 的固有代价，需要的产品用 LIBRARY。

宿主退出规则因此收敛为：对账写沿用现状（表里含 executable 或 `sidecar_relpath` 才退出）；指针切换从不要求；换名切换总是要求。

#### 4.4 LIBRARY 的其余部分

- `active.json` 与 `relkit-version.json` 新增 `bundle`（如 `"SvnAutoMerge.app"`），仅 BUNDLE 有值；`executable` 仍是相对安装根的完整路径（`versions/<id>/<bundle>/<ExecutableRelpath>`，TREE 省去 `<bundle>`），供在用判定与拉起；宿主传入的 `ExecutableRelpath` 始终相对内容根（§4.2）。已裁决：只加这一个字段，保持现有 JSON 键与数值类型不变，不改成 proto 定义（ProtoJSON 会把 int64 `code` 编码成字符串，破坏 Windows C++ launcher）。
- 删掉 `applyLibraryContent` 中按 `filesRoot` 后缀判断 `.app` 的死代码，改读表的 `unit` / `bundle_name`。
- `pruneVersions` 修两处现存缺陷，否则“多实例”是空话：
  - 现在按目录名字典序挑最新，`0.2.0+99` 排在 `0.2.0+221` 前面。改为按 `relkit-version.json` 的 code 排序。
  - 现在不看是否在运行。实例停在旧版本时连来两次更新，它所在目录会被删。改为跳过在用版本：引擎枚举进程可执行路径（darwin 用 sysctl `kern.proc.all` + `kern.procargs2`，不依赖 cgo，结果等同 `proc_pidpath`；Windows `QueryFullProcessImageName`；Linux `/proc/<pid>/exe`），落在 `versions/<id>/` 下即在用。不新增宿主契约字段。
- `switchActive`、`rollbackActive`、`deleteInstalled` 不变，它们按整目录工作。
- macOS 上 `install_root` 不得位于任何 `.app` 内（否则改写 launcher 包会破坏其签名），也不得等于 dataDir；引擎检查祖先目录，违反报 `LAYOUT_UNSUPPORTED`。
- 任何写 `active.json` 的一方（引擎、Windows 安装器、macOS launcher 导入 seed）都持有同一把 `.relkit-apply.lock`。

### 5. darwin 上拉起 BUNDLE 一律走 LaunchServices

`relaunchHost` 现在直接 `exec` 可执行文件。在 macOS 上这会让 relkit-updater 成为新进程的 TCC responsible process，权限弹窗与归属都会错（`install-layouts.md` §8 已写明）。改为：

- 内容为 BUNDLE 时：`open -n <bundle 路径> --args <宿主约定参数>`（`-n` 保证多实例产品能开出新实例；也可用 NSWorkspace，等价）。IN_PLACE 的 bundle 路径是 `install_root`；LIBRARY 是 `install_root/versions/<id>/<bundle>`。
- LIBRARY 且产品有 launcher 时，仍拉起版本内的 bundle，不拉 launcher，避免 launcher 再次走选择逻辑；launcher 只负责用户主动启动。
- TREE 内容在所有平台保持现状。

### 6. 引擎属于调用它的宿主：一般形态与托管库特例，废除共享 sidecar 刷新

第一版 §6（“共享 sidecar + 显式来源刷新”）有根本缺陷：`checkProtocol` 要求宿主与引擎协议严格相等（ADR 0026 “每个引擎构建只有一个协议，没有窗口”）。宿主本身被版本化（多版本并存）时，若共用安装根上的一份引擎：

- 刷新到新引擎后，仍在运行旧版本的实例、或回滚到的旧版本拿到更新协议的引擎 → `UPDATER_TOO_NEW`；
- 不刷新，则新版本宿主拿到旧引擎 → `UPDATER_TOO_OLD`。

怎么刷都错。ADR 0026 的构建闸门本来就保证“宿主 proto 副本 == 打进同一产物的引擎导出的 proto”，所以规则只有一条：引擎是调用它的那个宿主的内容的一部分，随宿主一起更新，宿主只调用与自己同一份内容里的引擎。

#### 6.1 一般形态：一个产品，宿主更新自己

标准形态里，一个安装根只属于一个产品，调用引擎的宿主就是被更新的内容本身。三种标准形态：

| 形态 | 宿主所在 | 引擎位置 | 根上的入口 |
| --- | --- | --- | --- |
| TREE/IN_PLACE | 安装根 | 安装根内，随表写入 | 无（宿主即入口） |
| TREE/LIBRARY 或 BUNDLE/LIBRARY | `versions/<id>/` | `versions/<id>/` 内（BUNDLE 在包内 `Contents/Resources/runtime/`） | launcher：只读 `active.json` 拉起版本，不调用引擎，由安装器写入，引擎从不更新它 |
| BUNDLE/IN_PLACE | `.app` 本身 | 包内 `Contents/Resources/runtime/` | 无（`.app` 即入口） |

现有产品的对照（已核实线上产物）：

| 产品 | 形态 | 现状 |
| --- | --- | --- |
| SvnMergeTool Windows | TREE/LIBRARY | 错误：`0.2.0+222` payload 21 个文件，无引擎；版本目录里的宿主调用根上安装器写的那份 |
| SvnMergeTool macOS | BUNDLE，两种 Placement | 引擎侧已就绪，宿主侧待接入（实施第 6 步） |

SvnMergeTool Windows 的后果：根上引擎只有安装器会写，`refreshSidecar` 两个候选源都不存在，应用内更新从不换引擎。从 `+218` 及以前在应用内升到 `+219`（protocol 5）后，新宿主对上根上旧引擎，更新检查失败，只能重跑安装器恢复。dev 渠道已受影响（推断，待 Windows 真机确认）；修复前 `+219` 及以后不得升 stable。

#### 6.2 特例：托管库（Loom）

Loom 是一个安装根上的两个产品：

- Loom 启动器（产品 `loom-launcher`）：TREE/IN_PLACE，安装根就是它的目录，payload 含 `loom-launcher.exe`、`relkit-updater.exe`、`launcher-dependencies.lock.json`（`0.1.0+59` 已核实）。它是唯一调用引擎的宿主，既更新自己，也为编辑器装、切、删版本。
- Loom 编辑器（产品 `loom`）：TREE/LIBRARY，`versions/<code>/` 加 `active.json`，`retain=2` 加 `reserved_codes`（按项目钉版本）。它不是宿主，不调用引擎，版本内不需要引擎。

这是 Unity Hub / JetBrains Toolbox 的形状：管理器本身原地更新，被管理的工具多版本并存。整体看是 versioned，但入口不可能 versioned（它负责读 `active.json`，必须待在固定路径）。按 §6 的规则它是正确的：引擎属于启动器这个宿主，随启动器的对账写一起更新。

它是特例而不是第四种形态，原因是它把两个产品放进同一个安装根，引擎因此要承担“同根共存”的义务。标准形态不需要这些义务，特例也不新增 Placement 或宿主契约字段。共存义务如下（前两条是复审发现的现存缺陷）：

1. 对账写只碰自己拥有的东西。`applyContentTable` 结尾的 `removeEmptyDirectories(install_root)` 会遍历整个安装根，包括编辑器的 `versions/`：既要扫过数 GB 的编辑器文件，也会删掉编辑器版本里本应存在的空目录。改为只尝试删除本次删除或替换过的条目的祖先目录，止于安装根。这对标准 TREE/IN_PLACE 同样是正确行为（安装根是开放目录，§4.1）。
2. 基线按产品区分。两个产品共用 dataDir（`<user_data>/relkit`），而基线文件名是固定的 `baseline.pb`。现在只有启动器用基线所以没出事，但这是隐含的巧合。改为 `baseline-<product>.pb`，旧文件名读一次后迁移。
3. 同根同锁。`.relkit-apply.lock` 在安装根，启动器自更新与编辑器装版互斥，这正是需要的，保持不变。
4. 在用判定覆盖被托管产品。prune 的进程枚举（§4.4）同样跳过正在运行的编辑器版本；启动器已有的 `in_use_codes`（显式删除）不变。
5. 被托管产品的内容格式受托管宿主的引擎约束：编辑器 payload 用到的格式，启动器自带的引擎必须认识。Windows 上不会出现 `/2`（无软链、无 BUNDLE），目前无约束；将来若有 macOS 托管库，启动器必须先于编辑器升级到认识 `/2` 的引擎，否则 selector 协商回落完整安装。

判定方式：安装根下同时存在 IN_PLACE 的宿主和另一产品的 LIBRARY，即为托管库。引擎不需要识别它，上述义务对所有对账写都成立；特例只是让这些义务第一次有了实际后果。

#### 6.3 决策

- 删除 `refreshSidecar` 及其测试。`2d38946` 修的路径问题随之消失，不留半套机制。
- `InstallSpec.sidecar_relpath` 保留、语义不变：对账写用它判断是否要求宿主退出；其余写法忽略。宿主契约不变，无需协议升级。
- 宿主构建闸门保证引擎在自身内容里，并沿用 ADR 0026 的 proto 逐字节比对。SvnMergeTool Windows：`validate_windows_payload_package` 必须要求 `files/relkit-updater.exe`。
- SvnMergeTool Windows 安装根上的 `relkit-updater.exe` 不再刷新，只服务修复前的旧版本；这些版本被 prune 后由下一版安装器删除。
- 已在 dev 升到 `+219..+222` 的 Windows 机器无法自救（旧宿主仍调用根上旧引擎），需手动运行一次 setup.exe。

### 7. 宿主侧契约（以 SvnMergeTool 为第一个落地者）

1. bundle 与版本目录只读。任何运行期写入（日志、缓存、状态）进 dataDir。SvnMergeTool 现在把日志写进 `.app/Contents/Resources/runtime/logs`（`resolvePortableLogsDir`），既破坏签名封印，也是 `kMacosPreservedEntries` 存在的唯一原因；改为 dataDir 后删除该 preserve。Windows 日志写在 `versions/<id>/logs` 同理，一并改掉（否则 prune 会连日志一起删）：改到安装根 `logs/`。它由安装器预建、不属于任何版本，引擎在 LIBRARY 下只动 `versions/` 和 `active.json`，不会碰它；放在安装根是为了让用户在安装目录里直接找到日志。macOS 的安装根是 `.app` 本身，没有这样的位置，所以进 dataDir。（SvnMergeTool Windows 已于 `9714526` 落地，含引擎随版本树与 §7.2。）
2. 引擎路径：宿主按“自己可执行文件所在的内容根”解析引擎，不再按安装根解析（§6）。
3. 安装根由自身路径推导，不靠 launcher 传参：可执行文件位于 `<root>/versions/<id>/...` 时 `install_root=<root>`、Placement=LIBRARY；否则 macOS 为 BUNDLE/IN_PLACE（`install_root` = 当前 `.app`），Windows 为 TREE/IN_PLACE。与现有 Windows `currentPortableAppRoot` 同一思路。存量 macOS 裸包因此能直接应用内升级；用户下次手动装 DMG 时迁到 launcher + library。
   例外：包被 Gatekeeper 随机化转移（App Translocation，直接运行“下载”里带 quarantine 的包）或位于只读卷（挂载中的 DMG）时，推导出的路径不是用户的安装位置。宿主用 `SecTranslocateIsTranslocatedURL` 与卷只读属性识别，不发起应用内更新，提示把应用拖进“应用程序”。
4. macOS launcher（bundle id `<id>.launcher`，已裁决）：原生小 `.app`（Swift/ObjC，不含 Flutter、不含引擎），读 `~/Library/Application Support/<id>/active.json`，用 NSWorkspace 拉起 `versions/<id>/<bundle>`。launcher 内嵌一份 seed `.app`，放在 `Contents/Library/Seed/`（嵌套代码的规范位置，不放 `Resources`，否则 `codesign --strict` 会抱怨）。首次运行或 DMG 重装后，若 seed 的 code 不在 library 中，`clonefile` 导入 `versions/<id>/` 并激活。这与 Windows Inno 重装写 `active.json` 语义对称，属安装器职责，引擎不参与。
5. Gatekeeper：从浏览器下载的 DMG 带 `com.apple.quarantine`，`clonefile` 会把它复制到导入的 seed 上，ad-hoc 签名的内层包随后会被拦截。launcher 导入后、对已通过 `codesign --verify` 的副本移除该属性。用户已经批准过包含这份 seed 的 launcher，这不扩大信任面。引擎下载的 payload 不带该属性，应用内更新不涉及。
6. 发布：macOS 同时出 DMG（完整安装，供首装与老引擎回落）与 `/2` payload（BUNDLE 单元）。同一张 payload 服务两种 Placement。
7. 修订 SvnMergeTool `adr-009` 第 5 条。Loom-Launcher 宿主侧无需改动；其共存义务（§6.2 第 1、2 条）在引擎里修。

### 8. 已知代价与待实测项

- Dock 固定：用户若把运行中的内层版本固定到 Dock，旧版本被清理后图标变问号。Fusion 360 同样存在。缓解：launcher 首启提示固定 launcher；`retain` 不低于 2。不在引擎层解决。
- TCC：ad-hoc 签名每次构建身份不同，换版本后隐私授权失效，与布局无关（IN_PLACE 整包也一样）。只有稳定签名身份能解决，独立跟进。
- launcher 与内层 app 的 bundle id 分开（`<id>.launcher`）后，Dock/Spotlight/“打开方式”的表现待实测。
- 非 APFS 卷（外置 HFS+）：交换退化为两次 rename，崩溃窗口靠兄弟路径恢复兜底（单测已覆盖各中间态）；真机 kill -9 于两次 rename 之间待测。
- `/Applications` 可写性：管理员用户默认可写；标准用户不可写时走 `PERMISSION_DENIED` 引导 DMG，待实测文案。
- seed 导入后移除 quarantine 的做法待真机确认无“仍要打开”拦截；`open` 拉起应用内更新后的新包同样待确认。
- 进程枚举判定“在用”（§4.4）依赖可执行路径；macOS 上被移动过的运行中 bundle 路径会变，需实测 `kern.procargs2` 返回值。
- Windows 指针切换的 `rename(P, T)` 在杀毒实时扫描下的失败率与重试时长待实测。
- 换名切换后，旧包中仍在运行的进程（宿主已退出，但可能有子进程或 XPC 服务）被删除文件后的行为待实测；APFS 上已打开的文件不受影响，但按路径懒加载的资源会失败。

## 否决项

- 压平软链（复制目标文件）：framework 结构被破坏，codesign 校验失败；体积翻倍。
- post 脚本重建软链：把结构正确性交给每个产品的脚本，违背唯一引擎；脚本运行前的中间态是一个签名无效的包。
- 只做整包交换、放弃 macOS LIBRARY：无法满足“不退出即装好”与多实例（`retain=2`），回滚也退化为重新下载。
- 只做 LIBRARY、不做整包：存量裸包用户必须先手动重装一次，且无 launcher 的产品没有路。
- 按 `.app` 扩展名推断 BUNDLE：即 `79be2be` 之前的做法，隐式规则会被重构悄悄删除；显式枚举可测试、可闸门。
- 在 zip 中存软链：各 zip 实现对软链的 external attributes 处理不一致，且解包库默认行为不安全。
- 新增第三种 Placement（如 BUNDLE_SWAP）：把 Content 的属性塞进 Placement 轴，破坏正交；四格表会退化成特判。
- 共享 sidecar + 刷新（本 ADR 第一版 §6）：与严格协议相等冲突，多版本并存时必然有一方失配。
- 给引擎加协议窗口来兼容共享 sidecar：推翻 ADR 0026 “单协议、无窗口”，换来的只是保住一个本不该共享的文件。
- 由宿主在 ApplyOp 里上报在用版本：要改宿主契约并升协议；引擎自己枚举进程即可得到同样信息。
- 让对账写去替换整个目录（把 TREE/IN_PLACE 也改成全新写）：开放安装根里有 launcher、`active.json`、用户文件与 preserve 条目，不归这份内容独占；Windows 上目录内任一文件被打开就无法改名。需要整目录替换的产品用 LIBRARY。
- LIBRARY 沿用对账写（现状：先 `RemoveAll` 再逐文件写并记 journal）：版本目录是全新的，journal 与备份无事可做；先删再写会删掉正在运行的同版本，且崩溃留下残缺的 active 版本。
- BUNDLE/IN_PLACE 先交换再跑 post 脚本、失败再换回（第二版）：交换后、校验前存在一个未经校验的包占据正式路径；全新写把脚本与签名校验都放在切换之前。
- 新增 `ALREADY_INSTALLED` 错误码：改宿主契约；同版本重装可做成幂等切换，冲突时复用 `OCCUPIED`。
- 换名切换记 journal 并按 journal 恢复（第四版）：`T`、`P`、`O` 三个兄弟路径的存在与否已唯一决定恢复动作，journal 只会成为第二个可能与磁盘不一致的状态源。
- 由宿主在 `InstallSpec` 上报 IN_PLACE 的内容单元：要改宿主契约，且宿主说错时会把树当包换名或把包当树对账；单元是磁盘事实，引擎自己判定并与表交叉校验。

## 实施顺序

0. 止血（与 macOS 工作解耦，先做）：
   - relkit（已实现，`cb03d6d`；其中 `writeMode` 后在第 3 步并入 `layoutFor`）：删 `refreshSidecar` 及测试；引入 `writeMode`，LIBRARY 改为全新写（`.partial` 物化 → post 脚本 → `relkit-version.json` → `rename`，去掉空转的 journal 与备份，不再 `RemoveAll`）；同版本重装按 4.2 幂等或 `OCCUPIED`；`pruneVersions` 按 code 排序、跳过在用版本；Windows `rename` 退避重试；对账写的空目录清理只限本次触及条目的祖先（§6.2-1）；基线文件按产品命名（§6.2-2）。
   - SvnMergeTool Windows：引擎放进 `versions/<id>/` 并改为调用自带引擎，`validate_windows_payload_package` 要求含引擎；日志移出版本目录。
   - （已完成）发 dev，在 Windows 真机验证：SvnMergeTool 从 `+222` 手动重装后能应用内升级；Loom 启动器自更新后编辑器 `versions/` 无改动，编辑器装新版、重装同版、另一实例停在旧版时更新，三项都正确。再决定 stable。（2026-11-05 结果：SvnMergeTool 手动重装后应用内升到 +225；启动器 0.1.0+57 → +59 自更新后编辑器 `versions/` 与 `active.json` 零差异。编辑器三项用户裁决不单独验证。2026-11-06 发 stable。）
1. relkit / Content 轴（已实现，`2276d82`）：`payload.proto` 字段、`CheckSymlinks`、`payload build --bundle`、`inspect`/`Extract`、payload/2 manifest；父目录软链加固。单测覆盖全部规则的正反例。
2. relkit / 协商（已实现，`2276d82`）：apply 偏好列表、`prefersApply`、stage 闸门对 `/2`、BUNDLE 仅 darwin、SYMLINK 拒绝 Windows。
3. relkit / 落盘（已实现，`2276d82`）：全新写支持 BUNDLE（物化到内容根、软链、codesign 校验）；换名切换（`controlDirFor`、磁盘判定单元并交叉校验、`RENAME_SWAP` 与两次 rename 回落、兄弟路径恢复、commit 删旧包与陈旧基线）；BUNDLE/LIBRARY 删死代码、`bundle` 字段、`install_root` 不得在 `.app` 内；宿主退出规则按 4.3；LaunchServices 拉起。单测用带 framework 软链的测试 `.app` 跑两种 Placement 与各崩溃中间态，另有真实 ad-hoc 签名与篡改反例；真机 kill -9 留待第 6 步。
4. 文档（已完成）：`docs/design/topology.md` 第七节去掉“待实现”并按实现重画写法流程；`install-layouts.md` §7（引擎随宿主）、§8（与本 ADR 对齐）、§11；ADR 0013/0014 加“已被 0027 修订”指针。
5. （已完成）发布 relkit（`2d38946` 会被第 0 步删除的代码覆盖，不单独发），升级两台服务器（`update.devcloud.woa.com`、`publish.firoyang.com`）。
6. SvnMergeTool macOS：日志进 dataDir、安装根推导、构建出 `/2` payload、发 dev 版，本机先验 BUNDLE/IN_PLACE（存量 `/Applications` 包），再做 launcher + seed 验 LIBRARY。（BUNDLE/IN_PLACE 已完成：本机 +224 DMG 手装后应用内升到 +225，换名切换、旧包删除、锁与临时目录清理、签名均正确。LIBRARY 与非 APFS 卷的 kill -9 测试未做，留待有产品采用 LIBRARY 时补。）
7. （已完成）SvnMergeTool `adr-009` 修订。
