#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""relkit 信任根密钥自检（key custody self-check）。

背景（2026-10-20 两次踩坑）：
  relkit-candidate-2026 与 relkit-candidate-2026b 两次都被判定为"私钥丢失"，
  实际两次都不是真丢，而是**没记录去哪找**。真正的缺陷是托管规划：
    - 私钥副本散落在 tmp/、.sim-scratch/ 这类被 .gitignore 忽略的"草稿"目录，
      语义上等于"随时可删"，且没有任何权威入口可以查询；
    - 于是下次要用时只能靠记忆/全文搜索，搜不到就误判丢失，进而想换根。

本脚本把"密钥在哪、是不是同一把、还对不对"变成一条可重复执行的命令：

  1. 从权威归档（默认 D:\\relkit-trust-archive\\keys）读取登记清单；
  2. 对每个条目的公钥取指纹，与清单里的期望指纹比对；
  3. 对每个条目的私钥做一次签名往返，确认签名 keyId == 该 keyId
     （指纹对得上但私钥不对，会在这里被抓住）；
  4. 打印每个副本存在性、size、sha256，明确标出缺失与漂移。

用法：
  python scripts/check_trust_keys.py                 # 默认归档目录
  python scripts/check_trust_keys.py --vault D:\\path\\to\\vault
  python scripts/check_trust_keys.py --json          # 机器可读

退出码：
  0  全部 PASS
  1  有缺失 / 指纹漂移 / 签名不匹配

约定（写进 docs/incidents 与 ADR，别再靠口口相传）：
  任何一次"私钥找不到"，先跑本脚本。脚本说没有，才算真的没有。
  脚本说没有之后，正确动作是**评估换根的真实代价**（见 docs/guide/trust-root-custody.md），
  而不是直接 generate 一把新的。
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import sys

sys.stdout.reconfigure(encoding="utf-8", errors="replace")

DEFAULT_VAULT = r"D:\relkit-trust-archive\keys"
MANIFEST_NAME = "custody-manifest.json"

# 密钥清单的骨架。fingerprint 是唯一权威标识；副本路径供检索与自检。
# 新增信任根时在这里登记，而不是只在对话里说一句"我放哪了"。
FALLBACK_MANIFEST = {
    "schema": "relkit.trust-keys-custody/1",
    "note": "权威登记表。新增/轮换信任根必须更新本文件，否则视为未托管。",
    "keys": [
        {
            "keyId": "relkit-candidate-2026b",
            "role": "candidate-root",
            "fingerprint": "sha256:3dc1ccb45c46f72f92e00b42bb15034b6ae2b28a5b32dba69eac0c86ef5bbf33",
            "status": "active",
            "public": "relkit-candidate-2026b.public.pb",
            "private": "relkit-candidate-2026b.private.pb",
            "usedBy": [
                "control-plane catalog signatures",
                "operator profile signatures (tencent-internal)",
            ],
            "anchoredIn": [
                "D:\\workspace\\Loom\\.relkit\\resolution.json authorityFingerprint",
                "D:\\workspace\\Loom-Launcher\\.relkit\\resolution.json authorityFingerprint",
                "D:\\workspace\\GitHub\\SvnMergeTool\\.relkit\\resolution.json authorityFingerprint",
                "bkci .ci/{loom-editor,loom-launcher,svnmergetool}.yaml --candidate-root-fingerprint",
            ],
        },
        {
            "keyId": "relkit-distribution-2026",
            "role": "distribution-root",
            "fingerprint": "sha256:8bcb6c24c9ce672f315f96eff446d9cee7edd2c6f3bfa113a3d4927f00199598",
            "status": "active-public-only",
            "public": "relkit-distribution-2026.public.pb",
            "private": None,
            "note": (
                "私钥不在开发机（仅公钥用于信任校验）。若要签发新 catalog 之外的 "
                "distribution 级操作，需先确认私钥托管位置，不要臆造。"
            ),
            "anchoredIn": [
                "Loom / Loom-Launcher / SvnMergeTool relkit.project.json trustFingerprint",
            ],
        },
    ],
}


