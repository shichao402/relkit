# relkit updater TypeScript bindings

此组件只发布从 `proto/updater/v1/updater.proto` 预生成的 protobuf-ES 类型；
不包含冻结的 legacy Node 客户端或 sidecar facade。产品仓由
`relkit_host.py install` 获取后直接 import，禁止在产品仓运行 codegen。

Release 附件包含预编译的 `dist/updater_pb.js` 与 `.d.ts`，产品通过
`@relkit/updater-bindings/updater/v1` 导入；不要直接编译生成的 `.ts` 源文件。

canonical ProtoJSON 的五变体见 `conformance/updater/`。
