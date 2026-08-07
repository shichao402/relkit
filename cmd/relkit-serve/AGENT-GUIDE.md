# relkit-serve 部署与运维手册（面向 Agent）

本文是**操作性知识的唯一来源（SSOT）**。`README.md` 解释这个服务为什么长这样，本文解释拿它怎么干活、哪里会出事。

这份文档同时内嵌在二进制里，随时可以取出：

```bash
relkit-serve agent-guide
```

因此在一台刚装好的机器上，不需要联网也不需要仓库就能读到它，且读到的一定是与当前运行的这个构建配套的版本。

---

## 0. 先确认二进制可用

```bash
relkit-serve -version
```

失败就说明还没装。**禁止**声称执行了任何 `relkit-serve` 命令、或编造它的输出。此时先确认平台再决定怎么装：

```bash
uname -sm            # 期望 Linux x86_64 或 Linux aarch64
```

从本仓库源码构建（需要 Go 1.25+；在仓库根目录执行）：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=$(git describe --tags --always)" \
  -o relkit-serve ./cmd/relkit-serve
```

或用 `./deploy/build-serve.sh` / `deploy/build-serve.ps1` 交叉编译多平台。

`CGO_ENABLED=0` 不是可选项。少了它，二进制会链接构建机的 libc，换到 musl 发行版（Alpine）或更旧的 glibc 上会直接起不来，而错误信息（`No such file or directory` 指向一个明明存在的文件）极具误导性。

---

## 1. 适用判定

**适用：** 需要在自己掌管的机器上托管 RUP 发布树；或需要一个能被 `relkit` 的 `http-put` 后端直接推送的下载端点。

**不适用：**

- 产物由仓库 CI、rsync、对象存储等别的机制送达 → 那边已经有 HTTP 下载了，`relkit` 用 `static-http` 后端即可，不需要部署本服务。
- 需要 HTTPS、限速、IP 白名单、多站点 → 在本服务前面挂一层 Nginx / Caddy 反代，而不是改本服务。反代时见 §2.6。

---

## 2. 红线

### 2.1 服务目录的写权限等于客户机的代码执行权限

这个目录里的产物会被客户端下载并**执行**。任何能往里写的人，等价于能在所有客户机上执行代码。

因此：上传 token 与私钥同级别对待；服务进程用专用非特权用户跑；`systemd` 单元里除发布目录外不给任何写权限。

签名机制在这里帮不上忙 —— 攻击者替换产物会被 manifest 里的 sha256 拦住，但他若能写目录，也就能写 `index`；他缺的只是签名私钥。所以**保住私钥是这道防线的全部**，而私钥不应该出现在这台机器上。

### 2.2 `noCache` 必须包含 `index/`

`index` 是可变指针，是一次发布的提交点。它一旦被缓存，**已经成功完成的发布在缓存过期前对客户端不可见**。现场表现是"发布完了但没人收到更新"，排查方向会被带到客户端和签名上去，而真正的原因在一个 HTTP 响应头里。

反过来，`immutable` 必须包含 `artifact/`：产物一旦发布就不再改变，不给永久缓存意味着每次检查更新都重传真正大的那部分。

`relkit-serve init` 生成的配置已经填好这两项。**禁止**为了"简化配置"把它们清空。

### 2.3 token 不能出现在命令行

服务端刻意没有提供直接传 token 的参数。若你打算通过任何方式绕过这一点：`ps` 的输出对机器上所有用户可见，并且命令会进入 shell 历史。

token 只走三条路：配置文件的 `uploadToken`（文件须 0600）、`uploadTokenFile` 指向的文件（0600）、环境变量 `RELKIT_SERVE_TOKEN`。

含 token 的文件权限过宽时服务会在启动日志里打 `WARNING`。**看到这条警告要处理，不要忽略。**

### 2.4 服务目录里只能放发布树

根路径 `/` 与各子目录会返回 HTML 目录列表，方便运维查看已发布内容。同时**任何路径都能被直接取走**。目录里若有备份、构建日志、密钥、`.git`，打开列表或猜到名字就能拿到。

不要把发布目录设在家目录、项目工作区或已有内容的共享目录上。用一个专用目录（约定 `/srv/releases`）。

### 2.5 不要给 `http.Server` 设 `WriteTimeout`

给改代码的人：它作用于整个响应。一个 200 MB 的包在慢链路上要传几分钟 —— 任何大到不会误杀它的值都已经大到保护不了任何东西，任何小到能起保护作用的值都会把正常下载截断在中途。慢速攻击由 `ReadHeaderTimeout` 与 `IdleTimeout` 处理。

同理，**不要为了统计字节数去包装 `ResponseWriter`**。`net/http` 只在 `ResponseWriter` 实现了 `io.ReaderFrom` 时走 `sendfile(2)`；包装它会静默地把每次下载的每个字节绕回用户态，而且没有任何测试会发现，因为传输内容仍然正确。现有的 `recorder` 转发了 `ReadFrom`。

### 2.6 挂反代时必须透传 `Cache-Control`

反代或 CDN 若用自己的策略覆盖了上游的缓存头，§2.2 的分流就失效了，而症状与 §2.2 完全相同。挂完反代后必须复验：

```bash
curl -sI https://dl.example.com/index/app/stable.json | grep -i cache-control
# 期望 no-cache
```

### 2.7 孤儿清理不删「仍被 index 引用」的对象

默认开启 GC：定时（默认 1h）扫一次，并且每次成功 `PUT index/...` 后异步再扫。规则是：

1. 读全部 `index/<product>/<channel>.json`（解码签名信封拿 payload，**不验签、不改写**）；
2. 收集仍指向本机的 `manifest/` / `artifact/` 路径（所有 channel 取并集）；
3. 删掉不在集合里的旧文件与空目录。

安全阀：没有可读 index、全部解析失败、或解析后没有本机引用时，**整轮不删任何东西**，只打 `gc: aborted` 日志。

因此「磁盘上只留最新版」取决于发布侧 PUT 的 index 是否只含当前节点（`relkit.json` 的 `retainVersions`）。index 里还挂着历史节点时，那些对象会被保留 —— 不要指望本服务去剪裁已签名的 index。业务侧只 PUT，不要也不需要 HTTP DELETE。

启动日志会写 `gc enabled (interval ...)` 或 `gc disabled`。关掉：`-gc=false`，或配置 `"gc": {"enabled": false}` / `"interval": "0"`。

---

## 3. 流程 A：全新部署一台

```
- [ ] 1. 建用户与目录
- [ ] 2. 装二进制
- [ ] 3. init 生成配置与 token
- [ ] 4. 装 systemd 单元并启动
- [ ] 5. 自检五项
- [ ] 6. 把 token 交给发布方
```

`deploy/install.sh` 把 1–4 步做完并跑第 5 步。**优先用它**，手工步骤仅在脚本不适用时参考（非 systemd 系统、容器内、无 root）：

```bash
sudo ./deploy/install.sh --binary ./dist/relkit-serve-linux-amd64
```

脚本是幂等的：重复执行不会重新生成 token，也不会覆盖已有配置。要强制换 token 见 §5。

### 手工步骤

```bash
# 1
sudo useradd -r -s /usr/sbin/nologin relkit
sudo install -d -o relkit -g relkit -m 0755 /srv/releases

