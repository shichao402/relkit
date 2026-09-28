# ADR 0017: 消费编排收进 CLI，模块源码通道取代 scripts/host

- 状态：Proposed
- 日期：2026-09-27
- 更新：2026-09-28（决策 7 updater 改走 module 通道；决策 4 兜底从 COS 改 CNB 附件，再改 GitHub 单源——`.cnb.yml` 发布面删除，CNB 定位收缩为每日同步镜像；内外网 goproxy 统一入口 `mirrors.tencent.com/go/` 实测定稿，v0.4.24 已发并端到端验证）
- 取代：[ADR 0011](0011-immutable-release-consume.md) 决策 1 中「scripts/host Release 附件」部分、决策 3、决策 4、决策 6；决策 2 的 lock schema 由 `relkit.consume/2` 演进为 `relkit.consume/3`。决策 5（consume/1 删除）保持历史事实。
- 修订：[ADR 0010](0010-single-updater-engine.md) 决策 7「consume 同一 SHA 产出 facade 与二进制」改述为「同一 commit」；[ADR 0012](0012-single-component-registry.md) 的注册表载体由 `facets.py` 改为 Go。另：`scripts/deploy` 发布层自始未入任何 ADR，本 ADR 一并收编其 `build` 面。
- 关联：[ADR 0007](0007-entry-mirror-must-be-reachable-and-cacheable.md)、[ADR 0009](0009-publisher-protocol-negotiation.md)

## 背景

ADR 0011 禁止宿主从源码消费，当时前提有二：制品下载不是瓶颈；源码消费面复杂且刚出过 sparse checkout「命令成功但工作树缺文件」事故。两个前提均已反转：

1. GitHub Release 制品成为宿主 CI 的主要时延（单平台约 21MB、全平台 40+MB、跨国链路），且 relkit 是多数流水线里唯一依赖 GitHub 可达性的组件。
2. relkit 源码消费面已收敛：直接依赖仅 protobuf 与 x/crypto，tag 即合法 Go module。但 Python 面实际有两层：产品 CI 面约 330KB（`scripts/host`：consume 51KB + hostlib 约 240KB + relkit_host 18KB），以及此前未被任何 ADR 记录的 `scripts/deploy` 发布层约 100KB（`relkit.py` 56KB + `relkit_ops.py` 14KB）。relkit 自己的 release CI 也在用 `python scripts/deploy/relkit.py build --all` 产出全部 Release 附件——即「消费编排 Python」与「发布编排 Python」双轨并存，Go CLI 与它们构成约 600KB 级的双语言维护面。

2026-09-28 增补：updater 单平台附件 8MB 级、每次消费升级都跨网拉取，是制品时延里最重的一块；而 module zip 仅 1.2MB 且内外网镜像均已收录。此前「updater 源码构建不可复现、必须走制品」的判断，前提是存在免费的制品镜像兜底——该前提不成立（COS 桶有存储/流量/证书续期成本，验证期成都桶已因此拆除；镜像站不收录任意制品，通用制品没有免费加速通道）。成本结构反转，通道裁决随之反转：updater 一并走 module 通道，可复现性改为工程手段保证（见决策 7）。另：统一入口成立前曾误判「腾讯无公开外网 goproxy」（实测的是 `goproxy.tencent.com` 等不存在的域名），正确入口是 `mirrors.tencent.com/go/`（302 门面 → `goproxy.woa.com` 双宿），本段与决策 1 均以更正后事实为准。

## 决策

