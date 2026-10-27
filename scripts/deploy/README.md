# deploy CLI

## 服务器入口（内外网不可混用）

| | 内网发布机 | 外网 CVM |
| --- | --- | --- |
| 连接 | `ssh -p 36000 root@update.devcloud.woa.com` | `ssh publish.firoyang.com` |
| 端口 | 36000（默认 22 不通） | 22，专用密钥 |
| 主机 | `VM-241-248-tencentos` / `9.134.241.248` | 云主机 `43.161.239.208` |

内网端口必须显式写 `-p 36000`。外网入口按域名连、端口 22，不要给它也配上 36000 或写死旧 IP：前者会落到 WOA 网关并返回 `HTTP/1.1 502 Bad Gateway`，看起来像 SSH 被拦，实际是走错了入口；后者指向的机器已经不再使用。外网这条用域名而非 IP，是因为换机时只改 DNS 即可。

内网三个 unit 与配置位置：

- `relkit-agent`：unit `/etc/systemd/system/relkit-agent.service`，配置 `/etc/relkit-agent/relkit-agent.json`，监听 `127.0.0.1:8787`，stateDir `/data/relkit-agent`，凭据目录 `/etc/relkit-agent/tokens/`
- `relkit-store`：监听 `:8080`
- `relkit-console`：监听 `:8081`

内网 nginx vhost 分工（`/etc/nginx/conf.d/`）：

- `publish.devcloud.woa.com`：`/v1/` 转发到 agent（CI 写面）
- `publish.devcloud.woa.com` 同时是 CI 取工具的入口：宿主 `relkit.project.json` 的 `server` 指向它，工具清单里的下载 URL 由 agent 的 `publicURL` 签出，也落在这个域。
- `update-internal.devcloud.woa.com`：只把协议树转给 store，供 updater 读取；不转发 `/v1/tools/` 与 `/v1/catalog`。完整样例见 `nginx-intranet.example.conf`。

发布工具后必须核对一次（换成主机实际使用的域名与刚发布的版本）：

```
python scripts/deploy/verify-published-tools.py \
  --base http://publish.devcloud.woa.com \
  --version "0.7.0+db0e150109e3-dirty"
```

它会读 Catalog 确认版本在册且 `current` 指向它、取 `/v1/tools/root.pub`、逐个下载工具校验摘要、用 `ssh-keygen -Y verify`（namespace `relkit-tool`）验签，并核对 `/v1/tools/current/<component>/<platform>` 回传的文件名与摘要。任何一项不对就返回非零——发布侧报成功不代表宿主能装。

本脚本只负责 Operator 机器的装机与换二进制。产品仓不再物化任何 Python 运维脚本（ADR 0019 §1）；产品侧的声明与策略走 `relkit.project.json` 与 `relkit-ops agent policy`。

唯一入口：`python scripts/deploy/relkit.py`（仓库根目录执行；Linux 上也可用 `python3 scripts/deploy/relkit.py`）。Agent 流程见 [`DecAssets/skills/relkit-deploy/SKILL.md`](../../DecAssets/skills/relkit-deploy/SKILL.md)。

启动时会自检 Python 3.9+。`scripts/deploy/requirements.txt` 目前没有第三方包，因此不会建 venv。若以后加了 pin，脚本会创建 `scripts/deploy/.venv`、`pip install`，然后 re-exec。

| 子命令 | 用途 |
|---|---|
| `build` | 交叉编译二进制并可生成 immutable Dart/Rust SDK ZIP（`--dart-sdk` / `--rust-sdk`）。stamp 读根目录 `VERSION.json` |
| `version` | 打印 SSOT（`--field number\|version\|build\|tag`）；发版 CI 用 `--check-tag` |
| `install serve` | 空机首装 systemd `relkit-store` |
| `install agent` | 空机首装 systemd `relkit-agent`。`--product-root` 必填：它同时是服务账号家目录、unit 的 `WorkingDirectory`、以及写进 agent 配置的 `productRootParent`（没默认值，空机无从推断，猜错要等发布时才爆） |
| `install console` | 首装 systemd `relkit-console`（ADR 0016：管理面独立 unit）。读 adapter 条目（`tree`）取自 `relkit-console.example.json`，或 `--config-example` 指定的配置文件，整块写入，脚本不解读条目、不探测本机 store；没有条目就失败。内网用 `--config-example scripts/deploy/relkit-console.intranet.example.json`。面板状态落 `/var/lib/relkit-console/panel.json`（`StateDirectory`）。`upgrade` 会把旧配置的 `dir`/`store` 一次性改写成 `tree` 条目 |
| `migrate-serve` | 现网仍在跑 `relkit-serve.service` 的盒子一步迁移到 `relkit-store.service`（ADR 0016 第 4 步：配置迁 `/etc/relkit-store/relkit-store.json`，树/token/数据文件不动，备份后可回滚） |
| `upgrade` | 已在跑的机器：探测、迁移、换二进制、可选重启 |

