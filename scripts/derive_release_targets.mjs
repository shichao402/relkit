#!/usr/bin/env node
// 发布计划推导：push 触发时从 git diff 推导产品列表与 per-product 发布参数，
// dispatch 显式入参时输出单产品计划。供 sim-release.yml 的 prepare 步骤调用，
// yaml 不再内联任何推导逻辑（PAC 铁律：yaml 只做编排，逻辑进项目脚本）。
//
// 用法:
//   node scripts/derive_release_targets.mjs <before_ref> <after_ref>
//   node scripts/derive_release_targets.mjs --dispatch <product> <version> <channel> [baseline]
//
// 输出（stdout）: 发布计划 JSON
//   {"products": ["sim-s1", ...],
//    "releases": {"<product>": {"version": "0.1.0+7", "baseline": "0.1.0+6", "channel": "dev"}}}
//
// 退出码:
//   0 = 正常输出计划（至少一个产品）
//   3 = 无待发布产品（push 无 CHANGELOG 变更；调用方应 skip 而非 fail）
//   1 = 参数/推导错误
//
// 推导规则（push 触发）:
//   - 本次 push 中 CHANGELOG 有变更的产品 = 待发布产品（全部展开，多产品矩阵）
//   - 无 CHANGELOG 变更 → exit 3（scripts/workflow 变更不应触发发布链；
//     历史 fallback sim-s1 兜底会重发已发布版本撞不可变门禁，已废弃）
//
// version/baseline 推导（每产品独立）:
//   - version = 该产品 CHANGELOG 顶部小节（必须带 +build）
//   - baseline = 倒数第二个小节（CDN 上的上一版，from 侧）；显式输入优先；
//     与 version 相同时置空（首版自装自验）

import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";

const PRODUCTS = ["sim-s1", "sim-s2-manager", "sim-s2-managed", "sim-s3"];

function die(message, code = 1) {
  process.stderr.write(`derive_release_targets: ${message}\n`);
  process.exit(code);
}

function gitOutput(gitArgs) {
  const out = spawnSync("git", gitArgs, { encoding: "utf8" });
  if (out.status !== 0) {
    die(`git ${gitArgs.join(" ")} failed: ${(out.stderr ?? "").trim()}`);
  }
  return (out.stdout ?? "").trim();
}

function changelogSections(product) {
  const text = readFileSync(`products/${product}/CHANGELOG.md`, "utf8");
  return text
    .split("\n")
    .filter((line) => line.startsWith("## "))
    .map((line) => line.replace(/^## +/, "").replace(/ +$/, ""));
}

function buildRelease(product, { explicitVersion, explicitBaseline, channel }) {
  let version = explicitVersion;
  if (!version) {
    const sections = changelogSections(product);
    if (sections.length === 0) die(`products/${product}/CHANGELOG.md has no release sections`);
    version = sections[0];
  }
  if (!/^\d+\.\d+\.\d+\+\d+$/.test(version)) {
    die(`version 必须形如 0.1.0+2 (带 +build)，got "${version}"`);
  }
  let baseline = explicitBaseline ?? "";
  if (!baseline) {
    const sections = changelogSections(product);
    // 倒数第二个小节 = from 侧（顶部小节是本次 to）
    baseline = sections[1] ?? "";
  }
  if (baseline === version) baseline = "";
  return { version, baseline, channel: channel || "dev" };
}

function emit(products, releases) {
  process.stdout.write(`${JSON.stringify({ products, releases })}\n`);
  process.exit(0);
}

const args = process.argv.slice(2);

if (args[0] === "--dispatch") {
  const [product, version, channel, baseline] = args.slice(1);
  if (!product) die("--dispatch 需要 <product> <version> <channel> [baseline]");
  if (!PRODUCTS.includes(product)) die(`unknown product "${product}"`);
  if (!version) die("--dispatch 需要 version（workflow_dispatch required）");
  emit([product], { [product]: buildRelease(product, { explicitVersion: version, explicitBaseline: baseline, channel }) });
}

const [before, after] = args;
if (!before || !after) die("需要 <before_ref> <after_ref>（push 模式）");

const changed = gitOutput(["diff", "--name-only", before, after, "--", "products/*/CHANGELOG.md"])
  .split("\n")
  .map((line) => line.trim())
  .filter(Boolean)
  .map((line) => line.replace(/^products\/([^/]+)\/CHANGELOG\.md$/, "$1"));

for (const p of changed) {
  if (!PRODUCTS.includes(p)) die(`unknown product "${p}"`);
}

const targets = [...new Set(changed)];
if (targets.length === 0) {
  // 无 CHANGELOG 变更：非发布提交（scripts/workflow/产品源码变更），
  // 不兜底、不发布（历史 sim-s1 兜底重发已发布版本必撞不可变门禁，废弃）。
  process.stdout.write("no CHANGELOG change in push; no release\n");
  process.exit(3);
}

const releases = {};
for (const p of targets) {
  releases[p] = buildRelease(p, {});
}
emit(targets, releases);
