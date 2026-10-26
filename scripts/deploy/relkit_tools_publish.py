#!/usr/bin/env python3
"""Upload one release's tool build to a local relkit agent.

This runs on the release server, next to the agent, because that is where both
the publishing credential and a reachable agent live. The deploy CLI stages the
binaries in a scratch directory, writes a spec describing them, and invokes this
with RELKIT_TOOLS_TOKEN set; nothing else needs the plaintext token.

Publishing is what makes /v1/catalog answer at all: the agent serves a document
it only produces after a tool manifest has been signed, so a server that never
published has an empty catalog and hosts cannot bootstrap the CLI from it.

The URLs the agent signs come from its own publicURL (the CI face hosts already
use), never from the address this script talks to. Talking over loopback is
fine; the agent refuses to sign a loopback origin.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any

# Tools are fetched by version and component by hosts, and re-downloaded on a
# mismatch, so the upload is generous but still bounded: a runaway file would be
# a release bug, not something to accept silently.
MAX_UPLOAD_BYTES = 512 * 1024 * 1024


class PublishError(RuntimeError):
    """A publish step failed for a reason the operator can act on."""


def request(
    method: str,
    url: str,
    *,
    token: str,
    body: bytes | None = None,
    content_type: str = "",
    timeout: float = 120.0,
) -> tuple[int, bytes]:
    headers = {"Authorization": f"Bearer {token}"}
    if content_type:
        headers["Content-Type"] = content_type
    req = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read()
    except urllib.error.URLError as exc:
        raise PublishError(f"{method} {url} failed: {exc.reason}") from exc


def describe(body: bytes) -> str:
    text = body.decode("utf-8", "replace").strip()
    return text if len(text) <= 400 else text[:400] + "..."


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def upload(base: str, token: str, version: str, spec: dict[str, Any]) -> list[dict[str, Any]]:
    listed: list[dict[str, Any]] = []
    root = Path(spec["_root"])
    for entry in spec["files"]:
        path = root / entry["name"]
        if not path.is_file():
            raise PublishError(f"{path} is missing; the staged tool set is incomplete")
        size = path.stat().st_size
        if size <= 0:
            raise PublishError(f"{path} is empty")
        if size > MAX_UPLOAD_BYTES:
            raise PublishError(f"{path} is {size} bytes, above the {MAX_UPLOAD_BYTES} byte limit")
        url = (
            f"{base}/v1/tools/{urllib.parse.quote(version, safe='')}"
            f"/{urllib.parse.quote(entry['name'], safe='')}"
        )
        status, body = request(
            "PUT",
            url,
            token=token,
            body=path.read_bytes(),
            content_type="application/octet-stream",
        )
        if status not in (200, 201, 204):
            raise PublishError(f"uploading {entry['name']} returned {status}: {describe(body)}")
        listed.append(
            {
                "name": entry["name"],
                "component": entry.get("component") or "",
                "version": entry.get("version") or version,
                "platform": entry.get("platform") or "",
                "os": entry.get("os") or "",
                "arch": entry.get("arch") or "",
                "size": size,
                "sha256": file_sha256(path),
            }
        )
        print(f"  uploaded {entry['name']} ({size} bytes)")
    return listed


def publish(base: str, token: str, version: str, spec: dict[str, Any], files: list[dict[str, Any]]) -> dict[str, Any]:
    payload = {
        "version": version,
        "commit": spec.get("commit") or "",
        "files": files,
    }
    status, body = request(
        "POST",
        f"{base}/v1/tools",
        token=token,
        body=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        content_type="application/json",
    )
    if status not in (200, 201):
        raise PublishError(f"publishing {version} returned {status}: {describe(body)}")
    try:
        return json.loads(body.decode("utf-8") or "{}")
    except json.JSONDecodeError:
        return {}


def verify(base: str, token: str, version: str) -> dict[str, Any]:
    """Read back the catalog a host would read.

    A publish that reports success but leaves the catalog without the version
    would break every host's bootstrap silently, so the read-back is part of
    publishing rather than an optional check.
    """
    status, body = request("GET", f"{base}/v1/catalog", token=token)
    if status != 200:
        raise PublishError(f"/v1/catalog returned {status} after publishing: {describe(body)}")
    try:
        doc = json.loads(body.decode("utf-8"))
    except json.JSONDecodeError as exc:
        raise PublishError(f"/v1/catalog is not JSON: {describe(body)}") from exc
    # The catalog is relkit.distribution-catalog/1: "releases" lists every
    # published version with its components, and "current" names the one a
    # version-less host request resolves to.
    releases = doc.get("releases") or []
    versions = {str(item.get("version") or "") for item in releases if isinstance(item, dict)}
    if version not in versions:
        raise PublishError(
            f"/v1/catalog does not list version {version} "
            f"(it lists {', '.join(sorted(v for v in versions if v)) or 'nothing'})"
        )
    if str(doc.get("current") or "") != version:
        raise PublishError(
            f"/v1/catalog lists {version} but current is {doc.get('current') or '(unset)'}; "
            "hosts resolving the alias would install a different tool build"
        )
    return doc


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="publish a release's tools to a local relkit agent")
    parser.add_argument("--server", required=True, help="agent base URL, loopback is fine")
    parser.add_argument("--spec", required=True, help="JSON describing version and files")
    args = parser.parse_args(argv)

    token = os.environ.get("RELKIT_TOOLS_TOKEN", "").strip()
    if not token:
        print("RELKIT_TOOLS_TOKEN is empty; refusing to publish", file=sys.stderr)
        return 2
    spec = json.loads(Path(args.spec).read_text(encoding="utf-8"))
    spec["_root"] = str(Path(args.spec).parent)
    version = str(spec.get("version") or "").strip()
    if not version:
        print("spec has no version", file=sys.stderr)
        return 2
    base = args.server.rstrip("/")
    try:
        files = upload(base, token, version, spec)
        doc = publish(base, token, version, spec, files)
        verify(base, token, version)
    except PublishError as exc:
        print(str(exc), file=sys.stderr)
        return 1
    seq = doc.get("seq") or doc.get("sequence") or ""
    print(f"published {version} with {len(files)} file(s), catalog seq {seq or 'unreported'}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
