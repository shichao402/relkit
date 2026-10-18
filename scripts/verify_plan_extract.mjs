#!/usr/bin/env node
// 发布计划 JSON 提取工具 —— workflow prepare / Materialize step 的唯一实现源
// （PAC 铁律：yaml 不内嵌提取逻辑，全部收敛到项目脚本）。
// 用法: printf '%s' "$PLAN" | node scripts/verify_plan_extract.mjs <product|matrix|releases>
//   <product> -> 输出 VERSION="..."/CHANNEL="..."/BASELINE="..." 三行（供 eval 物化进 GITHUB_ENV）
//   matrix    -> 输出 products 数组的单行 JSON（prepare 的 matrix 输出）
//   releases  -> 输出 releases 对象的单行 JSON（prepare 的 releases 输出）
// 任何模式失败（非法 JSON / 未知产品）必须以非零退出——workflow 侧用
// VAR=$(cmd) 赋值形式消费，set -e 下失败即 step 红，杜绝"step 绿但输出为空"
// （run 39096462210 教训：echo "x=$(cmd)" 吞掉命令替换失败码）。
import process from "node:process";

const arg = process.argv[2];
if (!arg) {
  console.error("usage: node scripts/verify_plan_extract.mjs <product|matrix|releases>");
  process.exit(2);
}
let s = "";
process.stdin.on("data", (d) => (s += d));
process.stdin.on("end", () => {
  if (s.charCodeAt(0) === 0xfeff) s = s.slice(1); // BOM 防御（PowerShell 管道注入）
  const plan = JSON.parse(s);
  if (arg === "matrix") {
    console.log(JSON.stringify(plan.products));
  } else if (arg === "releases") {
    console.log(JSON.stringify(plan.releases));
  } else {
    const r = plan[arg];
    if (!r || !r.version || !r.channel) {
      console.error(`no release params for product: ${arg}`);
      process.exit(3);
    }
    console.log(`VERSION=${JSON.stringify(r.version)}`);
    console.log(`CHANNEL=${JSON.stringify(r.channel)}`);
    console.log(`BASELINE=${JSON.stringify(r.baseline || "")}`);
  }
});
