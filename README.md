# relkit

RUP（Release & Update Protocol）的 Go 实现仓库：发布 CLI + 自托管分发服务，同一模块、两套单二进制。

| 二进制 | 路径 | 作用 |
|---|---|---|
| `relkit` | `cmd/relkit` | stage / 签名 / 上传 / 提交 |
| `relkit-serve` | `cmd/relkit-serve` | Range 下载 + 鉴权 PUT + 孤儿 GC |

当前版本：`0.2.0`（RUP **protobuf v2** 线格式）

曾用过其它语言做过原型；发布工具正式实现就是本仓库的 Go CLI。见 [`docs/adr/0001-go-only-publisher.md`](docs/adr/0001-go-only-publisher.md)。  
CLI 与 serve 合并决策见 [`docs/adr/0002-one-repo-cli-and-serve.md`](docs/adr/0002-one-repo-cli-and-serve.md)。  
Protobuf 线格式见 [`docs/adr/0003-protobuf-v2-wire-format.md`](docs/adr/0003-protobuf-v2-wire-format.md)（权威副本在 AgentsHelpMe/update-spec）。  
项目版本 SSOT 见 [`docs/adr/0004-project-version-ssot.md`](docs/adr/0004-project-version-ssot.md)：`VERSION.json` + `relkit version …`。

## Agent 开箱（接入项目时先读）

要从零给**另一个产品仓**接入发布工具 + 客户端 SDK，不要从 ADR 或运维手册开始，先读：

**[`docs/agent/README.md`](docs/agent/README.md)**

内含工具链清单、SDK 级联索引，以及只读探测脚本 `docs/agent/bootstrap.ps1` / `bootstrap.sh`。  
各语言 SDK 自己的开箱文在包内：`sdk/AGENT-QUICKSTART.md`（Go）、宿主观 `rup_client/AGENT-QUICKSTART.md`（Dart）。

日常**已接入后的发版**仍用：`relkit agent-guide` / `relkit-serve agent-guide`。

## 安装

从 [Releases](https://github.com/shichao402/relkit/releases) 下载对应平台二进制，或：

```bash
go install github.com/shichao402/relkit/cmd/relkit@latest
go install github.com/shichao402/relkit/cmd/relkit-serve@latest
go get github.com/shichao402/relkit/sdk@latest       # Go 客户端 SDK
go get github.com/shichao402/relkit/version@latest   # 项目 VERSION.json 读写（Go）
```

本地构建：

```bash
go build -o relkit ./cmd/relkit
go build -o relkit-serve ./cmd/relkit-serve
# 或交叉编译 serve：
./deploy/build-serve.sh        # Unix
./deploy/build-serve.ps1       # Windows
```

## relkit（发布 CLI）

```text
relkit init
relkit keygen
relkit version   # get|set|bump|code|path —— 项目版本 SSOT
relkit stage
relkit inspect
relkit simulate
relkit verify
relkit publish
relkit agent-guide
relkit backends
```

已实现后端：`local` · `static-http` · `http-put`

### 快速开始

```bash
relkit init --product demoapp
relkit keygen --key-id k1 --out keys --update-config
relkit version set 1.0.0+100
relkit stage --add dist/demoapp-win-x64.zip os=windows,arch=x64
relkit simulate --with-staged 1.0.0+100 --from all
relkit publish --dry-run
relkit publish
relkit verify --deep
```

`stage` / `publish` 省略版本参数时读 `VERSION.json`；默认 `codeStrategy` 为 `version-build`（code = `+build`）。

## relkit-serve（分发服务）

```bash
relkit-serve init -dir /srv/releases -out /etc/relkit-serve
relkit-serve -config /etc/relkit-serve/relkit-serve.json
```

Linux + systemd：

```bash
sudo ./deploy/install.sh --binary ./dist/relkit-serve-linux-amd64
```

运维手册：`relkit-serve agent-guide`（二进制内嵌），源文件在 [`cmd/relkit-serve/AGENT-GUIDE.md`](cmd/relkit-serve/AGENT-GUIDE.md)。设计说明见 [`cmd/relkit-serve/README.md`](cmd/relkit-serve/README.md)。

## 设计与规范来源

- 协议规范与夹具：[`AgentsHelpMe/update-spec`](https://github.com/shichao402/AgentsHelpMe/tree/main/update-spec)
- 发布侧手册：`relkit agent-guide`
- 服务侧手册：`relkit-serve agent-guide`

## Go / Dart 客户端 SDK

目录约定见 [`sdk/README.md`](sdk/README.md)：

| | |
|--|--|
| Go | `sdk/*.go` → `go get github.com/shichao402/relkit/sdk@latest` · [`sdk/AGENT-QUICKSTART.md`](sdk/AGENT-QUICKSTART.md) |
| Dart | `sdk/dart`（package `rup_client`）· [`sdk/dart/AGENT-QUICKSTART.md`](sdk/dart/AGENT-QUICKSTART.md) |

```go
import "github.com/shichao402/relkit/sdk"

u := &sdk.Updater{
    Product: "myapp", Channel: "stable", CurrentCode: 100,
    IndexURLs: []string{"https://cdn.example.com/index/myapp/stable.pb"},
    TrustedKeys: sdk.TrustedKeys{"k1": pub},
    ClientSelectors: map[string]string{"os": "windows", "arch": "x64"},
}
result := u.Check(ctx)
```

Dart：`rup_client`（git `path: sdk/dart`；级联见 [`docs/agent/sdk-cascade.md`](docs/agent/sdk-cascade.md)）。

## 开发与测试

```bash
go test ./...
```

覆盖：

- `chain` / `selectors` / `envelope` 的 conformance 夹具回归
- `local` / `static-http` / `http-put` 端到端发布与校验（`.pb`）
- `relkit-serve` 的 Range / PUT / GC / 配置加载
- `sdk` 客户端 Check/Download
- `version` 项目 VERSION.json SSOT（get/set/bump/code）
