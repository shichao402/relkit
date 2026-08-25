# 工具链开箱：把 relkit 接入宿主项目

面向 Agent。读完 [`README.md`](README.md) 并判定需要路径 **A** 后执行本清单。  
目标：宿主能用 `relkit` 管理版本并发 RUP 更新，无需理解全部 ADR。

## A0. 前置

```bash
relkit --version
```

失败则先安装（任选）：

```bash
go install cnb.cool/shichao402/relkit/cmd/relkit@latest
# 或从 https://cnb.cool/shichao402/relkit/-/releases 下载二进制并加入 PATH
```

自托管分发时再装 serve：

```bash
go install cnb.cool/shichao402/relkit/cmd/relkit-serve@latest
```

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
| 本机演练 / 离线 | `local` | `relkit backends` 看示例；**禁止**把 local 树当正式生产冒充上传 |
| 产物已由 CI/rsync 放到可预测 HTTP 路径 | `static-http` | 配 `stageDir` + 公网/内网 base URL |
| 自管机器 + 鉴权 PUT | `http-put` + `relkit-serve` | 先按 `relkit-serve agent-guide` 部署，再配 token 环境变量 |

改 `relkit.json` 后不要手改远端已签名文件。

## A3. 第一次端到端（建议用假产物）

准备一个小 zip（真应用可换成真实构建产物）：

```bash
relkit version set 0.1.0+1          # 或 bump
relkit stage --add dist/app-win.zip os=windows,arch=x64
# 多平台则继续 --add；正式产品通常需要 windows+macos 同版本一起 stage
relkit simulate --with-staged 0.1.0+1 --from all
relkit publish --dry-run
relkit publish                      # local / 已配好的后端
relkit verify --deep                # 对真实 HTTP 后端有意义
```

`simulate` **必须**带 `--with-staged`，否则看不到本次节点对旧客户端的影响。

## A4. 接入 CI 发布（原则，不替你写流水线细节）

1. CI **不写**版本号进业务逻辑；版本来自 tag / `VERSION.json`
2. 构建产物 → `relkit stage` →（双端齐套后）`publish`
3. 私钥只来自 CI secret；公钥已在客户端包内
4. 发版排障与红线：改读 `relkit agent-guide`，不要复制粘贴过期命令

参考实现（非规范）：SvnMergeTool 的蓝盾双 Job（一端 upload-only，一端等 peer 后统一 publish）。

## A5. 工具链开箱 Done

- [ ] `relkit version get` / `code` 正常
- [ ] `relkit.json` 已提交（无私钥）
- [ ] `.gitignore` 忽略私钥与本地 stage 垃圾
- [ ] 至少一次成功 `publish`（或 dry-run + 用户确认后端稍后配）
- [ ] 已记下 index URL 形态：`…/index/<product>/<channel>.pb`

然后去做 [`sdk-cascade.md`](sdk-cascade.md)。

## 常见阻塞

| 症状 | 处理 |
|------|------|
| `relkit` 找不到 | 安装进 PATH；Windows 可用宿主 `tools/bin/relkit.exe` 并设 `RELKIT_BIN` |
| init 后不知 product | 问用户；不要自创与现网不一致的 id |
| 只有单平台 artifact | 客户端另一平台 check 会失败；正式通道应齐套再 publish |
| 想改已发布 index | **禁止**手工编辑；发新版本或走官方 yank（若已实现） |
