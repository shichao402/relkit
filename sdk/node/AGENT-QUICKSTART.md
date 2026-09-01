# RUP Node SDK 开箱（Agent 用）

> 目标读者：给一个 **Node / TypeScript / Electron 宿主**接入 relkit 自动更新的 Agent。
> 协议契约在 relkit 仓库 [`SPEC.md`](../../SPEC.md)，结构 SSOT 在 [`proto/rup/v2/`](../../proto/rup/v2)。

## N0 何时用本文档

宿主进程是 Node（含 Electron 主进程、CLI、服务端）时用本文档。
宿主是 Go 用 [`../AGENT-QUICKSTART.md`](../AGENT-QUICKSTART.md)，Dart/Flutter 用 [`../dart/AGENT-QUICKSTART.md`](../dart/AGENT-QUICKSTART.md)。

多语言宿主以**需要检查更新的那个进程**为准。Electron 应用请在**主进程**接入：渲染进程没有文件系统与网络策略的完整权限，把更新逻辑放那里会在打包后才失败。

前置：发布侧至少已 `relkit publish` 过一个版本。若仓库根还没有 `VERSION.json` / `relkit.json`，先做
[`../../docs/agent/toolchain-onboard.md`](../../docs/agent/toolchain-onboard.md)。

## N1 安装

包名 `rup-client`，SSOT 是 `sdk/node/`。尚未发布到 npm registry，用本地路径或 git 依赖：

```jsonc
// 宿主 package.json
{
  "dependencies": {
    "rup-client": "file:../relkit/sdk/node"
  }
}
```

首次使用需要在 `sdk/node/` 内构建一次（生成代码已检入，无需 protoc）：

```bash
cd sdk/node
npm install
npm run build      # tsc -> dist/
npm test           # 跑 conformance + 单测，42 个
```

要求 Node >= 20（用到内置 Ed25519 与全局 `fetch`）。包是 ESM，CommonJS 宿主需用 `await import()`。

## N2 最小可用片段

```ts
import {
  FileUpdateStateStore,
  RupUpdater,
  TrustedKeys,
} from "rup-client";

const updater = new RupUpdater({
  product: "cronkit",
  channel: "stable",
  currentCode: 1,                       // 见 N3，禁止传 0
  trustedKeys: TrustedKeys.fromBase64({
    "cronkit-2026": "<base64 32 字节 ed25519 公钥>",
  }),
  clientSelectors: { os: "windows", arch: "x64" },
  entryUrls: ["https://raw.firoyang.com/rup/directory/cronkit.pb"],
  stateStore: new FileUpdateStateStore({
    directory: app.getPath("userData"),
    product: "cronkit",
    channel: "stable",
  }),
});

const result = await updater.check({ force: true });

switch (result.kind) {
  case "update-available": {
    const verified = await updater.download(result, {
      destinationDir: stagingDir,
      onProgress: (p) => console.log(p.fraction, p.bytesPerSecond),
    });
    // verified.path 的 sha256 已与签名 manifest 一致，可以交给宿主安装
    break;
  }
  case "fallback-required":
    // 只展示 message 并打开 manualUrl，禁止自动下载
    break;
  case "up-to-date":
  case "check-throttled":
    break;
  case "check-failed":
    console.warn(result.reason, result.attempts);
    break;
}
```

`check()` 的返回是 discriminated union，按 `kind` 分支。`attempts` 是唯一能区分
「服务器挂了」与「服务器正常但拒绝了我们」的东西，出问题先看它。

## N3 字段对齐（与发布侧必须逐字一致）

| 字段 | 取值来源 | 写错的表现 |
|---|---|---|
| `product` | `relkit.json` / `VERSION.json` | 整份 index 被拒 |
| `channel` | 发布时的 `--channel` | 拉错指针或 404 |
| `currentCode` | `VERSION.json` 的 `+build` 整数 | 选路错误 / 永远 up-to-date |
| `entryUrls` | 后端 `baseUrl` + `directory/<product>.pb` | check 失败 |
| `trustedKeys` | `relkit keygen` 的公钥，**编译期内嵌** | 验签失败 |
| `clientSelectors` | 与 `relkit stage --add … os=,arch=` 一致 | no artifact matches |

