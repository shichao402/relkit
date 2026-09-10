# 工具链开箱：把 relkit 接入宿主项目

面向 Agent。读完 [`README.md`](README.md) 并判定需要路径 **A** 后执行本清单。  
目标：宿主能用 `relkit` 管理版本并发 RUP 更新，无需理解全部 ADR。

## A0. 前置

```bash
relkit --version
```

失败则先安装（任选）：从 [Releases](https://github.com/shichao402/relkit/releases) 下载二进制并加入 PATH，或 clone 后 `go build -o relkit ./cmd/relkit`。不要 `go install go.firoyang.com/relkit/...`（模块名没有网络解析）。

自托管分发时再编 serve：`go build -o relkit-serve ./cmd/relkit-serve`。

确认工作目录是**宿主应用仓库根**（将出现 `relkit.json` 的地方），不是随便一个子目录。

## A1. 初始化项目元数据

```bash
cd /path/to/host
relkit init --product <product-id>
relkit version get          # 应能打印 x.y.z+build
```

约定：

- `product`：小写、稳定、与客户端硬编码一致（改名等于断更）
- 默认 channel 多为 `stable`
- 默认 `codeStrategy: version-build` → `code` = `VERSION.json` 里 `+` 后的整数

需要新密钥时：

```bash
relkit keygen --key-id <key-id> --out keys --update-config
```

- 私钥目录 `keys/`：**加入 .gitignore**，只进 CI 密钥或本机权限受限路径
- 公钥会进配置；客户端还必须**再内嵌一份**（见 SDK 开箱）

## A2. 选后端（只选一条主路径）

| 场景 | 后端 | 下一步 |
|------|------|--------|
| COS / S3 / MinIO | `s3-compatible` | 配长期钥环境变量；agent 只生成 query 预签名 URL |
| 自管 relkit-serve / 本机演练 | `relkit-compatible` | 配 `baseUrl`、可选 `uploadUrl`、必填 `tokenEnv`、可选 `timeoutSeconds` |
| 外部系统已放好对象 / 只读镜像 | `static-http` | 配绝对 `baseUrl`；不可作 ingest |

本机演练：

```bash
export RELKIT_SERVE_TOKEN='<relkit-serve init 输出的运营方 token>'
relkit-serve -dir ./dist -addr 127.0.0.1:30341
```

`local` / `http-put` 已删除。`casCredentials` 只过渡接受并忽略；新配置必须删除。

## A3. 第一次端到端（建议用假产物）

准备一个小 zip（真应用可换成真实构建产物）：

```bash
relkit version set 0.1.0+1          # 或 bump
relkit stage --add dist/app-win.zip os=windows,arch=x64
# 多平台则继续 --add；正式产品通常需要 windows+macos 同版本一起 stage
relkit simulate --with-staged 0.1.0+1 --from all
relkit publish --dry-run
relkit publish                      # 指向本机 serve 或已配好的后端
relkit verify --deep                # 对真实 HTTP 后端有意义
```

`simulate` **必须**带 `--with-staged`，否则看不到本次节点对旧客户端的影响。

## A4. 接入 CI 发布（原则，不替你写流水线细节）

生产推荐：CI 只 `relkit stage`，把 staged 树交给发布机上的 `relkit-agent` 持钥 `publish`。细节见 `cmd/relkit-agent/README.md`。

1. CI **不写**版本号进业务逻辑；版本来自 tag / `VERSION.json`
2. 构建产物 → `relkit stage`（会写出 `release-policy.json`）→ 上传 staged → 发布机 `publish`
3. **私钥、COS 密钥不进 CI、不进 staged 包。** 仓库 `relkit.json` 只给 stage 抽策略；机器侧 `publishTo` / 密钥 env 名在 `/etc/relkit-agent/products/<id>.json`
4. **禁止**把完整 `relkit.json` scp/覆盖到 `/srv/relkit/<id>/`。agent 不读产品根那份；覆盖只会冲掉本机密钥引用
5. 宿主仓必须使用 `relkit.consume/2` lock 钉 Release、commit、每个附件 URL 与
   SHA-256，并运行逐字节复制的 `scripts/relkit_consume.py install --target host`。
   禁止 sparse/vendor 源码、现场 Go 构建、浮动分支以及 PATH/LFS 二进制回退
6. 发版排障与红线：改读 `relkit agent-guide`，不要复制粘贴过期命令
7. **产物哈希要稳**：不要把每次不同的 `BuildTime` / 随机 seed 打进二进制。relkit 只认 sha256；哈希漂了，agent CAS 跳过上传和客户端跳过下载都打不中
8. **发布 CAS 不用宿主再实现**：agent 返回绝对 URL `requests[]`；客户端只执行请求，不实现 STS / SigV4 / `sign`

参考实现（非规范）：SvnMergeTool 的蓝盾双 Job（一端 upload-only，一端等 peer 后统一 publish）。

## A5. 工具链开箱 Done

- [ ] `relkit version get` / `code` 正常
- [ ] `relkit.json` 已提交（无私钥）
- [ ] `.gitignore` 忽略私钥与本地 stage 垃圾
- [ ] 至少一次成功 `publish`（或 dry-run + 用户确认后端稍后配）
- [ ] 已记下 index URL 形态：`…/index/<product>/<channel>.pb`
- [ ] `relkit.json` 含 `recovery`（至少两个官方手动入口）且已 `relkit onboard run repo.recovery-embed` 与 `repo.client-contract`
- [ ] relkit SDK/CLI/updater 来自同一份 `relkit.consume/2` lock；已
  `relkit onboard ack human.consume-lock-v2`
- [ ] 发版 ldflags 不注入每次不同的 `BuildTime`；已 `ack human.stable-artifact-hash`
- [ ] SDK `Download` 的 dest 对准已装路径（或已 ack N/A）；已 `ack human.download-dest-installed`

然后去做 [`sdk-cascade.md`](sdk-cascade.md)。

## 常见阻塞

| 症状 | 处理 |
|------|------|
| `relkit` 找不到 | 在宿主根运行 `python scripts/relkit_consume.py install --target host`；禁止从 PATH 或现场源码构建兜底 |
| init 后不知 product | 问用户；不要自创与现网不一致的 id |
| 只有单平台 artifact | 客户端另一平台 check 会失败；正式通道应齐套再 publish |
| 想改已发布 index | **禁止**手工编辑；发新版本或走官方 yank（若已实现） |
| CI 绿、SDK 已更新、网页还是旧版 | 协议面（`updates`/`raw`）和人页（`update` / Makers）不是一条通道。人页还要 profile 里 `makers.tokenEnv` 对应的变量已进 **正在跑的** agent（改 `/etc/relkit-agent/env` 后必须 restart）。缺 token 时 publish 仍可能 200，人页静默跳过 |
| `publish` 抱怨没有 `release-policy.json` / profile | 升级 CLI 再 `stage`；发布机必须有 `/etc/relkit-agent/products/<id>.json`，且 `product`/`signing.keyId` 与仓库一致 |
| 新产品 / 第二产品发版 | 每个产品自己的 profile 和 `keyId`。别用另一个产品刚发成功代替验收 |
