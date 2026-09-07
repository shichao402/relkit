# Roadmap

尚未排期的意向，不等于已接受的 ADR。落地前再写决策。

内容寻址有两个方向，不要混成一条：

- **A（下载）**：已装客户端，本地已有且哈希一致 → 不下载。见下节。
- **B（上传）**：发布侧，stage 之后往 COS/ingest 推产物时，blob 已存在 → 不重复上传。见 [发布侧：未变 blob 不重复上传](#发布侧未变-blob-不重复上传b)。「构建完成后发布：没变的东西不必上传」就是 **B，不是 A**。

## SDK：按 sha256 跳过未变 artifact（A）

- **状态**：意向（可单独落地，未排期）。**不依赖 B**（CAS / Promote）。也不强迫宿主改打包。
- **目标**：已装客户端：本地目标已存在且内容与 manifest 一致则不下载。多组件套件不要「版本新就全下」——只对哈希变了的 artifact 走网络。
- **macOS DMG**：不拆。现有 DMG 继续当 Console 产物 / 人页安装器。Mac 上 OTA 若仍是整份 dmg，A 几乎省不了下载——接受「省不了就省不了」。**不要**把「Console 必须拆成可 `ReplaceFile` 的 app 二进制、DMG 只留人页」当成要做的事。一句带过：A 对「单 artifact 就是整个 dmg」几乎无收益，这是宿主打包选择，relkit 不强迫改。
- **与现网（Go SDK 为主，Dart / Node 同形）**：
  - `DownloadArtifact` 在拉网前就会对 **`destPath`** 调 `fileMatches`：一致则直接返回 `VerifiedFile`，不 GET。
  - 校验是 **先 size 后 sha256**（`fileMatches` / Dart `_verify` / Node `verifyFile`）：`Stat`/`length` 与 manifest `size` 不同则立刻判定不匹配，**不算哈希**。size 相同才流式读整文件算 SHA-256。
  - 同一套函数也用于下完后的 `.part` 验收。镜像串行回退；支持 Range（`.part` + `.part.meta` 按 url / size / sha256 续传，未完成 part **不**为续传而全文件哈希）。设计文档用语是「先 size 后 sha256」。
  - **缺口不在「dest 碰巧已是目标文件」这一条捷径本身**，而在宿主用法：套件循环常把 Download 指到新临时路径、或「有新版本就对每个组件 Download」，已装文件对不上 `destPath`，现网短路打不中。会话内刚校验过的路径再次 `Download` 仍会再扫一遍（无 mtime 缓存）。
- **大文件 sha256 成本（必读）**：
  - 全文件 SHA-256 对几百 MiB 安装器/二进制是 CPU + 磁盘带宽。跳过前「每次都扫一遍」可能比直接下还亏——尤其本地已是新文件却重复扫、机械盘、或杀毒实时扫描跟着读盘时。
  - **权衡（按现网写，不发明）**：真正跳过时省的是 **网络 + 下完后对 `.part` 的那一次哈希**。但「决定能否跳过」在 **size 已相同** 时，现网仍会做 **一次本地全文件哈希**。size 不同则零哈希、直接下。同 size 且其实是旧内容时：先扫盘发现 mismatch，再下载再哈希，比「不问直接下」多亏一次本地全文件读。
  - **推荐策略（复用现网校验，禁止两套哈希）**：
    - **先比 size**（manifest 已有 `size`）：长度不同直接下，不算哈希。
    - **跳过判断与下载路径验收共用同一套**（现网已是先 size 后 sha256）。禁止「跳过时哈希一遍、下完再换一套再哈希一遍」。
    - **信任刚校验过的结果**：本次会话刚 `Download` 并校验通过的文件不要立刻再 hash。
    - **可选短路**：同 size 才 hash；或记录上次已验证的 `(path, size, mtime, sha256)`，元数据未变则跳过重算。**mtime/size 不是安全证据**，只避免重复 CPU；最终仍以 sha256 为准才能跳过下载。
    - **不要**为了省哈希去拆 dll/so、拆 DMG、或做 delta patch（见大节「不做」）。
  - **失败 / 不一致**：哈希读失败或 sha256 mismatch → **当需要下载**，不要静默当成已最新。
- **落点（规划）**：SDK `Download`（Go；Dart / Node 对齐），以及 Dec 等宿主的套件循环把「已装路径」交给同一套判断。协议不变（仍整 artifact + size/sha256，无 patch kind）。
- **明确不依赖**：B（`cas/{sha256}` / Promote，设计有、现网未走）、不强迫改 DMG/Console 打包。本条只保证客户端对未变字节少拉网。

## 发布侧：未变 blob 不重复上传（B）

- **状态**：agent 侧 Head + Promote 已落地（`backends.PutArtifactCAS`）；CI `cas/credentials` 仍未做。整包 `PUT /v1/staged` 仍可用，解包后 publish 走 CAS。
- **目标**：stage 之后往 COS / ingest 推产物时，**同一内容只 PUT 一次**。blob 已存在 → 跳过上传，只 Promote / 让 version 路径指向已有字节。
- **和 A 的关系**：两者都靠内容寻址，**方向相反**——A 是客户端少下，B 是发布侧少上。不要写成一条工作项。A **不依赖** B 先落地。
- **现网**：`local` 与 `s3-compatible` 实现 `Ingest`（Head 比 size，不重算 sha256；Promote = hardlink / CopyObject）。`http-put` / 只读 static-http 仍整文件 `PutArtifact`。`cas/{sha256}` 作为跨版本去重对象留下。
- **cas 回收**：仍被任意 channel 的 index → manifest 点名的 sha256 保留。`relkit-serve` GC 扫 `cas/`，删未引用对象。`publish.Run` 在写完 index 后，只删**本产品本轮裁掉且其他 channel 也不再引用**的 cas（不 List 整棵 `cas/`，以免同桶其他产品被误伤）。index 已提交，删除失败只打日志。
- **未走**：`POST /v1/cas/credentials`、CI 直传 ingest、`Materialize` / `artifactTo` 拆分。设计见 [`design/publish-agent.md`](design/publish-agent.md)。
- **未变判定**：必须是 **sha256（及 size）**，不是文件名、不是版本号。同名新版本若字节变了要传；文件名改了但哈希相同则不必再传 blob。
- **产品构建**：宿主若每次把 `BuildTime` 等打进二进制，哈希会变，**B 也跳不过**——那是产品构建问题，relkit 不替宿主改编译。
- **落点**：`internal/backends` 的 `Ingest` + `PutArtifactCAS`；`publish.Run` 已接。CI credentials / Materialize 仍待做。客户端契约不变。不改 SPEC 去做 delta/patch。

## 更新流量：整文件内容寻址，不做包内差分 / 跨平台拆库

- **状态**：意向（方向已拍，未排期；与 SPEC §1.2「不做文件级增量」一致，不推翻）
- **背景**：曾讨论为省流量做包内差分、拆 dll/so、甚至 Win/Mac 共用原生库。方向已否定那些。真正要落地的是 **A（少下）** 和 **B（少上）**，都在 artifact 粒度。
- **macOS DMG 不拆（已拍）**：不破坏现有 DMG 作为 Console 产物 / 人页安装器。不把「Console 拆成可 ReplaceFile 的 app、DMG 只留人页」列入要做。OTA 仍是整份 dmg 时，A 几乎无下载收益，接受即可。
- **做**：
  1. **A**：见 [SDK：按 sha256 跳过未变 artifact（A）](#sdk按-sha256-跳过未变-artifacta)。
  2. **B**：见 [发布侧：未变 blob 不重复上传（B）](#发布侧未变-blob-不重复上传b)。用户说的「没变不必上传」走这里。
  3. **产物哈希可稳定（产品侧）**。未改源码却每次注入 `BuildTime`，A/B 都会空转。relkit 可文档约定，不替宿主改编译。
  4. **版本号与下载集合解耦（可选，属 A 的套件用法）。** 同一次 SemVer 可含多 artifact；只改某一件时不必强制全下。连接门闩仍可比版本，下载集合以哈希为准。
- **不做**：
  - **文件级 delta / patch**（bsdiff、Courgette、块差分）：SPEC §1.2 非目标。协议继续只整 artifact；`artifacts[].kind` 预留位不提前实现。
  - **为差分或为省哈希而拆 dll/so/dylib。** 静态单文件保持整文件替换。
  - **强迫拆 macOS DMG / 把 Console 改成必须 ReplaceFile 的裸二进制。**
  - **Windows / macOS / Linux 共用同一份原生库。** 跨平台只复用数据（JS/CSS、清单、协议），不复用 so/dll。
  - **在 `.app` 里打补丁换单个已签名文件当默认路径。** macOS 改 bundle 内文件会弄坏签名；若宿主仍发整份 `.app`/dmg，就整包替换。Windows 可换单个 exe，仍须 rename-aside（现有 `sdk/apply`）。
- **与现网关系**：RUP 已是 per-artifact。Download 对同一 `destPath` 已有先 size 后 sha256 短路（A 的缺口在宿主循环）。发布：Ingest 后端已 Head+Promote；CI 仍可整包 PUT `/v1/staged`；`cas/credentials` 未走。
- **落点（规划）**：A、B 各见独立节；SPEC / AGENT 补「禁止 delta」对照即可。不新增 ADR，除非要改协议允许 patch kind。

## 公网 browse：访问计数用 51.la

- **状态**：意向（方向已拍，未嵌代码）
- **背景**：公网给人看的目录在 EdgeOne Makers（契约 `sites/updates-index/`，现纯静态）。曾评估用 Edge Function + KV 做点击/下载计数。KV 是最终一致（边缘缓存最长 60s）、没有原子 `INCR`，全国并发会大量丢数；Blob 强一致仍是 `get` 再 `put`，又慢又不准；运行时拿不到边缘节点 ID，按节点分片做不成。自建 Redis/API 对个人目录页不划算。腾讯分析（MTA）已停服；灯塔偏腾讯系 App/游戏，个人静态站开不顺、也不把数字画回页面。
- **目标**：要计数，但只在统计后台看，不把次数画在产品卡片上。browse dump 的 HTML 嵌 51.la JS，PV 与下载点击用自定义事件上报到 51.la。请求打他们的服务，不打自己的 CVM / COS / Makers 函数。
- **不做**：Makers KV / Blob 热路径 `+1`；为计数开 `edge-functions/`；腾讯分析 / 灯塔；自建 Redis 或计数 API；把实时次数写进 `catalog.json`。
- **边缘函数**：继续留给无状态改写（鉴权、跳转、geo、功能开关）。KV 只当偶尔更新的配置盘，不当账本。
- **落点（规划）**：`internal/browse` 生成的 HTML 嵌入 51.la；契约 README 写明。内网数据面同一份 dump 会带上脚本；精确下载次数仍以 serve 面板已有计数为准，不在此重复造账本。

## 自更新：整文件内容寻址，而不是安装器差分或跨系统拆库

- **状态**：意向（与 Dec Console 讨论后记下方向；未改 SPEC / SDK）。细节以 **A / B** 两节为准，本节不另开一套。
- **背景**：曾误以为自更新必须下整份安装器差分、或拆 dll/so、Win/Mac 共用原生库。[SPEC](../SPEC.md) v1 只做整包/整文件替换。发版若把 `BuildTime` 打进二进制，哈希会变，A/B 都跳不过。
- **已拍**：**macOS DMG 不拆**；不把「Console 拆成 ReplaceFile、DMG 只留人页」当待办。OTA 仍是整份 dmg 时 A 几乎无下载收益，接受。「没变不必上传」走 **B**。
- **指向**：A 见 [SDK：按 sha256 跳过未变 artifact（A）](#sdk按-sha256-跳过未变-artifacta)；B 见 [发布侧：未变 blob 不重复上传（B）](#发布侧未变-blob-不重复上传b)。不做 delta / 不拆 dll / 不跨 OS 共用原生库。
