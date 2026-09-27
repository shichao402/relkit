# 迁移计划：CLI 收编消费编排（ADR 0017 落地）

- 状态：Draft（依附 [ADR 0017](../adr/0017-cli-owns-consume-source-channel.md)）
- 日期：2026-09-27

## 阶段 0：地基（relkit 自身，1 周）

1. module path 迁移：`go.firoyang.com/relkit` → `github.com/shichao402/relkit`，158 个文件 import 替换，`go.mod` / facets 注册表 / README / docs 同步。调研实证（2026-09-27）：goproxy.woa.com 与 goproxy.cn 均已在 `github.com/shichao402/relkit` 路径收录至 v0.4.23，仅因 zip 内 module path 不匹配被客户端拒绝——本步骤是解锁既有通道，不是新建。
2. 打 v0.5.0 tag，观察 goproxy.cn 收录；内部 `goproxy.woa.com` 配好 GOPROXY/GOSUMDB（sum.woa.com）后确认同步。
3. 回填 ADR：0005/0007/0012 文中所有 `go.firoyang.com` 引用的指向说明（协议面不动，只改指针）。

## 阶段 1：CLI 吸收产品 CI 面（2–3 周）

按消费频率与风险分三批重写 `scripts/host` → `cmd/relkit`（每批一个 PR，conformance 测试先行）：

1. **第一批 `install` / `status` / `verify`**（consume 51KB 主体）：下载、验哈希、原子安装、版本探针。lock 升 consume/3：新增 `source` 块（module/version/H1/commit），制品块保留（updater、SDK zip）。`--version` 探针沿用，CLI 版本号由 ldflags 注入 module version。
2. **第二批 `release` / `ci` / `upgrade` / `fake`**（release.py 43KB + gates/inspect/reconcile 部分）：此时 lock 兼容读 consume/2 与 consume/3，写只出 consume/3。
3. **第三批收编 `build`**：`internal/registry` 落地（Go 单源），`relkit build --all` 产出全部 Release 附件，与 `scripts/deploy/relkit.py build --all` 字节对齐断言后切换 release.yml。
4. 全程双轨并存：Python 与 Go 版本同 release 发出，conformance 套件交叉验证，直到五产品全部切完。

## 阶段 2：五产品升级（1–2 周，与阶段 1 尾部重叠）

批次按「接入深、收益大、风险低」排序，每产品一 PR：

| 批次 | 产品 | 现状（来自 relkit 文档档案） | 迁移动作 |
|---|---|---|---|
| 2a | svnmergetool | 内网蓝盾 CI、git.woa.com、独立消费 ADR-007；PAC 定义在中央仓 osgame-client/bkci（windows-2016 池 + macos-macOS15.6 + docker tlinux3_ci 三环境） | 产品仓已有 toolchain.json `go: 1.26.3` 条目与 toolchain.py 自装机制（ensure_flutter 先例），补 `ensure_go`（goproxy.woa.com 单通道，约 1.2MB/版本）即可，无需依赖蓝盾镜像预装；lock → consume/3；入口 `relkit_host.py ci` → `relkit ci`；删除检入的 scripts/host 全套 17 文件 |
| 2a | dec | 公网产品、与 cronkit 共用发布机 profile | 同上；另因 dec 是发布机上的消费者（publish-agent.md），升级时序注意先升 CLI 后停 host-scripts 附件 |
| 2b | cronkit | 与 dec 共机、人页策略未定、尚未发版 | 随 dec 同机升级；人页策略悬而未决，不当作验收阻塞项 |
| 2b | loomeditor / loomlauncher | 接入档案不在 relkit 文档内 | 迁移前先盘接入面（lock 版本、CI 入口、SDK 依赖）；「先盘后迁」是该两仓的前置条件 |

通用动作（每产品）：CI 入口替换、lock 重写为 consume/3、删 `relkit_host.py` 调用、GOMODCACHE 进 CI 缓存键、一个发布演练（`relkit fake verify`）作为验收门。

## 阶段 3：退役（1 周）

1. 停发 host-scripts 附件（release.yml 摘掉；lock 无 hostScriptsSha256 的 consume/3 不再需要它）。
2. 删 `relkit_consume.py` 与 `hostlib` 产品 CI 面文件；`relkit_host.py` 缩为发布机运维工具。
3. `scripts/deploy/relkit.py` 的 `build` 面删除；`release.yml` 的 Test 步骤里 Python unittest 相应缩减。
4. docs 全面改版：CLI.md 增补 install/release/ci/build 章节，宿主接入文档从「下载附件」改为「go run 入口 + GOPROXY 配置」。

## 验收线

- 五产品各跑一次真实发布（svnmergetool 蓝盾、dec/cronkit 发布机、loom 两仓各自 CI）。
- 全平台 40+MB 附件下载从宿主 CI 消失（仅 updater + SDK zip 制品保留）。
- `go run github.com/shichao402/relkit/cmd/relkit@vX` 在内网（goproxy.woa.com）与公网（goproxy.cn）各验证一次。
- e2e：五产品 lock 均为 consume/3，conformance 套件全绿。

## 风险与回退

- **module path 不匹配是当前唯一阻塞**：goproxy.woa.com 已在 `github.com/shichao402/relkit` 路径收录 relkit 至 v0.4.23（zip 1.2MB、端点 200），公网 goproxy.cn 同路径 200；但 zip 内 module path 仍为 `go.firoyang.com/relkit`，`go run` 会被客户端拒绝。阶段 0 的 module path 迁移完成后此阻塞自动解除，无需任何镜像站侧操作。
- **双轨漂移**：并存期 conformance 交叉验证 + tag CI 断言两实现同 commit。
- **回退线**：任一产品迁移失败即恢复其 lock 为 consume/2 与 Python 入口（双轨并存期保留此能力），relkit 侧不删任何 Python 文件直至阶段 3 开始。
- **基线分叉（2026-09-27 fetch 核对）**：本地 master = a7e79e9 = v0.4.23，与 `origin/master`（发布线）一致，并未落后。`origin/main`（开发线，66b3cc9）领先 26 个提交（ADR 0016 store/console 拆分、GZ COS 退役、site.sinks 等）。若阶段 0 以 main 为起点，需先并入 master 的 2 个发布提交（6d34b95 TLS 1.2、a7e79e9 multipart）或确认已 cherry-pick；动工前 diff 本地未提交草案与 origin/main 的 docs/_pending，防撞车。
