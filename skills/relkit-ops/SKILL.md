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

Go 的 `relkit` / `relkit-serve` / `relkit-agent` 不是人用的第二套运维 CLI。发布协议仍由它们实现；常驻进程仍要跑。产品 token 与开箱决策只经 host.py。

## 状态与缓存

- 决策记录：**提交** `.relkit/onboarding.json`。瞬时文件只在 `.relkit/cache/`（`onboarding.md`、`onboarding.local.json`、staged 树）。
- `ensure_gitignore` 只保证 `.relkit/cache/` 与 `.relkit-keys/*.private.pb`。不要把产品状态写进 skill cache / `docs/`。重置：`onboard reset --yes`。

## 批量决策优先

1. 先运行 `onboard start` / `onboard inspect` 完成现状检查，再单独向用户确认本次意图：`fresh`（新接入）、`reconfigure`（重新开箱）或 `upgrade`（沿用已确认决策升级）。现状与意图冲突时先解释冲突，不猜。
2. 运行 `onboard questions --intent <intent> --json`。把其中 `questions` **一次性**交给用户；不要按旧步骤逐项来回问。问题中无安全推荐值时必须保留开放选择。
3. 将整批回答按输出的 `answerShape` 写入 `.relkit/cache/`，运行 `onboard apply --answers <file>`。它按 `revision` 检查现状是否变化，并原子校验全部答案；任何冲突都不会写入部分状态。
4. 若脚本返回冲突，只重问报错项；若 revision 过期，重新生成问题批次并只问变化项。
5. 决策落盘后再执行 action steps。可以并行委派互不写同一文件、互不改同一远端状态的调查或实现；共享产品状态、同一配置文件、serve/agent 注册与 release 必须按依赖顺序经 `relkit_host.py` 执行和复核。

逐项 `onboard set` 只作为修改单个已知答案的兼容入口，不是默认开箱体验。脚本保持非交互；由 agent 使用结构化提问收集用户批量答案。

## 闸门短指针

- 开箱前先跑 `onboard start` / `onboard inspect`：脚本会列出 `relkit.json` backends、VERSION、lock、SSH Include/通配匹配主机。`http-put` / `local` 等陈旧类型是 error，挡住 `product.id`。不要用手写确认代替 inspect。
- `ssh.host`：问人之前脚本已展开 `~/.ssh/config` 的 Include 与通配，并列出 exact / patterns / matched。通配本身不是 SSH 别名。写入 `onboard set ssh.host <值>`。
- `sidecar.layout`：只认 lock 装到 `tools/bin/relkit-updater`；可选 `relkit.json` `sidecar.packScript` 只校验接线，不硬编码 `.mjs`。真产物归 `pack.ci`。
- `fake.release`：只跑 `relkit_host.py fake verify`。缺 staged 树时脚本自己 dummy stage + simulate，禁止手调 `relkit.exe stage`。
- `updater.process`：封闭词 `rust` / `node` / `dart` / `go` / `other`。**不是** `rust-shell`。手写 DTO / `serde(default)` 吞缺键是 drift；以 `onboard explain updater.process` 为准。
- agent 发布：`release --execute` 必须由 CI 设 `RELKIT_RELEASE_VIA_CI=1`；本地不要发。
- `share-with`：磁盘 token 仍是**已有 owner** 的 `{owner}.token`，不打印新 token。细节以 `onboard explain token.isolation` 为准。
- 失败记账：带 `code=` 的 `Fail` 写入 `.relkit/cache/ops-journal.jsonl`（`unclassified` 不记）。`retrospect` 输出本次遇到 / 已修进脚本或 skill / 未消化；未消化非 0。

## 完成后

运行 `python scripts/host/relkit_host.py retrospect`，再运行 `status` 检查
`ops.retrospect` 已是 `verified`；只有前一命令退出码为 0，才能宣称开箱完成。
`RETROSPECT.md` 只是入口指针，阅读它本身不构成完成闸门。
