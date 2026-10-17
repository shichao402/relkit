#!/usr/bin/env node
// Canonical ArtifactSet producer for all sim products.
//
// 合同输入 (stdin): relkit.artifact-producer-input/1
// 合同输出 (stdout): relkit.artifact-set/1
//
// relkit 扩展合同：本脚本由 canonical wrapper 以**产品目录**为 cwd 执行，
// 所有路径都是产品目录相对（app/、CHANGELOG.md）；stdin 是 producer input；
// stdout 只输出 ArtifactSet JSON（构建日志全部走 stderr）。
//
// bootstrap 注入方式按各 facade 的编译期合同：
//   sim-s1        dart:   --define=SIM_BOOTSTRAP_B64=<b64> --define=SIM_VERSION=<v>
//   sim-s2-manager rust:  build.rs 读 SIM_BOOTSTRAP_FILE / SIM_MANAGED_BOOTSTRAP_FILE
//                          （两个订阅产品各一份），cargo:rustc-env 暴露 JSON；
//                          SIM_VERSION 由 --cfg 注入不可行，改为源文件占位替换。
//   sim-s2-managed node:  blob 产物，直接替换源内版本占位。
//   sim-s3         go:    -ldflags "-X main.version=… -X main.runtimeBootstrapBase64=…"
//
// 客户端 arch 词汇与 manifest 逐字匹配（selectors.MatchesClient 无归一化）：
// dart/rust 产品沿用 x64（与 SvnMergeTool/Loom 一致），go 产品沿用 amd64（与 Dec 一致）。

import { createHash } from "node:crypto";
import {
  copyFileSync,
  mkdirSync,
  readFileSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";

const PRODUCTS = new Set(["sim-s1", "sim-s2-manager", "sim-s2-managed", "sim-s3"]);

function fail(message) {
  process.stderr.write(`sim producer: ${message}\n`);
  process.exit(1);
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? process.cwd(),
    encoding: "utf8",
    env: options.env ?? process.env,
    shell: false,
  });
  if (result.stdout) process.stderr.write(result.stdout);
  if (result.stderr) process.stderr.write(result.stderr);
  if (result.status !== 0) {
    fail(`${command} failed with status ${result.status ?? result.signal}`);
  }
  return result.stdout;
}

function sha256(file) {
  return `sha256:${createHash("sha256").update(readFileSync(file)).digest("hex")}`;
}

function releaseCode(version) {
  const match = version.match(/\+([1-9][0-9]*)$/);
  if (!match) fail(`version ${version} has no positive numeric build metadata`);
  const value = Number(match[1]);
  if (!Number.isSafeInteger(value)) fail(`version ${version} has an unsafe release code`);
  return value;
}

function changelogFor(version) {
  const markdown = readFileSync("CHANGELOG.md", "utf8");
  const escaped = version.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const match = markdown.match(
    new RegExp(`^##\\s+${escaped}\\s*$\\r?\\n([\\s\\S]*?)(?=^##\\s+|(?![\\s\\S]))`, "m")
  );
  if (!match?.[1]?.trim()) {
    fail(`CHANGELOG.md has no release notes for ${version}`);
  }
  return match[1].trim();
}

function requireBootstrap(input, product) {
  const value = input.runtimeBootstraps?.[product];
  if (
    value?.schema !== "relkit.runtime-bootstrap/1" ||
    value?.resolutionDigest !== input.resolutionDigest ||
    value?.profile?.product !== product ||
    !Array.isArray(value?.trustedKeys) ||
    value.trustedKeys.length === 0
  ) {
    fail(`missing or invalid verified runtime bootstrap for ${product}`);
  }
  return value;
}

function artifact(id, file, kind, platform, selectors, relations = undefined) {
  return {
    id,
    path: path.basename(file),
    kind,
    platform,
    selectors,
    sha256: sha256(file),
    size: statSync(file).size,
    ...(relations ? { relations } : {}),
  };
}

function emitArtifactSet(input, artifacts, changelog) {
  return {
    schema: "relkit.artifact-set/1",
    resolutionDigest: input.resolutionDigest,
    product: input.product,
    version: input.version,
    releaseCode: releaseCode(input.version),
    channel: input.channel,
    changelog,
    mergeIdentity: {
      product: input.product,
      version: input.version,
      channel: input.channel,
      resolutionDigest: input.resolutionDigest,
    },
    artifacts,
  };
}

