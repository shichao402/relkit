# relkit-console

> 本目录是 [`go.firoyang.com/relkit`](https://github.com/shichao402/relkit) 仓库的一部分（`cmd/relkit-console`），与发布 CLI 同仓。设计决策见 [ADR 0016](../../docs/adr/0016-serve-split-store-console.md)。

relkit-serve 拆分出的**管理面**：`/-/admin` 操作面板、账户/会话（ADR 0006 的 bootstrap 流程不变）、下载统计展示、文件树浏览、`/-/latest/` 固定链接跳转。单个静态链接的可执行文件，无运行时依赖。

**角色：** 只读的管理与观察面。它不伺服发布树（GET `/` 不是它的职责，那是 store/browse 的），不做任何写操作（发布、token 轮换、GC 仍是 agent/store 的职责）。面板是人类鉴权面（用户名密码 + 会话 cookie），与数据面令牌（Bearer PUT）是两种信任域，进程隔离让一个 0day 拿不到写者权限。

store 拆包已完成（ADR 0016 第 3 步）：`cmd/relkit-serve` 已删除，`cmd/relkit-store` 承载纯存储面。console 与 store 可同机部署（同 `-dir`），console 只读。

## 快速开始

```bash
# 生成配置与一次性 bootstrap 令牌
relkit-console init -dir /srv/releases -out /etc/relkit-console

# 起服务（默认只听本机，按需加 -addr）
relkit-console -config /etc/relkit-console/relkit-console.json
```

浏览器打开 `http://127.0.0.1:8081/-/admin`，用 init 打印的一次性令牌创建第一个操作员。

## 数据从哪来：adapter

console 不直接假设数据在本机盘上。所有面板读取走一个 adapter 接口（`adapters.go`）：

```go
type Adapter interface {
    Name() string
    ReadKey(key string) ([]byte, error)
    ReadDir(dir string) ([]Entry, error)
    ModTime(key string) time.Time
}
```

- `rootAdapter`：只读本机发布树（`os.Root` 沙箱）。与 store 同机部署时的默认形态，也是当前唯一实现。
- `storeAdapter`（HTTP 经 relkit-compatible client 读 relkit-store）与 makers 只读查询是后续增强，见 ADR 0016 的排期。

面板的扫描逻辑（`ui.go` 的 scanProducts/readProductCard 一族）只认 adapter，不因数据面是本机盘还是远端而分叉——与 `internal/backends` 的 Backend 接口同一设计语言。

## 部署快照

若 agent state 目录下存在 `site/status.json`（site-rebuild 成功后落盘的部署事件快照，见 ADR 0016），console 会展示各 sink 的部署结果与 makers deployment ID。快照缺失或损坏时面板安静降级，不报错。

## 布局约定（与 store 同形，方便迁移）

- 发布树：`-dir` 指向 store 伺服的同一目录（只读）。
- admin state：`<dir>/.relkit-serve-admin.json`，文件名与格式不变，机器从 serve 迁到 console 时账户保留。
- 下载计数：`<dir>/.relkit-serve-stats.json`，与 store 共享；console 只读展示，计数由伺服树的一方记录。
- agent state（site 快照）：`-state-dir` 指向 agent 的 state 目录，缺省同 `-dir`。

## 与 serve 面板的差异

| 能力 | serve（旧） | console |
|---|---|---|
| `/-/admin` 面板、登录/账户 | 有 | 有（同模板同流程） |
| `/-/p/` 产品页、`/-/latest/` 跳转 | 有 | 有 |
| 目录浏览（`/-/admin/files`） | 有 | 有（经 adapter） |
| 下载计数 | 记录并展示 | 只读展示（共享 stats 文件） |
| GET `/` 树、PUT/DELETE、CAS、GC | 有 | **无**（store 的职责） |
| `browse/` 静态页伺服 | 有 | **无**（store/CDN 的职责） |

## 测试

```bash
go test ./cmd/relkit-console/
```
