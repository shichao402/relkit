# ADR 0008: CAS 数据面契约与后端类型收敛

- Status: Accepted
- Date: 2026-09-08
- Supersedes: [SPEC.md §13.1](../../SPEC.md) 中「`local` 后端必须始终可用」及「`get` 不能用文档 URL 替代，因为 `local` 的 `baseUrl` 此刻通常不可解析」。路径型「上传前即可确定 URL」的分界仍然有效。ADR 0001–0007 的决策正文不变；[ADR 0007](0007-entry-mirror-must-be-reachable-and-cacheable.md) 继续用 `s3-compatible` 作备援桶。

## 背景

`relkit cas-put` 本应只执行服务端签发的上传说明。实际前门按后端类型分叉：COS 曾走 STS + 客户端 Header SigV4（`sign`），`local` 发相对 URL 并把调用方长期 Bearer 抄回去。`relkit-agent` 经 `local` 直写 `relkit-serve` 的目录，与 serve 的 GC 双写同一棵树，未发布的 `cas/` 会被当成垃圾删掉。SPEC 还把 `local` 定为必须始终可用的离线兜底，把「本机目录 sink」和「CI 可达的上传目标」绑在一起。

## 决策

1. **客户端只执行请求描述，永不签名。** 凭据文档描述要发哪些 HTTP 请求。每个请求是绝对 `http(s)` URL + 头 + 到期时间，鉴权装在 URL 里（预签名 query 或能力票）。客户端不认识 SigV4 / TC3，也不按 `Type()` 分支。禁止 `sign` 字段；分片到来时按需再要一片的 URL，而不是让客户端自签。
2. **凭据按请求描述列表建模。** 每个 blob 带 `requests[]`（`method` / `url` / `headers` / `expiresAt`）。单对象上传恰好一个元素。分片续传只增加元素，并另给「索取下一片」的端点；形状不变。
3. **数据面独占自己的存储。** 控制面（`relkit-agent`）签发能力、Promote、写 index，不做第二个字节写者。禁止 agent 代理 `PUT` CAS 正文到本机目录。
4. **后端类型按「连哪套 API」命名，只保留三个：**
   - `s3-compatible`：COS / S3 / MinIO，长期钥 query 预签名。
   - `relkit-compatible`：`relkit-serve` 的能力票 / COPY / HEAD / DELETE。
   - `static-http`：只读镜像与已有外部 URL。
5. **删除 `local` 与 `http-put`。** 内网 ingest 用 `relkit-compatible`。本机演练起真 `relkit-serve`（文档与 `Skeleton()` 对齐 `127.0.0.1:30341`）。`http-put` 的配置形状（`baseUrl`、可选 `uploadUrl`、`tokenEnv`、`timeoutSeconds`）由 `relkit-compatible` 沿用。
6. **不自建 STS，不接厂商 STS SDK。** CAS 单对象与日后分片都由服务端签发 URL。STS 仅当客户端必须跑厂商传输管理器、分片数无法枚举时才考虑，届时是新的会话类型，不是复活 `sign`。
7. **自托管 CAS 用对象级能力 URL。** `cas/{sha256}` 没有产品分量，`productAllowsKey` 不能隔离 CAS。签名密钥只留在 `relkit-serve`；agent 向 serve 索取 URL，自己不持签发密钥。
   `relkit-compatible.uploadUrl` 与对象存储 endpoint 同义，必须同时可被 agent 和 CI 访问；远程 CI 场景不得使用 loopback。serve 的完整数据面 API 可以公开可达：普通写操作由运营方 Bearer 保护，CAS PUT 由短期对象能力保护。
8. **`cas/` GC 须未被引用且过保护期。** 默认 `gc.casGrace` 为 24h；`POST /-/cas/uploads` 登记的租约优先跳过。`manifest/` 与 `artifact/` 仍按 index 引用回收。
9. **传输错误与请求日志抹掉签名 query**（`sig`、`X-Amz-Signature` 及同等参数）。

## 后果

- CI 与 `relkit cas-put` 对公网 COS 和内网 serve 走同一套动作。
- 删 `local` 后，SPEC 的离线兜底改为本机 serve 或 `static-http` 声明外部已送达的树；U 盘 / rsync 不再靠一个伪造凭据的后端。
- 现网 `svn-auto-merge` 必须先把 profile 换成 `relkit-compatible` 并成功发版，再部署去掉 `local` 的 agent，否则会 `unsupported backend type "local"`。
- 守护进程仍名 `relkit-serve`；改进程名可与类型名解耦，单独做。
