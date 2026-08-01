# ADR 0003: Protobuf 为结构 SSOT，线格式升至 v2

- Status: Accepted
- Date: 2026-08-01

## 决策

RUP 的全部结构化对象与 SDK 可见的服务端响应，以 [`update-spec/proto`](../../proto/) 中的 Protobuf 定义为唯一结构来源。

- 线格式：`rup.*/2`，对象为 **protobuf 二进制**（逻辑 key 使用 `.pb` 后缀）。
- 签名：Ed25519 签的是 `Index` 的 protobuf 字节（经 `Envelope.payload` 承载），不再签 UTF-8 JSON。
- 读写：各语言只通过生成代码访问字段；禁止并行维护手写结构体。
- `selectors` / `meta` 使用 `repeated` 键值对，encode 前按 key 排序，避免 map 顺序导致同逻辑不同字节。

JSON v1（`rup.*/1`、`.json` key）废弃。现网迁移方式：用新版 `relkit` 重新 `stage` / `publish`；客户端只认 v2。

## 后果

- Go / Dart（及未来语言）SDK 共享同一套字段语义。
- `relkit` 与 `relkit-serve` 同仓消费生成的 Go API。
- 旧 index 不可被新客户端解析；需 republish。
