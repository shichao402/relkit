# Schema 索引

现行文档形状的 JSON Schema。每份对应一个 Go 包，字段与包内结构一一对应，以代码为最终判据。

| Schema | 对应包 | 谁读谁写 |
| --- | --- | --- |
| [`relkit.project/1`](relkit.project.1.schema.json) | `internal/vnext/project` | 宿主项目提交；`relkit release` 读 |
| [`relkit.runtime-profile/1`](relkit.runtime-profile.1.schema.json) | `internal/vnext/runtimeprofile` | 服务器签发；updater 与订阅方读 |
| [`relkit.runtime-bootstrap/2`](relkit.runtime-bootstrap.2.schema.json) | `internal/vnext/runtimebootstrap` | 宿主构建期生成并嵌入 updater；updater 读 |
| [`relkit.distribution-catalog/1`](relkit.distribution-catalog.1.schema.json) | `internal/vnext/catalog` | 服务器签发；宿主 CI 与工具读 |
| [`relkit.tools/1`](relkit.tools.1.schema.json) | `internal/vnext/tools` | 服务器签发；宿主 CI 读 |
| [`relkit.operator-profile/1`](relkit.operator-profile.1.schema.json) | `internal/vnext/operator` | 服务器部署时签发；`relkit-ops` 读 |
| [`relkit.artifact-set/1`](relkit.artifact-set.1.schema.json) | `internal/vnext/artifactset` | producer 产出；release core 读 |
| [`relkit.extension/1`](relkit.extension.1.schema.json) | `internal/vnext/extension` | release core 与 producer 之间的 stdin/stdout |
| [`relkit.custody-manifest/1`](relkit.custody-manifest.1.schema.json) | `internal/custody` | 每个持有钥匙的地方各一份；所有程序读 |

## 已经被删除的 schema

这些 schema 随所属层一起删除，不再有对应代码。列在这里是为了让检索找到明确答案，而不是让人翻到旧文件后以为它还有效。

| 已删除 | 原位置 | 原因 |
| --- | --- | --- |
| `relkit.resolution/1` | `docs/design/vnext/schemas/` | 宿主不再挑版本，resolution 整条链删除（ADR 0023 决策 3） |
| `relkit.runtime-bootstrap/1` | 同上 | `/1` 含 `resolutionDigest` 与 `trustedKeys` 多根集合，改为 `/2` 单根内嵌 |
| `relkit.distribution-catalog/1` 的候选/认证字段 | 同上 | 状态机、`certificationSequence`、`requires`/`provides` 全部删除；Schema 名不变，字段已重写 |
| `target-dimension.json`（selector 夹具） | `conformance/selector/` | 场景是"桌面 client 与 Android server 共用版本号"，移动端已退出范围 |

## 相关

- [现行架构](../architecture.md)
- [ADR 0023](../adr/0023-desktop-updater-epoch.md)
- [ADR 0019-0022 驳斥](../adr/0019-0022-refutation.md)
