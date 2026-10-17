// SIM-S2 manager 的 build.rs：读 producer 提供的两个 runtime bootstrap 文件，
// 校验 schema/product 后以 cargo:rustc-env 暴露 JSON 串（Loom-Launcher build.rs 同款）。

fn embed_runtime_bootstrap(path_env: &str, rustc_env: &str, expected_product: &str) {
    println!("cargo:rerun-if-env-changed={path_env}");
    let Some(path) = std::env::var_os(path_env) else {
        return;
    };
    println!(
        "cargo:rerun-if-changed={}",
        std::path::Path::new(&path).display()
    );
    let text = std::fs::read_to_string(&path)
        .unwrap_or_else(|error| panic!("{path_env} cannot be read: {error}"));
    let value: serde_json::Value = serde_json::from_str(&text)
        .unwrap_or_else(|error| panic!("{path_env} is not JSON: {error}"));
    assert_eq!(
        value.get("schema").and_then(|item| item.as_str()),
        Some("relkit.runtime-bootstrap/1"),
        "{path_env} is not a Relkit runtime bootstrap"
    );
    assert_eq!(
        value
            .pointer("/profile/product")
            .and_then(|item| item.as_str()),
        Some(expected_product),
        "{path_env} is for the wrong product"
    );
    let compact = serde_json::to_string(&value)
        .unwrap_or_else(|error| panic!("{path_env} cannot be normalized: {error}"));
    println!("cargo:rustc-env={rustc_env}={compact}");
}

fn embed_version() {
    // producer 以 SIM_VERSION env 注入本次发布版本；源文件不再被改写。
    println!("cargo:rerun-if-env-changed=SIM_VERSION");
    let Ok(version) = std::env::var("SIM_VERSION") else {
        return; // 本地 cargo check 走 version.rs 内置占位
    };
    let trimmed = version.trim();
    assert!(
        trimmed.contains('+'),
        "SIM_VERSION must be semver-with-build (e.g. 0.1.0+7), got {version:?}"
    );
    println!("cargo:rustc-env=SIM_VERSION_EMBED={trimmed}");
}

fn main() {
    embed_runtime_bootstrap(
        "SIM_BOOTSTRAP_FILE",
        "SIM_BOOTSTRAP_JSON",
        "sim-s2-manager",
    );
    embed_runtime_bootstrap(
        "SIM_MANAGED_BOOTSTRAP_FILE",
        "SIM_MANAGED_BOOTSTRAP_JSON",
        "sim-s2-managed",
    );
    embed_version();
}
