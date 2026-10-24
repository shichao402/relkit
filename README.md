# Relkit

Relkit 是一套稳定、可声明、可验证的发布与更新能力。宿主描述产品能力组合，
Relkit 不识别 Loom、Launcher、SvnMergeTool、Dec 或任何仓库身份。

版本跟随目标 Operator 的部署事实：服务端是唯一权威，宿主只声明服务器地址、
服务器根公钥与产品名；本次发布用哪个版本的 relkit、Profile 与工具，全部由服务端
决定。一份 ArtifactSet 描述一次构建产出什么，staged release 是不可变树。

使用者验收入口是
[`docs/design/vnext/USER-ACCEPTANCE.md`](docs/design/vnext/USER-ACCEPTANCE.md)。

## 产品边界

- `relkit`：ArtifactSet 生产与合并、staged 发布事务、Runtime Bootstrap、RUP 与 updater。
- `relkit-ops`：服务端运维入口（状态、计划、升级、凭据轮换与管理面）。
- `relkit-agent` / `relkit-store` / `relkit-console`：服务端发布、存储与管理面。
- `relkit-updater`：统一更新状态机；Go、Dart、Rust facade 共用同一 IPC 合同。

机器、Token、mirror 凭据和部署参数属于 Operator；产品版本、业务构建、制品内容和
UI 属于宿主。产品仓不得保存 Relkit 版本、服务 URL、Token、SSH 或私钥。

## 宿主接入

宿主只提交：

```text
relkit.project.json
relkit.custody.json
<宿主自己的 artifact producer 与产品代码>
```

`relkit.project.json` 只允许 `product`、`server`、`rootPublicKey` 三个必填字段，加上
可选的能力声明；读到未知字段直接报错。宿主不提交 CLI、不钉版本、不钉摘要。

构建从取工具开始（`scripts/relkit_tool.py` 是各宿主自带的实现）：

```bash
python3 scripts/relkit_tool.py --component cli --platform <os-arch> --out-dir .relkit-tools
.relkit-tools/relkit-<os-arch> artifacts produce \
  --version <product-version> \
  --channel <dev|stable> \
  --target <os-arch>
```

取工具固定三步：读 project 里的服务器与根公钥、下载 `<component>/<platform>` 与
`.signature`、用构建机自带的 `ssh-keygen -Y verify` 验签。验签通过才执行，所以宿主
从不需要知道服务器当前发的是哪个版本。

需要把 Runtime Bootstrap 内嵌进产品的宿主（SvnMergeTool 这类自带更新面的产品）
额外跑一次：

```bash
.relkit-tools/relkit-<os-arch> bootstrap build --project relkit.project.json --out .relkit/runtime-bootstrap.json
```

只在启动器里做更新的产品（Loom）不产出 bootstrap，这是 ADR 0047 的有意设计。

项目声明合同见
[`docs/adr/0023-desktop-updater-epoch.md`](docs/adr/0023-desktop-updater-epoch.md)。

## 构建与发布

每个平台 producer 只生成 `relkit.artifact-set/1`。多个平台用
`relkit artifacts merge` 合并；product、version、releaseCode 与 channel 任一不一致
都会失败。

产品 CI 到 ArtifactSet 为止，不持发布 endpoint、上传 Token 或签名私钥。平台之间
传递 ArtifactSet 用普通文件传输（当前编排用 mirrors Generic 仓库的 zip），刻意不
引入任何 relkit 子命令：stage 的产物路径必须与 ArtifactSet 同目录，所以 stage 与
release 一定在同一个工作区、同一个 job 里跑：

```text
stage → release plan → release execute
```

`relkit release --execute` 只允许 CI 调用（要求 `RELKIT_RELEASE_VIA_CI=1`），token
按 keyId 从宿主的 `relkit.custody.json` 里取（需求 15），编排本身不出现 token 值。

## SDK 与组件

引擎唯一：`relkit-updater` 承担全部更新逻辑，各语言只保留现役宿主的薄 IPC 客户端
（`sdk/dart` 是 facade + frame，Rust 是薄壳），Dart/Node 的厚实现已删除。SDK 从
服务器的 `/v1/tools/current/sdk-dart/<platform>` 等端点按需取回并验签，宿主仓库里
没有一个 relkit 二进制或版本号。

## Operator 运维

只读诊断：

```bash
relkit-ops status --inventory <inventory> --json
relkit-ops plan --inventory <inventory> --to latest --out upgrade.json --json
```

明确执行：

```bash
relkit-ops upgrade <operator> \
  --inventory <inventory> --plan upgrade.json --execute --json
relkit-ops rotate <credential> \
  --inventory <inventory> --execute --json
```

工具与 SDK 资产由服务器 `/v1/tools/` 面统一分发：每个组件按 `<component>/<platform>`
发出，随附 `.signature`，都受同一把根公钥背书。宿主与 CI 只认 `current` 别名，
不钉版本号或摘要，所以换代不需要改任何宿主仓库。

升级只暴露完整旧 Profile 或完整新 Profile。轮换必须完成 mint、secret store 更新、
reload/restart、健康与写权限验证、旧凭据吊销后才可激活。

空机部署和服务二进制升级见
[`scripts/deploy/README.md`](scripts/deploy/README.md) 与
[`DecAssets/skills/relkit-deploy/SKILL.md`](DecAssets/skills/relkit-deploy/SKILL.md)；
日常 Operator 编排见
[`DecAssets/skills/relkit-ops/SKILL.md`](DecAssets/skills/relkit-ops/SKILL.md)。

## 协议与设计

- [`SPEC.md`](SPEC.md)：RUP 规范。
- [`proto/`](proto/)：protobuf 结构 SSOT。
- [`docs/adr/0023-desktop-updater-epoch.md`](docs/adr/0023-desktop-updater-epoch.md)：
  桌面更新断代（现行）。
- [`docs/adr/0019-0022-refutation.md`](docs/adr/0019-0022-refutation.md)：
  0019~0022 的驳斥与退出说明。
- [`docs/architecture.md`](docs/architecture.md)：现行架构定档。

RUP Schema Set、Publish Protocol、Updater IPC 和 ArtifactSet 各自按合同窗口兼容；
不承诺任意 Relkit release 组件混搭。v0.5 及更早控制面已冻结，只接受安全与正确性
最小修复。

## 开发与验证

版本 SSOT 是 [`VERSION.json`](VERSION.json)。生成正式资产：

```bash
python3 scripts/deploy/relkit.py build \
  --cli --updater --dart-sdk --go-sdk --rust-sdk --bindings-ts
```

主要验证：

```bash
go test ./...
cargo test --manifest-path sdk/rust/Cargo.toml
cd sdk/dart && dart test
cd bindings/ts && npm test
```

`internal/vnext` 覆盖项目声明、ArtifactSet 生产与合并、staged 发布事务、工具分发与
Runtime Bootstrap 解析；`internal/vnext/tools` 与服务端 `/v1/tools` 面共同保证「工具
只从服务器来、只用宿主自带 ssh-keygen 验签」这一条不可绕过的路径。
