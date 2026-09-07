# Roadmap

尚未排期的意向，不等于已接受的 ADR。落地前再写决策。

内容寻址有两个方向，不要混成一条：

- **A（下载）**：已装客户端，本地已有且哈希一致 → 不下载。见下节。
- **B（上传）**：发布侧，stage 之后往 COS/ingest 推产物时，blob 已存在 → 不重复上传。见 [发布侧：未变 blob 不重复上传](#发布侧未变-blob-不重复上传b)。「构建完成后发布：没变的东西不必上传」就是 **B，不是 A**。

## SDK：按 sha256 跳过未变 artifact（A）

- **状态**：SDK 捷径已在；缺口在宿主用法。可单独改宿主，**不依赖**再扩 B。
- **目标**：已装客户端：本地目标已存在且内容与 manifest 一致则不下载。多组件套件不要「版本新就全下」——只对哈希变了的 artifact 走网络。
- **macOS DMG**：不拆。现有 DMG 继续当 Console 产物 / 人页安装器。Mac 上 OTA 若仍是整份 dmg，A 几乎省不了下载——接受「省不了就省不了」。**不要**把「Console 必须拆成可 `ReplaceFile` 的 app 二进制、DMG 只留人页」当成要做的事。
- **与现网（Go SDK 为主，Dart / Node 同形）**：
  - `DownloadArtifact` 在拉网前就会对 **`destPath`** 调 `fileMatches`：一致则直接返回 `VerifiedFile`，不 GET。
  - 校验是 **先 size 后 sha256**：长度不同不算哈希。size 相同才流式读整文件 SHA-256。
  - **缺口**：套件循环常把 Download 指到新临时路径，已装文件对不上 `destPath`，短路打不中。会话内刚校验过的路径再次 `Download` 仍会再扫一遍（无 mtime 缓存）。
- **大文件 sha256**：size 相同才全文件哈希；跳过判断与下载验收共用同一套。mtime/size 只可作 CPU 短路，不是安全证据。失败或 mismatch → 当需要下载。
- **宿主后续（Dec 等）**：套件 Download 把已装路径交给 `destPath`；构建去掉污染哈希的 `BuildTime`，否则 A/B 都当新文件。
- **明确不依赖**：CI `cas/credentials`、拆 DMG。协议不变（整 artifact + size/sha256，无 patch kind）。

## 发布侧：未变 blob 不重复上传（B）

- **状态**：**agent 侧已落地**（`8bbf755`）：`PutArtifactCAS`、Head + Promote、按 live sha256 收 cas。CI `cas/credentials` 仍未做。整包 `PUT /v1/staged` 仍可用，解包后 publish 走 CAS。
- **目标**：同一内容只 PUT 一次。blob 已存在 → 跳过上传，Promote 到本版 `artifact/`。
- **Promote 用 Copy 不是 Move**：COS 没有改 key 的真正 Move（文档里的移动 = Copy + Delete）。Promote 必须 Copy，才能留下 `cas/{sha256}` 供下一版 Head；Move 会拆掉跨版本去重。同桶 Copy 仍占第二份存储，靠下面的 cas 回收压住。
- **现网**：`local` / `s3-compatible` 实现 `Ingest`（Head 比 size，不重算 sha256；Promote = hardlink / CopyObject）和 `Deleter`。`http-put` 仍整文件 `PutArtifact`。
- **cas 回收**：仍被任意 channel 的 index → manifest 点名的 sha256 保留。`relkit-serve` GC 扫 `cas/`。`publish.Run` 在写完 index 后，只删本产品本轮裁掉且其他 channel 也不再引用的 cas（不 List 整棵 `cas/`）。删除失败只打日志。
- **未走**：`POST /v1/cas/credentials`（目标：s3 用 SigV4 预签名 PUT，不接 STS）、CI 直传 ingest、瘦 staged tar、`Materialize` / `artifactTo` 拆分。设计见 [`design/publish-agent.md`](design/publish-agent.md)。
- **未变判定**：sha256（及 size），不是文件名、不是版本号。
- **产品构建**：宿主每次把 `BuildTime` 打进二进制，哈希会变，B 也跳不过。relkit 不替宿主改编译。
- **落点**：`internal/backends` + `publish.Run` + serve GC。客户端契约不变。不改 SPEC 去做 delta/patch。

## 更新流量：整文件内容寻址，不做包内差分 / 跨平台拆库

- **状态**：方向已拍。B 的 agent 路径已落地；A 的宿主用法与产物稳定哈希在产品仓。
- **背景**：曾讨论包内差分、拆 dll/so、Win/Mac 共用原生库。已否定。
- **macOS DMG 不拆（已拍）**。
- **做**：
  1. **A**：SDK 已有 `destPath` 短路；宿主把已装路径对上。见上节。
  2. **B**：agent CAS 已落地；CI credentials 仍待做。见上节。
  3. **产物哈希可稳定（产品侧）**。
- **不做**：文件级 delta / patch；为差分拆 dll/so；强迫拆 DMG；跨 OS 共用原生库；在 `.app` 里打补丁换已签名文件。Windows 换单个 exe 仍须 rename-aside。
- **与现网关系**：Download 对同一 `destPath` 已短路。发布：Ingest 后端 Head+Promote+cas GC；CI 仍可整包 staged-put。

## 公网 browse：访问计数用 51.la

- **状态**：意向（方向已拍，未嵌代码）
- **背景**：公网给人看的目录在 EdgeOne Makers（契约 `sites/updates-index/`，现纯静态）。曾评估用 Edge Function + KV 做点击/下载计数。KV 是最终一致（边缘缓存最长 60s）、没有原子 `INCR`，全国并发会大量丢数；Blob 强一致仍是 `get` 再 `put`，又慢又不准；运行时拿不到边缘节点 ID，按节点分片做不成。自建 Redis/API 对个人目录页不划算。腾讯分析（MTA）已停服；灯塔偏腾讯系 App/游戏，个人静态站开不顺、也不把数字画回页面。
- **目标**：要计数，但只在统计后台看，不把次数画在产品卡片上。browse dump 的 HTML 嵌 51.la JS，PV 与下载点击用自定义事件上报到 51.la。请求打他们的服务，不打自己的 CVM / COS / Makers 函数。
- **不做**：Makers KV / Blob 热路径 `+1`；为计数开 `edge-functions/`；腾讯分析 / 灯塔；自建 Redis 或计数 API；把实时次数写进 `catalog.json`。
- **边缘函数**：继续留给无状态改写（鉴权、跳转、geo、功能开关）。KV 只当偶尔更新的配置盘，不当账本。
- **落点（规划）**：`internal/browse` 生成的 HTML 嵌入 51.la；契约 README 写明。内网数据面同一份 dump 会带上脚本；精确下载次数仍以 serve 面板已有计数为准，不在此重复造账本。
