# RUP — Release & Update Protocol

> 版本：`2`（draft）
> 状态：设计稿；**v2 线格式为 Protobuf 二进制**
> 协议标识前缀：`rup.`
> 结构 SSOT：[`proto/`](proto/)（见 ADR 0003）

本文定义**发布侧**与**客户端运行时**之间的唯一契约。协议与实现语言无关：发布侧由 [`relkit`](https://github.com/shichao402/relkit) 实现（见 `CLI.md`），客户端运行时由各语言 **SDK**（生成类型 + 手写编排逻辑）实现。双方只通过本文与 `.proto` 定义的 **protobuf 文档**通信。

**v1 JSON 线格式已废弃。** 旧的 `schema/*.json` 仅作历史对照。

关键词 **必须（MUST）**、**禁止（MUST NOT）**、**应该（SHOULD）**、**可以（MAY）** 按 RFC 2119 解释。凡标注「规范性」的小节，实现必须逐字遵守，否则会与其他实现产生行为分歧。

---

## 1. 设计目标与非目标

### 1.1 目标

1. **发布侧与托管后端解耦。** 客户端**禁止**自行拼接任何下载 URL，所有 URL 必须从签名文档中读取。这样托管后端（GitHub Release / Gitee / CNB / COS / 自建静态服务器）可以随时更换、并存、迁移，客户端代码不变。
2. **升级路径可控。** 发布方能声明「从某个旧版本不允许直接跳到最新，必须先经过某个中间版本」，客户端据此自动完成多跳升级。
3. **跨语言零分歧。** 版本比较、选路、验签这三处逻辑必须在 Go / Dart / Node / C# 等实现中给出完全一致的结果，由 `conformance/` 用例强制约束。
4. **纯本地阶段可离线、可重复、可单测。** 计算哈希、固化版本信息不得依赖网络或后端。
5. **完整性可验证。** 单一签名根 + 逐级哈希，能防篡改与防降级。
6. **稳定引导入口与可迁移更新服务。** 客户端**可以**只内嵌极少变更的 directory 入口 URL 列表（`entryUrls`），从签名的 directory 文档发现当前 index / fallback 取货点；加减机房与更换 serve **应该**优先改 directory 与发布双写，而不是改客户端常量。directory 与 index / fallback **必须**使用同一组发布公钥所对应的签名体系（见 §4、§16）。

### 1.2 非目标

1. **不做文件级增量（delta / patch）。** 本版本只支持整包替换。协议为将来扩展预留了 `artifacts[].kind` 与 `schema` 版本位，但 v1 实现**禁止**产出增量产物。
2. **不定义"如何安装"。** 替换二进制、唤起系统安装器、解压覆盖资源目录，这些完全由宿主决定，协议只负责把正确的字节和元数据交到宿主手里。
3. **不做服务端灰度分流。** v1 的 index 是一份对所有客户端相同的静态文档。`rollout` 字段已预留，但 v1 **禁止**依赖它。
4. **~~不提供多语言运行时 SDK~~（已撤销）。** v2 起提供官方 Go / Dart SDK；结构以 Protobuf 生成代码为唯一读写入口，各语言只实现编排逻辑。
5. **不依赖付费智能 DNS / Anycast / 全球 CDN 做就近调度。** 多区域可达性由 directory 列表 + 客户端对真实下载历史的排序解决（§12.7）；协议**禁止**把「每次检查前的专用连通性探测 / 测速下载」规定为选源前提。

---

## 2. 概念模型（规范性）

网络上存在五类对象，外加一类**只存在于本地**的中间产物。

| 对象 | 可变性 | 数量 | 作用 |
|---|---|---|---|
| **directory** | 可变，服务拓扑变更时覆盖 | 每个 product 一份 | 引导文档：给出该 product 可用的 index / fallback 绝对 URL 列表。**必须签名。** 不承载版本链，不替代 index。 |
| **index** | 可变，每次发布覆盖 | 每个 (product, channel) 一份 | 正常 OTA 入口。包含版本链条、升级路径约束、每份 manifest 的哈希与 URL。**必须签名。** |
| **manifest** | 不可变 | 每个版本一份 | 描述该版本包含哪些产物、每个产物的哈希、大小、URL、选择器。 |
| **artifact** | 不可变 | 每个版本 N 份 | 产物本体。 |
| **fallback** | 可变，救急时覆盖 | 每个 product 一份 | 当正常 OTA 链失效或某批版本被点名时，下发催促手工更新的规则与链接。**必须签名。** 不承载下载/安装。 |
| **staged**（本地） | — | 每个待发布版本一份 | 「已固化但尚未上网」的版本描述。**禁止**包含任何 URL 或后端信息。 |

**信任链：** 客户端内嵌公钥 →（可选）验证 directory 的签名并读取其中的 index URL → 验证 index 的签名 → 用 index 中的 `sha256` 校验 manifest → 用 manifest 中的 `sha256` 校验 artifact。因此 **manifest 与 artifact 禁止单独签名**，它们的完整性由 index 的签名传递而来。directory **禁止**携带 artifact 下载 URL；它只回答「去哪里取签名的 index / fallback」。

**Directory 信任链：** 同一套内嵌公钥 → 验证 directory 的签名信封 → 采用其中的服务列表。directory 使用的 `key_id` / 算法规则与 index、fallback **必须**一致（同一信任根）。

**Fallback 信任链：** 同一套内嵌公钥 → 验证 fallback 的签名信封 → 采用其中的规则与 `manualUrl`。Fallback **禁止**代替 index 完成自动下载；它只提供用户可打开的手工更新链接。

**发布的提交点（commit point）：** 一次**版本**发布中，写入 index 是**最后一步**。在 index 被更新之前，即使 artifact 和 manifest 已全部上传完毕，该版本对客户端也**必须**不可见。这消除了「清单指向尚不可下载的 URL」这一类不一致窗口。Fallback 的提交点是写入 `fallback/<product>.pb` 本身（独立于 index）。Directory 的提交点是写入 `directory/<product>.pb`（独立于单次版本发布；通常在服务拓扑变更时更新）。若 directory 与多份 index 镜像一并变更，发布方**必须**先保证各镜像上的 index / 产物字节已一致可见，再更新 directory。

---

## 3. 文件布局与后端映射（规范性）

发布工具在逻辑上操作五个 **key**：

```
directory/<product>.pb
index/<product>/<channel>.pb
manifest/<product>/<version>.pb
artifact/<product>/<version>/<filename>
fallback/<product>.pb
```

文档对象（directory / index / manifest / fallback / envelope）在网络上以 **protobuf 二进制**传输，`Content-Type` 宜为 `application/protobuf`（或 `application/x-protobuf`）。artifact 仍是原始文件字节。

key 是逻辑标识，**不是** URL。后端插件负责把 key 映射到物理位置并返回可访问的 URL。

**路径型后端**（本地目录 / COS / S3 / 自建静态服务器）直接把 key 当作路径：

```
https://cdn.example.com/directory/myapp.pb
https://cdn.example.com/index/myapp/stable.pb
https://cdn.example.com/manifest/myapp/1.5.0.pb
https://cdn.example.com/artifact/myapp/1.5.0/myapp-1.5.0-win-x64.zip
https://cdn.example.com/fallback/myapp.pb
```

**Release 型后端**（GitHub / Gitee / CNB）没有路径概念，只有 `(tag, assetName)` 二元组，因此 key 必须扁平化。推荐映射：

| key | tag | asset name |
|---|---|---|
| `directory/myapp.pb` | `directory`（固定 tag，反复覆盖） | `directory-myapp.pb` |
| `index/myapp/stable.pb` | `channel-stable`（固定 tag，反复覆盖） | `index-stable.pb` |
| `manifest/myapp/1.5.0.pb` | `v1.5.0` | `manifest-1.5.0.pb` |
| `artifact/myapp/1.5.0/app.zip` | `v1.5.0` | `app.zip` |
| `fallback/myapp.pb` | `fallback`（固定 tag，反复覆盖） | `fallback-myapp.pb` |

asset 名**禁止**包含 `/`。同一 tag 下 asset 名**必须**唯一。

> 现有实践对照：Dec 用 `ReleaseLatest` 分支上的 `version.json` 充当 index，RemoteCam 用固定 tag `UpdateConfig` / `config` 的 asset 充当 index。两者都是本节「固定位置的可变指针」的特例。

### 3.1 缓存策略（规范性）

- directory、index 与 fallback 是可变的，后端**应该**为其设置短缓存（≤ 60 秒）或禁用缓存。客户端在请求它们时**应该**追加一个缓存击穿查询参数（如 `?t=<unix秒>`）或发送 `Cache-Control: no-cache`。
- manifest 与 artifact 是不可变的，**应该**设置长缓存（≥ 1 年）。客户端**禁止**对它们做缓存击穿。

> 这一条是必须写进协议的，因为 `raw.githubusercontent.com` 与 jsdelivr 都有分钟级 CDN 缓存，不处理会导致「已发布但客户端几分钟内看不到」。

### 3.2 URL 要求（规范性）

- 文档中出现的所有 URL **必须**是绝对 URL，**禁止**相对路径。客户端**禁止**对其做任何拼接、拼装或补全。
- URL **应该**使用 `https`。签名与逐级哈希已能保证「装上的字节是发布方签发的」，因此 `http` 不至于导致安装被篡改的产物；但 `http` 会暴露用户正在使用的产品与版本，并便于中间人做流量分析或选择性阻断。
- 客户端**必须**支持跟随 HTTP 重定向，建议上限 5 跳。这不是可选项：GitHub Release 的下载地址会 302 跳转到 `objects.githubusercontent.com`，不支持重定向的实现会直接下载失败。
- 客户端**禁止**跟随把协议从 `https` 降级为 `http` 的重定向。
- 客户端**禁止**因为最终落地 URL 的域名与文档中的域名不同而拒绝下载 —— 上一条的 302 跳转必然导致域名变化。产物的可信性由 `sha256` 判定，而不是由域名判定。

---

## 4. 签名信封 envelope（规范性）

directory、index 与 fallback 在网络上传输时**必须**包裹在 protobuf `Envelope`（`rup.envelope/2`）中。manifest 与 artifact **禁止**使用信封。字段定义见 [`proto/rup/v2/envelope.proto`](proto/rup/v2/envelope.proto)。

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `schema` | string | 是 | 固定 `"rup.envelope/2"` |
| `payload` | bytes | 是 | **Directory、Index 或 Fallback 消息的 protobuf 序列化字节**（由拉取的逻辑 key 决定解析成哪一种） |
| `signatures` | repeated | 是 | 至少一个元素 |
| `signatures[].key_id` | string | 是 | 公钥标识 |
| `signatures[].alg` | string | 是 | 仅允许 `"ed25519"` |
| `signatures[].sig` | bytes | 是 | 对 **payload 原始字节**的 64 字节 Ed25519 签名 |

**为什么签 protobuf 字节：** 签名对象是一段确定的字节序列。各语言**必须**通过生成代码 `Marshal`/`Unmarshal`；`selectors` / `meta` 使用 `repeated` 键值对并在编码前按 key 排序，避免 map 遍历顺序导致同逻辑不同字节。

### 4.1 验签规则（规范性）

客户端**必须**按以下顺序处理：

1. 用生成代码解析 `Envelope`。若 `schema` 不是 `rup.envelope/2`，拒绝。
2. 取 `payload` 字节串 `B`（已是原始 bytes，无需 Base64）。
3. 遍历 `signatures`，找出 `key_id` 在客户端内嵌公钥集合中、且 `alg` 受支持的条目。
4. 对每个这样的条目，用对应公钥验证 `sig` 是否为 `B` 的有效签名。**至少一个**验证通过则整体通过；否则拒绝，且**禁止**回退到「不验签直接使用」。
5. 只有验签通过后，才**可以**把 `B` 解析为期望的消息类型（Index 或 Fallback）。**禁止**在验签前解析或信任 payload 中的任何内容。

`signatures` 允许多个条目，用途是**密钥轮换**。

---

## 5. index 对象（规范性）

```json
{
  "schema": "rup.index/1",
  "product": "myapp",
  "channel": "stable",
  "sequence": 42,
  "generatedAt": "2026-07-30T03:00:00Z",
  "minSupported": 100,
  "versions": [
    {
      "version": "1.0.0",
      "code": 100,
      "minFrom": 0,
      "releasedAt": "2026-01-10T00:00:00Z",
      "manifest": {
        "sha256": "a1b2...",
        "size": 1834,
        "urls": [
          "https://cdn.example.com/manifest/myapp/1.0.0.json",
          "https://mirror.example.cn/manifest/myapp/1.0.0.json"
        ]
      }
    },
    {
      "version": "1.2.0",
      "code": 120,
      "minFrom": 100,
      "releasedAt": "2026-04-02T00:00:00Z",
      "notes": "配置文件格式变更，更早的版本必须经由本版本升级。",
      "manifest": { "sha256": "c3d4...", "size": 1902, "urls": ["..."] }
    },
    {
      "version": "1.5.0",
      "code": 150,
      "minFrom": 120,
      "releasedAt": "2026-07-30T00:00:00Z",
      "manifest": { "sha256": "e5f6...", "size": 2011, "urls": ["..."] }
    }
  ]
}
```

### 5.1 顶层字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `schema` | string | 是 | 固定 `"rup.index/2"` |
| `product` | string | 是 | 产品标识。客户端**必须**校验它与自身期望一致，不一致则拒绝（防止把 A 产品的更新装到 B 产品上） |
| `channel` | string | 是 | 通道名，如 `stable` / `beta`。客户端**必须**校验与当前所处通道一致 |
| `sequence` | integer ≥ 1 | 是 | 该 channel 的发布序号，每次发布**必须**严格递增。用于防降级重放，见 §12.4 |
| `generatedAt` | string | 是 | RFC 3339 UTC 时间戳。仅供展示与排障，**禁止**用于任何判定逻辑 |
| `minSupported` | integer | 否 | 最低受支持的 `code`。客户端 `code` 小于此值时**必须**视为强制更新，见 §9.3 |
| `versions` | array | 是 | 版本节点数组，长度 ≥ 1。**应该**按 `code` 升序排列，但客户端**禁止**依赖该顺序 |
| `expiresAt` | string | 否 | 见 §14.3。v1 客户端**可以**忽略 |

### 5.2 版本节点字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `version` | string | 是 | 面向人的版本字符串，任意格式（`1.5.0`、`1.5.0+13`、`2026.07-rc1` 都合法）。**禁止**用于任何比较 |
| `code` | integer ≥ 0 | 是 | 机器用的单调版本号。同一 (product, channel) 内**必须**唯一 |
| `minFrom` | integer ≥ 0 | 否，缺省 `0` | 要**直接**升级到本节点，客户端当前 `code` 必须 ≥ 此值。见 §9 |
| `releasedAt` | string | 否 | RFC 3339 UTC 时间戳，仅供展示 |
| `yanked` | boolean | 否，缺省 `false` | 撤回标记。为 `true` 时该节点**禁止**作为升级目标，见 §9.4 |
| `notes` | string | 否 | 更新说明，**推荐 Markdown**。最新版本应内联全文；更早版本发布侧可清空并由 `notesUrl` 指向仓库 changelog |
| `notesUrl` | string | 否 | 更新说明的外部 http(s) 地址（通常指向仓库内 changelog 路径）。更早版本优先用链接而非内联全文 |
| `manifest` | object | 是 | 指向该版本的 manifest |
| `manifest.sha256` | string | 是 | 64 位小写十六进制 |
| `manifest.size` | integer ≥ 0 | 是 | 字节数 |
| `manifest.urls` | array\<string\> | 是 | 长度 ≥ 1。同一份内容的多个镜像地址 |
| `rollout` | object | 否 | 预留。v1 客户端**禁止**依赖 |

### 5.3 镜像的正确表达方式（规范性）

多镜像**必须**通过 `urls` 数组表达，**禁止**为不同镜像生成内容不同的多份 index 或 manifest。

理由：`manifest.sha256` 与 artifact 的 `sha256` 使得从任何一个镜像下载到的内容都可被验证为同一份字节，因此镜像只是「同一对象的多个取货点」。反之，若为每个镜像生成一份 URL 不同的清单，就会导致：签名要签多次、内容要维护多份、镜像之间可能不一致、还需要一个专门的转换脚本。

> 现有实践对照：RemoteCam 目前为 GitHub 和 Gitee 各生成一份内容不同的 `update_config_*.json`，并靠 `convert_config_to_gitee.py` 做 URL 替换，正是本节禁止的做法。改为 `urls` 数组后，全网只有一份字节完全相同的 index 和 manifest，签名只需一次。

由此得出发布顺序的规范性约束：一次发布中，**所有**镜像的 artifact 必须先全部上传完毕、所有 URL 收集齐，才能生成 manifest；所有镜像的 manifest 上传完毕后，才能生成并写入 index。

directory（§16）中列出的多个 `index_url` **必须**指向上述**同一份**已签名 index 字节的取货点（可位于不同区域 / 主机）。**禁止**为「中国机房」「欧洲机房」等分别签发内容不同（例如 `urls` 子集不同）的 index。区域差异**只**通过同一文档内的 `urls[]` 表达。

---

## 6. manifest 对象（规范性）

```json
{
  "schema": "rup.manifest/1",
  "product": "myapp",
  "version": "1.5.0",
  "code": 150,
  "releasedAt": "2026-07-30T00:00:00Z",
  "artifacts": [
    {
      "id": "app-windows-x64",
      "filename": "myapp-1.5.0-win-x64.zip",
      "size": 52428800,
      "sha256": "9f8e...",
      "kind": "archive",
      "selectors": { "os": "windows", "arch": "x64" },
      "urls": [
        "https://cdn.example.com/artifact/myapp/1.5.0/myapp-1.5.0-win-x64.zip",
        "https://mirror.example.cn/artifact/myapp/1.5.0/myapp-1.5.0-win-x64.zip"
      ],
      "meta": { "entryPoint": "myapp.exe" }
    },
    {
      "id": "app-macos-arm64",
      "filename": "myapp-1.5.0-mac-arm64.zip",
      "size": 61234567,
      "sha256": "1c2d...",
      "kind": "archive",
      "selectors": { "os": "macos", "arch": "arm64" },
      "urls": ["..."],
      "meta": { "entryPoint": "MyApp.app" }
    }
  ]
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `schema` | string | 是 | 固定 `"rup.manifest/2"` |
| `product` | string | 是 | **必须**与 index 的 `product` 一致，客户端**必须**校验 |
| `version` | string | 是 | **必须**与对应 index 节点的 `version` 一致，客户端**必须**校验 |
| `code` | integer | 是 | **必须**与对应 index 节点的 `code` 一致，客户端**必须**校验 |
| `releasedAt` | string | 否 | 仅供展示 |
| `artifacts` | array | 是 | 长度 ≥ 1 |
| `notes` | string | 否 | 仅供展示；推荐 Markdown |

### 6.1 artifact 字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `id` | string | 是 | 同一 manifest 内唯一。稳定标识，供宿主按 id 直接取件 |
| `filename` | string | 是 | 建议的落盘文件名。**禁止**包含路径分隔符、`..`、控制字符。客户端**必须**在落盘前校验，见 §14.4 |
| `size` | integer ≥ 0 | 是 | 字节数 |
| `sha256` | string | 是 | 64 位小写十六进制 |
| `kind` | string | 是 | 见 §6.2 |
| `selectors` | object | 是 | 字符串到字符串的映射，见 §11。允许为空对象 `{}`，表示适用于所有客户端 |
| `urls` | array\<string\> | 是 | 长度 ≥ 1 |
| `meta` | object | 否 | 宿主自定义的透传数据。协议**禁止**解释其内容 |

### 6.2 `kind` 取值

| 值 | 含义 |
|---|---|
| `archive` | 压缩包，宿主需解压 |
| `installer` | 安装包（dmg / exe / msi / apk / pkg），宿主需交给系统安装器 |
| `binary` | 单个可执行文件，宿主直接替换 |
| `blob` | 无语义的数据文件（配置、资源） |

`kind` 只是给宿主的提示，协议本身对不同 `kind` 的处理没有差异。宿主遇到无法识别的 `kind` **应该**忽略该 artifact 而不是报错，以便协议将来扩展。

### 6.3 `meta` 的定位

`meta` 是宿主专属的逃生舱。凡是「只有这个项目关心」的信息都放这里，例如 RemoteCam 需要知道「zip 解开后哪个文件是安装包」，就写 `{"installerEntry": "HelloKnightRCC_macos.dmg"}`。协议**禁止**为这类需求增加顶层字段。

---

## 7. staged 对象（规范性，仅存在于本地）

这是发布流程第一阶段的产出，是「已固化但尚未上网」的版本描述。

```json
{
  "schema": "rup.staged/1",
  "product": "myapp",
  "version": "1.5.0",
  "code": 150,
  "minFrom": 120,
  "channel": "stable",
  "createdAt": "2026-07-30T02:11:00Z",
  "notes": "修复若干问题",
  "artifacts": [
    {
      "id": "app-windows-x64",
      "filename": "myapp-1.5.0-win-x64.zip",
      "size": 52428800,
      "sha256": "9f8e...",
      "kind": "archive",
      "selectors": { "os": "windows", "arch": "x64" },
      "meta": { "entryPoint": "myapp.exe" },
      "sourcePath": "dist/win/myapp.zip"
    }
  ]
}
```

**定义性特征：staged 对象禁止包含 `urls`、后端名称、tag、bucket、域名或任何其他与托管位置有关的信息。** 这个约束不是风格偏好，它保证了：

- 第一阶段可完全离线执行，可重复，可单元测试；
- 同一份 staged 对象可以被发布到任意多个后端，而不需要重新计算哈希或重新打包；
- 「产物到底上网了没有」这个问题有了明确答案 —— staged 阶段一定没有。

`sourcePath` 是唯一的本地专有字段，仅供人工排查，发布时**必须**丢弃，**禁止**出现在 manifest 中。

---

## 8. 版本标识与比较（规范性）

**版本比较必须且只能基于 `code` 的整数比较。** 实现**禁止**解析 `version` 字符串来比较大小。

理由不是理论洁癖，而是已经发生过的事实：Dec 手写的比较器丢弃了 `-beta` 后缀，RemoteCam 手写的比较器丢弃了 `+build` 号，两个项目各自引入了一种版本比较缺陷。若让 Go / Dart / Node / C# 各自实现一遍 semver 解析（含预发布标识、构建元数据的排序规则），出现分歧几乎是必然的。整数比较不会。

`code` 的分配由发布方自行决定，只需满足**在同一 (product, channel) 内严格单调递增**。常见做法：CI 构建号、`major*10000 + minor*100 + patch`、发布日期数字。

### 8.1 客户端自身 code 的获取

宿主**必须**能在运行时得到自身的 `code`。推荐编译期注入（Dec 的 ldflags、Flutter 的 `+build`、Node 的 `package.json`）。

- 开发/未打包环境下若无法确定 `code`，宿主**应该**将其视为一个高于所有已发布版本的值（例如 `2^31-1`），从而不会触发更新。Dec 目前把 `dev` 视为「始终需要更新」，这在开发时会造成用开发版覆盖开发版，**不推荐**。
- 宿主**禁止**把无法确定的 `code` 当作 `0`，否则一次误判就会把开发环境降级成正式版。

---

## 9. 升级路径算法（规范性）

这是本协议最核心的规范性内容，也是与现有两个项目差异最大的地方 —— 它们都只有「latest 单点」，没有链条。

### 9.1 `minFrom` 的语义

节点 `V` 的 `minFrom` 表示：**客户端要直接升级到 `V`，其当前 `code` 必须 ≥ `V.minFrom`。**

- `minFrom = 0`（或缺省）：任何版本都能直接跳到 `V`。
- `minFrom = 120`：只有 `code ≥ 120` 的客户端能直接跳到 `V`；更旧的客户端必须先升到某个 `code ≥ 120` 的中间版本。

发布方只需填这一个整数，多跳链式升级由客户端算法自动完成。

### 9.2 `selectNextTarget`（规范性）

```
输入：index（已验签并解析）、currentCode（整数）
输出：一个版本节点，或 null

1. candidates = index.versions 中同时满足以下三条的节点：
     a. yanked 不为 true
     b. code > currentCode
     c. (minFrom 缺省视为 0) <= currentCode
2. 若 candidates 为空，返回 null
3. 返回 candidates 中 code 最大的节点
```

第 3 步**必须**按 `code` 取最大值，**禁止**取「数组中最后一个」。index 虽然应该按 code 升序排列，但客户端依赖数组顺序会在遇到乱序 index 时给出错误结果，而这类错误极难发现。`conformance/version-select/unordered.json` 专门覆盖此场景。

### 9.3 强制更新

若 `index.minSupported` 存在且 `currentCode < index.minSupported`，客户端**必须**将本次更新视为强制：不允许用户跳过或推迟。

`minSupported` 与 `minFrom` 是两个正交的概念：`minSupported` 回答「当前版本还能不能继续用」，`minFrom` 回答「下一跳该去哪」。一个客户端可能处于「必须更新，但只能先升到中间版本」的状态，此时两者同时生效。

### 9.4 撤回（yank）

`yanked: true` 的节点**禁止**作为升级目标，但**必须**保留在 `versions` 数组中（不能删除），因为其他节点的 `minFrom` 可能依赖它的 `code` 值来定义可达性，且客户端可能正运行在该版本上。

客户端若发现自己正运行在一个 `yanked` 节点对应的版本上，**应该**照常执行选路（这通常会得到一个可用的升级目标），**可以**向用户提示当前版本已被撤回。

若某个版本已下载但尚未安装，而再次检查时发现它被撤回，客户端**应该**丢弃已下载内容并重新选路。

### 9.5 `resolveUpgradePath`（规范性，用于展示）

```
输入：index、currentCode
输出：版本节点数组（可能为空）

path = []
c = currentCode
while true:
    t = selectNextTarget(index, c)
    if t == null: break
    path.append(t)
    c = t.code
return path
```

因为 `selectNextTarget` 保证 `t.code > c` 严格递增，且 `versions` 有限，该循环必然终止。实现**禁止**为它设置人为的迭代上限并在达到上限时报错。

此函数用于向用户展示「你需要经过 1.2.0 才能到达 1.5.0」，也是发布侧做可达性校验的基础。

---

## 10. 可达性校验（规范性，发布侧）

`minFrom` 配置错误会把客户端**永久锁死**：若某个仍在使用的旧版本找不到任何满足条件的下一跳，它将永远无法升级，只能靠手动重装挽救。这类错误在发布后无法通过再发一个版本来修复受影响的客户端，因此发布工具在写入 index 之前**必须**执行本节校验，存在任一错误则**必须**拒绝发布。

### 10.1 定义与起点集合

**head** 定义为 `versions` 中 `code` 最大且 `yanked` 不为 `true` 的节点，即「当前可达的最新版本」。

**起点集合** 定义为：

```
Starts = { V.code | V ∈ versions } ∪ { 0 } ∪ ({ minSupported } 若该字段存在)
```

对每个起点 `c ∈ Starts`：

- 若 `c >= head.code`，**跳过**。这覆盖了 head 自身（其升级路径为空数组，本就无需可达），也覆盖了「head 被撤回后，运行在更高 code 上的客户端」这一已知无解情形（见 §9.4，只能靠发布新版本解决）。
- 否则，`resolveUpgradePath(index, c)` 的最后一项**必须**是 head。

起点包含所有节点的 `code`（含 `yanked` 节点），因为客户端可能正运行在任何已发布过的版本上，包括已被撤回的版本。

> **为什么检查有限个起点就足够。** 客户端的 `code` 可以是任意整数，看似需要检查无穷多个起点，实际不必。设 `c` 不等于任何节点的 `code`，令 `p` 为不大于 `c` 的最大节点 `code`。若从 `p` 出发可达 head，则存在节点 `V` 满足 `V.minFrom <= p` 且 `V.code > p`；由于 `p` 是不大于 `c` 的最大节点 `code`，区间 `(p, c]` 内不存在任何节点 `code`，故 `V.code > c`；又 `V.minFrom <= p <= c`，因此 `c` 同样能前进到 `V`，此后与从 `p` 出发的路径合流。归纳可得：**若所有节点起点与 `0` 均可达 head，则任意整数起点均可达 head。** 这使校验在 O(n²) 内完成，并保证不存在「校验通过但仍有客户端被锁死」的情形。

### 10.2 必须拒绝的错误

| 错误码 | 条件 |
|---|---|
| `no-head` | 不存在非 `yanked` 节点（全部被撤回），无任何可用版本 |
| `duplicate-code` | `versions` 中存在重复的 `code` |
| `unreachable` | 某个节点起点 `c < head.code` 无法到达 head |
| `min-supported-unreachable` | `minSupported` 起点无法到达 head |
| `min-supported-above-head` | `minSupported > head.code`，将导致全体客户端被判定为强制更新却无处可去 |
| `sequence-not-increasing` | 新 index 的 `sequence` 未严格大于远端现有 index 的 `sequence` |
| `manifest-digest-mismatch` | 某节点的 `manifest.sha256` 或 `manifest.size` 与实际上传的 manifest 字节不符 |

`code` 重复必须拒绝而不是警告，因为 `code` 是唯一合法的比较依据，重复会使选路结果依赖数组顺序。

由 §10.1 的推论可知，当所有节点起点均可达时 `min-supported-unreachable` 不会单独出现；保留该独立错误码是为了在校验实现自身有缺陷时给出更精确的诊断。

### 10.3 警告

| 警告码 | 条件 |
|---|---|
| `zero-unreachable` | 起点 `0` 无法到达 head |

`zero-unreachable` 是警告而非错误，因为发布方**可以**有意放弃极旧版本（例如令最早节点的 `minFrom` 大于 0）。选择这样做时**应该**同时设置 `minSupported`，以便那些客户端至少能收到明确的「请手动重新安装」提示，而不是静默地永不更新。

### 10.4 修剪历史节点的约束

index 会随发布次数增长。每个节点约 300 字节，1000 次发布约 300 KB，通常无需修剪。若确实需要修剪，**禁止**简单地删除最旧的 N 个节点，**必须**在删除后重新执行 §10 的全部校验。

原因：删除一个中转节点会直接破坏可达性。例如 `1.5.0` 的 `minFrom = 120`，若把 `code = 120` 的 `1.2.0` 节点删掉，所有 `code < 120` 的客户端将再也找不到合法的下一跳。

---

## 11. 产物选择（规范性）

`selectors` 是一个自由的字符串键值映射。客户端在运行时提供一个自身的选择器集合 `S`，然后按下述规则匹配。

**匹配规则：** artifact `A` 匹配客户端集合 `S`，当且仅当 `A.selectors` 中**每一个**键值对 `(k, v)` 都满足 `S[k] == v`。`A.selectors` 中未出现的键一律忽略；`S` 中多出的键一律忽略。空的 `selectors` 匹配任何客户端。

**多个匹配时：** 客户端**必须**取匹配结果中 `id` 字典序最小的那一个，并**可以**记录一条警告。同时，发布工具在 stage 阶段**必须**校验同一 manifest 内不存在两个 `selectors` 完全相同的 artifact。把仲裁规则定死（而不是「取第一个」）是为了让不同语言实现在异常输入下也给出一致结果。

### 11.1 标准选择器键

以下键名的含义由协议保留，实现**必须**按此解释：

| 键 | 取值示例 |
|---|---|
| `os` | `windows` `macos` `linux` `android` `ios` |
| `arch` | `x64` `arm64` `x86` `armv7` |
| `target` | 宿主自定义的子目标，如 RemoteCam 的 `client` / `server` |
| `abi` | 如 `musl` / `glibc` |
| `variant` | 同一平台的不同变体，如 `portable` / `setup` |

发布方**可以**使用其他自定义键。协议**禁止**限制键的集合。

> 这一节替代了 RemoteCam 目前的 `client.platforms.macos` 这种固定嵌套结构 —— 那种结构下新增一个 Linux 平台就必须同时修改生成脚本的命令行参数、workflow 和客户端解析代码。改成扁平数组加选择器后，新增平台只是多一个数组元素。

---

## 12. 客户端行为规范

本节的约束保证不同宿主的用户体验与安全性一致。

### 12.1 检查更新的流程（规范性）

0. **解析 index URL 列表（引导）：**
   - 若宿主配置了 `entryUrls`（directory 入口）：依次尝试（§12.7 可重排顺序），取第一个验签通过且 `product` 匹配的 directory（§16）；再从其 `services` 得到 index URL 有序列表（同样可经 §12.7 重排）。directory 验签失败或 `product` 不匹配则**必须**视为该入口不可用并试下一个；**禁止**降级为不验签使用。
   - 若宿主直接配置了 `indexUrls`：跳过 directory，使用该列表。
   - 宿主**可以**同时配置二者：此时**必须**先成功得到一份 directory，再用 directory 中的列表；**禁止**在 directory 可用时静默改用过期的内嵌 `indexUrls` 充当「更快的路径」（内嵌 `indexUrls` 仅可作为未配置 `entryUrls` 时的兼容模式）。
1. 依次尝试上一步得到的 index URL 列表，取第一个成功获取的响应。
2. 验签（§4.1）。失败则**必须**视为该源不可用，继续尝试下一个源；**禁止**降级为不验签使用。
3. 校验 `product` 与 `channel` 与自身期望一致，不一致则拒绝该源。
4. 校验 `sequence`（§12.4）。
5. 执行 `selectNextTarget`。为 null 则本次无更新。
6. 从目标节点的 `manifest.urls` 依次尝试下载 manifest（顺序可经 §12.7 重排），校验 `size` 与 `sha256`。不匹配则**必须**丢弃并尝试下一个 URL。
7. 校验 manifest 的 `product` / `version` / `code` 与 index 节点一致。
8. 按 §11 选出 artifact。

### 12.2 节流与退避（规范性默认值）

| 参数 | 默认值 | 说明 |
|---|---|---|
| 成功检查后的最小间隔 | 24 小时 | |
| 失败后的重试间隔 | 1 小时 | |
| index / manifest / directory 请求超时 | 10 秒 | |
| artifact 下载超时 | 无总时长上限，但空闲（无数据）超时 60 秒 | 大文件不能用固定总超时 |

宿主**可以**调整这些默认值，**应该**允许用户手动触发一次忽略节流的检查。

状态**必须**持久化在宿主的用户数据目录，至少包含：`lastCheckAt`、`lastResult`、`lastSeenSequence`、`skipped`（用户选择跳过的 code 列表）。若使用 directory，**应该**持久化 `lastSeenDirectorySequence`。为支持 §12.7，**应该**持久化各候选源的最近成功/失败与可选吞吐记录。状态**必须**按 (product, channel) 分别存储（directory 序号可按 product 存储）。

### 12.3 下载与校验（规范性）

- 支持断点续传的宿主**应该**使用 `Range` 请求，并在续传完成后按整文件 `sha256` 校验。
- 校验**必须**在文件完整落盘后进行，顺序为先比 `size` 再比 `sha256`。
- 校验失败**必须**删除该文件，且**禁止**将其用于安装。可以换一个 URL 重试。
- 部分下载的临时文件**必须**与校验通过的文件在命名或目录上明确区分，避免把未完成的文件当成可用产物。
- 多个 URL 之间**必须**顺序回退，**禁止**并行向多个镜像请求同一个 artifact。

> 现有实践对照：Dec 目前完全没有校验，下载的二进制直接替换自身；RemoteCam 有 per-file SHA256 但没有签名。本协议要求两者都有。

### 12.4 防降级（规范性）

客户端**必须**持久化记录该 (product, channel) 见过的最大 `sequence`（`lastSeenSequence`）。

- 若某个源返回的 index 的 `sequence` **小于** `lastSeenSequence`，客户端**必须**拒绝采用该 index。
- 该拒绝**禁止**被当作错误上报给用户。客户端**应该**继续尝试其他源；若所有源都返回较小的 `sequence`，则视为「本次无更新」并保持已知状态。

这条约束的必要性：签名只能证明「这份 index 确实由发布方签发过」，不能证明「它是最新的一份」。攻击者若能控制网络，可以重放一份旧的、签名依然有效的 index，把客户端引导去安装一个有已知漏洞的旧版本。

`sequence` 不同步的正常场景（GitHub 已更新、Gitee 尚未同步）会命中「继续尝试其他源」这条分支，因此不会造成故障，最终一致后自然恢复。

### 12.5 应用更新

协议**不定义**如何安装。协议的职责终止于「把一个哈希校验通过的本地文件路径，连同它的 `kind`、`filename`、`meta` 一起交给宿主」。

宿主**应该**为自己的安装方式实现失败回滚。参考 Dec 的做法：替换二进制前先把原文件重命名为 `.bak`，复制失败则从 `.bak` 恢复。

### 12.6 Fallback 检查（规范性）

Fallback 是正常 OTA（§12.1）之外的**救急通道**：当某些已装版本的更新链失效、用户又不会主动去下载页时，发布方通过签名文档催促其打开手工更新链接。

#### 12.6.1 Fallback 文档

字段定义见 [`proto/rup/v2/objects.proto`](proto/rup/v2/objects.proto) 中的 `Fallback` / `FallbackRule`。

| 字段 | 说明 |
|---|---|
| `schema` | 必须为 `"rup.fallback/2"` |
| `product` | 必须与客户端期望的 product 一致 |
| `sequence` | ≥ 1，每次覆盖写入**必须**严格递增；防回滚独立于 index 的 `sequence` |
| `rules` | 有序规则列表；客户端采用**第一条**同时满足 code 区间与 selectors 的规则 |

规则匹配：`min_code <= currentCode <= max_code`（闭区间）。`selectors` 为空则匹配所有客户端；非空时按 §11 的选择器匹配语义（本消息无 artifact，只做约束过滤）。`manual_url` **必须**是绝对 URL，且**必须**来自本签名文档；客户端与宿主**禁止**为催更目的硬编码替代链接。

空 `rules` 表示当前无催更（用于解除先前规则）。

#### 12.6.2 检查流程

1. 若宿主未配置 fallback URL 列表，跳过本节，结果仅由 §12.1 决定。
2. 依次尝试 fallback URL 列表，取第一个验签且 `product` / `sequence` 通过的响应。验签失败**必须**继续下一源；**禁止**不验签使用。
3. 校验 `sequence`：持久化该 product 见过的最大 fallback `sequence`（独立于 index 的 `lastSeenSequence`）。小于已知值则拒绝该源。
4. 在 `rules` 中找首条匹配当前 `currentCode` 与 `clientSelectors` 的规则；找不到则视为「无催更」。

#### 12.6.3 与正常检查的合并优先级

当宿主同时执行 §12.1 与 §12.6 时，对外结果**必须**按以下优先级合并：

1. **UpdateAvailable**（正常 OTA 可用）— 优先走应用内更新，即使 fallback 规则也命中。
2. **FallbackRequired**（规则命中）— 在无可用 OTA 时催促手工更新（覆盖 `UpToDate` 与 `CheckFailed`）。
3. 其余正常检查结果不变。

SDK **应该**额外暴露仅跑 §12.6 的入口，供下载/安装失败后再次催更。

Fallback **禁止**执行自动下载或 apply；宿主只展示 `message` 并打开 `manual_url`。

### 12.7 多源顺序与学习（规范性）

本节适用于：`entryUrls`、directory 内的 `services`、以及文档中的 `urls[]`。

1. **禁止**为「选择从哪里下载」而单独发起探测、测速或预下载；**禁止**因此产生随后丢弃的流量。
2. 多个候选之间**必须**顺序尝试；**禁止**并行请求同一个逻辑对象（与 §12.3 一致）。
3. 客户端**可以**根据本机在**真实**更新流程中记录的成功/失败与可选吞吐，对候选列表重排；无历史时**必须**保持文档/配置给出的默认顺序（directory 内按 `priority` 升序，同优先按数组序；`urls[]` 按数组序）。
4. 记账**必须**只发生在真实的 directory / index / manifest / artifact 请求完成之后（成功或失败）。
5. 学习数据**禁止**削弱验签、`sha256` 或 `sequence` / `directory_sequence` 防降级规则：被拒绝的响应不得记为「成功源」。

推荐排序（非唯一算法，但合规实现**应该**语义接近）：上次完整成功的候选优先 → 有吞吐记录的按吞吐降序 → 默认序；近期连续失败的**可以**暂时后置，**禁止**永久拉黑到无法再试（除非已不在当前 directory / 文档列表中）。

---

## 13. 后端插件接口

后端插件是发布工具的扩展点，接口只有四个方法 —— 三个写、一个读：

```
put_artifact(local_path: str, key: str) -> list[str]
    上传不可变产物，返回可公开访问的 URL 列表（通常一个元素）

put_immutable(data: bytes, key: str) -> list[str]
    上传不可变的小文件（manifest），返回 URL 列表

put_pointer(data: bytes, key: str) -> list[str]
    写入或覆盖可变指针（index），返回 URL 列表

get(key: str) -> bytes | None
    读回 key 处的字节，不存在则返回 None
```

三个写方法按**可变性**而非按体积划分，因为调用方绝不能弄错的正是这个区别：`put_pointer` 是一次发布的提交点，它之前的每一步失败都可以安全重试（没有 index 指向的产物与 manifest 对客户端不可见），而指针一旦写入就立刻对全部客户端生效。

`get` 用于发布工具自身读回现状（校验现有 index、比对 manifest 哈希）。它**不能**由「按文档里的 URL 去下载」替代：`local` 后端的 URL 描述的是将来准备托管的位置，此刻通常不可解析。判断「客户端真的下载得到吗」是另一件事，属于深度校验的范畴。

### 13.1 两类后端（规范性）

按 §3 的 key 映射方式，后端分成两类，区别不是实现细节，而决定了接口能不能拆：

**路径型**（本地目录、COS / S3、CNB 仓库直链、自建静态服务、CDN 回源）。key 直接是路径，URL 为 `baseUrl + key`，**在上传之前就已确定**。因此「怎么把字节送上去」与「URL 长什么样」是两件正交的事：

| | 放置 | 定位 |
|---|---|---|
| 本地目录 | 写文件 | `baseUrl + key` |
| CNB 仓库 | 写进仓库工作区，由仓库自身的 CI 接手 | `baseUrl + key` |
| COS / S3 | 签名调用上传 API | `baseUrl + key` |
| 自建静态服务 | rsync / scp / WebDAV | `baseUrl + key` |

右列对全部路径型后端是同一份逻辑，只有左列各不相同。**从客户端看，这一整类后端完全同质：都只是一次普通的 HTTP GET。** 因此新增一种托管方式通常只需实现放置，下载侧与校验侧不必改动，客户端更是完全无感。

注意 CNB 那一行的左列：它不需要任何上传机制。产物随仓库流转，发布工具只负责把文件摆进工作区的正确位置，剩下的由仓库的 CI 完成。「不上传」是放置的一种合法实现，而非缺失。

**Release 型**（GitHub / Gitee / CNB Release 附件）。没有路径概念，只有 `(tag, assetName)`，且下载 URL 是**上传响应返回的**（GitHub 的 `browser_download_url`）。放置与定位无法分离，必须整体实现。

发布工具**应该**按这个分界组织实现：路径型共用一份 URL 推导与越界保护，Release 型各自完整实现。**禁止**为了统一而要求路径型后端也「先上传才能知道 URL」—— 那会让离线预演、只读校验、以及「产物已由别的系统送达」这三种场景都无法表达。

约束：

- 实现**必须**在 `put_pointer` 成功返回后保证内容对匿名访问者可见（或明确说明其最终一致延迟）。
- 实现**必须**支持匿名读取产出的 URL，或在文档中明确声明需要凭据（此时宿主必须自行处理鉴权，属于协议范围之外）。
- `local` 后端**必须**始终可用：它把完整的 key 目录树输出到本地文件系统，并按配置的 `baseUrl` 生成 URL。有了它，任何静态托管方式（对象存储、自建 Nginx、手工 rsync、U 盘）都能兜底，发布方不会被任何单一平台卡住。

### 13.2 关于 CNB 的现状（非规范性）

截至撰写时，CNB 的制品库只提供生态专用类型（Docker / Helm / Maven / npm / PyPI / Cargo / Conan 等），**没有通用（raw / generic）制品库**；社区已就此提出诉求但尚未实现。通用文件的官方路径是 **Release 附件**，通过 `cnbcool/attachments` 插件或 Open API 上传下载，文档中的下载示例均携带 token。因此在把 CNB 作为客户端直接下载源之前，**必须**先验证一件事：公开仓库的 Release 附件是否存在稳定的、匿名可访问的 HTTP 直链。

本协议的设计使这个问题不会阻塞任何决策：URL 一律写在签名文档里，因此后端换成 CNB Release、COS、自建静态服务器或它们的任意组合，客户端代码都不需要改动。这也正是 §1.1 第 1 条把「禁止客户端拼接 URL」列为首要目标的原因 —— Dec 目前从 `{os}-{arch}` 推导 GitHub 下载地址的做法，在换到 CNB 或 COS 时会直接失效。

---

## 14. 安全模型

### 14.1 威胁与对策

| 威胁 | 对策 |
|---|---|
| 传输损坏 | artifact 的 `sha256` |
| 篡改 artifact | manifest 的 `sha256` + index 的签名 |
| 篡改 manifest | index 中记录的 `manifest.sha256` + index 的签名 |
| 篡改或伪造 index / fallback / directory | ed25519 签名，公钥内嵌于客户端 |
| 篡改 directory 以指引错误取货点 | directory 验签；即使误导，装包仍受 index 签名与哈希约束 |
| 重放旧 index 实施降级 | `sequence` 单调性检查（§12.4） |
| 重放旧 directory | `directory_sequence` 单调性检查（§16.2） |
| 冻结（一直返回旧的合法 index，使客户端不知道有新版本） | 部分缓解，见 §14.3 |
| 恶意 `filename` 造成路径穿越 | §14.4 |
| 私钥泄露 | 多签名信封支持密钥轮换（§4.1） |
| 以探测为名的流量浪费 / 元数据泄露 | §12.7 禁止专用探测 |

### 14.2 密钥管理

- 私钥**禁止**进入代码仓库。**应该**存放于 CI 的加密 secret 或离线签名机。
- 工具**应该**支持「离线签名」：在联网机器上完成上传与 URL 收集，把待签字节导出，在离线机器上签名后再导回。
- 公钥以 `keyId` 标识，内嵌于客户端。客户端**应该**同时内嵌至少两把公钥（当前 + 备用），以便在不发布新客户端的前提下完成轮换。
- 发布方在写指针之前**应该**用「客户端所信任的那一组公钥」把刚生成的签名验一遍。§12.1 要求客户端对验签失败保持沉默，这使得「用客户端不认识的 `keyId` 签名」成为一种没有任何报错的发布事故：发布流程全程成功，而所有客户端从此停止更新，现场看起来像服务端故障。发布方手上本来就有这组公钥（客户端内嵌的正是它），自检的成本近乎为零。

### 14.3 冻结攻击与 `expiresAt`（非规范性）

`sequence` 能防降级，但不能防「攻击者持续返回一份旧的、签名有效的 index」。标准解法是给 index 加 `expiresAt`，客户端拒绝过期文档，迫使发布方定期重新签发。

代价是发布方即使没有新版本也必须周期性重签，对个人项目偏重。因此 v1 将 `expiresAt` 定义为可选字段，客户端**可以**忽略。是否启用留给后续决定。

### 14.4 文件名安全（规范性）

客户端在使用 `filename` 落盘前**必须**校验：不含路径分隔符（`/` `\`）、不等于 `.` 或 `..`、不含 `..` 片段、不含控制字符或 NUL、在 Windows 上不是保留设备名（`CON` `PRN` `AUX` `NUL` `COM1`–`COM9` `LPT1`–`LPT9`）。校验失败**必须**拒绝该 artifact。

**禁止**直接把远端提供的 `filename` 拼进本地路径而不做校验 —— 这是经典的路径穿越入口。

---

## 15. 兼容与演进

- `schema` 字段的格式是 `<对象名>/<主版本>`。主版本变化表示不兼容变更。
- 客户端遇到**无法识别的主版本**时**必须**拒绝该文档并保持当前版本，**禁止**猜测性解析。
- 客户端遇到**无法识别的字段**时**必须**忽略它。因此在同一主版本内新增可选字段是向后兼容的。
- 客户端遇到无法识别的 `kind` 或 `selectors` 键时，**应该**跳过该 artifact 而非报错。
- 一致性用例（`conformance/`）的目录名带版本号。任何对规范性行为的修改都**必须**同时更新用例。

---

## 16. directory 对象（规范性）

directory 是 product 级的**引导文档**：告诉客户端「当前该去哪些绝对 URL 拉取签名的 index / fallback」。它**不是**第二套版本清单。

设计说明（非规范性）：客户端内嵌的 `entryUrls` 一主多备（例如公网静态主站 + CNB raw + GitHub raw）只托管这份小文档的镜像；真正的更新流量仍由 directory 指向的更新服务及 index 内 `urls[]` 承担。细节见 [`docs/design/bootstrap-directory.md`](docs/design/bootstrap-directory.md) 与 ADR 0005。

### 16.1 字段

字段的 protobuf SSOT 见 [`proto/rup/v2/objects.proto`](proto/rup/v2/objects.proto) 中的 `UpdateDirectory`（消息名；逻辑对象仍称 directory，schema 为 `rup.directory/2`）。

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `schema` | string | 是 | 必须为 `"rup.directory/2"` |
| `product` | string | 是 | 必须与客户端期望的 product 一致 |
| `directory_sequence` | integer ≥ 1 | 是 | 该 product 目录文档的发布序号，每次覆盖写入**必须**严格递增 |
| `updated_at` | string | 否 | RFC 3339 UTC，仅供展示与排障 |
| `services` | repeated | 是 | 长度 ≥ 1 |

`services[]`：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `id` | string | 是 | 稳定标识，供 §12.7 学习偏好引用；同一 directory 内唯一 |
| `priority` | integer | 是 | 无学习数据时的默认序；**数值越小越优先** |
| `index_url` | string | 是 | 绝对 URL，指向该 product 某 channel 所用 index 的取货点（宿主按 channel 选择对应 service 或由发布方保证 URL 已按 channel 区分；见下） |
| `fallback_url` | string | 否 | 绝对 URL，指向签名 fallback |

Channel：v2 首版 directory **可以**为每个 `(product)` 提供面向默认 channel 的 `index_url`，或由发布约定 `services` 内用 `id` / 约定命名区分 channel。若同一 product 多 channel 需要互不干扰的引导，发布方**应该**使用明确可区分的 `index_url`（例如路径中含 channel），客户端**必须**只采用与当前 channel 匹配的条目（实现可用 `id` 前缀或后续可选 `channel` 字段；加入可选 `channel` 字段属同主版本兼容扩展）。

### 16.2 校验与防降级（规范性）

1. 拉取后**必须**按 §4.1 验签；失败则丢弃该入口。
2. **必须**校验 `product`。
3. 客户端**应该**持久化 `lastSeenDirectorySequence`；若某入口返回的 `directory_sequence` **小于**已知值，**必须**拒绝该份 directory，并试下一个 `entryUrl`。该拒绝**禁止**当作用户可见的更新失败（与 §12.4 同类）。
4. `services[].index_url` / `fallback_url` **必须**为绝对 URL；客户端**禁止**拼接。

### 16.3 发布约束（规范性）

1. directory 与 index / fallback **必须**由同一信任根签名（通常同一组发布私钥）。
2. 签名**一次**，再把**相同字节**分发到各个 `entryUrls` 镜像。**禁止**为不同入口生成内容不同的 directory。
3. `services` 内各 `index_url` 在目录生效时**必须**能取到**字节一致**的已签名 index（§5.3）。
4. 持钥签名动作**必须**集中；上传到多个静态宿主**可以**并行，且**禁止**要求每个宿主各自持有私钥。

### 16.4 与直接配置 `indexUrls` 的关系

未使用 directory 的宿主仍可只配置 `indexUrls`，行为与历史 §12.1 兼容。新宿主**应该**优先 `entryUrls` + directory，以便在不发版客户端的情况下迁移更新服务拓扑。

---

## 附录 A：与现有两个项目的映射

| 概念 | Dec | RemoteCam | 本协议 |
|---|---|---|---|
| index | `ReleaseLatest` 分支的 `version.json`（仅一个版本号） | 固定 tag 的 `update_config_*.json`（index 与 manifest 合并） | `index/<product>/<channel>.json`，已签名，含完整版本链 |
| manifest | 无（URL 从平台名推导） | 与 index 合并，只有最新一份 | 每版本一份不可变文档 |
| 版本比较 | 手写三段整数，丢弃 `-beta` | 只比 `x.y.z`，丢弃 `+build` | 仅比较 `code` 整数 |
| 多源 | 版本检查三源回退；下载单源 | 检查双源并行竞速；下载失败回退 | directory 引导 + `urls` 数组，顺序回退；全网同一份 index/manifest 字节；**禁止**为选源并行竞速（§12.7） |
| 校验 | 无 | per-file SHA256 | 签名 + 逐级 SHA256 |
| 升级路径 | 无，只有 latest | 无，只有 latest | `minFrom` + `selectNextTarget` |
| 历史版本 | 覆盖后丢失 | 覆盖后丢失 | index 中保留全部节点 |
| 平台维度 | 命名约定 `dec-{os}-{arch}` | 固定嵌套 `platforms.{macos,windows,android}` | 扁平数组 + `selectors` |
| 发布顺序 | 产物先于指针 | manifest 先于产物（存在不一致窗口） | 产物 → manifest → index，index 为提交点 |
