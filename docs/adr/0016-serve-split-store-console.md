# ADR 0016: serve 拆分为 relkit-store 与 relkit-console，site-rebuild 落部署快照

- Status: Accepted
- Date: 2026-09-25
- Context: [publish-topology.md](../design/publish-topology.md)（拓扑总览）、[ADR 0015](0015-declarative-site-sinks.md)（sink 声明化）、[ADR 0006](0006-admin-panel-bootstrap.md)（面板鉴权）

## 背景

relkit-serve 现在承载三个平面：

1. **存储面**：`/` 树的 GET/PUT/DELETE、Range、CAS mint、GC、上传令牌——完整 `relkit-compatible` 数据面。
2. **管理面**：`/-/admin` 操作面板、账户/会话、下载统计、文件树浏览。
3. **页面壳**：catalog stub、`/-/latest/` 跳转、`browse/` 目录页。

三平面在代码中按路由天然分界（存储面 `handler.go`/`upload.go`/`gc.go`/`capability.go`，管理面 `admin.go`/`ui.go`/`stats.go`），共享状态只有下载计数（store 记录、面板展示）与面板要看的目录列表。serve 已是单机瓶颈：内网它是数据面本体，外网它只是操作面壳（本机空目录，`/-/admin` 只能看本机盘），未来面板要长成管理 COS 的后台，路径已不兼容（面板扫盘逻辑写死「扫本机目录」，换了数据面就瞎）。

同时，拆分后的 console 需要能回答「pages 现在什么样」：Makers 侧部署成没成、本机 dump 副本何时落盘。现状 rebuild 只有 `dump.sha256` 一个哈希状态文件，部署结果（deployment ID、各 sink 成败）无处可查；要修 sink 需要上机看日志。

## 决策

### 1. serve 拆为两个二进制：relkit-store 与 relkit-console

- `relkit-store`：纯存储面。`/` 树读写、`/-/health`、`/-/version`、CAS mint、GC、上传令牌、preflight。目录改名一次到位（`cmd/relkit-store`），协议不变——客户端与 CI 无法感知换了二进制。
- `relkit-console`：管理面独立二进制。`/-/admin` 面板、账户/会话（ADR 0006 不变）、`/-/latest/` 跳转、文件树浏览、统计展示。挂 `--store <url>` 或本机盘只读挂载，经 adapter 读存储事实。
- agent 不动。发布路径上任何组件都不感知拆分。
- **为什么不用多进程合并**：面板是人类鉴权面（用户名密码+会话 cookie），与数据面令牌（Bearer PUT）是两种信任域。合并进程意味着一个 0day 让写者拿到面板会话；独立进程 + 只读 adapter 是爆炸半径的隔离。

### 2. console 的 adapter 层：读统一接口，写不进 console

console 对存储事实的访问统一走一个小接口层，所有面板数据经 adapter 进来，不直接摸 store 的文件系统：

```go
// internal/console/adapters.go
type Adapter interface {
    Name() string                      // 展示名
    ReadKey(key string) ([]byte, error) // 读一个 RUP 键（index/site/latest 前缀）
    ListDir(dir string) ([]Entry, error) // 目录浏览
}
```

- 首批两个实现：`storeAdapter`（经 HTTP GET 树访问 relkit-store，复用 `backends` 包的 `relkit-compatible` client）与 `dumpAdapter`（只读本机 agent state 的 `site/dump/`）。
- makers 的只读查询（Pages OpenAPI deployments 列表）是后续增强项，届时加第三个实现，不阻塞本次拆分。
- **写操作不进 console**。发布、token 轮换、GC 触发仍是 agent/store 的职责；console 面板对它们只展示。这是管理层与数据面的边界，防止 console 变成第二个发布入口。
原则与 Backend adapter（`internal/backends/base.go`）一致：切面不因 type 分叉，面板不因数据面是本机盘还是 COS 而分叉。

### 3. site-rebuild 落 `status.json` 部署快照

