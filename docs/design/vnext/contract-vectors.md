# 协议合同向量（requires / provides）

配套 [ADR 0019](../../adr/0019-vnext-breaking-and-protocol-contracts.md)。  
不做 Relkit release 两两矩阵；只维护下列合同的版本窗口与能力声明。

## 四类合同

| 合同 ID | Peer A | Peer B | 现网锚点 | 兼容规则 |
| --- | --- | --- | --- | --- |
| `rupSchemaSet` | updater / client | directory·index·manifest·artifact | `proto/rup/v2` + conformance | 冻结 schema set + 行为 conformance；**≠** consume `protocol` |
| `publishProtocol` | CLI / publisher | agent / store | `internal/publishproto`，当前选择 `3`、兼容窗口 `[2,3]` | inclusive `[min,max]` 交集；无交集 hard fail |
| `updaterIpc` | language facade | updater sidecar | `proto/updater/v1`，现网 lock 常见 `[3,3]` | inclusive `[min,max]`；无交集 tooOld/tooNew |
| `artifactSet` | artifact producer | release core | `relkit.artifact-set/1` | schema 版本相等或声明兼容 |

## 作用域（强制）

每条 requirement 必须带 peer/domain，禁止裸 capability。

示例（distribution 侧 `provides` / component 侧 `requires`）：

```json
{
  "provides": [
    { "domain": "rupSchemaSet", "peer": "updater", "id": "rup.v2" },
    { "domain": "publishProtocol", "peer": "cli", "min": 2, "max": 2 },
    { "domain": "updaterIpc", "peer": "facade", "min": 3, "max": 3 },
    { "domain": "artifactSet", "peer": "producer", "id": "relkit.artifact-set/1" }
  ],
  "requires": [
    { "domain": "publishProtocol", "peer": "agent", "min": 2, "max": 2 },
    { "domain": "rupSchemaSet", "peer": "directory", "id": "rup.v2" }
  ]
}
```

## Resolver 检查顺序

1. 选择完整 distribution（exact 或 latest-compatible）  
2. distribution 内 facade/sidecar/schema/CLI 同源  
3. 每条 `requires` 被 Operator Profile / peer component 的 `provides` 满足  
4. project capability fingerprint 所需组件与平台行存在  
5. 写入 `relkit.resolution/1` 的 `contracts` 块  

失败一律 fail closed，不回退到更旧未认证版本。

## 演进规则

- 扩大 `max` 或新增 optional capability：minor，跑受影响 cohort  
- 提高 `min` 或删除行为：major，全部已登记活跃宿主 + 旧客户端 conformance  
- 禁止用「某个宿主需要」作为扩大全局行为的理由；先改合同再改实现  