# 2
sudo install -m 0755 relkit-serve-linux-amd64 /usr/local/bin/relkit-serve

# 3
sudo relkit-serve init -dir /srv/releases -out /etc/relkit-serve
sudo chown -R relkit:relkit /etc/relkit-serve

# 4
sudo cp deploy/relkit-serve.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now relkit-serve
```

`init` 会打印一行 `export RELKIT_UPLOAD_TOKEN=...`。这是**唯一一次**能看到 token 明文的机会 —— 服务端只保存它的 sha256，事后无法反查。当场交给发布方，见 §4。

### 第 5 步：自检五项

每一项都独立排除一类故障，全过才算部署完成。

```bash
BASE=http://localhost:8080

# 1) 活着
curl -fsS $BASE/-/health && echo OK

# 2) 上传端点开着，且鉴权生效（无 token 必须 401）
curl -s -o /dev/null -w '%{http_code}\n' -X PUT --data-binary 'x' $BASE/probe.txt
# 期望 401。得到 405 说明服务是只读的（没读到 token，回看启动日志）

# 3) 带 token 能写
curl -fsS -X PUT -H "Authorization: Bearer $TOKEN" --data-binary 'x' $BASE/probe.txt

# 4) Range 生效 —— 客户端并发下载依赖它
curl -s -o /dev/null -w '%{http_code}\n' -r 0-0 $BASE/probe.txt
# 期望 206。得到 200 说明 Range 没生效，客户端会退化成单线程

# 5) 根目录可浏览
curl -s -o /dev/null -w '%{http_code}\n' $BASE/
# 期望 200，正文是 HTML 目录列表

curl -fsS -X DELETE -H "Authorization: Bearer $TOKEN" $BASE/probe.txt 2>/dev/null || \
  sudo rm -f /srv/releases/probe.txt   # 服务不支持 DELETE，探针文件手工清掉
