# deploy CLI

唯一入口：`python deploy/relkit.py`（仓库根目录执行；Linux 上也可用 `python3 deploy/relkit.py`）。

启动时会自检 Python 3.9+。`deploy/requirements.txt` 目前没有第三方包，因此不会建 venv。若以后加了 pin，脚本会创建 `deploy/.venv`、`pip install`，然后 re-exec。

| 子命令 | 用途 |
|---|---|
| `build` | 交叉编译二进制并可生成 immutable Dart/Rust SDK ZIP（`--dart-sdk` / `--rust-sdk`） |
| `install serve` | 空机首装 systemd `relkit-serve` |
| `install agent` | 空机首装 systemd `relkit-agent` |
| `upgrade` | 已在跑的机器：探测、迁移、换二进制、可选重启 |

禁止：用 example JSON 覆盖现网配置；把 token 或带 `sig=` 的 URL 打进聊天/工单；upgrade 默默 `--rotate-token`；跳过发布验证。产品 token 的签发/轮换/吊销不在本脚本，走产品仓 `relkit_host.py serve`。

## 黄金路径（升级已有内网机）

在仓库根、已能 `ssh <Host>`（端口走 `~/.ssh/config`）：

```bash
python deploy/relkit.py build --serve --agent --os linux --arch amd64
python deploy/relkit.py upgrade --host update.devcloud.woa.com --plan --serve-listen-addr :8080 --public-base-url http://update.devcloud.woa.com:8080/ --public-upload-url http://update.devcloud.woa.com:8080/
python deploy/relkit.py upgrade --host update.devcloud.woa.com --apply --serve-listen-addr :8080 --public-base-url http://update.devcloud.woa.com:8080/ --public-upload-url http://update.devcloud.woa.com:8080/
```

`--plan` 只读并打印脱敏探测结果、本机 HEAD 与协议窗口。`--apply` 默认从当前干净 HEAD 构建 linux/amd64 的 agent+serve，在目标机 `/var/backups/relkit/<utc>/` 备份后换文件并重启。`--stage-only` 只写盘不重启，不得称为升级完成。`--unsafe-from-dist` 才使用已有 `dist/`，并持续告警。失败会从该备份回滚。

upgrade **保留** 现网 `dir`；仅在显式传入 `--serve-listen-addr` 时修改 `addr`。它会：补 `gc.casGrace`、清 `casCredentials`、把可推导的 `local`/`http-put` 改成 `relkit-compatible`（推导不了就停）、用显式 `--public-base-url` / `--public-upload-url` 修正已有 `relkit-compatible` 端点、按 live json 重写 `ReadWritePaths`。

`uploadUrl` 与 COS 的 endpoint 同义，必须同时可被 agent 和 CI 访问；远程 CI 场景禁止配置 loopback。自建 `relkit-serve` 应独立监听公开的数据面端口，不经 agent 的 nginx 搬运上传正文。`baseUrl` 可与 `uploadUrl` 相同，也可使用独立只读域名/CDN。nginx 样例的 `/` 仅保留旧签名 URL 的 GET 兼容入口，写操作必须直达 serve。

现网数据面是 `update.devcloud.woa.com:8080`，与控制面共用主机名但不共用端口，也不经 nginx。后续可给数据面绑定独立 DNS，届时同时替换 `baseUrl` 与 `uploadUrl`。

`upgrade` 默认从当前 HEAD 构建；改完代码直接 `--apply` 即可。只有 `--unsafe-from-dist` 才会把盘上已有的 `dist/` 送上去。

一台机只跑其中一个进程是合法的：外网发布机 `cvm-gz` 数据面在 COS，没有 `relkit-serve`（[publish-topology](../docs/design/publish-topology.md) §5），升级时加 `--agent-only`。要升的组件目标机没跑，upgrade 会在探测后报错并给出用哪个 flag 跳过、或该跑哪条 `install`，不会抛栈。

agent 的写端点要求 publisher 双向窗口握手（[ADR 0009](../docs/adr/0009-publisher-protocol-negotiation.md)）。升级 agent 后必须用同一 release 的 publisher。滚动放行时可临时下调 `minPublishProtocol`（设 0 关闭）。

目标机必须已有 Python 3.9+（`python3` 或 `/usr/bin/python3`）、`systemctl`、sudo。CAS 探针的 key 必须是 body 的 sha256，能力 PUT 不要带 publish protocol 头。

## 首装

```bash
sudo python3 deploy/relkit.py install serve --binary ./dist/relkit-serve-linux-amd64
sudo python3 deploy/relkit.py install agent --binary ./dist/relkit-agent-linux-amd64
```

已有实例且 `--addr`/`--dir` 与现网不一致时，`install serve` 拒绝执行（避免把 30341 / `/srv/releases` 写进内网机）。这时用 `upgrade`。

产品仓注册/轮换/吊销上传 token：`python scripts/host/relkit_host.py serve …`（需要 `--execute`；重启另加 `--restart`）。

## 测试

```bash
python -m unittest deploy/test_relkit.py
```

Release 构建使用 `--rust-sdk` 生成 `relkit-sdk-rust.zip`。ZIP 的时间戳、权限和
路径排序固定，并携带 canonical `proto/updater/v1/updater.proto`，因此消费仓在
`third_party/relkit/sdk/rust` 可直接用 vendored protoc 重建 DTO。
