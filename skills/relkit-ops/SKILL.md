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

## 闸门短指针

- `sidecar.layout`：只认 lock 装到 `tools/bin/relkit-updater`；可选 `relkit.json` `sidecar.packScript` 只校验接线，不硬编码 `.mjs`。真产物归 `pack.ci`。
- `fake.release`：dummy staged 可丢；真实 `release --execute` 前清掉非当前版本的 staged 树。
- `updater.process`：封闭词 `rust` / `node` / `dart` / `go` / `other`。**不是** `rust-shell`。手写 DTO / `serde(default)` 吞缺键是 drift；以 `onboard explain updater.process` 为准。
- agent 发布：`release --execute` 必须由 CI 设 `RELKIT_RELEASE_VIA_CI=1`；本地不要发。
- `share-with`：磁盘 token 仍是**已有 owner** 的 `{owner}.token`，不打印新 token。细节以 `onboard explain token.isolation` 为准。

## 开箱前环境检查

在 `onboard start` 或问 `product.id` 之前：先 `status`，并读现有接线，判断 **fresh / upgrade / cleanup-then-continue**。不要把已有产品当成绿地。

至少看：`relkit.json`（`backends` 类型、`http-put`/`local` 残留、signing、directory URL）、`VERSION.json`（以及是否还留 `VERSION.yaml`）、`scripts/relkit.lock.json`、已提交的 `.relkit/onboarding.json`、serve/agent 是否已登记、token 文件名是否 owner、`scripts/host` 树哈希 vs lock、`tools/bin`、调用 `Updater.open` 的 sidecar 是否走官方 facade。

陈旧后端类型、孤儿 token 文件名等：先列出清理项并等人选，再写配置。

## 配置类决策（`ssh.host` 等同理）

在让人挑选之前先把配置读完，再给出**编号的具体候选项**。人决定；agent 不代选、不编造 Host、不手写 `~/.ssh/config`。

`ssh.host`：

1. 读 `~/.ssh/config` 以及每一条 `Include`（展开 `~`）。通配 `Host`（如 `*.devcloud.woa.com`）**本身不是可 SSH 的别名**。从 Include、`known_hosts`、产品文档（如 `relkit.json` 里的 `update.devcloud.woa.com`）收集真实主机名，并列出**实际匹配该模式**的 Host 别名。
2. 对照 `relkit_host.py` 的 `ssh_host_allowed` / `ssh_host_recommend`：闸门认精确 Host **和**能 `fnmatch` 上的名字；`recommend` 会同时列出 `candidates`（精确）与 `patterns`（通配）。只扫非通配 `Host` 行会漏掉 Include 里的机器（例如只看到 `git.woa.com` / `cvm-gz`）。
3. 用编号列表问人。写入用 `onboard set ssh.host <值>`，值必须能过闸门。

## 完成后

运行 `python scripts/host/relkit_host.py retrospect`，再运行 `status` 检查
`ops.retrospect` 已是 `verified`；只有前一命令退出码为 0，才能宣称开箱完成。
`RETROSPECT.md` 只是入口指针，阅读它本身不构成完成闸门。
