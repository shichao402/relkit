# 调研：svnmergetool 内网 CI 消费面（2026-09-27）

> 依附 [ADR 0017](../adr/0017-cli-owns-consume-source-channel.md)、[迁移计划](2026-09-27-cli-owns-consume-migration-plan.md) 阶段 2a 批次前置调研。

## 证据来源

- 产品仓 `osgame-client/SvnMergeTool` master：`.relkit/onboarding.json`、`scripts/relkit.lock.json`、`scripts/toolchain.json`、`scripts/lib/toolchain.py`、`scripts/ci_release.py`、`tools/bin/README.md`
- 中央 PAC 仓 `osgame-client/bkci`：`.ci/svnmergetool.yaml`
- 实网探测：goproxy.woa.com / goproxy.cn 的 `@v/list`、`@v/v0.4.23.zip`、`@v/v0.4.23.mod` 端点

## CI 结构（PAC 中央仓定义，非产品仓内）

| Job | 环境 | 角色 |
|---|---|---|
| 构建-Windows | `windows-2016` 蓄水池机 | ci_build.bat 双平台产物 |
| 构建-MacOS | `macos-macOS15.6` Mac mini（xcode 26.3） | ci_build.sh |
| 汇总发布 / 收尾 | `docker tlinux3_ci 1.*` | ci_release.sh → `relkit_host.py release --execute` |

触发：`dev/*`、`stable/*` tag 或手动（channel 下拉）。token 走蓝盾 settings `relkit_upload_token`，与 loom 系共用（onboarding `token.isolation: share-with:loom`）。

## 当前消费形态

- lock `relkit.consume/2` pin v0.4.9（commit 49b69663），制品 URL 全指向 GitHub Release——内网 CI 跨网拉取是时延根源。
- `scripts/host` 全套 17 文件检入产品仓（事故 INC-2026-09-08 后 `tools/bin` 二进制已撤出，仅留 README）。
- 发布链：`ci_release.py` → `publish_rup_release.py --aggregate --stage-only` + `RELKIT_RELEASE_VIA_CI=1 relkit_host.py release --execute`。
- CA 链过期问题已由 `scripts/lib/ca_bundle.py`（certifi）解决，是附件下载路径的历史补丁。

## Go 可得性

- `toolchain.json` 已有 `go: 1.26.3` 条目，但产品仓全码搜索零引用、`toolchain.py` 无 `ensure_go`——空位，无消费者。
- `toolchain.py` 的 ensure_* 家族（flutter/cocoapods/git-lfs/innosetup）每次构建自装到工作区 `.toolchain/`，蓝盾云机「预置不可信」是既定前提。补 `ensure_go` 有直接先例可循，不依赖蓝盾镜像预装。
- 「蓝盾无 Go 是风险」不成立：构建机 Go 可得性从未被验证过，且存在自装路径兜底。已从迁移计划风险表降级为 2a 批次待办。

## 内网代理实测（2026-09-27，本机）

- `goproxy.woa.com/github.com/shichao402/relkit/@v/list` → 200，已收录 v0.1.0 至 v0.4.23 共 57 版（fetch 后核实：本地 master 即 v0.4.23，无版本落后；此前"落后 14 版"为误判，见结论 3）。
- `@v/v0.4.23.zip` → 200，仅 1.2MB（对比全平台附件 40+MB）。
- `@v/v0.4.23.mod` → `module go.firoyang.com/relkit`，依赖仅 protobuf v1.36.5 + x/crypto v0.55.0。
- `go.firoyang.com/relkit/@v/list` → 404（旧路径从未被收录）。
- 公网 `goproxy.cn` 同路径 200 作对照。

## 结论

1. 镜像站通道已存在且活跃（v0.4.23 已同步），唯一阻塞是 zip 内 module path 与请求路径不匹配——`go run github.com/shichao402/relkit/...@vX` 会被 Go 客户端拒绝。ADR 0017 决策 1 的 module path 迁移是解锁动作，镜像站侧无需任何操作。
2. svnmergetool 迁移动作收敛为：产品仓补 `ensure_go`（toolchain.json 条目已在）、lock 升 consume/3、CI 入口换 `relkit ci`、删检入的 scripts/host。COS 预编译 CLI 兜底通道对 svnmergetool 而言不需要。
3. 基线核对（2026-09-27 fetch 后）：本地 master = a7e79e9 = v0.4.23，与 `origin/master` 一致，**并未落后**。真实形态是 `origin/main`（66b3cc9，ADR 0016 store/console 拆分等 26 个提交）与 `origin/master`（v0.4.23 发布基线）双分支并行：master 是发布线，main 是开发线。所谓"本地落后 14 个版本"是误判——那是 goproxy 收录列表（v0.1.0…v0.4.23）与本地 checkout 的版本号差，并非 git 基线落后。阶段 0 动工前无需 `git fetch` 同步，但应明确以哪条分支为迁移起点：若以 main 为起点，需先合并 master 的 2 个发布提交（6d34b95 TLS 1.2、a7e79e9 multipart）或确认它们已被 cherry-pick 进 main。
4. 遗留待办：`origin/main` 的 `docs/_pending` 与本地未提交的迁移计划/ADR 草案是否有重叠，动工前需 diff 一次防撞车。

