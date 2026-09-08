# Publish Agent（CI → 发布机 → COS）

---
title: Publish Agent
category: design
created: 2026-08-12
updated: 2026-09-07
status: approved
related: docs/design/update-ingress-cos.md, docs/design/publish-topology.md, CLI.md, cmd/relkit-agent/README.md
---

## 1. 决议

CI **不持** RUP 签名私钥，也 **不持** 长期后端写密钥。  
CI `relkit stage` 后走 **同一套** CAS 协议：向凭据文档里那**唯一一个**目的地上传一次；agent 持钥 `publish.Run`（Promote / Materialize + 签指针），其余后端的副本由 agent 在数据面之间分发。现网代码仍接受整包 `PUT /v1/staged`。

数据面只是 API 不同：公网 `s3-compatible` → COS；内网 `relkit-compatible` → relkit-serve。客户端永远不连 agent。这条改动是为了让发布机只做管理角色，**不是**为了跨境加速。

## 2. 信任边界

发布配置拆成两份，在 agent 上合并后才调用 `publish.Run`：

| 文件 | 谁写、放哪 | 装什么 | 禁止装什么 |
|---|---|---|---|
| `release-policy.json` | 仓库 `relkit stage` 写入 staged 树，随元数据上传 | 产品 id、通道、`codeStrategy`、公钥、directory 入口、site 文案 / Makers 项目 id | 私钥路径、后端凭据、`ingest` / `artifactTo` / `pointerTo`、Makers `tokenEnv`、changelog 本地 `file` |
| publish profile | 发布机 `/etc/relkit-agent/products/<product>.json` | `product` + `signing.keyId`（与 policy 对齐）、私钥 env/path、backends、`ingest`、`artifactTo` / `pointerTo`、directory 发布目标、Makers `tokenEnv` | 公钥集、通道策略、entryUrls |

产品根上的 `relkit.json` **不是** agent 发布配置。仓库里那份只给本地 / CI 的 `relkit stage` 抽 policy 用；发布机上若还留着旧副本，应迁 profile 后改名为 `relkit.json.migrated` 或删掉。

`product` 与 `signing.keyId` 必须两边一致，否则拒绝发布。产品树根永远是 agent 配置里的 `products.<id>.root`，不以 JSON 文档里的路径为准。

| 角色 | 持有 | 不持有 |
|---|---|---|
| CI runner | 源码、产物、**该产品** Bearer、portable policy；发布瞬间执行凭据文档中的 `requests[]` | 签名私钥、长期后端写密钥、publish profile、别的产品的 token |
| relkit-agent | 私钥、长期后端凭据（填凭据文档 / Promote / 写指针）、本机 publish profile | 客户端下载流量；不读产品根 `relkit.json` |
| COS / 内网磁盘 | 匿名读已发布文件 | 签名动作、控制面配置 |

### 2.1 staged 树（CI 交给 agent 的内容）

agent 只需要清单与策略，不需要产物副本：

```text
.relkit/staged/<version>/
  staged.pb              # 版本、code、产物清单与哈希
  release-policy.json    # 仓库侧 portable 策略
```

CI 本机仍会有 `artifacts/`（stage 用来算 sha256）。目标路径：这些文件按凭据文档 PUT 到 **ingest 后端**的 `cas/{sha256}`，**每个 blob 一次**，与本轮有几个后端无关；交给 agent 的 tar **不含** `artifacts/`（只有 `staged.pb` + `release-policy.json`）。现网整包 tar 仍可带上 `artifacts/`，agent 解包后走 `PutArtifactCAS`。瘦 tar 路径下，缺文件是否可发布由 ingest 上是否已有对应 `cas/{sha256}` 决定（见失败语义），不要在解包时因为没有 `artifacts/` 就失败。

缺少 `release-policy.json` 或本机没有可读 profile 时直接失败。

### 2.2 publish profile

