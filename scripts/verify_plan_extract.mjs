#!/usr/bin/env node
// 本地验证 workflow prepare/materialize step 的 JSON 提取单行逻辑（与 yaml 内嵌同型）。
// 用法: echo "$PLAN" | node scripts/verify_plan_extract.mjs <product> [matrix]

import process from "node:process";

const arg = process.argv[2];
let s = "";
process.stdin.on("data", (d) => (s += d));
process.stdin.on("end", () => {
  if (s.charCodeAt(0) === 0xfeff) s = s.slice(1); // BOM 防御（PowerShell 管道注入）
  if (arg === "matrix") {
    console.log(JSON.stringify(JSON.parse(s).products));
  } else {
    const r = JSON.parse(s)[arg];
    console.log(`VERSION=${JSON.stringify(r.version)}`);
    console.log(`CHANNEL=${JSON.stringify(r.channel)}`);
    console.log(`BASELINE=${JSON.stringify(r.baseline || "")}`);
  }
});
