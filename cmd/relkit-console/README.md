# relkit-console

> 本目录是 [`github.com/shichao402/relkit`](https://github.com/shichao402/relkit) 仓库的一部分（`cmd/relkit-console`），与发布 CLI 同仓。设计决策见 [ADR 0016](../../docs/adr/0016-serve-split-store-console.md)。

relkit-serve 拆分出的**管理面**：单页管理界面、账户/会话（ADR 0006 的 bootstrap 流程不变）、产品与令牌管理、文件树浏览、`/-/latest/` 固定链接跳转。单个静态链接的可执行文件，前端产物用 `go:embed` 打进二进制，无运行时依赖。

**角色：** 管理与观察面。它不伺服发布树（GET `/` 不是它的职责，那是 store/browse 的），也不直接写发布树：注册产品、签发/轮换/吊销令牌、下线产品这些写操作都转发给 relkit-agent，用操作员自己的 Bearer 代为执行。面板是人类鉴权面（用户名密码 + 会话 cookie），与数据面令牌（Bearer PUT）是两种信任域，进程隔离让一个 0day 拿不到写者权限。

store 拆包已完成（ADR 0016 第 3 步）：`cmd/relkit-serve` 已删除，`cmd/relkit-store` 承载纯存储面。console 与 store 可同机部署，console 照样只经读 adapter 读树，不直接打开本机目录。

## 前端技术栈

界面是一个 React 单页应用，源码在 [`web/`](web/)，构建产物落在 `internal/webui/dist/` 并由 `internal/webui/webui.go` 的 `//go:embed all:dist` 打进二进制。

| 关注点 | 选型 |
|---|---|
| 框架 | React 19 + TypeScript |
| 构建 | Vite 7 |
| 样式 | Tailwind CSS 4 + 一套 shadcn/ui 风格的 Radix UI 组件（`web/src/components/ui/`） |
| 路由 | React Router 7，`basename="/-/admin"` |
| 数据 | TanStack Query v5（写操作 `retry: 0`，不做自动重放） |
| 提示与图标 | sonner、lucide-react |
| 测试 | vitest + @testing-library/react |

## 本地开发与构建

`go build` 不需要 Node：`dist/` 已入库（[`.gitignore`](../../.gitignore) 里对该目录有白名单），保证只有 Go 工具链的机器也能编译。

改前端时需要先在 `web/` 装依赖：

```bash
cd cmd/relkit-console/web
npm ci
npm run dev      # 开发服务器，代理到已启动的 console
npm test         # vitest
npm run build    # 产物写入 ../internal/webui/dist/，随后 go build 即为最新界面
```

提交前请把 `npm run build` 的产物一起提交，否则二进制里还是旧界面。

## 快速开始

```bash
# 生成配置与面板 cookie key；-tree 是读 adapter 条目（JSON 文本，或 @文件 指向只含条目的 JSON）
relkit-console init -tree '{"type":"relkit-compatible","url":"http://127.0.0.1:8080"}' -out /etc/relkit-console

# 起服务（默认只听本机，按需加 -addr）
relkit-console -config /etc/relkit-console/relkit-console.json
```

浏览器打开 `http://127.0.0.1:8081/-/admin`，用 init 打印的一次性令牌创建第一个操作员。

面板锁死（无操作员、bootstrap 失效）时的破窗路径：

```bash
relkit-console init -reset-admin -out /etc/relkit-console   # 重置 bootstrap，清空全部操作员
systemctl restart relkit-console
```

存储侧初始化（upload token、product token）不在此处，用 `relkit-store init`。

## 写操作走 agent

注册产品、签发/轮换/吊销令牌、下线产品都需要 agent。用 `-agent` 或配置里的 `"agent"` 指向它：

```json
{
  "addr": "127.0.0.1:8081",
  "tree": {"type": "relkit-compatible", "url": "http://127.0.0.1:8080"},
  "agent": "http://127.0.0.1:8787"
}
```

产品根目录留空时，由 agent 决定落在哪：优先用它配置的 `productRootParent`，否则从已注册产品的共同父目录推断。两者都得不到时注册会被拒（报 `no default product root is configured ...`），不会退回到某个写死的路径。

## 数据从哪来：adapter

console 不认识任何具体存储。所有读取走一个 adapter 接口（`adapters.go`），具体实现由配置里的 `tree` 条目决定：条目写 `type` 和该实现自己的参数，工厂 `newAdapter` 按 `type` 选实现，入口只拿到 `Adapter` 接口。形状与存储面 Backend（`internal/backends`）相同，type 名也共用。

```go
type Adapter interface {
    Name() string
    ReadKey(key string) ([]byte, error)
    ReadDir(dir string) ([]Entry, error)
    ModTime(key string) time.Time
}
```

```json
"tree": {"type": "relkit-compatible", "url": "http://127.0.0.1:8080"}
```

可用实现：

| type | 参数 | 说明 |
|---|---|---|
| `relkit-compatible` | `url`（必填，http/https） | 经 HTTP 读 relkit-compatible 树：`ReadKey` 走 GET（no-cache 前缀与 store 一致），`ReadDir` 走 `/-/list/<dir>` JSON 端点（服务端已过滤保留键）。console 与 store 同机也走这条路，内外网同一形态 |

`tree` 必填；缺 `type`、未知 `type`、条目里出现该实现不认识的参数都会在启动时报错并列出可用 type。命令行 `-tree` 可覆盖配置（JSON 或 `@文件`）。新增实现（例如 `s3-compatible`）只在 `adapters.go` 注册一个 type，入口、部署脚本、配置顶层都不用改。

旧配置的顶层 `dir`、`store`、`stateDir` 会被拒绝启动，提示用部署脚本 `upgrade` 迁移；本机目录直读的 `rootAdapter` 已删除。

## 布局约定

- 发布树：只经读 adapter 读，console 没有 `-dir`。
- 面板状态（会话签名 key）：`/var/lib/relkit-console/panel.json`，由 systemd `StateDirectory=relkit-console` 提供，不在发布树里。`-state`/配置 `adminStateFile` 可改，必须是绝对路径。
- 账户与 token：由 agent 持有，console 通过 agent 的 API 读写。
- 下载计数：暂不提供（console 与 store 两侧均已删除），以后另行设计。

## 与旧界面的差异

旧的手写 HTML 模板（`templates/*.html`）与 `panel.go`/`ui.go` 里的渲染代码已删除，不保留两套界面。旧 URL 全部 302 到新路由，不会 404：

| 旧 URL | 现在 |
|---|---|
| `/-/admin/panel` | `/-/admin/products` |
| `/-/admin/panel/users`、`roles`、`products`、`tokens` | `/-/admin/users`、`roles`、`products`、`tokens` |
| `/-/admin/panel/tokens?product=<id>` | `/-/admin/products/<id>/tokens` |
| `/-/p/<id>` | `/-/admin/products/<id>` |
| `/-/admin/login`、`/-/admin/setup` | `/-/admin/sign-in`、`/-/admin/` |

`/-/latest/*` 与 `/-/health` 的路径与行为保持不变。

能力差异：旧界面的 Site 卡片（makers Pages 部署列表与 `site/status.json` 新鲜度）随旧界面一并移除，等价信息看 agent 侧。

## 测试

```bash
go test ./cmd/relkit-console/
```

前端另有 `cd web && npm test`（见上）。
