#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Update T8.3 section in the CI release stability plan (single-line swap)."""
import io

PATH = r"D:/workspace/GitHub/relkit/docs/_pending/2026-10-04-ci-release-stability-plan.md"

OLD = "- T8.3 收尾（半天）：mirrors 存量盘点（`mirror/github/relkit/` ≤5 代确认 + 懒加载回填区域自然生长状况）；五仓各一次 dev 验证（第 0 步同步 + 换机验证）"

NEW = (
    "- T8.3 收尾（半天）【已完成 2026-10-04】：mirrors 存量盘点——`mirror/github/relkit/` 现存 4 代"
    "（v0.5.12/13/15/17，≤5 达标；v0.5.13 分区由 editor #28 Sync 阶段首次按需同步创建），"
    "懒加载回填区 node 26.5.0 / python 3.12.10 / rustup / nsis 3.10 全部生长命中"
    "（nsis-3.10.zip 由 launcher #21 官方源下载回填、editor #28 跨仓消费）；"
    "已知残留：`mirror/nsis/3.10/download` 是 launcher #20 旧 bug（URL 尾段推导文件名）失败构建"
    "回填的无后缀脏数据，filename 契约修复后不再有消费方请求该路径，无害死数据，"
    "待持 generic 写凭据人工 DELETE（agent 侧只有匿名读）。"
    "五仓 dev 验证全绿：SvnMergeTool #43、Loom-Launcher #21（RUP dev 索引 +29）、"
    "Loom editor #28（RUP dev 索引 +70，lock v0.5.13 首次按需同步）、"
    "cronkit 0.1.0+25（公网 dev 索引，GitHub Actions 链）、dec v1.13.112（公网 dev 索引）。"
    "外网两仓（dec/cronkit）走 GitHub Actions + go run 直连源，不经蓝盾同步流水线，拓扑本就独立，验证确认其发布链未受内网改造影响"
)


def main():
    with io.open(PATH, "r", encoding="utf-8") as fh:
        lines = fh.readlines()
    count = 0
    for i, line in enumerate(lines):
        if OLD in line:
            lines[i] = line.replace(OLD, NEW, 1)
            count += 1
    assert count == 1, "expect exactly one T8.3 line, got %d" % count
    with io.open(PATH, "w", encoding="utf-8", newline="") as fh:
        fh.writelines(lines)
    print("replaced:", count)


if __name__ == "__main__":
    main()
