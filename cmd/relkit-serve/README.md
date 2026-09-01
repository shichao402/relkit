# relkit-serve

> 本目录是 [`cnb.cool/shichao402/relkit`](https://cnb.cool/shichao402/relkit) 仓库的一部分（`cmd/relkit-serve`），与发布 CLI 同仓。部署脚本在仓库根的 `deploy/`。

一个静态文件服务，用来托管 [RUP](https://cnb.cool/shichao402/relkit/blob/main/SPEC.md) 发布树。单个静态链接的可执行文件，无运行时依赖。

**角色：** 这是内网数据面的一种实现——匿名 GET 已经写好的文件。发布控制面是 `relkit-agent`（持钥、写盘或写 COS）。新产品不要把 serve 的 PUT 当成另一套发布协议。

做这些事：

1. **对外提供下载**，正确支持 Range 请求，因此客户端可以多线程并发下载。
2. **对外目录是发布写好的 `browse/` 静态页。** GET `/` 原样返回 `browse/index.html`；没有 dump 就一页说明，不再现场扫盘画门户。操作面板在 `/-/admin`（现算产品卡、`/-/p/`、`/-/admin/files`）。
3. **可选地接受带鉴权的 PUT 上传**（遗留）。迁到本机 agent 之后应关掉 token，让 PUT 返回 405。
4. **按 index 引用清理孤儿**，删掉已不被任何 channel 的 index 引用的旧 `manifest/` / `artifact/`。

也可以纯当通用静态服务用 —— 把缓存前缀改成你自己的路径约定即可。只是它的差异化价值来自理解 RUP 的可变/不可变语义，见下文。

**要部署它，读 [`AGENT-GUIDE.md`](AGENT-GUIDE.md)**，那是操作性知识的唯一来源。本文解释它为什么长这样。

---

## 先澄清一件事：多线程下载不是服务端功能

「多线程下载」是**客户端行为**：客户端开若干条连接，每条带 `Range: bytes=X-Y` 头取文件的一段，最后拼起来。服务端要做的只是正确响应 Range 请求（返回 `206 Partial Content`）。

因此：

- **HTTPS 与多线程下载完全正交，不存在冲突。** TLS 在传输层，Range 是它之上的 HTTP 语义。每条并发连接各自握手，互不影响。所有下载器（aria2、IDM、curl `--range`）对 https 链接都是这么工作的。
- Go 的 `net/http` 里 `http.ServeContent` **已经实现了** Range、多段 Range、`If-Range`、`If-Modified-Since` 与条件请求。所以这部分不是本程序的工作量，本程序的价值在它周围：留在服务目录内、缓存头分流、以及不破坏零拷贝发送路径。

---

## 为什么不直接用 Nginx 或 Caddy

诚实的回答：**如果只要下载，Caddy 就够了**，它同样是单个 Go 二进制，Range、高并发、自动 HTTPS 都有，且经过远超本程序的生产验证。

自己写只有两个理由，都与 RUP 直接相关：

**一、历史上需要一个上传端点。** 让旧的 `http-put` 一条命令完成发布。新产品应改走 `relkit-agent` 写同一目录（`local` 后端）。没有 token 时 PUT 已是 405。

**二、需要服务端理解「哪个路径是可变的」。** RUP 里 `index/` 是可变指针，`manifest/` 与 `artifact/` 一旦发布就不再改变。通用服务器无法区分，只能给所有路径同一套缓存策略，而两种错法都有代价：

- 缓存了 `index/` → 一次已经完成的发布，在缓存过期前看起来「还没发布」。
- 不缓存 `artifact/` → 每次更新都重新传输真正大的那部分。

本程序默认按前缀分流，规则可在配置文件里覆盖。

---

## 快速开始

```bash
# 生成配置与 token
relkit-serve init -dir /srv/releases -out /etc/relkit-serve

# 起服务
relkit-serve -config /etc/relkit-serve/relkit-serve.json
```

Linux + systemd 上一步到位（建用户、装二进制、写配置、装单元、跑五项自检）：

```bash
sudo ./deploy/install.sh --binary ./dist/relkit-serve-linux-amd64
```

未提供 token 时上传端点关闭，`PUT` 返回 405。这是默认状态，也就是说默认安全。

健康检查在 `/-/health`。放在 `/-/` 下是为了永远不会遮蔽真实文件：RUP 的 key 以 `index/`、`manifest/`、`artifact/` 开头，而横线不是合法标识符首字符。

## 命令

| 命令 | 作用 |
|---|---|
| `relkit-serve` | 起服务 |
| `relkit-serve init` | 生成配置骨架与运营方 token |
| `relkit-serve init -product <id>` | 为该产品签发隔离上传 token（合并进已有配置） |
| `relkit-serve init -product <id> -share-with <id>` | 把新产品挂到已有 token 上（不签发新秘密） |
| `relkit-serve init -list-products` | 列出已放行的产品与 token 文件（不打明文） |
| `relkit-serve init -product <id> -remove` | 吊销该产品的上传 token（重启后生效） |
| `relkit-serve agent-guide` | 打印内嵌的部署运维手册 |
| `relkit-serve -version` | 版本 |

手册内嵌在二进制里，因此在一台刚装好的机器上不联网也能读到，且读到的一定与当前运行的构建配套。

## 配置

字段与参数的完整对照表在 [`AGENT-GUIDE.md`](AGENT-GUIDE.md) §6，示例在 `deploy/relkit-serve.example.json`。要点：

- 缺省按 `./relkit-serve.json`、`/etc/relkit-serve.json` 顺序查找，可用 `-config` 指定；**启动日志总会打印实际用了哪个文件**。
- **未知字段一律报错。** 拼错键名而静默沿用默认值是最难查的一类配置故障 —— 服务照常启动、报告成功，行为却与配置文件写的不一样。
- 优先级是**命令行 > 环境变量 > 配置文件**。环境变量在中间，是为了让容器或 systemd drop-in 能轮换 token 而不必改一个可能由配置管理系统托管的文件。
- 运营方 token 三条路：配置文件里的 `uploadToken`、`uploadTokenFile` 指向的文件、环境变量 `RELKIT_SERVE_TOKEN`。产品隔离 token 走 `uploadTokens`（文件 + `products` 列表）。**不支持用命令行参数传 token**：那样它会出现在 `ps` 的输出里，机器上任何用户都能看到，并且会进入 shell 历史。含 token 的文件权限过宽时启动日志会打 `WARNING`。

### 强制升级发布工具

在服务端配置中设置：

```json
"publish": {"minProtocol": 2}
```

之后，所有已鉴权的 `PUT` 都必须带
`X-Relkit-Publish-Protocol: 2`（或更高），否则服务端在创建目录或临时文件前返回
`426 Upgrade Required`。这只限制发布者；SDK 的 index / manifest / artifact GET
完全不受影响。

当前 `relkit publish` 会先 POST `/-/publish/preflight`。协议不满足时，CI 在上传
大文件前就以“upgrade relkit”失败；每个后续 PUT 仍会重复检查，防止不知道
preflight 的旧工具绕过。`minProtocol: 0` 保留对通用 PUT / WebDAV 发布器的兼容，
生产环境要强制统一发布工具时应显式设为 `2`。

## 日志

一行一个请求，含客户端 IP、方法、路径、状态码、字节数、耗时，以及 `Range` 头（如有）。

记录 `Range` 是有意的：它是「这个客户端在并发下载」的直接信号。有人报告下载慢时，第一件要看的事就是慢的那些请求里有没有 `Range`。

---

## 网页

协议客户端不读这些页面 —— 它只取签名过的 index，再按其中的绝对 URL 下载。页面是给人看的，因此按人的问题组织，而不是按 RUP 的 key 空间。

| 路径 | 内容 |
|---|---|
| `/` | 对外目录：有 `browse/index.html` 就原样返回；否则一页说明。不是操作面板 |
| `/browse/` | 同一份 dump（`index.html`、`<product>.html`、`catalog.json`） |
| `/-/admin` | 操作面板：现算产品卡。须登录。以后长成 relkit 后台；不是给人书签的目录 |
| `/-/p/<product>` | 操作面板里的单产品页（现算，含下载次数与技术细节）。须登录 |
| `/-/latest/<product>/<channel>/<artifact-id>` | 长期有效的下载地址；读取发布时写好的 `latest/<product>/<channel>.json` 后 302 到具体产物 |
| `/<dir>/` | 目录列表：名字、大小、修改时间（数据面，匿名） |
| `/-/admin/files` | 发布树根目录列表（旧书签 `/?files=1` 会 301 过来）。须登录 |

几点设计取舍：

- **对外目录是 browse dump，操作面板在 `/-/admin`。** 一台 serve 通常带多个产品，但给人看的首页必须是发布写好的静态页（和公网 Makers 同一份）。现算门户、文件树给运维，走 `/-/admin`。`.pb` 链接只在操作面板的产品页。
- **产品与版本从 index 现读**，不额外维护状态文件：`index/<product>/<channel>.pb` 是发布之后唯一必然为真的东西。页面按请求解析它们（都是极小的 protobuf），因此发布完刷新即见，不需要缓存失效逻辑。
- **`index/` 下没有一个能解析的文档时，`/-/admin` 退回文件列表**。这保证「纯当通用静态服务用」的场景不受影响，也让一台 index 全坏的机器仍然能被翻查。
- **下载链接挂在 channel 行上，不做单独的“推荐下载”按钮**。面板不替人选 channel：想要 stable 就点 stable 那行，想尝鲜就点 dev 那行。链接指向的产物按 `User-Agent` 猜平台，认不出来（curl、Linux 等）就整列留空 —— 递错包比不给链接更糟。
- **固定下载地址只读发布指针**。每个 channel 发布都原子覆盖自己的 `latest/<product>/<channel>.json`，其中已列好该版本的 artifact ID、selectors 与 URL；`/-/latest/<product>/<channel>/<artifact-id>` 不扫描 index / manifest，也不会因为 dev 发布而改动 stable 的地址。URL 里写明 channel，是因为一条贴进文档的链接必须自己说清它跟的是哪条线。
- **下载次数写入服务目录根下的 `.relkit-serve-stats.json`**，重启后仍在。页面上写明起算时间。这个文件不出现在目录列表里，GET/PUT 也会被拒绝，因此它不是又一个可被取走的对象。计数本身在内存里累加，落盘是防抖之后的拷贝，热路径上不会多一次写。只统计「无 Range 或 Range 从 0 开始」的 GET，因此 16 线程下载算一次而不是十六次。需要把文件放到树外时，配置 `statsFile`（并给 systemd 加上对应的 `ReadWritePaths`）。

**产品文案由产品团队维护**，写在产品仓库的 `relkit.json`；每次发布同步到 `site/<product>.json`。服务端配置只保留整站标题，以及迁移期 fallback。

```json
"site": {
  "title": "Demo App",
  "description": "…",
  "homepage": "https://…"
}
```

页面无外部字体与图片，全部内联，因此在不通公网的内网浏览器里也能正常渲染。主题默认跟随 `prefers-color-scheme`，右上角可切换 System / Light / Dark；唯一的内联 JavaScript 只负责把这项偏好保存在浏览器 `localStorage`，不参与发布内容渲染。

---

## 与 relkit 对接

配一个 `http-put` 后端：

```json
{
  "backends": {
    "dl": {
      "type": "http-put",
      "baseUrl": "http://dl.internal:8080/",
      "tokenEnv": "RELKIT_UPLOAD_TOKEN"
    }
  },
  "publishTo": ["dl"]
}
```

```bash
export RELKIT_UPLOAD_TOKEN='<init -product 打印的那个，只属于本产品>'
relkit stage 1.0.0 --code 100 --add dist/app.zip os=windows,arch=x64
relkit publish 1.0.0
relkit verify --deep
```

`baseUrl` 与 `uploadUrl` 可以不同，而且这是常见情形而非特例：下载走 CDN 或公网域名，上传走内网地址。

```json
{
  "type": "http-put",
  "baseUrl": "https://cdn.example.com/releases/",
  "uploadUrl": "http://10.0.0.5:8080/",
  "tokenEnv": "RELKIT_UPLOAD_TOKEN"
}
```

`baseUrl` 会被写进 manifest 并被签名，所以它必须是客户端实际访问的地址；写错了要重新发布才能改。

若产物由别的机制送达（仓库 CI、rsync），则用 `static-http` 后端配 `stageDir`，本服务只管对外下载。

### 只留最新版时怎么配合

本服务**不改写、不重签** index。GC 只做一件事：扫全部 `index/<product>/<channel>.json`，把仍被引用的本地 `manifest/`、`artifact/` 留住，其余删掉。

因此若运营策略是「整包更新、磁盘上只留当前版本」，发布侧 PUT 的 index 就应只含当前那一个版本节点。在 `relkit.json` 设 `"retainVersions": 1`（或 `2`/`3` 留最近几个回退点）即可；`relkit publish` 会在签名前裁掉更旧的节点。若 index 里仍挂着历史节点，那些节点引用的对象会被视为 live，GC 不会删 —— 这是刻意的安全行为。多 channel（如 `stable` / `beta`）按引用并集保留。

触发方式：默认每小时扫一次；每次成功 PUT `index/` 后也会异步再扫（带短 debounce）。可用 `-gc=false` 或配置 `"gc": {"enabled": false}` 关闭。业务侧始终只 PUT，不需要 DELETE。

---

## 构建

```bash
./build.sh 0.1.0        # 或 .\build.ps1
```

产物在 `dist/`，约 5.5 MB，覆盖 linux/amd64、linux/arm64、windows/amd64、darwin/arm64。

`CGO_ENABLED=0` 是「任何 Linux 都能跑」的关键：否则二进制会链接构建机的 libc，在 musl 发行版（如 Alpine）或更旧的 glibc 上会直接起不来，而错误信息（`No such file or directory` 指向一个明明存在的文件）极具误导性。本程序用到的一切都有纯 Go 实现（包括 DNS 解析），所以这里没有任何取舍。CI 会验证产物确实是静态 ELF。

---

## 性能

`go test -bench . -benchtime 2s`，AMD Ryzen 9 9950X，loopback：

| 场景 | 吞吐 |
|---|---|
| 单连接整文件 | 1113 MB/s |
| 8 并发各拉整文件 | 3839 MB/s |
| 64 并发 | 4341 MB/s |
| 256 并发 | 4082 MB/s |
| 单客户端 16 路 Range 切片 | 1375 MB/s |

这些数字测的是 HTTP 栈与文件路径，不含真实网络。要点在于**256 并发下吞吐没有塌陷**（与 64 并发基本持平）。作为参照，万兆网卡理论上限约 1250 MB/s，所以服务端有三倍以上余量 —— 瓶颈会先出现在网卡和磁盘上。

有两个非显然的陷阱是刻意避开的，任何在此基础上改动的人都需要知道：

**不要给 `http.Server` 设 `WriteTimeout`。** 它作用于整个响应。一个 200 MB 的包在慢链路上要传几分钟，任何大到不会误杀它的值都已经大到保护不了任何东西；任何小到能起保护作用的值都会把正常下载截断在中途。慢速攻击由 `ReadHeaderTimeout` 与 `IdleTimeout` 处理。

**包装 `ResponseWriter` 会关掉 sendfile。** `net/http` 只有在 `ResponseWriter` 实现了 `io.ReaderFrom` 时才走 `sendfile(2)` 零拷贝。为了记录字节数而包装它，会静默地把每次下载的每个字节都绕回用户态 —— 而且没有任何测试会发现，因为字节内容仍然正确。本程序的 `recorder` 转发了 `ReadFrom`，两者兼得。

### 在真实环境测

```bash
# 单连接基线
curl -o /dev/null -w '%{speed_download}\n' http://dl.internal:8080/artifact/app/1.0.0/app.zip

# 16 路并发
aria2c -x16 -s16 -d /tmp http://dl.internal:8080/artifact/app/1.0.0/app.zip
```

---

## 安全

**为什么纯 HTTP 是可以接受的。** RUP 的完整性来自 index 上的 ed25519 签名与 manifest 里的 sha256，不依赖传输层。中间人替换产物会被哈希拦住，替换 index 会被签名拦住，回放旧 index 会被 `sequence` 拦住。TLS 在这套设计下买到的是保密性（别人看不到你在下载什么版本），而不是完整性。内网分发通常不需要，需要时在前面挂一层反代即可 —— 但要记得让 `Cache-Control` 透传，否则上面的缓存分流会失效。

**服务目录的写权限等于客户机的代码执行权限。** 这个目录里的产物会被客户端下载并执行。上传 token 因此与私钥同级别对待：以 sha256 摘要保存、常量时间比较（既不泄露内容也不泄露长度）、不接受从命令行传入。签名机制在这里帮不上忙 —— 能写目录的人也能写 `index`，他缺的只是签名私钥，所以**签名私钥不应该出现在这台机器上**。GC 也因此只读 index 信封解码引用、从不持有私钥去改写 index。

**上传是原子的。** 写入先落临时文件再 `rename`。`rename(2)` 在同一文件系统内是原子的，这一点在这里格外重要：index 是一次发布的提交点，客户端必须看到「上一个版本」或「新版本」，绝不能看到半个文件。已经打开旧文件的读者不受影响，因为 rename 只替换目录项。

**路径限制。** 所有文件操作走 `os.Root`，它把操作限制在服务目录内，**包括经由符号链接的逃逸** —— 这正是手写前缀检查通常漏掉的一类。请求路径在此之前还会被单独拒绝一次，这样遍历尝试在日志里表现为 404 而不是一个打开失败。

**对外目录是静态 dump；操作面板须登录。** `GET /` 返回 `browse/index.html`（没有则短说明）。现算门户在 `/-/admin`，登录后可看。各子目录仍返回 HTML 索引（数据面，匿名）。协议客户端仍只信任签名过的 index。因此发布目录里仍然不要放别的东西：列表会把它们暴露出来，且任何路径都能被按名取走。

面板鉴权是实例本地的一次性引导凭据：`init` 打印 `RELKIT_ADMIN_BOOTSTRAP`，用它创建第一个运营账户后即作废。日常用户名密码登录；忘了就 SSH `init -reset-admin`。上传 token 不能登录面板，引导凭据不要进 Dec。见 [ADR 0006](../../docs/adr/0006-admin-panel-bootstrap.md)。

---

## 测试

```bash
go test ./...                              # 功能
go test -run "^$" -bench . -benchtime 2s   # 吞吐
```

覆盖：Range 切片正确性、16 并发下载拼回原文件、HEAD 只返回大小、缓存头按前缀分流、路径遍历被拒、目录列表、操作面板（登录后：多产品、channel 排序、坏 index 退回列表、`/-/admin/files`、HEAD 无正文、不可缓存；未登录 302；一次性引导建户后作废、`-reset-admin`、状态文件不对外提供）、产品页（channel 下载卡片、折叠技术详情、固定链接、未知产品 404）、System / Light / Dark 主题控件、各 channel 按 UA 给下载链接（含认不出时不给）、固定 latest 地址（读发布指针后 302、路径不合法即 404）、下载计数（Range 不重复计数、HEAD 不计数、重启后仍在、计数文件不对外提供）、`site` 文案覆盖、上传鉴权（缺失 / 错误 / 格式错误）、产品隔离 token（本产品可写、他产品 403、前缀兄弟 403）、发布协议 preflight 与 426 门禁、指针可覆盖、超限上传被拒且不留残留、临时文件不残留、配置文件解析与未知字段拒绝、token 三种来源与优先级、`init` 不覆盖既有 token、`init -product` 合并配置且 `-token-only` 不改 json 也不轮换面板、`-product -share-with` 挂到已有 token 且不打印明文、`-list-products` 不打明文且不新建目录、`-remove` 摘条目删文件而运营方 token 与手改字段不受影响（共用文件时只摘 id）、内嵌手册存在、孤儿 GC（单版清理 / 多 channel 并集 / 坏 index 不删 / index PUT 触发）。