缺省路径：与 `relkit-agent.json` 同目录的 `products/<product>.json`（安装后即 `/etc/relkit-agent/products/<id>.json`）。`products.<id>.profile` 可覆盖。profile 只含端点与环境变量**名**，不含密钥明文；init 写成 `0644`，以便 root 跑 init 后 `relkit` 用户仍能读。

### 2.3 CAS：一次 ingest，之后由 agent 分发

**硬约束一：字节只跨「CI → 数据面」一次。** 产物由 CI 上传**恰好一次**到该产品的 **primary ingest**；其余 Backend 的副本由 agent 在数据面之间取得。CI 不知道本轮有几个后端。

**硬约束二：对外切面不按后端类型分叉。** CI、`publish.Run`、客户端看到同一套 key 与同一套调用。query 预签名 / 能力 URL、CopyObject / COPY、字节从哪儿来，只允许出现在 **Backend / agent 实现**里。

上一版让 CI 按 `publishTo` 里每个后端各拿一个 `putUrl`、各传一遍，**作废**。两个后端就是同样的 380MiB 跨境两次；更糟的是 git 类后端的写路径会把字节从 GitHub 送进广州 CVM 再推回 GitHub，跨境三次去搬一份本来就在 GitHub 上的文件。

#### primary ingest

profile 里每个产品声明一个 `ingest`，取值是 `artifactTo` 中某个后端名：

- 必须支持 `Head` + `Promote`（即 `s3-compatible` 与 `relkit-compatible`）。
- 公网产品选 COS，内网产品选 `relkit-compatible`。`static-http` 只读，**不可**作 ingest。
- 未声明时取 `artifactTo` 里第一个满足条件的后端；一个都没有则拒绝发布。
- **`artifactTo` 落地前：** 现网 profile 只有 `publishTo`。ingest 取 `publishTo` 里**第一个实现 `Ingest` 的后端**。一个都没有 → `POST /v1/cas/credentials` 返回 400，CI 继续整包 `PUT /v1/staged`。

`cas/{sha256}` 只存在于 ingest 后端（可加 `prefix`）。别的后端只有 `artifact/<product>/<version>/<filename>`。

#### 三层切面

| 谁 | 看见什么 | 不许看见 |
|---|---|---|
| 客户端 | 签名文档里的 `urls[]`；按 size / sha256 验收 | `cas/`、预签名、ingest 是谁、agent |
| `publish.Run` | 对 ingest：`Head(casKey)` → `Promote(casKey, artifactKey)`；对其余 artifact 后端：`Materialize(blob, artifactKey)`；再 `PutImmutable` / `PutPointer` | `Type()=="s3-compatible"` 分支、腾讯云 STS SDK |
| CI | `POST /v1/cas/credentials` → 对**尚无可 GET URL** 的 blob 各 PUT 一次 → 元数据里带上已有 URL → `POST /v1/publish` | 本轮有几个后端；按托管商写两套脚本；自己签 index |

#### artifactTo / pointerTo

`publishTo` 一把扇给所有后端是上一版的错。profile 拆两份：

| | 装什么 | 谁适合 |
|---|---|---|
| `artifactTo` | `artifact/`（几十~几百 MiB） | COS、`relkit-compatible`。默认只有 ingest 一家 |
| `pointerTo` | `manifest/` `index/` `directory/` `fallback/`（几 KB 签名 pb） | ingest + `entryUrls` 备桶（异地域 COS，见 [ADR 0007](../adr/0007-entry-mirror-must-be-reachable-and-cacheable.md)） |

`pointerTo` 里承载 `entryUrls` 的那些后端必须过 ADR 0007 三条准入：目标网络可达、`Cache-Control` 我方可配、失效域与主正交。**CNB / GitHub raw 不合格**——大陆不可达且 raw 端点缓存由平台定，而 `directory/` 是要求短缓存的可变指针。它们仍可作只读校验镜像，但不写进客户端常量。

`static-http` 永远不进 `artifactTo` / `pointerTo`，因为它不提供写接口。若外部 CI / git / rsync 已经放好对象，只把可匿名读取的绝对 URL 申报为取货点。

`pointerTo` 未声明时等于 `artifactTo`。两个列表都必须 `Writable()`。

