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
import { existsSync, mkdirSync } from "node:fs";
import path from "node:path";
import process from "node:process";

function fail(message) {
  process.stderr.write(`sim e2e driver: ${message}\n`);
  process.exit(1);
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
  },
  "sim-s2-manager": {
    exe: (root) => path.join(root, "sim-s2-manager.exe"),
  },
  "sim-s2-managed": {
    exe: (root) => path.join(root, "sim-s2-managed.js"),
    node: true,
  },
  "sim-s3": {
    exe: (root) => path.join(root, `${component}.exe`),
  },
};

const spec = PRODUCT_DIRS[product];
if (!spec) fail(`unknown product ${product}`);

// 安装根: 默认 ./.e2e/<product>/install
const installRoot =
  argValue("install-root") ?? path.join(repoRoot, ".e2e", product, "install");
mkdirSync(installRoot, { recursive: true });

function runExe(args) {
  const exe = spec.exe(installRoot);
  const env = {
    ...process.env,
    SIM_CHANNEL: channel,
    SIM_INSTALL_ROOT: installRoot,
  };
  if (spec.node) {
    return spawnSync("node", [exe, ...args], { encoding: "utf8", env });
  }
  return spawnSync(exe, args, { encoding: "utf8", env });
}

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
    assertVersion(expectVersion, `round ${round} after-apply`);
    current = expectVersion;
    continue;
  }
  if ((check.stdout ?? "").includes("up-to-date") && current === expectVersion) {
    process.stdout.write(`[ok] round ${round}: up-to-date at ${current}\n`);
    break;
  }
}

if (current !== expectVersion) {
  fail(`最终版本 ${current} ≠ 期望 ${expectVersion}`);
}

process.stdout.write(`\ne2e PASS: ${product} ${fromVersion} → ${expectVersion} (${channel})\n`);
