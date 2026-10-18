# relkit 缺口留档（sim-hosts 实证，2026-10-18）

> 来源：SIM-HOSTS 换根链任务（R1-R16）在 sim 四产品上端到端实证暴露的 relkit 平台缺口。
> 每条含：现象、根因、当前权宜、对 relkit 的改进建议。workflow 内对应位置已有 TODO 注释。

## 1. 单平台产品无合法 merge 路径（passthrough 缺失）

- 现象：sim-s2-manager / sim-s2-managed / sim-s3 只有一个平台的 ArtifactSet；
  `relkit artifacts merge` 硬性要求 ≥2 输入（vnext.go:1083），单输入直接报错。
- 根因：merge 子命令无单输入合法路径；release --execute 前的合并步骤对单平台产品
  无官方 passthrough 通道。
- 当前权宜（sim-release.yml Merge ArtifactSets step）：单输入时手动 `cp` artifact-set.json
  与全部制品文件直通，**跳过了 merge 内部的 VerifyFiles / 重复路径校验**。
- 建议：relkit 提供官方单输入支持（merge --allow-single 或 `artifacts passthrough`
  子命令，保留校验），届时删除 workflow 的 cp 分支，统一走 merge。

## 2. offline bundle 不物化未订阅产品的产品根

- 现象：sim-s2-managed（updater-projection，Loom 同款）无订阅、Node blob 无 facade，
  offline bundle 通道下 bootstrap 不物化其产品 RUP Root；`verify released` 必然报
  `no Product RUP Root was provisioned`。
- 根因：offlinebundle.Install 只写 RuntimeProfiles，未订阅产品没有 Product RUP Root 通道。
- 当前权宜（sim-release.yml Stage and execute release step）：从 GitHub vars 注入
  RELKIT_PRODUCT_ROOT_SIM_S2_MANAGED（产品根公钥 pb64），显式 `--product-root` 传入
  verify released。按 SKILL.md 该显式注入仅为诊断手段，不得作为唯一验收证据。
- 建议：relkit 增加 manager 代管验收通道——由 manager 的双订阅 resolution（覆盖
  managed 的发布验收）替代对 managed 自身产品根的依赖；届时删除 workflow 注入段。

## 3. manager 代管（projection）产品缺独立发布验收语义

- 现象：managed 类产品的"自升级 e2e"是伪命题（R12 实证：main.js 任意 argv 都打
  hello-world，无 check 子命令；更新由 manager 驱动）。发布链上只有"制品存在 +
  install-only 断言"（本轮 +8 全绿实证），升级语义必须由 manager e2e 的
  assertManagedVersion 代管断言覆盖。
- 根因：relkit 的发布验收模型（verify released）假设产品自身持有 updater 投影；
  projection 产品天然不满足。
- 建议：relkit 增加代管验收原语（例如 verify released --via-manager，或 manager
  resolution 覆盖声明），把代管语义显式化，避免每个下游仓库自行拼装验收断言。

## 4. 跨渠道 manifest 覆盖缺陷（dec stable 残留同类）

- 现象：发布到某渠道（如 stable）会复写同名 manifest 的渠道字段语义；跨渠道并存的
  manifest 存在覆盖缺陷（dec stable 存量残留 3 节点 + cronkit stable 5 节点即此类）。
- 根因：manifest 的渠道字段是单值，渠道切换/并存时缺少 per-channel 投影。
- 建议：manifest schema 增加 per-channel 段（或 channel map），发布时只投影目标渠道，
  不覆写其他渠道的存在性。dec/cronkit stable 存量届时一条命令可清。

## 5. GitHub Actions 矩阵边界行为（workflow 工程教训，非 relkit 本体）

- 空矩阵（fromJSON 展开为 []）下，矩阵 job 不创建实例会直接把 run 打红（无失败注解、
  prepare 绿）——须 `if: needs.prepare.outputs.matrix != '[]'` 守卫让 job 显式 skipped
  （run 39097646193 → 39097917909 实证修复）。
- 矩阵维度值必须是标量；对象列表（target/runner/host_target）只属于 include。维度挂
  对象 = job 创建失败，任何输入下均缺席（run 39096462210 / 39097384892 实证）。
- 步骤 env 不向下延续：每步引用 matrix.product 都必须自己声明（run 39097637338 的
  `PRODUCT: unbound variable`）。
- 提取逻辑不内嵌 yaml：全部收敛到 scripts/（PAC 铁律），`VAR=$(cmd)` 赋值消费 +
  set -e 才是 fail-loud（`echo "x=$(cmd)"` 会吞失败码，run 39096462210 实证）。