**落地状态（2026-09-08）：** agent 侧 `PutArtifactCAS`、`POST /v1/cas/credentials`、绝对 URL `requests[]`、s3-compatible 长期钥 query 预签名、relkit-compatible 能力 URL、`relkit cas-put`、瘦 staged tar 与 **Materialize 已落地**。`casCredentials` 仅过渡接受并忽略；客户端不再接受 `sign`。**`artifactTo` / `pointerTo` 与已有外部 URL 申报仍未实现。** 现网 profile 仍只有 `publishTo`。AssumeRole 未接。

- `artifactTo` / `pointerTo` 默认都由 `publishTo` 迁移而来（未声明即全等），保持旧 profile 可用。
- 已有的 `directory.publishTo` 是 `pointerTo` 的雏形，**并进 `pointerTo`**，不要再加第三个目标列表。
- `publishTo` 可以有多家：CI 仍只喂 ingest；其余副本由 agent `Materialize`。`artifactTo` 字段落地后再把产物列表从 `publishTo` 里拆出来。

#### git 后端不是只读 static-http

CNB / GitHub 的 raw 地址只能在完整发布树已经由外部流程提交并可匿名读取后，作为只读 `static-http` 镜像。`static-http` 不提供 `Put*`，也不接受 `stageDir`；需要由 relkit 完成写入时，必须使用 `s3-compatible` 或 `relkit-compatible`，禁止把「写进工作目录、等人手或另一条 CI 去推」当成发布完成。

agent **HEAD 比 size**，**不重算 sha256**。损坏的 CAS = 这一版装不上，不突破签名。

#### 凭据文档（CI 唯一入口）

`POST /v1/cas/credentials` 的响应按 blob 给出 `requests[]`。每个请求只包含 `method`、**绝对** `url`、可选 `headers` 与 `expiresAt`；单对象上传恰好一个元素。CI 与 `relkit cas-put` 逐项执行 HTTP 请求，不认识后端类型，不实现 SigV4 / TC3 / STS，也不接受 `sign`。

agent 只向 **ingest 后端**索取请求描述：

- `s3-compatible`：发布机使用长期 `accessKeyEnv` / `secretKeyEnv` 对 PUT 生成 query 预签名 URL；COS、S3、MinIO 同形。`casCredentials` 若仍出现在旧 profile 中只会被接受并忽略，应尽快删除。
- `relkit-compatible`：agent 用 `tokenEnv` 中的 `RELKIT_SERVE_TOKEN` 调 `POST /-/cas/uploads`；serve 返回对象级能力 URL。能力签名密钥只在 serve 数据面，CAS 正文不经 agent 代理。

响应永远只有 ingest 一个上传目的地。禁止相对 URL、禁止把长期 Bearer 抄入 `requests[].headers`、禁止给 CI 两套脚本，也禁止让 CI 按后端数量循环上传。

#### Promote 与 Materialize

- **`Promote(cas, artifact)`** 只在 ingest 后端上发生：同存储内把对象变成正式 key。COS 用 CopyObject，serve 用 COPY。`Head` 命中且 size 一致时 **只 Promote**，即使 agent 盘上没有该文件（CI 已直传到 `cas/`）。缺源且本地仍有整包副本时才 `PutArtifact` 回退。
- **`Materialize(blob, artifactKey)`** 是其余可写 `artifactTo` 后端拿到副本的唯一途径：agent 取字节（从 ingest，或从 staged 已带的 URL）再交给另一 `s3-compatible` / `relkit-compatible` 后端。`static-http` 不参与 Materialize。

调用方不选实现，`publish.Run` 里不出现后端类型判断。

#### 产物已经在数据面上（GitHub Release / CNB 附件 / 已有 raw）

CI 在找 agent **之前**就建好 GitHub Release（或 CNB 附件），每个产物已有匿名（或稳定 302）下载 URL，甚至已有 sha256——这不是 GitHub 专属捷径，而是 **staged 里已经带了可 GET 的 `urls[]`**。

