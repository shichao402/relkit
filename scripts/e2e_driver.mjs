#!/usr/bin/env node
// sim e2e driver: 发布后拉起真实模拟客户端跑 check → download → apply →
// 重启断言版本。在 CI（windows runner）与本机都可跑。
//
// 用法:
//   node scripts/e2e_driver.mjs --product sim-s1 --from 0.1.0+1 --expect 0.1.0+2 \
//     [--install-root DIR] [--channel dev] [--rounds 4] [--component sim-server]
//
// 前置: 对应产品已通过 relkitw release --execute 发布到公网 Operator；
// 安装根已由 workflow 的 bootstrap 步骤放置 from 版本二进制 + relkit-updater sidecar。

import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync } from "node:fs";
import path from "node:path";
import process from "node:process";

function fail(message) {
  process.stderr.write(`sim e2e driver: ${message}\n`);
  process.exit(1);
}

function sleepSync(ms) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);
}

// 两段式 apply 语义（Windows library placement）：apply 返回 accepted 只代表
// worker 已接受任务；真正安装由后台 worker 异步完成，新版本落在
// versions/<v>/ 下，active.json 指针指向它。顶层 exe 因文件锁永不更新。
// 因此 after-apply 断言对象是 active.json 及其 executable，而非顶层 exe。
function readActive() {
  const p = path.join(installRoot, "active.json");
  if (!existsSync(p)) return null;
  try {
    return JSON.parse(readFileSync(p, "utf8"));
  } catch {
    return null;
  }
}

function waitActiveVersion(expected, timeoutMs = 30000) {
  const deadline = Date.now() + timeoutMs;
  let last = null;
  while (Date.now() < deadline) {
    const a = readActive();
    last = a;
    if (a && a.version === expected) {
      const exeAbs = path.join(installRoot, a.executable ?? "");
      if (a.executable && existsSync(exeAbs)) return a;
    }
    sleepSync(500);
  }
  fail(`等待 active.json 指向 ${expected} 超时: ${JSON.stringify(last)}`);
}

function runVersionAt(exePath) {
  const env = {
    ...process.env,
    SIM_CHANNEL: channel,
    SIM_INSTALL_ROOT: installRoot,
  };
  if (exePath.endsWith(".js")) {
    return spawnSync("node", [exePath, "--version"], { encoding: "utf8", env });
  }
  return spawnSync(exePath, ["--version"], { encoding: "utf8", env });
}

function argValue(name) {
  const at = process.argv.indexOf(`--${name}`);
  return at >= 0 && at + 1 < process.argv.length ? process.argv[at + 1] : undefined;
}

const product = argValue("product");
const fromVersion = argValue("from");
const expectVersion = argValue("expect");
if (!product || !fromVersion || !expectVersion) {
  fail("usage: --product <id> --from <ver> --expect <ver> [--install-root DIR] [--channel dev] [--rounds 4] [--component NAME]");
}

const channel = argValue("channel") ?? "dev";
const rounds = Number(argValue("rounds") ?? "4");
const component = argValue("component") ?? "sim-server";
const repoRoot = process.cwd();

const PRODUCT_DIRS = {
  "sim-s1": {
    exe: (root) => path.join(root, "sim-s1.exe"),
    defaultExeName: "sim-s1.exe",
  },
  "sim-s2-manager": {
    exe: (root) => path.join(root, "sim-s2-manager.exe"),
    defaultExeName: "sim-s2-manager.exe",
  },
  "sim-s2-managed": {
    exe: (root) => path.join(root, "sim-s2-managed.js"),
    node: true,
    defaultExeName: "sim-s2-managed.js",
  },
  "sim-s3": {
    exe: (root) => path.join(root, `${component}.exe`),
    defaultExeName: "sim-server.exe",
  },
};

const spec = PRODUCT_DIRS[product];
if (!spec) fail(`unknown product ${product}`);

// 安装根: 默认 ./.e2e/<product>/install
const installRoot =
  argValue("install-root") ?? path.join(repoRoot, ".e2e", product, "install");
mkdirSync(installRoot, { recursive: true });

function runExe(args) {
  const exe = activeExeOverride ?? spec.exe(installRoot);
  const env = {
    ...process.env,
    SIM_CHANNEL: channel,
    SIM_INSTALL_ROOT: installRoot,
  };
  if (spec.node || exe.endsWith(".js")) {
    return spawnSync("node", [exe, ...args], { encoding: "utf8", env });
  }
  return spawnSync(exe, args, { encoding: "utf8", env });
}
let activeExeOverride;

