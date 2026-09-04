# relkit-agent

> 本目录是仓库的一部分（`cmd/relkit-agent`）。部署脚本在仓库根的 `deploy/`。本目录没有 `AGENT-GUIDE.md`；设计见 [`docs/design/publish-agent.md`](../../docs/design/publish-agent.md)。

发布控制面：CI 只 `relkit stage` 并上传 staged 树；本机持签名私钥与后端凭据执行 `publish.Run`。客户端永远不连 agent。

## 配置路径

| 路径 | 作用 |
|---|---|
| `/etc/relkit-agent/relkit-agent.json` | 监听地址、`uploadTokens`、`products` map |
| `/etc/relkit-agent/tokens/<id>.token` | **该产品**的上传 Bearer（0600）。CI 环境变量名是 `RELKIT_UPLOAD_TOKEN` |
| `/etc/relkit-agent/env` | `RELKIT_PRIVATE_KEY`、`COS_SECRET_ID`、`COS_SECRET_KEY` 等（systemd `EnvironmentFile`） |
| `/etc/relkit-agent/products/<id>.json` | 本机 **publish profile**（缺省；可用 `products.<id>.profile` 覆盖） |
| `/srv/relkit/<id>/` | 产品树根：私钥文件、`.relkit/staged/<version>/` |
| `/var/lib/relkit-agent` | 幂等回放等状态 |

**没有**实例级 `/etc/relkit-agent/token`，也 **没有** `RELKIT_AGENT_TOKEN`。配置里出现 `uploadToken` / `uploadTokenFile`、或进程环境里出现 `RELKIT_AGENT_TOKEN`，agent **拒绝启动**。一条 token 文件不得挂多个 product id（启动失败）。

`products.<id>.root` 永远是产品根；不以 JSON 文档里的路径为准。

分片上传可调字段（均可省略，默认 8MiB / 1MiB–64MiB / 16 路 / 24h）：

| 字段 | 作用 |
|---|---|
| `partSize` | 客户端未指定时的默认片大小 |
| `minPartSize` / `maxPartSize` | 夹取客户端请求的片大小 |
| `maxParts` | 单次上传最多片数 |
| `maxPartConcurrency` | 同一 upload id 同时进行的 part PUT 上限，超出返回 429 |
| `uploadTTL` | 未完成会话过期（Go duration，如 `24h`） |

客户端用 `relkit staged-put`，片大小与并发另由 `--part-size` / `--concurrency` 或环境变量 `RELKIT_UPLOAD_PART_SIZE` / `RELKIT_UPLOAD_CONCURRENCY` 决定，不能超过 agent 上限。

## 发布时读哪份配置

**只认这一条路径：** staged 树里的 `release-policy.json` + 本机 `/etc/relkit-agent/products/<id>.json`（`product` 与 `signing.keyId` 必须一致）。缺任一则失败。

产品根上的 `relkit.json` **不是**发布配置，agent 不会读它。`relkit stage` 写出的 policy 不含私钥、backends、`publishTo`；profile 不含公钥集与通道策略。

## 运维命令

```text
relkit-agent -config /etc/relkit-agent/relkit-agent.json
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -list-products
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> [-root /srv/relkit/<id>]
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> -token-only
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> -migrate-profile
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> -remove
```

- `-product`：建 root（若尚未登记）、写入 map、**签发该产品 token**（打印一次 `RELKIT_UPLOAD_TOKEN`）。已登记但还没有产品 token 时只签发、并删掉 json 里的实例级字段。
- `-token-only`：轮换该产品 token，不改 root / profile。
- `-migrate-profile`：一次性工具。从已有 `<root>/relkit.json` 抽出机器侧字段写到 `products/<id>.json`，拒绝覆盖已有 profile，并把产品根那份改名为 `relkit.json.migrated`。
- `-remove`：从 map 摘掉 id，并删除其 token 文件；磁盘上的树、密钥、profile 留下。
- 改完后 `systemctl restart relkit-agent`。把新 token **先**交给该产品 CI，再重启。

新产品：先 `-product`，再手写 `/etc/relkit-agent/products/<id>.json`（`product` + `signing.keyId` + backends）。不要往产品根塞发布凭据。

改 `/etc/relkit-agent/env` 后必须 `systemctl restart`。COS / 产品上传 token / EdgeOne 是三套东西。root 跑 init 时 `products/` 必须 `0755`，`tokens/` 给服务用户可读（文件 0600、目录让 relkit 能读）。

## HTTP

- `GET /-/health`
- `PUT /v1/drop/{product}/{version}/{filename}`（及鉴权 GET/HEAD）
- `PUT /v1/staged/{product}/{version}`
- `POST /v1/staged/{product}/{version}/uploads`（分片会话；`partSize` 可在 JSON 里请求，受配置夹取）
- `PUT /v1/staged/{product}/{version}/uploads/{id}/parts/{n}`
- `GET` / `DELETE` `/v1/staged/{product}/{version}/uploads/{id}`
- `POST /v1/staged/{product}/{version}/uploads/{id}/complete`
- `POST /v1/publish`

无任何产品 token 时写端点 405。Bearer 对但产品不对是 **403**。安装：`deploy/install-agent.sh`。
