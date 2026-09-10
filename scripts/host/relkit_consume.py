#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Install immutable relkit release artifacts into a host repository.

Copy this file byte-for-byte to ``scripts/relkit_consume.py`` in the host.
All variable input is pinned by ``scripts/relkit.lock.json``. This consumer
never clones relkit, installs Go, or builds source code.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import shutil
import subprocess
import sys
import tempfile
import time
import traceback
import zipfile
from pathlib import Path
from typing import Any, Optional, Sequence
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

LOCK_SCHEMA = "relkit.consume/2"
COMPONENTS = ("sdk-dart", "cli", "updater")
TARGETS = (
    "linux-amd64",
    "linux-arm64",
    "windows-amd64",
    "darwin-amd64",
    "darwin-arm64",
)
DOWNLOAD_ATTEMPTS = 3


def force_utf8_stdio() -> None:
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if reconfigure is not None:
            try:
                reconfigure(encoding="utf-8", errors="replace")
            except (ValueError, OSError):
                pass


def host_root(script_file: Optional[Path] = None) -> Path:
    return (script_file or Path(__file__)).resolve().parent.parent


def load_lock(path: Path) -> dict[str, Any]:
    data = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise RuntimeError(f"{path} is not a JSON object")
    if data.get("schema") != LOCK_SCHEMA:
        raise RuntimeError(
            f"{path} must use schema {LOCK_SCHEMA!r}; source-build locks are unsupported"
        )
    release = str(data.get("release") or "").strip()
    commit = str(data.get("commit") or "").strip()
    if not release or not commit:
        raise RuntimeError(f"{path} must pin non-empty release and commit")
    artifacts = data.get("artifacts")
    if not isinstance(artifacts, dict):
        raise RuntimeError(f"{path} must contain an artifacts object")
    return data


def host_target() -> str:
    system = platform.system().lower()
    machine = platform.machine().lower()
    os_name = {"windows": "windows", "linux": "linux", "darwin": "darwin"}.get(
        system
    )
    if os_name is None:
        raise RuntimeError(f"unsupported host OS: {system}")
    arch = "arm64" if machine in ("arm64", "aarch64") else "amd64"
    target = f"{os_name}-{arch}"
    if target not in TARGETS:
        raise RuntimeError(f"unsupported host target: {target}")
    return target


def artifact_spec(
    lock: dict[str, Any], component: str, target: str
) -> dict[str, str]:
    artifacts = lock["artifacts"]
    raw = artifacts.get(component)
    if component != "sdk-dart":
        raw = raw.get(target) if isinstance(raw, dict) else None
    if not isinstance(raw, dict):
        suffix = "" if component == "sdk-dart" else f" for {target}"
        raise RuntimeError(f"lock has no {component} artifact{suffix}")
    url = str(raw.get("url") or "").strip()
    digest = str(raw.get("sha256") or "").strip().lower()
    if not url.startswith(("https://", "http://")):
        raise RuntimeError(f"{component} artifact URL must be absolute HTTP(S)")
    if len(digest) != 64 or any(ch not in "0123456789abcdef" for ch in digest):
        raise RuntimeError(f"{component} artifact has invalid sha256")
    return {"url": url, "sha256": digest}


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def download_artifact(root: Path, component: str, spec: dict[str, str]) -> Path:
    cache = root / ".relkit" / "artifacts"
    cache.mkdir(parents=True, exist_ok=True)
    destination = cache / spec["sha256"]
    if destination.is_file() and file_sha256(destination) == spec["sha256"]:
        print(f"relkit consume: cache hit {component} {spec['sha256'][:12]}")
        return destination
    destination.unlink(missing_ok=True)
    errors: list[str] = []
    for attempt in range(1, DOWNLOAD_ATTEMPTS + 1):
        temporary = destination.with_suffix(f".tmp-{os.getpid()}")
        temporary.unlink(missing_ok=True)
        try:
            print(f"relkit consume: download {component} ({attempt}/{DOWNLOAD_ATTEMPTS})")
            request = Request(
                spec["url"],
                headers={"User-Agent": "relkit-consume/2"},
                method="GET",
            )
            with urlopen(request, timeout=120) as response, temporary.open("wb") as out:
                shutil.copyfileobj(response, out)
            actual = file_sha256(temporary)
            if actual != spec["sha256"]:
                raise RuntimeError(
                    f"sha256 mismatch: expected {spec['sha256']}, got {actual}"
                )
            os.replace(temporary, destination)
            return destination
        except (HTTPError, URLError, OSError, TimeoutError, RuntimeError) as error:
            temporary.unlink(missing_ok=True)
            errors.append(str(error))
            if attempt < DOWNLOAD_ATTEMPTS:
                time.sleep(2**attempt)
    raise RuntimeError(
        f"cannot download {component} from {spec['url']}:\n  - "
        + "\n  - ".join(errors)
    )


def binary_destination(root: Path, component: str, target: str) -> Path:
    windows = target.startswith("windows-")
    if component == "cli":
        if windows:
            name = "relkit.exe"
        elif target == "linux-amd64":
            name = "relkit-linux-amd64"
        else:
            name = "relkit"
    elif component == "updater":
        name = "relkit-updater.exe" if windows else "relkit-updater"
    else:
        raise RuntimeError(f"{component} is not a binary component")
    return root / "tools" / "bin" / name


