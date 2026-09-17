# ADR 0014：内容、落点与版本库

- 状态：Accepted
- 日期：2026-09-17
- 关联：[0010](0010-single-updater-engine.md)、[0013](0013-internal-update-payload.md)
- 取代：ADR 0010 中 Layout 分类、SPEC 附录 B 的 Layout API

## 背景

原 `Layout` 同时编码内容形态、安装落点和删除策略：

- `preserve` 只对 wholeRoot 生效；
- `retain` / `reserved_codes` 只对 versionedDir 生效；
- fileSet 另带 artifact 到目标路径的映射；
- `requires_host_exit` 被写成 Layout 常量。

这些并非同一维度，导致 updater 有 wholeRoot、versionedDir、fileSet 三条 apply
实现，Go/Dart/legacy 命令又各自复制部分逻辑。

## 决策

只保留两根独立轴：

- **Content** 由 artifact kind 决定：`payload` 是结构化内部更新；
  `archive` 是兼容期可合成文件表的旧内容；installer/binary/blob 走完整安装。
- **Placement** 由宿主 `InstallSpec` 决定：`IN_PLACE` 落到 `install_root`；
  `LIBRARY` 落到 `versions/<version>/` 并通过 `active.json` 激活。

归属/删除不是第三根轴，统一采用 ADR 0013 的基线差分规则。原布局映射为：

- wholeRoot → IN_PLACE；
- versionedDir → LIBRARY；
- fileSet → IN_PLACE，其不删除旧文件的历史行为被修正。

所有内部内容先变成同一种 `FileTable`，再进入：

```
parse -> reconcile -> precheck -> journal -> activate
```

旧 archive 解包后遍历内容合成 FileTable，不再拥有独立的 whole-root 交换算法。
macOS `.app` 在合成表时选择 bundle 根。Reconcile 只依赖表和基线；
Activate 才关心 Placement。

`InstallSpec` 使用 `Placement placement` 和独立 `LibraryPolicy`。后者包含 retain
与 reserved_codes，使字段作用域在类型上可见。`file_set` 删除。

LIBRARY 是正式版本库，不只是 apply 副作用：安装、列出、切换 active、回滚、
按 retain 清理均由唯一 Go 引擎实现。`install_only` 安装但不激活；项目 pin
通过 `reserved_codes` 参与清理。按项目选择版本的路由接口保留，但本次不实现。

## 唯一实现

删除以下旁路：

- `cmd/relkit-apply`
- `sdk/apply`
- `sdk/dart/lib/src/apply`

宿主只通过 ADR 0010 的 facade 调 `relkit-updater`。脚本、删除边界、journal、
sidecar 刷新和版本库策略不再允许语言实现漂移。

## 后果

in-place 资源更新可在宿主运行时完成；只有触及当前 executable/sidecar 或预检
发现占用才要求退出。library 将完整目标版本写入独立目录，成功后才切换指针，
旧版本由统一策略清理。

[`docs/design/install-layouts.md`](../design/install-layouts.md) 保留为迁移背景，
其 Layout API 和分支实现以本 ADR 为准。
