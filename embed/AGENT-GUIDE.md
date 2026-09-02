# RUP 发布操作手册（面向 Agent）

本文是**已接入 relkit 之后**发版、红线与排障的操作性 SSOT。  
若项目**还没开箱**（没有 `relkit.json` / 客户端未接 SDK），先读仓库内：

**[`docs/agent/README.md`](../docs/agent/README.md)**

不要从本文第 3 节直接开干。

`skills/rup-release/SKILL.md` 只做能力声明并指回本文，不复制内容 —— 任何操作细节的修改都只改这里。

三份文档的分工：

| 文档 | 回答的问题 | 读者 |
|---|---|---|
| `SPEC.md` | 协议是什么，字段与算法如何定义 | 实现客户端或工具的人 |
| `CLI.md` | 发布工具应该被设计成什么样 | 实现工具的人 |
| **本文** | **拿这个工具怎么干活，哪里会出事** | **执行发布任务的 Agent** |

---

## 0. 先确认工具可用

`relkit` 是否可用**必须实测确定**，不要假设。执行任何流程之前先跑一次：

```bash
relkit --version
```

正式工具是 Go 单二进制（[cnb.cool/shichao402/relkit](https://cnb.cool/shichao402/relkit)）。安装任选其一：

```bash
go install cnb.cool/shichao402/relkit/cmd/relkit@latest
# 或从 https://cnb.cool/shichao402/relkit/-/releases 下载对应平台二进制并放进 PATH
```

`relkit --version` 失败就说明工具在当前环境不可用。此时：

- **禁止**声称执行了任何 `relkit` 命令，**禁止**编造命令输出。
- 直接告知用户工具不可用，并询问是要先安装还是先实现它。

**当前实现范围**（用 `relkit backends` 确认本次构建实到哪一步）：

| 能力 | 状态 |
|---|---|
| `init` `keygen` `version` `stage` `inspect` `simulate` `publish` `fallback` `verify` `agent-guide` `backends` | 可用 |
| `local` 后端（输出完整 key 目录树，可离线跑通全流程） | 可用 |
| `static-http` 后端（任何按路径提供 HTTP 下载的托管，校验走真实 HTTP） | 可用 |
| `http-put` 后端（带鉴权 PUT 上传，配 relkit-serve 或任何 PUT / WebDAV 端点） | 可用 |
| `s3-compatible` 后端（COS / S3 / MinIO，SigV4 上传；客户端经 `baseUrl` 下载） | 可用 |
| `github-release` / `cnb-release` 后端 | **未实现** |
| `yank` / `unyank` / `min-supported` 命令 | **未实现** |
| JSON Schema 完整校验（`verify` 目前只检查关键字段的结构） | **未实现** |

关于后端怎么选，看两件事：**产物由谁送上去**、**URL 是否可预测**。

**推荐生产拓扑**（详见仓库 `docs/design/update-ingress-cos.md`）：客户端内嵌几乎不变的 `entryUrls`，主入口为**自有域名绑定 COS** 上的 `directory/<product>.pb`；发布在可信 CVM 上持钥执行，再经 `s3-compatible` 写到 COS（及 CNB/GitHub 备援）。directory / index 等是签名文档，可以进 COS；Bearer 式上传服务进不了 COS。

- 托管方按可预测路径提供 HTTP 下载，且产物已有别的机制送达（仓库 CI、rsync、独立上传步骤）→ 用 `static-http`，配 `stageDir` 指向那个机制会取用的目录。CNB 仓库直链属于这一类。
- 自己掌管一台服务器 → 数据面用 `relkit-serve` 或 nginx 做 Range GET；**写入走 `relkit-agent`**（`local` 后端写同一目录）。`http-put` + serve PUT 是遗留路径。token 只从环境变量读。serve **不作**公网推荐主入口（公网数据面是 COS）。
- 需要工具自己带凭据上传到对象存储（COS / S3）→ 用 `s3-compatible`（`endpoint` / `bucket` / `prefix` / `baseUrl` / `accessKeyEnv` / `secretKeyEnv`；可选 `region` / `forcePathStyle` / `timeoutSeconds`）。
- 只想审计别人发布的站点 → `static-http` 不配 `stageDir`，得到一个只读后端，`verify` 可用而 `publish` 会直接拒绝。

后端镜像切换（如 COS → CNB）靠双写 + 改 directory / `urls[]`，不改客户端常量；步骤与旧 manifest 单 URL 陷阱见 `docs/design/update-ingress-cos.md` §8。

**禁止**用 `local` 后端加手工上传冒充一次正式发布。手工上传绕过了「指针最后写」这个保证，中途出错会让客户端看到半个发布。若产物确实由外部机制搬运，那就用 `static-http` + `stageDir`，让工具仍然掌管写入顺序。

另外，规范性行为随时可以单独自检（在 relkit 仓库）：

```bash
go test ./internal/chain ./internal/selectors ./internal/envelope
```

它跑的是选路、可达性、产物选择、验签四处逻辑，且校验的就是发布时真正执行的那份代码。

---

## 1. 适用判定

**适用：** 项目根目录存在 `relkit.json`（以及配套的 `VERSION.json`），或用户明确要求用 RUP / relkit 发布、撤回版本、设置强制更新下限、排查客户端收不到更新、为项目接入自动更新。

**不适用：** 项目用的是别的发布机制，且**没有** `relkit.json`。判断方法：看有没有 `relkit.json`，没有就问，不要猜。一旦采用 relkit，版本号必须以 `VERSION.json`（`rup.version/1`）为 SSOT，用 `relkit version …` 读写，禁止再维护第二套版本解析器。

---

## 2. 红线

这些错误的共同特征是**发布后无法通过再发一个版本来补救**。执行任何写操作前先过一遍。

### 2.1 `minFrom` 配错会永久锁死客户端

若某个仍在使用的旧版本找不到满足 `minFrom` 的下一跳，它将永远收不到更新，只能靠用户手动重装。事后再发新版本也救不回来，因为它连"有新版本"都不知道。

**因此：任何新增版本或抬高 `minFrom` 的操作，必须先运行 `relkit simulate --with-staged <version> --from all` 并确认每个起点都能到达最新版本。** 工具的可达性校验会拦下大部分情况，但 `simulate` 能让你在动手之前就看见谁会被卡住。注意必须带 `--with-staged`：不带它就只模拟了远端现状，看不到本次待发布节点的影响，等于什么都没验。

### 2.2 `code` 必须严格单调递增

新版本的 `code` 必须大于远端所有现有 `code`。倒退会导致新版本在客户端看来"比已装的旧"，所有人都收不到更新，且现象极不直观。

CI 构建号（`GITHUB_RUN_NUMBER` 等）在更换 CI 平台、重建仓库、重命名 workflow 时会退回 1。遇到这类迁移，用 `env:VAR+N` 抬高偏移量。

### 2.3 私钥

**禁止**把私钥写入仓库、`relkit.json`、命令行参数、日志、对话或共享环境变量。私钥只经该产品 `signing.privateKeyPath` 指向的本机受限文件传入。若发现私钥已进入版本控制，停下并告知用户需要轮换密钥，不要试图用 `git rm` 掩盖。

### 2.4 禁止手工编辑远端 index 与 manifest

`index` 是签名对象。手工改动会使签名失效，客户端将拒绝该源（且按规范不会回退到"不验签直接用"）。所有修改都必须经 `relkit publish` / `yank` / `min-supported`，由工具重新签名并递增 `sequence`。

### 2.5 禁止为不同镜像生成不同清单

所有后端上的 `index` 与 `manifest` 必须**字节完全相同**，镜像差异只体现在 `urls` 数组多几个元素。为每个镜像各生成一份内容不同的清单会导致签名要签多次、内容要维护多份、镜像间可能不一致。

### 2.6 撤回前必须重新校验可达性

`yank` 会改变"可达的最新版本"，可能切断其他版本的升级路径。撤回一个坏版本时最不该发生的事，就是顺手把所有人锁死。工具会强制校验，**禁止**用任何跳过参数绕过。

---

## 3. 流程 A：发布一个新版本

前提：产物已构建完成，`relkit.json` 已存在。

```
- [ ] 1. 确认版本号、code、发布通道
- [ ] 2. 确认 minFrom
- [ ] 3. stage
- [ ] 4. simulate 复核升级路径
- [ ] 5. dry-run
- [ ] 6. publish
- [ ] 7. verify
```

**1. 确认版本号、code、通道。** 版本字符串的权威来源是根目录 `VERSION.json`（`relkit version get`）。`code` 按 `relkit.json` 的 `codeStrategy` 取值：新项目默认 `version-build`（code = `+build` 段）；`explicit` 必须显式传 `--code`；`semver` 与 `env:*` 由工具自行取值，此时**不要**再传 `--code`。通道缺省用 `defaultChannel`，用户说"发预览版 / beta"时才切到 `beta`。

改号只用官方入口：

```bash
relkit version set 1.5.0+120
relkit version bump build
relkit version code          # 打印将用于发布的 code
```

`relkit stage` / `publish` 可省略版本参数，此时直接读 `VERSION.json`。

**2. 确认 `minFrom`。** 见 §6.2 决策表。**默认填 0**，只有在存在不可跳过的迁移时才抬高。不确定时问用户，不要自己假设。

整包更新、不需要过程版本的产品，在 `relkit.json` 设 `"retainVersions": 1`（或保留最近几个，如 `2`/`3`）。发布时 index 只保留最高 code 的 N 个节点；`relkit-serve` 的孤儿 GC 随后会删掉不再被引用的旧 `manifest/` / `artifact/`。缺省 `0` 表示保留完整历史（过程升级链仍需要）。**不要**在仍依赖 `minFrom` 中间跳的产品上把保留数裁到断链。

**3. stage。** 纯本地操作，不联网，可反复重跑。

```bash
relkit stage 1.5.0 --code 150 --min-from 0 \
  --add dist/app-win-x64.zip   os=windows,arch=x64 \
  --add dist/app-mac-arm64.zip os=macos,arch=arm64
```

核对工具打印的 `kind` 推断结果与自动生成的 `id`，确认符合预期。产物默认被拷贝进 `.relkit/staged/<version>/artifacts/`，因此这一步之后源目录可以被清理或重建。

**4. simulate 复核。** 即使 `minFrom` 填的是 0 也要跑，它顺带验证了链条整体没有被之前的发布破坏：

```bash
relkit simulate --with-staged 1.5.0 --from all
```

逐行核对每个起点的落点。这一步不写任何东西，也不需要私钥。

**5. dry-run。** 完成全部校验并打印计划，不产生任何副作用：

```bash
relkit publish 1.5.0 --dry-run
```

**6. publish。** 顺序由工具保证：先校验，再上传产物与清单，**最后**写 `index` 指针。指针写入之前的任何失败都是安全的（新版本对客户端还不可见），直接重跑即可。

给人看的索引：publish 会在 `.relkit/browse/` 写出 `index.html`、`<product>.html`、`catalog.json`。`HostsBrowse` 的后端（`local` / `http-put`）把同一份 dump 写到数据面 `browse/`；配了 `site.makers` 且本轮有协议专用后端（如 COS）时，再 Upload 到 Makers。`--to local` 即使配了 makers 也跳过。换站点托管加 BrowseSink 实现，不要按后端 `Type()` 猜测。见 `docs/design/publish-topology.md`。

**7. verify。** 确认各后端一致、哈希吻合：

```bash
relkit verify
relkit verify --deep   # 逐个 artifact 发 HEAD，确认可匿名访问
```

首次接入某个后端、或后端的匿名访问策略不明确时，`--deep` 是回答"客户端到底下不下来"最直接的手段。

---

## 4. 流程 B：接入一个新项目

```
- [ ] 1. keygen
- [ ] 2. 确认私钥已被忽略
- [ ] 3. init 并填写 relkit.json
- [ ] 4. 先用 local 后端端到端跑通
- [ ] 5. 客户端内嵌公钥与 index URL
- [ ] 6. 接入真实后端
```

**1–2. 生成密钥并确认私钥安全。** `relkit keygen --key-id k1 --out keys/`。公钥文件可以提交（客户端要内嵌它），私钥文件绝不可以。确认 `.gitignore` 覆盖了私钥后再继续。

**3. 填写 `relkit.json`。** 结构见 `CLI.md` §5。凭据一律只写环境变量名。协议对象走 `publishTo` 后端；人页走 BrowseSink（`HostsBrowse` 的数据面 `browse/`，或 `site.makers`）。拓扑见 `docs/design/publish-topology.md`。

**4. 先跑通 `local` 后端。** 它把完整目录树输出到本地，整个发布流程可以完全离线验证。**在 `local` 端到端跑通之前，不要接真实后端** —— 否则一旦出问题，你无法区分是流程错了还是后端配置错了。

**5. 客户端侧。** 客户端需要内嵌：公钥（含 `keyId`）、几乎不变的 `entryUrls`（指向签名 directory；推荐主 URL 为自有域名 COS，见 `docs/design/update-ingress-cos.md`），或兼容模式下的 `index` URL 列表、自身的 `product` / `channel` / `code`、以及本平台的 selectors。客户端行为按 `SPEC.md` §12 / §16 实现，实现完成后用 `conformance/` 验证 —— 特别是 `version-select/unordered.json`（防"取数组最后一个"）与 `signature/`（防降级与验签绕过）。

**6. 接入真实后端。** 后端能力存疑时（例如 CNB 的 Release 附件是否支持匿名下载），先按 `CLI.md` §6.4 的方法在未认证环境下实测，再决定是否把它作为客户端下载源。凡是「按可预测路径提供 HTTP 下载」的托管（仓库直链、对象存储挂域名、Nginx、CDN），用 `static-http` 即可，不需要专门实现；若托管方是 relkit-serve 或任何 PUT / WebDAV 端点，用 `http-put` 可以让发布一步完成。推荐生产主数据面为自有域名 COS：用 `s3-compatible` 由 CLI 直接写桶；迁移镜像时见 `docs/design/update-ingress-cos.md` §8。

---

## 5. 流程 C：撤回一个坏版本

```bash
relkit yank 1.5.0 --reason "启动崩溃"
```

要点：

- `yank` 只置 `yanked: true`，**禁止**删除节点 —— 其他节点的 `minFrom` 语义与仍运行在该版本上的客户端都依赖它继续存在。
- 若校验报告撤回会切断升级路径，**不要**寻找绕过参数。正确做法是先发一个修复版本，再撤回坏版本。
- 已经装上被撤回版本的客户端**不会被降级**（协议不支持降级）。它们会停在原地直到有更高的可用版本。因此撤回之后要尽快发修复版，并据此告知用户影响范围。
- 撤回后跑 `relkit simulate --from all`，确认各起点的新落点符合预期。

---

## 6. 决策表

### 6.1 `code` 从哪来

| `codeStrategy` | 取值 | 注意 |
|---|---|---|
| `version-build` | `x.y.z+build` 的 build 段 | **新项目缺省**；与 `VERSION.json` 对齐 |
| `explicit` | 必须显式传 `--code` | 遗留；能迁就迁 |
| `semver` | 由 major/minor/patch 编码 | 与版本号一一对应，好对账 |
| `env:VAR` / `env:VAR+N` | CI 构建号 | 天然单调，但有重置风险，见 §2.2 |

### 6.2 `minFrom` 填什么

| 情况 | `minFrom` |
|---|---|
| 普通发布（绝大多数） | `0` |
| 存在必须先经过的迁移（配置格式变更、数据结构升级、更新器自身变更） | 承载该迁移的那个版本的 `code` |
| 不确定 | 问用户，**不要**猜 |

`minFrom` 写在**新版本**上，含义是"要跳到我，你至少得在哪儿"。想强制客户端先经过 1.2.0，就把 1.5.0 的 `minFrom` 设为 1.2.0 的 `code`。

### 6.3 发布到哪些后端

缺省用 `relkit.json` 的 `publishTo`。用户明确指定时用 `--to`。首次验证或离线演练用 `--to local`。

---

## 7. 错误码处置

| 错误码 | 含义 | 处置 |
|---|---|---|
| `unreachable` | 有旧版本会被永久锁死 | 降低新版本的 `minFrom`，或补一个中转版本。**禁止**忽略 |
| `min-supported-above-head` | `minSupported` 高于最新版本 | 改为不高于最新版本的 `code` |
| `min-supported-unreachable` | 从强更下限出发到不了最新版 | 同 `unreachable` |
| `duplicate-code` | `code` 重复 | 检查 `code` 来源，尤其确认 CI 构建号是否被重置 |
| `no-head` | 所有版本都被撤回 | 发一个新版本；不要靠 unyank 掩盖 |
| `sequence-not-increasing` | 远端 index 比预期新 | 先 `relkit verify`；通常是有人已经发过一次，或多个后端不同步 |
| `manifest-digest-mismatch` | index 记录的 manifest `sha256`/`size` 与远端实际的 manifest 字节不符 | 远端状态已损坏（多为 manifest 被覆盖而 index 未同步）。重跑 `publish` 让工具重新生成并填入正确摘要，**禁止**手改 index |
| 警告 `zero-unreachable` | 全新安装（`code=0`）无法起步 | 若是有意放弃极旧版本则可接受，但应同时设置 `minSupported` 让那些客户端至少收到明确提示 |

`publish` 第 1 步还会重算 staged 内每个产物的 `sha256` 并与 `staged.json` 的记录比对。不符说明 stage 之后产物被改动过（常见于重新构建了源目录但没重新 stage），处置是重新 `stage`，**禁止**手改 staged 目录里的哈希。

`publish` 第 9 步会用 `signing.publicKeys` 验一遍刚生成的签名。报「the configured public keys reject it」时，说明 `signing.keyId` 指向的名字不在 `publicKeys` 里 —— 通常是 `relkit init` 预填的占位 `keyId` 一直没改。处置是把 `signing.keyId` 改成 `publicKeys` 里真实存在的那个名字，并确认签名用的是它的私钥。此时指针尚未写入，客户端看到的仍是上一个版本，**不要**去手改已发布的 index。

这条检查值得单独记住，因为它拦下的是唯一一种「发布全程成功、客户端全部静默失效」的事故：客户端按 `SPEC.md` §12.1 对验签失败保持沉默，现场表现是「更新服务器坏了」，排查方向会被完全带偏。

---

## 8. 流程 D：诊断"客户端收不到更新"

按顺序排查，每一步都能独立排除一类原因：

1. **`relkit verify`** —— 远端 index 是否存在、签名是否有效、可达性是否完好、各后端是否一致。
2. **`relkit simulate --from <客户端的 code>`** —— 该客户端**应该**看到什么。若结果为空，问题在版本链条（`minFrom` 或 `code`），不在客户端。
3. **`product` / `channel` 是否匹配** —— 不符时客户端会整体拒绝该源（`SPEC.md` §12.1 步骤 3），症状是"什么都没发生"而非报错。
   同一步顺带确认**签名 `keyId` 是否在客户端内嵌的公钥集合里**。`relkit verify` 用的是 `signing.publicKeys`，若它与客户端实际内嵌的公钥不同，verify 会通过而客户端仍然全部拒绝。核对客户端源码里内嵌的 `keyId` 与公钥，而不只看配置文件。
4. **`sequence` 是否倒退** —— 客户端持久化记录见过的最大 `sequence`，更小的一律拒绝，且按规范**不会**上报为错误。检查是否某个镜像落后于其他后端。
5. **selectors 是否匹配该客户端平台** —— artifact 声明的每个 selector 键都必须在客户端集合里相等。客户端缺少某个键就不匹配，症状是"检查到新版本但没有可下载的产物"。
6. **是否被节流** —— 缺省成功检查后 24 小时内不再请求。手动触发检查可排除。
7. **CDN / 对象存储缓存** —— `directory` / `index` / `fallback` 应为短缓存（或 no-cache）并附缓存击穿参数；`manifest` / `artifact` 可长缓存。刚发布后短时间内看不到，先怀疑可变前缀被缓存过久（COS 自定义域名场景见 `docs/design/update-ingress-cos.md` §5）。
8. **流水线 200 但给人看的站没变** —— 先分清域名：`publish.*` 是 agent；`updates.*` / `raw.*` 是 RUP/COS；`update.*` 往往是 EdgeOne 人页。协议面更新只证明 COS 写成功。人页还要 `site.makers` 进 staged policy、profile 里有 `tokenEnv`、对应 token 已在 agent 进程里（改 env 必须重启）。日志里有 `human index will not be deployed` 就是这条，不是客户端坏了。
9. **走 relkit-agent 时** —— 发布配置是 staged `release-policy.json` + `/etc/relkit-agent/products/<id>.json`。不要改、也不要覆盖 `/srv/relkit/<id>/relkit.json`。init 若用 root：`products/` 目录须 `0755`，否则服务用户读不到 profile。

---

## 9. 禁止行为

- 声称执行了 `relkit` 命令而实际上工具尚未实现（见 §0）。
- 跳过 `simulate`，仅凭"校验通过了"就发布抬高了 `minFrom` 的版本。
- 用任何参数绕过可达性校验。
- 手工编辑远端 `index` 或 `manifest`。
- 为不同镜像生成内容不同的清单。
- 把私钥写进仓库、参数、日志或对话。
- 解析 `version` 字符串来比较版本大小 —— 唯一合法的比较依据是 `code`。
- 遇到 `unreachable` 时改用"删掉挡路的旧节点"来消除报错。删除中转节点正是产生 `unreachable` 的原因之一。

---

## 10. 汇报要求

发布完成后至少说明：

- 版本号、`code`、通道、`minFrom`；
- 实际发布到了哪些后端，各自的 `index` URL；
- 新的 `sequence`；
- `simulate` 的结论，即各起点的升级落点；
- `verify` 是否通过；
- 若中途失败：失败发生在写 `index` 指针之前还是之后（这决定了新版本对客户端是否已可见），以及重跑是否安全。

## Fallback 救急催更

当正常 OTA 链坏掉、需要催已装客户端去手工下载页时：

```
relkit fallback set --max-code 17 --url https://update.example/artifact/myapp/ --message "请前往下载页手动更新" --mandatory
relkit fallback set --clear
```

写入签名文档 `fallback/<product>.pb`（与 index 同一套公钥）。客户端 SDK 配置 `fallbackUrls` 后，在无可用 OTA 时返回 `FallbackRequired`。