这时该 blob 的 ingest **已经完成，只是完成在别人家**。省略的是「CI 再传一次」，不是「其余后端不用副本」——所以判断是 **(blob × 后端)** 级，不是 blob 级：

1. staged 有 URL → agent `Head` 比 size（不重算 sha256），该 blob **不进凭据文档**，manifest 采用这些 URL 作为取货点。
2. 该 URL 所在主机不在 `artifactTo` 里 → 它只是 manifest 里多一条取货点；`artifactTo` 里的后端仍各自 `Materialize` 一份，字节由 agent 从该 URL 取。
3. 完全没有 URL → 凭据文档 PUT ingest 的 `cas/`，再 `Promote`。

跨境那一次不可约（字节必须进境才能服务国内用户），但只发生一次，且由 agent 决定方向。

agent 仍然只做管理面：合并 policy、签名、写 manifest / index / directory。客户端仍只信签名文档里的 size / sha256；GitHub 上有人替换同名 Release 资产，下载会对不上哈希，装不上，签不被突破。

这与「CAS 已存在」是同一条接口，不是第二条 CI。差别只是 URL 不一定长成 `baseUrl + artifact/...`（Release 资产是 `.../releases/download/tag/file`）。SPEC 禁止客户端自己拼路径，**允许**文档里写这种绝对 URL。

指针文档仍要落到 `pointerTo` 的**可写** Backend 上，即 `s3-compatible` 或 `relkit-compatible`。不要因为产物已经在 Release / raw URL 上，就把签名私钥下放到外部 CI 去「顺便签一下」。

GitHub → CNB（`git-cnb` 传 Release 附件再在 CNB CI 调 agent）实测比直灌广州机快一点，仍不够快，**不作为发布路径**。

#### 失败语义

上传在 CI、`Promote` / `Materialize` 在 agent，两段分属不同进程与时刻，所以：

- 凭据文档过期或 PUT 失败 → CI 自己重试。同 sha256 写同 key 是幂等的，重传安全。
- `POST /v1/publish` 时：ingest `Head` 命中 → 只 Promote，不要求本地文件；`Head` 不到且 staged 树里有该文件 → 现网整包回退（PUT cas 再 Promote）；`Head` 不到且本地也没有 → **整轮失败**，不写任何指针。
- 某个非 ingest 的 `artifactTo` 后端 `Materialize` 失败 → 该后端这一版缺副本。是继续（manifest 少一条取货点）还是整轮失败由 `--allow-partial` 决定；缺副本的后端名必须打出来。
- 指针写失败仍是原来的 `allowPartial` 语义：签名版本可能已上线，而 site / latest / browse 落后。

#### COS 存储与生命周期

仅当 ingest 是 COS 时运维侧配置；**不是**另一条发布协议。inbox 前缀 `Days=1`，分片碎片同样 1 天。刊例 0.118 元/GB/月；380MiB 存满 `Days=1` 窗口大约 **0.15–0.3 分**。6h / 48h 价差可忽略。不要沉低频。

#### 实现落点（无第二套流程）

| | `s3-compatible` | `relkit-compatible` | `static-http` |
|---|---|---|---|
| GET 切面 | 匿名 HTTP | 匿名 HTTP | 匿名 HTTP |
| 写切面 | SigV4 / CopyObject | 能力 URL / COPY | 无；由外部系统放置 |
| 可作 ingest | 是 | 是 | 否 |
| CI 字节直达 | query 预签名 PUT | serve 能力 URL PUT | 不适用 |
| 取得 artifact 副本 | `Promote`（同桶 CopyObject） | `Promote`（COPY） | 只引用已存在的绝对 URL |
| 默认角色 | 公网 ingest / pointer | 内网 ingest / pointer | 只读镜像 / 外部 URL |
| 清 inbox | 桶生命周期或后端删除 | serve GC：租约 + `casGrace` | 无 inbox |

`local` / `http-put` 已删除。没有第二条「无 COS 发布流程」：内网只是 ingest = `relkit-compatible`，请求 URL 由 serve 签发。

