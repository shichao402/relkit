# 发布工具设计（暂定名 `relkit`）

> 状态：vNext 单一路径（ADR 0019 §1 断代已执行）。
> 正式实现是 **Go 单二进制**：[github.com/shichao402/relkit](https://github.com/shichao402/relkit)。

本文描述发布侧（产品仓、Operator、接入方）的对外契约。协议本身以 [`SPEC.md`](SPEC.md) 为准，本文不引入 SPEC 之外的语义。

v0.5 控制面（`relkit.json` 发布配置、consume lock、`scripts/host`、Python gates、onboarding 账本、`relkit ci` / `cas-put` / `staged-put` / `simulate` / `yank` 等命令）已整体删除，不存在兼容分支。历史形态见 [`docs/_pending/`](docs/_pending/README.md) 与 ADR 0011 / 0012 / 0017。

---

## 1. 硬约束

1. **Go 单二进制，标准库为主。** 业务侧 CI 与本地直接跑 `relkit`，不必 vendor 源码、不必自写上传逻辑。
2. **可单文件分发。** `go build` / GitHub Releases 多平台二进制；不要求 `go install`。
3. **Ed25519 用运行时原生实现**（`crypto/ed25519`）。签名对象是文档的原始字节，见 SPEC §4。
4. **产品仓只声明身份，不保存部署事实。** `relkit.project.json` 只写产品名、服务器地址与服务器根公钥，不写 relkit 版本、摘要、Token、SSH 或私钥（需求 12.1）。
5. **服务器是唯一权威。** 用哪个版本的 relkit、工具、Runtime Profile，全部由服务器决定；宿主只问 `current` 别名。
6. **协议按合同窗口兼容**：RUP Schema Set `rup.v2`、Publish Protocol `[2,3]`、Updater IPC `[4,4]`、ArtifactSet `relkit.artifact-set/1`。不承诺跨窗口混搭。

---

## 2. 两段式流程

发布拆成两段，每段只持它必须持的东西：

```text
产品 CI（每平台）                      Operator Job（同一工作区）
────────────────                       ─────────────────────
artifacts produce → artifact-set.json
                     │（普通文件传输）
                     ├──────────────▶  解包 ArtifactSet
                     │                 stage → release plan → release execute
```

- **产品 CI** 只做 produce。不持发布 endpoint、不持上传 Token、不持签名私钥。
- **Operator Job** 持 token，并在**同一工作区**完成 stage 之后的全部动作。stage 以
  `artifact-set.json` 所在目录为产物根逐字节校验，staged 树又落在项目根下，所以
  stage 与 release 不能拆到两个 job 里。
- 平台之间只传 ArtifactSet 这一个普通目录，编排用 mirrors Generic 仓库的 zip 传，
  刻意不引入任何 relkit 子命令。

### 2.1 工具从哪里来

宿主与 CI 都不持有 relkit 二进制。取工具固定三步，代码在各宿主的
`scripts/relkit_tool.py`：

1. 读 `relkit.project.json` 的 `server` 与 `rootPublicKey`；
2. `GET <server>/v1/tools/current/<component>/<platform>` 与其 `/signature`；
3. 用构建机自带的 `ssh-keygen -Y verify -n relkit-tool` 对照 `<公钥行>` 验签，通过才用。

---

## 3. 命令清单

| 命令 | 联网 | 作用 |
|---|---|---|
| `relkit artifacts` | 否 | `produce` 单平台出 `relkit.artifact-set/1`；`merge` 合并多平台 |
| `relkit bootstrap` | 是 | `build` 产出 `relkit.runtime-bootstrap/2`，由宿主构建时内嵌 |
| `relkit payload` | 否 | `build` 把树打成 payload zip 并出文件表（ADR 0013） |
| `relkit stage` | 否 | 由 ArtifactSet 生成不可变 staged 树 |
| `relkit inspect` | 否 | 打印某个 protobuf / JSON 文档，供人读 |
| `relkit release` | 是 | 发布事务门：plan（dry-run）/ `--execute` |
| `relkit build` | 否 | 按 components registry 交叉编译组件二进制与 Release 附件 |
| `relkit custody` | 否 | 按 keyId 读凭据清单，报告载体与可用性（不含秘密） |

只有这 8 个命令。`relkit --version` 打印版本。

`resolve` / `doctor` / `materialize` / `wrapper` / `verify` 五个命令已随
「宿主持有 resolution」模型一起删除（ADR 0023）。再出现即视为回退。

---

## 4. 命令详解

> **历史章节（已失效）。** 下面 4.1 `resolve`、4.2 `doctor`、4.3 `materialize`、4.4
> `wrapper`、4.9 `verify released` 五节描述的是「宿主持有 resolution」时代，相关命令
> 已随 ADR 0023 删除，执行会直接报 `unknown command`。现行命令面只有第 3 节列出的 8
> 个；断代后的接入方式见 README 的「宿主接入」与 ADR 0023。

### 4.1 `relkit resolve`

按 `relkit.project.json` 声明的 Operator 与能力需求，从 Catalog 选出唯一 distribution，产出 immutable resolution 与签名 bundle。

- `--project`：默认 `relkit.project.json`
- `--operator` / `--catalog`：显式指定签名文档，默认按 `operatorRef` 从 bootstrap 映射解析
- `--trust-key` / `--candidate-trust-key`：可重复；根公钥
- `--candidate-root-fingerprint`：candidate 车道锚定
- `--out`：默认 `.relkit/resolution.json`，也可用 `RELKIT_RESOLUTION` 环境变量
- `--bundle-out`：默认与 resolution 同目录的 `resolution-bundle.json`
- `--watermarks`：带序号的防回滚水位
- `--replace`：允许覆盖既有 resolution
- `--candidate <digest>`：以指定 candidate 作为候选输入
- `--runtime-profile <product>=<path>` / `--runtime-profile-root <product>=<key>`：订阅其他产品的 Runtime Profile
- `--min-profile-sequence` / `--min-catalog-sequence`：最小序号门

同一组输入必须得到同一 digest。已存在的 resolution 默认复用，不静默重算。

### 4.2 `relkit doctor`

只读诊断，不改盘、不发网络写请求。检查 resolution 与 bundle 的签名、Operator deployment、identity、Runtime Profile 与能力合同是否与 `relkit.project.json` 一致。

- `--resolution` / `--resolution-bundle` / `--project` / `--operator`
- `--candidate-root-fingerprint`
- `--runtime-profile` / `--runtime-profile-root`
- `--json`：结构化输出，供 CI 与其他工具消费

失败时给出稳定 `code`，不要求人去翻 Skill 日志。

### 4.3 `relkit materialize`

按 resolution 摘要把组件物化到 `.relkit/components/`，并校验每份资产的 SHA-256。

- `--resolution`：默认同 `resolve`
- `--project-root`：默认当前目录
- `--target`：默认 `host`
- `--json`

常用落点：

```text
.relkit/components/bin/relkit[.exe]
.relkit/components/bin/relkit-updater[.exe]
.relkit/components/sdk/go
.relkit/components/sdk/dart
.relkit/components/sdk/rust
.relkit/components/bindings/ts
```

宿主不 clone Relkit，不从 PATH 猜 CLI，也不读任何 consume lock。

### 4.4 `relkit wrapper`

生成 `relkitw`（POSIX shell）与 `relkitw.cmd`（Windows）。文件头声明不可定制：改配置要改 `relkit.project.json` 后重新生成。

- `--project`：默认 `relkit.project.json`
- `--out-dir`：默认当前目录

`relkitw` 负责定位 bootstrap、校验 resolution、必要时先 materialize 再执行目标子命令。bootstrap 解析顺序：`RELKIT_BOOTSTRAP_CLI` → 仓内 `.relkit/bootstrap/relkit` → `RELKIT_HOME` → 平台默认 operators 目录 → `RELKIT_BOOTSTRAP_URL` + `RELKIT_BOOTSTRAP_SHA256`。

### 4.5 `relkit artifacts`

两个子命令，输出都是 `relkit.artifact-set/1`：

- `produce --version <v> --channel <dev|stable> --target <os-arch>`：单平台产出。`--out` 指定文件，`--out-dir` 指定目录；缺省落到 `<resolution 目录>/artifact-sets/<version>/<target>/artifact-set.json`
- `merge --in A --in B [--in C ...] --out merged.json`：至少两份输入。product、version、channel 与 resolution digest 任一不一致即失败

产物内容由 `relkit.project.json` 的 `artifactProducer` 命令提供，relkit 不识别仓库身份。

### 4.6 `relkit payload build`

把一棵目录打成 payload zip，并输出 `relkit.payload/1` 文件表（ADR 0013）：

```bash
relkit payload build --tree dist/app --out dist/app-payload.zip
```

内部更新走完整文件表，不做二进制差分。

### 4.7 `relkit stage`

由 ArtifactSet 与 resolution 生成 staged 树。**`--artifact-set` 是唯一输入形态**；`relkit.json` / release-policy 的 staging 面已删除。

- `--artifact-set`：必填
- `--project`：默认 `relkit.project.json`
- `--resolution`：默认 `.relkit/resolution.json`
- `--resolution-bundle`：默认同目录 `resolution-bundle.json`
- `--candidate-root-fingerprint`：默认取 `RELKIT_CANDIDATE_ROOT_FINGERPRINT`
- `--code`：正整数；缺省按 `VERSION.json` 的 code 策略
- `--min-from`：最小可升级 code
- `--notes-url`：变更说明链接

产出落在 `.relkit/cache/staged/<version>/`，含五件套：

```text
staged.pb              签名 staged 文档
resolution.json        immutable resolution
resolution-bundle.json 签名 distribution bundle
relkit.project.json    产品声明快照
artifact-set.json      ArtifactSet 快照
```

旧 staged 树里的 `release-policy.json` 不再产出，agent 也拒绝缺 resolution digest 的 staged 树。

### 4.8 `relkit inspect`

打印一个已解析的文档，供人读，不做信任判定。

- 无参数或 `--resolution`：打印本次选中的 resolution
- `--file <path>`：打印 staged / manifest / index / envelope / key 等 protobuf 文档
- `--json` / `--raw`

信封签名不会被 inspect 重新验证；要验证信任用 `relkit verify released`。

### 4.9 `relkit verify released`

对已发布版本做端到端校验：resolution bundle 签名、distribution/product root 归属、index sequence 单调、manifest 摘要与实际对象一致。

```bash
relkit verify released --version <v> --channel <dev|stable> --resolution <path>
```

- `--product-root`：可重复；不给则从 bootstrap 映射取
- `--consumer` / `--directory`：覆盖 Operator Profile 里的端点
- `--resolution-digest` / `--operator`：显式锚定，不匹配即失败
- `--min-index-sequence` / `--min-directory-sequence`：防回滚
- `--deep` / `--to`：深校验与链式目标

校验失败以非零退出码结束；`NOTE` / `WARN` 不影响成功判定。

### 4.10 `relkit release`

发布事务门。默认 dry-run，打印发布计划；`--execute` 才真正发布。

- `--version`：必填
- `--project` / `--resolution`
- `--allow-backfill`：允许序号回退的逃生门，日常不用
- `--json`

`--execute` 前置条件：`RELKIT_RELEASE_VIA_CI=1`，且签名 Operator 的 `secretRefs` 指向的环境变量里有凭据。执行序：按摘要上传缺失 CAS blob → 上传只含 vNext 控制面文档的瘦 staged tar → `POST /v1/publish`。

agent 端点取自签名 Operator 的 `endpoints.agent`。CI 网络需要走不同入口时用 `RELKIT_AGENT_TRANSPORT_URL` 显式覆盖（绝对 `http(s)` URL，无凭据、无 query、无 fragment），同样要求 `RELKIT_RELEASE_VIA_CI=1`。

### 4.11 `relkit build`

按 `internal/vnext/registry/components.json` 交叉编译组件与 Release 附件。构建命名只有一个来源，不再有 `release-contract.json`。

- `--project-root`：默认当前目录
- `--out`：默认 `dist`
- `--os` / `--arch`：单目标
- `--all`：全部目标
- `--check-tag <tag>`：校验 tag 与 `VERSION.json` 一致
- 组件开关来自注册表的 `build.flag`：`--store` `--console` `--agent` `--ops` `--cli` `--updater` `--dart-sdk` `--go-sdk` `--rust-sdk` `--bindings-ts`

未在注册表登记的 flag 直接报错。

---

## 5. 配置文件

### 5.1 `relkit.project.json`（产品仓唯一声明）

schema `relkit.project/1`。允许的字段只有下列这些，读到未知字段直接报错：

| 字段 | 必填 | 说明 |
|---|---|---|
| `product` | 是 | 产品 id，发布与目录文档的键 |
| `server` | 是 | 唯一写面地址（如 `http://publish.devcloud.woa.com`） |
| `rootPublicKey` | 是 | 服务器根公钥，OpenSSH 单行格式 |
| `channels` | 否 | 支持的渠道；声明了就只接受其中列出的 |
| `selectorDimensions` | 否 | 产物选择维度，断代后只有 `["os-arch"]` |
| `artifactProducer` | 否 | 出 ArtifactSet 的命令与 env allowlist |
| `versionProvider` | 否 | 出产品版本的命令（可空，缺省读 `VERSION.json`） |
| `site` | 否 | 可选人页声明；省略即不发人页文档 |

`operatorRef`、`trustFingerprint`、`roles`、`requiredContracts`、`capabilityFingerprint`、
`runtimeProfileSubscriptions`、`runtimeProfilePublication`、`recovery`、`osArch`、
`facades`、`projections`、`topology`、`versionPolicy` 全部已随 resolution 模型删除。

### 5.2 `VERSION.json`（版本 SSOT）

仓库根必须提供，与 `relkit.project.json` 同级（ADR 0004）。code 派生策略由 `versionPolicy` 决定：`explicit` / `semver` / `version-build`。宿主语言不得自备版本号来源。

### 5.3 operator 侧配置

发布机配置（`/etc/relkit-agent/relkit-agent.json`）与产品 profile（`/etc/relkit-agent/products/<product>.json`）属 Operator，不进产品仓。字段说明见 [`cmd/relkit-agent/README.md`](cmd/relkit-agent/README.md)。

**产品仓不保存**：Relkit 版本、服务 URL、Token、SSH、mirror 凭据、私钥。凭据一律通过 `*Env` 字段引用环境变量名。

### 5.4 凭据与私钥

- 配置里的 `*Env` 字段只给变量名，值来自环境。
- `signing.privateKeyPath` 供 Operator 侧使用，指向的文件禁止入库。
- `signing.publicKeys` 列出受信公钥，是客户端要内嵌的内容；轮换期同时列新旧两把，对应 SPEC §4.1 的多签名条目。

---

## 6. 后端与端点

### 6.1 只有两种后端类型

`internal/backends` 只接受：

| type | 用途 |
|---|---|
| `s3-compatible` | 对象存储（COS / S3 / MinIO）。CI 与 agent 都可用 |
| `relkit-compatible` | `relkit-store` 数据面。内网主用 |

`local` / `http-put` / `static-http` 已删除，配置里出现即报 `unsupported backend type`。

切面不因 type 分叉：CI、`publish.Run`、客户端看到的接口对所有后端相同。禁止在 publish 或 CI 脚本里写 `if backend.Type()=="s3-compatible"`。

### 6.2 `relkit-compatible`

```json
{
  "type": "relkit-compatible",
  "baseUrl": "http://127.0.0.1:8080/",
  "uploadUrl": "http://127.0.0.1:8080/",
  "tokenEnv": "RELKIT_SERVE_TOKEN",
  "timeoutSeconds": 600
}
```

`uploadUrl` 可选，缺省等于 `baseUrl`。

### 6.3 CAS 上传凭据文档

`POST /v1/cas/credentials` 按 blob 返回 `requests[]`：每项含 `method`、绝对 `url`、可选 `headers` 与 `expiresAt`；分片再加 `offset` / `length`。`relkit release --execute` 的上传阶段逐项执行，不认识后端类型，不实现 SigV4 / TC3 / STS，也不接受 `sign` 字段。目的地只有该产品的 primary ingest 一个。

### 6.4 推荐对外入口（自有域名 + COS）

```text
https://updates.<your-domain>/directory/<product>.pb
https://updates.<your-domain>/index/<product>/<channel>.pb
https://updates.<your-domain>/manifest/...
https://updates.<your-domain>/artifact/...
```

`baseUrl` 配成 `https://updates.<your-domain>/`（末尾 `/`），与 SPEC §3 路径型后端一致。发布机不必出现在客户端 `entryUrls` 里。

### 6.5 `s3-compatible`

```json
{
  "type": "s3-compatible",
  "endpoint": "https://cos.ap-guangzhou.myqcloud.com",
  "bucket": "myapp-release",
  "prefix": "rup/",
  "baseUrl": "https://dl.example.com/rup/",
  "accessKeyEnv": "COS_SECRET_ID",
  "secretKeyEnv": "COS_SECRET_KEY",
  "region": "ap-guangzhou",
  "forcePathStyle": false,
  "timeoutSeconds": 600
}
```

`region` / `forcePathStyle` / `timeoutSeconds` 可选。公网产品的 primary ingest 一般是这一种。

### 6.6 `relkit-agent`（发布机）

CI 只 `stage`，发布机持钥发布。二进制：`cmd/relkit-agent`。运维命令与路径见 [`cmd/relkit-agent/README.md`](cmd/relkit-agent/README.md)。

- 发布配置：staged vNext 控制面 + `/etc/relkit-agent/products/<product>.json`（缺一不可）
- 产品挂载与 token：`relkit-ops agent policy --config <agent-cfg> --policy <relkit.agent-product-policy/1>`；机上轮换用 `relkit-agent init -config <cfg> -product <id> -token-only`
- `PUT /v1/drop/{product}/{version}/{filename}`：多平台 Job 交换 build-scoped zip（Bearer；GET/HEAD/DELETE 同样鉴权）
- `PUT /v1/staged/{product}/{version}`：staged 目录的 tar.gz（Bearer；整包与瘦包两条路径都收）
- `POST /v1/cas/credentials`：为缺失 blob 返回唯一 ingest 的 `requests[]`。协议 3 且对象大于一片时，COS/S3 改为多条带 `offset` / `length` 的分片 URL，客户端把 `ETag` 交回 `POST /v1/cas/complete`
- `POST /v1/publish`：触发 `publish.Run`（按 product 串行 + 幂等键）
- `POST /v1/unpublish`：删节点 + 重签提交指针 + 重投影 site/latest/channel + 删 release 文档。CAS/manifest 孤儿由 store GC 回收。与 publish 共产品锁，并发 409。CLI：`relkit-agent unpublish -product <id> -version <v> [--dry-run]`
- `GET /-/health`

`relkit cas-put` 与 `relkit staged-put` 两个旧子命令已随 ADR 0019 §1 删除，其职责并入 `relkit release --execute`。

部署样例与 Caddy 反代见 [`scripts/deploy/`](scripts/deploy/)。设计说明：[`docs/design/publish-agent.md`](docs/design/publish-agent.md)。

---

## 7. CI 集成

CI 不持签名私钥，也不持长期后端写密钥。Runner 在 `relkit stage` 之后用 `relkit release --execute` 一次完成上传与提交；整包 `PUT /v1/staged` 路径仍兼容。

```yaml
- name: Produce ArtifactSet
  run: |
    relkit artifacts produce --version "${VERSION}" --channel "${CHANNEL}" \
      --target windows-amd64 --out dist/artifact-set.json

- name: Stage
  run: relkit stage --artifact-set dist/artifact-set.json

- name: Release via relkit CLI
  env:
    RELKIT_RELEASE_VIA_CI: "1"
  run: relkit release --execute --version "${VERSION}"
```

跨地域直传时把片调小、在途路数调低，用 `RELKIT_UPLOAD_PART_SIZE`（缺省 8MiB）与 `RELKIT_UPLOAD_CONCURRENCY`（缺省 4，片和 blob 共用），命令仍是同一条 `relkit release --execute`。

`RELKIT_RELEASE_VIA_CI=1` 是 `--execute` 的前置门：本机 shell 不持 Operator 凭据，不得发布；未设该变量时 `relkit release` 为 dry-run，打印发布计划后返回。决策记录见 [ADR 0017](docs/adr/0017-cli-owns-consume-source-channel.md) 与 [ADR 0019](docs/adr/0019-vnext-breaking-and-protocol-contracts.md)。

不要在 CI 里设置 `RELKIT_PRIVATE_KEY` 或任何签名私钥环境变量。设计说明：[`docs/design/publish-agent.md`](docs/design/publish-agent.md)。

---

## 8. 实现位置

正式代码在 [github.com/shichao402/relkit](https://github.com/shichao402/relkit)：

```text
cmd/relkit/                 CLI 入口（11 个命令）
cmd/relkit-agent/           发布机控制面
cmd/relkit-store/           内网数据面
cmd/relkit-ops/             Operator 编排
internal/chain/             选路与可达性（SPEC §9、§10）
internal/selectors/         产物选择（SPEC §11）
internal/envelope/          签名信封（SPEC §4）
internal/model/ jsonio/     对象构造、确定性序列化
internal/config/ keys/      配置与密钥
internal/stage/ publish/    固化与发布编排
internal/verify/            已发布产物校验
internal/httpx/ backends/   HTTP 与 relkit-compatible / s3-compatible
internal/vnext/             vNext 控制面（catalog / operator / resolution / registry / release）
e2e/                        离线与真实 HTTP 端到端
testdata/conformance/       协议夹具副本（与本目录同步）
```

`chain` / `selectors` / `envelope` 是规范性行为的权威实现，conformance 测试直接加载本目录（或仓库内 `testdata/conformance`）的 JSON 夹具。

组件声明的唯一来源是 [`internal/vnext/registry/components.json`](internal/vnext/registry/components.json)。ADR 0019 §1 已删除 `internal/registry`、`internal/consume`、`scripts/host` 与 Python `facets.py`。
