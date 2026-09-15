# ADR 0012：单一组件 Registry

- 状态：Accepted
- 日期：2026-09-15
- 关联：[0010](0010-single-updater-engine.md)、[0011](0011-immutable-release-consume.md)

## 背景

组件曾分别硬编码在 deploy、lock 重写、consume 和产品门禁中。增加一门语言
需要同步多处分支，漏掉任一处都会生成不完整 lock，甚至在替换 Go SDK 时静默
删除新 SDK。

## 决策

唯一清单位于 `scripts/host/hostlib/facets.py`。每一行属于三个 role 之一：

- `host-binary`：`serve`、`agent`，只构建并安装到主机。
- `product-binary`：`cli`、`updater`，按 target 固定进 lock 和 `tools/bin`。
- `product-tree`：`host-scripts`、各 SDK 与 bindings，作为 portable zip 安装。

行内声明 build flag、Go package / binary prefix、archive、源和安装路径、完整性
探针、target 安装名 override、stack detection、默认消费、import 信号及 WebView
投影义务。deploy 反向 import host 树里的清单；build 参数与产物名、SHA256SUMS /
lock key、consume 目标与完整性检查、保留子树和 updater 门禁均由它派生。

门禁注册在 `hostlib/gates.py` 并遍历清单。`updater.process=other` 不是豁免：
`relkit.json` 必须声明 `updater.entry`，有 WebView 时还要声明
`updater.projection`。`updater.urlAllowlist` 只按路径豁免 URL 检测，不豁免
sidecar chokepoint 或手写形状检测。

`scripts/host/relkit_host.py` 只组装 parser/dispatch，并兼容 re-export 既有
测试和产品脚本使用的符号。实现按职责位于 `hostlib/`：
`state`（持久状态）、`ssh`（SSH 配置与执行）、`inspect`（只读盘点）、
`onboard`（决策流程）、`reconcile`（drift 收敛）、`remote`（远端产品运维）、
`release`（consume/lock/发布）、`retrospect`（最终机械检查）、`digest`
（整树哈希）与 `gates`（注册式产品合同）。模块通过入口运行时绑定获得兼容
注入点，不互相 import 形成循环。

## 否决

- 为 bindings-ts 再增加一组 `if component == ...`。
- 把清单放在 deploy 树，使产品 consume 后看不到发布时的声明。
- 按语言维护平行的组件数组、安装目标或门禁。
- 以缺少 SDK import 作为未声明更新功能仓库的失败证据。
- 让 `other` 依赖 Agent 口头解释，或让 URL 白名单豁免手写 DTO。

## 后果

增加 facade 只加一行 registry；字段缺失在 import 时显式失败。清单随
`host-scripts` 整树哈希和 release lock 一起固定。
