// SIM-S2 manager: 管理器 + 被管理产品（模拟 Loom-Launcher 形态）
//
// Rust facade 驱动 updater，与 Loom-Launcher 同构。双订阅:
//   - sim-s2-manager 自身（in-place 自更新）
//   - sim-s2-managed（library 安装/切换，由 manager 代管）
// 两段式 apply: managed 进程在跑时只下载不应用（模拟 launcher 的
// on_editor_all_closed 语义）。
// 编译期注入: build.rs 把两个 bootstrap 文件转成 SIM_BOOTSTRAP_JSON /
// SIM_MANAGED_BOOTSTRAP_JSON（cargo:rustc-env），版本来自 version.rs 占位替换。
//
// hello-world 业务: 命令行玩具。

mod version;

use relkit_updater::proto::check_result::Kind as CheckKind;
use relkit_updater::proto::updater_event::Kind as EventKind;
use relkit_updater::proto::{
    ClientProfile, InstallSpec, LibraryPolicy, Placement, Runtime,
};
use relkit_updater::runtime_bootstrap_from_json;
use relkit_updater::{OpenResult, Updater};
use std::path::PathBuf;

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let command = args.get(1).map(|s| s.as_str()).unwrap_or("status");

    match command {
        "--version" => {
            println!("sim-s2-manager {}", version::SIM_VERSION);
        }
        "check" | "apply" => {
            run_update(command);
        }
        _ => {
            println!("sim-s2-manager hello world");
            println!("commands: --version | check | apply | managed-check | managed-apply");
        }
    }
}

fn run_update(command: &str) {
    // 自更新（in-place）
    println!("== self (sim-s2-manager, in-place) ==");
    match open_updater("sim-s2-manager") {
        Ok(updater) => drive_self(updater, command),
        Err(e) => eprintln!("open self updater: {}", e),
    }

    // 被管理产品（library）——两段式: managed 在跑则只下载
    println!("== managed (sim-s2-managed, library) ==");
    match open_updater("sim-s2-managed") {
        Ok(updater) => drive_managed(updater, command),
        Err(e) => eprintln!("open managed updater: {}", e),
    }
}

fn check_once(updater: &Updater) -> relkit_updater::proto::CheckResult {
    updater.check(true, 0, None)
}

fn drive_self(updater: Updater, command: &str) {
    let check = check_once(&updater);
    match check.kind {
        Some(CheckKind::UpdateAvailable(ref available)) => {
            println!(
                "check-result: update-available {} plan={}",
                available.version, available.plan_id
            );
            if command == "check" {
                return;
            }
            let _ = updater.download(available.plan_id.clone(), |_| {});
            match updater.apply(available.plan_id.clone(), |_| {}) {
                _ => println!("apply-result: ok"),
            }
        }
        Some(CheckKind::UpToDate(ref value)) => {
            println!("check-result: up-to-date sequence={}", value.sequence);
        }
        Some(CheckKind::Failed(ref failed)) => {
            if let Some(error) = &failed.error {
                eprintln!("check failed: {}", error.message);
            }
        }
        other => println!("check-result: {:?}", other.map(|_| "other")),
    }
}

fn drive_managed(updater: Updater, command: &str) {
    let check = check_once(&updater);
    match check.kind {
        Some(CheckKind::UpdateAvailable(ref available)) => {
            println!(
                "check-result: update-available {} plan={}",
                available.version, available.plan_id
            );
            if command == "check" {
                return;
            }
            let install_only = managed_running();
            let _ = updater.download(available.plan_id.clone(), |_| {});
            if install_only {
                // 两段式: 进程在跑，只下载，不应用
                println!("apply-result: staged-only (managed process running)");
                return;
            }
            let result = updater.apply(available.plan_id.clone(), |_| {});
            match result.kind {
                Some(relkit_updater::proto::apply_result::Kind::Failed(ref failed)) => {
                    if let Some(error) = &failed.error {
                        eprintln!("apply failed: {}", error.message);
                    }
                }
                _ => println!("apply-result: ok"),
            }
        }
        Some(CheckKind::UpToDate(ref value)) => {
            println!("check-result: up-to-date sequence={}", value.sequence);
        }
        Some(CheckKind::Failed(ref failed)) => {
            if let Some(error) = &failed.error {
                eprintln!("check failed: {}", error.message);
            }
        }
        other => println!("check-result: {:?}", other.map(|_| "other")),
    }
}