1. **module path 迁至 `github.com/shichao402/relkit`**，tag 遵循 Go semver；迁移后首个 tag 为 v0.4.24（2026-09-28 实测：镜像 `.mod` 端点已是新 module path），CLI 收编首版为 v0.5.0。仓库公开、内外网 goproxy 可收录是本 ADR 的硬前提。通道形态（2026-09-28 实测定稿）：**统一入口 `GOPROXY=https://mirrors.tencent.com/go/`**——腾讯镜像站内外网边缘均 302 至 `goproxy.woa.com`，后者 split-DNS 双宿（内网解析内网地址，公网解析 `175.27.22.2` 且直连 200），内外网产品 CI 共用同一句配置；本机已以该入口端到端 `go run cmd/relkit@v0.4.24` 验证成功（含 sumdb 透传校验）。fallback 为直连 `goproxy.woa.com`（双宿域名）；社区镜像 `goproxy.cn` / `goproxy.io` 为备源（新路径 list/zip 均 200）。若仓库必须保持私有，本 ADR 整体作废。
2. **产品 CI 面编排命令移入 CLI**：`install`、`upgrade`、`release`、`ci`、`status`、`verify`、`fake`。lock 升级为 `relkit.consume/3`：`source` 块（module、version、H1、commit）+ 制品块（dart/node/rust SDK zip 与预编译 CLI 兜底的 URL 列表与 sha256；updater 退出制品块）。单一 schema、可选块，不产生两种 lock 形态。
3. **信任根迁移**：从「Release 附件 sha256 + hostScriptsSha256」迁到「sumdb + 宿主自身 go.sum」——`GOSUMDB` 不分环境：默认 `sum.golang.org`，由统一入口 `mirrors.tencent.com/go/` 透传校验（302 → `goproxy.woa.com` 代理路径，2026-09-28 实测 `/sumdb/sum.golang.org/supported` 200）；内网如需独立 sumdb（`sum.woa.com`）可配，但不作分环境要求。CLI 版本号解析链：ldflags 注入（Release 制品）→ `debug.ReadBuildInfo` module version（module 通道 `go run/install @vX`，`v` 前缀归一化）→ `devel` 占位（工作副本构建），由 `internal/buildver` 统一实现，CLI 与 updater 共用，`--version` 烟雾探针沿用。updater 二进制哈希退出信任根：其完整性由 module H1 + sumdb 覆盖（决策 7）。
4. **scripts/host 附件停止发布**；`relkit_consume.py` 在 CLI 吸收其职责后删除；`relkit_host.py` 缩减为发布机运维工具（serve/agent/keys），不再进入产品仓。此路径覆盖全部宿主而非仅 Go 产品：非 Go 宿主同样以 `go run github.com/shichao402/relkit/cmd/relkit@vX` 为入口，构建机需备 Go 工具链（`ensure_go` 自装，toolchain.json 条目与 ensure_flutter 先例俱在）；确无 Go 的构建机用预编译 CLI 兜底——GitHub Release 单源（2026-09-28 复审：原「CNB 附件备」已废弃，`.cnb.yml` 发布面删除——CNB 定位收缩为每日同步镜像，附件上传是纯负担：镜像 CI 滞后约 24h、需单独维护 build 面与 Release 通道、且蓝盾构建机对 CNB 附件域名的直连可达性从未实测；lock 仍锁 sha256，不改变信任根）。`scripts/deploy/relkit.py` 的 `build` 命令（组件注册表驱动、`--all` 产出全部附件）同步收编为 CLI `build` 子命令——它是 N-1 自举的另一半：没有它，发布 vN 的流水线无法用 vN-1 CLI 产出 vN 的 Release。deploy 层其余命令（install-serve/install-agent/remote/upgrade）归运维面，进决策 8。
5. **「同一 SHA」改述为「同一 commit」**（承接 ADR 0010 决策 7）：facade 源码来自 module tag，updater 亦出自同一 module version（决策 7），「同一 commit」由通道天然成立；tag CI 的同 commit 断言退化为防御性校验，仍写入 lock 供消费端核对。
6. **组件注册表移入 Go `internal/registry`**（ADR 0012 载体变更）：`release-contract.json` 保持线上契约。过渡期注册表由 Go 侧单源生成、Python 侧只消费不定义；迁移完成后删除 facets.py。
7. **updater 改走 module 通道，非 Go SDK 留制品通道**：`go install github.com/shichao402/relkit/cmd/relkit-updater@vX` 取代 Release 二进制下载——统一入口 `mirrors.tencent.com/go/`（内外网同一配置），首次 1.2MB module zip 对比 updater 单平台附件 8MB 级跨网拉取，且与 CLI 本体共享 GOMODCACHE；备源 goproxy.cn / goproxy.io。facade 与 updater 出自同一 module version，IPC 配对不再依赖跨通道 commit 断言。可复现性：`-trimpath` + toolchain 钉死（`toolchain.json` `go: 1.26.3`）；release CI 的 golang 镜像若与 `ensure_go` 同 patch，tag 断言 Release 附件与源码构建 byte-equal，否则 envelope 只锁 H1、二进制哈希降为可选交叉验证。dart/node/rust SDK zip 由 CLI `install` 装载，lock 制品块 URL 为 GitHub Release 单源（CNB 备援已随 `.cnb.yml` 发布面删除而废弃，见决策 4）。sdk-go 对 Go 宿主改为直接 `require` module，zip+replace 机制作废。
8. **运维面不进本轮**：`relkit_host.py` 的 serve/agent/keys 与 `scripts/deploy` 的 install-serve/install-agent/remote/upgrade 均仅发布机使用，是否 CLI 化另行决策。
9. **协议面零改动**：rup.v2、relkit.updater.v1、安装布局、bootstrap directory 全部不动；已装客户端与 serve/agent 在滚动升级期间互不感知。
10. **relkit 自举采用 N-1 模式**：发布 vN 的流水线使用 vN-1 的 CLI；`go run @vN-1` 是永久稳定入口，不追求发布当刻用上自身。
11. **缓存**：GOMODCACHE 进入 CI 缓存键（键为 go.sum 哈希），updater 与 CLI 本体同享 module 缓存，常态消费接近零下载；`.relkit/cache/artifacts` 收缩为仅服务 dart/node/rust SDK zip 与预编译 CLI 兜底，键为 lock 制品块 sha256。

## 对 ADR 0011 事故教训的承接

- 「命令成功但工作树缺文件」：模块缓存内容寻址，H1 不匹配即拒收，不存在静默半树。
- 「tag 后才发现构建不了」：哈希与构建均锚定 tag 产物；失败暴露在 preflight（go.sum / sumdb 验签），不落在构建尾部。
- 「顺序对了才对」的约定消失：文件集来自 module zip 本身，不来自 release 时的工作树。

## 结果

- 宿主 CI 下载面：全平台 40+MB 跨国 Release 缩减为首次 1.2MB module zip（统一入口 `mirrors.tencent.com/go/`，内外网同配置，含 updater）+ KB 至低 MB 级 SDK zip（GitHub 单源）；常态命中缓存后接近零下载，updater 附件跨网拉取从消费面消失。
- relkit 不再是宿主流水线中唯一依赖 GitHub 可达性的组件（SDK zip 与预编译 CLI 兜底同为 GitHub 单源，与 module 通道共享可达性假设）；Python 双语言维护面退役。
- 代价：产品 CI 面约 240KB Python 逻辑需 Go 重写并移植 conformance（另有 deploy 层 `build` 收编）；双实现并存期需防漂移；五个接入产品（loomeditor、loomlauncher、dec、svnmergetool、cronkit）需要一轮 lock 与 CI 入口升级；relkit 自身受 N-1 自举约束；无 Go 构建机以 GitHub Release 预编译 CLI 兜底（lock 锁 sha256，不改变信任根）。
