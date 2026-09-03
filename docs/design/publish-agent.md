# Publish Agent（CI → 发布机 → COS）

---
title: Publish Agent
category: design
created: 2026-08-12
updated: 2026-09-01
status: approved
related: docs/design/update-ingress-cos.md, docs/design/publish-topology.md, CLI.md, cmd/relkit-agent/README.md
---

## 1. 决议

CI **不持** RUP 签名私钥，也 **不持** COS 写密钥。  
CI 只做 `relkit stage`，把 staged 树打包后交给发布机上的 `relkit-agent`；agent 持钥执行 `publish.Run` 写入数据面。

数据面只是地点不同：公网 `s3-compatible` → COS；内网 `local` → WOA 磁盘。客户端永远不连 agent。

## 2. 信任边界

发布配置拆成两份，在 agent 上合并后才调用 `publish.Run`：

| 文件 | 谁写、放哪 | 装什么 | 禁止装什么 |
|---|---|---|---|
| `release-policy.json` | 仓库 `relkit stage` 写入 staged 树，随 tar.gz 上传 | 产品 id、通道、`codeStrategy`、公钥、directory 入口、site 文案 / Makers 项目 id | 私钥路径、后端凭据、`publishTo`、Makers `tokenEnv`、changelog 本地 `file` |
| publish profile | 发布机 `/etc/relkit-agent/products/<product>.json` | `product` + `signing.keyId`（与 policy 对齐）、私钥 env/path、backends、`publishTo`、directory 发布目标、Makers `tokenEnv` | 公钥集、通道策略、entryUrls |

产品根上的 `relkit.json` **不是** agent 发布配置。仓库里那份只给本地 / CI 的 `relkit stage` 抽 policy 用；发布机上若还留着旧副本，应迁 profile 后改名为 `relkit.json.migrated` 或删掉。

`product` 与 `signing.keyId` 必须两边一致，否则拒绝发布。产品树根永远是 agent 配置里的 `products.<id>.root`，不以 JSON 文档里的路径为准。

| 角色 | 持有 | 不持有 |
|---|---|---|
| CI runner | 源码、产物、**该产品**的 `RELKIT_UPLOAD_TOKEN`、随 stage 生成的 portable policy | 签名私钥、COS SecretKey、publish profile、别的产品的 token |
| relkit-agent | 私钥、COS 密钥、本机 publish profile | 客户端下载流量；不读产品根 `relkit.json` |
| COS / 内网磁盘 | 匿名读已发布文件 | 签名动作、控制面配置 |

### 2.1 staged 树（CI 交给 agent 的内容）

`.relkit/staged/<version>/` 必须自足，agent 只解包这一棵：

```text
.relkit/staged/<version>/
  staged.pb              # 版本、code、产物清单与哈希
  release-policy.json    # 仓库侧 portable 策略（stage 时从仓库 relkit.json 抽出）
  artifacts/<filename>   # 产物副本；publish 只按文件名读，不信 staged.pb 里的 source_path
```

缺少 `release-policy.json` 或本机没有可读 profile 时直接失败，没有第三条路径。

### 2.2 publish profile

缺省路径：与 `relkit-agent.json` 同目录的 `products/<product>.json`（安装后即 `/etc/relkit-agent/products/<id>.json`）。`products.<id>.profile` 可覆盖。profile 只含端点与环境变量**名**，不含密钥明文；init 写成 `0644`，以便 root 跑 init 后 `relkit` 用户仍能读。

## 3. HTTP 表面

- `GET /-/health`
- `PUT /v1/drop/{product}/{version}/{filename}` — 双 Job 交换口：一端先把 zip 放下，另一端再 HEAD/GET 取走。Bearer。不是发布。
- GET / HEAD `/v1/drop/{product}/{version}/{filename}` — 同上，鉴权后才能读未发布包
- `PUT /v1/staged/{product}/{version}` — body = staged 目录的 `tar.gz`，Bearer 鉴权
- `POST /v1/publish` — JSON：`product` / `version` / 可选 `to` / `dryRun` / `stagedSha256` / `idempotencyKey`

无 token 时写端点返回 405。Token 加载时 SHA-256，比较用 constant-time。

## 4. 幂等与串行

`publish.Run` **不幂等**（同版本重发会抬高 `sequence`）。  
Agent 用 `stagedSha256`（或显式 `idempotencyKey`）落盘回放，重复请求直接返回上次结果。  
同一 `product` 并发 publish 返回 409。

## 5. 部署

见 [`deploy/`](../../deploy/)：

