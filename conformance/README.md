# RUP 一致性用例（Conformance Fixtures）v2

本目录是语言无关的测试数据。任何 RUP 客户端实现（Go / Dart / Node / C#…）都**必须**跑通全部用例，任何发布侧实现都**必须**跑通 `reachability/`。

## 为什么需要它

规范性行为集中在四处：版本选路、可达性校验、产物选择、签名验证。这四处如果由各语言各写一遍，出现分歧几乎是必然的 —— 现实证据是 Dec 的版本比较器丢弃了 `-beta` 后缀、RemoteCam 的版本比较器丢弃了 `+build` 号，两个项目**各自**引入了一种缺陷，且都没有被发现。

用例是防止「N 个项目 N 种 bug」的唯一低成本手段。写一次，所有实现复用。

## 目录

| 目录 | 被测行为 | 对应规范 |
|---|---|---|
| `version-select/` | `selectNextTarget` 与 `resolveUpgradePath` | SPEC.md §9 |
| `reachability/` | 发布前的可达性校验（发布侧，非客户端） | SPEC.md §10 |
| `selector/` | artifact 匹配与多匹配仲裁 | SPEC.md §11 |
| `signature/` | 信封验签、密钥轮换、防降级 | SPEC.md §4.1、§12.1、§12.4 |
| `recovery/` | 全网失败时宿主必须给出编译期内嵌 `RecoveryHelp` | ADR 0007 |

## 怎么跑

本目录只有 JSON 夹具，不含执行器。

| 实现 | 如何跑 |
|---|---|
| 发布工具 / Go（本仓） | `go test ./internal/chain ./internal/selectors ./internal/envelope`（权威行为在这些包里） |
| Dart 客户端 `sdk/dart/` | `cd sdk/dart && dart test test/conformance_test.dart` |
| Node 客户端 `sdk/node/` | `cd sdk/node && npm test`（含 unit + conformance；`npm run conformance` 只跑夹具） |

排查「某个升级路径为什么是这个结果」时，直接读 relkit 仓库的 `internal/chain`，比读散文快。

客户端用各运行时自带的 Ed25519（Go `crypto/ed25519`、Dart `cryptography`、Node `crypto.verify` 等）。签名夹具里的密钥是测试专用平凡种子，禁止用于真实发布。

## 通用规则

1. **禁止补全。** runner **必须**按原样使用用例文件中的 `index` / `manifest` 对象，**禁止**填充缺省字段或改写。用例因此显得冗长（例如每个版本节点都带一个占位 `manifest`），这是有意的：用例文件同时是合法的协议文档，可以直接喂给 `../schema/` 下的 JSON Schema 做二次校验。
2. **占位值。** 选路与可达性用例不关心 manifest 的实际内容，其 `sha256` 使用可辨识的重复数字（如 64 个 `1`），`urls` 使用 `.invalid` 域名。这些值满足 Schema 但不可解析，从而保证实现不会意外去访问网络。
3. **节点引用。** 期望结果用 `version` 字符串引用版本节点，而不是数组下标 —— 下标会让「乱序」用例失去意义。
4. **null 的含义。** `expectTarget: null` 表示 `selectNextTarget` 必须返回空，即「当前没有可达的更新」。
5. **失败即不合规。** 任何一个用例不通过，该实现**禁止**声称兼容 RUP v2。

## `version-select/` 用例格式

```json
{
  "name": "required-intermediate",
  "description": "人类可读的场景说明",
  "index": { "…完整的 rup.index/1 对象…" },
  "cases": [
    {
      "currentCode": 100,
      "expectTarget": "1.2.0",
      "expectPath": ["1.2.0", "1.5.0"],
      "expectMandatory": false,
      "why": "1.5.0 要求 minFrom=120，当前 100 不满足，因此只能先到 1.2.0"
    }
  ]
}
```

| 字段 | 说明 |
|---|---|
| `currentCode` | 客户端当前的 `code` |
| `expectTarget` | `selectNextTarget` 的期望返回，用 `version` 引用，或 `null` |
| `expectPath` | `resolveUpgradePath` 的期望返回，`version` 数组，空数组表示无更新 |
| `expectMandatory` | 是否命中 `minSupported`（SPEC.md §9.3）。省略时视为 `false` |
| `why` | 仅供人阅读，实现**禁止**解析 |

## `reachability/` 用例格式

