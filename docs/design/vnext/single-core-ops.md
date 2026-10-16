# 单一 Core、Wrapper、Registry 与 relkit-ops

配套 [ADR 0020](../../adr/0020-vnext-catalog-operator-resolution.md) §6、[ADR 0021](../../adr/0021-vnext-artifact-runtime-registry.md) §4、[ADR 0022](../../adr/0022-vnext-certification-ops-skill.md) §1–3。

## 运行时拓扑

```text
relkitw ──► relkit CLI (selected distribution)
              ├─ resolve → relkit.resolution/1
              ├─ install / doctor / inspect
              └─ stage → release plan → execute → verify released

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

## 生产控制面命令

- `relkit-ops distribution create`：从官方 `relkit.release/1` manifest、完整 Release 资产和 immutable URL 生成 distribution；缺少 Registry 声明的任一资产即失败。distribution digest 只覆盖不可变身份，排除 `state` 与 `certificationSequence`。
- `relkit-ops catalog put/transition`：先校验上述 identity digest，再执行 candidate → certified 生命周期。
- `relkit-ops agent policy`：默认只输出计划；`--execute --backup <path>` 才原子更新已 provision 产品的 Distribution Root、Operator deployment 与 lane，并保留 backend、Token 等未知字段。远端传输、重启与健康检查仍由声明式 deploy adapter 负责。
- `relkit-ops runtime-profile publish`：请求 agent 使用现场 Product RUP 私钥签发首个 Runtime Profile；私钥不进入 CI 或 Operator Profile。
