# ADR 0009: Publisher 协议双向协商

- Status: Accepted
- Date: 2026-09-09
- Supersedes: agent/serve 上「`declared >= min` 即放行」的单向门槛
- Incident: [INC-2026-09-08](../incidents/2026-09-08-stale-publisher-put-root.md)

## 背景

CAS 凭据文档从 `putUrl` 改成 `requests[]` 之后，旧 publisher 仍然能通过 Bearer 鉴权。它把缺失字段当成空 URL，对 agent origin 发出 `PUT /`。单向 `>= min` 放行未知的未来协议 `99`，同样会把读不懂的文档交给客户端。

## 决策

1. **publisher 与 server 各声明 `[min, max]`。** 当前 build 的窗口是 `[2, 2]`。请求头：
   - `X-Relkit-Publish-Protocol`：publisher 本 build 的 `Current`
   - `X-Relkit-Publish-Protocol-Min` / `-Max`：它能说的窗口
   - `X-Relkit-Version`：诊断用
   只带协议号、不带 min/max 的旧实现，视为退化窗口 `[n, n]`。
2. **无交集则 426。** `publisher_upgrade_required`（过旧）与 `publisher_too_new`（过新）分开。JSON 带回 `minProtocol` / `maxProtocol` / `selected` / `serverVersion` / `serverCommit`。
3. **agent 与 serve 共用 `internal/publishproto`。** 禁止各写一份门槛。
4. **鉴权后的 preflight。** serve：`POST /-/publish/preflight`（运营方 Bearer）。agent：`POST /v1/publish/preflight`（产品 token + `product`）。`cas-put` / `staged-put` 在哈希或上传正文前先协商。能力 URL 的最终 CAS PUT **只**靠能力签名，不得附加协议头。
5. **health/version 公开 build 身份与协议范围**（`/-/health`、`/-/version`），兼容策略仍以鉴权 preflight 为准，匿名探针读不到比 health 更多的产品 token 信息。
6. **禁用握手**仅限算子把 `minPublishProtocol` 设为 `0` 的滚动窗口；默认不得关闭。

## 否决

- 靠代理、`NO_PROXY`、裸 IP 掩盖错误 path
- agent 代理 CAS 正文
- 未来协议号只要 `>= min` 就放行
- 把协议头打到能力 URL PUT 上

## 后果

- 升级 agent/serve 后，消费仓必须用同一 release 的 publisher；漂移变成 426 而不是 `PUT /`
- 滚动发布时可临时下调 `minPublishProtocol`，但 `max` 仍挡住过新客户端
- 消费仓协议号从 lock / consume 输出读取，不再手抄常量
