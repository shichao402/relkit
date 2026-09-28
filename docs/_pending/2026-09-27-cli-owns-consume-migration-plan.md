# 迁移计划：CLI 收编消费编排（ADR 0017 落地）

- 状态：Draft（依附 [ADR 0017](../adr/0017-cli-owns-consume-source-channel.md)）
- 日期：2026-09-27（2026-09-28 更新：阶段 0 进度、内外网统一入口 `mirrors.tencent.com/go/` 定稿并经 v0.4.24 端到端验证、updater 通道化落点）

## 阶段 0：地基（relkit 自身，1 周）

1. module path 迁移：**已完成**（main `ac9860b`，master `edbbf1c` 合入；`go.mod` 已为 `github.com/shichao402/relkit`，158 文件 import 替换随提交落地）。历史调研（2026-09-27）作证：goproxy.woa.com 与 goproxy.cn 均已在该路径收录至 v0.4.23，此前唯一阻塞即 zip 内 module path 不匹配，本步骤是解锁既有通道，不是新建。
2. 打 v0.5.0 tag（CLI 收编首版；打前按 ADR 0004 把项目版本 SSOT 推到 0.5.0），随后统一入口实测：`GOPROXY=https://mirrors.tencent.com/go/`（内外网边缘均 302 → `goproxy.woa.com`，split-DNS 双宿，内外网 CI 共用同一配置）下跑 `go run github.com/shichao402/relkit/cmd/relkit@vX` 与 `go install github.com/shichao402/relkit/cmd/relkit-updater@vX`。v0.4.24 已用该入口端到端验证成功（2026-09-28，sumdb 透传校验通过，`--version` 正常）。fallback：直连 `goproxy.woa.com`；备源 `goproxy.cn` / `goproxy.io`（新路径均 200）。注意 v0.4.23 及更早 zip 在新路径下 module path 不匹配属预期（tag 早于迁移提交），客户端拒绝不影响 v0.4.24+。
3. 回填 ADR：0005/0007/0012 文中所有 `go.firoyang.com` 引用的指向说明（协议面不动，只改指针）。

## 阶段 1：CLI 吸收产品 CI 面（2–3 周）

按消费频率与风险分三批重写 `scripts/host` → `cmd/relkit`（每批一个 PR，conformance 测试先行）：

1. **第一批 `install` / `status` / `verify`**（consume 51KB 主体）：下载、验哈希、原子安装、版本探针。lock 升 consume/3：新增 `source` 块（module/version/H1/commit），制品块仅剩 dart/node/rust SDK zip 与预编译 CLI 兜底（updater 退出制品块，完整性由 H1 + sumdb 覆盖）。制品块 URL 为列表（GitHub Release 主、CNB 附件备）；CNB 侧 build 面需扩至制品块全量（当前 `.cnb.yml` 仅产 store/agent/cli/updater/dart-sdk）。`--version` 探针沿用，CLI 版本号解析链为 ldflags 注入（Release 制品）→ `debug.ReadBuildInfo` module version（module 通道，`v` 前缀归一化）→ `devel` 占位，由 `internal/buildver` 统一实现（CLI 与 updater 共用，两通道都出真实版本）。
2. **第二批 `release` / `ci` / `upgrade` / `fake`**（release.py 43KB + gates/inspect/reconcile 部分）：此时 lock 兼容读 consume/2 与 consume/3，写只出 consume/3。
3. **第三批收编 `build`**：`internal/registry` 落地（Go 单源），`relkit build --all` 产出全部 Release 附件，与 `scripts/deploy/relkit.py build --all` 字节对齐断言后切换 release.yml。updater 源码构建以 `-trimpath` + toolchain 钉死（go 1.26.3）保证可复现；release CI 镜像与 `ensure_go` 同 patch 才能断言 byte-equal，否则 envelope 只锁 H1、二进制哈希作可选交叉验证。
4. 全程双轨并存：Python 与 Go 版本同 release 发出，conformance 套件交叉验证，直到五产品全部切完。

## 阶段 2：五产品升级（1–2 周，与阶段 1 尾部重叠）

批次按「接入深、收益大、风险低」排序，每产品一 PR：

