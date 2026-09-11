# relkit-agent

> 本目录是仓库的一部分（`cmd/relkit-agent`）。部署脚本在仓库根的 `deploy/`。产品仓运维入口是 `scripts/host/relkit_host.py`。设计见 [`docs/design/publish-agent.md`](../../docs/design/publish-agent.md)。

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

签名用的公钥取 staged 树 `release-policy.json` 的 `signing.publicKeys`。profile 没有公钥集。这台机上应至少留一个 staged 版本。

## 运维

装机 / 换二进制：`python deploy/relkit.py install agent` / `upgrade`。给产品挂 profile：产品仓 `python scripts/host/relkit_host.py agent add --execute`（重启另加 `--restart`）。`init` 是内部写配置接口，不是人用 CLI。

证书续期与 agent 同机、不同进程：安装 `deploy/relkit-cos-cert-renew.service` + `.timer`，配置 `/etc/relkit-cos-cert/renew.json`（`targets[]` 每条是 region + bucket + domain）。不要把 COS 密钥写进 agent 的同一份 env 以外的仓库文件。

改完后 `systemctl restart relkit-agent`。把新 token **先**交给该产品 CI，再重启。

新产品：产品仓 `relkit_host.py agent add --execute`，再在机上准备 `/etc/relkit-agent/products/<id>.json`（`product` + `signing.keyId` + backends）。不要往产品根塞发布凭据。

改 `/etc/relkit-agent/env` 后必须 `systemctl restart`。COS / 产品上传 token / EdgeOne 是三套东西。root 跑 init 时 `products/` 必须 `0755`，`tokens/` 给服务用户可读（文件 0600、目录让 relkit 能读）。

## HTTP

- `GET /-/health`
- `PUT /v1/drop/{product}/{version}/{filename}`（及鉴权 GET/HEAD/DELETE；供 build-scoped 多平台交换）
- `POST /v1/cas/credentials`（按 profile 的第一个 ingest 返回缺失 blob 的绝对 URL `requests[]`；客户端不签名）
- `PUT /v1/staged/{product}/{version}`
- `POST /v1/staged/{product}/{version}/uploads`（分片会话；`partSize` 可在 JSON 里请求，受配置夹取）
- `PUT /v1/staged/{product}/{version}/uploads/{id}/parts/{n}`
- `GET` / `DELETE` `/v1/staged/{product}/{version}/uploads/{id}`
- `POST /v1/staged/{product}/{version}/uploads/{id}/complete`
- `POST /v1/publish`

无任何产品 token 时写端点 405。Bearer 对但产品不对是 **403**。安装：`python3 deploy/relkit.py install agent --binary …`。已有实例：`python deploy/relkit.py upgrade --host <Host>`。

## 删除旧后端前的四步部署顺序

1. 发布机先从全部 profile 清掉 `casCredentials`。
2. 内网先部署带上传租约与 `gc.casGrace`（默认 24h）的 relkit-serve。
3. 开启 serve 写入面，把 profile 迁到 `relkit-compatible`，完成一次真实发版与 verify。
4. 稳定后才部署已删除 `local` / `http-put` 类型的 agent/CLI。