def install_binary(
    root: Path, component: str, target: str, artifact: Path, digest: str
) -> Path:
    destination = binary_destination(root, component, target)
    if destination.is_file() and file_sha256(destination) == digest:
        print(f"relkit consume: verified {destination.relative_to(root)}")
        return destination
    destination.parent.mkdir(parents=True, exist_ok=True)
    temporary = destination.with_suffix(destination.suffix + f".tmp-{os.getpid()}")
    shutil.copyfile(artifact, temporary)
    if not target.startswith("windows-"):
        temporary.chmod(temporary.stat().st_mode | 0o755)
    os.replace(temporary, destination)
    if file_sha256(destination) != digest:
        raise RuntimeError(f"installed binary hash drifted: {destination}")
    print(f"relkit consume: installed {destination.relative_to(root)}")
    return destination


def safe_extract_sdk(artifact: Path, destination: Path, digest: str) -> None:
    marker = destination / ".relkit-artifact.json"
    if marker.is_file():
        try:
            state = json.loads(marker.read_text(encoding="utf-8"))
            if (
                state.get("sha256") == digest
                and (destination / "pubspec.yaml").is_file()
                and (destination / "lib").is_dir()
            ):
                print("relkit consume: verified third_party/relkit/sdk/dart")
                return
        except (OSError, ValueError):
            pass

    destination.parent.mkdir(parents=True, exist_ok=True)
    temporary = Path(
        tempfile.mkdtemp(prefix=".dart-sdk-", dir=str(destination.parent))
    )
    backup = destination.with_name(destination.name + ".relkit-old")
    try:
        with zipfile.ZipFile(artifact) as archive:
            root = temporary.resolve()
            for member in archive.infolist():
                target = (temporary / member.filename).resolve()
                if target != root and root not in target.parents:
                    raise RuntimeError(f"unsafe SDK archive path: {member.filename}")
            archive.extractall(temporary)
        if not (temporary / "pubspec.yaml").is_file() or not (temporary / "lib").is_dir():
            raise RuntimeError("Dart SDK artifact lacks pubspec.yaml or lib/")
        (temporary / ".relkit-artifact.json").write_text(
            json.dumps({"schema": LOCK_SCHEMA, "sha256": digest}, indent=2) + "\n",
            encoding="utf-8",
        )
        if backup.exists():
            shutil.rmtree(backup)
        if destination.exists():
            os.replace(destination, backup)
        os.replace(temporary, destination)
        if backup.exists():
            shutil.rmtree(backup)
        print("relkit consume: installed third_party/relkit/sdk/dart")
    except Exception:
        if not destination.exists() and backup.exists():
            os.replace(backup, destination)
        raise
    finally:
        if temporary.exists():
            shutil.rmtree(temporary)


def verify_binary(path: Path, component: str) -> None:
    result = subprocess.run(
        [str(path), "--version"],
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        timeout=30,
        check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"{component} smoke test failed ({result.returncode}): "
            f"{(result.stderr or result.stdout).strip()}"
        )


def check_installed(
    root: Path, lock: dict[str, Any], component: str, target: str
) -> None:
    spec = artifact_spec(lock, component, target)
    if component == "sdk-dart":
        destination = root / "third_party" / "relkit" / "sdk" / "dart"
        marker = destination / ".relkit-artifact.json"
        state = json.loads(marker.read_text(encoding="utf-8"))
        if state.get("sha256") != spec["sha256"]:
            raise RuntimeError("installed Dart SDK does not match lock")
        if not (destination / "pubspec.yaml").is_file():
            raise RuntimeError("installed Dart SDK is incomplete")
        return
    destination = binary_destination(root, component, target)
    if not destination.is_file() or file_sha256(destination) != spec["sha256"]:
        raise RuntimeError(f"installed {component} does not match lock: {destination}")
    verify_binary(destination, component)


def create_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Install checksum-pinned relkit release artifacts"
    )
    parser.add_argument("command", choices=("install", "check"))
    parser.add_argument("--lock", help="relkit.consume/2 lock JSON")
    parser.add_argument("--project-root", help="host project root")
    parser.add_argument("--target", default="host", choices=("host", *TARGETS))
    parser.add_argument(
        "--component",
        action="append",
        choices=COMPONENTS,
        help="component to install/check; repeatable (default: all)",
    )
    parser.add_argument("--resolved-out", help="write resolved installation JSON")
    return parser


def main(argv: Optional[Sequence[str]] = None) -> int:
    force_utf8_stdio()
    try:
        args = create_parser().parse_args(argv)
        root = (
            Path(args.project_root).resolve()
            if args.project_root
            else host_root()
        )
        lock_path = (
            Path(args.lock).resolve()
            if args.lock
            else root / "scripts" / "relkit.lock.json"
        )
        lock = load_lock(lock_path)
        target = host_target() if args.target == "host" else args.target
        components = args.component or list(COMPONENTS)
        resolved_artifacts: dict[str, str] = {}
        for component in components:
            spec = artifact_spec(lock, component, target)
            resolved_artifacts[component] = spec["sha256"]
            if args.command == "install":
                artifact = download_artifact(root, component, spec)
                if component == "sdk-dart":
                    safe_extract_sdk(
                        artifact,
                        root / "third_party" / "relkit" / "sdk" / "dart",
                        spec["sha256"],
                    )
                else:
                    destination = install_binary(
                        root, component, target, artifact, spec["sha256"]
                    )
                    verify_binary(destination, component)
            check_installed(root, lock, component, target)
        resolved = {
            "schema": LOCK_SCHEMA,
            "release": lock["release"],
            "commit": lock["commit"],
            "target": target,
            "artifacts": resolved_artifacts,
        }
        if args.resolved_out:
            output = Path(args.resolved_out)
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_text(json.dumps(resolved, indent=2) + "\n", encoding="utf-8")
        print(
            f"relkit consume: {args.command} complete "
            f"release={lock['release']} target={target}"
        )
        return 0
    except Exception as error:
        print(f"ERROR: relkit consume failed: {error}", file=sys.stderr)
        print(traceback.format_exc(), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
