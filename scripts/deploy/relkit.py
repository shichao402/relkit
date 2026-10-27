#!/usr/bin/env python3
"""Unified relkit deploy CLI: build, empty-machine install, upgrade binaries.

Stdlib only unless scripts/deploy/requirements.txt lists packages. In that case
this file creates scripts/deploy/.venv, pip-installs, and re-execs itself.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import secrets
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.request
import zipfile
from datetime import datetime, timezone
from pathlib import Path, PurePosixPath
from typing import Any, Optional, Sequence

DEPLOY_DIR = Path(__file__).resolve().parent


def _repo_root() -> Path:
    for candidate in (DEPLOY_DIR, *DEPLOY_DIR.parents):
        if (candidate / "go.mod").is_file():
            return candidate
    return DEPLOY_DIR.parent


REPO_ROOT = _repo_root()
REQUIREMENTS = DEPLOY_DIR / "requirements.txt"
VENV_DIR = DEPLOY_DIR / ".venv"
BOOTSTRAP_ENV = "RELKIT_DEPLOY_BOOTSTRAPPED"
MIN_PY = (3, 9)
# Sibling copies on a remote /tmp/relkit-deploy load relkit_ops and components
# from the same directory they were shipped into.
if str(DEPLOY_DIR) not in sys.path:
    sys.path.insert(0, str(DEPLOY_DIR))
from components import (  # noqa: E402
    BY_NAME,
    REGISTRY_PATH,
    TARGETS,
    components as registry_components,
)

from relkit_ops import (  # noqa: E402
    admin_merge,
    dump_json,
    ensure_cas_grace,
    epoch_split_agent_config,
    extract_export,
    load_json_object,
    loopback_base,
    migrate_agent_config,
    migrate_profile,
    missing_upgrade_targets,
    parse_exec_binary,
    parse_exec_config,
    parse_listen_port,
    parse_requirements,
    parse_ssot_document,
    read_write_paths,
    redact_text,
    redact_value,
    render_agent_unit,
    render_console_unit,
    render_store_unit,
    token_mode_ok,
)


class Fail(Exception):
    pass


def die(message: str, code: int = 1) -> None:
    print(f"error: {message}", file=sys.stderr)
    raise SystemExit(code)


def step(message: str) -> None:
    print(f"\n==> {message}")


def venv_python() -> Path:
    if os.name == "nt":
        return VENV_DIR / "Scripts" / "python.exe"
    return VENV_DIR / "bin" / "python"


def ensure_bootstrap() -> None:
    if os.environ.get(BOOTSTRAP_ENV) == "1":
        return
    if sys.version_info < MIN_PY:
        die(
            f"Python {MIN_PY[0]}.{MIN_PY[1]}+ is required "
            f"(found {sys.version.split()[0]}). Install python3 and retry."
        )
    reqs: list[str] = []
    if REQUIREMENTS.is_file():
        reqs = parse_requirements(REQUIREMENTS.read_text(encoding="utf-8"))
    if not reqs:
        os.environ[BOOTSTRAP_ENV] = "1"
        return
    py = venv_python()
    stamp = VENV_DIR / ".relkit-req-hash"
    wanted = str(hash(tuple(reqs)))
    if py.is_file() and stamp.is_file() and stamp.read_text(encoding="utf-8").strip() == wanted:
        if Path(sys.executable).resolve() != py.resolve():
            os.environ[BOOTSTRAP_ENV] = "1"
            os.execv(str(py), [str(py), str(Path(__file__).resolve()), *sys.argv[1:]])
        os.environ[BOOTSTRAP_ENV] = "1"
        return
    print("==> bootstrap: creating scripts/deploy/.venv and installing requirements")
    subprocess.run([sys.executable, "-m", "ensurepip", "--upgrade"], check=False)
    VENV_DIR.mkdir(parents=True, exist_ok=True)
    created = subprocess.run(
        [sys.executable, "-m", "venv", str(VENV_DIR)],
        check=False,
    )
    if created.returncode != 0 or not py.is_file():
        die(
            "could not create scripts/deploy/.venv (python3-venv / ensurepip missing). "
            "Install a full Python 3.9+ and retry."
        )
    pip = subprocess.run(
        [str(py), "-m", "pip", "install", "-r", str(REQUIREMENTS)],
        check=False,
    )
    if pip.returncode != 0:
        die("pip install -r scripts/deploy/requirements.txt failed")
    stamp.write_text(wanted + "\n", encoding="utf-8")
    os.environ[BOOTSTRAP_ENV] = "1"
    os.execv(str(py), [str(py), str(Path(__file__).resolve()), *sys.argv[1:]])


def require_cmd(name: str) -> str:
    path = shutil.which(name)
    if not path:
        die(f"missing command {name!r}; install it and retry")
    return path


def run(
    argv: Sequence[str],
    *,
    cwd: Optional[Path] = None,
    check: bool = True,
    env: Optional[dict[str, str]] = None,
    capture: bool = False,
    input_text: Optional[str] = None,
) -> subprocess.CompletedProcess[str]:
    completed = subprocess.run(
        list(argv),
        cwd=str(cwd) if cwd else None,
        env=env,
        text=True,
        encoding="utf-8",
        errors="replace",
        input=input_text,
        capture_output=capture,
        check=False,
    )
    if check and completed.returncode != 0:
        err = (completed.stderr or completed.stdout or "").strip()
        raise Fail(f"{' '.join(argv)} failed ({completed.returncode}): {redact_text(err)}")
    return completed


def git_identity() -> dict[str, str]:
    head = run(["git", "rev-parse", "HEAD"], cwd=REPO_ROOT, check=False, capture=True)
    commit = (head.stdout or "").strip() or "unknown"
    dirty_out = run(["git", "status", "--porcelain"], cwd=REPO_ROOT, check=False, capture=True)
    dirty = bool((dirty_out.stdout or "").strip())
    return {"commit": commit, "dirty": "true" if dirty else "false"}


def load_ssot() -> dict[str, Any]:
    path = REPO_ROOT / "VERSION.json"
    if not path.is_file():
        die("missing VERSION.json; it is the relkit version SSOT")
    try:
        return parse_ssot_document(path.read_text(encoding="utf-8"))
    except ValueError as exc:
        die(str(exc))
        raise


def git_stamp(number: str) -> str:
    ident = git_identity()
    stamp = f"{number}+{ident['commit'][:12]}"
    if ident["dirty"] == "true":
        stamp += "-dirty"
    return stamp


def release_source_paths(*prefixes: str) -> list[str]:
    tracked = run(
        ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", *prefixes],
        cwd=REPO_ROOT,
        capture=True,
    ).stdout
    return sorted(item for item in tracked.split("\0") if item)


def write_deterministic_zip(
    destination: Path, entries: Sequence[tuple[Path, str]]
) -> None:
    with zipfile.ZipFile(
        destination,
        "w",
        compression=zipfile.ZIP_DEFLATED,
        compresslevel=9,
    ) as archive:
        for source, archive_name in sorted(entries, key=lambda item: item[1]):
            info = zipfile.ZipInfo(
                archive_name,
                date_time=(1980, 1, 1, 0, 0, 0),
            )
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o100644 << 16
            archive.writestr(info, source.read_bytes(), compresslevel=9)


def tree_entries(source: str) -> list[tuple[Path, str]]:
    source_root = REPO_ROOT / source
    paths = release_source_paths(source)
    if not paths:
        raise Fail(f"{source} has no tracked files")
    return [
        (REPO_ROOT / relative, (REPO_ROOT / relative).relative_to(source_root).as_posix())
        for relative in paths
        if (REPO_ROOT / relative).is_file()
    ]


def working_tree_entries(source: str, exclude_parts: Sequence[str]) -> list[tuple[Path, str]]:
    source_root = REPO_ROOT / source
    if not source_root.is_dir():
        raise Fail(f"{source} is missing")
    excluded = set(exclude_parts)
    return [
        (path, path.relative_to(source_root).as_posix())
        for path in sorted(source_root.rglob("*"))
        if path.is_file() and not excluded.intersection(path.parts)
    ]


def listed_file_entries(source: str, names: Sequence[str], component: str) -> list[tuple[Path, str]]:
    source_root = REPO_ROOT / source
    entries = [(source_root / relative, relative) for relative in names]
    missing = [relative for path, relative in entries if not path.is_file()]
    if missing:
        raise Fail(f"{component} is incomplete: " + ", ".join(missing))
    return entries


def component_entries(name: str) -> list[tuple[Path, str]]:
    row = BY_NAME[name]
    if row.role != "product-tree":
        raise Fail(f"{name} is not a packed product-tree")
    kind = row.pack_kind()
    if kind == "listed-files":
        entries = listed_file_entries(row.source, row.pack_files, row.name)
    elif kind == "working-tree":
        entries = working_tree_entries(row.source, row.pack_exclude_parts)
    elif kind == "tracked-tree":
        entries = tree_entries(row.source)
    else:
        # go-deps existed only for the retired sdk-go component; the epoch
        # upgrade moves the Go packages into internal/ and ships no Go SDK
        # (requirement 135 keeps pack-shape and this table in sync).
        raise Fail(f"{row.name} has unknown pack {row.pack}")
    for extra_src, extra_dest in row.pack_extras:
        path = REPO_ROOT / extra_src
        if not path.is_file():
            raise Fail(f"{row.name} extra {extra_src} is missing")
        entries.append((path, extra_dest))
    if row.pack_exclude_paths:
        entries = [
            item for item in entries
            if not pack_path_excluded(item[1], row.pack_exclude_paths)
        ]
    if not entries:
        raise Fail(f"{row.name} packed no files")
    # The retired source-grep facade gate (ADR 0019 §1) used to scan these
    # entries for an in-process updater. Structure alone decides what is packed
    # now: a facade's own conformance suite proves it, not a text match here.
    return entries


def pack_path_excluded(archive_name: str, prefixes: Sequence[str]) -> bool:
    for prefix in prefixes:
        if prefix.endswith("/"):
            if archive_name == prefix[:-1] or archive_name.startswith(prefix):
                return True
        elif archive_name == prefix or archive_name.startswith(prefix + "/"):
            return True
    return False


def rust_sdk_entries() -> list[tuple[Path, str]]:
    return component_entries("sdk-rust")


def cmd_build(args: argparse.Namespace) -> None:
    require_cmd("go")
    out_dir = Path(args.out)
    if not out_dir.is_absolute():
        out_dir = REPO_ROOT / out_dir
    out_dir.mkdir(parents=True, exist_ok=True)
    binary_rows = registry_components("host-binary") + registry_components("product-binary")
    tree_rows = registry_components("product-tree")
    if getattr(args, "all", False):
        targets = [row.name for row in binary_rows]
        selected_trees = list(tree_rows)
    else:
        targets = [
            row.name for row in binary_rows
            if getattr(args, row.build_flag.replace("-", "_"), False)
        ]
        selected_trees = [
            row for row in tree_rows
            if getattr(args, row.build_flag.replace("-", "_"), False)
        ]
        if not targets and not selected_trees:
            targets = ["store", "agent"]
    stamp = git_stamp(load_ssot()["number"])
    platforms = [tuple(target.split("-", 1)) for target in TARGETS]
    if args.os:
        platforms = [p for p in platforms if p[0] == args.os]
    if args.arch:
        platforms = [p for p in platforms if p[1] == args.arch]
    if not platforms:
        die("no build platforms left after --os/--arch filters")
    pkgs = {row.name: (row.go_package, row.binary_prefix) for row in binary_rows}
    step(f"build {stamp}")
    # The retired scripts/host/release-contract.json used to pin these windows
    # (ADR 0019 §1). Their Go sources are the SSOT now; validating the window
    # itself is `relkit build`'s job, and this deploy path keeps the same rule.
    ipc_min, ipc_max = updater_ipc_window()
    protocol_min, protocol_max = publish_protocol_window()
    if protocol_min <= 0 or protocol_max < protocol_min:
        die("internal/publishproto declares an invalid protocol window")
    if ipc_min <= 0 or ipc_max < ipc_min:
        die("internal/updater declares an invalid IPC window")
    protocol = {"min": protocol_min, "max": protocol_max}
    built: list[dict[str, Any]] = []
    manifest_file = out_dir / "manifest.json"
    if getattr(args, "append", False) and manifest_file.is_file():
        # A deploy ships infrastructure binaries and tools in one release, but
        # building them in two invocations must not leave the manifest claiming
        # only the second batch: the tools publish step verifies every tool
        # against this manifest, so an overwritten manifest would silently
        # shrink the release. Carry forward rows for the same version only;
        # a different stamp means the earlier rows describe other bytes.
        previous = load_json_object(manifest_file.read_text(encoding="utf-8"))
        if previous.get("version") == stamp:
            built.extend(
                row for row in (previous.get("artifacts") or [])
                if isinstance(row, dict) and row.get("sha256")
            )
    for kind in targets:
        pkg, prefix = pkgs[kind]
        row = BY_NAME[kind]
        # A component declares the platforms it actually ships for: `ops`
        # carries a darwin-universal entry the plain build matrix cannot
        # express, and building a target a component never publishes would put
        # an artifact into the release manifest that no host may ever download.
        component_platforms = platforms
        if row.targets and "all" not in row.targets:
            declared = {target for target in row.targets if "-" in target}
            narrowed = [p for p in platforms if f"{p[0]}-{p[1]}" in declared]
            if not narrowed:
                raise Fail(
                    f"{kind} declares targets {', '.join(row.targets)} but none of them "
                    f"is in the build matrix after --os/--arch filtering"
                )
            component_platforms = narrowed
        ldflags = f"-s -w -X main.version={stamp}"
        for os_name, arch in component_platforms:
            name = f"{prefix}-{os_name}-{arch}"
            if os_name == "windows":
                name += ".exe"
            dest = out_dir / name
            env = os.environ.copy()
            env["CGO_ENABLED"] = "0"
            env["GOOS"] = os_name
            env["GOARCH"] = arch
            # Never assign os.environ["GOOS"]: it would break later `go test` on Windows.
            print(f"  {name}")
            run(
                ["go", "build", "-trimpath", "-ldflags", ldflags, "-o", str(dest), pkg],
                cwd=REPO_ROOT,
                env=env,
            )
            built = [
                row for row in built
                if (row["component"], row["os"], row["arch"]) != (kind, os_name, arch)
            ]
            built.append({"component": kind, "os": os_name, "arch": arch, "path": dest.name, "sha256": file_sha256(dest)})
    for row in selected_trees:
        entries = component_entries(row.name)
        archive = out_dir / row.archive
        write_deterministic_zip(archive, entries)
        built.append(
            {
                "component": row.name,
                "os": "any",
                "arch": "any",
                "path": archive.name,
                "sha256": file_sha256(archive),
            }
        )
    ident = git_identity()
    manifest = {
        "schema": "relkit.release/1",
        "version": stamp,
        "commit": ident["commit"],
        "dirty": ident["dirty"] == "true",
        "minProtocol": protocol.get("min"),
        "maxProtocol": protocol.get("max"),
        "minUpdaterIpc": ipc_min,
        "maxUpdaterIpc": ipc_max,
        "builtAt": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "artifacts": built,
    }
    (out_dir / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(f"output in {out_dir}")


def updater_ipc_window() -> tuple[int, int]:
    """Sidecar IPC window from internal/updater/const.go (same numbers as the facades)."""
    text = (REPO_ROOT / "internal" / "updater" / "const.go").read_text(encoding="utf-8")
    found_min = re.search(r"IPCMin\s+uint32\s*=\s*(\d+)", text)
    found_max = re.search(r"IPCMax\s+uint32\s*=\s*(\d+)", text)
    if found_min is None or found_max is None:
        die("cannot read IPCMin/IPCMax from internal/updater/const.go")
    return int(found_min.group(1)), int(found_max.group(1))


def publish_protocol_window() -> tuple[int, int]:
    """Publisher HTTP window from internal/publishproto/publishproto.go."""
    text = (REPO_ROOT / "internal" / "publishproto" / "publishproto.go").read_text(
        encoding="utf-8"
    )
    found_min = re.search(r"^\s*Min\s*=\s*(\d+)\s*$", text, re.MULTILINE)
    found_max = re.search(r"^\s*Max\s*=\s*(\d+)\s*$", text, re.MULTILINE)
    if found_min is None or found_max is None:
        die("cannot read Min/Max from internal/publishproto/publishproto.go")
    return int(found_min.group(1)), int(found_max.group(1))


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def must_root() -> None:
    if hasattr(os, "geteuid") and os.geteuid() != 0:
        die("run with sudo")


def ensure_user(name: str, home: str) -> None:
    check = run(["id", name], check=False, capture=True)
    if check.returncode == 0:
        print(f"user {name}: already exists")
        return
    created = run(
        ["useradd", "-r", "-s", "/usr/sbin/nologin", "-d", home, name],
        check=False,
        capture=True,
    )
    if created.returncode != 0:
        run(["useradd", "-r", "-s", "/sbin/nologin", "-d", home, name])
    print(f"user {name}: created")


def install_file(src: Path, dest: Path, mode: int) -> None:
    dest.parent.mkdir(parents=True, exist_ok=True)
    tmp = dest.with_name(dest.name + ".new")
    shutil.copy2(src, tmp)
    os.chmod(tmp, mode)
    os.replace(tmp, dest)


def chown_path(path: Path, user: str) -> None:
    shutil.chown(str(path), user=user, group=user)


def http_call(
    method: str,
    url: str,
    *,
    headers: Optional[dict[str, str]] = None,
    data: Optional[bytes] = None,
    timeout: int = 15,
    publish_protocol: bool = True,
) -> tuple[int, bytes]:
    hdrs: dict[str, str] = {}
    if publish_protocol:
        hdrs["X-Relkit-Publish-Protocol"] = "2"
    if data is not None:
        hdrs["Content-Length"] = str(len(data))
    if headers:
        hdrs.update(headers)
    req = urllib.request.Request(url, data=data, method=method, headers=hdrs)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read()
    except urllib.error.URLError as exc:
        raise Fail(f"{method} {url}: {exc}") from exc


def wait_health(base: str, unit: str) -> None:
    url = base.rstrip("/") + "/-/health"
    for _ in range(15):
        try:
            code, _ = http_call("GET", url)
            if code == 200:
                return
        except Fail:
            pass
        time.sleep(1)
    log = run(["journalctl", "-u", unit, "-n", "40", "--no-pager"], check=False, capture=True)
    print(redact_text(log.stdout or ""), file=sys.stderr)
    raise Fail(f"{unit} did not become healthy at {url}")


def self_check_serve(
    addr: str, token: str, public_upload_url: Optional[str] = None
) -> None:
    local_base = loopback_base(addr)
    wait_health(local_base, "relkit-store")
    base = (public_upload_url or local_base).rstrip("/")
    if public_upload_url:
        print(f"relkit-compatible endpoint {base}")
    print("1/7 health              ok")
    code, _ = http_call("PUT", base + "/.probe~", data=b"x")
    if code == 405:
        raise Fail("upload endpoint is disabled; the service did not read the token")
    if code != 401:
        raise Fail(f"unauthenticated PUT returned {code}, expected 401")
    print("2/7 upload auth         ok")
    payload = b"relkit-store probe"
    digest = hashlib.sha256(payload).hexdigest()
    mint_body = json.dumps({"key": f"cas/{digest}", "size": len(payload), "ttl": 300}).encode()
    code, raw = http_call(
        "POST",
        base + "/-/cas/uploads",
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        },
        data=mint_body,
    )
    if code != 200:
        raise Fail(f"CAS mint returned {code}")
    minted = json.loads(raw.decode("utf-8", "replace"))
    cap = minted.get("url") if isinstance(minted, dict) else None
    if not cap:
        requests = minted.get("requests") if isinstance(minted, dict) else None
        if isinstance(requests, list) and requests:
            first = requests[0]
            if isinstance(first, dict):
                cap = first.get("url")
    if not isinstance(cap, str) or not cap:
        raise Fail("CAS capability response has no url")
    if "://" not in cap or not cap.startswith("http"):
        raise Fail("CAS capability url is not absolute")
    code, _ = http_call("PUT", cap, data=payload, publish_protocol=False)
    if code not in (200, 201, 204):
        raise Fail(f"capability PUT returned {code}")
    print("3/7 capability upload   ok")
    artifact = "/artifact/probe/1/probe.txt"
    code, _ = http_call(
        "PUT",
        base + artifact,
        headers={
            "Authorization": f"Bearer {token}",
            "X-Relkit-Copy-Source": f"cas/{digest}",
        },
    )
    if code not in (200, 201, 204):
        raise Fail(f"COPY promote returned {code}")
    print("4/7 copy promote        ok")
    code, _ = http_call(
        "HEAD",
        base + artifact,
        headers={"Authorization": f"Bearer {token}"},
    )
    if code != 200:
        raise Fail(f"authenticated HEAD returned {code}, expected 200")
    print("5/7 authenticated HEAD  ok")
    code, _ = http_call(
        "GET",
        base + artifact,
        headers={"Range": "bytes=0-3"},
    )
    if code != 206:
        raise Fail(f"ranged GET returned {code}, expected 206")
    print("6/7 range requests      ok")
    code, _ = http_call("GET", base + "/")
    if code not in (200, 404):
        raise Fail(f"GET / returned {code}")
    print("7/7 root                ok")
    http_call("DELETE", base + artifact, headers={"Authorization": f"Bearer {token}"})
    http_call(
        "DELETE",
        base + f"/cas/{digest}",
        headers={"Authorization": f"Bearer {token}"},
    )


def self_check_agent(addr: str) -> None:
    base = loopback_base(addr)
    wait_health(base, "relkit-agent")
    print("agent health ok")


def write_serve_unit(
    *,
    user: str,
    prefix: str,
    config_path: str,
    serve_cfg: dict[str, Any],
) -> None:
    template = (DEPLOY_DIR / "relkit-store.service").read_text(encoding="utf-8")
    unit = render_store_unit(
        template,
        user=user,
        prefix=prefix,
        config_path=config_path,
        read_write_paths=read_write_paths(serve_cfg),
        addr=str(serve_cfg.get("addr") or ""),
    )
    dest = Path("/etc/systemd/system/relkit-store.service")
    dest.write_text(unit, encoding="utf-8")
    os.chmod(dest, 0o644)
    verify = run(["systemd-analyze", "verify", str(dest)], check=False, capture=True)
    if verify.returncode != 0:
        raise Fail(redact_text(verify.stderr or verify.stdout or "unit verify failed"))


def write_agent_unit(
    *,
    user: str,
    prefix: str,
    config_path: str,
    working_directory: str,
) -> None:
    template = (DEPLOY_DIR / "relkit-agent.service").read_text(encoding="utf-8")
    unit = render_agent_unit(
        template,
        user=user,
        prefix=prefix,
        config_path=config_path,
        working_directory=working_directory,
    )
    dest = Path("/etc/systemd/system/relkit-agent.service")
    dest.write_text(unit, encoding="utf-8")
    os.chmod(dest, 0o644)


# The panel's own state: one cookie signing key, nothing else; losing it only
# signs operators out. It lives in the console's systemd StateDirectory, never
# in the release tree: the panel reads that tree through its tree adapter like
# any other client, and keeps no shortcut into it (same shape on every box).
CONSOLE_STATE_DIR = "/var/lib/relkit-console"
CONSOLE_STATE_FILE = CONSOLE_STATE_DIR + "/panel.json"

# The example a fresh console config is cut from. Its "tree" entry is the
# panel's tree adapter; this script copies the entry whole and never reads
# inside it, so the deploy layer does not know which adapter a box uses.
CONSOLE_CONFIG_EXAMPLE = "relkit-console.example.json"


def console_admin_path(console_cfg: dict[str, Any]) -> PurePosixPath:
    """Where the panel opens its cookie-key file, mirroring resolveAdminPath:
    empty means the StateDirectory default; anything else must be absolute."""
    configured = str(console_cfg.get("adminStateFile") or "").strip()
    if configured.startswith("/"):
        return PurePosixPath(configured)
    return PurePosixPath(CONSOLE_STATE_FILE)


def console_legacy_panel_files(console_cfg: dict[str, Any], config_path: str) -> list[Path]:
    """Where an older console kept its cookie key, in preference order.

    Older layouts put it in the release tree (named in `adminStateFile`, or
    under the serve-era or panel-era default name), or, through a relative
    `adminStateFile`, beside the config. Moving the first one that exists keeps
    every operator signed in across the upgrade.
    """
    out: list[Path] = []
    configured = str(console_cfg.get("adminStateFile") or "").strip()
    tree = str(console_cfg.get("dir") or "").strip()
    if configured:
        if configured.startswith("/"):
            out.append(Path(configured))
        else:
            if config_path:
                out.append(Path(config_path).parent / configured)
            if tree:
                out.append(Path(tree) / configured)
    if tree:
        out.append(Path(tree) / ".relkit-console-panel.json")
        out.append(Path(tree) / ADMIN_STATE_FILE)
    seen: set[str] = set()
    unique: list[Path] = []
    for path in out:
        key = str(path)
        if key not in seen:
            seen.add(key)
            unique.append(path)
    return unique


def console_read_write_paths(console_cfg: dict[str, Any]) -> list[str]:
    """Writable paths the unit needs beyond its StateDirectory: none, unless an
    operator deliberately put the panel state file somewhere else."""
    parent = console_admin_path(console_cfg).parent.as_posix()
    if parent == CONSOLE_STATE_DIR:
        return []
    return [parent]


def write_console_unit(
    *,
    user: str,
    prefix: str,
    config_path: str,
    console_cfg: dict[str, Any],
) -> None:
    template = (DEPLOY_DIR / "relkit-console.service").read_text(encoding="utf-8")
    unit = render_console_unit(
        template,
        user=user,
        prefix=prefix,
        config_path=config_path,
        extra_write_paths=console_read_write_paths(console_cfg),
    )
    dest = Path("/etc/systemd/system/relkit-console.service")
    dest.write_text(unit, encoding="utf-8")
    os.chmod(dest, 0o644)
    verify = run(["systemd-analyze", "verify", str(dest)], check=False, capture=True)
    if verify.returncode != 0:
        raise Fail(redact_text(verify.stderr or verify.stdout or "unit verify failed"))


def fix_token_perms(path: Path, user: str) -> None:
    if not path.is_file():
        return
    os.chmod(path, 0o600)
    try:
        chown_path(path, user)
    except LookupError:
        pass
    mode = stat.S_IMODE(path.stat().st_mode)
    if not token_mode_ok(mode):
        raise Fail(f"{path} mode {oct(mode)} is too open; want 0600")


# The admin-account file keeps its historical name so a box migrated from
# relkit-serve keeps its operator accounts (ADR 0016).
ADMIN_STATE_FILE = ".relkit-serve-admin.json"


def cmd_install_serve(args: argparse.Namespace) -> None:
    must_root()
    require_cmd("systemctl")
    binary = Path(args.binary)
    if not binary.is_file():
        die(f"{binary} does not exist")
    user = args.user
    prefix = args.prefix
    config_dir = Path(args.config_dir)
    directory = Path(args.dir)
    config_path = config_dir / "relkit-store.json"
    token_path = config_dir / "relkit-store.token"
    dest_bin = Path(prefix) / "relkit-store"

    if config_path.is_file() and not args.force_reconfigure:
        live = load_json_object(config_path.read_text(encoding="utf-8"))
        live_addr = str(live.get("addr") or "")
        live_dir = str(live.get("dir") or "")
        if args.addr != live_addr or str(directory) != live_dir:
            die(
                "existing instance found; refusing to apply install defaults "
                f"(live addr={live_addr} dir={live_dir}). "
                "Use `upgrade` or pass matching --addr/--dir, or --force-reconfigure."
            )

    step(f"Service account: {user}")
    ensure_user(user, str(directory))
    directory.mkdir(parents=True, exist_ok=True)
    os.chmod(directory, 0o755)
    chown_path(directory, user)

    step(f"Binary: {dest_bin}")
    install_file(binary, dest_bin, 0o755)
    run([str(dest_bin), "-version"])

    step(f"Config: {config_dir}")
    config_dir.mkdir(parents=True, exist_ok=True)
    os.chmod(config_dir, 0o750)
    chown_path(config_dir, user)

    new_token = ""
    new_bootstrap = ""
    if not config_path.is_file() or not token_path.is_file():
        init = run(
            [str(dest_bin), "init", "-dir", str(directory), "-out", str(config_dir), "-force"],
            capture=True,
        )
        new_token = extract_export("RELKIT_SERVE_TOKEN", init.stdout or "") or extract_export(
            "RELKIT_UPLOAD_TOKEN", init.stdout or ""
        ) or ""
        new_bootstrap = extract_export("RELKIT_ADMIN_BOOTSTRAP", init.stdout or "") or ""
        cfg = load_json_object(config_path.read_text(encoding="utf-8"))
        cfg["addr"] = args.addr
        cfg["dir"] = str(directory)
        cfg, _ = ensure_cas_grace(cfg)
        config_path.write_text(dump_json(cfg), encoding="utf-8")
        chown_path(config_path, user)
    elif args.rotate_token:
        print("rotating operator token; every publisher must get the new value")
        init = run(
            [str(dest_bin), "init", "-out", str(config_dir), "-token-only"],
            capture=True,
        )
        new_token = extract_export("RELKIT_SERVE_TOKEN", init.stdout or "") or extract_export(
            "RELKIT_UPLOAD_TOKEN", init.stdout or ""
        ) or ""
    else:
        print("keeping existing config and token")

    fix_token_perms(token_path, user)
    for extra in (directory / ADMIN_STATE_FILE, config_dir / "admin.json"):
        if extra.is_file():
            os.chmod(extra, 0o600)
            chown_path(extra, user)

    cfg = load_json_object(config_path.read_text(encoding="utf-8"))
    write_serve_unit(user=user, prefix=prefix, config_path=str(config_path), serve_cfg=cfg)
    run(["systemctl", "daemon-reload"])
    run(["systemctl", "enable", "relkit-store"], check=False)
    run(["systemctl", "restart", "relkit-store"])
    token = new_token or token_path.read_text(encoding="utf-8").strip()
    if not token:
        die("cannot read operator token to finish self-check")
    step("Self-check")
    self_check_serve(str(cfg.get("addr") or args.addr), token)
    step("Done")
    print(f"listening   {cfg.get('addr')}")
    print(f"serving     {cfg.get('dir')}")
    print(f"config      {config_path}")
    if new_token:
        print("\nShown once; server stores only sha256:\n")
        print(f"  export RELKIT_SERVE_TOKEN='{new_token}'")
    if new_bootstrap:
        print("\nOne-shot admin bootstrap (do not store):\n")
        print(f"  export RELKIT_ADMIN_BOOTSTRAP='{new_bootstrap}'")


def write_product_root_parent(config_path: Path, product_root: str) -> str:
    """Record where product trees live so the agent gives the same answer.

    The agent decides where a product registered without a root lands from
    productRootParent, and it refuses to guess on a box that has no products
    yet. The installer always knows the answer, because the same value just
    became the service account's home and the unit's WorkingDirectory. Leaving
    it out of the config is how an empty box ended up with one path in the unit
    and no default at all inside the agent.

    Paths are interpreted as POSIX, the target's own convention, not the
    convention of whichever host runs the installer.
    """
    posix = PurePosixPath(str(product_root))
    if not posix.is_absolute():
        raise Fail(f"--product-root must be an absolute path, got {product_root}")
    parent = str(posix).rstrip("/") or "/"
    cfg = load_json_object(config_path.read_text(encoding="utf-8"))
    existing = str(cfg.get("productRootParent") or "").strip().rstrip("/")
    if existing and existing != parent:
        raise Fail(
            f"config already sets productRootParent to {existing}, but --product-root "
            f"is {parent}; the unit's WorkingDirectory comes from the flag, so the two "
            "must agree. Re-run with the configured value, or edit the config first"
        )
    if existing != parent:
        cfg["productRootParent"] = parent
        config_path.write_text(dump_json(cfg), encoding="utf-8")
        os.chmod(config_path, 0o644)
        print(f"productRootParent {parent} written to {config_path}")
    return parent


def admin_document_products(doc: Any) -> dict[str, str]:
    """Return ``{id: root}`` for every product registered in an admin document.

    The document is JSON off disk, so nothing about its shape is guaranteed:
    entries may be missing an id or a root, and a truncated file yields an empty
    mapping rather than an exception.
    """
    roots: dict[str, str] = {}
    if not isinstance(doc, dict):
        return roots
    for entry in doc.get("products") or []:
        if isinstance(entry, dict) and entry.get("id") and entry.get("root"):
            roots[str(entry["id"])] = str(entry["root"])
    return roots

def registered_products_for_backfill(
    *,
    config_products: dict[str, str],
    existing_admin: dict[str, Any],
    merged_admin: dict[str, Any],
) -> dict[str, str]:
    """Every product root this box knows, from all three places it can live.

    All three count. The config map holds products on a box that has not
    finished the epoch split; merged_admin holds the ones this run is about to
    write; existing_admin holds the ones already on disk. Skipping the last is
    how an epoch-finished box reported no products at all and let the backfill
    fall back to the unit's WorkingDirectory instead of the paths the operator
    actually registered.
    """
    roots = dict(config_products)
    roots.update(admin_document_products(existing_admin))
    roots.update(admin_document_products(merged_admin))
    return roots

def _common_parent(roots: Sequence[str]) -> str:
    """Return the one directory every root sits under, or "" when they differ.

    Mirrors the agent's own rule: the registered products vote, and a machine
    whose products are scattered has no convention worth writing down.
    """
    parent = ""
    for root in roots:
        text = str(root or "").strip()
        if not text:
            continue
        directory = str(PurePosixPath(text).parent)
        if not parent:
            parent = directory
            continue
        if directory != parent:
            return ""
    return parent


def backfill_product_root_parent(
    cfg: dict[str, Any],
    *,
    working_directory: str,
    product_roots: dict[str, str],
) -> tuple[dict[str, Any], str]:
    """Give an upgraded agent the productRootParent that install agent writes.

    Boxes installed before the flag was mandatory carry no productRootParent, so
    they work only while the agent can infer one from their products. That
    inference disappears the moment a box has no products, which is the empty-box
    case the mandatory flag exists to remove.

    The registered products are the better evidence of the two. Each root is the
    path an operator typed for that product, so the set of them describes where
    release trees actually sit. The unit's WorkingDirectory only reports the
    argument some installer was handed, possibly long ago and possibly for a
    directory that has since moved: the live box this was checked against runs
    in /data/relkit while a /srv/relkit default was still sitting in the repo,
    so a stale value of that shape would be copied forward as if it were fact.
    Preferring the products keeps the config describing the disk.

    WorkingDirectory still decides when it is the only thing to go on, and a
    disagreement is reported rather than hidden, because the two values mean
    different things and an operator should see that they diverged.
    """
    if str(cfg.get("productRootParent") or "").strip():
        return cfg, ""
    inferred_parent = _common_parent(list(product_roots.values()))
    unit_parent = ""
    if working_directory.strip():
        unit_parent = str(PurePosixPath(working_directory.strip())).rstrip("/")
    chosen = inferred_parent or unit_parent
    if not chosen:
        return cfg, (
            "left productRootParent unset: the registered products share no parent "
            "and the unit names no WorkingDirectory, so there is nothing to infer from"
        )
    updated = dict(cfg)
    updated["productRootParent"] = chosen
    source = "the registered products" if inferred_parent else "the unit's WorkingDirectory"
    note = f"productRootParent {chosen} backfilled from {source}"
    if inferred_parent and unit_parent and unit_parent != inferred_parent:
        note += (
            f" (the unit still runs in {unit_parent}; the products are not there, "
            "so the unit's value was not used)"
        )
    return updated, note


def cmd_install_agent(args: argparse.Namespace) -> None:
    must_root()
    require_cmd("systemctl")
    binary = Path(args.binary)
    if not binary.is_file():
        die(f"{binary} does not exist")
    user = args.user
    config_dir = Path(args.config_dir)
    state_dir = Path(args.state_dir)
    product_root = Path(args.product_root)
    posix_root = PurePosixPath(str(args.product_root))
    if not posix_root.is_absolute():
        die(f"--product-root must be an absolute path, got {args.product_root}")
    config_path = config_dir / "relkit-agent.json"
    dest_bin = Path(args.prefix) / "relkit-agent"

    step(f"Service account: {user}")
    ensure_user(user, str(product_root))
    for path, mode in ((config_dir, 0o755), (state_dir, 0o755), (product_root, 0o755)):
        path.mkdir(parents=True, exist_ok=True)
        os.chmod(path, mode)
    leftover = config_dir / "token"
    if leftover.is_file():
        print(f"WARNING: {leftover} is leftover instance-wide credential; delete after per-product tokens")
    if not config_path.is_file():
        example = Path(args.config_example) if args.config_example else DEPLOY_DIR / "relkit-agent.example.json"
        if not example.is_file():
            die(f"{example} does not exist")
        shutil.copy2(example, config_path)
        cfg = load_json_object(config_path.read_text(encoding="utf-8"))
        cfg["stateDir"] = str(state_dir)
        config_path.write_text(dump_json(cfg), encoding="utf-8")
        os.chmod(config_path, 0o644)
    # The agent answers "where does a product registered without a root land"
    # from productRootParent, and it has no hardcoded fallback on purpose. The
    # installer already knows the answer, because the same value just became
    # the service account's home and the unit's WorkingDirectory, so it writes
    # it down instead of leaving the flag and the config disagreeing.
    write_product_root_parent(config_path, str(args.product_root))
    install_file(binary, dest_bin, 0o755)
    run([str(dest_bin), "-version"])
    tokens = config_dir / "tokens"
    tokens.mkdir(parents=True, exist_ok=True)
    os.chmod(tokens, 0o750)
    write_agent_unit(
        user=user,
        prefix=args.prefix,
        config_path=str(config_path),
        working_directory=str(product_root),
    )
    run(["chown", "-R", f"{user}:{user}", str(state_dir), str(product_root)])
    run(["systemctl", "daemon-reload"])
    run(["systemctl", "enable", "--now", "relkit-agent"], check=False)
    cfg = load_json_object(config_path.read_text(encoding="utf-8"))
    self_check_agent(str(cfg.get("addr") or "127.0.0.1:8787"))
    print(f"config {config_path}")
    print("next: EnvironmentFile=/etc/relkit-agent/env for RELKIT_SERVE_TOKEN / COS keys")
    print("new product: vNext relkit-ops agent policy --config <cfg> --policy <relkit.agent-product-policy/1>")


def local_service_url(unit: str) -> str:
    """Loopback base URL of a service on this box, read from its live config.
    Empty when the unit or its config is absent."""
    show = systemd_show(unit, "ExecStart")
    cfg_path = parse_exec_config(show.get("ExecStart") or "")
    if not cfg_path or not Path(cfg_path).is_file():
        return ""
    addr = str(load_json_object(Path(cfg_path).read_text(encoding="utf-8")).get("addr") or "")
    return loopback_base(addr) if addr else ""


def console_tree_spec(example: Path) -> str:
    """The "tree" entry of a console config example, serialized unchanged.

    The entry is the panel's tree adapter: a type plus that type's own
    parameters. Only relkit-console interprets it; this returns it as an
    opaque JSON block.
    """
    if not example.is_file():
        die(f"{example} does not exist; a console config needs a \"tree\" adapter entry")
    doc = load_json_object(example.read_text(encoding="utf-8"))
    tree = doc.get("tree")
    if not isinstance(tree, dict) or not tree:
        die(f"{example} has no \"tree\" adapter entry; the panel cannot read the tree without one")
    return json.dumps(tree, separators=(",", ":"))


def cmd_install_console(args: argparse.Namespace) -> None:
    must_root()
    require_cmd("systemctl")
    binary = Path(args.binary)
    if not binary.is_file():
        die(f"{binary} does not exist")
    user = args.user
    prefix = args.prefix
    config_dir = Path(args.config_dir)
    config_path = config_dir / "relkit-console.json"
    dest_bin = Path(prefix) / "relkit-console"

    # The panel reads the tree through its tree adapter. Which adapter, and
    # with what parameters, is the "tree" entry of the config example; it is
    # carried over whole, so this script never learns what is behind it.
    example = Path(args.config_example) if args.config_example else DEPLOY_DIR / CONSOLE_CONFIG_EXAMPLE
    tree_spec = console_tree_spec(example)
    agent_url = args.agent or local_service_url("relkit-agent")

    step(f"Service account: {user}")
    ensure_user(user, CONSOLE_STATE_DIR)

    step(f"Binary: {dest_bin}")
    install_file(binary, dest_bin, 0o755)
    run([str(dest_bin), "-version"])

    step(f"Config: {config_dir}")
    config_dir.mkdir(parents=True, exist_ok=True)
    os.chmod(config_dir, 0o750)
    chown_path(config_dir, user)

    if not config_path.is_file():
        init_args = [str(dest_bin), "init", "-tree", tree_spec, "-out", str(config_dir), "-force"]
        if agent_url:
            init_args += ["-agent", agent_url]
        run(init_args, capture=True)
        cfg_doc = load_json_object(config_path.read_text(encoding="utf-8"))
        cfg_doc["addr"] = args.addr
        config_path.write_text(dump_json(cfg_doc), encoding="utf-8")
    else:
        print("keeping existing config")
        cfg_doc = load_json_object(config_path.read_text(encoding="utf-8"))
    chown_path(config_path, user)
    panel_file = Path(str(console_admin_path(cfg_doc)))
    if panel_file.is_file():
        os.chmod(panel_file, 0o600)
        chown_path(panel_file, user)
        chown_path(panel_file.parent, user)

    step("Unit")
    write_console_unit(user=user, prefix=prefix, config_path=str(config_path), console_cfg=cfg_doc)
    run(["systemctl", "daemon-reload"])
    run(["systemctl", "enable", "relkit-console"], check=False)
    run(["systemctl", "restart", "relkit-console"])

    step("Self-check")
    addr = str(cfg_doc.get("addr") or args.addr)
    wait_health(loopback_base(addr), "relkit-console")
    print(f"panel on {addr}")
    print(f"config {config_path}")
    print("first operator: relkit-agent init -bootstrap, then open /-/admin")
    print("break-glass: relkit-console init -reset-admin -out " + str(config_dir))


def migrate_serve_to_store(
    *,
    user: str,
    prefix: str,
    backup_root: Path,
) -> list[str]:
    """Retire a live relkit-serve.service box into relkit-store (ADR 0016 step 4).

    Moves the config to /etc/relkit-store/relkit-store.json (same JSON body,
    new canonical name), writes the store unit with ReadWritePaths from the
    live config, and swaps the units under one daemon-reload. The release
    tree, token files, and data files (.relkit-serve-admin.json etc.) stay
    exactly where they are; a rollback restores the old unit and config path.
    """
    show = systemd_show("relkit-serve", "ExecStart", "User", "FragmentPath", "ActiveState")
    if not show.get("FragmentPath"):
        raise Fail("no relkit-serve.service on this box; nothing to migrate")
    old_config = parse_exec_config(show.get("ExecStart") or "")
    if not old_config:
        raise Fail("cannot find relkit-serve -config from systemd")
    old_config_file = Path(old_config)
    if not old_config_file.is_file():
        raise Fail(f"serve config {old_config_file} is missing")
    dest_bin = Path(prefix) / "relkit-store"

    notes: list[str] = []
    backup_files(
        backup_root / "serve",
        [dest_bin, old_config_file, Path("/etc/systemd/system/relkit-serve.service")],
    )

    # New canonical layout: /etc/relkit-store/relkit-store.json. Same JSON
    # body; ADR 0016 keeps protocol/data-file names, the config file moves.
    config_dir = Path("/etc/relkit-store")
    config_dir.mkdir(parents=True, exist_ok=True)
    new_config = config_dir / "relkit-store.json"
    cfg = load_json_object(old_config_file.read_text(encoding="utf-8"))
    if new_config.is_file() and new_config != old_config_file:
        raise Fail(f"{new_config} already exists; refusing to clobber a live store config")
    new_config.write_text(dump_json(cfg), encoding="utf-8")
    os.chmod(new_config, 0o640)
    chown_path(new_config, user)
    notes.append(f"config {old_config} -> {new_config}")

    # Token files sit next to the old config; carry them over so the new
    # unit's config-relative lookups keep working. uploadTokens[].file is
    # resolved relative to the config directory, so the tokens/ subdir moves
    # with them - missing it leaves the store crash-looping on boot.
    for name in ("relkit-serve.token", "relkit-store.token"):
        token_src = old_config_file.parent / name
        if token_src.is_file():
            token_dest = config_dir / name
            if token_dest.is_file():
                continue
            shutil.copy2(token_src, token_dest)
            fix_token_perms(token_dest, user)
            notes.append(f"token {name} -> {config_dir}")
    tokens_src = old_config_file.parent / "tokens"
    if tokens_src.is_dir():
        tokens_dest = config_dir / "tokens"
        if not tokens_dest.exists():
            # copy2 keeps each file's original mode and owner-agnostic perms;
            # some boxes hand out tokens root:<group> 0640 (group-readable),
            # and flattening them to 0600 owner-relkit breaks the store's
            # boot-time read. Preserve what the old config dir expressed.
            shutil.copytree(tokens_src, tokens_dest, copy_function=shutil.copy2)
            # The dir itself must be traversable by the service user.
            os.chmod(tokens_dest, 0o750)
            for path in tokens_dest.rglob("*"):
                if path.is_dir():
                    os.chmod(path, 0o750)
                elif not (path.stat().st_mode & 0o040):
                    # file not group/other readable: ensure the service user
                    # can read it if it ended up owned by someone else.
                    try:
                        chown_path(path, user)
                        os.chmod(path, 0o640)
                    except (LookupError, OSError):
                        pass
            notes.append(f"tokens/ -> {tokens_dest}")

    write_serve_unit(user=user, prefix=prefix, config_path=str(new_config), serve_cfg=cfg)

    run(["systemctl", "stop", "relkit-serve"])
    run(["systemctl", "disable", "relkit-serve"], check=False)
    run(["systemctl", "daemon-reload"])
    run(["systemctl", "enable", "relkit-store"], check=False)
    run(["systemctl", "restart", "relkit-store"])
    notes.append("relkit-serve.service stopped and disabled; relkit-store.service live")

    addr = str(cfg.get("addr") or "")
    token_file = new_config.parent / "relkit-store.token"
    if token_file.is_file():
        token = token_file.read_text(encoding="utf-8").strip()
        self_check_serve(addr, token)
        notes.append("store self-checked")
    else:
        wait_health(loopback_base(addr), "relkit-store")
        notes.append("store healthy (no token file for full self-check)")
    return notes


def systemd_show(unit: str, *props: str) -> dict[str, str]:
    argv = ["systemctl", "show", unit, "--no-pager"]
    for prop in props:
        argv.append(f"-p{prop}")
    out = run(argv, capture=True)
    data: dict[str, str] = {}
    for line in (out.stdout or "").splitlines():
        if "=" in line:
            key, _, value = line.partition("=")
            data[key] = value
    return data


# Directories that hold regenerable build state rather than configuration.
# Copying them into every snapshot made each upgrade backup ~250MB instead of
# ~12MB: the agent's products/ directory contains .relkit/cache/staged, which
# stores the payload artifacts of the release currently being published.
BACKUP_SKIP_DIRS = (".relkit/cache",)


def backup_ignore(root: Path):
    """Skip regenerable cache subtrees while copying root.

    The pattern is matched against the path relative to root: a bare name
    filter would also drop unrelated directories that happen to be called
    cache. A directory whose entire content sits in a skipped subtree (such as
    .relkit, whose only child is cache/) is skipped as well, so the snapshot
    does not carry empty scaffolding that suggests something was backed up.
    """

    def ignore(directory: str, names: list[str]) -> set[str]:
        skipped: set[str] = set()
        for name in names:
            relative = _backup_relative(Path(directory) / name, root)
            if relative is None:
                continue
            if _backup_skipped(relative) or _backup_empty_after_skip(Path(directory) / name, root):
                skipped.add(name)
        return skipped

    return ignore


def _backup_relative(path: Path, root: Path) -> Optional[str]:
    try:
        return path.resolve().relative_to(root.resolve()).as_posix()
    except ValueError:
        return None


def _backup_skipped(relative: str) -> bool:
    return any(
        relative == skip or relative.startswith(skip + "/")
        for skip in BACKUP_SKIP_DIRS
    )


def _backup_empty_after_skip(directory: Path, root: Path) -> bool:
    """True when directory holds nothing but skipped subtrees."""
    if not directory.is_dir():
        return False
    for child in directory.iterdir():
        relative = _backup_relative(child, root)
        if relative is None or _backup_skipped(relative):
            continue
        if child.is_dir():
            if not _backup_empty_after_skip(child, root):
                return False
        else:
            return False
    return True


def backup_files(backup_root: Path, files: Sequence[Path]) -> None:
    backup_root.mkdir(parents=True, exist_ok=True)
    os.chmod(backup_root, 0o700)
    for path in files:
        if not path.exists():
            continue
        dest = backup_root / path.name
        if path.is_dir():
            shutil.copytree(path, dest, dirs_exist_ok=True, ignore=backup_ignore(path))
        else:
            shutil.copy2(path, dest)


SNAPSHOT_NAME = re.compile(r"^\d{8}T\d{6}Z$")


def snapshot_dirs(backup_root: Path) -> list[Path]:
    """Upgrade snapshots only.

    Names are the UTC stamp the CLI writes. Anything else in the directory (a
    hand-made tarball, an ad-hoc copy) is deliberately not a rotation
    candidate: deleting it would destroy data nobody asked us to manage.
    """
    if not backup_root.is_dir():
        return []
    return sorted(
        (path for path in backup_root.iterdir() if path.is_dir() and SNAPSHOT_NAME.match(path.name)),
        key=lambda path: path.name,
    )


def prune_backups(
    backup_root: Path,
    keep: int,
    current: Optional[Path] = None,
    dry_run: bool = False,
) -> list[str]:
    """Delete the oldest snapshots until `keep` remain. Never removes `current`.

    Rotation must never delete the snapshot a failed apply would roll back
    from, so `current` is exempt from the count: the retained set is `current`
    plus the newest snapshots that fill the remaining budget. With dry_run the
    same selection is reported but nothing is deleted, so a preview can never
    disagree with the real run.
    """
    if keep < 1:
        raise Fail(f"keep must be at least 1, got {keep}")
    snapshots = snapshot_dirs(backup_root)
    protected = {current.resolve()} if current is not None else set()
    exempt = [path for path in snapshots if path.resolve() in protected]
    candidates = [path for path in snapshots if path.resolve() not in protected]
    budget = max(0, keep - len(exempt))
    surplus = len(candidates) - budget
    if surplus <= 0:
        return []
    doomed = candidates[:surplus]
    if not dry_run:
        for path in doomed:
            shutil.rmtree(path)
    return [path.name for path in doomed]


def probe_local() -> dict[str, Any]:
    probe: dict[str, Any] = {"hostname": os.uname().nodename if hasattr(os, "uname") else ""}
    for unit, kind in (("relkit-store", "serve"), ("relkit-agent", "agent"), ("relkit-console", "console")):
        show = systemd_show(unit, "FragmentPath", "User", "Group", "ExecStart", "ReadWritePaths", "StateDirectory", "ProtectSystem", "ActiveState")
        entry: dict[str, Any] = {"unit": show}
        exec_start = show.get("ExecStart") or ""
        config_path = parse_exec_config(exec_start)
        entry["configPath"] = config_path
        if config_path and Path(config_path).is_file():
            cfg = load_json_object(Path(config_path).read_text(encoding="utf-8"))
            entry["config"] = redact_value(cfg)
            entry["configRawOk"] = True
        bin_name = {"serve": "relkit-store", "agent": "relkit-agent", "console": "relkit-console"}[kind]
        bin_path = parse_exec_binary(exec_start) or shutil.which(bin_name) or f"/usr/local/bin/{bin_name}"
        entry["binary"] = bin_path
        present = Path(bin_path).is_file()
        entry["present"] = present
        if present:
            ver = run([bin_path, "-version"], check=False, capture=True)
            entry["version"] = (ver.stdout or ver.stderr or "").strip()
        else:
            entry["version"] = ""
        if kind == "console":
            # Where the running panel actually opens its cookie-key file, and
            # whether the unit may write there. A mismatch is the serve-era
            # relative adminStateFile, and it only shows up as a crash loop.
            cfg = entry.get("config") if isinstance(entry.get("config"), dict) else {}
            panel_file = console_admin_path(cfg)
            entry["panelStateFile"] = str(panel_file)
            entry["panelStateFilePresent"] = Path(str(panel_file)).is_file()
            unit_props = entry.get("unit") or {}
            writable = str(unit_props.get("ReadWritePaths") or "").split()
            writable += [
                "/var/lib/" + name.strip("/")
                for name in str(unit_props.get("StateDirectory") or "").split()
            ]
            entry["panelStateWritable"] = any(
                str(panel_file).startswith(path.rstrip("/") + "/") or str(panel_file) == path
                for path in writable
            )
            entry["retiredConfigKeys"] = console_retired_keys(cfg)
        probe[kind] = entry
    agent_cfg_path = probe.get("agent", {}).get("configPath")
    products_dir = Path("/etc/relkit-agent/products")
    if agent_cfg_path:
        products_dir = Path(agent_cfg_path).parent / "products"
    profiles: dict[str, Any] = {}
    if products_dir.is_dir():
        for path in sorted(products_dir.glob("*.json")):
            try:
                profiles[path.name] = redact_value(load_json_object(path.read_text(encoding="utf-8")))
            except (OSError, ValueError) as exc:
                profiles[path.name] = {"error": str(exc)}
    probe["profiles"] = profiles
    probe["python"] = sys.version.split()[0]
    return probe


def unit_state(unit: str) -> dict[str, str]:
    return systemd_show(unit, "ActiveState", "SubState", "MainPID", "NRestarts", "ExecMainStatus")


def require_unit_running(unit: str) -> None:
    """Prove the fresh process is the one serving, not a leftover impostor.

    A restart can lose a race for the port to an older process: systemd then
    reports "activating (auto-restart)" while the old binary happily answers on
    the socket. Curling the port alone therefore proves nothing - it returns 200
    from whatever is listening, which is exactly how a crash loop went unnoticed.
    This checks the unit reached a steady active state with a live main PID.
    """
    state: dict[str, str] = {}
    for _ in range(15):
        state = unit_state(unit)
        if state.get("ActiveState") == "active" and state.get("MainPID", "0") not in ("", "0"):
            return
        time.sleep(1)
    try:
        log = run(["journalctl", "-u", unit, "-n", "40", "--no-pager"], check=False, capture=True)
        print(redact_text(log.stdout or ""), file=sys.stderr)
    except (Fail, OSError):
        # Diagnostics must never replace the verdict: on a host without systemd
        # the journal call fails for its own reasons, and the reason the unit is
        # not running is what the operator needs to read.
        pass
    raise Fail(
        f"{unit} is {state.get('ActiveState') or 'unknown'}"
        f"/{state.get('SubState') or 'unknown'} after restart "
        f"(main pid {state.get('MainPID') or '0'}, {state.get('NRestarts') or '0'} restarts); "
        "refusing to report a healthy upgrade"
    )


def listener_pid(port: str) -> str:
    """PID listening on a TCP port, via ss. Empty when nothing is listening."""
    out = run(["ss", "-lntpH"], check=False, capture=True)
    for line in (out.stdout or "").splitlines():
        fields = line.split()
        if len(fields) < 4:
            continue
        local = fields[3]
        if not local.endswith(":" + port):
            continue
        found = re.search(r"pid=(\d+)", line)
        if found:
            return found.group(1)
    return ""


def require_listener_belongs_to(unit: str, addr: str) -> None:
    """The socket must be held by the unit's own main process.

    This is the check that would have caught the silent failure: relkit-store
    crash-looped on "address already in use" while a retired relkit-serve
    process answered every probe. Everything looked green because the probes
    are answered - just by the wrong program.

    Binding happens after the process reaches "active", so the listener is
    polled rather than sampled once: an instant read would call a healthy start
    a failure and roll a good upgrade back.
    """
    port = str(addr or "").rsplit(":", 1)[-1]
    if not port.isdigit():
        return
    pid = ""
    for _ in range(20):
        pid = listener_pid(port)
        if pid:
            break
        time.sleep(0.5)
    if not pid:
        raise Fail(f"nothing is listening on :{port} after restarting {unit}")
    main_pid = unit_state(unit).get("MainPID") or ""
    if main_pid and main_pid not in ("0", pid):
        cmd = run(["ps", "-o", "cmd=", "-p", pid], check=False, capture=True)
        raise Fail(
            f":{port} is held by pid {pid} ({(cmd.stdout or '').strip() or 'unknown command'}), "
            f"but {unit} runs as pid {main_pid}; the unit is not the process serving traffic"
        )


def retire_legacy_serve() -> list[str]:
    """Stop the pre-ADR-0016 relkit-serve unit for good.

    On a migrated box relkit-serve is disabled but its unit file is still
    installed with Restart=always. systemd had restarted it over a million
    times trying to bind a port relkit-store owned; each restart was a chance to
    win the race and hijack :8080 during an upgrade. Leaving it installed keeps
    that race alive: a unit that cannot start is not a fallback.

    Masking is what makes the retirement durable, but mask cannot replace an
    existing unit file - it only creates a /dev/null symlink where no file is.
    The file is therefore moved into the upgrade backup first, which also keeps
    rollback possible.
    """
    unit = Path("/etc/systemd/system/relkit-serve.service")
    if not unit.is_file():
        return []
    notes = ["found retired relkit-serve.service still installed; stopping, disabling and masking it"]
    run(["systemctl", "stop", "relkit-serve"], check=False)
    run(["systemctl", "disable", "relkit-serve"], check=False)
    masked = run(["systemctl", "mask", "relkit-serve"], check=False, capture=True)
    if masked.returncode != 0:
        # mask refused because the unit file is a real file. Retire it by name
        # instead: systemd then has nothing to start, which is the same outcome.
        retired = unit.with_suffix(".service.retired")
        try:
            unit.rename(retired)
            notes.append(f"moved {unit} to {retired.name}; systemd can no longer start it")
        except OSError as exc:
            notes.append(
                "could not retire relkit-serve: "
                + redact_text(str(exc))
                + " (the unit is stopped and disabled, but it is still installed)"
            )
        run(["systemctl", "daemon-reload"], check=False)
        return notes
    notes.append("relkit-serve stopped, disabled and masked so it can never take the port again")
    return notes


def restart_unit(unit: str) -> None:
    """Restart a unit, surviving a crash loop left over from an earlier run.

    systemd refuses to start a unit that already failed too many times
    ("start-limit-hit"), so an upgrade could not even try to bring the service
    back: the failure it reports is the *previous* run's, not this one's. The
    counter is reset first, which is safe because this is a deliberate start.
    """
    run(["systemctl", "reset-failed", unit], check=False)
    started = run(["systemctl", "restart", unit], check=False, capture=True)
    if started.returncode == 0:
        return
    detail = redact_text((started.stderr or started.stdout or "").strip())
    state = unit_state(unit)
    if state.get("SubState") == "auto-restart" or "start-limit" in detail:
        log = run(["journalctl", "-u", unit, "-n", "40", "--no-pager"], check=False, capture=True)
        print(redact_text(log.stdout or ""), file=sys.stderr)
    raise Fail(f"systemctl restart {unit} failed ({started.returncode}): {detail}")


def apply_serve_upgrade(
    *,
    binary: Optional[Path],
    user: str,
    prefix: str,
    listen_addr: Optional[str],
    public_upload_url: Optional[str],
    restart: bool,
    backup_root: Path,
) -> list[str]:
    notes: list[str] = []
    show = systemd_show("relkit-store", "ExecStart", "User", "FragmentPath")
    config_path = parse_exec_config(show.get("ExecStart") or "")
    if not config_path:
        raise Fail("cannot find relkit-store -config from systemd")
    config_file = Path(config_path)
    token_file = config_file.parent / "relkit-store.token"
    legacy_token_file = config_file.parent / "relkit-serve.token"
    if not token_file.is_file() and legacy_token_file.is_file():
        token_file = legacy_token_file
        notes.append("using legacy token file relkit-serve.token (migrate-serve box)")
    dest_bin = Path(prefix) / "relkit-store"
    legacy_unit = Path("/etc/systemd/system/relkit-serve.service")
    backup_files(
        backup_root / "serve",
        [dest_bin, config_file, Path("/etc/systemd/system/relkit-store.service"), legacy_unit],
    )
    # Retire the old unit only once its file is safely in the backup: rollback
    # has to be able to put it back.
    notes.extend(retire_legacy_serve())
    cfg = load_json_object(config_file.read_text(encoding="utf-8"))
    live_addr = cfg.get("addr")
    live_dir = cfg.get("dir")
    cfg, grace_notes = ensure_cas_grace(cfg)
    notes.extend(grace_notes)
    if listen_addr and listen_addr != live_addr:
        cfg["addr"] = listen_addr
        notes.append(f"changed serve addr {live_addr} -> {listen_addr}")
    if cfg.get("dir") != live_dir:
        raise Fail("upgrade refused to change dir")
    config_file.write_text(dump_json(cfg), encoding="utf-8")
    if binary:
        install_file(binary, dest_bin, 0o755)
        notes.append(f"installed {dest_bin}")
    write_serve_unit(user=user, prefix=prefix, config_path=str(config_file), serve_cfg=cfg)
    notes.append(f"ReadWritePaths={' '.join(read_write_paths(cfg))}")
    fix_token_perms(token_file, user)
    run(["systemctl", "daemon-reload"])
    if restart:
        restart_unit("relkit-store")
        require_unit_running("relkit-store")
        require_listener_belongs_to("relkit-store", str(cfg.get("addr") or ""))
        token = token_file.read_text(encoding="utf-8").strip()
        self_check_serve(
            str(cfg.get("addr") or ""),
            token,
            public_upload_url=public_upload_url,
        )
        notes.append("serve restarted and self-checked")
    else:
        notes.append("serve files updated; restart skipped")
    return notes


def tool_publish_token_path(config_dir: Path) -> Path:
    """Where the deploy pipeline's tool-publish credential lives.

    It sits beside the agent config, in the same directory that already holds
    the custody manifest, so the file is protected by the same ownership rules
    and the config can name it by an absolute path.

    The result is written into a Linux config file, so it is built with POSIX
    separators rather than the host's: deploying from Windows must not produce
    a config the server cannot read.
    """
    return config_dir / "tools-publish.token"


def ensure_tool_publish_token(config_dir: Path, *, user: str, rotate: bool = False) -> tuple[Path, str, bool]:
    """Make the agent able to accept tool publishing, returning (path, token, minted).

    Tool publishing is what requirement 12.6 needs: the deploy pipeline uploads
    the version's tool files and the server signs them with its root. The
    credential is minted here rather than shipped in, so no plaintext deploy
    token travels through a spec file or a command line. An existing token is
    reused unless rotation is asked for, which keeps a working release pipeline
    working across upgrades.

    The plaintext is returned because the same process has to publish tools with
    it; it is deliberately never written into notes or printed.
    """
    path = tool_publish_token_path(config_dir)
    if path.is_file() and not rotate:
        existing = path.read_text(encoding="utf-8").strip()
        if existing:
            return path, existing, False
    token = secrets.token_urlsafe(32)
    config_dir.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(path.name + ".tmp~")
    tmp.write_text(token + "\n", encoding="utf-8")
    os.chmod(tmp, 0o600)
    os.replace(tmp, path)
    try:
        chown_path(path, user)
        chown_path(config_dir, user)
    except LookupError:
        pass
    return path, token, True


def publish_release_tools(
    *,
    files: list[dict[str, Any]],
    version: str,
    commit: str,
    user: str,
    restart: bool,
) -> list[str]:
    """Publish this release's tool build to the local agent.

    Runs on the server, because both the credential and a reachable agent are
    local. The signed manifest records absolute download URLs; the agent takes
    them from its own publicURL (the address hosts already hold in
    relkit.project.json) and refuses to sign a loopback origin, so this side
    does not pass an address at all.

    Tool publishing is what makes /v1/catalog answer: the endpoint serves a
    document that only exists once something has been published, so an agent
    with no tool release returns 404 and hosts have nothing to bootstrap from.
    """
    if not files:
        return []
    if not version:
        raise Fail("cannot publish tools without a release version")
    helper = DEPLOY_DIR / "relkit_tools_publish.py"
    if not helper.is_file():
        raise Fail(f"missing {helper.name}; the release cannot publish its tools")
    staged = Path("/tmp/relkit-tools-publish")
    if staged.exists():
        shutil.rmtree(staged, ignore_errors=True)
    staged.mkdir(parents=True, exist_ok=True)
    for row in files:
        source = Path(str(row.get("path") or ""))
        if not source.is_file():
            raise Fail(f"tool artifact {source} is missing; refusing to publish a partial tool set")
        shutil.copy2(source, staged / source.name)
    payload = {
        "version": version,
        "commit": commit,
        "files": [
            {
                "name": Path(str(row["path"])).name,
                "component": row.get("component") or "",
                # A tree component ships one artifact for every platform, so it
                # is published under the "all" alias hosts ask for by name
                # rather than under the any-any string its os/arch would imply.
                "platform": tool_platform(row.get("os") or "", row.get("arch") or ""),
                "os": row.get("os") or "",
                "arch": row.get("arch") or "",
            }
            for row in files
        ],
    }
    spec_file = staged / "spec.json"
    spec_file.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")
    show = systemd_show("relkit-agent", "ExecStart")
    cfg_path = parse_exec_config(show.get("ExecStart") or "")
    addr = ""
    token = ""
    if cfg_path and Path(cfg_path).is_file():
        cfg = load_json_object(Path(cfg_path).read_text(encoding="utf-8"))
        addr = str(cfg.get("addr") or "")
        token_file = str(cfg.get("toolsTokenFile") or "")
        if token_file and Path(token_file).is_file():
            token = Path(token_file).read_text(encoding="utf-8").strip()
    if not addr:
        raise Fail("the agent has no listen address; cannot publish tools")
    if not token:
        raise Fail(
            "the agent accepts no tool publishing (toolsTokenFile unset or empty); "
            "enable tool publishing before publishing a release that ships tools"
        )
    base_root = loopback_base(addr)
    result = run(
        [
            sys.executable,
            str(helper),
            "--server",
            base_root,
            "--spec",
            str(spec_file),
        ],
        capture=True,
        env={"RELKIT_TOOLS_TOKEN": token},
    )
    output = (result.stdout or "").strip()
    if restart:
        # Restarting is not needed for the catalog to appear (it is written
        # before the response), but a clean restart proves the served document
        # survives a process boundary, which is what hosts will hit.
        restart_unit("relkit-agent")
        require_unit_running("relkit-agent")
    return [f"published tools {version}: {output.splitlines()[-1] if output else '(no output)'}"]


def apply_agent_upgrade(
    *,
    binary: Optional[Path],
    user: str,
    prefix: str,
    public_upload_url: Optional[str],
    serve_addr: str,
    restart: bool,
    backup_root: Path,
    root_key_id: str = "",
    custody_manifest: str = "",
    admin_state_file: str = "",
    enable_tool_publish: bool = False,
) -> list[str]:
    notes: list[str] = []
    show = systemd_show("relkit-agent", "ExecStart", "User", "FragmentPath")
    config_path = parse_exec_config(show.get("ExecStart") or "") or "/etc/relkit-agent/relkit-agent.json"
    config_file = Path(config_path)
    dest_bin = Path(prefix) / "relkit-agent"
    products_dir = config_file.parent / "products"
    config_dir = config_file.parent
    cfg: dict[str, Any] = {}
    if config_file.is_file():
        cfg = load_json_object(config_file.read_text(encoding="utf-8"))
    # The live unit names the directory install agent was handed. Read it before
    # touching the config, because it is the best evidence for backfilling a
    # productRootParent that older installs never wrote.
    unit_path = Path(show.get("FragmentPath") or "/etc/systemd/system/relkit-agent.service")
    live_working_dir = ""
    if unit_path.is_file():
        for line in unit_path.read_text(encoding="utf-8").splitlines():
            if line.startswith("WorkingDirectory="):
                live_working_dir = line.split("=", 1)[1].strip()
                break
    # The custody manifest belongs beside the config: it describes this
    # machine's identity, not any one product's data.
    manifest_file = config_dir / (Path(custody_manifest).name if custody_manifest else "custody-manifest.json")
    # The admin document must be found where the agent will look for it. An
    # explicit path wins; otherwise it is <stateDir>/admin.json, exactly like
    # openAdminStore derives it, so a migration cannot write a document the
    # server never reads.
    if admin_state_file:
        admin_file = Path(admin_state_file)
    else:
        state_dir = str(cfg.get("stateDir") or "").strip()
        if state_dir and not state_dir.startswith("/"):
            state_dir = str(config_dir / state_dir)
        if not state_dir:
            state_dir = str(config_dir / "relkit-agent-state")
        admin_file = Path(state_dir) / "admin.json"
    backup_files(
        backup_root / "agent",
        [
            dest_bin,
            config_file,
            Path("/etc/systemd/system/relkit-agent.service"),
            products_dir,
            manifest_file,
            admin_file,
        ],
    )

    # A token's plaintext is only ever in the file it was issued into. Hash the
    # trimmed content so the pre-epoch credential keeps working after the move;
    # nothing is reissued, because reissuing would break every publisher's CI.
    token_hashes: dict[str, str] = {}
    for entry in cfg.get("uploadTokens") or []:
        if not isinstance(entry, dict):
            continue
        carrier = str(entry.get("file") or "").strip()
        if not carrier:
            continue
        path = Path(carrier)
        if not path.is_absolute():
            path = config_dir / path
        if path.is_file():
            token_hashes[carrier] = hashlib.sha256(
                path.read_text(encoding="utf-8").strip().encode("utf-8")
            ).hexdigest()
        # A legacy uploadToken (instance-wide, not per product) has no product to
        # authorize, so it is deliberately not carried over: requirement 11.1
        # says a token names exactly one product.

    product_roots: dict[str, str] = {}
    for entry_id, entry in (cfg.get("products") or {}).items():
        if isinstance(entry, dict) and entry.get("root"):
            product_roots[entry_id] = str(entry["root"])

    effective_key_id = root_key_id
    if not effective_key_id:
        effective_key_id = str(cfg.get("rootKeyId") or "").strip()
    cfg, admin_doc, split_notes = epoch_split_agent_config(
        cfg,
        product_roots=product_roots,
        token_hashes=token_hashes,
        root_key_id=effective_key_id,
        custody_manifest=str(manifest_file),
    )
    notes.extend(split_notes)
    if enable_tool_publish:
        # Requirement 12.6: the deploy pipeline uploads the version's tools and
        # the server signs them. Without this credential the agent serves tools
        # it already has but refuses every upload, which is how /v1/catalog
        # stayed empty. Minting happens on the box so no plaintext token is
        # carried in the spec.
        token_path, _, minted = ensure_tool_publish_token(config_dir, user=user)
        # The config is read on the server, so the value is stated with POSIX
        # separators even though this runs wherever the deploy is driven from.
        cfg["toolsTokenFile"] = token_path.as_posix()
        notes.append(
            f"tool publishing enabled via {token_path}"
            + (" (new credential minted)" if minted else " (existing credential reused)")
        )
    # Registration now lives in the admin document, so the backfill reads the
    # merged result: a box mid-migration still has its roots in the config file,
    # and judging "do these share a parent" from the pre-merge value would miss
    # the products that just arrived.
    existing_admin: dict[str, Any] = {}
    if admin_file.is_file():
        existing_admin = load_json_object(admin_file.read_text(encoding="utf-8"))
    merged_admin: dict[str, Any] = {}
    if admin_doc.get("products") or admin_doc.get("tokens"):
        merged_admin, merge_notes = admin_merge(existing_admin, admin_doc)
        admin_file.parent.mkdir(parents=True, exist_ok=True)
        os.chmod(admin_file.parent, 0o700)
        admin_file.write_text(dump_json(merged_admin), encoding="utf-8")
        os.chmod(admin_file, 0o600)
        try:
            chown_path(admin_file, user)
            chown_path(admin_file.parent, user)
        except LookupError:
            pass
        notes.extend(f"admin state: {item}" for item in merge_notes)
        notes.append(f"admin state written to {admin_file}")
    # The document already on disk counts even when this run has nothing to
    # merge. An epoch-finished box keeps every product there and nothing in the
    # config, so ignoring it left the backfill with no products and let it
    # conclude from the unit alone.
    admin_products = registered_products_for_backfill(
        config_products=product_roots,
        existing_admin=existing_admin,
        merged_admin=merged_admin,
    )
    cfg, backfill_note = backfill_product_root_parent(
        cfg, working_directory=live_working_dir, product_roots=admin_products
    )
    if backfill_note:
        notes.append(backfill_note)
    config_file.write_text(dump_json(cfg), encoding="utf-8")

    if manifest_file.is_file():
        os.chmod(manifest_file, 0o600)
        try:
            chown_path(manifest_file, user)
        except LookupError:
            pass
    if products_dir.is_dir():
        for path in sorted(products_dir.glob("*.json")):
            profile = load_json_object(path.read_text(encoding="utf-8"))
            migrated, more = migrate_profile(
                profile,
                serve_addr=serve_addr,
                public_upload_url=public_upload_url,
            )
            if more:
                path.write_text(dump_json(migrated), encoding="utf-8")
                notes.extend(f"{path.name}: {item}" for item in more)
    if binary:
        install_file(binary, dest_bin, 0o755)
        notes.append(f"installed {dest_bin}")
    if not live_working_dir:
        raise Fail("cannot find WorkingDirectory in the live relkit-agent unit; refusing to guess")
    write_agent_unit(
        user=user,
        prefix=prefix,
        config_path=str(config_file),
        working_directory=live_working_dir,
    )
    run(["systemctl", "daemon-reload"])
    if restart:
        restart_unit("relkit-agent")
        require_unit_running("relkit-agent")
        require_listener_belongs_to("relkit-agent", str(cfg.get("addr") or "127.0.0.1:8787"))
        self_check_agent(str(cfg.get("addr") or "127.0.0.1:8787"))
        notes.append("agent restarted and health-checked")
        version = run([str(dest_bin), "-version"], capture=True)
        notes.append((version.stdout or "").strip() or "relkit-agent version ok")
    else:
        notes.append("agent files updated; restart skipped")
    return notes


# Old console configs named one tree adapter's parameters at the top level:
# `dir` (read the tree from this box's disk) and later `store` (read it from
# this relkit-store URL), plus a serve-era `stateDir`. The new binary is
# strict-decoded and refuses all three; this migration is the only place in
# the deploy layer that knows what they meant.
_CONSOLE_LEGACY_KEYS = ("dir", "store", "stateDir")


def console_retired_keys(console_cfg: dict[str, Any]) -> list[str]:
    """Legacy top-level keys a console config still carries."""
    return [key for key in _CONSOLE_LEGACY_KEYS if key in console_cfg]


def migrate_console_config(
    console_cfg: dict[str, Any], local_store_url: str
) -> tuple[dict[str, Any], list[str]]:
    """Rewrite a legacy console config into the "tree" adapter shape, once.

    `store` becomes a relkit-compatible tree entry with that URL. A config with
    only `dir` read this box's disk; the same tree is served by this box's
    store, so it becomes a relkit-compatible entry with local_store_url. An
    existing "tree" entry is never opened or changed. The panel state may not
    sit in the release tree or on a relative path. Returns the new config and
    one note per change; a migrated config yields no notes.
    """
    cfg = dict(console_cfg)
    notes: list[str] = []
    tree_dir = str(cfg.get("dir") or "").strip().rstrip("/")
    legacy_store = str(cfg.get("store") or "").strip()
    for key in _CONSOLE_LEGACY_KEYS:
        if key in cfg:
            notes.append(f"dropped retired console key {key}={cfg.pop(key)!r}")
    if "tree" not in cfg:
        url = legacy_store or (local_store_url if tree_dir else "")
        if url:
            cfg["tree"] = {"type": "relkit-compatible", "url": url}
            notes.append(f"panel tree adapter set to relkit-compatible at {url}")
    configured = str(cfg.get("adminStateFile") or "").strip()
    if configured:
        in_tree = bool(tree_dir) and (configured == tree_dir or configured.startswith(tree_dir + "/"))
        if not configured.startswith("/") or in_tree:
            cfg.pop("adminStateFile", None)
            notes.append(f"panel state moves from {configured} to {CONSOLE_STATE_FILE}")
    return cfg, notes


def apply_console_upgrade(
    *,
    binary: Optional[Path],
    user: str,
    prefix: str,
    agent_addr: str,
    local_store_url: str,
    restart: bool,
    backup_root: Path,
) -> list[str]:
    """Upgrade relkit-console in place onto the tree-adapter layout.

    The panel used to open the release tree directly when it shared a box with
    the store, and kept its cookie key inside that tree. Both are gone: it reads
    through the "tree" adapter entry like any client, and its key lives under
    its own StateDirectory. The existing key is carried over so nobody is
    signed out, and the copy left in the tree is removed (it is in the backup).
    """
    notes: list[str] = []
    show = systemd_show("relkit-console", "ExecStart", "User", "FragmentPath")
    config_path = parse_exec_config(show.get("ExecStart") or "") or "/etc/relkit-console/relkit-console.json"
    config_file = Path(config_path)
    dest_bin = Path(prefix) / "relkit-console"
    cfg: dict[str, Any] = {}
    if config_file.is_file():
        cfg = load_json_object(config_file.read_text(encoding="utf-8"))
    legacy_files = console_legacy_panel_files(cfg, str(config_file))
    backup_files(
        backup_root / "console",
        [
            dest_bin,
            config_file,
            Path("/etc/systemd/system/relkit-console.service"),
            *legacy_files,
        ],
    )

    # local_store_url is only consulted to migrate a legacy `dir` config.
    cfg, cfg_notes = migrate_console_config(cfg, local_store_url)
    notes.extend(cfg_notes)
    if "tree" not in cfg:
        raise Fail("relkit-console config has no \"tree\" adapter entry and none could be migrated")
    panel_file = Path(str(console_admin_path(cfg)))
    if not panel_file.is_file():
        source = next((p for p in legacy_files if p != panel_file and p.is_file()), None)
        if source is not None:
            panel_file.parent.mkdir(parents=True, exist_ok=True)
            os.chmod(panel_file.parent, 0o700)
            shutil.copy2(source, panel_file)
            notes.append(f"panel cookie key carried over from {source} to {panel_file}")
    for stray in legacy_files:
        if stray != panel_file and stray.is_file():
            try:
                stray.unlink()
                notes.append(f"removed old panel state {stray} (backed up)")
            except OSError as exc:
                notes.append(f"left old panel state {stray}: {exc}")
    # The panel is the human face of the agent (requirement 13.6): without the
    # agent URL it renders read-only pages and cannot manage anything.
    if agent_addr and str(cfg.get("agent") or "") != agent_addr:
        cfg["agent"] = agent_addr
        notes.append(f"panel now manages the agent at {agent_addr}")
    if config_file.is_file() or cfg:
        config_file.parent.mkdir(parents=True, exist_ok=True)
        config_file.write_text(dump_json(cfg), encoding="utf-8")
        os.chmod(config_file, 0o644)
        try:
            chown_path(config_file, user)
        except LookupError:
            pass
    if panel_file.is_file():
        os.chmod(panel_file, 0o600)
        try:
            chown_path(panel_file, user)
            chown_path(panel_file.parent, user)
        except LookupError:
            pass

    if binary:
        install_file(binary, dest_bin, 0o755)
        notes.append(f"installed {dest_bin}")
    write_console_unit(
        user=user,
        prefix=prefix,
        config_path=str(config_file),
        console_cfg=cfg,
    )
    run(["systemctl", "daemon-reload"])
    if restart:
        restart_unit("relkit-console")
        console_addr = str(cfg.get("addr") or "127.0.0.1:8081")
        require_unit_running("relkit-console")
        require_listener_belongs_to("relkit-console", console_addr)
        wait_health(loopback_base(console_addr), "relkit-console")
        notes.append("console restarted and health-checked")
        version = run([str(dest_bin), "-version"], capture=True)
        notes.append((version.stdout or "").strip() or "relkit-console version ok")
    else:
        notes.append("console files updated; restart skipped")
    return notes


def rollback(backup_root: Path, prefix: str) -> None:
    serve_bin = backup_root / "serve" / "relkit-store"
    if serve_bin.is_file():
        install_file(serve_bin, Path(prefix) / "relkit-store", 0o755)
    serve_cfg = backup_root / "serve" / "relkit-store.json"
    if serve_cfg.is_file():
        dest = Path("/etc/relkit-store/relkit-store.json")
        show = systemd_show("relkit-store", "ExecStart")
        config_path = parse_exec_config(show.get("ExecStart") or "")
        if config_path:
            dest = Path(config_path)
        shutil.copy2(serve_cfg, dest)
    unit = backup_root / "serve" / "relkit-store.service"
    if unit.is_file():
        shutil.copy2(unit, "/etc/systemd/system/relkit-store.service")
    # A retired relkit-serve unit is restored before the unit set is reloaded, so
    # a failed upgrade leaves the box exactly as it was: old serve binary, old
    # unit, no permanent retirement that nobody asked for.
    retired_serve = backup_root / "serve" / "relkit-serve.service"
    if retired_serve.is_file():
        shutil.copy2(retired_serve, "/etc/systemd/system/relkit-serve.service")
        stale = Path("/etc/systemd/system/relkit-serve.service.retired")
        if stale.exists():
            stale.unlink()
    agent_bin = backup_root / "agent" / "relkit-agent"
    if agent_bin.is_file():
        install_file(agent_bin, Path(prefix) / "relkit-agent", 0o755)
    agent_cfg = backup_root / "agent" / "relkit-agent.json"
    if agent_cfg.is_file():
        dest = Path("/etc/relkit-agent/relkit-agent.json")
        show = systemd_show("relkit-agent", "ExecStart")
        config_path = parse_exec_config(show.get("ExecStart") or "")
        if config_path:
            dest = Path(config_path)
        shutil.copy2(agent_cfg, dest)
    products = backup_root / "agent" / "products"
    if products.is_dir():
        live = Path("/etc/relkit-agent/products")
        if live.parent.is_dir():
            if live.exists():
                shutil.rmtree(live)
            shutil.copytree(products, live)
    agent_unit = backup_root / "agent" / "relkit-agent.service"
    if agent_unit.is_file():
        shutil.copy2(agent_unit, "/etc/systemd/system/relkit-agent.service")
    console_bin = backup_root / "console" / "relkit-console"
    if console_bin.is_file():
        install_file(console_bin, Path(prefix) / "relkit-console", 0o755)
    console_cfg = backup_root / "console" / "relkit-console.json"
    if console_cfg.is_file():
        dest = Path("/etc/relkit-console/relkit-console.json")
        show = systemd_show("relkit-console", "ExecStart")
        config_path = parse_exec_config(show.get("ExecStart") or "")
        if config_path:
            dest = Path(config_path)
        if dest.parent.is_dir():
            shutil.copy2(console_cfg, dest)
    console_unit = backup_root / "console" / "relkit-console.service"
    if console_unit.is_file():
        shutil.copy2(console_unit, "/etc/systemd/system/relkit-console.service")
    run(["systemctl", "daemon-reload"], check=False)
    run(["systemctl", "restart", "relkit-store"], check=False)
    run(["systemctl", "restart", "relkit-agent"], check=False)
    if console_bin.is_file() or console_cfg.is_file() or console_unit.is_file():
        run(["systemctl", "restart", "relkit-console"], check=False)


def cmd_remote(args: argparse.Namespace) -> None:
    must_root()
    if args.remote_cmd == "probe":
        print(json.dumps(probe_local(), ensure_ascii=False, indent=2))
        return
    if args.remote_cmd == "migrate-serve":
        spec = load_json_object(Path(args.spec).read_text(encoding="utf-8"))
        backup = Path(spec.get("backup") or f"/var/backups/relkit/{int(time.time())}")
        prefix = str(spec.get("prefix") or "/usr/local/bin")
        user = str(spec.get("user") or "relkit")
        store_bin = Path(spec["storeBinary"]) if spec.get("storeBinary") else None
        notes: list[str] = []
        try:
            if store_bin:
                install_file(store_bin, Path(prefix) / "relkit-store", 0o755)
                notes.append(f"installed {prefix}/relkit-store")
            notes.extend(
                migrate_serve_to_store(user=user, prefix=prefix, backup_root=backup)
            )
        except Exception as exc:
            print(f"migrate failed: {redact_text(str(exc))}", file=sys.stderr)
            raise Fail("migrate-serve failed at " + str(backup)) from exc
        print(json.dumps({"ok": True, "backup": str(backup), "notes": notes}, ensure_ascii=False, indent=2))
        return
    if args.remote_cmd == "prune-backups":
        root = Path(args.backup_root or "/var/backups/relkit")
        if not root.is_dir():
            print(json.dumps({"ok": True, "root": str(root), "total": 0, "removed": [], "remaining": [], "dryRun": args.dry_run}, ensure_ascii=False, indent=2))
            return
        snapshots = [path.name for path in snapshot_dirs(root)]
        if args.dry_run:
            # Report what a real run would do, using the same selection rule, so
            # --dry-run never disagrees with --apply about which snapshots go.
            removed = prune_backups(root, args.keep, dry_run=True)
        else:
            removed = prune_backups(root, args.keep)
        gone = set(removed)
        remaining = [name for name in snapshots if name not in gone]
        print(
            json.dumps(
                {
                    "ok": True,
                    "root": str(root),
                    "total": len(snapshots),
                    "removed": removed,
                    "remaining": remaining,
                    "dryRun": args.dry_run,
                },
                ensure_ascii=False,
                indent=2,
            )
        )
        return
    if args.remote_cmd == "apply":
        spec = load_json_object(Path(args.spec).read_text(encoding="utf-8"))
        backup = Path(spec.get("backup") or f"/var/backups/relkit/{int(time.time())}")
        prefix = str(spec.get("prefix") or "/usr/local/bin")
        user = str(spec.get("user") or "relkit")
        restart = bool(spec.get("restart"))
        public_upload = spec.get("publicUploadUrl")
        serve_bin = Path(spec["serveBinary"]) if spec.get("serveBinary") else None
        agent_bin = Path(spec["agentBinary"]) if spec.get("agentBinary") else None
        console_bin = Path(spec["consoleBinary"]) if spec.get("consoleBinary") else None
        notes: list[str] = []
        try:
            if spec.get("upgradeServe"):
                notes.extend(
                    apply_serve_upgrade(
                        binary=serve_bin,
                        user=user,
                        prefix=prefix,
                        listen_addr=spec.get("serveListenAddr"),
                        public_upload_url=public_upload,
                        restart=restart,
                        backup_root=backup,
                    )
                )
            serve_addr = ""
            show = systemd_show("relkit-store", "ExecStart")
            cfg_path = parse_exec_config(show.get("ExecStart") or "")
            if cfg_path and Path(cfg_path).is_file():
                serve_addr = str(load_json_object(Path(cfg_path).read_text(encoding="utf-8")).get("addr") or "")
            if spec.get("upgradeAgent"):
                notes.extend(
                    apply_agent_upgrade(
                        binary=agent_bin,
                        user=user,
                        prefix=prefix,
                        public_upload_url=public_upload,
                        serve_addr=serve_addr,
                        restart=restart,
                        backup_root=backup,
                        root_key_id=spec.get("rootKeyId") or "",
                        custody_manifest=spec.get("custodyManifest") or "",
                        admin_state_file=spec.get("adminStateFile") or "",
                        enable_tool_publish=bool(spec.get("enableToolPublish")),
                    )
                )
            if spec.get("upgradeConsole"):
                # The panel talks to the agent, so it is configured after the
                # agent has been migrated: the address written into the panel
                # config must be the live one.
                agent_addr = ""
                show = systemd_show("relkit-agent", "ExecStart")
                cfg_path = parse_exec_config(show.get("ExecStart") or "")
                if cfg_path and Path(cfg_path).is_file():
                    agent_addr = str(
                        load_json_object(Path(cfg_path).read_text(encoding="utf-8")).get("addr") or ""
                    )
                if agent_addr:
                    agent_addr = loopback_base(agent_addr)
                notes.extend(
                    apply_console_upgrade(
                        binary=console_bin,
                        user=user,
                        prefix=prefix,
                        agent_addr=agent_addr,
                        local_store_url=loopback_base(serve_addr) if serve_addr else "",
                        restart=restart,
                        backup_root=backup,
                    )
                )
            tool_files = spec.get("toolFiles") or []
            if tool_files:
                # Requirement 12.6: deploy publishes the version's tools and the
                # server signs them, so hosts can fetch a tool build that matches
                # the release they just installed. Doing it here rather than by
                # hand keeps the catalog in step with the release: a version
                # whose tools were never published is a version that hosts
                # cannot bootstrap the CLI from.
                notes.extend(
                    publish_release_tools(
                        files=tool_files,
                        version=str(spec.get("version") or ""),
                        commit=str(spec.get("commit") or ""),
                        user=user,
                        restart=restart,
                    )
                )
        except Exception as exc:
            print(f"apply failed: {redact_text(str(exc))}", file=sys.stderr)
            rollback(backup, prefix)
            raise Fail("upgrade failed; rolled back from " + str(backup)) from exc
        keep = int(spec.get("keepBackups") or 0)
        if keep:
            try:
                removed = prune_backups(backup.parent, keep, current=backup)
            except OSError as exc:
                # Rotation is housekeeping: an upgrade that applied cleanly must
                # not be reported as failed because an old snapshot would not
                # delete. Report it and leave the directory alone.
                notes.append(f"backup rotation skipped: {redact_text(str(exc))}")
                removed = []
            if removed:
                notes.append(
                    f"backup rotation kept {keep} snapshot(s), removed {len(removed)}: "
                    + ", ".join(removed)
                )
        print(json.dumps({"ok": True, "backup": str(backup), "notes": notes}, ensure_ascii=False, indent=2))
        return
    die(f"unknown remote command {args.remote_cmd}")


def ssh_argv(host: str, extra: Sequence[str] | None = None) -> list[str]:
    require_cmd("ssh")
    argv = ["ssh", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", host]
    if extra:
        argv.extend(extra)
    return argv


def remote_bootstrap_sources() -> list[tuple[Path, str]]:
    """Local files copied next to the remote CLI. dest is a subdir of remote_dir ('' = root)."""
    return [
        (DEPLOY_DIR / "relkit.py", ""),
        (DEPLOY_DIR / "relkit_ops.py", ""),
        (DEPLOY_DIR / "components.py", ""),
        # components.py reads the registry at import time. A remote bootstrap has
        # no repository above it, so the table has to travel with the CLI.
        (REGISTRY_PATH, ""),
        (DEPLOY_DIR / "relkit-store.service", ""),
        (DEPLOY_DIR / "relkit-agent.service", ""),
        (DEPLOY_DIR / "relkit-console.service", ""),
        # Tool publishing runs on the server: it needs the local credential and
        # a reachable agent, so the helper travels with the CLI.
        (DEPLOY_DIR / "relkit_tools_publish.py", ""),
    ]


def scp_to(host: str, sources: Sequence[Path], dest: str) -> None:
    require_cmd("scp")
    argv = ["scp", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new"]
    argv.extend(str(path) for path in sources)
    argv.append(f"{host}:{dest}")
    run(argv)


def remote_python(host: str) -> str:
    for interpreter in ("python3", "/usr/bin/python3", "python3.11", "python3.9"):
        out = run(
            ssh_argv(host, [interpreter, "--version"]),
            capture=True,
            check=False,
        )
        blob = (out.stdout or "") + "\n" + (out.stderr or "")
        line = ""
        for candidate in blob.splitlines():
            if candidate.strip().startswith("Python "):
                line = candidate.strip()
        parts = line.replace("Python", "").strip().split(".")
        if out.returncode != 0 or len(parts) < 2 or not parts[0].isdigit() or not parts[1].isdigit():
            continue
        major, minor = int(parts[0]), int(parts[1])
        if (major, minor) < MIN_PY:
            die(f"{host} {interpreter} is {major}.{minor}; need {MIN_PY[0]}.{MIN_PY[1]}+")
        return interpreter
    die(f"{host} has no usable python3 (tried python3 and /usr/bin/python3)")


def cmd_migrate_serve(args: argparse.Namespace) -> None:
    host = args.host
    py = remote_python(host)
    remote_dir = "/tmp/relkit-deploy"
    run(ssh_argv(host, ["mkdir", "-p", remote_dir]))
    by_dest: dict[str, list[Path]] = {}
    for path, sub in remote_bootstrap_sources():
        by_dest.setdefault(sub, []).append(path)
    for sub, paths in by_dest.items():
        dest = remote_dir if not sub else f"{remote_dir}/{sub}"
        if sub:
            run(ssh_argv(host, ["mkdir", "-p", dest]))
        scp_to(host, paths, dest + "/")

    store_bin = Path(args.store_binary)
    if not store_bin.is_absolute():
        store_bin = (REPO_ROOT / store_bin).resolve()
    if not store_bin.is_file():
        die(f"store binary not found: {store_bin}")
    scp_to(host, [store_bin], remote_dir + "/")

    spec = {
        "user": args.user,
        "prefix": args.prefix,
        "storeBinary": f"{remote_dir}/{store_bin.name}",
        "backup": f"/var/backups/relkit/{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}",
    }
    fd, spec_name = tempfile.mkstemp(prefix="relkit-spec-", suffix=".json")
    os.close(fd)
    spec_path = Path(spec_name)
    spec_path.write_text(json.dumps(spec, indent=2) + "\n", encoding="utf-8")
    try:
        scp_to(host, [spec_path], remote_dir + "/spec.json")
    finally:
        try:
            spec_path.unlink()
        except OSError:
            pass
    apply = run(
        ssh_argv(
            host,
            ["sudo", py, remote_dir + "/relkit.py", "remote", "migrate-serve", "--spec", remote_dir + "/spec.json"],
        ),
        capture=True,
        check=False,
    )
    print(redact_text(apply.stdout or ""))
    if apply.returncode != 0:
        print(redact_text(apply.stderr or ""), file=sys.stderr)
        die("remote migrate-serve failed")


def cmd_upgrade(args: argparse.Namespace) -> None:
    host = args.host
    if not host:
        die("--host is required")
    if not args.plan and not args.apply:
        args.plan = True
    if args.apply and args.plan:
        die("pass either --plan or --apply")
    py = remote_python(host)
    remote_dir = "/tmp/relkit-deploy"
    run(ssh_argv(host, ["mkdir", "-p", remote_dir]))
    by_dest: dict[str, list[Path]] = {}
    for path, sub in remote_bootstrap_sources():
        by_dest.setdefault(sub, []).append(path)
    for sub, paths in by_dest.items():
        dest = remote_dir if not sub else f"{remote_dir}/{sub}"
        if sub:
            run(ssh_argv(host, ["mkdir", "-p", dest]))
        scp_to(host, paths, dest + "/")
    probe_out = run(
        ssh_argv(host, ["sudo", py, remote_dir + "/relkit.py", "remote", "probe"]),
        capture=True,
    )
    probe = load_json_object(probe_out.stdout or "{}")
    print(json.dumps(redact_value(probe), ensure_ascii=False, indent=2))
    want_serve = not args.agent_only
    want_agent = not args.serve_only
    want_console = bool((probe.get("console") or {}).get("unit", {}).get("FragmentPath")) and not args.no_console
    missing = missing_upgrade_targets(probe, want_serve=want_serve, want_agent=want_agent)
    if probe.get("console") and not (probe.get("console") or {}).get("present") and want_console:
        missing.append(
            "relkit-console (no binary at "
            f"{(probe.get('console') or {}).get('binary') or 'unknown path'}); "
            "pass --no-console to skip it, or run 'install console' to add it"
        )
    if missing:
        die(f"{host} does not run " + "; ".join(missing))
    serve_cfg = (probe.get("serve") or {}).get("config") or {}
    if want_serve and isinstance(serve_cfg, dict):
        _, grace = ensure_cas_grace(serve_cfg)
        print("serve migrate notes:", grace or ["addr/dir preserved"])
    public_upload = args.public_upload_url
    for name, profile in (probe.get("profiles") or {}).items():
        if not isinstance(profile, dict) or profile.get("error"):
            continue
        try:
            _, notes = migrate_profile(
                profile,
                serve_addr=str(serve_cfg.get("addr") or ""),
                public_upload_url=public_upload,
            )
            print(f"profile {name}:", notes or ["no changes"])
        except ValueError as exc:
            die(f"profile {name}: {exc}")
    ident = git_identity()
    protocol_min, protocol_max = publish_protocol_window()
    print(
        "source:",
        json.dumps(
            {
                "commit": ident["commit"],
                "dirty": ident["dirty"],
                "protocol": f"{protocol_min}-{protocol_max}",
            },
            indent=2,
        ),
    )
    if ident["dirty"] == "true" and args.apply and not args.allow_dirty:
        die("refusing to upgrade from a dirty HEAD; commit or pass --allow-dirty")
    if args.plan:
        print("\nplan only; no files changed. Re-run with --apply to build, ship, and restart.")
        return
    restart = not args.stage_only
    if not restart:
        print("WARNING: --stage-only updates files but does not restart; upgrade is not complete")
    if args.keep_backups < 1:
        die("--keep-backups must be at least 1")
    keep_backups = 0 if args.no_prune_backups else args.keep_backups
    # Requirement 12.6: a release that ships tools also publishes them, so a
    # host that installed this version can fetch a matching CLI afterwards. It
    # is skippable only for a server that never serves tools.
    want_tools = not args.no_tools
    spec = {
        "upgradeServe": want_serve,
        "upgradeAgent": want_agent,
        "upgradeConsole": want_console,
        "restart": restart,
        "prefix": args.prefix,
        "user": args.user,
        "serveListenAddr": args.serve_listen_addr,
        "publicUploadUrl": public_upload,
        "backup": f"/var/backups/relkit/{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}",
        "keepBackups": keep_backups,
        "rootKeyId": args.root_key_id,
        "custodyManifest": args.custody_manifest,
        "adminStateFile": args.admin_state_file,
        "enableToolPublish": want_tools,
        "version": git_stamp(load_ssot()["number"]),
        "commit": ident["commit"],
    }
    uploads: list[Path] = []
    if not args.unsafe_from_dist:
        step(f"building linux/amd64 {'agent+serve+console' if want_tools else 'agent+serve+console'} from HEAD")
        cmd_build(
            argparse.Namespace(
                store=spec["upgradeServe"],
                agent=spec["upgradeAgent"],
                console=spec["upgradeConsole"],
                out="dist",
                os="linux",
                arch="amd64",
                append=False,
            )
        )
        if want_tools:
            # Tools are what host CI downloads on every build, so they must
            # cover every platform a host builds on (requirement 12). They are
            # built in a second pass so the server binaries stay linux/amd64:
            # pinning the tool flags to this machine's platform -- which is what
            # this did -- silently shrinks the served tool surface back to one
            # platform on every upgrade.
            step("building tools for every host platform")
            cmd_build(
                argparse.Namespace(
                    **{row.build_flag.replace("-", "_"): True for row in TOOL_COMPONENTS},
                    out="dist",
                    os="",
                    arch="",
                    # Carry the server rows forward: one release is described by
                    # one manifest, and the tools publish step verifies every
                    # tool against it.
                    append=True,
                )
            )
    tools_built: list[dict[str, Any]] = []
    if want_tools:
        manifest_path = REPO_ROOT / "dist" / "manifest.json"
        if manifest_path.is_file():
            tools_built = [
                row
                for row in load_json_object(manifest_path.read_text(encoding="utf-8")).get("artifacts", [])
                if row.get("component") in TOOL_COMPONENT_NAMES
            ]
        if not tools_built:
            die(
                f"the release declares no {', '.join(TOOL_COMPONENT_NAMES)} artifact in "
                "dist/manifest.json; build them or pass --no-tools"
            )
    if spec["upgradeServe"]:
        serve_bin = Path(args.serve_binary)
        if not serve_bin.is_absolute():
            serve_bin = (REPO_ROOT / serve_bin).resolve()
        if not serve_bin.is_file():
            die(f"serve binary not found: {serve_bin}")
        uploads.append(serve_bin)
        spec["serveBinary"] = f"{remote_dir}/{serve_bin.name}"
    if spec["upgradeAgent"]:
        agent_bin = Path(args.agent_binary)
        if not agent_bin.is_absolute():
            agent_bin = (REPO_ROOT / agent_bin).resolve()
        if not agent_bin.is_file():
            die(f"agent binary not found: {agent_bin}")
        uploads.append(agent_bin)
        spec["agentBinary"] = f"{remote_dir}/{agent_bin.name}"
    if spec["upgradeConsole"]:
        console_bin = Path(args.console_binary)
        if not console_bin.is_absolute():
            console_bin = (REPO_ROOT / console_bin).resolve()
        if not console_bin.is_file():
            die(f"console binary not found: {console_bin}")
        uploads.append(console_bin)
        spec["consoleBinary"] = f"{remote_dir}/{console_bin.name}"
    if tools_built:
        tool_rows: list[dict[str, Any]] = []
        for row in tools_built:
            tool_path = REPO_ROOT / "dist" / str(row["path"])
            if not tool_path.is_file():
                die(f"tool artifact listed in the manifest is missing: {tool_path}")
            uploads.append(tool_path)
            tool_rows.append(
                {
                    "path": f"{remote_dir}/{tool_path.name}",
                    "component": row.get("component") or "",
                    "os": row.get("os") or "",
                    "arch": row.get("arch") or "",
                }
            )
        spec["toolFiles"] = tool_rows
    if uploads:
        scp_to(host, uploads, remote_dir + "/")
    fd, spec_name = tempfile.mkstemp(prefix="relkit-spec-", suffix=".json")
    os.close(fd)
    spec_path = Path(spec_name)
    spec_path.write_text(json.dumps(spec, indent=2) + "\n", encoding="utf-8")
    try:
        scp_to(host, [spec_path], remote_dir + "/spec.json")
    finally:
        try:
            spec_path.unlink()
        except OSError:
            pass
    apply = run(
        ssh_argv(
            host,
            [
                "sudo",
                py,
                remote_dir + "/relkit.py",
                "remote",
                "apply",
                "--spec",
                remote_dir + "/spec.json",
            ],
        ),
        capture=True,
        check=False,
    )
    print(redact_text(apply.stdout or ""))
    if apply.returncode != 0:
        print(redact_text(apply.stderr or ""), file=sys.stderr)
        die("remote apply failed")
    if not restart:
        print("apply finished as stage-only; systemd still runs the previous process")


def cmd_prune_backups(args: argparse.Namespace) -> None:
    host = args.host
    if args.keep < 1:
        die("--keep must be at least 1")
    py = remote_python(host)
    remote_dir = "/tmp/relkit-deploy"
    run(ssh_argv(host, ["mkdir", "-p", remote_dir]))
    by_dest: dict[str, list[Path]] = {}
    for path, sub in remote_bootstrap_sources():
        by_dest.setdefault(sub, []).append(path)
    for sub, paths in by_dest.items():
        dest = remote_dir if not sub else f"{remote_dir}/{sub}"
        if sub:
            run(ssh_argv(host, ["mkdir", "-p", dest]))
        scp_to(host, paths, dest + "/")
    argv = [
        "sudo",
        py,
        remote_dir + "/relkit.py",
        "remote",
        "prune-backups",
        "--keep",
        str(args.keep),
    ]
    if args.dry_run:
        argv.append("--dry-run")
    out = run(ssh_argv(host, argv), capture=True, check=False)
    print(out.stdout or "")
    if out.returncode != 0:
        print(redact_text(out.stderr or ""), file=sys.stderr)
        die("remote prune-backups failed")


def cmd_version(args: argparse.Namespace) -> None:
    ssot = load_ssot()
    if args.check_tag:
        expected = ssot["tag"]
        if args.check_tag != expected:
            die(f"git tag {args.check_tag} does not match VERSION.json {expected}")
        print(expected)
        return
    print(ssot[args.field])


# The relkit components a host CI downloads and runs. Only the ones a host
# actually fetches are published as tools: the server programs are installed by
# the deploy pipeline itself, never fetched by a host.
#
# sdk-dart is included because requirement 12 makes the server the only place a
# host materialises its thin client from: SvnMergeTool resolves its `rup_client`
# dependency out of the served tool surface at build time, so a tool surface
# without sdk-dart leaves that host unable to build at all.
#
# sdk-rust is included for the same reason: Loom-Launcher's Rust shell links the
# facade crate (`relkit-updater = { path = ".relkit/components/sdk/rust" }`), so
# a server that serves no sdk-rust leaves that host unable to build at all. It is
# a thin IPC client (it spawns the updater sidecar), which is exactly the shape
# requirement 12 keeps on the server.
TOOL_COMPONENT_NAMES = ("cli", "updater", "ops", "sdk-dart", "sdk-rust")
# Binaries are cross-compiled per platform; trees are platform-independent and
# are published once under the "all" platform a host asks for by name.
TOOL_BINARY_NAMES = ("cli", "updater", "ops")
TOOL_TREE_NAMES = ("sdk-dart", "sdk-rust")
TOOL_COMPONENT_FLAGS = TOOL_BINARY_NAMES


def tool_component_flag(name: str) -> str:
    row = BY_NAME.get(name)
    return row.build_flag.replace("-", "_") if row else ""


def tool_components() -> list[Any]:
    """Return the registry rows a host CI downloads as tools."""
    return [BY_NAME[name] for name in TOOL_COMPONENT_NAMES if name in BY_NAME]


TOOL_COMPONENTS = tool_components()


def tool_platform(os_name: str, arch: str) -> str:
    # A tree component (sdk-dart) ships one artifact for every platform;
    # `relkit build` records it as os/arch = any, while hosts ask for it by the
    # "all" platform name. Anything else is the plain os-arch selector.
    if os_name == "any" and arch == "any":
        return "all"
    return f"{os_name}-{arch}"


def release_artifact_index(manifest: dict[str, Any]) -> dict[tuple[str, str, str], str]:
    """Map (component, os, arch) to the digest the release manifest declares.

    Requirement 12.6: the pipeline validates every tool against the official
    Release before the server signs it, so a stray local build can never become
    the toolchain a host runs.
    """
    index: dict[tuple[str, str, str], str] = {}
    for row in manifest.get("artifacts") or []:
        if not isinstance(row, dict):
            continue
        component = str(row.get("component") or "")
        os_name = str(row.get("os") or "")
        arch = str(row.get("arch") or "")
        digest = str(row.get("sha256") or "")
        if component and os_name and arch and digest:
            index[(component, os_name, arch)] = digest
    return index


def file_digest(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1 << 20), b""):
            digest.update(chunk)
    return "sha256:" + digest.hexdigest()


def strip_digest_prefix(value: str) -> str:
    """Return the bare hex digest, dropping any algorithm prefix and case.

    Two writers record artifact digests: `relkit build` (Go) writes bare hex
    into dist/manifest.json, while this module's own file_sha256 also writes
    bare hex and file_digest prefixes "sha256:". Comparing raw strings therefore
    reports a mismatch for a byte-identical file and blocks every tool publish,
    so the comparison normalises both sides.
    """
    text = str(value or "").strip().lower()
    if text.startswith("sha256:"):
        text = text[len("sha256:"):]
    return text


def cmd_tools(args: argparse.Namespace) -> None:
    if args.tools_cmd != "publish":
        die(f"unknown tools command {args.tools_cmd}")
    dist = Path(args.dist)
    if not dist.is_dir():
        die(f"{dist} does not exist; run relkit build first")
    manifest_path = Path(args.release_manifest) if args.release_manifest else dist / "manifest.json"
    if not manifest_path.is_file():
        die(f"{manifest_path} does not exist; requirement 12.6 needs the official release manifest")
    release = load_json_object(manifest_path.read_text(encoding="utf-8"))
    if release.get("schema") != "relkit.release/1":
        die(f"{manifest_path} schema is {release.get('schema')!r}, expected relkit.release/1")
    version = str(release.get("version") or "")
    commit = str(release.get("commit") or "")
    if not version or not commit:
        die(f"{manifest_path} is missing version or commit")
    if args.version and args.version != version:
        die(f"--version {args.version} does not match {manifest_path} version {version}")

    base = (args.server or "").rstrip("/")
    if not base:
        die("--server is required; it is the agent the tools are published to")
    token = args.token or os.environ.get("RELKIT_TOOLS_TOKEN", "")
    if not token:
        die("--token or RELKIT_TOOLS_TOKEN is required (the deploy pipeline credential)")

    index = release_artifact_index(release)
    plan: list[dict[str, Any]] = []
    for flag in TOOL_BINARY_NAMES:
        row = BY_NAME.get(flag)
        if row is None:
            continue
        for target in TARGETS:
            parts = target.split("-", 1)
            if len(parts) != 2:
                continue
            os_name, arch = parts[0], parts[1]
            declared = index.get((flag, os_name, arch))
            if declared is None:
                # A platform the release did not build is not an error: not
                # every component targets every platform.
                continue
            name = f"{row.binary_prefix}-{os_name}-{arch}"
            if os_name == "windows":
                name += ".exe"
            path = dist / name
            if not path.is_file():
                die(f"{path} is missing; the release manifest declares {flag} for {target}")
            actual = file_digest(path)
            # The release manifest records a bare digest (relkit build writes
            # hex, and so does Python file_sha256), while file_digest prefixes
            # its result. Normalising both sides keeps the check strict about
            # the value without turning the prefix into a false mismatch.
            if strip_digest_prefix(actual) != strip_digest_prefix(declared):
                die(
                    f"{path} digest {actual} does not match the official release "
                    f"{declared} for {flag}/{target}; refusing to publish a tool the "
                    "release did not build"
                )
            plan.append({
                "component": flag, "name": name, "platform": tool_platform(os_name, arch),
                "os": os_name, "arch": arch, "size": path.stat().st_size,
                "sha256": actual, "path": path,
            })
    for name in TOOL_TREE_NAMES:
        row = BY_NAME.get(name)
        if row is None or not row.archive:
            continue
        # A tree ships one artifact for every platform, and `relkit build`
        # records it that way (os/arch = any). Hosts still select by platform,
        # so it is published under the "all" platform they ask for by name.
        declared = index.get((name, "any", "any"))
        if declared is None:
            continue
        path = dist / row.archive
        if not path.is_file():
            die(f"{path} is missing; the release manifest declares {name}")
        actual = file_digest(path)
        if strip_digest_prefix(actual) != strip_digest_prefix(declared):
            die(
                f"{path} digest {actual} does not match the official release "
                f"{declared} for {name}; refusing to publish a tool the "
                "release did not build"
            )
        plan.append({
            "component": name, "name": row.archive, "platform": "all",
            "os": "any", "arch": "any", "size": path.stat().st_size,
            "sha256": actual, "path": path,
        })
    if not plan:
        die(f"the release manifest declares no {', '.join(TOOL_COMPONENT_NAMES)} artifact")
    if args.dry_run:
        print(json.dumps(
            [{"component": row["component"], "name": row["name"], "platform": row["platform"],
              "sha256": row["sha256"]} for row in plan],
            ensure_ascii=False, indent=2,
        ))
        return

    for row in plan:
        url = f"{base}/v1/tools/{version}/{row['name']}"
        code, body = http_call(
            "PUT", url,
            headers={"Authorization": f"Bearer {token}"},
            data=row["path"].read_bytes(),
            timeout=600,
            publish_protocol=False,
        )
        if code not in (200, 201):
            die(f"upload {row['name']} returned {code}: {body[:400].decode('utf-8', 'replace')}")
        print(f"uploaded {row['name']} ({row['platform']})")

    payload = {
        "version": version,
        "commit": commit,
        "files": [
            {key: row[key] for key in ("component", "name", "platform", "os", "arch", "size", "sha256")}
            for row in plan
        ],
    }
    # The agent signs its own publicURL into every tool URL; the caller has no
    # say, so publishing over loopback cannot leak 127.0.0.1 into the manifest.
    code, body = http_call(
        "POST", f"{base}/v1/tools",
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
        data=json.dumps(payload).encode(),
        timeout=120,
        publish_protocol=False,
    )
    if code != 200:
        die(f"publish manifest returned {code}: {body[:400].decode('utf-8', 'replace')}")
    result = json.loads(body.decode("utf-8", "replace"))
    print(json.dumps(result, ensure_ascii=False, indent=2))
    print(f"tools {version} are current; host CI downloads them from {base}")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="scripts/deploy/relkit.py", description="relkit deploy CLI")
    sub = parser.add_subparsers(dest="cmd", required=True)

    build = sub.add_parser("build", help="cross-compile binaries")
    for row in registry_components():
        build.add_argument(f"--{row.build_flag}", action="store_true")
    build.add_argument("--all", action="store_true", help="build every registry component")
    build.add_argument(
        "--append",
        action="store_true",
        help=(
            "keep artifacts already listed in <out>/manifest.json for the same "
            "version instead of overwriting them; one release is built from both "
            "the host binaries and the tool binaries"
        ),
    )
    build.add_argument("--out", default="dist")
    build.add_argument("--os")
    build.add_argument("--arch")

    install = sub.add_parser("install", help="first-time systemd install")
    install_sub = install.add_subparsers(dest="component", required=True)
    serve_i = install_sub.add_parser("serve")
    serve_i.add_argument("--binary", required=True)
    serve_i.add_argument("--dir", default="/srv/releases")
    serve_i.add_argument("--config-dir", default="/etc/relkit-store")
    serve_i.add_argument("--addr", default="127.0.0.1:30341")
    serve_i.add_argument("--user", default="relkit")
    serve_i.add_argument("--prefix", default="/usr/local/bin")
    serve_i.add_argument("--rotate-token", action="store_true")
    serve_i.add_argument("--force-reconfigure", action="store_true")
    agent_i = install_sub.add_parser("agent")
    agent_i.add_argument("--binary", required=True)
    agent_i.add_argument("--config-dir", default="/etc/relkit-agent")
    agent_i.add_argument("--state-dir", default="/var/lib/relkit-agent")
    agent_i.add_argument(
        "--product-root",
        required=True,
        help=(
            "parent directory of the product trees, written to the agent config as "
            "productRootParent; also becomes the service account's home and the unit's "
            "WorkingDirectory. Must be absolute. There is no default: nothing on an empty "
            "box says where releases live, and a guessed path the service account cannot "
            "create only fails later, at publish time"
        ),
    )
    agent_i.add_argument(
        "--config-example",
        default="",
        help="JSON copied when relkit-agent.json is absent (default: relkit-agent.example.json)",
    )
    agent_i.add_argument("--user", default="relkit")
    agent_i.add_argument("--prefix", default="/usr/local/bin")
    console_i = install_sub.add_parser("console")
    console_i.add_argument("--binary", required=True)
    console_i.add_argument(
        "--config-example",
        default="",
        help="config whose \"tree\" adapter entry the panel uses (default: " + CONSOLE_CONFIG_EXAMPLE + ")",
    )
    console_i.add_argument(
        "--agent",
        default="",
        help="relkit-agent base URL the panel manages (default: the agent on this box)",
    )
    console_i.add_argument("--config-dir", default="/etc/relkit-console")
    console_i.add_argument("--addr", default="127.0.0.1:8081")
    console_i.add_argument("--user", default="relkit")
    console_i.add_argument("--prefix", default="/usr/local/bin")

    migrate = sub.add_parser(
        "migrate-serve",
        help="retire a live relkit-serve.service box into relkit-store (ADR 0016 step 4)",
    )
    migrate.add_argument("--host", required=True)
    migrate.add_argument("--store-binary", required=True)
    migrate.add_argument("--user", default="relkit")
    migrate.add_argument("--prefix", default="/usr/local/bin")

    upgrade = sub.add_parser("upgrade", help="upgrade an already-running host")
    upgrade.add_argument("--host", required=True)
    upgrade.add_argument("--plan", action="store_true")
    upgrade.add_argument("--apply", action="store_true")
    upgrade.add_argument("--restart", action="store_true", help="deprecated: apply already restarts unless --stage-only")
    upgrade.add_argument("--stage-only", action="store_true", help="write files but do not restart; not a completed upgrade")
    upgrade.add_argument("--allow-dirty", action="store_true")
    upgrade.add_argument("--unsafe-from-dist", action="store_true", help="ship existing dist/ binaries instead of building HEAD")
    upgrade.add_argument("--serve-binary", default="dist/relkit-store-linux-amd64")
    upgrade.add_argument("--agent-binary", default="dist/relkit-agent-linux-amd64")
    upgrade.add_argument("--console-binary", default="dist/relkit-console-linux-amd64")
    upgrade.add_argument(
        "--no-tools",
        action="store_true",
        help=(
            "do not build, publish, or enable publishing of the cli/updater/ops tools; "
            "only a server that never serves tools should pass this"
        ),
    )
    upgrade.add_argument(
        "--no-console",
        action="store_true",
        help="skip relkit-console even when the host runs it",
    )
    upgrade.add_argument(
        "--serve-listen-addr",
        help="explicit relkit-store listen address (for example :8080)",
    )
    upgrade.add_argument(
        "--public-upload-url",
        help="CI- and agent-reachable relkit-compatible write endpoint",
    )
    upgrade.add_argument("--user", default="relkit")
    upgrade.add_argument("--prefix", default="/usr/local/bin")
    upgrade.add_argument("--serve-only", action="store_true")
    upgrade.add_argument("--agent-only", action="store_true")
    upgrade.add_argument(
        "--root-key-id",
        default="",
        help="keyId of the server's one root; written into the agent config on the epoch upgrade",
    )
    upgrade.add_argument(
        "--custody-manifest",
        default="",
        help="custody manifest filename the agent reads its keys from (default custody-manifest.json beside the config)",
    )
    upgrade.add_argument(
        "--admin-state-file",
        default="",
        help=(
            "path of the agent admin document "
            "(default <stateDir>/admin.json, and <configDir>/relkit-agent-state/admin.json "
            "when stateDir is unset)"
        ),
    )
    upgrade.add_argument(
        "--keep-backups",
        type=int,
        default=5,
        metavar="N",
        help="keep the newest N upgrade snapshots under /var/backups/relkit and delete older ones (default 5)",
    )
    upgrade.add_argument(
        "--no-prune-backups",
        action="store_true",
        help="keep every snapshot; disables rotation for this run",
    )

    prune = sub.add_parser("prune-backups", help="rotate existing upgrade snapshots on a host")
    prune.add_argument("--host", required=True)
    prune.add_argument("--keep", type=int, default=5, metavar="N", help="keep the newest N snapshots (default 5)")
    prune.add_argument("--dry-run", action="store_true", help="list what would be removed, delete nothing")

    ver = sub.add_parser("version", help="print VERSION.json (relkit SSOT)")
    ver.add_argument(
        "--field",
        choices=("number", "version", "build", "tag"),
        default="number",
        help="number=x.y.z, version=x.y.z+build, tag=v{number}",
    )
    ver.add_argument(
        "--check-tag",
        metavar="TAG",
        help="require TAG to equal v{number} from VERSION.json",
    )

    tools = sub.add_parser("tools", help="publish relkit tools to an agent (requirement 12.6)")
    tools_sub = tools.add_subparsers(dest="tools_cmd", required=True)
    tools_publish = tools_sub.add_parser(
        "publish",
        help="verify every tool against the official release, upload, then let the server sign",
    )
    tools_publish.add_argument("--server", required=True, help="agent base URL, for example https://updates.example")
    tools_publish.add_argument("--dist", default="dist", help="directory holding the built tool files (default dist)")
    tools_publish.add_argument(
        "--release-manifest",
        default="",
        help="official relkit.release/1 manifest (default <dist>/manifest.json)",
    )
    tools_publish.add_argument("--version", default="", help="require the release version to equal this")
    tools_publish.add_argument("--token", default="", help="deploy token; defaults to RELKIT_TOOLS_TOKEN")
    tools_publish.add_argument("--dry-run", action="store_true", help="print the plan, upload nothing")

    remote = sub.add_parser("remote", help=argparse.SUPPRESS)
    remote.add_argument("remote_cmd", choices=["probe", "apply", "migrate-serve", "prune-backups"])
    remote.add_argument("--spec")
    remote.add_argument("--keep", type=int)
    remote.add_argument("--dry-run", action="store_true")
    remote.add_argument("--backup-root")

    return parser


def main(argv: Optional[Sequence[str]] = None) -> int:
    ensure_bootstrap()
    parser = build_parser()
    args = parser.parse_args(argv)
    try:
        if args.cmd == "build":
            cmd_build(args)
        elif args.cmd == "install":
            if args.component == "serve":
                cmd_install_serve(args)
            elif args.component == "console":
                cmd_install_console(args)
            else:
                cmd_install_agent(args)
        elif args.cmd == "upgrade":
            cmd_upgrade(args)
        elif args.cmd == "migrate-serve":
            cmd_migrate_serve(args)
        elif args.cmd == "prune-backups":
            cmd_prune_backups(args)
        elif args.cmd == "version":
            cmd_version(args)
        elif args.cmd == "tools":
            cmd_tools(args)
        elif args.cmd == "remote":
            cmd_remote(args)
        else:
            die("unknown command")
        return 0
    except Fail as exc:
        die(str(exc))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
