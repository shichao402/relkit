# 单一 Core、Wrapper、Registry 与 relkit-ops

配套 [ADR 0020](../../adr/0020-vnext-catalog-operator-resolution.md) §6、[ADR 0021](../../adr/0021-vnext-artifact-runtime-registry.md) §4、[ADR 0022](../../adr/0022-vnext-certification-ops-skill.md) §1–3。

## 运行时拓扑

```text
relkitw ──► relkit CLI (selected distribution)
              ├─ resolve → relkit.resolution/1
              ├─ install / doctor / inspect
              ├─ stage → simulate → fake → publish
              └─ verify released

relkit-ops ──► Operator Profile 签发 / upgrade / rotate / mirror / fleet cert
relkit-deploy ──► agent / store / console 二进制落机

Skill ──► 只编排上述命令，不实现第二套 schema/gates
```

## Wrapper（`relkitw`）

- Relkit 生成，宿主不可客制化  
- 内置 Distribution Root fingerprint  
- 读 `relkit.project.json` → 取 Operator Profile → 取得 CLI → 生成 resolution  
- 删除三仓复制的 `scripts/relkit_cli.py`  

## Registry

- 唯一声明式 component registry  
- 生成/校验 Go 打包、facade 列表、transport（module vs asset）  
- 无仓库名单段；只有能力与合同字段  

## 与宿主边界

| 宿主拥有 | Relkit 拥有 | Ops 拥有 |
| --- | --- | --- |
| 产品版本、业务构建、UI | resolution、签名、协议、updater | Operator、Token、SSH、mirror |
| producer 制品内容 | 发布事务、directory/index | Profile 签发与升级 |
| 检查更新时机 | facade/projection 合同 | CI adapter / fleet 登记 |
