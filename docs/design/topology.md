# 拓扑（唯一来源）

---
title: 拓扑（唯一来源）
category: design
created: 2026-10-26
updated: 2026-10-26
status: approved
依据: ADR 0023
---

本文是 relkit 拓扑图的唯一来源。其它文档需要画拓扑时引用本文的对应小节，不再各画一份。图分散在多处必然漂移，而漂移过的图比没有图更误导。

本文只画结构，不重复协议细节。协议看 [SPEC.md](../../SPEC.md)，规矩看 [architecture.md](../architecture.md)，历史决策看 [ADR](../adr/)。

判据只有一条：图和文字里的每个事实，都能在代码或现网配置里找到对应。现网配置的样例在 `scripts/deploy/`（内网 `nginx-intranet.example.conf`，外网 `nginx-public.example.conf`）。

五张图分工：图一总览一台服务器上有什么、域名转到哪；图二发布链路（CI 写面）；图三下载链路（updater 读面）；图四信任与凭据；图五一次发布的时序。

三句话概括现状：每台服务器三个进程；三组域名各管一件事；发布与下载是两条独立链路，唯一交汇点是 store 里的发布树。

## 一、总览

```plantuml
@startuml
skinparam componentStyle rectangle
skinparam shadowing false
left to right direction

package "宿主侧" {
  [宿主 CI] as ci
  [薄 IPC 客户端] as thin
  [relkit-updater 引擎] as engine
}

[运营浏览器] as op
[装包的人] as human

node "服务器（内网 1 台 / 外网 1 台，同构）" {
  node "nginx" {
    [publish.*] as vpub
    [update-internal.*] as vint
    [update.*] as vupd
  }
  [relkit-agent\n127.0.0.1:8787] as agent
  [relkit-store\n:8080] as store
  [relkit-console\n127.0.0.1:8081] as console
  database "发布树\n内 /data/relkit-serve\n外 /srv/releases" as tree
  folder "根私钥\n凭据清单" as key
  folder "人页静态目录\n外 /srv/relkit/site\n内 发布树 browse/" as site
}

ci --> vpub : 取工具 / CAS 直传 / 发布
op --> vpub : /-/admin
engine --> vint : directory / index / manifest / artifact
human --> vupd
thin --> engine

vpub --> agent : /v1/*  /-/health  /-/site
vpub --> console : /-/admin  /-/p/  /-/latest/
vpub --> store : 其余（CAS 能力 URL PUT、兼容读）
vint --> store : 只 GET/HEAD
vupd --> site : 人页

agent --> key : 签名
agent --> store : Promote / 写 manifest / 写 index 指针
agent --> site : site rebuild
console --> agent : 写操作走 /v1/admin/*
console ..> tree : 只读
store --> tree
@enduml
```

三组域名各管一件事，这是内外网同构的部分：

| 域名 | 唯一职责 | 转发目标 |
| --- | --- | --- |
| `publish.*` | CI 写面 + 运营面板入口 | `/v1/`、`/-/health`、`/-/site` → agent 8787；`/-/admin`、`/-/p/`、`/-/latest/` → console 8081；其余 → store 8080 |
| `update-internal.*` | updater 读面（SSOT） | 全部 → store 8080，仅 GET/HEAD；不转发 `/v1/`，所以不提供 `/v1/tools/` 与 `/v1/catalog` |
| `update.*` | 人页 | 外网：nginx 直接伺服静态目录 `/srv/relkit/site`，没有 console。内网：转 store，读发布树里的 `browse/`；该 vhost 上残留的 `/-/admin` 等 console 路由属于迁移期遗留，面板的正式入口是 `publish.*/-/admin` |

三个进程的职责边界：

- `relkit-agent` 是唯一持有根私钥、唯一做签名的进程。它签 Profile / Catalog / directory / index / manifest 与工具的 SSH 签名，签发 CAS 上传凭据，接收 `POST /v1/publish`，经 store 的接口完成 Promote、写 manifest、写 index 指针，最后跑 site rebuild。它自己不经手产物正文。
- `relkit-store` 管发布树：匿名 GET/HEAD（含 Range），凭能力 URL 接收 CI 直传的 `cas/<sha256>`，接受 agent 的受控写入。不含签名能力。
- `relkit-console` 按 adapter 只读读取发布树（本机目录，或 `-store` 指定的 store URL）。用户、角色、产品、token 这些写操作全部转 agent 的 `/v1/admin/*`，自己不写发布树。

所以"谁写发布树"有两个入口：CI 凭 agent 签发的能力 URL 直接 PUT `cas/`；其余写入（`artifact/`、`manifest/`、`index/`、人面数据文档）都由 agent 经 store 接口完成。

