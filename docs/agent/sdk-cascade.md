# SDK 开箱级联

工具链开箱（[`toolchain-onboard.md`](toolchain-onboard.md)）解决「怎么发布」。  
本页解决「客户端怎么接」，并约定**多语言级联**方式。

## 级联规则（给 Agent）

1. 探测宿主语言（见 bootstrap 脚本输出）。
2. 打开**该语言**目录下的 `AGENT-QUICKSTART.md`，不要用另一语言的 API 硬套。
3. 若宿主是多语言（例如 Flutter UI + Go 工具），以**需要检查更新的那个进程**为准；必要时开两条清单。
4. 新增语言 SDK 时，必须同时：
   - 在本表登记一行；
   - 在该 SDK 根目录提供 `AGENT-QUICKSTART.md`（结构对齐 Dart/Go 现稿）；
   - 更新 `bootstrap.sh` / `bootstrap.ps1` 的探测分支。

## 当前已登记 SDK

| 语言 | 包 / 模块 | 开箱文档 | 依赖安装 |
|------|-----------|----------|----------|
| Go | `cnb.cool/shichao402/relkit/sdk` | [`../../sdk/AGENT-QUICKSTART.md`](../../sdk/AGENT-QUICKSTART.md) | `go get cnb.cool/shichao402/relkit/sdk@latest` |
| Dart | `rup_client`（**SSOT：`sdk/dart/`**） | [`../../sdk/dart/AGENT-QUICKSTART.md`](../../sdk/dart/AGENT-QUICKSTART.md) | git `path: sdk/dart`；内网宿主可镜像，见该产品 `VENDORED.md` |
| Node / TypeScript | `rup-client`（**SSOT：`sdk/node/`**） | [`../../sdk/node/AGENT-QUICKSTART.md`](../../sdk/node/AGENT-QUICKSTART.md) | `npm i`（本地 `file:../relkit/sdk/node` 或 git 依赖） |

目录说明见 [`../../sdk/README.md`](../../sdk/README.md)。

## 所有 SDK 共用的接入契约

客户端必须提供且与发布侧**逐字一致**：

| 字段 | 含义 | 失败表现 |
|------|------|----------|
| `product` | 产品 id | 整份 index 被拒 |
| `channel` | 如 `stable` | 拉错指针或 404 |
| `currentCode` | 当前已装 code | 选路错误 / 永远 up-to-date |
| `indexUrls` | 签名 index 的 URL 列表（串行尝试） | check 失败 |
| `fallbackUrls` | 签名 fallback 的 URL 列表（可选，救急催更） | 无催更 |
| `trustedKeys` | keyId → ed25519 公钥（**编译期内嵌**） | 验签失败 |
| `clientSelectors` | 如 `os`/`arch` | 找不到 artifact |

协议保证（SDK 已做，宿主不要重做一遍）：

- Index：Ed25519 验签 + sequence 防回滚
- Manifest / artifact：size + sha256
- 镜像串行，禁止竞速
- 下载：优先 Range 多连接；不支持则降级；可续传（以各 SDK 实现为准）

宿主负责：

- 何时检查、UI、是否安装
- Apply（换目录 / 装包 / 重启）；Go SDK 默认不带 apply，Dart 可选 `apply/`，Node 不提供 apply

## 开箱后冒烟

发布侧已有至少一个版本时：

1. 用比 head 更小的 `currentCode` 调 `check` → 应得到 available
2. `download` → 文件 sha256 与 manifest 一致
3. 断网 / 错公钥 → 应失败且不留下「通过校验」的假文件

然后把日常发版交回 `relkit agent-guide`。