## 3. HTTP 表面

- `GET /-/health`
- `PUT /v1/drop/{product}/{version}/{filename}` — 双 Job 交换口：一端先把 zip 放下，另一端再 HEAD/GET 取走。Bearer。不是发布。
- GET / HEAD `/v1/drop/{product}/{version}/{filename}` — 同上，鉴权后才能读未发布包
- `PUT /v1/staged/{product}/{version}` — staged 树的 `tar.gz`。现网常带 `artifacts/`；目标路径只含 `staged.pb` + `release-policy.json`
- `POST /v1/staged/{product}/{version}/uploads` — 创建分片会话。JSON：`bytes`、`sha256`、可选 `partSize`
- `PUT /v1/staged/{product}/{version}/uploads/{id}/parts/{n}` — 一片；可选 `X-Relkit-Part-SHA256`
- `GET` / `DELETE` `/v1/staged/{product}/{version}/uploads/{id}` — 查询已收片 / 放弃
- `POST /v1/staged/{product}/{version}/uploads/{id}/complete` — 拼装、校验整包 sha256、解包（与整包 PUT 同一落地路径）
- `POST /v1/publish` — JSON：`product` / `version` / 可选 `to` / `dryRun` / `stagedSha256` / `idempotencyKey`
- `POST /v1/cas/credentials` — 统一上传说明：每个待上传 blob 给绝对 URL 的 `requests[]`，目的地只有 **ingest 后端**一个。S3/COS 是长期钥 query 预签名，relkit-compatible 是 serve 能力 URL；CI 不按 Type 分支、不签名。

CI 默认走 `relkit staged-put`：多连接并发 PUT 各片。片大小与并发由客户端 `--part-size` / `--concurrency`（或 `RELKIT_UPLOAD_PART_SIZE` / `RELKIT_UPLOAD_CONCURRENCY`）决定，agent 配置夹取上限。同一 `bytes+sha256` 的未完成会话可续传。

无 token 时写端点返回 405。Token 加载时 SHA-256，比较用 constant-time。

## 4. 幂等与串行

`publish.Run` **不幂等**（同版本重发会抬高 `sequence`）。  
Agent 用 `stagedSha256`（或显式 `idempotencyKey`）落盘回放，重复请求直接返回上次结果。  
同一 `product` 并发 publish 返回 409。

## 5. 部署

见 [`deploy/`](../../deploy/)：

- `relkit-agent.example.json`
- `relkit-agent.intranet.example.json`（WOA 控制面）
- `relkit-intranet-product.example.json`（内网 `relkit-compatible` publish profile 骨架）
- `nginx-intranet.example.conf`（内网 `update.devcloud.woa.com`：`/v1/` → agent `:8787`，GET → serve 数据面）
- `relkit-agent.service`
- `Caddyfile.relkit-agent.example`（`publish.firoyang.com` → `127.0.0.1:8787`）
- `install-agent.sh`

DNS：`publish.firoyang.com` A → 发布机公网 IP。Agent 只听本机；TLS 由前面的反向代理终止。

实装说明（2026-08）：发布机上已有 nginx 占用 `:80`，因此 HTTPS 用 **nginx + certbot** 反代 `127.0.0.1:8787`，而不是再起 Caddy。若主机是空机，仍可用 `deploy/Caddyfile.relkit-agent.example`。

产品清单走本机 CLI，不要手改 `uploadTokens`。每个产品一张 token 文件：`tokens/<id>.token`，CI 环境变量名固定为 `RELKIT_UPLOAD_TOKEN`（值因仓库而异）。**禁止**实例级 `uploadTokenFile` / `RELKIT_AGENT_TOKEN`：配置或环境里出现即拒绝启动。一条 token 也不得挂多个 product id。

```text
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -list-products
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> [-root /srv/relkit/<id>]
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> -token-only
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> -migrate-profile
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> -remove
```

