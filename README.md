# relkit

`relkit` 是 RUP（Release & Update Protocol）的 Go 发布端实现，目标是替代 `AgentsHelpMe/update-spec/relkit/` 里的 Python CLI，给业务团队提供一个可直接分发的单二进制工具。

当前版本：`0.1.0`

## 安装

直接下载发布页里的对应平台二进制，或自行安装：

```bash
go install github.com/shichao402/relkit/cmd/relkit@latest
```

本地开发构建：

```bash
go build -o relkit ./cmd/relkit
```

## 已实现命令

```text
relkit init
relkit keygen
relkit stage
relkit inspect
relkit simulate
relkit verify
relkit publish
relkit agent-guide
relkit backends
```

已实现后端：

- `local`
- `static-http`
- `http-put`

## 快速开始

1. 初始化项目配置：

```bash
relkit init --product demoapp
```

2. 生成签名密钥，并把公钥写入 `relkit.json`：

```bash
relkit keygen --key-id k1 --out keys --update-config
```

3. 在 `relkit.json` 里给当前机器补上私钥路径，或改为使用环境变量：

```json
{
  "signing": {
    "keyId": "k1",
    "privateKeyEnv": "RELKIT_PRIVATE_KEY",
    "privateKeyPath": "keys/k1.private.json"
  }
}
```

4. 固化待发布产物：

```bash
relkit stage 1.0.0 --code 100 \
  --add dist/demoapp-win-x64.zip os=windows,arch=x64 \
  --add dist/demoapp-mac-arm64.zip os=macos,arch=arm64
```

5. 发布前先看升级路径并做 dry-run：

```bash
relkit simulate --with-staged 1.0.0 --from all
relkit publish 1.0.0 --dry-run
```

6. 正式发布并校验：

```bash
relkit publish 1.0.0
relkit verify
relkit verify --deep
```

`local` 后端会把完整的发布树写到 `dist/publish/`；如果你已经有静态托管或仓库 CI，可以切到 `static-http`；如果你用 `relkit-serve` 或任意 PUT / WebDAV 端点，可改用 `http-put`。

## 设计与规范来源

本仓库实现遵循以下 SSOT（目前仍在 AgentsHelpMe 工作区里评审）：

- 协议规范：[`AgentsHelpMe/update-spec/SPEC.md`](https://github.com/shichao402/AgentsHelpMe/tree/main/update-spec/SPEC.md)
- CLI 设计：[`CLI.md`](https://github.com/shichao402/AgentsHelpMe/tree/main/update-spec/CLI.md)
- 一致性夹具：[`conformance/`](https://github.com/shichao402/AgentsHelpMe/tree/main/update-spec/conformance)
- 操作手册：`relkit agent-guide`（二进制内嵌）
- 配套分发服务：[relkit-serve](https://github.com/shichao402/relkit-serve)

## 开发与测试

运行全部测试：

```bash
go test ./...
```

当前测试覆盖：

- `chain` / `selectors` / `envelope` 的 conformance 夹具回归
- 本地 `local` 后端完整发布流
- `static-http` 的真实 HTTP 校验
- `http-put` 的带鉴权上传与读回校验

## 与 Python 版的关系

Python 版 `update-spec/relkit/` 仍然保留为设计与行为参考；Go 版 `relkit` 才是面向生产分发的单二进制 CLI。