```json
{
  "name": "broken-chain",
  "index": { "…" },
  "expectValid": false,
  "expectErrors": ["unreachable"],
  "expectWarnings": []
}
```

`expectValid` 为 `false` 表示发布工具**必须**拒绝发布该 index。仅有警告时 `expectValid` 为 `true`。

`expectErrors` / `expectWarnings` 使用下列稳定码（SPEC.md §10.2、§10.3）。实现**必须**产生列出的全部码，**可以**附加自己的诊断文本，但**禁止**用别的码替代：

| 码 | 类别 | 含义 |
|---|---|---|
| `no-head` | 错误 | 全部节点被撤回，无可用版本 |
| `duplicate-code` | 错误 | `code` 重复 |
| `unreachable` | 错误 | 某个节点起点无法到达 head |
| `min-supported-unreachable` | 错误 | 从 `minSupported` 出发无法到达 head |
| `min-supported-above-head` | 错误 | `minSupported` 高于 head 的 `code` |
| `zero-unreachable` | 警告 | 从 `code=0` 无法到达 head |

`sequence-not-increasing` 与 `manifest-digest-mismatch` 需要远端状态或真实文件，无法用静态用例表达，因此不在本目录覆盖范围内。

## `selector/` 用例格式

```json
{
  "name": "os-arch",
  "manifest": { "…完整的 rup.manifest/1 对象…" },
  "cases": [
    { "clientSelectors": { "os": "windows", "arch": "x64" }, "expectArtifactId": "app-windows-x64" },
    { "clientSelectors": { "os": "solaris", "arch": "x64" }, "expectArtifactId": null }
  ]
}
```

## `signature/` 用例格式

该目录下的签名用例由确定性 Ed25519 生成并检入仓库，**禁止**手工改 envelope 字节 —— 手改会使签名与 payload 失配。若需扩展，用 `keys.json` 里的平凡种子在实现侧重新生成并整文件替换。

`payload` 是 **Index 的 protobuf 字节**（`rup.index/2`），`schema` 是 `rup.envelope/2`（`wrong-envelope-schema` 故意写成 `rup.envelope/1`）。签名覆盖这些 payload 字节，与线上线格式相同。runner **必须按原样使用**这些字段，禁止把 JSON 重编码后再重签。

`keys.json` 不是用例，而是供 runner 使用的密钥表。它同时包含公钥与私钥种子：**这些是测试专用的平凡密钥（种子为重复的 `0x01`、`0x02` 等），禁止用于任何真实发布。** 可信 keyId 为 `k1` 与 `k2`，`kx` 与 `ky` 代表客户端不认识的签名者。

`envelope.json` 覆盖 SPEC.md §4.1 的验签流程，以及 §12.1 步骤 1–3 的 `product` / `channel` 守卫：

```json
{
  "name": "envelope",
  "trustedKeys": ["k1", "k2"],
  "expectProduct": "conformance",
  "expectChannel": "stable",
  "cases": [
    { "name": "valid-k1", "envelope": { "schema": "rup.envelope/2", "payload": "…base64 protobuf Index…", "signatures": ["…"] }, "expectAccepted": true }
  ]
}
```

`expectAccepted` 表示客户端是否可以把该信封当作可信 index 使用。为 `false` 的用例中有几个特别值得注意：

- `unknown-key`：签名在密码学上完全有效，但签名者的 `keyId` 不在信任列表里。**必须**拒绝。
- `cross-payload-replay`：一份由 `k1` 对**另一个** index 产生的真实签名，被贴到当前 payload 上。这能抓出「对重新序列化后的对象验签」而非「对 payload 原始字节验签」的实现。
- `unsupported-alg`：`alg` 为未知算法时**禁止**跳过验签。
- `no-signatures`：空签名数组**禁止**被理解为「没什么要检查的，因此通过」。
- `rotation-untrusted-first`：签名数组第一项来自未知密钥，第二项来自可信密钥，期望**接受**。实现**禁止**在遇到第一个不可用的签名时就放弃。

`anti-rollback.json` 覆盖 §12.4 的 `sequence` 单调性，是纯逻辑用例，不涉及密钥：

```json
{ "lastSeenSequence": 42, "indexSequence": 41, "expectAccepted": false }
```

注意 `lastSeenSequence` 等于 `indexSequence` 时**必须**接受 —— 这是「再次取到同一份 index」的正常情况。`lastSeenSequence` 为 `null` 表示客户端首次运行。
