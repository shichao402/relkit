# Publish Agent（CI → 发布机 → COS）

---
title: Publish Agent
category: design
created: 2026-08-12
updated: 2026-08-27
status: approved
related: docs/design/update-ingress-cos.md, docs/design/publish-topology.md, CLI.md, cmd/relkit-agent
---

## 1. 决议

CI **不持** RUP 签名私钥，也 **不持** COS 写密钥。  
CI 只做 `relkit stage`，把 staged 树打包后交给发布机上的 `relkit-agent`；agent 持钥执行 `publish.Run` 写入数据面。

数据面只是地点不同：公网 `s3-compatible` → COS；内网 `local` → WOA 磁盘。客户端永远不连 agent。

## 2. 信任边界

| 角色 | 持有 | 不持有 |
|---|---|---|
| CI runner | 源码、产物、`RELKIT_AGENT_TOKEN` | 签名私钥、COS SecretKey |
| relkit-agent | 私钥、COS 密钥、产品 `relkit.json` | 客户端下载流量 |
| COS / 内网磁盘 | 匿名读已发布文件 | 签名动作 |

## 3. HTTP 表面

- `GET /-/health`
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
- `relkit-intranet-product.example.json`（产品 `relkit.json` 的 `local` 后端）
- `relkit-agent.service`
- `Caddyfile.relkit-agent.example`（`publish.firoyang.com` → `127.0.0.1:8787`）
- `install-agent.sh`

DNS：`publish.firoyang.com` A → 发布机公网 IP。Agent 只听本机；TLS 由前面的反向代理终止。

实装说明（2026-08）：发布机上已有 nginx 占用 `:80`，因此 HTTPS 用 **nginx + certbot** 反代 `127.0.0.1:8787`，而不是再起 Caddy。若主机是空机，仍可用 `deploy/Caddyfile.relkit-agent.example`。

产品清单走本机 CLI，不要手改 JSON。Agent 只有一个 `uploadTokenFile`：map 里每个产品共用这一把 CI token（同族分享是默认行为，不会签发新产品秘密）。

```text
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -list-products
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> [-root /srv/relkit/<id>]
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> -remove
```

`-product` 会创建 root 目录并把 id 写进 `products`，**不**生成 `relkit.json`、私钥或新 token。列出只打 id、root 和 token **文件路径**。`-remove` 只从 map 摘掉 id，磁盘上的产品树和密钥留下。改完后 `systemctl restart relkit-agent`。

## 6. Token 轮换

1. 在发布机生成新 token 写入 `uploadTokenFile`  
2. 滚动重启 agent  
3. 更新 CI secret `RELKIT_AGENT_TOKEN`  
4. 旧 token 立即失效（无宽限期；需要宽限时跑双 token 需另实现）

## 7. 内网：WOA 上同样跑 agent

内网不必把产物发到公网 COS。控制面仍是 agent，数据面仍是 WOA 目录（现有 `https://update.devcloud.woa.com/` 的 GET 树）。

1. 在箱上安装 `relkit-agent`（`deploy/install-agent.sh`），配置见 `deploy/relkit-agent.intranet.example.json`。
2. 每个产品的 `relkit.json` 把 `publishTo` 指到 `local`，`outputDir` 为 serve / nginx 对外 GET 的根（例：`/data/relkit-serve`），`baseUrl` 为现有内网更新域名。样例：`deploy/relkit-intranet-product.example.json`。
3. 私钥只在这台机上。CI 仍只持 agent Bearer。
4. 对外继续匿名 GET 现有域名。`relkit-serve` 可以继续提供 Range GET；新产品不要再走 serve 的 PUT。旧产品的 `http-put` 可留到迁完。

给 agent 的 token 按产品拆，或与 serve 的 `uploadTokens` 对齐到迁完为止。不要把某产品的 token 发给无关仓库。

## 8. 给人看的索引页

`publish.Run` 会写出 `browse/<product>.html`、合并后的 `browse/index.html` 与 `browse/catalog.json`（产品树 dump 在 `.relkit/browse/`）。协议客户端不读这些页。

| | 公网 | 内网 |
|---|---|---|
| 托管 | EdgeOne Makers（契约在本仓库 `sites/updates-index/`，**不要把 dump 拷进该目录当发版步骤**） | 数据面 `browse/` |
| 怎么上去 | 产品 `relkit.json` 配 `site.makers`；publish 写出 dump 后直接 Upload | `local` / 遗留 `http-put` 由 publish 直接写；即使配了 makers 也跳过 |
| 动态 | 现在没有 `edge-functions/`，就是静态站。以后要函数只加在该子目录，内网不跟 |

COS 不放 HTML。不要为此打开静态网站源站。页上不把 `.pb` 当导航，也不加载外链字体或图。