内外网不是同一套配置的复制品，差异只有这些：

| 项 | 内网 | 外网 |
| --- | --- | --- |
| 域名 | `*.devcloud.woa.com` | `*.firoyang.com` |
| TLS 终结 | WOA 网关在箱外终结，箱上只 `listen 80` | nginx stream 按 SNI 分流（`stream-sni.conf`）→ `127.0.0.1:4443`，vhost 用 `ssl proxy_protocol` |
| CI 写面 scheme | `http://`（蓝盾池到 443 被拦，箱上无 443 监听；已接受的偏离 D2） | `https://` |
| store 数据目录 | `/data/relkit-serve` | `/srv/releases` |
| agent 产品根 | `/data/relkit` | `/srv/relkit` |
| 人页 sink | `type: backend`，写进发布树 `browse/` | `type: backend` + `type: directory`（`/srv/relkit/site`），nginx 直接伺服目录 |
| agent `publicURL` | `http://publish.devcloud.woa.com` | `https://publish.firoyang.com` |

CI 一律打域名，不打裸 IP、不打回环 hostname，也不建 SSH 隧道借道 loopback 端口。

## 二、发布链路

发布是从 CI 到发布树的一条单向链。字节只跨「CI → 数据面」一次，之后的写入都在服务器内部完成。

```plantuml
@startuml
skinparam shadowing false

actor "宿主 CI" as ci
participant "nginx publish.*" as ngx
participant "relkit-agent" as agent
participant "relkit-store" as store

ci -> ngx : GET /v1/tools/current/<工具>/<平台>\n+ /signature
ngx -> agent
agent --> ci : 工具字节 + SSH 签名
ci -> ci : ssh-keygen -Y verify\nallowed_signers = 项目里的根公钥\n通过才执行
ci -> ngx : GET /v1/runtime-profile、/v1/catalog
ngx -> agent
agent --> ci : 根签文档，工具用根公钥验签

ci -> ci : relkit stage\n多端产物收成一棵树\n本地算 sha256
ci -> ngx : POST /v1/cas/credentials（Bearer = 产品 token）
ngx -> agent
agent --> ci : 唯一目的地：store 的能力 URL
ci -> ngx : PUT cas/<sha256>\n每个 blob 恰好一次
ngx -> store
ci -> ngx : PUT /v1/staged 控制面五件套
ngx -> agent
ci -> ngx : POST /v1/publish
ngx -> agent
agent -> store : Head 比 size，不重算 sha256\nPromote cas → artifact
agent -> store : PutImmutable manifest（根签）
agent -> store : 写 index 指针（根签）= 真发布
agent -> store : 写人面数据文档\nsite/ latest/ channel/ release/
agent -> agent : site rebuild → 人页 sink
@enduml
```

要点：

- 宿主仓不放 `relkitw`、不钉 relkit 版本、不放组件摘要。取工具固定三步：下载、验签、执行，脚本里不含版本号与摘要，所以能写进流水线 yaml 并冻结。
- 工具只在 `publish.*` 上提供。下载地址由 agent 用自己配置的 `publicURL` 签出，调用方传什么 baseUrl 一律忽略；`publicURL` 没配或配成 loopback 时拒绝发布工具并返回 409。
- 写 index 指针才算发布完成。此前都只是把字节搬到位。
- 下线产品是不可逆操作：注册消失、token 失效，发布树目录留在磁盘上。面板会先做一次预演，把 agent 的判断逐条列出来，确认后才真删。

## 三、下载链路

下载是另一条独立链，客户端从不接触 agent，也不持有任何 token。

```plantuml
@startuml
skinparam shadowing false

actor "宿主应用" as app
participant "薄 IPC 客户端" as thin
participant "relkit-updater" as engine
participant "nginx update-internal.*" as ngx
database "store 发布树" as tree

app -> thin : 检查更新\n透传 Runtime Bootstrap/2 原文
thin -> engine : 4 字节大端长度前缀 + protobuf 帧
engine -> engine : 用 bootstrap 内嵌根公钥\n验根签 Runtime Profile\n校验 product / channel\n从 Profile 取 entryUrls
engine -> ngx : GET directory/<product>.pb
ngx -> tree
engine -> engine : 验根签\ndirectory_sequence 防回滚
engine -> ngx : GET index/<product>/<channel>.pb
engine -> engine : 验根签 + sequence 防回滚\n解升级链选下一跳
engine -> ngx : GET manifest/<product>/<version>.pb
engine -> engine : 验根签\n按 urls[] 顺序回退
engine -> ngx : GET artifact/...（Range 续传）
engine -> engine : 先比 size 再验 sha256
engine --> thin : 事件
thin --> app
@enduml
```