`-product` 会创建 root（若尚未登记）、把 id 写进 `products`，并 **签发该产品 token**（只打印一次明文）。已在 map 里但还没有产品 token 时，同样签发并删掉 json 里的实例级字段。列出只打 id、root、profile 路径和 token **文件路径**。`-remove` 从 map 摘掉 id 并删除其 token 文件，磁盘上的产品树、密钥和 profile 留下。改完后先把新 `RELKIT_UPLOAD_TOKEN` 交给该产品 CI，再 `systemctl restart relkit-agent`。

### 5.1 删除旧后端前的部署顺序

必须严格分四步，不能把配置迁移和删除类型二进制一起上线：

1. **发布机清 `casCredentials`。** 从全部 publish profile 删除该字段；过渡版本虽接受并忽略，但不得继续把 `sts` 当有效配置。
2. **内网 serve 先部署 GC。** 先上线支持 CAS 上传租约与 `gc.casGrace`（默认 `24h`）的 relkit-serve，并确认 GC 正常。
3. **开写入面、迁 profile、发版验证。** 为 serve 配运营方 `RELKIT_SERVE_TOKEN`，把 profile 改成 `relkit-compatible`（`baseUrl`、可选 `uploadUrl`、必填 `tokenEnv`、可选 `timeoutSeconds`），完成一次 `cas-put` → publish → verify。
4. **稳定后才部署删除类型版本。** 观察现网稳定后，最后升级到不再包含 `local` / `http-put` 的 agent/CLI；否则旧 profile 会直接报 unsupported backend type。

## 6. Token 轮换

1. `relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> -token-only`
2. 把打印的 `RELKIT_UPLOAD_TOKEN` 写进 **该产品** 的 CI secret
3. `systemctl restart relkit-agent`
4. 旧 token 立即失效（无宽限期）

不要轮换「整台机一把」。没有这种东西。

## 7. 内网：WOA 上同样跑 agent

内网不必把产物发到公网 COS。控制面仍是 agent，数据面仍是 WOA 目录（现有 `https://update.devcloud.woa.com/` 的 GET 树）。

1. 在箱上安装 `relkit-agent`（`deploy/install-agent.sh`），配置见 `deploy/relkit-agent.intranet.example.json`。
2. 每个产品的 **publish profile** 把 `ingest`、`artifactTo`、`pointerTo` 都指到 `relkit-compatible`；`baseUrl` 为现有内网更新域名，`uploadUrl` 指向 serve 写入面，`tokenEnv` 为 `RELKIT_SERVE_TOKEN`。样例：`deploy/relkit-intranet-product.example.json`。旧机若产品根还留着整份配置，可先 `-migrate-profile`。
3. 私钥只在这台机上。CI 只持 **该产品** 的 `RELKIT_UPLOAD_TOKEN`。
4. 对外继续匿名 GET 现有域名。`relkit-serve` 可以继续提供 Range GET；写入统一走 serve 签发的 CAS 能力 URL。

Agent 的 token 与 serve 的 `uploadTokens` 一样按产品拆。不要把某产品的 token 发给无关仓库。不要用实例级 Bearer 当「同机共享」。

## 8. 给人看的索引页

`publish.Run` 会写出 `browse/<product>.html`、合并后的 `browse/index.html` 与 `browse/catalog.json`（产品树 dump 在 `.relkit/browse/`）。协议客户端不读这些页。

| | 公网 | 内网 |
|---|---|---|
| 托管 | EdgeOne Makers（契约在本仓库 `sites/updates-index/`，**不要把 dump 拷进该目录当发版步骤**） | 数据面 `browse/` |
| 怎么上去 | 仓库 policy 带 `site.makers.projectId`；本机 profile 带 `tokenEnv`；publish 写出 dump 后直接 Upload | `relkit-compatible` 由 publish 直接写；即使配了 makers 也跳过 |
| 动态 | 现在没有 `edge-functions/`，就是静态站。计数走 51.la（见 `docs/ROADMAP.md`），不要 KV。以后要函数只加在该子目录，且不当账本；内网不跟 |

COS 不放 HTML。不要为此打开静态网站源站。页上不把 `.pb` 当导航，也不加载外链字体或图。
