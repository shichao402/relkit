# ADR 0017: 消费编排收进 CLI，模块源码通道取代 scripts/host

- 状态：Proposed
- 日期：2026-09-27
- 取代：[ADR 0011](0011-immutable-release-consume.md) 决策 1 中「scripts/host Release 附件」部分、决策 3、决策 4、决策 6；决策 2 的 lock schema 由 `relkit.consume/2` 演进为 `relkit.consume/3`。决策 5（consume/1 删除）保持历史事实。
- 修订：[ADR 0010](0010-single-updater-engine.md) 决策 7「consume 同一 SHA 产出 facade 与二进制」改述为「同一 commit」；[ADR 0012](0012-single-component-registry.md) 的注册表载体由 `facets.py` 改为 Go。另：`scripts/deploy` 发布层自始未入任何 ADR，本 ADR 一并收编其 `build` 面。
- 关联：[ADR 0007](0007-entry-mirror-must-be-reachable-and-cacheable.md)、[ADR 0009](0009-publisher-protocol-negotiation.md)

## 背景

ADR 0011 禁止宿主从源码消费，当时前提有二：制品下载不是瓶颈；源码消费面复杂且刚出过 sparse checkout「命令成功但工作树缺文件」事故。两个前提均已反转：

1. GitHub Release 制品成为宿主 CI 的主要时延（单平台约 21MB、全平台 40+MB、跨国链路），且 relkit 是多数流水线里唯一依赖 GitHub 可达性的组件。
2. relkit 源码消费面已收敛：直接依赖仅 protobuf 与 x/crypto，tag 即合法 Go module。但 Python 面实际有两层：产品 CI 面约 330KB（`scripts/host`：consume 51KB + hostlib 约 240KB + relkit_host 18KB），以及此前未被任何 ADR 记录的 `scripts/deploy` 发布层约 100KB（`relkit.py` 56KB + `relkit_ops.py` 14KB）。relkit 自己的 release CI 也在用 `python scripts/deploy/relkit.py build --all` 产出全部 Release 附件——即「消费编排 Python」与「发布编排 Python」双轨并存，Go CLI 与它们构成约 600KB 级的双语言维护面。

## 决策

1. **module path 迁至 `github.com/shichao402/relkit`**，tag 遵循 Go semver，下一个 tag 为 v0.5.0。仓库公开、goproxy.cn 与 goproxy.woa.com 可收录是本 ADR 的硬前提；若仓库必须保持私有，本 ADR 整体作废。
2. **产品 CI 面编排命令移入 CLI**：`install`、`upgrade`、`release`、`ci`、`status`、`verify`、`fake`。lock 升级为 `relkit.consume/3`：`source` 块（module、version、H1、commit）+ 制品块（updater 与 dart/node/rust SDK 的 URL 与 sha256）。单一 schema、可选块，不产生两种 lock 形态。
3. **信任根迁移**：从「Release 附件 sha256 + hostScriptsSha256」迁到「sumdb（公共 sum.golang.org，内网 sum.woa.com）+ 宿主自身 go.sum」。CLI 版本号经 ldflags 从 module version 注入，`--version` 烟雾探针沿用。
4. **scripts/host 附件停止发布**；`relkit_consume.py` 在 CLI 吸收其职责后删除；`relkit_host.py` 缩减为发布机运维工具（serve/agent/keys），不再进入产品仓。此路径覆盖全部宿主而非仅 Go 产品：非 Go 宿主同样以 `go run github.com/shichao402/relkit/cmd/relkit@vX` 为入口，构建机需备 Go 工具链；确无 Go 的构建机用 COS 镜像的预编译 CLI 兜底，镜像只解决可达性，lock 仍锁 sha256，不改变信任根。`scripts/deploy/relkit.py` 的 `build` 命令（组件注册表驱动、`--all` 产出全部附件）同步收编为 CLI `build` 子命令——它是 N-1 自举的另一半：没有它，发布 vN 的流水线无法用 vN-1 CLI 产出 vN 的 Release。deploy 层其余命令（install-serve/install-agent/remote/upgrade）归运维面，进决策 8。
5. **「同一 SHA」改述为「同一 commit」**（承接 ADR 0010 决策 7）：facade 源码来自 module tag，updater 二进制来自该 tag 的 Release；tag CI 断言两者同 commit 并写入 lock，消费端核对。
6. **组件注册表移入 Go `internal/registry`**（ADR 0012 载体变更）：`release-contract.json` 保持线上契约。过渡期注册表由 Go 侧单源生成、Python 侧只消费不定义；迁移完成后删除 facets.py。
7. **updater 与非 Go SDK 继续走制品通道**：updater 哈希进入产品 envelope 审计链且源码构建不可复现；dart/node/rust SDK zip 由 CLI `install` 装载。sdk-go 对 Go 宿主改为直接 `require` module，zip+replace 机制作废。
8. **运维面不进本轮**：`relkit_host.py` 的 serve/agent/keys 与 `scripts/deploy` 的 install-serve/install-agent/remote/upgrade 均仅发布机使用，是否 CLI 化另行决策。
9. **协议面零改动**：rup.v2、relkit.updater.v1、安装布局、bootstrap directory 全部不动；已装客户端与 serve/agent 在滚动升级期间互不感知。
10. **relkit 自举采用 N-1 模式**：发布 vN 的流水线使用 vN-1 的 CLI；`go run @vN-1` 是永久稳定入口，不追求发布当刻用上自身。
11. **缓存**：GOMODCACHE 进入 CI 缓存键（键为 go.sum 哈希）；`.relkit/cache/artifacts` 继续服务 updater 与 SDK 制品。

## 对 ADR 0011 事故教训的承接

- 「命令成功但工作树缺文件」：模块缓存内容寻址，H1 不匹配即拒收，不存在静默半树。
- 「tag 后才发现构建不了」：哈希与构建均锚定 tag 产物；失败暴露在 preflight（go.sum / sumdb 验签），不落在构建尾部。
- 「顺序对了才对」的约定消失：文件集来自 module zip 本身，不来自 release 时的工作树。

## 结果

- 宿主 CI 下载面：全平台 40+MB 跨国 Release 缩减为首次 2–6MB 单通道（内网走 goproxy.woa.com），常态命中 GOMODCACHE 后接近零下载。
- relkit 不再是宿主流水线中唯一依赖 GitHub 可达性的组件；Python 双语言维护面退役。
- 代价：产品 CI 面约 240KB Python 逻辑需 Go 重写并移植 conformance（另有 deploy 层 `build` 收编）；双实现并存期需防漂移；五个接入产品（loomeditor、loomlauncher、dec、svnmergetool、cronkit）需要一轮 lock 与 CI 入口升级；relkit 自身受 N-1 自举约束；无 Go 构建机需补 COS 镜像兜底通道（预编译 CLI + lock sha256，镜像不改变信任根）。