要点：

- 薄客户端只负责找引擎、起子进程、编解码帧、把事件交给 UI。解析 bootstrap、解析 index/manifest、验签、算摘要、决定版本、管理安装目录全部在引擎里。任何一处客户端开始解析协议文档，就是架构被破坏。
- 入口地址不由宿主手写：entryUrls 写在根签的 Runtime Profile 里，宿主侧若另行声明，必须与之一致，否则报 `PROFILE_INVALID`（`internal/updater/engine.go`）。
- 验签失败不降级、不换源、不使用明文。防回滚只有前进。
- 下载不需要 token。内容由根签名保证真伪，鉴权只在写面存在。

## 四、信任与凭据

两样东西，各管一件事，不能互相替代。

```plantuml
@startuml
skinparam shadowing false
left to right direction

rectangle "服务器根（Ed25519，每台服务器一把）\n私钥只在这台服务器 + 离线备份\n证明：这是这台服务器发的、没被改过" as root
rectangle "token（每条绑定唯一产品）\n服务器只存 hex(sha256)\n证明：这个宿主能操作哪个产品" as token

rectangle "Runtime Profile" as p
rectangle "Catalog" as c
rectangle "directory / index / manifest" as d
rectangle "工具 SSH 签名\nnamespace relkit-tool" as t
rectangle "artifact\n不签名，只由 manifest 里的\nsize + sha256 绑定" as a

rectangle "写面\n/v1/cas/*  /v1/staged/  /v1/publish\n/v1/unpublish" as w

root --> p : 签
root --> c : 签
root --> d : 签
root --> t : 签
d ..> a : 摘要绑定
token --> w : 鉴权

rectangle "宿主项目配置\nserver + rootPublicKey + product" as host
rectangle "宿主 CI 密钥\n只注入 token" as cisec
host ..> root : 只持公钥
cisec ..> token
@enduml
```

要点：

- 文档靠根签名认证，文档里点名的文件靠摘要绑定。两层就够，没有产品级签名密钥，也没有第二层签名文档。
- 一个应用只认一台服务器的根。根公钥来自那台服务器产出的接入配置，原样写进宿主项目配置；CI 用它验工具，宿主构建把它写进 Runtime Bootstrap 供引擎使用。
- token 不做任何验证服务器或验证内容的事。读面（`update-internal.*`）完全不需要 token。
- 运营账户（`/v1/admin/*`）是第三种凭据：登录会话，由 agent 管理，只用于面板，不进 CI，也不进客户端。
- 钥匙位置只登记在凭据清单（`relkit.custody-manifest/1`），程序按 keyId 取，规矩见 [architecture.md](../architecture.md) 第三节。

## 五、一次发布的时序

把发布与下载串起来看，一次完整发布长这样。

```plantuml
@startuml
skinparam shadowing false

participant "宿主 CI" as ci
participant "nginx publish.*" as ngx
participant "relkit-agent" as agent
database "store" as store
participant "薄客户端 + 引擎" as client

ci -> ngx : 取工具（下载 → 验签 → 执行）
ngx -> agent
agent --> ci : 工具
ci -> ci : relkit stage 收树算摘要\n版本闸门
ci -> ngx : POST /v1/cas/credentials
ngx -> agent
agent --> ci : 目的地（store 能力 URL）
ci -> store : 经 publish.* PUT 缺失的 cas blob
ci -> ngx : PUT /v1/staged
ci -> ngx : POST /v1/publish
ngx -> agent
agent -> store : Promote → artifact
agent -> store : PutImmutable manifest
agent -> store : 写 index 指针
agent -> store : 写人面数据文档
agent -> agent : site rebuild（人页）
agent --> ci : 发布结果
== 此后客户端才看得到 ==
client -> store : 经 update-internal.* GET directory / index / manifest / artifact
client -> client : 逐层验签，通过才装
@enduml
```

时序上有一处容易误会：人页永远滞后于协议面。数据文档写在 index 提交之后，某次 rebuild 失败只让人页旧一会儿，不会回滚已经生效的协议 index。

## 相关

- [architecture.md](../architecture.md)：现行架构，哪些规矩不能破
- [ADR 0023](../adr/0023-desktop-updater-epoch.md)：桌面更新断代决策，本文的结构以其为准
- [publish-agent.md](publish-agent.md)：发布协议与 HTTP 表面细节
- [SPEC.md](../../SPEC.md)：协议与客户端内嵌合约
- [trust-root-custody.md](../guide/trust-root-custody.md)：根密钥放哪、怎么确认在
