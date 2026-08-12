# Publish Agent（CI → 发布机 → COS）

---
title: Publish Agent
category: design
created: 2026-08-12
updated: 2026-08-12
status: approved
related: docs/design/update-ingress-cos.md, CLI.md, cmd/relkit-agent
---

## 1. 决议

CI **不持** RUP 签名私钥，也 **不持** COS 写密钥。  
CI 只做 `relkit stage`，把 staged 树打包后交给发布机上的 `relkit-agent`；agent 持钥执行 `publish.Run` 写 COS。

## 2. 信任边界

| 角色 | 持有 | 不持有 |
|---|---|---|
| CI runner | 源码、产物、`RELKIT_AGENT_TOKEN` | 签名私钥、COS SecretKey |
| relkit-agent | 私钥、COS 密钥、产品 `relkit.json` | 客户端下载流量 |
| COS | 匿名读对象 | 签名动作 |

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
- `relkit-agent.service`
- `Caddyfile.relkit-agent.example`（`publish.firoyang.com` → `127.0.0.1:8787`）
- `install-agent.sh`

DNS：`publish.firoyang.com` A → 发布机公网 IP。Agent 只听本机；TLS 由前面的反向代理终止。

实装说明（2026-08）：发布机上已有 nginx 占用 `:80`，因此 HTTPS 用 **nginx + certbot** 反代 `127.0.0.1:8787`，而不是再起 Caddy。若主机是空机，仍可用 `deploy/Caddyfile.relkit-agent.example`。

## 6. Token 轮换

1. 在发布机生成新 token 写入 `uploadTokenFile`  
2. 滚动重启 agent  
3. 更新 CI secret `RELKIT_AGENT_TOKEN`  
4. 旧 token 立即失效（无宽限期；需要宽限时跑双 token 需另实现）
