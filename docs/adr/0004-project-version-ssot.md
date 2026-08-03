# ADR 0004: 项目版本号由 relkit 接管（VERSION.json SSOT）

- Status: Accepted
- Date: 2026-08-03

## 决策

接入 RUP / relkit 的项目**必须**在仓库根目录（与 `relkit.json` 同级）提供规格化的 `VERSION.json`，并由 relkit 提供读写与 code 派生；宿主语言**禁止**自备版本解析器作为权威实现。

### 文件规格

```json
{
  "schema": "rup.version/1",
  "version": "1.2.3+45"
}
```

- `schema` 必须为 `rup.version/1`
- `version` 必须为 `x.y.z+build`（`build` 为非负整数）
- 其它顶层字段可保留（宿主扩展），但 `version` / `schema` 语义由 relkit 独占

### 官方入口（跨语言）

```text
relkit version get [--field version|number|build|code] [--json]
relkit version set <x.y.z+build>
relkit version bump major|minor|patch|build
relkit version code
relkit version path
```

任意语言 / CI 只调上述 CLI（或 Go 包 `github.com/shichao402/relkit/version`），不各自解析 JSON。

### 与线格式的关系

| 层 | 载体 | 字段 |
|---|---|---|
| 项目侧 SSOT | `VERSION.json` | `version` |
| 发布配置 | `relkit.json` `codeStrategy` | 默认 `version-build`（code = `+build`） |
| 线上 protobuf | Index / Manifest | `version` + `code` |

`relkit stage` / `publish` 在省略版本参数时读取 `VERSION.json`。

### 遗留格式

只读兼容旧的 `{"app":{"version":"..."}}`。任何 `set` / `bump` / `init` 写入都规范化为 `rup.version/1`。

## 后果

- 消除各仓库自写 `version_manager` / PyYAML 依赖漂移
- 无 JSON 运行时的语言通过 CLI 接入，不自行解析
- SvnMergeTool 等业务仓将 VERSION 与 `codeStrategy` 对齐到本 ADR
