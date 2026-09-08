# deploy CLI

唯一入口：`python deploy/relkit.py`（仓库根目录执行；Linux 上也可用 `python3 deploy/relkit.py`）。

启动时会自检 Python 3.9+。`deploy/requirements.txt` 目前没有第三方包，因此不会建 venv。若以后加了 pin，脚本会创建 `deploy/.venv`、`pip install`，然后 re-exec。

| 子命令 | 用途 |
|---|---|
| `build` | 交叉编译 serve / agent（默认两者；`CGO_ENABLED=0`） |
| `install serve` | 空机首装 systemd `relkit-serve` |
| `install agent` | 空机首装 systemd `relkit-agent` |
| `upgrade` | 已在跑的机器：探测、迁移、换二进制、可选重启 |
| `token` | 显式轮换运营方/产品 token（**不**绑在 upgrade 上） |

禁止：用 example JSON 覆盖现网配置；把 token 或带 `sig=` 的 URL 打进聊天/工单；upgrade 默默 `--rotate-token`；跳过发布验证。

## 黄金路径（升级已有内网机）

在仓库根、已能 `ssh <Host>`（端口走 `~/.ssh/config`）：

```bash
python deploy/relkit.py build --serve --agent --os linux --arch amd64
python deploy/relkit.py upgrade --host update.devcloud.woa.com --plan --public-base-url https://update.devcloud.woa.com/
python deploy/relkit.py upgrade --host update.devcloud.woa.com --apply --restart --public-base-url https://update.devcloud.woa.com/
```

`--plan` 只读并打印脱敏探测结果。`--apply` 在目标机 `/var/backups/relkit/<utc>/` 备份后改文件；没有 `--restart` 则不切进程。失败会从该备份回滚。

upgrade **保留** 现网 `addr` / `dir` / nginx。它会：补 `gc.casGrace`、清 `casCredentials`、把可推导的 `local`/`http-put` 改成 `relkit-compatible`（推导不了就停）、按 live json 重写 `ReadWritePaths`。

目标机必须已有 Python 3.9+（`python3` 或 `/usr/bin/python3`）、`systemctl`、sudo。CAS 探针的 key 必须是 body 的 sha256，能力 PUT 不要带 publish protocol 头。

CI 在另一台机器时，`cas-put` 不能使用 serve 的 loopback 能力 URL。profile 的 `baseUrl` 必须是构建机可达的下载 origin，且该 origin 的 nginx 要对 `/cas/` 放行带签名的 PUT（见 `nginx-intranet.example.conf`）。本机 upgrade 自检仍打 127.0.0.1，不能代替这次发布验证。

## 首装

```bash
sudo python3 deploy/relkit.py install serve --binary ./dist/relkit-serve-linux-amd64
sudo python3 deploy/relkit.py install agent --binary ./dist/relkit-agent-linux-amd64
```

已有实例且 `--addr`/`--dir` 与现网不一致时，`install serve` 拒绝执行（避免把 30341 / `/srv/releases` 写进内网机）。这时用 `upgrade`。

## Token（两阶段）

```bash
python deploy/relkit.py token --host update.devcloud.woa.com --prepare
# 把打印的 RELKIT_SERVE_TOKEN 交给所有 agent / 凭据库（只出现一次）
python deploy/relkit.py token --host update.devcloud.woa.com --activate
```

`--activate` 才会 `systemctl restart`。产品隔离 token 加 `--product <id>`。

## 测试

```bash
python -m unittest deploy/test_relkit.py
```
