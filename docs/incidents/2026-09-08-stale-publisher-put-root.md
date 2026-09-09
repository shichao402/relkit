# INC-2026-09-08 过期 publisher 对 agent 发出 `PUT /`

- Date: 2026-09-08
- Status: Mitigated
- Products: relkit (control/data plane), SvnMergeTool (consumer CI)
- Related: [ADR 0009](../adr/0009-publisher-protocol-negotiation.md), consumer [ADR-007](https://git.woa.com/osgame-client/SvnMergeTool)

## 症状

SvnMergeTool-dev 在 CAS 上传阶段失败。蓝盾日志只有 `relkit cas-put ... 失败 (exit 2)`，nginx 侧出现 `PUT / HTTP/1.1` 403/400（`cannot write to a directory path`）。多次被误判为「CI 透明代理改写了 PUT」。

## 根因

检入的 `tools/bin/relkit-*` 停在 `4bf302b` 之前的契约：CAS 凭据文档里找 `putUrl`。agent 已改为 `requests[]` 能力 URL，旧客户端读到空字符串，相对 agent origin 解析成 `PUT /`，自己把请求打到 nginx。

**不是**代理改写、不是必须改成裸 IP、也不是要把 CAS 正文经 agent 转发。那些假设会把内网拓扑和外网 COS 拆开，禁止再走。

## 误判清单（禁止再写进文档/脚本）

| 假设 | 为什么错 |
|---|---|
| BlueShield 透明代理把 PUT 改写成 `/` | 没有任何中间件会好心改你的 path；path 来自我们自己的客户端 |
| `NO_PROXY` / `unset HTTP_PROXY` 能根治 `PUT /` | 最多影响出站代理，不能修空 `putUrl` |
| 数据面改成裸 IP 才能绕过 nginx | 正确拓扑是 hostname:8080 直连 serve；nginx `/` 只保留旧签名 GET |
| 在 aggregate Job 现编 CLI 就能替代握手 | 源码构建有独立失败面（sparse 丢掉 `go.mod`）；握手必须先拒绝旧客户端 |

## 修复

1. publisher 声明协议头；agent/serve 在鉴权后校验窗口，无交集返回 426。
2. 旧客户端不再拿到它读不懂的凭据文档。
3. 消费仓 `run_relkit` 把 stderr 的 `error:` 行带进异常，避免蓝盾截断只剩 exit 2。
4. bump 必须落 changelog 小节，避免 `stage` 在双平台构建之后才因缺 notes 失败。

## 验收

- [x] `0.2.0+119` / `+120` 发布成功，catalog `code` 单调
- [x] 日志无 `PUT /`、无裸 IP 作为发布端点
- [ ] 无协议头 / 协议 99 的 publisher 在任何正文上传前 426（协议窗口落地后）
- [ ] `upgrade` 默认从当前 HEAD 构建，不再可能无声装上旧 `dist/`
- [ ] 消费仓同一 run 的 SDK 与 CLI 共用解析后的 SHA

## 后续

见计划「固化发布升级闭环」：双向窗口、部署 provenance、`scripts/consume.py`、同内容幂等重跑。
