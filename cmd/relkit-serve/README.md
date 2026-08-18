# relkit-serve

> 本目录是 [`cnb.cool/shichao402/relkit`](https://cnb.cool/shichao402/relkit) 仓库的一部分（`cmd/relkit-serve`），与发布 CLI 同仓。部署脚本在仓库根的 `deploy/`。

一个静态文件服务，用来托管 [RUP](https://cnb.cool/shichao402/relkit/blob/main/SPEC.md) 发布树。单个静态链接的可执行文件，无运行时依赖。

做三件事：

1. **对外提供下载**，正确支持 Range 请求，因此客户端可以多线程并发下载。
2. **可选地接受带鉴权的 PUT 上传**，因此 `relkit` 能直接把一次发布推送上来。
3. **按 index 引用清理孤儿**，删掉已不被任何 channel 的 index 引用的旧 `manifest/` / `artifact/`，避免历史整包永久占盘。

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

**一、需要一个上传端点。** 让 `relkit` 一条命令完成发布，而不是发布完再手工 rsync。Caddy 没有内置的写入端点，加插件要重新编译。

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
| `relkit-serve init` | 生成配置骨架与随机 token |
| `relkit-serve agent-guide` | 打印内嵌的部署运维手册 |
| `relkit-serve -version` | 版本 |

手册内嵌在二进制里，因此在一台刚装好的机器上不联网也能读到，且读到的一定与当前运行的构建配套。

## 配置

字段与参数的完整对照表在 [`AGENT-GUIDE.md`](AGENT-GUIDE.md) §6，示例在 `deploy/relkit-serve.example.json`。要点：

- 缺省按 `./relkit-serve.json`、`/etc/relkit-serve.json` 顺序查找，可用 `-config` 指定；**启动日志总会打印实际用了哪个文件**。
- **未知字段一律报错。** 拼错键名而静默沿用默认值是最难查的一类配置故障 —— 服务照常启动、报告成功，行为却与配置文件写的不一样。
- 优先级是**命令行 > 环境变量 > 配置文件**。环境变量在中间，是为了让容器或 systemd drop-in 能轮换 token 而不必改一个可能由配置管理系统托管的文件。
- token 三条路：配置文件里的 `uploadToken`、`uploadTokenFile` 指向的文件、环境变量 `RELKIT_SERVE_TOKEN`。**不支持用命令行参数传 token**：那样它会出现在 `ps` 的输出里，机器上任何用户都能看到，并且会进入 shell 历史。含 token 的文件权限过宽时启动日志会打 `WARNING`。

## 日志

一行一个请求，含客户端 IP、方法、路径、状态码、字节数、耗时，以及 `Range` 头（如有）。

记录 `Range` 是有意的：它是「这个客户端在并发下载」的直接信号。有人报告下载慢时，第一件要看的事就是慢的那些请求里有没有 `Range`。

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
export RELKIT_UPLOAD_TOKEN='<init 打印的那个>'
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

**目录列表对运维开放。** `GET /` 与各子目录返回简单的 HTML 索引，方便在浏览器里查看已发布内容。协议客户端仍只信任签名过的 index；列表不是信任边界。因此发布目录里仍然不要放别的东西：列表会把它们暴露出来，且任何路径都能被按名取走。

---

## 测试

```bash
go test ./...                              # 功能
go test -run "^$" -bench . -benchtime 2s   # 吞吐
```

覆盖：Range 切片正确性、16 并发下载拼回原文件、HEAD 只返回大小、缓存头按前缀分流、路径遍历被拒、目录列表、上传鉴权（缺失 / 错误 / 格式错误）、指针可覆盖、超限上传被拒且不留残留、临时文件不残留、配置文件解析与未知字段拒绝、token 三种来源与优先级、`init` 不覆盖既有 token、内嵌手册存在、孤儿 GC（单版清理 / 多 channel 并集 / 坏 index 不删 / index PUT 触发）。
