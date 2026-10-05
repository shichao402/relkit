#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Update T8.2 section head in the CI release stability plan (single-line swap)."""
import io

PATH = r"D:/workspace/GitHub/relkit/docs/_pending/2026-10-04-ci-release-stability-plan.md"

OLD_HEAD = (
    "- T8.2 消费端改造（1-2 天）【SvnMergeTool 侧已完成 2026-10-04，#43 全绿实证；"
    "loom/launcher 侧未开始】：各产品仓 ci_prepare_env 从\"ci-env 孤儿分支 clone + 物化\"改为："
    "第 0 步读本仓 lock 的 relkit 版本 → 阻塞式触发蓝盾同步流水线（等执行完成）；"
)

NEW_HEAD = (
    "- T8.2 消费端改造（1-2 天）【三仓已全部实施 2026-10-04，两仓全绿一仓验证中："
    "SvnMergeTool #43 全绿（六轮排障收口）；"
    "Loom-Launcher #21 全绿（#17/#18 编排与 Join-Path 三参→422a3db/7ef2d0f，"
    "#19 MSVC 探测误报→Test-Msvc 三段式宽松探测 9ea46da，"
    "#20 nsis 从 sourceforge URL 尾段推导文件名得无后缀 download 被 Expand-Archive 拒"
    "→toolchain.json 显式 filename 1a854b5，"
    "#21 全绿：nsis mirrors miss→官方源下载→backfill PUT 201→lazy-load 消费成功，"
    "relkit 五资产 mirrors 命中 v0.5.12，Publish code 29 sequence 16，RUP dev 索引 +29 节点落地）；"
    "Loom editor 六件套 59d7770（+743/-261）+ bkci loom-editor.yaml v4（3dcfdd4，"
    "Sync stage SubPipelineExec 阻塞调用 + MIRRORS_* 注入）+ PAC 同步确认 latestVersion 6 "
    "stages [stage-1, Sync, Build]；验证轮 #28（dev/0.2.2+70，lock v0.5.13 首次按需同步 mirrors 新分区）"
    "进行中】：各产品仓 ci_prepare_env 从\"ci-env 孤儿分支 clone + 物化\"改为："
    "第 0 步读本仓 lock 的 relkit 版本 → 阻塞式触发蓝盾同步流水线（等执行完成）；"
)


def main():
    with io.open(PATH, "r", encoding="utf-8") as fh:
        lines = fh.readlines()
    count = 0
    for i, line in enumerate(lines):
        if OLD_HEAD in line:
            lines[i] = line.replace(OLD_HEAD, NEW_HEAD, 1)
            count += 1
    assert count == 1, "expect exactly one T8.2 head, got %d" % count
    with io.open(PATH, "w", encoding="utf-8", newline="") as fh:
        fh.writelines(lines)
    print("replaced:", count)


if __name__ == "__main__":
    main()
