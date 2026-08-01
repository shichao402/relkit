# relkit

`relkit` 是 RUP（Release & Update Protocol）的 Go 发布端实现：给业务团队一个可直接分发的单二进制工具，完成 stage / 签名 / 上传 / 提交，无需自写发布脚本。

当前版本：`0.1.0`

曾用过其它语言做过原型；正式实现就是本仓库。见 [`docs/adr/0001-go-only-publisher.md`](docs/adr/0001-go-only-publisher.md)。

## 安装

直接下载 [Releases](https://github.com/shichao402/relkit/releases) 里对应平台二进制，或：

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

- 协议规范与夹具：[`AgentsHelpMe/update-spec`](https://github.com/shichao402/AgentsHelpMe/tree/main/update-spec)
- 操作手册：`relkit agent-guide`（二进制内嵌）
- 配套分发服务：[relkit-serve](https://github.com/shichao402/relkit-serve)

## 开发与测试

```bash
go test ./...
```

覆盖：

- `chain` / `selectors` / `envelope` 的 conformance 夹具回归
- `local` / `static-http` / `http-put` 端到端发布与校验