三个容易踩的点：

`currentCode` 禁止传 0。开发态（未打包）请传一个高于所有已发布 code 的值，
例如 `app.isPackaged ? BUILD_CODE : 2147483647`。传 0 会让开发构建乐于把自己替换成正式版（SPEC §8.1）。

`arch` 用 `x64` 而不是 `amd64`，对齐 [`SPEC.md`](../../SPEC.md) §11.1 的标准取值表。

`trustedKeys` 只能编译期内嵌，禁止运行时下载。建议同时内嵌两把（当前 + 备用）以便轮换。
写法上三种都接受，按手头的形态选，不必先转换：

```ts
trustedKeys: { "cronkit-2026": "<base64>" }            // 字符串按 base64 解
trustedKeys: { "cronkit-2026": rawBytes }              // 32 字节 Uint8Array
trustedKeys: TrustedKeys.fromBase64({ ... })           // 预构建，多个 updater 共享时省一次解码
```

## N4 能力边界

SDK 已做（宿主不要重做一遍）：

- Index / directory / fallback：Ed25519 验签 + sequence 防回滚
- Manifest / artifact：size + sha256（先 size 再 hash）
- 镜像串行回退，禁止竞速；可变文档带 cache-buster，不可变对象不带
- 下载：Range 多连接并行 + `.part` / `.part.meta` 续传，不支持 Range 自动降级
- 节流（成功 24h / 失败 1h）与源学习排序
- artifact 文件名安全校验（SPEC §14.4）

宿主负责：

- 何时检查、UI、是否安装、是否 skip
- **Apply**。本包**不含** apply：Electron 换目录要解决占用文件、稳定 launcher 路径、
  单实例锁与开机自启注册，全都不是协议内容且逐宿主不同。拿 `verified.path` 自己装。

## N5 Done 判定

- [ ] `npm test` 在 `sdk/node/` 全绿（conformance 13 项 + 单测 29 项）
- [ ] 用比 head 更小的 `currentCode` 调 `check` → 得到 `update-available`
- [ ] `download` 得到的文件 sha256 与 manifest 一致
- [ ] 断网 → 失败且不留下「通过校验」的假文件
- [ ] 故意换错公钥 → 验签失败且拒绝使用，不降级为不验签
- [ ] `product` 改错一个字 → 整份 index 被拒

夹具是 v1 JSON，验证的是**语义**对齐。上线前请另做一次**格式**对齐：
用 `relkit publish` 真实写出的 v2 protobuf 发布树（`local` 后端 + 本机静态 HTTP 即可）
跑一遍 check → download，确认 SDK 能读 Go 侧真正产出的字节。本次接入已用此法抓到两个夹具覆盖不到的缺陷。

## N6 排障

**`no artifact matches selectors`**：`clientSelectors` 与 stage 时的键值不一致。
最常见是 `arch=x64` 对 `arch=amd64`。用 `matchingArtifacts()` 打印候选集。

**`signature check failed (notVerified)`**：公钥不对，或验签对象错了。
本 SDK 对 `Envelope.payload` 的原始字节验签；若你自己实现过一版并对重新序列化的对象验签，
会表现为签名「无缘无故」失效。

**`sequence N is older than the last accepted M`**：镜像滞后，不是错误，
SDK 会静默换下一源。若所有源都滞后，等复制追上。

**永远 `up-to-date`**：`currentCode` 传成了高值（开发态逻辑漏到了打包构建里），
或 index 里 head 的 `minFrom` 高于当前 code。

**`check-throttled`**：正常节流。用户主动点「检查更新」要传 `force: true`。

## Fallback

本文档解决不了的问题，按顺序读：
[`../../SPEC.md`](../../SPEC.md) → [`../../docs/agent/sdk-cascade.md`](../../docs/agent/sdk-cascade.md) →
[`../dart/AGENT-QUICKSTART.md`](../dart/AGENT-QUICKSTART.md)（Dart 是行为参照实现）。
行为分歧一律以 `conformance/` 夹具为准。
