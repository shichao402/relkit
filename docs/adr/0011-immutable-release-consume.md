# ADR 0011：宿主只消费不可变 Release 附件

- 状态：Accepted
- 日期：2026-09-10
- 取代：`relkit.consume/1` 源码 sparse checkout 与宿主现场构建

## 背景

宿主发布机曾在旧 Git 的 sparse checkout 中得到“命令成功但工作树缺文件”的结果。
修复又受旧 bootstrap 脚本执行顺序影响，导致产品版本已打 tag 后才发现 relkit 无法构建。
这些失败面属于工具生产过程，不应由每个产品发布流水线重复承担。

## 决策

1. relkit tag CI 用 `build --all` 一次性构建清单上的全部组件：CLI、updater、SDK、只含预编译 protobuf-ES 产物的 `bindings-ts`（JS + `.d.ts`，产品不再编译生成源），以及 `scripts/host` Release 附件。加一门面只改 `hostlib/facets.py` 一行，不改 workflow 的组件列表。
2. 宿主只接受 `relkit.consume/2` lock。lock 固定 Release、commit、consumer 脚本
   SHA-256，以及每个附件的绝对 URL 和 SHA-256。
3. `scripts/host/relkit_consume.py` 只下载、验哈希、原子安装和运行版本探针。
   lock 含 host scripts 附件时，`install` 先自修复 `scripts/host`，再进入产品构建。
4. 宿主不得 clone relkit、安装 Go、现场编译，或回退 PATH/LFS 中的二进制。
5. `relkit.consume/1`、`scripts/consume.py` 及其参数直接删除，不提供兼容层。
6. `scripts/host` 附件和 `hostScriptsSha256` 覆盖整棵相对路径树；内容按 LF
   归一化，忽略 `__pycache__`。safe extract 验证每个相对路径并原子替换整树。

## 结果

- 产品 CI 是否能发布不再依赖 Git/sparse/Go 行为。
- SDK、publisher 与 updater 可证明来自同一 Release。
- 产品仓不得靠人工复制 host 脚本；`install` 成功即证明脚本树与 lock 一致。
- 升级失败发生在 lock/preflight 阶段，不再依靠 Agent 从构建尾部日志猜测。
