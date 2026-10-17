// SIM-S2 managed: 被管理产品（模拟 Loom 形态）
//
// Node/TS projection：被管理端自己不驱动 updater（updater-projection 角色），
// 由 manager（sim-s2-manager）负责安装/切换/更新它。本程序只是被管理的
// hello-world 二进制，业务逻辑极简：打印版本与产品名。
//
// 注意 roles: ["publisher", "updater-projection"]——与 Loom 相同，
// 自己发布制品 + Runtime Profile，更新由 manager 侧订阅执行。
// 版本号由 producer 构建时替换下面的占位字面量。

const version = process.env.SIM_VERSION ?? "0.1.0+1";

if (process.argv[2] === "--version") {
  console.log(version);
} else {
  console.log(`sim-s2-managed hello world (${version})`);
}
