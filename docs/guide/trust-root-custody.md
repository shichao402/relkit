# 信任根密钥托管规范

本文回答三个问题：密钥放在哪、怎么确认它在、真丢了该怎么办。

依据 [ADR 0023](../adr/0023-desktop-updater-epoch.md) 决策 4 与 [现行架构](../architecture.md) §二。

## 模型：每台服务器一把根

每台 agent 服务器各有一把 Ed25519 根密钥对。它只做一件事：证明某份文档或文件是这台服务器发出的、没被改过。

| 服务器 | 根 keyId | 指纹 | 私钥位置 |
| --- | --- | --- | --- |
| 内网（tencent-internal） | `tencent-internal-server-root`（原 `relkit-candidate-2026b`） | `sha256:3dc1ccb4…bf33` | 归档中 |
| 外网（firoyang.com） | `firoyang-public-root`（2026-10-27 整机重建时生成） | `sha256:06253482…8e4c` | 归档中，服务器副本 `/etc/relkit-agent/keys/` |

外网那把是**新服务器身份**，不是"私钥找不到后换根"，不违反下文硬规则。

已删除，不再有任何形式的存在：Distribution Root `relkit-distribution-2026`、每产品一把的产品签名密钥、Runtime Profile Root、Candidate Root。四把根分权已由 ADR 0023 驳斥（同一台机器上的私钥一起泄露，分域不产生隔离效果）。

## 为什么要有这份文档

2026-10-20 之前，`relkit-candidate-2026` 与 `relkit-candidate-2026b` 两次被判定为"私钥丢失"。
两次都不是真丢，两次都是**没记录去哪找**。

真正的缺陷不在密钥本身，在托管规划：

- 私钥副本散落在 `tmp/` 与 `.sim-scratch/` 这类被 `.gitignore` 忽略的**草稿目录**。这类目录的语义就是"随时可删"，`git clean -xdf` 一条命令就能清空，且不属于任何备份范围。
- 没有任何权威入口可以查询"这把私钥在哪"。要用的时候只能靠记忆或全文搜索，搜不到就得出"丢了"的错误结论。
- 一旦误判丢失，处置动作就是"再生成一把"，即换根。而换根是一个代价被严重低估的动作（见下文），一次误判的代价远大于查一次登记表。

结论：不是不该用这把密钥，而是要**按最佳实践用它**——正确登记、正确校验、正确评估换根。

## 权威归档位置

| 项目 | 值 |
| --- | --- |
| 归档根 | `D:\relkit-trust-archive\keys` |
| 登记表 | `D:\relkit-trust-archive\keys\custody-manifest.json` |
| 自检命令 | `python D:\workspace\GitHub\relkit\scripts\check_trust_keys.py` |

归档根刻意放在**所有 git 仓库之外**。仓库内的 `tmp/`、`.sim-scratch/` 一律视为草稿，不作为托管位置。

登记表是单一权威入口，格式为 `relkit.custody-manifest/1`（见 [schema](../schemas/relkit.custody-manifest.1.schema.json)）。每台服务器各登记一条 `kind: root` 条目，含 keyId、用途、载体、公钥指纹。新增或轮换信任根时必须更新它，否则该密钥视为未托管。

清单不是全局文件，而是每个持有钥匙的地方各有一份、格式相同、读取代码相同：

| scope | 在哪 | 装什么 |
| --- | --- | --- |
| `server` | 每台 agent 服务器 | 该服务器的根私钥、token 存储位置 |
| `host-ci` | 随宿主项目配置提交 | 只写 keyId 与环境变量名，不含任何秘密 |
| `archive` | `D:\relkit-trust-archive\keys\` | 离线备份的私钥副本与公钥 |

## 只按 keyId 取钥匙

代码与脚本不再知道任何钥匙放在哪，只知道 keyId。要用钥匙时拿 keyId 问清单，清单告诉它从哪个文件或哪个环境变量读。这条规矩写在 [现行架构](../architecture.md) §三，靠评审遵守，不做自动检查。

Go 程序用 `internal/custody`。Python 与 shell 脚本调用命令，不各写一份解析：

```bash
relkit custody check --manifest <manifest> --key <keyId> --json
relkit custody get --manifest <manifest> --key <keyId>
relkit custody public --manifest <manifest> --key <keyId>
```

反面例子（断代前）：agent 配置写 `privateKeyPath`，store 和 agent 各自读 token 文件，部署脚本与 CI 脚本各自直接读环境变量，全仓约 13 个文件各管各的。这些已全部改为 keyId 引用。

## 自检做什么

`scripts/check_trust_keys.py` 把"密钥在哪、是不是同一把、还对不对"变成一条可重复执行的命令。它调用 `relkit custody check`，不自己解析清单：

1. 读登记表，逐条列出公钥与私钥副本。
2. 对公钥取指纹，与登记值比对。指纹漂移即 FAIL。
3. 对私钥做一次**本地签名往返**，确认签名产出的 keyId 等于登记值。这一步是关键：文件存在、大小对、甚至 SHA256 对，都不能证明它是那把私钥；只有签名结果能。
4. 打印每份副本的字节数与 SHA256，缺失项明确点名。

退出码 0 表示全部一致，1 表示有缺失、漂移或签名不匹配。

已验证的负向行为：把私钥临时改名藏起来，脚本立即返回 `RESULT = FAIL` 并输出 `relkit-candidate-2026b: private key missing`。

agent 启动时也做同样的事：校验所载私钥与登记的本服务器根公钥一致，不一致拒绝启动。

## 硬规则

**任何一次"私钥找不到"，先跑自检脚本。脚本说没有，才算真的没有。**

禁止在未执行自检前就断言密钥丢失，更禁止在断言丢失后直接 `key generate` 生成新根。

## 真丢了要付什么代价

"重新配一把有什么问题"——先看清换根的实际影响面，再决定。

断代后不设计换根流程，不做新旧根重叠窗口。换根的唯一后果是：这台服务器上所有已装应用失去信任链，必须重装才能继续接收更新。

| 影响面 | 具体动作 |
| --- | --- |
| 该服务器上所有已装桌面应用 | 内嵌的是旧根公钥，验不过新签的 index/manifest，必须重装 |
| 三宿主项目配置 | `rootPublicKey` 改为新根；CI 的 `allowed_signers` 同步 |
| 该服务器已签文档 | Catalog、Profile、各产品 index/manifest、工具签名全部用新根重签 |
| 凭据清单 | 登记新 keyId 与指纹，旧条目标作废 |
| 归档 | 新私钥入归档并离线备份 |

正确的处置顺序永远是：先自检 → 确认不在 → 再评估是否真的必须换根 → 换根时按上表逐项做完，并同步更新登记表。

## 何时才该真的换根

换根是合规动作，不是救急动作。以下情况应当换根：

- 自检确认私钥在所有登记位置均不存在，且无法从任何备份恢复。
- 私钥疑似泄露（这是换根最主要的正当理由，且必须立即执行）。

以下情况**不构成**换根理由：

- "我一时找不到"——先跑自检。
- "重新配一把比较快"——上表的代价远大于查一次登记表。

## 相关

- [现行架构](../architecture.md)：信任与凭据模型
- [ADR 0023](../adr/0023-desktop-updater-epoch.md)：每服务器一把根的决策
- [ADR 0019-0022 驳斥](../adr/0019-0022-refutation.md)：四把根分权为何被拆
- [凭据清单 schema](../schemas/relkit.custody-manifest.1.schema.json)
- 自检脚本：[scripts/check_trust_keys.py](../../scripts/check_trust_keys.py)
