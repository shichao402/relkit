# relkit Rust updater facade
## WebView JSON

`check_result_to_json` / `check_result_from_json` 使用 `pbjson-build` 从
`proto/updater/v1/updater.proto` 生成的 canonical ProtoJSON 实现，不维护
`*Wire` 镜像。`Timestamp` 编码为 RFC3339（例如
`"2026-08-17T01:15:06.123Z"`），int64 遵循 ProtoJSON 字符串编码；为严格
WebView 合同，标量默认值也会输出。

五种 `CheckResult` 变体必须与 `conformance/updater/` fixture 双向
round-trip。产品只消费 release lock 安装的 crate 和 TS bindings，不在产品仓
运行 protobuf codegen。

供 Tauri/Rust 宿主调用 `relkit-updater` sidecar。crate 只负责本机进程、protobuf
分帧、IPC 握手、scheduler，以及把 `CheckResult` 投影成 WebView JSON。不重新实现
更新算法。

```toml
[dependencies]
relkit-updater = { path = "third_party/relkit/sdk/rust" }
```

DTO 在构建时从 `proto/updater/v1/updater.proto` 生成。仓库内构建读取根目录
`proto/`；Release ZIP 会携带同一份 canonical IDL，消费到稳定路径后无需系统
`protoc`，`protoc-bin-vendored` 会提供固定工具链。

```rust
use relkit_updater::{check_result_to_json, OpenResult, Updater};
use relkit_updater::proto::{ClientProfile, Runtime};

match Updater::open(ClientProfile::default(), Runtime::default()) {
    OpenResult::Opened { updater, capabilities: _ } => {
        let result = updater.check(true, 0, None);
        let json = check_result_to_json(&result).expect("check result has a kind");
        // 把投影 JSON 交给 WebView。不要手写 CheckResult DTO，不要 serde(default)。
    }
    OpenResult::Failed(error) => {
        // 向 WebView 返回结构化 ErrorCode，不抛裸字符串。
    }
}
```

`check_result_to_json` 始终写出 IDL 标量，包括空的 `releaseNotesMarkdown` 和
`mandatory: false`。缺键是投影器故障，不是旧 sidecar；宿主侧保持严格反序列化。

sidecar 查找顺序：`Runtime.sidecar_path`、`RELKIT_UPDATER`、安装根目录默认文件名、
当前壳可执行文件同目录、`InstallSpec.sidecar_relpath`。

开发验证：`cargo test --manifest-path sdk/rust/Cargo.toml`。
