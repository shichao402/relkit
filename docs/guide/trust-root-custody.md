# 信任根密钥托管规范

本文回答三个问题：密钥放在哪、怎么确认它在、真丢了该怎么办。

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

登记表是单一权威入口。新增或轮换信任根时必须更新它，否则该密钥视为未托管。

## 自检做什么

`scripts/check_trust_keys.py` 把"密钥在哪、是不是同一把、还对不对"变成一条可重复执行的命令：

1. 读登记表，逐条列出公钥与私钥副本。
2. 对公钥取指纹，与登记值比对。指纹漂移即 FAIL。
3. 对私钥做一次**本地签名往返**（`relkit-ops operator sign`），确认签名产出的 `keyId` 等于登记值。这一步是关键：文件存在、大小对、甚至 SHA256 对，都不能证明它是那把私钥；只有签名结果能。
4. 打印每份副本的字节数与 SHA256，缺失项明确点名。

退出码 0 表示全部一致，1 表示有缺失、漂移或签名不匹配。

已验证的负向行为：把私钥临时改名藏起来，脚本立即返回 `RESULT = FAIL` 并输出 `relkit-candidate-2026b: private key missing`。

## 硬规则

**任何一次"私钥找不到"，先跑自检脚本。脚本说没有，才算真的没有。**

禁止在未执行自检前就断言密钥丢失，更禁止在断言丢失后直接 `key generate` 生成新根。

## 当前登记清单

| keyId | 角色 | 指纹 | 私钥 |
| --- | --- | --- | --- |
| `relkit-candidate-2026b` | candidate-root | `sha256:3dc1ccb4…bf33` | 在归档中 |
| `relkit-distribution-2026` | distribution-root | `sha256:8bcb6c24…9598` | 不在开发机（仅公钥） |

`relkit-candidate-2026b` 是三产品现行 resolution 的 `authorityFingerprint`，同时硬编码在三条蓝盾流水线的 `--candidate-root-fingerprint` 参数里。

## 真丢了要付什么代价

"重新配一把有什么问题"——先看清换根的实际影响面，再决定：

| 影响面 | 具体动作 |
| --- | --- |
| 三产品项目声明 | 不需要改（`trustFingerprint` 指向的是 distribution root，不是 candidate root） |
| 三产品已装 resolution | `authorityFingerprint` 全部失效，必须重新离线安装 bundle |
| 三条蓝盾流水线 | 每条的 `--candidate-root-fingerprint` 硬编码需改（loom-editor 2 处、loom-launcher 2 处、svnmergetool 3 处） |
| control-plane catalog | 需用新根重签，sequence 递增 |
| operator profile | 需用新根重签，sequence 递增；旧 profile 的 watermark 会因内容变更判 equivocation，必须保证新 sequence 更大 |
| 已发布 candidate bundle | 三个产品全部作废重打包重上传，因为 bundle 内嵌信任根 |
| Pre-release | 需发新 tag（如 `control-plane-candidates-v0.6.23`），旧 tag 保留 |

也就是说，换根不是"生成一个文件"，而是"重做一整轮控制面签发与三产品 bundle 分发"。这把密钥正常使用时只是签两个 JSON；它丢失时的处置成本，是这个签发流程的完整重跑，外加三个产品各自重新安装。

正确的处置顺序永远是：先自检 → 确认不在 → 再评估是否真的必须换根 → 换根时按上表逐项做完，并同步更新登记表。

## 何时才该真的换根

换根是合规动作，不是救急动作。以下情况应当换根：

- 自检确认私钥在所有登记位置均不存在，且无法从任何备份恢复。
- 私钥疑似泄露（这是换根最主要的正当理由，且必须立即执行）。

以下情况**不构成**换根理由：

- "我一时找不到"——先跑自检。
- "重新配一把比较快"——上表的代价通常远大于查一次登记表。
- "反正是测试密钥"——`relkit-candidate-2026b` 是三产品生产线正在使用的生产候选根，不是测试密钥。

## 相关

- 签名与 catalog/operator 语义：[ADR 0020](../adr/0020-vnext-catalog-operator-resolution.md)
- 离线 bundle 与候选根：[cutover.md](../design/vnext/cutover.md)
- 自检脚本：[scripts/check_trust_keys.py](../../scripts/check_trust_keys.py)
