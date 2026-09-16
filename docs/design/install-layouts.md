# 安装布局（Install Layout）落盘契约

---
title: 安装布局（Install Layout）落盘契约
category: design
created: 2026-09-16
updated: 2026-09-16
status: approved
related: SPEC.md 附录 B, ADR 0010, internal/updater/apply.go, sdk/dart/lib/src/apply/swap.dart
---

## 1. 这篇要解决什么

SPEC 附录 B 用一张三行表列出了 `wholeRoot` / `versionedDir` / `fileSet`，这对「协议不管安装」
这个立场是够的，但对**要接入的宿主**不够。下游选 `versionedDir` 时至少有三个问题表里没有答案，
而且每一个答错都会把安装目录搞坏：

1. 版本目录建起来之后，人双击的那个稳定入口由谁提供？引擎会不会生成？
2. `versionedDir` 是不是「可以边跑边装」？宿主要不要退出？
3. `retain` 到底按什么算？填 1 和填 3 分别发生什么？

已经发生过一次下游按表面语义推断走偏：把 `versionedDir` 当成 Chrome / Squirrel 那种
「装的时候谁开着都无所谓」的模型，据此规划多实例并发更新方案。**实际不是**（见 §4）。
这篇把三种布局的落盘行为写全，作为 SPEC 附录 B 的展开。

权威实现是 Go 引擎 `internal/updater/apply.go`（`relkit-updater` 二进制，ADR 0010）。
Dart SDK `sdk/dart/lib/src/apply/swap.dart` 是同一语义的第二实现，差异见 §6。

## 2. 三种布局对照

| | `wholeRoot` | `versionedDir` | `fileSet` |
|---|---|---|---|
| 落盘位置 | 整个 `installRoot` 换掉 | 新建 `installRoot/versions/<version>/` | 按 `fileSet[]` 逐个文件写到 `destRelpath` |
| 旧版本 | rename 成 `<root>.relkit-old`，成功后删除 | 原地不动，由 `retain` 决定何时清 | 每个文件 rename 成 `<dest>.relkit-bak`，提交后删除 |
| 宿主必须退出 | **是** | **是** | 否 |
| 失败回滚 | 把 `.relkit-old` 改回来；改不回来报 `INCOMPLETE` | 删掉新建的版本目录，旧版本与 `active.json` 未动 | 按 journal 逆序还原 `.relkit-bak` |
| `preserve` 生效 | 是（从备份里捞回来） | 否 | 否 |
| `retain` 生效 | 否 | 是 | 否 |
| 占用冲突 | 换根要 rename 整个目录，被占用就重试 60s 后失败 | 只写新目录，不碰在用文件 | 单文件被占用则该文件失败并回滚 |

「宿主必须退出」即 `ApplyAccepted.requires_host_exit`，由 `layoutRequiresHostExit`
（`internal/updater/engine.go`）决定：`wholeRoot` 与 `versionedDir` 都为 true，只有 `fileSet` 为 false。

## 3. `versionedDir` 的落盘形状

```
installRoot/
  <launcher>.exe           ← 产品自带的稳定入口，引擎从不生成、从不覆盖
  relkit-updater.exe       ← 引擎每次 apply 都会刷新（§7）
  active.json              ← 指针，原子写
  update_apply.json        ← 会话状态（versionedDir 写在这里，其它布局写在 dataDir）
  versions/
    0.1.2+80/              ← 上一版，按 retain 决定留不留
    0.1.2+81/              ← 当前版，payload 整棵树拷进来
```

版本目录名取 `plan.version`，其中 `/` 换成 `_`；`version` 为空时退化成 `code-<code>`。
payload 是 artifact zip 解包后的**整棵树**，不做「单顶层目录剥壳」——zip 里是什么结构，
版本目录里就是什么结构。（唯一例外在 macOS：`unpackFirst` 会先在解包结果里找 `.app` bundle
当 payload 根，这条主要服务 `wholeRoot`。）

`active.json`（`internal/updater/apply.go` 的 `activePointer`，原子写：临时文件 + rename）：

| 字段 | 含义 |
|---|---|
| `code` | 目标版本的整数 code |
| `version` | 目标版本串 |
| `path` | 版本目录相对 `installRoot` 的路径，如 `versions/0.1.2+81` |
| `executable` | `path` + `executableRelpath` 拼出的可执行文件相对路径 |
| `previous` | 切换前 `active.json` 里的 `path`；首次安装为空 |

注意 `executableRelpath` 是**相对版本目录**（即相对 payload 根）解释的，不是相对 `installRoot`。

