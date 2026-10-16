# Relkit

Relkit 是一套稳定、可声明、可验证的发布与更新能力。宿主描述产品能力组合，
Relkit 不识别 Loom、Launcher、SvnMergeTool、Dec 或任何仓库身份。

版本跟随目标 Operator 的部署事实：

- 固定自建服务使用 `exact`，客户端与服务端选择同一精确 distribution；
- 滚动服务使用 `latest-compatible`，只选择已认证且合同匹配的 distribution；
- 一次构建生成一份 immutable resolution，所有平台、阶段和发布证据共用其 digest；
- candidate 使用独立身份、Catalog 与权限，不能污染 stable。

使用者验收入口是
[`docs/design/vnext/USER-ACCEPTANCE.md`](docs/design/vnext/USER-ACCEPTANCE.md)。

## 产品边界

- `relkit`：resolve、doctor、组件物化、ArtifactSet、发布事务、RUP 与 updater。
- `relkit-ops`：Operator Profile、Catalog、离线 bundle、升级、凭据轮换与 fleet 认证。
- `relkit-agent` / `relkit-store` / `relkit-console`：服务端发布、存储与管理面。
- `relkit-updater`：统一更新状态机；Go、Dart、Rust facade 共用同一 IPC 合同。
- `relkit-ops` Skill：只解释结构化证据和编排命令，不实现第二套门禁或步骤账本。

机器、Token、mirror 凭据和部署参数属于 Operator；产品版本、业务构建、制品内容和
UI 属于宿主。产品仓不得保存 Relkit 版本、服务 URL、Token、SSH 或私钥。

## 宿主接入

宿主只提交：

```text
relkit.project.json
relkitw
relkitw.cmd
<宿主自己的 artifact producer 与产品代码>
```

Operator 先 provision bootstrap，或向完全离线 CI 提供签名 distribution bundle。
随后宿主统一执行：

```bash
./relkitw inspect --json
./relkitw doctor
./relkitw artifacts produce \
  --version <product-version> \
  --channel <dev|stable> \
  --target <os-arch>
```

`relkitw` 验证 Catalog、Operator Profile、Runtime Profile 与 resolution，再按摘要把
CLI、updater、SDK 和 projection 物化到 `.relkit/components/`。宿主不 clone Relkit，
不从 PATH 猜 CLI，也不维护 consume lock、`scripts/host` 或 onboarding 状态。

项目声明合同见
[`docs/adr/0020-vnext-catalog-operator-resolution.md`](docs/adr/0020-vnext-catalog-operator-resolution.md)；
三个匿名组合场景见
[`docs/design/vnext/representative-scenarios.md`](docs/design/vnext/representative-scenarios.md)。

## 构建与发布

每个平台 producer 只生成 `relkit.artifact-set/1`。跨平台 Job 通过
`relkit artifacts merge` 合并；product、version、channel 与 resolution digest
任一不一致都会失败。

产品 CI 到 merged ArtifactSet 为止，不持发布 endpoint、上传 Token 或签名私钥。
独立 Operator Job 执行同一 resolution 下的：

```text
stage → simulate → fake verify → publish → verify released
```

artifact、manifest、index、directory、Runtime Profile 或多 backend 一致性任一关键
步骤失败，事务不得报告成功。网页等非关键投影可独立重建。

Runtime Profile 让管理器订阅另一个产品，或让产品自订阅，而不抄 URL、公钥与恢复
入口。首次发布可由 Operator 用 `relkit-ops runtime-profile create` 建立签名 Profile。

## SDK 与组件

distribution 由声明式 registry 生成和验证。常用物化路径：

```text
.relkit/components/sdk/go
.relkit/components/sdk/dart
.relkit/components/sdk/rust
.relkit/components/bindings/ts
.relkit/components/bin/relkit-updater[.exe]
```

Go 宿主使用生成的 `.relkit/components/sdk/go/relkit.mod`；Rust 与 TypeScript 使用
物化 path dependency。项目的长期依赖文件不钉 Relkit 历史版本。

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

将正式 Release 的组件同步到 Operator 声明的内容寻址 mirror：

```bash
relkit-ops distribution mirror \
  --distribution relkit-distribution.json \
  --release-assets-base https://github.com/example/relkit/releases/download/v1.2.3 \
  --mirror-root https://mirror.example/relkit \
  --json
```

命令从 `MIRRORS_USERNAME` / `MIRRORS_PASSWORD` 读取写凭据，逐项校验上游
SHA-256，将 distribution 声明的全部组件及 Release 附件上传到
`blobs/<sha256>`，并在上传后读回校验。产品 Job 与持 Token 的
Operator Job 之间使用 `relkit-ops artifact-set push|pull` 传递不可变 ArtifactSet；
产品 Job 不接触 `RELKIT_UPLOAD_TOKEN`。

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
- [`docs/adr/0019-vnext-breaking-and-protocol-contracts.md`](docs/adr/0019-vnext-breaking-and-protocol-contracts.md)：
  断代与稳定协议合同。
- [`docs/adr/0020-vnext-catalog-operator-resolution.md`](docs/adr/0020-vnext-catalog-operator-resolution.md)：
  Catalog、Operator 与 resolution。
- [`docs/adr/0021-vnext-artifact-runtime-registry.md`](docs/adr/0021-vnext-artifact-runtime-registry.md)：
  ArtifactSet、Runtime Profile 与 registry。
- [`docs/adr/0022-vnext-certification-ops-skill.md`](docs/adr/0022-vnext-certification-ops-skill.md)：
  认证、ops 与 Skill。

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

`internal/vnext` 覆盖合同、签名、resolution、物化、ArtifactSet、Runtime Profile、
发布事务、Operator 原子升级与认证；`conformance/` 覆盖协议和旧客户端窗口。
