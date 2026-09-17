# ADR 0013：内部更新轨与 Payload

- 状态：Accepted
- 日期：2026-09-17
- 关联：[0010](0010-single-updater-engine.md)、[0011](0011-immutable-release-consume.md)

## 背景

既有 artifact 只承诺「可下载且哈希正确」。exe / dmg / deb 等系统安装包和
可被 updater 解压覆盖的 zip 没有协议级区分，发布侧也不验证包内结构。它适合
冷启动安装，但不足以安全表达已装客户端的文件增加、覆盖、删除和版本迁移脚本。

本决策不引入二进制 delta/patch，也不把每个文件变成独立 RUP artifact。

## 决策

发布与消费分成两条并存的轨：

- **完整安装**：exe / msi / dmg / pkg / deb / rpm / zip 等不透明产物。供冷
  启动、跨大版本和内部更新不可用时使用；宿主执行平台正常安装流程。
- **内部更新**：`ArtifactKind=PAYLOAD`。一个 payload zip 内有 `files.pb`、
  `files/` 和可选 `scripts/`，由 `relkit-updater` 统一应用。

`files.pb` 使用 `relkit.payload/1`，是目标版本的完整文件表。每个文件声明
相对路径、大小、 SHA-256 和权限；正文必须与表一一对应，不能缺失或多出。
路径每一段都执行跨平台文件名检查，禁止绝对路径、`..`、反斜杠、Windows
保留名和符号链接。

删除不由发布方写删除列表，也没有 Ownership 开关。引擎使用固定基线差分：

```
delete = previous_baseline - current_table - preserve
```

首次接入或基线损坏时只增加/覆盖、不删除，并在成功提交后原子写入新基线。
删除前若当前文件 SHA-256 与基线不同，视为用户修改，保留并记录警告。
`InstallSpec.preserve` 与包内 `preserve` 取并集；命中路径不覆盖、不删除。

这会有两处有意的行为修正：

- 原 `wholeRoot` 不再扫描并删除用户放进安装目录但从未被 relkit 拥有的文件；
- 原 `fileSet` 会删除曾由 relkit 安装、但新文件表已移除的旧文件。

脚本只有 `pre` 和 `post` 两个阶段，必须来自包内、带 SHA-256 和超时，并显式
选择 `direct`、`sh` 或 `powershell` 白名单解释器；不接受任意 shell 字符串，
不提权。pre 失败不修改文件；post 失败按文件 journal 回滚。脚本是目标版本
脚本，跨多版跳跃不会补跑中间版本脚本；必须执行的逐版迁移通过 `minFrom`
强制多跳。

`UpdateAvailable.apply_disposition` 向 facade 暴露 `INTERNAL` 或
`FULL_INSTALL`，宿主不解析 payload 私有结构。是否要求宿主退出按本次文件表
计算：in-place 更新触及当前 executable/sidecar 时退出，纯资源更新不退出；
library 中新版本 executable 不属于当前运行树。

## 发布门禁

`relkit stage --payload <tree>` 生成 payload，不能由扩展名推断。输入树可用
`.relkit-payload/manifest.json` 声明 preserve 和脚本，配置目录不会进入安装树。
stage、publish 和 agent 都重新做结构校验。

payload 自动带 selector `apply=relkit-payload`，老客户端没有该 selector，
因此不会选中。每个 payload 必须有一个去掉 `apply` 后 selectors 完全相同的
完整安装 artifact，避免老客户端无包可选。

## 兼容与版本

`Artifact.kind` 从 string 改为 `ArtifactKind` enum，manifest/staged schema
分别升到 `rup.manifest/3` 和 `rup.staged/3`。线上 index 仍是 `rup.index/2`。
老 RUP 客户端会忽略 wire type 不同的 kind，但 payload 仍由新 selector 隔离。

本机 IDL 删除 Layout/fileSet 并改变既有字段含义，不能诚实地声称兼容 IPC
1–2，因此 IPC 窗口是 `[3,3]`，而不是只把最大值扩成 `[1,3]`。

## 否决

- 二进制 patch、逐文件 CAS artifact、发布方维护删除列表
- `fullTree` / `declaredOnly` Ownership 枚举或 glob 管辖范围
- 扫描安装目录删除「表外文件」
- 任意 shell 字符串、payload 提权、回滚脚本

## 后果

发布包第一次有可验证的内部结构；内部更新可以事务化增、覆、删和运行迁移。
完整安装轨不受结构约束并继续服务冷启动。基线丢失时宁可遗留旧文件，也不冒险
删除用户数据。