// 写出 bootstrap JSON 文件（供 rust build.rs 读取；Loom-Launcher 同款注入面），
// 返回文件绝对路径。放在 out-dir 内的 .runtime-input/ 下，构建结束统一清理。
function writeBootstrapFile(input, product, out) {
  const bootstrap = requireBootstrap(input, product);
  const dir = path.join(out, ".runtime-input");
  mkdirSync(dir, { recursive: true });
  const file = path.join(dir, `${product}.json`);
  writeFileSync(file, `${JSON.stringify(bootstrap)}\n`);
  return file;
}

// ---- per-product build entrypoints（cwd = 产品目录）--------------------

function buildSimS1(input, out) {
  // dart compile exe（AOT）。bootstrap 经 --define 编译期注入（模拟 SvnMergeTool：
  // String.fromEnvironment('RELKIT_RUNTIME_BOOTSTRAP_B64')，此处用 SIM_BOOTSTRAP_B64）。
  const bootstrap = requireBootstrap(input, "sim-s1");
  const bootstrapB64 = Buffer.from(JSON.stringify(bootstrap), "utf8").toString("base64");
  const target = input.target; // windows-amd64 | darwin-universal
  const exeSuffix = target.startsWith("windows") ? ".exe" : "";
  const binary = path.join(out, `sim-s1-${target}${exeSuffix}`);
  run("dart", ["pub", "get"], { cwd: "app" });
  run("dart", [
    "compile",
    "exe",
    `--define=SIM_BOOTSTRAP_B64=${bootstrapB64}`,
    `--define=SIM_VERSION=${input.version}`,
    "-o",
    binary,
    "bin/sim_s1.dart",
  ], { cwd: "app" });
  const artifacts = [];
  if (target === "windows-amd64") {
    const selectors = { os: "windows", arch: "x64" };
    artifacts.push(
      artifact("windows-installer", binary, "installer", target, selectors),
      payloadArtifact(input, out, binary, "windows-payload", target, selectors, "windows-installer", `sim-s1${exeSuffix}`),
    );
  } else {
    artifacts.push(
      artifact("macos-installer", binary, "installer", target, { os: "macos" }),
    );
  }
  return artifacts;
}

function buildSimS2Manager(input, out) {
  // cargo build --release。build.rs 读 SIM_BOOTSTRAP_FILE/SIM_MANAGED_BOOTSTRAP_FILE
  // 两个 bootstrap（双订阅），cargo:rustc-env 暴露 SIM_BOOTSTRAP_JSON/SIM_MANAGED_BOOTSTRAP_JSON。
  const selfBootstrap = writeBootstrapFile(input, "sim-s2-manager", out);
  const managedBootstrap = writeBootstrapFile(input, "sim-s2-managed", out);
  run("cargo", ["build", "--release", "--manifest-path", "app/Cargo.toml"], {
    env: {
      ...process.env,
      SIM_VERSION: input.version,
      SIM_BOOTSTRAP_FILE: selfBootstrap,
      SIM_MANAGED_BOOTSTRAP_FILE: managedBootstrap,
    },
  });
  const binary = path.join("app", "target", "release", "sim-s2-manager.exe");
  const selectors = { os: "windows", arch: "x64" };
  try {
    return [
      artifact("windows-installer", binary, "installer", input.target, selectors),
      payloadArtifact(input, out, binary, "windows-payload", input.target, selectors, "windows-installer", "sim-s2-manager.exe"),
    ];
  } finally {
    rmSync(path.join(out, ".runtime-input"), { recursive: true, force: true });
  }
}

function buildSimS2Managed(input, out) {
  // Node blob 制品（模拟 Loom 的 TS 产物形态）：替换源内版本占位后作为 blob 发布。
  const target = input.target; // windows-amd64
  const destination = path.join(out, `sim-s2-managed-${target}.js`);
  const patched = readFileSync("app/main.js", "utf8").replace(
    'process.env.SIM_VERSION ?? "0.1.0+1"',
    JSON.stringify(input.version),
  );
  writeFileSync(destination, patched);
  const selectors = { os: "windows", arch: "x64" };
  return [
    artifact("windows-installer", destination, "blob", target, selectors),
    payloadArtifact(input, out, destination, "windows-payload", target, selectors, "windows-installer", "sim-s2-managed.js"),
  ];
}

