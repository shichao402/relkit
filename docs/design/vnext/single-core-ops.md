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

宿主与 Relkit 只有四条正式交界面：

1. **工具消费**：宿主声明能力与逻辑 `operatorRef`；wrapper、Catalog/Profile 与 resolution
   决定实际工具链。宿主可选择 Operator，不能复制 bootstrap、改 resolution 或单独钉 CLI。
2. **制品发布**：producer 是业务客制化入口，只返回 ArtifactSet；Relkit 负责 merge、
   stage、签名、发布事务和消费者读回。宿主可决定制品内容，不能直接写 index/directory。
3. **运行时订阅**：宿主决定订阅哪些产品、UI 和检查时机；受信 Runtime Profile、
   canonical facade 与 updater 状态机决定 URL、密钥和 apply。宿主不能复制 DTO 或另写 updater。
4. **机器运维**：ops inventory、CI/deploy/mirror adapter 可适配现场；Operator Profile、
   凭据、服务升级和认证域不进入产品仓，也不能反向改变业务制品。

允许的客制化必须落在 project/producer/extension/ops adapter 的版本化合同中。凡是需要
修改签名对象、resolution、发布事务、updater 状态机，或在 core 判断仓库名的需求，都不在
允许范围内；应先补通用能力合同和组合测试。

## Skill 与用户交互

Skill 不能把内部架构问题转交给产品使用者逐项拍板。默认流程是：

1. `inspect/status/doctor` 先读取现场事实
2. core/ops 用安全、简单、可验证的默认值闭合信任根、Profile、Catalog 和发布事务
3. 只询问会改变产品行为或风险归属的真实业务选择
4. 对用户展示“将连哪个 Operator、用哪个 distribution、影响哪些产品、如何回滚”，
   不要求用户先理解 `fleet`、root 层级或 schema 字段

`fleet` 在用户界面应解释为“本认证域用于升级回归的场景/canary 登记”，不是服务器集群；
`pin` 应解释为“目标 Operator 的固定部署事实”，不是项目自行选择旧版本。

删除 onboarding 步骤账本后，门禁仍由结构化 core/ops 命令执行；减少问题数量不能变成
绕过验证或让 Skill 自己记一份“已完成”。

## 生产控制面命令

- `relkit-ops distribution create`：从官方 `relkit.release/1` manifest、完整 Release 资产和 immutable URL 生成 distribution；缺少 Registry 声明的任一资产即失败。distribution digest 只覆盖不可变身份，排除 `state` 与 `certificationSequence`。
- `relkit-ops distribution mirror`：按 distribution 摘要同步全部 component 与 `releaseAssets` 到 Operator 内容寻址 mirror，并逐项读回校验；mirror 只是字节供给，不参与版本选择。
- `relkit-ops catalog put/transition`：先校验上述 identity digest，再执行 candidate → certified 生命周期。
- `relkit-ops agent policy`：默认只输出计划；`--execute --backup <path>` 才原子更新已 provision 产品的 Distribution Root、Operator deployment 与 lane，并保留 backend、Token 等未知字段。远端传输、重启与健康检查仍由声明式 deploy adapter 负责。
- `relkit-ops runtime-profile publish`：请求 agent 使用现场 Product RUP 私钥签发首个 Runtime Profile；私钥不进入 CI 或 Operator Profile。