rebuild 成功路径的末尾（`dump.sha256` 写入之后、members 写入之前）加写一个 `site/status.json`：

```json
{
  "at": "2026-09-25T14:30:00+08:00",
  "sinks": [
    {"name": "makers:relkit-updates-index", "ok": true, "deploymentId": "2g7..."},
    {"name": "directory:/srv/relkit-site", "ok": true}
  ]
}
```

- 各 sink 的 `deploymentId`：makers sink 从 `DeployDump` 返回的 `Result.DeploymentID` 透出。为此把 `sink.Deploy` 的返回值从 `error` 扩展为 `(*DeployResult, error)`，`DeployResult{DeploymentID string}`。这是 sink 接口的唯一改动，与 `Name()`/`Deploy()` 极简形状保持一致。
- `directory`/`backend` sink 的 `DeployResult` 为零值（无云端 deployment ID 概念），status.json 里 `deploymentId` 留空。
- **sink 失败即中止本轮 rebuild，不写快照**：快照只记录全量成功的部署事件，一个 sink 失败时面板上不留"半轮"状态；下轮 rebuild 成功时快照整体重写。失败原因走 rebuild 的错误返回（agent 日志）。
- `status.json` 写入失败不阻断已完成的部署：sink 都成功后写快照失败只影响可见性，rebuild 的错误返回让 agent 日志可见，dump 本身已分发。
- console 读 `site/status.json` 与本机 dump 一起展示：本机 dump 的 mtime + 各 sink 部署结果。
- 快照与 `dump.sha256` 对称：`dump.sha256` 记「内容指纹」，`status.json` 记「部署事件」。不变式：sink 失败不写新 hash（下轮全量重发）也不写 status.json（面板上这轮没有新事件）；`unchanged` 路径不重写快照，快照仅记录有实际部署动作的 rebuild。

## 实现顺序

1. site 包：sink 接口扩展 + `status.json` 写入（已完成）。
2. console 拆包 `cmd/relkit-console`（已完成，a51e05e）：搬运 admin.go/ui.go/stats.go + templates，adapter 层首版（rootAdapter），`-dir` 只读挂载。
3. store 拆包 `cmd/relkit-store`（已完成）：serve 目录改名一次到位并删除，协议不变；部署侧（unit 模板、deploy CLI、hostlib、CI、文档）同步切换。`RELKIT_SERVE_TOKEN` 环境变量名与 `.relkit-serve-admin.json` / `.relkit-serve-stats.json` / `.relkit-serve-cas.key` 数据文件名保留旧值，迁移盒子不重配 token、不丢账户与计数。
4. 部署切换与收尾文档（待实机执行）：两台机器 systemd/nginx 切换、`relkit-store.service` 上线、console 独立 unit。

## 后果

- serve 命名已退役：`cmd/relkit-serve` 删除，`cmd/relkit-store` / `cmd/relkit-console` 为唯二服务二进制。协议、配置形状、数据文件名对客户端与运维面保持不变。
- 面板功能短期回退：console 首版只读，不做账户管理 CRUD（继续用 `init -reset-admin` SSH 路径），统计与目录浏览迁走。
功能不回退：console 首版保留现有全部面板能力（登录/账户、统计、目录浏览、产品页），只是数据来源改走 adapter。
- `ui.go` 的 scanProducts/readProductCard 一族读盘逻辑成为 storeAdapter 的主体，逻辑不变，只是换了调用方。
- 测试搬迁：admin_test/ui_test/stats_test 随 console 走，handler/capability/gc/upload 测试随 store 走。
- ADR 0006 的 bootstrap 流程不变，admin state 文件继续由 console 持有。
- deploy 脚本与 systemd 单元已切换（`relkit-store.service`；console 独立 unit 留在部署切换轮）。
- ROADMAP「操作面板从本机盘长成发布管理」的迁移路径不变：console + adapter 是它的落地形态。