## 4. 入口由产品提供，引擎不生成 launcher

这是最容易误判的一条。引擎在 `installRoot` 下只碰四样东西：`versions/`、`active.json`、
`update_apply.json`、`relkit-updater[.exe]`。**它不会创建、不会更新任何 launcher，也不会动快捷方式。**

所以 `versionedDir` 要成立，产品必须自己在 `installRoot` 放一个稳定入口：读 `active.json`，
按 `executable` 起真正的程序。安装器（NSIS / MSI / pkg）的快捷方式、开始菜单项、卸载项
都指向这个 launcher，而不是指向某个版本目录里的 exe——后者会在下次更新后失效。
Dart SDK 的 `swap_test.dart` 把这条钉成了断言：切换前后根目录那个 `SvnAutoMerge.exe`
内容不变，换的只是 `versions/` 与 `active.json`。

因此 `versionedDir` **不是**「装完就生效」：装完只是写了新目录和指针，真正生效发生在
launcher 下一次解析 `active.json` 的时候。

## 5. 宿主退出与重启

`versionedDir` 同样要求宿主退出，理由不是「文件会被占用」，而是引擎的 apply 流程统一如此：

1. 宿主调 `apply` → 引擎记下 `pid = os.Getppid()`（**调用方那一个进程**），拷一份 worker 到
   `<stagedRoot>/.worker/`，用 `--worker <sessionId> --data-dir <dataDir>` 拉起来，
   然后回 `ApplyAccepted{requires_host_exit}` 就结束。
2. worker 先 `WAITING_FOR_EXIT`：轮询那个 pid，最长 5 分钟，每 500ms 一次心跳。超时报
   `ERROR_CODE_OCCUPIED` 并进 `NEEDS_ATTENTION`，**不会**强杀宿主。
3. pid 消失后进 `COPYING`，按布局执行。
4. `relaunch=true` 且 `executableRelpath` 非空时进 `RELAUNCHING`：`versionedDir` 下从
   `active.json` 的 `executable` 起，读不到指针才退回 `installRoot/executableRelpath`，
   工作目录是 `installRoot`。只起**一个**进程。
5. `COMPLETED`。

只等这一个 pid 意味着：同一安装根下**其它正在运行的实例既不被等待，也不被通知**。
对 `wholeRoot` 这是危险的（别人正在用的文件会被整根换掉）；对 `versionedDir` 要看 `retain`
（§6）——只要旧版本目录还在，别的实例就一直跑在自己那棵树上，不受影响，直到它自己重启。

## 6. `retain` 语义与两处实现差异

Go 引擎（`pruneVersions`）：

| `retain` | 保留 | 清理 |
|---|---|---|
| `<= 0` | 当作 2 | 同 `retain=2` |
| `1` | 只留当前版 | 删除其余全部，**包括上一版** |
| `2` | 当前版 + 上一版 | 删除其余全部 |
| `>= 3` | 当前版 + 上一版进保留集 | **什么都不删**（清理分支被 `retain <= 2` 挡住），目录无界增长 |

两条要下游注意：

- **`retain=1` 与多实例共存冲突。** 删除上一版目录时，如果还有进程跑在那棵树上，Windows 会因
  文件占用而部分失败：没被锁的文件照删，锁住的留下，结果是一棵残缺的旧版本树。引擎忽略删除
  错误（`_ = os.RemoveAll`），不会报错。要允许多实例并存跨过一次更新，`retain` 至少给 2。
- **`retain >= 3` 当前等于「全留」**，不是「留三份」。Dart SDK 的 `_pruneVersionDirectories`
  按「最近 N 份」正确实现，且 `retainVersions < 1` 直接抛 `ArgumentError`（Go 侧则把 `<= 0`
  当 2）。两边语义不一致；取齐之前，Go 侧只有 1 / 2 两个有效取值。

## 7. sidecar 刷新位置

`sidecarRelpath` 非空时，每次 apply 结束都会把 payload 里的那一份更新器覆盖到
**`installRoot/relkit-updater[.exe]`**——注意是安装根，不是版本目录。做法是先把旧的 rename
成 `.old`，拷新的进去，再删掉 `.old`。所以 `versionedDir` 下更新器是跨版本共享的一份，
和 launcher 一样住在根目录。源路径先找 `<payload>/<sidecarRelpath>`，找不到再找
`<stagedRoot>/<sidecarRelpath>`，都没有就跳过（静默）。

## 8. macOS 可以版本并存，但不能照搬当前落盘形状

