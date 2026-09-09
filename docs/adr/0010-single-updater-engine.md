# ADR 0010: 单引擎 updater 与宿主 facade

- Status: Accepted
- Date: 2026-09-09
- Relates: [0003](0003-protobuf-v2-wire-format.md)（线上 RUP）、[0009](0009-publisher-protocol-negotiation.md)（窗口协商模式）

## 背景

Go / Dart / Node 三套 SDK 各自实现 check、下载与（部分）apply，行为已经漂移：skip 是否生效、可变指针是否 cache-bust、index 验签成功后 manifest 失败是否换源、`lastResult` 字符串、scheduler 间隔。`relkit-apply` 只覆盖 Windows `versionedDir`，与 Dart `wholeRoot` 不对齐。

线上签名对象继续是 `rup.v2`。本机安装、session、plan、IPC 不得污染那份协议。

## 决策

1. **唯一引擎。** check、fallback、节流、源学习、下载验收、plan/session、apply/rollback 只在 Go `relkit-updater` 实现一次。它是按操作拉起的短命进程，不是 daemon，不持发布私钥。
2. **两层协议。** `rup.v2` = 线上签名对象。`relkit.updater.v1` = 本机 IPC、state、plan、session、InstallSpec。安装布局不进入 index/manifest。
3. **IPC。** 一次进程一条 `UpdaterRequest`；stdout 只输出 4 字节大端长度分帧的 protobuf `UpdaterEvent`；stderr 只写人类日志。取消 = 杀子进程。apply worker（`--worker`）在宿主退出后靠 session 文件继续。
4. **IPC 窗口。** 模式对齐 ADR 0009 的 `[min,max]`，当前 `[1,1]`。无交集返回 `updaterTooOld` / `updaterTooNew`。窗口是 facade 编译期常量，不进 `ClientProfile`。协议头不是 `X-Relkit-Publish-Protocol`。
5. **宿主 facade 与 wire 同一 IDL。** 方法、结果五变体、错误码、`ClientProfile` 由 `protoc-gen-relkit-facade` 生成。禁止手写第二份 DTO 或转换层。Go 仅允许导出首字母大写。失败返回带 `ErrorCode` 的结果对象，不抛裸异常。
6. **数值宽度。** `code` / `sequence` / `size` 等与 `rup.v2` 一样用 `int64`。间隔用 `google.protobuf.Duration`；引擎按 `minCheckInterval`（5 分钟）钳制，不信任宿主小时整数。
7. **sidecar。** 每个产品安装树一份同版本 `relkit-updater`，不进 PATH、不注册系统服务。consume 同一 SHA 产出 facade 与二进制。
8. **旧 SDK。** 冻结功能，仅作迁移桥；稳定后删除旁路实现。引擎读取 legacy JSON state 一次并写成 `state.pb`，水位与 skipped 必须导入。

## 宿主调用面（签名 SSOT）

见 [`proto/updater/v1/updater.proto`](../../proto/updater/v1/updater.proto) 与生成 facade。方法：`open` / `capabilities` / `check` / `skip` / `download` / `apply` / `status` / `cleanup` / `cancel` / `scheduler`。

`check` 结果变体：`upToDate` | `updateAvailable` | `fallbackRequired` | `throttled` | `failed`。优先级：available > fallback > 其余。

`download`/`apply` 只接受引擎签发的 `planId`。

## 行为裁决（不再以某 SDK 为准）

- 非 mandatory 被 skip 后 check 返回 `upToDate`；mandatory 不可 skip（`skipDenied`）。
- directory/index/fallback cache-bust；manifest/artifact 按 digest 缓存。
- 某 index 源可信但 manifest/artifact 失败时继续下一 index 源。
- prior notes 范围：当前 code 到本次 target（含跨越节点）。
- artifact filename 拒绝绝对路径、分隔符、`.`/`..`、保留名、目录逃逸。
- `lastResult` 只持久化 enum。legacy 映射见 `internal/updater/legacy.go`。

## 否决

- 常驻 updater 服务 / 系统级共享二进制
- JSON 行 IPC、自由文本错误码
- 各语言再写一套 check/apply 算法
- 宿主自算下次检查时间
- 把 InstallSpec 写进签名 index

## 后果

- 接新语言 = 加一个 facade 生成目标
- SvnMergeTool / Dec 只编排 UI 与进程生命周期
- 消费仓必须同一 consume SHA 带上 sidecar