```

第 4 项容易被跳过，但它是唯一能证明"多线程下载真的可用"的检查。经过反代之后尤其要重测：有些反代配置会吃掉 `Range`。

---

## 4. 流程 B：让 relkit 推到这台机器

在**发布方**（不是服务器）的项目里配一个 `http-put` 后端：

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

`tokenEnv` 写的是**环境变量名**，不是 token 本身。这个文件要进版本控制。

```bash
export RELKIT_UPLOAD_TOKEN='<init 打印的那个>'
relkit publish 1.0.0
relkit verify --deep
```

下载与上传地址可以不同，而且这是常见情形而非特例 —— 下载走公网域名或 CDN，上传走内网：

```json
{
  "type": "http-put",
  "baseUrl": "https://dl.example.com/releases/",
  "uploadUrl": "http://10.0.0.5:8080/",
  "tokenEnv": "RELKIT_UPLOAD_TOKEN"
}
```

`baseUrl` 必须与客户端实际访问的地址一致，因为它会被写进 manifest 的 `urls` 并被签名。写错了要重新发布才能改。配完先跑 `relkit verify --deep`，它会逐个产物发 HEAD，确认那些 URL 真的能匿名取到。

---

## 5. 流程 C：轮换 token

```bash
sudo ./deploy/install.sh --binary /usr/local/bin/relkit-serve --rotate-token
```

或者手工：

```bash
sudo relkit-serve init -out /etc/relkit-serve -token-only
sudo chown -R relkit:relkit /etc/relkit-serve
sudo systemctl restart relkit-serve
```

**用 `-token-only`，不要用 `-force`。** `-force` 会连配置文件一起重新生成，把手工改过的监听地址、缓存前缀等一并静默恢复成默认值 —— 而恢复了的缓存前缀唯一的症状是「发布晚了几分钟才生效」（§2.2）。

要点：

- 旧 token 在服务重启的那一刻失效。**先把新 token 交给所有发布方再重启**，否则他们的发布会失败。
- 失败是安全的：`relkit publish` 上传产物时就会拿到 401 而中止，此时 `index` 指针尚未移动，客户端看到的仍是上一个版本。重跑即可，不会留下半个发布。
- 只读的下载不受影响，客户端不需要任何凭据。

不想动配置文件时，也可以用环境变量临时覆盖（优先级：命令行 > 环境变量 > 配置文件）：

```ini
# /etc/systemd/system/relkit-serve.service.d/token.conf
[Service]
Environment=RELKIT_SERVE_TOKEN=<新 token>
```

---

## 6. 配置参考

配置文件缺省按 `./relkit-serve.json`、`/etc/relkit-serve.json` 的顺序查找，可用 `-config` 指定。**启动日志总会打印实际用了哪个文件（或"none"）**，所以不会出现"意外读到了别的配置"这种情况。

**未知字段一律报错**，不会被忽略。拼错一个键名而静默沿用默认值是最难查的一类配置故障：服务照常启动、报告成功，行为却与配置文件写的不一样 —— 尤其是缓存前缀，唯一症状会是"发布晚了几分钟才生效"。

| 字段 | 对应参数 | 默认 | 说明 |
|---|---|---|---|
| `addr` | `-addr` | `:8080` | 监听地址 |
| `dir` | `-dir` | `.` | 对外提供的目录 |
| `uploadToken` | 无 | 无 | token 明文；文件须 0600 |
| `uploadTokenFile` | `-token-file` | 无 | token 文件路径，相对路径按配置文件所在目录解析 |
| `maxUpload` | `-max-upload` | `4GiB` | 单次上传上限，接受 `512MiB` 这类写法 |
| `cache.noCache` | `-nocache` | `["index/"]` | 这些前缀按 no-cache 返回，见 §2.2 |
| `cache.immutable` | `-immutable` | `["manifest/","artifact/"]` | 这些前缀按永久缓存返回 |
| `cache.defaultMaxAge` | `-default-max-age` | `60` | 两个列表都不命中时的 `max-age` |
| `gc.enabled` | `-gc` | `true` | 是否启用孤儿 `manifest/` / `artifact/` 清理，见 §2.7 |
| `gc.interval` | `-gc-interval` | `1h` | 定时扫盘间隔；`0` 与 `enabled=false` 一样关闭 GC |
| `shutdownTimeout` | `-shutdown-timeout` | `30s` | 收到停止信号后留给进行中下载的时间 |
| `logRequests` | `-quiet` | `true` | 是否打印请求日志 |

`uploadToken` 与 `uploadTokenFile` 只能给一个，同时给会启动失败。都不给则服务只读，`PUT` 返回 405 —— 这是默认状态，也就是说默认安全。

优先级是**命令行 > 环境变量 > 配置文件**。环境变量放在中间，是为了让容器或 systemd drop-in 能轮换 token 而不必改一个可能由配置管理系统托管的文件。

---

## 7. 流程 D：排查

### 服务起不来

看 `journalctl -u relkit-serve -n 50`。四类原因，报错都是明确的：

| 报错 | 原因 |
|---|---|
| `unknown field "..."` | 配置文件键名拼错 |
| `cannot serve ...: no such file` | `dir` 不存在，先建目录 |
| `cannot listen on ...: address already in use` | 端口被占，`ss -ltnp \| grep 8080` |
| `uploadTokenFile: permission denied` | 服务用户读不到 token 文件，检查 owner |

### `PUT` 返回 405

服务处于只读状态，即没读到 token。看启动日志那行：`read-only: no token supplied` 还是 `PUT uploads enabled`。按 §6 检查三个来源，最常见的是 systemd 里没把配置文件路径传对，于是它读了 `/etc/relkit-serve.json` 而 token 在 `/etc/relkit-serve/relkit-serve.json`。

### `PUT` 返回 401

发布方的 token 与服务端不一致。服务端只存 sha256，**无法反查明文**，所以不要试图"确认服务端 token 是什么"，直接按 §5 重设。

也确认发布方那侧环境变量真的传进去了 —— `relkit` 读的是 `relkit.json` 里 `tokenEnv` 指定的那个名字，不是固定的 `RELKIT_UPLOAD_TOKEN`。

### 文件明明在，却 404

1. 看启动日志的 `serving <path> on ...`，确认服务目录是你以为的那个。
2. 大小写：Linux 文件系统区分大小写，而 manifest 里的 key 由 `relkit` 生成，两边必须一致。
3. 路径里含 `..` 或以 `.` 开头的段会被直接拒为 404，这是有意的。

### 下载慢

先分清是并发问题还是带宽问题。请求日志里带 `Range` 表示客户端在并发下载：

```bash
journalctl -u relkit-serve -n 200 | grep -i range
```

一条 `Range` 都没有，说明客户端在单线程下载（或反代吃掉了 `Range`，见 §3 自检第 4 项）。都有 `Range` 但仍然慢，那瓶颈在网卡或磁盘，不在本服务 —— 服务端在 256 并发下仍有万兆网卡三倍以上的余量，可用 `go test -bench` 在本机复现基线。

### 客户端收不到新版本

这一步之前先确认问题真的在服务端：

```bash
curl -sI $BASE/index/<product>/<channel>.json | grep -iE 'cache-control|last-modified'
```

`Cache-Control` 不是 `no-cache` → 问题在 §2.2 或 §2.6。是 `no-cache` 且 `Last-Modified` 是刚才 → 服务端已经正确了，转到 `relkit` 侧的 `AGENT-GUIDE.md` §8 继续排查。

---

## 8. 禁止行为

- 声称执行了 `relkit-serve` 命令而实际上二进制不可用。
- 用命令行参数或任何进版本控制的文件传 token。
- 清空 `cache.noCache` 里的 `index/`，或清空 `cache.immutable` 里的 `artifact/`。
- 以 root 运行服务进程。
- 把发布目录设在含有其他内容的目录上。
- 忽略启动日志里的 `WARNING`（目前只有一种：含 token 的文件权限过宽）。
- 给 `http.Server` 设 `WriteTimeout`，或包装 `ResponseWriter` 而不转发 `ReadFrom`。
- 在服务器上放签名私钥。这台机器的定位是分发，不是签名。
- 为了「腾空间」手工乱删仍被某 channel index 引用的 `manifest/` / `artifact/`；交给 GC，或先改发布侧 index 再让 GC 扫。
- 给业务侧开 HTTP DELETE，或让发布流程自己判断删哪个版本目录。

---

## 9. 汇报要求

部署完成后至少说明：

- 版本（`relkit-serve -version`）、监听地址、服务目录；
- 实际生效的配置文件路径（照抄启动日志那一行）；
- 上传端点是开还是关；
- GC 是开还是关（照抄启动日志）；
- §3 自检五项的结果；
- token 交付给了谁、通过什么渠道（**不要**在汇报里带上 token 本身）；
- 若挂了反代：`Cache-Control` 透传的复验结果。
