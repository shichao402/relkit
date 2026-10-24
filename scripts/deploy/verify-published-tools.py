#!/usr/bin/env python3
"""Verify a published toolchain the way a host does.

Run this after publishing tools, from a machine that is not the server, over the
origin the signed manifest actually names. Two failure modes are invisible from
loopback on the server and both of them ship silently:

- the manifest signs download URLs no host can reach (for example when the read
  vhost does not proxy /v1/tools/ to the agent), and
- the sidecar signature does not verify with the public key the manifest carries.

Every digest and every verification step works on bytes read from disk. The
uploads are binaries and the signatures cover their exact bytes, so decoding them
as text makes verification fail with "incorrect signature" for a reason that has
nothing to do with the release.

Usage:
    python scripts/deploy/verify-published-tools.py --base https://update-internal.devcloud.woa.com --version <version>
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import urllib.request

# The signature namespace is fixed by the server's signing side; a mismatch here
# reports a broken signature even when the signature is fine.
SIGN_NAMESPACE = "relkit-tool"


def fetch(url: str, timeout: float = 120.0) -> bytes:
    with urllib.request.urlopen(url, timeout=timeout) as response:
        return response.read()


def head(url: str, timeout: float = 60.0) -> dict[str, str]:
    request = urllib.request.Request(url, method="HEAD")
    with urllib.request.urlopen(request, timeout=timeout) as response:
        return {key.lower(): value for key, value in response.headers.items()}


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="verify a published relkit toolchain")
    parser.add_argument("--base", required=True, help="origin hosts download from, e.g. https://update-internal.devcloud.woa.com")
    parser.add_argument("--version", required=True, help="tool version to verify, as published")
    parser.add_argument("--keep", action="store_true", help="keep downloaded files for inspection")
    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    if shutil.which("ssh-keygen") is None:
        print("ssh-keygen is not on PATH; it is the only verifier for these signatures", file=sys.stderr)
        return 2
    base = args.base.rstrip("/")
    version = args.version
    problems: list[str] = []

    print("--- catalog ---")
    catalog = json.loads(fetch(f"{base}/v1/catalog").decode("utf-8"))
    print(f"sequence = {catalog.get('sequence')}")
    print(f"current  = {catalog.get('current')}")
    versions = {str(item.get("version") or "") for item in catalog.get("releases") or []}
    if version not in versions:
        problems.append(f"catalog does not list {version}")
    # An alias-resolving host installs whatever current names, so a catalog whose
    # current is a different version would hand that host another build.
    if str(catalog.get("current") or "") != version:
        problems.append(f"catalog current is {catalog.get('current') or '(unset)'}, not {version}")

    print("--- root public key ---")
    root_pub = fetch(f"{base}/v1/tools/root.pub").decode("utf-8").strip()
    print(root_pub[:60] + "...")
    if not root_pub.startswith(("ssh-ed25519 ", "ssh-rsa ")):
        problems.append("root.pub is not an OpenSSH public key line")

    print("--- manifest ---")
    manifest = json.loads(fetch(f"{base}/v1/tools/{version}/manifest.json").decode("utf-8"))
    tools = manifest.get("tools") or []
    print(f"schema={manifest.get('schema')} rootKeyId={manifest.get('rootKeyId')} tools={len(tools)}")
    if not tools:
        problems.append("manifest lists no tools")
    if manifest.get("rootPublicKey") and manifest["rootPublicKey"].strip() != root_pub:
        problems.append("manifest carries a different root public key than /v1/tools/root.pub")

    work = tempfile.mkdtemp(prefix="relkit-tool-verify-")
    try:
        # Principal "*" is the form ssh-keygen -Y verify accepts here. Writing the
        # keyId on the line like an authorized_keys entry makes every check fail
        # with "Could not verify signature".
        allowed = os.path.join(work, "allowed_signers")
        with open(allowed, "w", encoding="utf-8", newline="\n") as handle:
            handle.write("* " + root_pub + "\n")

        print("--- download, digest, signature ---")
        for tool in tools:
            name = tool["name"]
            path = os.path.join(work, name)
            payload = fetch(tool["url"])
            with open(path, "wb") as handle:
                handle.write(payload)

            digest = "sha256:" + hashlib.sha256(payload).hexdigest()
            digest_ok = digest == tool["sha256"]

            signature = fetch(tool["signatureUrl"])
            signature_path = path + ".sig"
            with open(signature_path, "wb") as handle:
                handle.write(signature)

            result = subprocess.run(
                ["ssh-keygen", "-Y", "verify", "-f", allowed, "-I", "*", "-n", SIGN_NAMESPACE, "-s", signature_path],
                input=payload,
                capture_output=True,
            )
            signature_ok = result.returncode == 0
            mark = "OK  " if digest_ok and signature_ok else "FAIL"
            print(
                f"  {mark} {tool.get('component', ''):<8} {tool.get('platform', ''):<14} {name} "
                f"digest={'ok' if digest_ok else 'MISMATCH'} "
                f"signature={'ok' if signature_ok else 'BAD'}"
            )
            if not signature_ok:
                detail = result.stderr.decode("utf-8", "replace").strip()
                print("    " + (detail[:300] or "(no stderr)"))
            if not (digest_ok and signature_ok):
                problems.append(name)

        print("--- component addressing (what the frozen CI template uses) ---")
        for tool in tools:
            url = f"{base}/v1/tools/current/{tool['component']}/{tool['platform']}"
            headers = head(url)
            declared_name = headers.get("x-relkit-tool-name", "")
            declared_sha = headers.get("x-relkit-tool-sha256", "")
            same_name = declared_name == tool["name"]
            same_sha = declared_sha.replace("sha256:", "") == tool["sha256"].replace("sha256:", "")
            print(
                f"  {'OK  ' if same_name and same_sha else 'FAIL'} "
                f"{tool['component']}/{tool['platform']} -> {declared_name or '(none)'}"
            )
            if not (same_name and same_sha):
                problems.append(f"{tool['component']}/{tool['platform']}")
    finally:
        if args.keep:
            print(f"kept downloads in {work}")
        else:
            shutil.rmtree(work, ignore_errors=True)

    if problems:
        print("FAILED: " + ", ".join(problems))
        return 1
    print("ALL OK")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
