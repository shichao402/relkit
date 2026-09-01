# relkit-agent

> 本目录是仓库的一部分（`cmd/relkit-agent`）。部署脚本在仓库根的 `deploy/`。本目录没有 `AGENT-GUIDE.md`；设计见 [`docs/design/publish-agent.md`](../../docs/design/publish-agent.md)。

发布控制面：CI 只 `relkit stage` 并上传 staged 树；本机持签名私钥与后端凭据执行 `publish.Run`。客户端永远不连 agent。

## 配置路径

| 路径 | 作用 |
|---|---|
| `/etc/relkit-agent/relkit-agent.json` | 监听地址、token 文件、`products` map |
| `/etc/relkit-agent/token` | CI 共用的 Bearer（`uploadTokenFile`） |
| `/etc/relkit-agent/env` | `RELKIT_PRIVATE_KEY`、`COS_SECRET_ID`、`COS_SECRET_KEY` 等（systemd `EnvironmentFile`） |
| `/etc/relkit-agent/products/<id>.json` | 本机 **publish profile**（缺省；可用 `products.<id>.profile` 覆盖） |
| `/srv/relkit/<id>/` | 产品树根：私钥文件、`.relkit/staged/<version>/` |
| `/var/lib/relkit-agent` | 幂等回放等状态 |

`products.<id>.root` 永远是产品根；不以 JSON 文档里的路径为准。

## 发布时读哪份配置

**只认这一条路径：** staged 树里的 `release-policy.json` + 本机 `/etc/relkit-agent/products/<id>.json`（`product` 与 `signing.keyId` 必须一致）。缺任一则失败。

产品根上的 `relkit.json` **不是**发布配置，agent 不会读它。`relkit stage` 写出的 policy 不含私钥、backends、`publishTo`；profile 不含公钥集与通道策略。

## 运维命令

```text
relkit-agent -config /etc/relkit-agent/relkit-agent.json
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -list-products
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> [-root /srv/relkit/<id>]
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> -migrate-profile
relkit-agent init -config /etc/relkit-agent/relkit-agent.json -product <id> -remove
```

- `-product`：建 root、写入 map，**不**生成 policy / profile / 私钥 / 新 token。
- `-migrate-profile`：一次性工具。从已有 `<root>/relkit.json` 抽出机器侧字段写到 `products/<id>.json`，拒绝覆盖已有 profile，并把产品根那份改名为 `relkit.json.migrated`（agent 永不读它）。
- `-remove`：只从 map 摘掉 id；磁盘上的树、密钥、profile 留下。
- 改完后 `systemctl restart relkit-agent`。

新产品：先 `-product`，再手写 `/etc/relkit-agent/products/<id>.json`（`product` + `signing.keyId` + backends）。不要往产品根塞发布凭据。

改 `/etc/relkit-agent/env` 后必须 `systemctl restart`。COS / agent Bearer / EdgeOne 是三套东西，换其中一个不会让另外两个通道自动好。root 跑 init 时 `products/` 必须 `0755`。

## HTTP

- `GET /-/health`
- `PUT /v1/drop/{product}/{version}/{filename}`（及鉴权 GET/HEAD）
- `PUT /v1/staged/{product}/{version}`
- `POST /v1/publish`

无 token 时写端点 405。安装：`deploy/install-agent.sh`。