禁止：用 example JSON 覆盖现网配置；把 token 或带 `sig=` 的 URL 打进聊天/工单；upgrade 默默 `--rotate-token`；跳过发布验证。产品 token 的签发/轮换/吊销走 Operator 侧 `relkit-ops agent policy` 或在机上执行 `relkit-agent init -product <id> -token-only`；不要用腾讯云 TAT / MCP 代跑本脚本。

## 黄金路径（升级已有内网机）

在仓库根、已能 `ssh <Host>`（端口走 `~/.ssh/config`）：

```bash
python scripts/deploy/relkit.py upgrade --host cvm-gz --plan
python scripts/deploy/relkit.py upgrade --host cvm-gz --apply
python scripts/deploy/relkit.py upgrade --host update.devcloud.woa.com --plan
python scripts/deploy/relkit.py upgrade --host update.devcloud.woa.com --apply
```

`--plan` 只读并打印脱敏探测结果、本机 HEAD 与协议窗口。`--apply` 默认从当前干净 HEAD 构建 linux/amd64 的 agent+serve，在目标机 `/var/backups/relkit/<utc>/` 备份后换文件并重启。`--stage-only` 只写盘不重启，不得称为升级完成。`--unsafe-from-dist` 才使用已有 `dist/`，并持续告警。失败会从该备份回滚。

upgrade **保留** 现网 `dir`；仅在显式传入 `--serve-listen-addr` 时修改 `addr`。它会：补 `gc.casGrace`、清 `casCredentials`、把可推导的 `local`/`http-put` 改成 `relkit-compatible`（推导不了就停）、遇到 `static-http` 直接停（该类型已删除、不会改写成可写后端）、用显式 `--public-upload-url` 修正已有 `relkit-compatible` 的 `uploadUrl`、按 live json 重写 `ReadWritePaths`。产品 `baseUrl` 是产品注册信息，upgrade 从不改写；旧 `local`/`http-put` 后端没声明 `baseUrl` 时直接停，先在 profile 里写好读面再升级。

`uploadUrl` 与 COS 的 endpoint 同义，必须同时可被 agent 和 CI 访问；远程 CI 场景禁止配置 loopback。自建 `relkit-store` 应独立监听公开的数据面端口，不经 agent 的 nginx 搬运上传正文。`baseUrl` 可与 `uploadUrl` 相同，也可使用独立只读域名/CDN。nginx 样例的 `/` 仅保留旧签名 URL 的 GET 兼容入口，写操作必须直达 serve。

内网与外网三域名绝对同构（2026-10-19）：`publish.devcloud.woa.com`（CI 写面）、`update-internal.devcloud.woa.com`（SDK 读面，profile `baseUrl` 指向这里）、`update.devcloud.woa.com`（人页 + 存量兼容读）。内网 publish profile 的 `baseUrl` 切 `https://update-internal.devcloud.woa.com/`、`uploadUrl` 切 `https://publish.devcloud.woa.com/`（对齐外网 dec/cronkit 2026-09-26 形态）。旧口径「数据面 `update.devcloud.woa.com:8080` 与控制面共主机名不共端口」退役，新接入不要再用 `:8080` 裸端口 URL。

`upgrade` 默认从当前 HEAD 构建；改完代码直接 `--apply` 即可。只有 `--unsafe-from-dist` 才会把盘上已有的 `dist/` 送上去。

工具下载 URL 由 agent 自己的 `publicURL`（`/etc/relkit-agent/relkit-agent.json`）决定，调用方不再传。宿主 `relkit.project.json` 的 `server` 指向 publish 域，CI 从 `{server}/v1/tools/current/...` 取工具，所以 `publicURL` 必须就是 publish 域（内网 `http://publish.devcloud.woa.com`，外网 `https://publish.firoyang.com`）。没配或配成 loopback 时，工具发布返回 409 并触发整体回滚。update-internal 只给 updater 提供协议树，不转发 `/v1/tools/` 与 `/v1/catalog`。

