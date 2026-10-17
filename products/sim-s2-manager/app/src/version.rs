// 编译期版本：默认占位，producer 构建时经 build.rs 的 cargo:rustc-env=SIM_VERSION_EMBED
// 覆盖（env 优先于内置值，本地 cargo check 无 env 也能编译）。

pub const SIM_VERSION_BUILTIN: &str = "0.1.0+1";

pub const SIM_VERSION: &str = match option_env!("SIM_VERSION_EMBED") {
    Some(embedded) => embedded,
    None => SIM_VERSION_BUILTIN,
};
