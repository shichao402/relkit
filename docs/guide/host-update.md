# 宿主接入：两轨发布与 InstallSpec

- 对象：产品仓（任意语言）接入 relkit 更新
- 不写：引擎实现、payload 内部校验算法、某门 SDK 源码路径
- 权威细节：[ADR 0013](../adr/0013-internal-update-payload.md)、[ADR 0014](../adr/0014-placement-and-version-library.md)、[`SPEC.md`](../../SPEC.md) §6.2 / §11 / §12.5 / 附录 B、[`CLI.md`](../../CLI.md) `relkit stage`

---

## 1. 两轨

同一版本可以（且在启用内部更新时必须）同时发布两类 artifact：

| 轨 | stage 入口 | `kind` | 用途 |
|---|---|---|---|
| 完整安装 | `relkit stage --install <路径>` | `archive` / `installer` / `binary` / `blob` | 冷启动、人页下载、`/-/latest/...` 固定链、内部更新不可用时的回退 |
| 内部更新 | `relkit stage --payload <目录>` | `payload` | 已装客户端经 `relkit-updater` 事务化增/覆/删 |

### 1.1 完整安装

`--install` 产出不透明安装产物。扩展名推断 `kind`（见 CLI.md）；不要写成 `kind=payload`。

人页与 `latest` 指针**不暴露** `kind=payload` 的条目；过滤在 relkit 发布/站点侧完成，产品仓无需再滤一层。

### 1.2 内部更新

`--payload <目录>` 把输入树打成 zip，结构固定为：

- `files.pb`（`relkit.payload/1` 完整文件表）
- `files/`（与表一一对应的正文）
- 可选 `scripts/`（pre/post，白名单解释器）

stage **注入** selector `apply=relkit-payload`（不要手写其它 `apply` 值）。新引擎 check 的客户端选择器带同一键，因而优先选中 payload；老客户端没有该键，仍只匹配同组完整安装包。

输入树可用 `.relkit-payload/manifest.json` 声明包内 `preserve` 与脚本；该配置目录不进入安装树。结构规则见 ADR 0013，宿主不必解析 zip 内部。

### 1.3 闸门：同组必须有完整安装包

每个 payload **必须**另有一个去掉 `apply` 后 `selectors` 完全相同的完整安装 artifact。只发 payload 会被 stage/publish 拒绝。

原因：老客户端与人页/latest 只消费完整安装轨；payload 不能单独承担冷启动。

---

## 2. 宿主合同（语言无关）

### 2.1 只 spawn sidecar

- 唯一引擎是安装树内的 `relkit-updater`（与 facade 同 SHA 锁定）。
- 宿主只通过**生成的 facade** 编排：`check` / `download` / `apply` / …。
- **禁止**在产品仓再实现 apply、解压 payload、或手写第二份 CheckResult / InstallSpec DTO。
- 本机协议是 `relkit.updater.v1`；线上签名对象仍是 `rup.v2`。InstallSpec **不进** index/manifest。

### 2.2 IPC 窗口

当前窗口是破坏性 **`[3, 3]`**（不是历史的 1–2）。窗口是 facade 编译期常量，不写进 `ClientProfile`。混版本 sidecar 与 facade 会导致协商失败。发版 `manifest.json` 带 `minUpdaterIpc` / `maxUpdaterIpc`；产品仓 `relkit.lock.json` 的 `updaterIpc` 由 upgrade 从这份 manifest 填写，不要手改、也不要沿用上一份 lock。

### 2.3 `InstallSpec.placement`

| 值 | 原名（已废） | 行为 |
|---|---|---|
| `IN_PLACE` | `wholeRoot` | 按文件表写入 `install_root` |
| `LIBRARY` | `versionedDir` | 写入 `versions/<version>/`，成功后切换 `active.json` |

`LibraryPolicy`（仅 LIBRARY 有意义）：

- `retain`：保留几个已安装版本（实现默认语义见 ADR 0014）
- `reserved_codes`：清理时保留的项目 pin