SPEC 附录 B 原来要求引擎拒绝 `macos` + `versionedDir`，却没有论证。这条平台禁令过强。
版本目录不只用来绕开 Windows 文件占用，也可以承担**回退、并行保留多个工具版本、按项目
选择兼容版本**；这些产品需求在 macOS 上同样成立。

行业已有两种成熟形状：

- Autodesk Fusion 的网页安装版把真实 `.app` 放在
  `~/Library/Application Support/Autodesk/webdeploy/production/<id>/`，用户看到的是
  `~/Applications` 下指向当前安装的 launcher / alias。它证明 macOS 可以用稳定入口加
  版本化的完整 app bundle；Fusion 本身是持续更新产品，并不向用户开放任意旧版选择。
- Unity Hub 在 macOS 上并列管理多份 Unity Editor，并按项目所需 Editor 版本启动；
  JetBrains Toolbox 也支持同一 IDE 多版本并列。这才是「项目钉版本」的直接先例。

关键不是禁止版本目录，而是保持每个版本的 **`.app` bundle 完整、独立签名与公证**：

```
installRoot/
  Loom Launcher.app
  versions/
    0.2.2+21/Loom Editor.app
    0.2.3+35/Loom Editor.app
```

稳定 launcher 应通过 LaunchServices / `NSWorkspace` 启动目标 `.app`，不能直接 exec
`Contents/MacOS/*`；后者可能把 launcher 变成 TCC 眼中的 responsible process。
只要各版本使用一致的 bundle identifier、签名身份与 designated requirement，路径变化
本身不会必然让 TCC 权限失效。代码签名封住的是**各自完整 bundle 的内容**，并不禁止多个
签名完整的 bundle 并列存在。

当前 Go 实现还没有完整支持这套 macOS 形状：`unpackFirst` 在 darwin 上找到 `.app` 后把
该 bundle 当 payload 根，`copyTree(payload, versions/<version>)` 因而把 bundle **里面的
内容**复制进一个不带 `.app` 后缀的裸目录；全局 `active.json` 也只表达一个当前版本，
不能表达「项目 A → 旧版、项目 B → 新版」。因此结论应是「当前实现需扩展后才能支持」，
不是「macOS 原理上不应支持」。

要支持按项目选版，还需在安装布局之外补一层版本路由：

1. 项目声明 Loom 兼容要求（精确 code 或闭区间；不能只靠“最新”）。
2. launcher 先定位项目配置，再从已安装版本中选择满足要求的一份；没有则引导安装。
3. `active.json` 只保留为默认版本指针；项目钉定另存映射，或由 launcher 每次解析项目要求。
4. 清理必须认识项目 pin 与正在运行的版本，禁止只按 current / previous 删除。
5. 更新器需支持安装指定版本而不强制把全局 active 切过去，并提供显式切换 / 回退操作。

## 9. 与 `sidecar.layout` 闸门的区别

产品仓运维 skill（`relkit-ops`）里有一条叫 `sidecar.layout` 的闸门，说的是
「`relkit-updater` 二进制打进发布树时放在哪、`tools/bin` 下叫什么」，属于**打包**问题。
本文讲的 `InstallSpec.layout` 是**安装**问题。两者同名不同事，别串。

## 10. 接入 checklist（`versionedDir`）

1. 产品提供 launcher，放在 `installRoot` 根，读 `active.json` → 起 `executable`；
   macOS launcher 必须启动完整 `.app` bundle。
2. 安装器把快捷方式 / 开始菜单 / 卸载项指向 launcher；首装时自己摆出
   `versions/<version>/` 与 `active.json`，否则第一次更新前 launcher 无处可去。
3. `executableRelpath` 按**相对版本目录**填。
4. `retain` 填 2（要允许多实例跨更新共存）或 1（只要当前版、且确定同时只有一个实例）。
5. 宿主收到 `requires_host_exit=true` 后必须真的退出自己的进程；别的实例不受管，
   需要的话由产品自己协调。
6. 别指望 `preserve`：它只对 `wholeRoot` 生效。`versionedDir` 的用户数据本来就不该放在版本目录里。

## 11. 已知差异（待处理）

| # | 现状 | 影响 |
|---|---|---|
| D1 | ~~SPEC 原来要求拒绝 macos + versionedDir；darwin 解包剥 .app~~ **已修**：versionedDir 拷完整 `.app`；payload 无 bundle 则失败 | 完成 |
| D2 | Go `pruneVersions` 在 `retain >= 3` 时不清理；Dart `_pruneVersionDirectories` 按最近 N 份清理 | Go 已改为最近 N 份 + reserved_codes |
