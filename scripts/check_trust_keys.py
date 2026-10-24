#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""relkit 凭据清单自检（key custody self-check）。

背景：
  relkit-candidate-2026 与 relkit-candidate-2026b 两次都被判定为"私钥丢失"，
  实际两次都不是真丢，而是**没记录去哪找**。断代后的模型把这件事反过来做：
  程序不再知道任何钥匙放在哪，只知道 keyId；要用钥匙时按 keyId 问清单
  （relkit.custody-manifest/1），清单告诉它从哪个文件或哪个环境变量读。

  于是"清单和现实脱节"会立刻暴露：缺项、载体不可读、私钥与登记公钥指纹
  对不上，程序启动就报错并指出是哪个 keyId。

本脚本是这条规则的脚本侧入口。它自己不解析清单——清单解析、载体读取、
签名往返全部由 relkit 的同一个清单读取模块完成，脚本只负责调用与汇总：

    python scripts/check_trust_keys.py                     # 默认归档目录
    python scripts/check_trust_keys.py --vault D:\\path     # 指定清单所在目录
    python scripts/check_trust_keys.py --key internal-root # 只校验指定 keyId
    python scripts/check_trust_keys.py --json              # 机器可读

退出码：
  0  全部 PASS
  1  有缺项 / 载体不可读 / 指纹不符 / relkit 缺失

约定（见 docs/architecture.md 与 docs/guide/trust-root-custody.md）：
  任何一次"私钥找不到"，先跑本脚本。脚本说没有，才算真的没有。
  脚本说没有之后，正确动作是**评估换根的真实代价**，
  而不是直接 generate 一把新的。
"""
from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys

sys.stdout.reconfigure(encoding="utf-8", errors="replace")

DEFAULT_VAULT = r"D:\relkit-trust-archive\keys"
MANIFEST_NAME = "custody-manifest.json"


def find_relkit() -> str | None:
    """Locate the relkit binary that owns the manifest reading module."""
    candidates: list[str] = []
    explicit = os.environ.get("RELKIT_BIN")
    if explicit:
        candidates.append(explicit)
    for name in ("relkit.exe", "relkit"):
        found = shutil.which(name)
        if found:
            candidates.append(found)
    candidates.append(
        os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
                     "relkit.exe")
    )
    for path in candidates:
        if path and os.path.isfile(path):
            return path
    return None


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--vault", default=DEFAULT_VAULT,
                        help="目录，内含 " + MANIFEST_NAME)
    parser.add_argument("--manifest", default=None,
                        help="直接指定清单文件（优先于 --vault）")
    parser.add_argument("--key", action="append", default=[],
                        help="要校验的 keyId，可重复；缺省校验清单内全部")
    parser.add_argument("--json", action="store_true", help="机器可读输出")
    args = parser.parse_args()

    manifest_path = args.manifest or os.path.join(args.vault, MANIFEST_NAME)
    relkit = find_relkit()

    if not os.path.isfile(manifest_path):
        payload = {
            "manifest": manifest_path,
            "relkit": relkit,
            "ok": False,
            "failures": [f"manifest missing: {manifest_path}"],
        }
        if args.json:
            print(json.dumps(payload, indent=2, ensure_ascii=False))
        else:
            print(f"manifest : {manifest_path}")
            print("RESULT = FAIL")
            print(f"  - manifest missing: {manifest_path}")
        return 1

    if not relkit:
        payload = {
            "manifest": manifest_path,
            "relkit": None,
            "ok": False,
            "failures": ["relkit binary not found; set RELKIT_BIN or build ./cmd/relkit"],
        }
        if args.json:
            print(json.dumps(payload, indent=2, ensure_ascii=False))
        else:
            print(f"manifest : {manifest_path}")
            print("relkit   : NOT FOUND")
            print("RESULT = FAIL")
            print("  - relkit binary not found; the manifest is never parsed by this script")
        return 1

    command = [relkit, "custody", "check", "--manifest", manifest_path, "--json"]
    for key_id in args.key:
        command.extend(["--key", key_id])

    proc = subprocess.run(command, capture_output=True, text=True,
                          encoding="utf-8", errors="replace")
    stdout = (proc.stdout or "").strip()
    stderr = (proc.stderr or "").strip()

    report: dict = {}
    if stdout:
        try:
            report = json.loads(stdout)
        except json.JSONDecodeError:
            report = {}

    ok = proc.returncode == 0
    payload = {
        "manifest": manifest_path,
        "relkit": relkit,
        "ok": ok,
        "report": report,
        "failures": [] if ok else [stderr or "custody check failed"],
    }

    if args.json:
        print(json.dumps(payload, indent=2, ensure_ascii=False))
        return 0 if ok else 1

    print(f"manifest : {manifest_path}")
    print(f"relkit   : {relkit}")
    scope = report.get("scope")
    if scope:
        print(f"scope    : {scope}")
    print()
    for entry in report.get("keys", []):
        verified = entry.get("verified") or ""
        state = "OK  " if verified else "FAIL"
        print(f"[{state}] {entry.get('keyId')}  "
              f"({entry.get('kind')} via {entry.get('carrier')})")
        if entry.get("fingerprint"):
            print(f"          fingerprint {entry['fingerprint']}")
        if verified:
            print(f"          verified    {verified}")
        print()
    if not ok:
        print("RESULT = FAIL")
        for line in (stderr or "").splitlines():
            print(f"  - {line}")
        return 1
    print("RESULT = PASS  清单登记、载体读取与签名归属全部一致")
    return 0


if __name__ == "__main__":
    sys.exit(main())
