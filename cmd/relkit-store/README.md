# relkit-store

> 本目录是 [`go.firoyang.com/relkit`](https://github.com/shichao402/relkit) 仓库的一部分（`cmd/relkit-store`），与发布 CLI 同仓。设计决策见 [ADR 0016](../../docs/adr/0016-serve-split-store-console.md)。

relkit-serve 拆分出的**存储面**（ADR 0016 第 3 步）：`/` 树的 GET/HEAD/PUT/DELETE、Range 下载、CAS 能力上传（mint → 带 sig 的 PUT → COPY promote）、孤儿 GC、上传令牌、发布协议协商与 preflight。单个静态链接的可执行文件，协议与配置形状与 serve 完全一致——客户端、CI、agent 不感知换了二进制。

**不再有的能力**（拆给了 console）：`/-/admin` 操作面板、账户/会话、`/-/p/` 产品页、`/-/latest/` 跳转、下载统计展示。管理面见 [`cmd/relkit-console`](../relkit-console/README.md)。

## 快速开始

```bash
# 生成配置骨架与运营方 token
relkit-store init -dir /srv/releases -out /etc/relkit-store

# 起服务
relkit-store -config /etc/relkit-store/relkit-store.json

# 产品 token 签发/共享/吊销（与 serve 同一套子命令）
relkit-store init -product demoapp -out /etc/relkit-store
relkit-store init -list-products -out /etc/relkit-store
```

Linux + systemd：`sudo python3 scripts/deploy/relkit.py install serve --binary ./dist/relkit-store-linux-amd64`。

## 路由面

| 路径 | 方法 | 语义 |
|---|---|---|
| `/-/health` | GET | protobuf 健康检查（无鉴权） |
| `/-/version` | GET | 版本串 |
| `/-/publish/preflight` | POST | 发布协议协商（`Protocol` header 窗口） |
| `/-/cas/uploads` | POST | CAS 能力 mint（运营方 Bearer，返回带 sig 的 PUT URL） |
| `/`（树） | GET/HEAD | 匿名读 + Range；`Cache-Control` 按前缀（`index/` 等 no-cache，`manifest/`/`artifact/` immutable） |
| `/`（树） | PUT/DELETE | 运营方或产品 Bearer；`X-Relkit-Copy-Source` 走 COPY promote |
| `browse/<name>` | GET | 静态目录页（agent site-rebuild 的 dump 落点） |

根目录无 dump 时返回一页 catalog stub（无面板链接）；目录浏览的完整面板在 console。

## 命名兼容（迁移盒子不重配）

| 项 | 值 | 说明 |
|---|---|---|
| 环境变量 | `RELKIT_SERVE_TOKEN` | systemd drop-in 换二进制后继续可用 |
| admin state | `<dir>/.relkit-serve-admin.json` | console 继续持有，账户不丢 |
| 下载计数 | `<dir>/.relkit-serve-stats.json` | console 只读共享 |
| CAS 签名密钥 | `<dir>/.relkit-serve-cas.key` | 能力 URL 验签密钥 |

配置文件与 token 文件改用 `relkit-store.json` / `relkit-store.token`（新装机）；`/etc/relkit-serve` 时代的老盒子用 `upgrade` 路径迁移时由部署脚本处理。

## 测试

```bash
go test ./cmd/relkit-store/
go test -bench . -benchtime 3s ./cmd/relkit-store/   # loopback 吞吐基准
```