`preserve`：不覆盖、不删除的相对路径（与包内 preserve 取并集）。

**删除不是可配置轴。** 引擎固定基线差分：

```
delete = previous_baseline − current_table − preserve
```

不要写 Ownership / `fullTree` / `declaredOnly`；不要指望扫描安装目录删「表外用户文件」。

### 2.4 `apply_disposition` 与退出

`UpdateAvailable.apply_disposition`：

| 值 | 含义 | 宿主动作 |
|---|---|---|
| `INTERNAL` | 选中了 `kind=payload` | 交给 `relkit-updater` apply；**禁止**自行解压 |
| `FULL_INSTALL` | 完整安装轨 | 把已校验本地路径交给平台安装流程 |

`requires_host_exit` 由本次 plan / 文件表判定（例如 in-place 触及当前 executable/sidecar），**不是** `placement` 的常量。LIBRARY 下新版本目录通常不属于当前运行树。

### 2.5 禁止项（易踩）

- 不要写 `meta.layout`（Layout API 已废）。
- 不要把 `placement` 放进 artifact `selectors`；落点只属于本机 `InstallSpec`。
- 不要只升级 SDK 而不 lock 同 SHA 的 `relkit-updater` sidecar（IPC `[3,3]`）。

---

## 3. 最小接入步骤

1. **锁定 sidecar + SDK**  
   按组件 registry / 产品仓既有 consume 流程，把 facade 与同版本 `relkit-updater` 升到支持 payload / Placement 的 release（IPC `[3,3]`）。

2. **填写 `ClientProfile` + `InstallSpec`**  
   - Profile：`product`、`allowed_channels`、入口/index/fallback URL、信任公钥。  
   - InstallSpec：`placement`（`IN_PLACE` 或 `LIBRARY`）、`install_root`、`executable_relpath`、`sidecar_relpath`、需要时的 `preserve` / `LibraryPolicy`。  
   Runtime 再带上 `channel`、`current_code`、`data_dir`，以及本机 `client_selectors`（`os`/`arch` 等）。**不要**自己填 `apply=relkit-payload`：引擎 check 时会注入该键。

3. **stage 两轨（同组 selectors）**  
   虚构产品 `demoapp` 示例：

   ```bash
   relkit stage 1.5.0 --code 150 \
     --install dist/demoapp-win-x64.zip  kind=archive,os=windows,arch=x64 \
     --payload dist/demoapp-payload      os=windows,arch=x64
   ```

   第二条会自动得到 `kind=payload` 与 `apply=relkit-payload`；第一条是同组闸门要求的完整安装包。多平台则每组 `os`/`arch`（及自定义键）各自成对。

4. **publish 后验人页**  
   人页与 `latest` 只应出现完整安装包。产品侧**不必**额外过滤 payload；若仍看到 payload，说明指针/站点未走当前 relkit 发布路径，应查 publish/agent，而不是在产品仓写过滤逻辑。

5. **宿主运行时**  
   facade `check` → 若 `INTERNAL` 则 `download` + `apply`；若 `FULL_INSTALL` 则走平台安装。是否先退出看 `requires_host_exit`。

---

## 4. 权威指针

| 主题 | 文档 |
|---|---|
| payload 结构、基线差分、selector 隔离、IPC `[3,3]` | [ADR 0013](../adr/0013-internal-update-payload.md) |
| Placement、`LibraryPolicy`、单引擎 apply | [ADR 0014](../adr/0014-placement-and-version-library.md) |
| `kind`、产物选择、`apply` 收窄、§12.5 应用 | [`SPEC.md`](../../SPEC.md) §6.2、§11、§12.5、附录 B |
| `--install` / `--payload` 标志与 stage 校验 | [`CLI.md`](../../CLI.md) §4.2 |
| 单引擎 facade / sidecar | [ADR 0010](../adr/0010-single-updater-engine.md) |
| 旧 Layout 背景（已取代） | [`docs/design/install-layouts.md`](../design/install-layouts.md) |