fn managed_running() -> bool {
    // 模拟 launcher 的 on_editor_all_closed 语义: managed hello-world 在跑则占用。
    // tasklist 只在 windows 上可用；其余平台视为未运行。
    if !cfg!(windows) {
        return false;
    }
    std::process::Command::new("tasklist")
        .output()
        .map(|o| String::from_utf8_lossy(&o.stdout).contains("sim-s2-managed"))
        .unwrap_or(false)
}

fn profile_from(json: Option<&'static str>, expected_product: &str) -> Result<ClientProfile, String> {
    let json = json.ok_or_else(|| {
        format!("构建没有嵌入 {expected_product} 的已验证 Runtime Profile；请通过 Relkit artifact producer 构建")
    })?;
    let value = runtime_bootstrap_from_json(json)
        .map_err(|error| format!("{expected_product} Runtime Profile 无效：{error}"))?;
    if value.client_profile.product != expected_product {
        return Err(format!(
            "Runtime Profile 产品是 {}，期望 {expected_product}",
            value.client_profile.product
        ));
    }
    Ok(value.client_profile)
}

fn open_updater(product: &str) -> Result<Updater, String> {
    let bootstrap = if product == "sim-s2-manager" {
        option_env!("SIM_BOOTSTRAP_JSON")
    } else {
        option_env!("SIM_MANAGED_BOOTSTRAP_JSON")
    };
    let profile = profile_from(bootstrap, product)?;

    let exe_dir = std::env::current_exe()
        .ok()
        .and_then(|p| p.parent().map(|d| d.to_path_buf()))
        .unwrap_or_else(|| PathBuf::from("."));
    let install_root = std::env::var("SIM_INSTALL_ROOT")
        .map(PathBuf::from)
        .unwrap_or_else(|_| exe_dir.clone());
    let channel = std::env::var("SIM_CHANNEL").unwrap_or_else(|_| "dev".into());

    let current_code = build_number(version::SIM_VERSION);
    let (placement, executable, library) = if product == "sim-s2-manager" {
        (Placement::InPlace, "sim-s2-manager.exe".to_string(), None)
    } else {
        (
            Placement::Library,
            "sim-s2-managed.js".to_string(),
            Some(LibraryPolicy {
                retain: 3,
                reserved_codes: Vec::new(),
            }),
        )
    };
    let selectors = std::collections::HashMap::from([
        ("os".to_string(), "windows".to_string()),
        ("arch".to_string(), "x64".to_string()),
    ]);
    let runtime = Runtime {
        channel,
        current_code,
        client_selectors: selectors,
        data_dir: std::env::temp_dir()
            .join(format!("sim-s2-{}-updater", product))
            .to_string_lossy()
            .into_owned(),
        install: Some(InstallSpec {
            placement: placement as i32,
            install_root: install_root.to_string_lossy().into_owned(),
            executable_relpath: executable,
            sidecar_relpath: "relkit-updater.exe".into(),
            library,
            ..Default::default()
        }),
        sidecar_path: install_root
            .join("relkit-updater.exe")
            .to_string_lossy()
            .into_owned(),
        ..Default::default()
    };
    match Updater::open(profile, runtime) {
        OpenResult::Opened { updater, .. } => Ok(*updater),
        OpenResult::Failed(error) => Err(error.message),
    }
}

fn build_number(version: &str) -> i64 {
    version
        .rsplit_once('+')
        .and_then(|(_, build)| build.parse().ok())
        .unwrap_or(1)
}

// 保留 EventKind 导入以备进度回调扩展（download/apply 的事件流在后续 e2e 用）。
#[allow(dead_code)]
fn _event_kind_debug(_: Option<EventKind>) {}
