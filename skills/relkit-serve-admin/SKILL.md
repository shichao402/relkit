---
name: relkit-serve-admin
description: >
  管理一台已经在跑的 relkit-serve：列出它放行了哪些产品、为某个产品签发隔离上传 token、轮换、吊销。
  当用户要求「给某台更新服务加一个产品」「换掉某产品的上传 token」「某产品下线/token 泄漏要吊销」
  「看看那台机现在放行了哪些产品」时使用。管理只走 SSH + 目标机本地 `relkit-serve init`，服务端没有管理 API。
---

# relkit-serve 服务端凭据管理

## 第一步：读操作手册

**执行任何写操作之前，必须先完整读一遍手册。** 它是操作性知识的唯一来源，本 skill 只声明边界与红线，不复制操作细节。

按顺序查找：

1. 目标机上 `relkit-serve agent-guide`（与那台机实际运行的构建配套，最可信）
2. 若当前工作区是 relkit 仓：[`cmd/relkit-serve/AGENT-GUIDE.md`](../../cmd/relkit-serve/AGENT-GUIDE.md)
3. 否则本机 checkout：`D:\workspace\GitHub\relkit\cmd\relkit-serve\AGENT-GUIDE.md`

重点是 §2.3 / §2.3.1（凭据分层）与 §5（轮换与吊销）。两处都读不到就停下来告知用户，**禁止**凭记忆执行 —— 这里有几个改错了会让整台机上所有产品一起发不出去的地方。

## 与 rup-release 的分工

判据是「动的是服务器的凭据，还是产品的发布」：

- **服务端增删产品 token、列产品、吊销** → 本 skill。
- **发一个版本、撤回版本、客户端收不到更新** → `rup-release`，本 skill 不碰发布流程。
- 两件事都要做时，先本 skill 把 token 发出去，再切到 `rup-release` 发布。

## 前提：SSH 身份不由本 skill 管理

登录用本机 OpenSSH 已经配好的身份 —— 那些 `Host` 与 `IdentityFile` 是 `dec pull`（user 平面）从 `bundle/woa`、`bundle/tencent-cloud` 落地写进 `~/.ssh/config` 的。

因此：

- **不要**为 relkit 新建一个存密钥、或存「密钥指针」的 bundle。SSH 身份已经有归属，复制一份只会多一处要同步的真相。
- **不要**把主机清单塞进 Bitwarden。主机名和配置目录不是机密，`~/.ssh/config` 本身就是清单。
- `ssh` 连不上就**停下来**报告（没 pull 过对应 bundle、Host 名不对、跳板不通），**禁止**改用别的凭据硬连，**禁止**编造已经连上并伪造输出。

## 定位实例

两件事必须确定，任何一件靠猜都会把改动写到没人读的地方：

1. **哪台机**：用用户点名的 `Host`。用户没点名时，从 `~/.ssh/config` 列出候选请他确认，不要自己挑一个"看起来像"的。
2. **哪个配置目录**：约定是 `/etc/relkit-serve`。不确定就上机确认，照抄启动日志里 `config:` 那一行：

```bash
ssh <host> systemctl status relkit-serve --no-pager
ssh <host> journalctl -u relkit-serve -n 30 --no-pager | grep -i '^.*config:'
```

`-out` 指错的后果是安静的：新配置写到了另一个目录，服务读的还是老的，看起来"改完了但没生效"。

## 四个动作

先 `ssh <host> relkit-serve -version` 证明二进制可用，再做下面任何一件事。`<dir>` 是上一节确认过的配置目录。

**列出已放行的产品**（只读，安全）：

```bash
ssh <host> sudo relkit-serve init -out <dir> -list-products
```

**为新产品签发隔离 token**：

```bash
ssh <host> sudo relkit-serve init -out <dir> -product <id>
```

`sudo` 签完后把配置目录收归服务用户（手册里的 `chown`），再重启。

**把新产品挂到已有 token 上**（同族 / 同一发布方，不签发新秘密）：

```bash
ssh <host> sudo relkit-serve init -out <dir> -product <id> -share-with <existing-id>
```

**轮换某产品的 token**（用 `-token-only`，不要用 `-force`）：

```bash
ssh <host> sudo relkit-serve init -out <dir> -product <id> -token-only
```

**吊销某产品**：

```bash
ssh <host> sudo relkit-serve init -out <dir> -product <id> -remove
```

三件写操作都要重启才生效，且轮换/吊销的顺序不能反：**先把新 token 交到发布方手里，再重启**；吊销则相反，重启越早越好。**生产机重启必须等用户明确说可以**，不要默认 `systemctl restart`。`-share-with` 不打印明文，但仍要重启才放行新产品 id。

```bash
ssh <host> sudo systemctl restart relkit-serve
```

**禁止**用 `sed`、`jq -i` 或手写 JSON 去改 `uploadTokens`。手写的条目不过校验，最常见的结果是配置指向一个不存在的 token 文件，服务下次重启直接起不来，一台机上所有产品一起停发。

## token 的落地：只进凭据库，不进聊天

`init` 打印的那行 `export RELKIT_UPLOAD_TOKEN=...` 是**唯一一次**能看到明文的机会：服务端只存 sha256，事后无法反查。没接住就只能再轮换一次，**不要猜、不要编**。

拿到之后立刻落地：

1. 写进**该产品所在项目**的 `.secrets/project/`（Bitwarden folder = 那个仓库 `.dec/config.yaml` 的 `project_name`，例如 `SvnMergeTool`）。上传 token 属于项目身份，不要为此新建 Dec bundle；`woa` / `tencent-cloud` 才是 bundle。
2. 用 Dec 把 Note 推到上述 folder；当前工作区不是该产品仓库时停下来换根。
3. 回复里只说「已写入项目 `<project_name>` 的 `<Note 名>`」，**不带明文**。

当前工作区不是该产品的仓库时（Dec 的 MCP 默认作用当前项目根），**停下来**让用户切到那个仓库或明确授权，不要把 B 产品的 token 写进 A 项目的 `.secrets/`。

**吊销之后归档，不要直接抹掉本地证据。** 远程 `-remove` 成功后：把该项目 `.secrets/project/` 里对应的上传 token 条目改名或移到归档路径（例如加 `.revoked` 后缀），再推 Bitwarden。`dec_delete` 要用户 `confirmed=true`，默认不删远端 Note。

同样**禁止**：把明文贴进回复、提交信息、工单、聊天记录，或任何进版本控制的文件。

## 边界

不属于本 skill：

- 首次部署一台新机（装二进制、建用户、写 systemd）→ 照手册 §3 走。
- 运营方全树 token 的轮换 → 手册 §5，那是运维凭据，不要顺手塞给某个产品的 CI。
- 给服务端加 HTTP 管理接口 → 明确不做，理由见手册 §2.3.1。用户要求时先说明「上传 token 不是管理口令」，再问是不是 SSH 不通。

## 完成后汇报

- 哪台机、实际生效的配置目录（照抄启动日志）；
- 做了什么（签发 / 轮换 / 吊销 / 只是列出），涉及哪个产品；
- `-list-products` 的结果，用来证明现在放行的就是预期的那些；
- 是否已重启 —— 没重启就明说「改动尚未生效，旧 token 仍然可写」；
- token 存到了哪个项目 folder 的哪个条目（**不带明文**），以及还需要谁去 CI 里替换。
