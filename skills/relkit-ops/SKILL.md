---
name: relkit-ops
description: >
  产品仓 relkit 开箱、发版、升 lock、在已有 serve/agent 上注册/列产品/轮换/吊销 token。
  有 scripts/host/relkit_host.py（或 scripts/relkit_host.py）时使用；换机上二进制才进 relkit 仓 deploy/relkit.py。
---

# relkit 运维

## 入口

- **产品仓**：只跑 `python scripts/host/relkit_host.py`（无参数只分流，不替你确认）。子命令、闸门、drift 以该脚本 `--help` / `onboard explain` / `status` 为准。
- **relkit 仓且要换箱子上的二进制**：`python deploy/relkit.py` 的 `build` / `install` / `upgrade`。空机首装不是产品开箱的一步。
- 判断不了就问人。不要手拼 SSH 写配置，不要编造命令输出。
- 开箱状态只在产品仓 `.relkit/onboarding.json`。`status` 会覆盖同目录 `onboarding.md`（gitignore 投影）。不要写进 skill cache / `docs/`。重置：`onboard reset --yes`。

Go 的 `relkit` / `relkit-serve` / `relkit-agent` 不是人用的第二套运维 CLI。发布协议仍由它们实现；常驻进程仍要跑。产品 token 与开箱决策只经 host.py。

## 完成后

先读同目录 [RETROSPECT.md](RETROSPECT.md)，对账后再宣称完成。