function buildSimS3(input, out) {
  // go build × 3 components（go facade; component × audience 选择器）。
  // bootstrap 经 -ldflags -X main.runtimeBootstrapBase64 注入（与 Dec 同款）。
  const bootstrap = requireBootstrap(input, "sim-s3");
  const bootstrapB64 = Buffer.from(JSON.stringify(bootstrap), "utf8").toString("base64");
  const target = input.target; // linux-amd64 | windows-amd64 | darwin-arm64
  const GOOS = { "linux-amd64": "linux", "windows-amd64": "windows", "darwin-arm64": "darwin" }[target];
  const GOARCH = { "linux-amd64": "amd64", "windows-amd64": "amd64", "darwin-arm64": "arm64" }[target];
  const suffix = GOOS === "windows" ? ".exe" : "";
  const components = ["sim-server", "sim-exec", "sim-host-setup"];
  const artifacts = [];
  for (const component of components) {
    const binary = path.join(out, `${component}-${GOOS}-${GOARCH}${suffix}`);
    run(
      "go",
      [
        "build",
        "-trimpath",
        "-ldflags",
        `-X main.version=${input.version} -X main.runtimeBootstrapBase64=${bootstrapB64}`,
        "-o",
        binary,
        `./app/cmd/${component}`,
      ],
      {
        env: {
          ...process.env,
          GOOS: GOOS,
          GOARCH: GOARCH,
          CGO_ENABLED: "0",
        },
      },
    );
    const selectors = {
      os: GOOS,
      arch: GOARCH,
      component: component,
      audience: "runtime",
    };
    artifacts.push(
      artifact(`${component}-${target}-installer`, binary, "installer", target, selectors),
      payloadArtifact(input, out, binary, `${component}-${target}-payload`, target, selectors, `${component}-${target}-installer`, `${component}${suffix}`),
    );
  }
  return artifacts;
}

function payloadArtifact(input, out, binary, id, target, selectors, installerId, finalName) {
  // 用 canonical wrapper 提供的 RELKIT_CORE 组 payload（与真实 producer 同路径）。
  const core = String(process.env.RELKIT_CORE ?? "").trim();
  if (!core) fail("RELKIT_CORE was not supplied by the canonical wrapper");
  const tree = path.join(out, ".payload-tree", id);
  mkdirSync(tree, { recursive: true });
  // payload 树 = 安装目录树：主二进制用最终安装名（与 executableRelpath 一致）
  // + relkit-updater sidecar（从物化缓存拷入）。
  copyFileSync(binary, path.join(tree, finalName));
  const sidecarName = process.platform === "win32" ? "relkit-updater.exe" : "relkit-updater";
  const sidecarSource = path.join(".relkit", "components", "bin", sidecarName);
  copyFileSync(sidecarSource, path.join(tree, sidecarName));
  const payload = path.join(out, `${id}.zip`);
  run(core, ["payload", "build", "--tree", tree, "--out", payload]);
  return artifact(id, payload, "payload", target, selectors, { payloadOf: installerId });
}

// ---- stdin contract ----------------------------------------------------

const chunks = [];
for await (const chunk of process.stdin) chunks.push(chunk);
let input;
try {
  input = JSON.parse(Buffer.concat(chunks).toString("utf8"));
} catch (error) {
  fail(`invalid producer input: ${error}`);
}
if (input.schema !== "relkit.artifact-producer-input/1") {
  fail(`unexpected schema ${input.schema}`);
}
if (!PRODUCTS.has(input.product)) {
  fail(`unexpected product ${input.product}`);
}

const outputDirectory = path.resolve(String(input.outputDirectory));
mkdirSync(outputDirectory, { recursive: true });

const builders = {
  "sim-s1": buildSimS1,
  "sim-s2-manager": buildSimS2Manager,
  "sim-s2-managed": buildSimS2Managed,
  "sim-s3": buildSimS3,
};

const artifacts = builders[input.product](input, outputDirectory);
const result = emitArtifactSet(input, artifacts, changelogFor(input.version));
process.stdout.write(`${JSON.stringify(result)}\n`);
