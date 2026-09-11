# relkit Rust updater facade

供 Tauri/Rust 宿主调用 `relkit-updater` sidecar。crate 只负责本机进程、protobuf
分帧、IPC 握手和 scheduler，不重新实现更新算法。

```toml
[dependencies]
relkit-updater = { path = "third_party/relkit/sdk/rust" }
```

DTO 在构建时从 `proto/updater/v1/updater.proto` 生成。仓库内构建读取根目录
`proto/`；Release ZIP 会携带同一份 canonical IDL，消费到稳定路径后无需系统
`protoc`，`protoc-bin-vendored` 会提供固定工具链。

```rust
use relkit_updater::{OpenResult, Updater};
use relkit_updater::proto::{ClientProfile, Runtime};

match Updater::open(ClientProfile::default(), Runtime::default()) {
    OpenResult::Opened { updater, capabilities } => {
        let result = updater.check(true, 0, None);
        // 将生成的 CheckResult 映射到 Tauri command 的窄桥。
    }
    OpenResult::Failed(error) => {
        // 向 WebView 返回结构化 ErrorCode，不抛裸字符串。
    }
}
```

sidecar 查找顺序：`Runtime.sidecar_path`、`RELKIT_UPDATER`、安装根目录默认文件名、
当前壳可执行文件同目录、`InstallSpec.sidecar_relpath`。

开发验证：`cargo test --manifest-path sdk/rust/Cargo.toml`。
