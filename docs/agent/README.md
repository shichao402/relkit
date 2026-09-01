# Agent 开箱入口（RUP / relkit）

> **这是接入项目的 Agent 应阅读的第一份文档。**  
> 目标：从「空项目 / 未接入」走到「能发布 + 客户端能检查更新」。  
> 读完并按清单执行后，应能在不追问架构背景的情况下完成开箱。

这份文档**不是**：

| 文档 | 用途 | 何时再读 |
|------|------|----------|
| [`docs/adr/*`](../adr/) | 设计决策 | 要改协议/拆仓库时 |
| [`embed/AGENT-GUIDE.md`](../../embed/AGENT-GUIDE.md)（`relkit agent-guide`） | **已接入后**怎么发版、红线、排障 | 开箱完成后再用 |
| [`cmd/relkit-serve/AGENT-GUIDE.md`](../../cmd/relkit-serve/AGENT-GUIDE.md) | 分发服务部署运维 | 需要自托管 serve 时 |
| 各 SDK `README.md` | 给人读的 API 简介 | 开箱清单走完之后 |

## 0. 30 秒判定你要干什么

在宿主项目根目录看迹象：

| 现象 | 你要走的开箱路径 |
|------|------------------|
| 没有 `relkit.json` / `VERSION.json`，用户要「接入自动更新」 | **A 工具链开箱** → 再 **B SDK 开箱** |
| 已有 `relkit.json`，用户要「发一版 / 排查客户端收不到」 | 跳过开箱，直接 `relkit agent-guide` |
| 只有客户端、发布已由别人接管 | 只走 **B SDK 开箱** |
| 只要自建下载站 | `relkit-serve agent-guide`（不必整套 SDK） |

**禁止**：在未确认路径时就改 `minFrom`、手改远端 index、或把私钥写进仓库。

## 1. 推荐执行顺序（一键开箱）

```text
1. 读完本文 §0–§2
2. 跑 docs/agent/bootstrap（见下）—— 它只做探测与指路，不偷偷改仓库
3. 按打印出的路径打开下一份文档并逐项打勾
4. 开箱完成标准（§3）全部满足后，才允许声称「已接入」
```

### 跑级联探测脚本

在 **relkit 仓库根**（或已把本目录拷进上下文时）：

```bash
# Unix
./docs/agent/bootstrap.sh /path/to/host-project

# Windows PowerShell
./docs/agent/bootstrap.ps1 -HostRoot D:\path\to\host-project
```

脚本会：

1. 检查 `relkit` / `relkit-serve` 是否在 PATH
2. 探测宿主是否已有 `relkit.json`、`VERSION.json`、Dart / Go / Node 工程迹象
3. **打印下一步必须打开的文档路径**（工具链 + 对应语言 SDK）
4. 不写入宿主文件（写入只由后续清单里的显式命令完成）

语言 SDK 采用**级联**：当前入口是 Go / Dart / Node；以后每加一种语言，在 [`sdk-cascade.md`](sdk-cascade.md) 登记，并在该 SDK 目录放 `AGENT-QUICKSTART.md`，bootstrap 自动挂上。

## 2. 文档地图

| 步骤 | 文档 | 产出 |
|------|------|------|
| A | [`toolchain-onboard.md`](toolchain-onboard.md) | 宿主具备 `VERSION.json` + `relkit.json` + 密钥约定 + 首次可 `publish`/`verify` 的路径 |
| B | [`sdk-cascade.md`](sdk-cascade.md) → 各语言 `AGENT-QUICKSTART.md` | 宿主进程能 `check` / `download`（apply 视平台） |
| 之后日常 | `relkit agent-guide` | 发版 / simulate / 红线 |

Go SDK 开箱：[`../../sdk/AGENT-QUICKSTART.md`](../../sdk/AGENT-QUICKSTART.md)  
Dart SDK 开箱：[`../../sdk/dart/AGENT-QUICKSTART.md`](../../sdk/dart/AGENT-QUICKSTART.md)  
Node SDK 开箱：[`../../sdk/node/AGENT-QUICKSTART.md`](../../sdk/node/AGENT-QUICKSTART.md)

## 3. 开箱完成标准（全部勾上才算 Done）

工具链侧：

- [ ] `relkit --version` 可用
- [ ] 宿主根有 `VERSION.json`（`schema: rup.version/1`）且 `relkit version get` 成功
- [ ] 宿主根有 `relkit.json`，`product` / `channel` / 后端配置齐全
- [ ] 公钥策略已定：客户端将**编译内嵌**公钥；私钥只在本机/CI 密钥库
- [ ] 至少做过一次 dry-run 或 `local`/`http-put` 试发布，并理解 `verify`

SDK 侧（按语言）：

- [ ] 依赖已加入（Go module / Dart pub）
- [ ] `product`、`channel`、`indexUrls`、`trustedKeys`、`clientSelectors` 与发布侧一致
- [ ] 进程内能跑通 `check`；有产物时可 `download` 且 sha256 校验通过
- [ ] Apply（若需要）有明确宿主策略；未实现则文档里写明「仅下载到目录」

## 4. 对 Agent 的硬性要求

1. **先读清单、再改文件**。不要同时 refactor 业务代码。
2. **私钥永不入库、不进对话摘要、不写进 `relkit.json`。**
3. **版本号只认 `VERSION.json` + `relkit version …`**，禁止再发明 YAML/自定义解析器。
4. **镜像 URL 可多、清单必须一份**；index/manifest 字节在各镜像上相同。
5. 开箱完成后，发版一律改走 `relkit agent-guide`，不要靠记忆拼命令。
6. 若宿主已有另一套自动更新，停下来问用户；不要默默双轨。
