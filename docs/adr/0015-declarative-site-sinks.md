# ADR 0015: browse dump 部署 sink 声明化，并新增 directory sink

- Status: Accepted（已实现：internal/site sinks、agent site.sinks 解析、directory sink、迁移脚本与巡检均已落地）
- Date: 2026-09-25
- Context: [publish-topology.md](../design/publish-topology.md)（拓扑总览）、[publish-agent.md](../design/publish-agent.md) §人页 rebuild（现状机制）。

## 背景

人页（browse dump：`index.html` / `<product>.html` / `catalog.json`）由 `relkit-agent site-rebuild` 对全部产品的数据面事实静态重建，一次性渲染完整 dump 后分发。现状（`internal/site/site.go`）：

1. **Makers sink 是写死的特例。** `site.makers`（projectId / region / tokenEnv）写在 agent 顶层配置里，`collect()` 里 `makersCfg != nil` 就追加一个 `makersSink`。它是 sink 平面里唯一一个不依赖产品 profile 的成员，类型也只有一个。
2. **其余 sink 从存储拓扑推导，不声明。** 每个产品 profile 的 `publishTo` 后端里，`HostsBrowse()==true` 的自动成为 sink（目前满足该 flag 的只有 `relkit-compatible` 一个 backend 类型）。「页面落在哪里」不是独立决策，被「存储后端恰好是谁」绑架。
3. **`HostsBrowse` 语义是混的。** 它把「技术上能伺服 HTML」（几乎任何 GET 树都行）和「策略上允许 HTML 进这里」（COS 不放 HTML 是设计决定，见 [update-ingress-cos.md](../design/update-ingress-cos.md)「协议客户端不读 HTML」）压成一个布尔。策略只能跟着 backend 类型硬编码，不能按部署选择。
4. **Makers 不可用时没有第二个格子。** Makers 是外部依赖（Pages OpenAPI + token）；它挂了、欠费、被关或迁移时，人页没有可切换的落点。想退到「薄 nginx 伺服一个本地目录」这条路，现状也不存在：`local` backend 已删除，dump 是内存里的 `map[string][]byte`，推完 sink 即消失，没有「写目录给外部哑静态服务」的出口。
5. **远期管理层需要 sink 平面独立存在。** 管理层的目标是在一处管理多个后端存储和页面宿主；sink 若继续寄生在产品 profile 的拓扑推导里，管理层无从枚举、切换页面宿主。

## 决策

1. **agent 顶层新增 `site.sinks[]`，sink 声明化。** 每个 sink 是一个对象，带 `type` 字段与该类型自己的配置。`site-rebuild` 以 `site.sinks[]` 为准分发 dump；产品 profile 不参与 sink 选择，产品 `relkit.json` 照旧只提供 site 文案。
2. **首批三类 sink：**
   - `makers`：现行为，配置即现 `site.makers`（projectId / region / tokenEnv）。
   - `backend`：指定一个已配置 backend 的名字，把 dump `PutPointer` 到该 backend（现 `backendSink` 行为）。内网 `relkit-compatible` 数据面即 `{"type": "backend", "backend": "<name>"}`。
   - `directory`：把 dump 写入本机（或挂载）目录，原子替换（写临时目录 + `rename`），外部静态服务（nginx / Caddy / 任意静态宿主）伺服该目录。这是「Makers 不可用时切到可用服务」的保底格子。
3. **`site.makers` 短期兼容、长期淘汰。** 配置加载时 `site.makers` 等价展开为一个 `{"type": "makers", ...}` sink；两处同时配置视为错误。发布机脚本升级时迁移旧配置并提示新写法，之后在 CHANGELOG 标记 deprecated，最终移除。
4. **`HostsBrowse` 从「自动 sink 推导」降级为「能力探测」。** `site-rebuild` 不再自动把 `HostsBrowse` 后端注册为 sink；该 flag 保留，仅用于校验 `{"type": "backend"}` sink 指向的后端是否具备伺服完整 dump 的能力（目录树 GET、按 content-type 返回）。COS 桶不写进 sinks 就是不放 HTML；写进就是显式允许。策略从 backend 类型硬编码变为每次部署的显式选择。
5. **dump 产物本身也落 `directory`。** `site-rebuild` 总是先写本机目录（agent state 目录下 `site/dump/`），成功后再分发到各 sink。这份本机副本既做 `directory` sink 的源，也做审计/回滚参照，与 ADR 0011 的「不信任网络读」一致——重建依据永远是本地事实。
6. **sink 部署失败不影响协议发布。** 协议数据面（`site/` / `latest/` 指针）与人页分发解耦：sink 部署失败时 publish 仍是成功（协议对象已落），`site-rebuild` 报错退出，修好 sink 配置后单独重跑 `relkit-agent site-rebuild`，不发新版本。这回答了 publish-config-lessons 的待决项「缺 Makers token 时 publish 是 warning 还是失败」：rebuild/sink 阶段是非协议路径，失败不阻断协议面。
7. **不写新的页面服务器。** `relkit-pages` 之类的自建人页服务不做。静态托管是被 nginx / Caddy / Makers / 任意静态服务彻底解决的问题，relkit 的职责止于「把 dump 可靠地落到宿主手里」；`directory` sink + 外部哑静态服务即完整方案。
8. **保持 dump 分发的幂等与护栏。** `dump.sha256`（含 sink 名单）与 `members.json` 完整快照护栏不变：sink 集合变化导致 hash 变化属预期重部署；快照缺失产品仍拒绝重建。

## 配置示例

```json
{
  "site": {
    "sinks": [
      {"type": "makers", "projectId": "makers-xxx", "region": "china", "tokenEnv": "EDGEONE_PAGES_API_TOKEN"},
      {"type": "backend", "backend": "intranet-serve"},
      {"type": "directory", "path": "/srv/relkit-site"}
    ]
  }
}
```

内网最小配置（无 Makers）：`sinks: [{"type": "backend", "backend": "..."}]` 或仅一个 `directory`。公网最小配置：仅 `makers` 一项，行为与今天完全一致。

## 后果

- agent 配置 schema 变更，`relkit-agent.json` 需要迁移（`site.makers` → `site.sinks[]`），发布机脚本升级要处理旧配置。
- `site-rebuild` 失败语义变清晰：区分「数据不完整（成员护栏）」与「某 sink 部署失败」，后者不污染 `dump.sha256` 状态（失败时不写新 hash，重跑自然全量重发）。
- `directory` sink 要求目标目录可写且外部静态服务已配置好；agent 只负责写文件，不负责 nginx 配置管理。
- 多 sink 全量分发，dump 很小（三个文件/产品），成本可忽略；不做增量分发。
- 本 ADR 只声明 sink 平面。远期管理层对多后端/多宿主的编排不在本 ADR 范围，但它依赖本 ADR 的声明化前提。