def sha256_file(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as fh:
        for chunk in iter(lambda: fh.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def find_ops() -> str | None:
    candidates = [
        os.path.join(os.environ.get("RELKIT_OPS", ""), "relkit-ops.exe") if os.environ.get("RELKIT_OPS") else "",
        r"D:\workspace\GitHub\relkit\tmp\trr\tools\relkit-ops.exe",
    ]
    for path in candidates:
        if path and os.path.isfile(path):
            return path
    return None


def run(cmd: list[str]) -> tuple[int, str, str]:
    proc = subprocess.run(cmd, capture_output=True, text=True,
                          encoding="utf-8", errors="replace")
    return proc.returncode, (proc.stdout or "").strip(), (proc.stderr or "").strip()


def fingerprint_of(ops: str, public_path: str) -> tuple[str | None, str]:
    code, out, err = run([ops, "key", "fingerprint", "-public", public_path])
    if code != 0:
        return None, (err or out)
    parts = out.split()
    if len(parts) < 2:
        return None, f"unexpected output: {out}"
    return parts[1], parts[0]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--vault", default=DEFAULT_VAULT)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()

    manifest_path = os.path.join(args.vault, MANIFEST_NAME)
    if os.path.isfile(manifest_path):
        with open(manifest_path, encoding="utf-8") as fh:
            manifest = json.load(fh)
        manifest_source = manifest_path
    else:
        manifest = FALLBACK_MANIFEST
        manifest_source = "built-in fallback (manifest not yet written)"

    ops = find_ops()
    report = {
        "vault": args.vault,
        "manifest": manifest_source,
        "ops": ops,
        "keys": [],
    }
    failures: list[str] = []

    if not os.path.isdir(args.vault):
        failures.append(f"vault directory missing: {args.vault}")

    for entry in manifest.get("keys", []):
        key_id = entry.get("keyId", "?")
        expected = entry.get("fingerprint", "")
        record = {
            "keyId": key_id,
            "role": entry.get("role"),
            "status": entry.get("status"),
            "expectedFingerprint": expected,
            "public": None,
            "private": None,
            "problems": [],
        }

        pub_name = entry.get("public")
        if pub_name:
            pub_path = os.path.join(args.vault, pub_name)
            if not os.path.isfile(pub_path):
                record["problems"].append(f"public key missing: {pub_path}")
                failures.append(f"{key_id}: public key missing")
            else:
                record["public"] = {
                    "path": pub_path,
                    "bytes": os.path.getsize(pub_path),
                    "sha256": sha256_file(pub_path),
                }
                if ops:
                    fp, actual_id = fingerprint_of(ops, pub_path)
                    record["public"]["fingerprint"] = fp
                    record["public"]["keyId"] = actual_id
                    if fp != expected:
                        record["problems"].append(
                            f"public fingerprint drift: {fp} != {expected}")
                        failures.append(f"{key_id}: public fingerprint drift")
                    if actual_id != key_id:
                        record["problems"].append(
                            f"public keyId mismatch: {actual_id} != {key_id}")
                        failures.append(f"{key_id}: public keyId mismatch")
                else:
                    record["problems"].append("relkit-ops not found; fingerprint not verified")
                    failures.append(f"{key_id}: relkit-ops unavailable")

        priv_name = entry.get("private")
        if priv_name:
            priv_path = os.path.join(args.vault, priv_name)
            if not os.path.isfile(priv_path):
                record["problems"].append(f"private key missing: {priv_path}")
                failures.append(f"{key_id}: private key missing")
            else:
                priv_sha = sha256_file(priv_path)
                record["private"] = {
                    "path": priv_path,
                    "bytes": os.path.getsize(priv_path),
                    "sha256": priv_sha,
                }
                # 私钥归属校验：私钥本身不能直接取指纹（它是 privatekey schema），
                # 因此做一次签名往返，看签名出来的 keyId 是否等于登记值。
                if ops:
                    signed_id = sign_roundtrip_id(ops, priv_path, args.vault)
                    record["private"]["signedKeyId"] = signed_id
                    if signed_id != key_id:
                        record["problems"].append(
                            f"private key signs as {signed_id}, expected {key_id}")
                        failures.append(f"{key_id}: private key does not match keyId")
                else:
                    record["problems"].append("relkit-ops not found; sign roundtrip skipped")

        report["keys"].append(record)

    report["failures"] = failures
    report["ok"] = not failures

    if args.json:
        print(json.dumps(report, indent=2, ensure_ascii=False))
    else:
        print(f"vault    : {args.vault}")
        print(f"manifest : {manifest_source}")
        print(f"ops      : {ops or 'NOT FOUND'}")
        print()
        for record in report["keys"]:
            state = "OK  " if not record["problems"] else "FAIL"
            print(f"[{state}] {record['keyId']}  ({record['role']}, {record['status']})")
            pub = record.get("public")
            if pub:
                print(f"          public  {pub['bytes']}B  sha256={pub['sha256'][:16]}…  "
                      f"fp={str(pub.get('fingerprint'))[:34]}…")
            if record.get("private"):
                priv = record["private"]
                print(f"          private {priv['bytes']}B  sha256={priv['sha256'][:16]}…  "
                      f"signsAs={priv.get('signedKeyId')}")
            elif record.get("status", "").startswith("active"):
                print("          private (not held on this host by design)")
            for problem in record["problems"]:
                print(f"          ! {problem}")
            print()
        if failures:
            print("RESULT = FAIL")
            for item in failures:
                print(f"  - {item}")
            return 1
        print("RESULT = PASS  密钥托管完整，指纹与签名归属全部一致")
    return 0 if not failures else 1


def sign_roundtrip_id(ops: str, private_path: str, workdir: str) -> str | None:
    """用私钥签一份最小 operator profile，返回签名里的 keyId。

    只依赖 operator sign，不需要联网、不写任何生产产物。
    """
    unsigned = {
        "schema": "relkit.operator-profile/1",
        "deploymentId": "custody-selfcheck",
        "serverRelease": {
            "version": "v0.0.0",
            "commit": "0" * 40,
            "distributionDigest": "sha256:" + "0" * 64,
        },
        "selectionPolicy": "exact",
        "provides": [
            {"domain": "publishProtocol", "max": 3, "min": 2, "peer": "agent"},
            {"domain": "artifactSet", "id": "relkit.artifact-set/1", "peer": "producer"},
            {"domain": "updaterIpc", "max": 3, "min": 3, "peer": "facade"},
            {"domain": "rupSchemaSet", "id": "rup.v2", "peer": "directory"},
        ],
        "publishTransaction": ["artifact", "metadata", "index"],
        "endpoints": {
            "agent": "http://127.0.0.1/",
            "consumer": "http://127.0.0.1/",
            "directory": "http://127.0.0.1/",
        },
        "backends": ["intranet"],
        "serviceIdentity": "custody-selfcheck",
        "secretRefs": {"agentBearer": "env://RELKIT_UPLOAD_TOKEN"},
        "sources": {"mirrors": ["http://127.0.0.1/"]},
        "sequence": 1,
    }
    tmp_dir = os.path.join(workdir, ".selfcheck")
    os.makedirs(tmp_dir, exist_ok=True)
    unsigned_path = os.path.join(tmp_dir, "operator-unsigned.json")
    signed_path = os.path.join(tmp_dir, "operator-signed.json")
    with open(unsigned_path, "w", encoding="utf-8", newline="\n") as fh:
        json.dump(unsigned, fh, indent=2)
        fh.write("\n")
    code, out, err = run([ops, "operator", "sign",
                          "--input", unsigned_path,
                          "--out", signed_path,
                          "--key", private_path])
    if code != 0:
        return f"sign failed: {(err or out)[:120]}"
    try:
        with open(signed_path, encoding="utf-8") as fh:
            signed = json.load(fh)
        return signed["signatures"][0]["keyId"]
    except Exception as exc:  # noqa: BLE001
        return f"parse failed: {exc}"


if __name__ == "__main__":
    sys.exit(main())