**agent 与 serve 固定配套。** 外网 `cvm-gz` 与内网 `update.devcloud.woa.com` 都必须跑两个进程（外网 serve 是操作面，数据面仍可在 COS；内网 serve 是 `relkit-compatible` 数据面，见 [publish-topology](../../docs/design/publish-topology.md) §5）。缺哪个先 `install` 哪个，不要用 `--agent-only` / `--serve-only` 当现网默认。要升的组件目标机没跑，upgrade 会在探测后报错并指出该跑哪条 `install`，不会抛栈。

agent 的写端点要求 publisher 双向窗口握手（[ADR 0009](../../docs/adr/0009-publisher-protocol-negotiation.md)）。升级 agent 后必须用同一 release 的 publisher。滚动放行时可临时下调 `minPublishProtocol`（设 0 关闭）。

agent 对 publish profile 是 strict 解析：未知字段即拒（publish 阶段 400），不承诺向后兼容。升级前扫一遍目标机 `/etc/relkit-agent/products/*.json` 是否带结构体外字段（合法字段集 = relkit 源码 `internal/config/policy.go` 的 `PublishProfile` / `PublishDirectoryProfile` / `PublicKeyConfig`），有则删字段再升级。vNext 断代后通道与版本策略既不在 profile 也不在 staged 文档里，出现即拒；排障链路见 [`cmd/relkit-agent/README.md`](../../cmd/relkit-agent/README.md)。

人页静态目录由 agent 顶层 `site.sinks[]`（ADR 0015）拥有；产品 profile 不携带站点配置。现网形态：外网 `[{"type":"backend","backend":"serve"},{"type":"directory","path":"/srv/relkit/site"}]`（nginx 直接伺服该目录），内网 `[{"type":"backend",...}]`（写进发布树 `browse/`），通用样例 `relkit-agent.example.json` 即外网形态。upgrade 仍会把遗留的 `site.makers` 迁移为 `site.sinks: [{"type":"makers",...}]`，但不会猜 projectId，也不会覆盖现网 agent JSON；Makers 现网已不用。改完 sinks 重启后用 `curl -fsS http://127.0.0.1:8787/-/site` 检查脱敏 readiness，再运行 `sudo relkit-agent site-rebuild -config /etc/relkit-agent/relkit-agent.json`。修人页不需要重发产品版本。

目标机必须已有 Python 3.9+（`python3` 或 `/usr/bin/python3`）、`systemctl`、sudo。CAS 探针的 key 必须是 body 的 sha256，能力 PUT 不要带 publish protocol 头。

## 首装

```bash
sudo python3 scripts/deploy/relkit.py install serve --binary ./dist/relkit-store-linux-amd64
sudo python3 scripts/deploy/relkit.py install agent \
  --binary ./dist/relkit-agent-linux-amd64 \
  --product-root /data/relkit
```

`--product-root` 没有默认值，必须给绝对路径，示例里的 `/data/relkit` 只是占位，换成目标机上真实的发布父目录。它一次决定三件事：服务账号的家目录、unit 的 `WorkingDirectory`、agent 配置里的 `productRootParent`（注册产品不填 root 时落到 `<productRootParent>/<产品名>`）。装在哪个盘、服务账号能否写，只有装的人知道，脚本不猜。

内网 `update.devcloud.woa.com` 的数据放在 `/data`，不要用上面的默认值（`127.0.0.1:30341`、`/srv/releases`、`/var/lib/relkit-agent`）：

```bash
sudo python3 scripts/deploy/relkit.py install serve \
  --binary ./dist/relkit-serve-linux-amd64 \
  --addr :8080 \
  --dir /data/relkit-serve
sudo python3 scripts/deploy/relkit.py install agent \
  --binary ./dist/relkit-agent-linux-amd64 \
  --state-dir /data/relkit-agent \
  --product-root /data/relkit \
  --config-example scripts/deploy/relkit-agent.intranet.example.json
```

产品树在 `/data/relkit/<product>`。已有实例且 `--addr`/`--dir` 与现网不一致时，`install serve` 拒绝执行（避免把 30341 / `/srv/releases` 写进内网机）。这时用 `upgrade`。

产品仓注册/轮换/吊销上传 token：由 Operator 侧 `relkit-ops agent policy --config <cfg> --policy <relkit.agent-product-policy/1>` 写入，或在机上执行 `relkit-agent init -product <id> -token-only`（重启另加 `systemctl restart relkit-agent`）。

## 测试

```bash
python -m unittest scripts/deploy/test_relkit.py
```

Release 构建使用 `--rust-sdk` 生成 `relkit-sdk-rust.zip`。ZIP 的时间戳、权限和
路径排序固定，并携带 canonical `proto/updater/v1/updater.proto`，因此消费仓在
`third_party/relkit/sdk/rust` 可直接用 vendored protoc 重建 DTO。