// 1. 初始安装检查: from 版本二进制与 sidecar 必须已在 install root。
const initialExe = spec.exe(installRoot);
if (!existsSync(initialExe)) {
  fail(
    `install root 未就绪: ${initialExe} 不存在。请先由 workflow 的 bootstrap 步骤放置 from 版本二进制与 relkit-updater sidecar。`,
  );
}

function assertVersion(expected, label) {
  const out = runExe(["--version"]);
  if (out.status !== 0) fail(`${label}: --version 退出 ${out.status}: ${out.stderr}`);
  const line = (out.stdout ?? "").trim();
  if (!line.includes(expected)) {
    fail(`${label}: 期望版本含 "${expected}"，实际 "${line}"`);
  }
  process.stdout.write(`[ok] ${label}: ${line}\n`);
}

// manager 场景: apply 后代管的 sim-s2-managed 也被切换到 expect 版本（library 安装）。
// 同样按 active.json 指针断言（两段式 apply，顶层文件不代表当前版本）。
function assertManagedVersion(expected, label) {
  const managedRoot = path.join(installRoot, "managed");
  const p = path.join(managedRoot, "active.json");
  if (!existsSync(p)) return; // 无代管物则跳过
  let a;
  try {
    a = JSON.parse(readFileSync(p, "utf8"));
  } catch (e) {
    fail(`${label} (managed): active.json 解析失败: ${e.message}`);
  }
  if (a?.version !== expected) {
    fail(`${label} (managed): active.json 版本 ${a?.version} ≠ ${expected}`);
  }
  const exe = path.join(managedRoot, a.executable ?? path.join(`versions/${expected}`, "sim-s2-managed.js"));
  const out = spawnSync("node", [exe, "--version"], { encoding: "utf8" });
  if (out.status !== 0) fail(`${label} (managed): 退出 ${out.status}: ${out.stderr}`);
  const line = (out.stdout ?? "").trim();
  if (!line.includes(expected)) {
    fail(`${label} (managed): 期望版本含 "${expected}"，实际 "${line}"`);
  }
  process.stdout.write(`[ok] ${label} (managed library): ${line}\n`);
}

assertVersion(fromVersion, "baseline");

// 2. 更新循环: check → apply → 重启 → 断言新版本
let current = fromVersion;
for (let round = 1; round <= rounds; round++) {
  process.stdout.write(`round ${round}: check\n`);
  const check = runExe(["check"]);
  if (check.status !== 0) {
    fail(`round ${round} check 退出 ${check.status}: ${check.stderr}`);
  }
  process.stdout.write((check.stdout ?? "").trim() + "\n");
  if ((check.stdout ?? "").includes("update-available")) {
    process.stdout.write(`round ${round}: apply\n`);
    const apply = runExe(["apply"]);
    if (apply.status !== 0) {
      fail(`round ${round} apply 退出 ${apply.status}: ${apply.stderr}`);
    }
    process.stdout.write((apply.stdout ?? "").trim() + "\n");
    // 两段式 apply: accepted ≠ 安装完成，轮询 active.json 到 expect 版本。
    const active = waitActiveVersion(expectVersion);
    const exeAt = path.join(installRoot, active.executable ?? path.join(`versions/${expectVersion}`, spec.defaultExeName ?? "sim-s1.exe"));
    const vout = runVersionAt(exeAt);
    if (vout.status !== 0) {
      fail(`round ${round} after-apply: ${exeAt} --version 退出 ${vout.status}: ${vout.stderr}`);
    }
    const vline = (vout.stdout ?? "").trim();
    if (!vline.includes(expectVersion)) {
      fail(`round ${round} after-apply: 期望版本含 "${expectVersion}"，实际 "${vline}"`);
    }
    process.stdout.write(`[ok] round ${round} after-apply (active=${active.version}): ${vline}\n`);
    // 模拟重启: 后续轮次从 active 指针的新 exe 运行（真实客户端升级后重启即切到新版本）。
    activeExeOverride = exeAt;
    if (product === "sim-s2-manager") {
      assertManagedVersion(expectVersion, `round ${round} after-apply`);
    }
    current = expectVersion;
    continue;
  }
  if ((check.stdout ?? "").includes("up-to-date") && current === expectVersion) {
    process.stdout.write(`[ok] round ${round}: up-to-date at ${current} (restart verified)\n`);
    break;
  }
}

if (current !== expectVersion) {
  fail(`最终版本 ${current} ≠ 期望 ${expectVersion}`);
}

process.stdout.write(`\ne2e PASS: ${product} ${fromVersion} → ${expectVersion} (${channel})\n`);