| 批次 | 产品 | 现状（来自 relkit 文档档案） | 迁移动作 |
|---|---|---|---|
| 2a | svnmergetool | 内网蓝盾 CI、git.woa.com、独立消费 ADR-007；PAC 定义在中央仓 osgame-client/bkci（windows-2016 池 + macos-macOS15.6 + docker tlinux3_ci 三环境） | 产品仓已有 toolchain.json `go: 1.26.3` 条目与 toolchain.py 自装机制（ensure_flutter 先例），补 `ensure_go`（`GOPROXY=https://mirrors.tencent.com/go/`，约 1.2MB/版本）即可，无需依赖蓝盾镜像预装；lock → consume/3；入口 `relkit_host.py ci` → `relkit ci`；删除检入的 scripts/host 全套 17 文件；updater 一并走 module 通道，蓝盾三环境跨网制品仅剩 dart SDK zip（GitHub 主 / CNB 备） |
| 2a | dec | 公网产品、与 cronkit 共用发布机 profile | 同上；dec 也在腾讯生态内，`mirrors.tencent.com/go/` 对公网边缘同样 302 → `goproxy.woa.com` 双宿（公网 IP 175.27.22.2 直连 200），内外网同一句配置；updater 同通道，跨网制品仅剩 SDK zip；另因 dec 是发布机上的消费者（publish-agent.md），升级时序注意先升 CLI 后停 host-scripts 附件 |
| 2b | cronkit | 与 dec 共机、人页策略未定、尚未发版 | 随 dec 同机升级；人页策略悬而未决，不当作验收阻塞项 |
| 2b | loomeditor / loomlauncher | 接入档案不在 relkit 文档内 | 迁移前先盘接入面（lock 版本、CI 入口、SDK 依赖）；「先盘后迁」是该两仓的前置条件 |

通用动作（每产品）：CI 入口替换、lock 重写为 consume/3、删 `relkit_host.py` 调用、GOMODCACHE 进 CI 缓存键（go.sum 哈希）、`GOPROXY=https://mirrors.tencent.com/go/`（内外网统一入口，302 → goproxy.woa.com 双宿；fallback 直连 goproxy.woa.com，备源 goproxy.cn / goproxy.io）、一个发布演练（`relkit fake verify`）作为验收门。

## 阶段 3：退役（1 周）

1. 停发 host-scripts 附件（release.yml 摘掉；lock 无 hostScriptsSha256 的 consume/3 不再需要它）。updater 附件在五产品全部切至 consume/3 后停发（`.cnb.yml` build 面同步收缩为 store/agent/cli/SDK）。
2. 删 `relkit_consume.py` 与 `hostlib` 产品 CI 面文件；`relkit_host.py` 缩为发布机运维工具。
3. `scripts/deploy/relkit.py` 的 `build` 面删除；`release.yml` 的 Test 步骤里 Python unittest 相应缩减。
4. docs 全面改版：CLI.md 增补 install/release/ci/build 章节，宿主接入文档从「下载附件」改为「go run 入口 + GOPROXY 配置（内外网统一 `https://mirrors.tencent.com/go/`，备源 goproxy.cn / goproxy.io）」。

## 验收线

- 五产品各跑一次真实发布（svnmergetool 蓝盾、dec/cronkit 发布机、loom 两仓各自 CI）。
- 全平台 40+MB 附件下载从宿主 CI 消失；跨网制品仅剩 SDK zip（GitHub 主 / CNB 备），updater 附件停发。
- `go run github.com/shichao402/relkit/cmd/relkit@vX` 与 `go install github.com/shichao402/relkit/cmd/relkit-updater@vX` 在统一入口 `mirrors.tencent.com/go/` 下验证（v0.4.24 已于 2026-09-28 在本机跑通，v0.5.0 复测一次收尾）。
- e2e：五产品 lock 均为 consume/3，conformance 套件全绿。

## 风险与回退

- **module path 阻塞已解除且消费侧通道已打通**（2026-09-28 核对）：迁移已合入 master/main，v0.4.24 tag 已发（`edbbf1c`），镜像 `.mod` 已是新 module path，统一入口下 `go run@v0.4.24` 端到端成功。v0.4.23 及更早版本在新路径下不可 `go run` 属预期，无需镜像站侧操作。
- **CNB 附件可达性未实测**：SDK zip 与预编译 CLI 兜底的备援通道依赖蓝盾构建机直连 CNB 附件域名，动工前实测一次；不可达则兜底退化为 GitHub 单源（锁 sha256 不变）。
- **byte-equal 断言条件**：release CI 的 golang 镜像 tag 是浮动 patch，与 `ensure_go` 1.26.3 不同 patch 时 Release 附件与源码构建字节不同——此时 envelope 只锁 H1，二进制哈希作可选交叉验证。
- **双轨漂移**：并存期 conformance 交叉验证 + tag CI 断言两实现同 commit。
- **回退线**：任一产品迁移失败即恢复其 lock 为 consume/2 与 Python 入口（双轨并存期保留此能力），relkit 侧不删任何 Python 文件直至阶段 3 开始。
- **基线已收敛（2026-09-28 核对）**：本地 master = `edbbf1c` = origin/master（合入 main `ac9860b` 的 module path 迁移，含发布线 2 提交），工作区干净（此前 4 处未提交改动与 3 份草案已入库），stash 2 条（`wip-dec-skills`、`6bacd0a` WIP）保留待处理。v0.5.0 以收编本计划与 ADR 0017 修订后的 master 顶端为基线打 tag。