- `relkit-agent.example.json`
- `relkit-agent.intranet.example.json`（WOA：agent 写本地目录）
- `relkit-intranet-product.example.json`（内网 publish profile 的 `local` 后端骨架；也可当迁 profile 前的旧 `relkit.json` 参考）
- `nginx-intranet.example.conf`（内网 `update.devcloud.woa.com`：`/v1/` → agent `:8787`，GET → serve `:8080`）
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

### 5.1 部署顺序

1. `deploy/install-agent.sh`：二进制、`/etc/relkit-agent/relkit-agent.json`、默认产品根、`systemctl enable --now`。**不**写实例级 token 文件。
2. `EnvironmentFile=/etc/relkit-agent/env`：`RELKIT_PRIVATE_KEY`、`COS_SECRET_ID`、`COS_SECRET_KEY`（以及 Makers token 等）。密钥不进 JSON。
3. `init -product <id>` 登记新产品并签发 **该产品** 的 `RELKIT_UPLOAD_TOKEN`。
4. 把签名私钥放到该产品 root（或只走 env）。
5. **本机 publish profile**（二选一，必须在第一次 publish 之前完成）：
   - 旧机：产品根已有整份 `relkit.json` 时执行 `init -product <id> -migrate-profile`。它抽出机器侧字段写到 `/etc/relkit-agent/products/<id>.json`，把产品根那份改名为 `relkit.json.migrated`，且拒绝覆盖已存在的 profile。
   - 新机：按 `deploy/relkit-intranet-product.example.json` 或公网 `s3-compatible` 样例手写 `/etc/relkit-agent/products/<id>.json`（`product` / `signing.keyId` 与仓库 policy 对齐）。**不要**往 `/srv/relkit/<id>/` 塞发布凭据。
6. `systemctl restart relkit-agent`。
7. 之后 CI 只 `relkit stage`（写出 `release-policy.json`）再 `PUT /v1/staged` + `POST /v1/publish`。

## 6. Token 轮换

1. `relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> -token-only`
2. 把打印的 `RELKIT_UPLOAD_TOKEN` 写进 **该产品** 的 CI secret
3. `systemctl restart relkit-agent`
4. 旧 token 立即失效（无宽限期）

不要轮换「整台机一把」。没有这种东西。

## 7. 内网：WOA 上同样跑 agent

内网不必把产物发到公网 COS。控制面仍是 agent，数据面仍是 WOA 目录（现有 `https://update.devcloud.woa.com/` 的 GET 树）。

1. 在箱上安装 `relkit-agent`（`deploy/install-agent.sh`），配置见 `deploy/relkit-agent.intranet.example.json`。
2. 每个产品的 **publish profile** 把 `publishTo` 指到 `local`，`outputDir` 为 serve / nginx 对外 GET 的根（例：`/data/relkit-serve`），`baseUrl` 为现有内网更新域名。样例：`deploy/relkit-intranet-product.example.json`。旧机若产品根还留着整份配置，可先 `-migrate-profile`。
3. 私钥只在这台机上。CI 只持 **该产品** 的 `RELKIT_UPLOAD_TOKEN`。
4. 对外继续匿名 GET 现有域名。`relkit-serve` 可以继续提供 Range GET；新产品不要再走 serve 的 PUT。旧产品的 `http-put` 可留到迁完。

Agent 的 token 与 serve 的 `uploadTokens` 一样按产品拆。不要把某产品的 token 发给无关仓库。不要用实例级 Bearer 当「同机共享」。

## 8. 给人看的索引页

`publish.Run` 会写出 `browse/<product>.html`、合并后的 `browse/index.html` 与 `browse/catalog.json`（产品树 dump 在 `.relkit/browse/`）。协议客户端不读这些页。

| | 公网 | 内网 |
|---|---|---|
| 托管 | EdgeOne Makers（契约在本仓库 `sites/updates-index/`，**不要把 dump 拷进该目录当发版步骤**） | 数据面 `browse/` |
| 怎么上去 | 仓库 policy 带 `site.makers.projectId`；本机 profile 带 `tokenEnv`；publish 写出 dump 后直接 Upload | `local` / 遗留 `http-put` 由 publish 直接写；即使配了 makers 也跳过 |
| 动态 | 现在没有 `edge-functions/`，就是静态站。计数走 51.la（见 `docs/ROADMAP.md`），不要 KV。以后要函数只加在该子目录，且不当账本；内网不跟 |

COS 不放 HTML。不要为此打开静态网站源站。页上不把 `.pb` 当导航，也不加载外链字体或图。
