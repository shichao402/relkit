#!/usr/bin/env python3
"""Unified relkit deploy CLI: bootstrap, build, install, upgrade, token.

Stdlib only unless deploy/requirements.txt lists packages. In that case this
file creates deploy/.venv, pip-installs, and re-execs itself.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Optional, Sequence

DEPLOY_DIR = Path(__file__).resolve().parent
REPO_ROOT = DEPLOY_DIR.parent
REQUIREMENTS = DEPLOY_DIR / "requirements.txt"
VENV_DIR = DEPLOY_DIR / ".venv"
BOOTSTRAP_ENV = "RELKIT_DEPLOY_BOOTSTRAPPED"
MIN_PY = (3, 9)

# Ensure sibling imports work when copied to /tmp on a remote host.
if str(DEPLOY_DIR) not in sys.path:
    sys.path.insert(0, str(DEPLOY_DIR))

from relkit_ops import (  # noqa: E402
    dump_json,
    ensure_cas_grace,
    extract_export,
    load_json_object,
    loopback_base,
    migrate_agent_config,
    migrate_profile,
    parse_exec_config,
    parse_listen_port,
    parse_requirements,
    read_write_paths,
    redact_text,
    redact_value,
    render_agent_unit,
    render_serve_unit,
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
    print("==> bootstrap: creating deploy/.venv and installing requirements")
    subprocess.run([sys.executable, "-m", "ensurepip", "--upgrade"], check=False)
    VENV_DIR.mkdir(parents=True, exist_ok=True)
    created = subprocess.run(
        [sys.executable, "-m", "venv", str(VENV_DIR)],
        check=False,
    )
    if created.returncode != 0 or not py.is_file():
        die(
            "could not create deploy/.venv (python3-venv / ensurepip missing). "
            "Install a full Python 3.9+ and retry."
        )
    pip = subprocess.run(
        [str(py), "-m", "pip", "install", "-r", str(REQUIREMENTS)],
        check=False,
    )
    if pip.returncode != 0:
        die("pip install -r deploy/requirements.txt failed")
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


def git_stamp(version: str) -> str:
    completed = run(
        ["git", "rev-parse", "--short", "HEAD"],
        cwd=REPO_ROOT,
        check=False,
        capture=True,
    )
    commit = (completed.stdout or "").strip() or "unknown"
    return f"{version}+{commit}"


def cmd_build(args: argparse.Namespace) -> None:
    require_cmd("go")
    out_dir = Path(args.out)
    if not out_dir.is_absolute():
        out_dir = REPO_ROOT / out_dir
    out_dir.mkdir(parents=True, exist_ok=True)
    targets = []
    if args.serve:
        targets.append("serve")
    if args.agent:
        targets.append("agent")
    if args.cli:
        targets.append("cli")
    if not targets:
        targets = ["serve", "agent"]
    stamp = git_stamp(args.version)
    platforms = [
        ("linux", "amd64"),
        ("linux", "arm64"),
        ("windows", "amd64"),
        ("darwin", "arm64"),
    ]
    if args.os:
        platforms = [p for p in platforms if p[0] == args.os]
    if args.arch:
        platforms = [p for p in platforms if p[1] == args.arch]
    if not platforms:
        die("no build platforms left after --os/--arch filters")
    pkgs = {
        "serve": ("./cmd/relkit-serve", "relkit-serve"),
        "agent": ("./cmd/relkit-agent", "relkit-agent"),
        "cli": ("./cmd/relkit", "relkit"),
    }
    step(f"build {stamp}")
    for kind in targets:
        pkg, prefix = pkgs[kind]
        ldflags = f"-s -w -X main.version={stamp}"
        for os_name, arch in platforms:
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
    print(f"output in {out_dir}")


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
    wait_health(local_base, "relkit-serve")
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
    payload = b"relkit-serve probe"
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
    template = (DEPLOY_DIR / "relkit-serve.service").read_text(encoding="utf-8")
    unit = render_serve_unit(
        template,
        user=user,
        prefix=prefix,
        config_path=config_path,
        read_write_paths=read_write_paths(serve_cfg),
        addr=str(serve_cfg.get("addr") or ""),
    )
    dest = Path("/etc/systemd/system/relkit-serve.service")
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
    config_path = config_dir / "relkit-serve.json"
    token_path = config_dir / "relkit-serve.token"
    dest_bin = Path(prefix) / "relkit-serve"

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
    for extra in (directory / ".relkit-serve-admin.json", config_dir / "admin.json"):
        if extra.is_file():
            os.chmod(extra, 0o600)
            chown_path(extra, user)

    cfg = load_json_object(config_path.read_text(encoding="utf-8"))
    write_serve_unit(user=user, prefix=prefix, config_path=str(config_path), serve_cfg=cfg)
    run(["systemctl", "daemon-reload"])
    run(["systemctl", "enable", "relkit-serve"], check=False)
    run(["systemctl", "restart", "relkit-serve"])
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
        shutil.copy2(DEPLOY_DIR / "relkit-agent.example.json", config_path)
        os.chmod(config_path, 0o644)
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
    print("new product: relkit-agent init -config ... -product <id>")


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


def backup_files(backup_root: Path, files: Sequence[Path]) -> None:
    backup_root.mkdir(parents=True, exist_ok=True)
    os.chmod(backup_root, 0o700)
    for path in files:
        if not path.exists():
            continue
        dest = backup_root / path.name
        if path.is_dir():
            shutil.copytree(path, dest, dirs_exist_ok=True)
        else:
            shutil.copy2(path, dest)


def probe_local() -> dict[str, Any]:
    probe: dict[str, Any] = {"hostname": os.uname().nodename if hasattr(os, "uname") else ""}
    for unit, kind in (("relkit-serve", "serve"), ("relkit-agent", "agent")):
        show = systemd_show(unit, "FragmentPath", "User", "Group", "ExecStart", "ReadWritePaths", "ProtectSystem", "ActiveState")
        entry: dict[str, Any] = {"unit": show}
        exec_start = show.get("ExecStart") or ""
        config_path = parse_exec_config(exec_start)
        entry["configPath"] = config_path
        if config_path and Path(config_path).is_file():
            cfg = load_json_object(Path(config_path).read_text(encoding="utf-8"))
            entry["config"] = redact_value(cfg)
            entry["configRawOk"] = True
        bin_name = "relkit-serve" if kind == "serve" else "relkit-agent"
        which = shutil.which(bin_name) or f"/usr/local/bin/{bin_name}"
        ver = run([which, "-version"], check=False, capture=True)
        entry["version"] = (ver.stdout or ver.stderr or "").strip()
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


def apply_serve_upgrade(
    *,
    binary: Optional[Path],
    user: str,
    prefix: str,
    public_base_url: Optional[str],
    public_upload_url: Optional[str],
    restart: bool,
    backup_root: Path,
) -> list[str]:
    notes: list[str] = []
    show = systemd_show("relkit-serve", "ExecStart", "User", "FragmentPath")
    config_path = parse_exec_config(show.get("ExecStart") or "")
    if not config_path:
        raise Fail("cannot find relkit-serve -config from systemd")
    config_file = Path(config_path)
    token_file = config_file.parent / "relkit-serve.token"
    dest_bin = Path(prefix) / "relkit-serve"
    backup_files(backup_root / "serve", [dest_bin, config_file, Path("/etc/systemd/system/relkit-serve.service")])
    cfg = load_json_object(config_file.read_text(encoding="utf-8"))
    live_addr = cfg.get("addr")
    live_dir = cfg.get("dir")
    cfg, grace_notes = ensure_cas_grace(cfg)
    notes.extend(grace_notes)
    if cfg.get("addr") != live_addr or cfg.get("dir") != live_dir:
        raise Fail("upgrade refused to change addr/dir")
    config_file.write_text(dump_json(cfg), encoding="utf-8")
    if binary:
        install_file(binary, dest_bin, 0o755)
        notes.append(f"installed {dest_bin}")
    write_serve_unit(user=user, prefix=prefix, config_path=str(config_file), serve_cfg=cfg)
    notes.append(f"ReadWritePaths={' '.join(read_write_paths(cfg))}")
    fix_token_perms(token_file, user)
    run(["systemctl", "daemon-reload"])
    if restart:
        run(["systemctl", "restart", "relkit-serve"])
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


def apply_agent_upgrade(
    *,
    binary: Optional[Path],
    user: str,
    prefix: str,
    public_base_url: Optional[str],
    public_upload_url: Optional[str],
    serve_addr: str,
    restart: bool,
    backup_root: Path,
) -> list[str]:
    notes: list[str] = []
    show = systemd_show("relkit-agent", "ExecStart", "User")
    config_path = parse_exec_config(show.get("ExecStart") or "") or "/etc/relkit-agent/relkit-agent.json"
    config_file = Path(config_path)
    dest_bin = Path(prefix) / "relkit-agent"
    products_dir = config_file.parent / "products"
    backup_files(
        backup_root / "agent",
        [dest_bin, config_file, Path("/etc/systemd/system/relkit-agent.service"), products_dir],
    )
    cfg = load_json_object(config_file.read_text(encoding="utf-8"))
    cfg, agent_notes = migrate_agent_config(cfg)
    notes.extend(agent_notes)
    config_file.write_text(dump_json(cfg), encoding="utf-8")
    if products_dir.is_dir():
        for path in sorted(products_dir.glob("*.json")):
            profile = load_json_object(path.read_text(encoding="utf-8"))
            migrated, more = migrate_profile(
                profile,
                serve_addr=serve_addr,
                public_base_url=public_base_url,
                public_upload_url=public_upload_url,
            )
            if more:
                path.write_text(dump_json(migrated), encoding="utf-8")
                notes.extend(f"{path.name}: {item}" for item in more)
    if binary:
        install_file(binary, dest_bin, 0o755)
        notes.append(f"installed {dest_bin}")
    write_agent_unit(
        user=user,
        prefix=prefix,
        config_path=str(config_file),
        working_directory=str(cfg.get("products") and "/srv/relkit" or "/srv/relkit"),
    )
    run(["systemctl", "daemon-reload"])
    if restart:
        run(["systemctl", "restart", "relkit-agent"])
        self_check_agent(str(cfg.get("addr") or "127.0.0.1:8787"))
        notes.append("agent restarted and health-checked")
        for name, spec in (cfg.get("products") or {}).items():
            if not isinstance(spec, dict):
                continue
            check = run(
                [
                    str(dest_bin),
                    "onboard",
                    "check",
                    "-config",
                    str(config_file),
                    "-product",
                    name,
                    "-json",
                ],
                check=False,
                capture=True,
            )
            print(redact_text((check.stdout or check.stderr or "").strip()))
            if check.returncode != 0:
                notes.append(f"onboard check {name} failed (see output; not rolling back)")
            else:
                notes.append(f"onboard check {name} ok")
    else:
        notes.append("agent files updated; restart skipped")
    return notes


def rollback(backup_root: Path, prefix: str) -> None:
    serve_bin = backup_root / "serve" / "relkit-serve"
    if serve_bin.is_file():
        install_file(serve_bin, Path(prefix) / "relkit-serve", 0o755)
    serve_cfg = backup_root / "serve" / "relkit-serve.json"
    if serve_cfg.is_file():
        dest = Path("/etc/relkit-serve/relkit-serve.json")
        show = systemd_show("relkit-serve", "ExecStart")
        config_path = parse_exec_config(show.get("ExecStart") or "")
        if config_path:
            dest = Path(config_path)
        shutil.copy2(serve_cfg, dest)
    unit = backup_root / "serve" / "relkit-serve.service"
    if unit.is_file():
        shutil.copy2(unit, "/etc/systemd/system/relkit-serve.service")
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
    run(["systemctl", "daemon-reload"], check=False)
    run(["systemctl", "restart", "relkit-serve"], check=False)
    run(["systemctl", "restart", "relkit-agent"], check=False)


def cmd_remote(args: argparse.Namespace) -> None:
    must_root()
    if args.remote_cmd == "probe":
        print(json.dumps(probe_local(), ensure_ascii=False, indent=2))
        return
    if args.remote_cmd == "apply":
        spec = load_json_object(Path(args.spec).read_text(encoding="utf-8"))
        backup = Path(spec.get("backup") or f"/var/backups/relkit/{int(time.time())}")
        prefix = str(spec.get("prefix") or "/usr/local/bin")
        user = str(spec.get("user") or "relkit")
        restart = bool(spec.get("restart"))
        public_base = spec.get("publicBaseUrl")
        public_upload = spec.get("publicUploadUrl")
        serve_bin = Path(spec["serveBinary"]) if spec.get("serveBinary") else None
        agent_bin = Path(spec["agentBinary"]) if spec.get("agentBinary") else None
        notes: list[str] = []
        try:
            if spec.get("upgradeServe"):
                notes.extend(
                    apply_serve_upgrade(
                        binary=serve_bin,
                        user=user,
                        prefix=prefix,
                        public_base_url=public_base,
                        public_upload_url=public_upload,
                        restart=restart,
                        backup_root=backup,
                    )
                )
            serve_addr = ""
            show = systemd_show("relkit-serve", "ExecStart")
            cfg_path = parse_exec_config(show.get("ExecStart") or "")
            if cfg_path and Path(cfg_path).is_file():
                serve_addr = str(load_json_object(Path(cfg_path).read_text(encoding="utf-8")).get("addr") or "")
            if spec.get("upgradeAgent"):
                notes.extend(
                    apply_agent_upgrade(
                        binary=agent_bin,
                        user=user,
                        prefix=prefix,
                        public_base_url=public_base,
                        public_upload_url=public_upload,
                        serve_addr=serve_addr,
                        restart=restart,
                        backup_root=backup,
                    )
                )
        except Exception as exc:
            print(f"apply failed: {redact_text(str(exc))}", file=sys.stderr)
            rollback(backup, prefix)
            raise Fail("upgrade failed; rolled back from " + str(backup)) from exc
        print(json.dumps({"ok": True, "backup": str(backup), "notes": notes}, ensure_ascii=False, indent=2))
        return
    die(f"unknown remote command {args.remote_cmd}")


def ssh_argv(host: str, extra: Sequence[str] | None = None) -> list[str]:
    require_cmd("ssh")
    argv = ["ssh", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", host]
    if extra:
        argv.extend(extra)
    return argv


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
    scp_to(host, [DEPLOY_DIR / "relkit.py", DEPLOY_DIR / "relkit_ops.py", DEPLOY_DIR / "relkit-serve.service", DEPLOY_DIR / "relkit-agent.service"], remote_dir + "/")
    probe_out = run(
        ssh_argv(host, ["sudo", py, remote_dir + "/relkit.py", "remote", "probe"]),
        capture=True,
    )
    probe = load_json_object(probe_out.stdout or "{}")
    print(json.dumps(redact_value(probe), ensure_ascii=False, indent=2))
    serve_cfg = (probe.get("serve") or {}).get("config") or {}
    if isinstance(serve_cfg, dict):
        _, grace = ensure_cas_grace(serve_cfg)
        print("serve migrate notes:", grace or ["addr/dir preserved"])
    public = args.public_base_url
    public_upload = args.public_upload_url
    for name, profile in (probe.get("profiles") or {}).items():
        if not isinstance(profile, dict) or profile.get("error"):
            continue
        try:
            _, notes = migrate_profile(
                profile,
                serve_addr=str(serve_cfg.get("addr") or ""),
                public_base_url=public,
                public_upload_url=public_upload,
            )
            print(f"profile {name}:", notes or ["no changes"])
        except ValueError as exc:
            die(f"profile {name}: {exc}")
    if args.plan:
        print("\nplan only; no files changed. Re-run with --apply --restart to execute.")
        return
    spec = {
        "upgradeServe": not args.agent_only,
        "upgradeAgent": not args.serve_only,
        "restart": bool(args.restart),
        "prefix": args.prefix,
        "user": args.user,
        "publicBaseUrl": public,
        "publicUploadUrl": public_upload,
        "backup": f"/var/backups/relkit/{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}",
    }
    uploads: list[Path] = []
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
    if not args.restart:
        print("apply finished without --restart; systemd still runs the previous process")


def cmd_token(args: argparse.Namespace) -> None:
    if args.target == "local":
        must_root()
        if args.action == "prepare":
            extra = ["-token-only"]
            if args.product:
                extra = ["-product", args.product, "-token-only"]
            out = run(["relkit-serve", "init", "-out", args.config_dir, *extra], capture=True)
            token = extract_export("RELKIT_SERVE_TOKEN", out.stdout or "") or extract_export(
                "RELKIT_UPLOAD_TOKEN", out.stdout or ""
            )
            if not token:
                die("init did not print a token")
            print("Shown once. Deliver to publishers before --activate / restart.\n")
            env_name = "RELKIT_UPLOAD_TOKEN" if args.product else "RELKIT_SERVE_TOKEN"
            print(f"  export {env_name}='{token}'")
            print("\nDo not restart yet.")
            return
        if args.action == "activate":
            run(["systemctl", "restart", "relkit-serve"])
            print("relkit-serve restarted; old operator/product token is now invalid")
            return
        die("pass --prepare or --activate")
    host = args.host
    if not host:
        die("--host is required unless --local")
    remote_python(host)
    remote_dir = "/tmp/relkit-deploy"
    run(ssh_argv(host, ["mkdir", "-p", remote_dir]))
    scp_to(host, [DEPLOY_DIR / "relkit.py", DEPLOY_DIR / "relkit_ops.py"], remote_dir + "/")
    if args.action == "prepare":
        extra = ""
        if args.product:
            extra = f" -product {args.product}"
        out = run(
            ssh_argv(
                host,
                [
                    "sudo",
                    "relkit-serve",
                    "init",
                    "-out",
                    args.config_dir,
                    *([item for item in extra.split() if item] + ["-token-only"]),
                ],
            ),
            capture=True,
        )
        print(redact_text(out.stdout or ""))
        token = extract_export("RELKIT_SERVE_TOKEN", out.stdout or "") or extract_export(
            "RELKIT_UPLOAD_TOKEN", out.stdout or ""
        )
        if token:
            env_name = "RELKIT_UPLOAD_TOKEN" if args.product else "RELKIT_SERVE_TOKEN"
            print("Shown once. Deliver before --activate.\n")
            print(f"  export {env_name}='{token}'")
        return
    if args.action == "activate":
        run(ssh_argv(host, ["sudo", "systemctl", "restart", "relkit-serve"]))
        print("relkit-serve restarted on", host)
        return
    die("pass --prepare or --activate")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="deploy/relkit.py", description="relkit deploy CLI")
    sub = parser.add_subparsers(dest="cmd", required=True)

    build = sub.add_parser("build", help="cross-compile binaries")
    build.add_argument("--serve", action="store_true")
    build.add_argument("--agent", action="store_true")
    build.add_argument("--cli", action="store_true")
    build.add_argument("--version", default="0.2.1")
    build.add_argument("--out", default="dist")
    build.add_argument("--os")
    build.add_argument("--arch")

    install = sub.add_parser("install", help="first-time systemd install")
    install_sub = install.add_subparsers(dest="component", required=True)
    serve_i = install_sub.add_parser("serve")
    serve_i.add_argument("--binary", required=True)
    serve_i.add_argument("--dir", default="/srv/releases")
    serve_i.add_argument("--config-dir", default="/etc/relkit-serve")
    serve_i.add_argument("--addr", default="127.0.0.1:30341")
    serve_i.add_argument("--user", default="relkit")
    serve_i.add_argument("--prefix", default="/usr/local/bin")
    serve_i.add_argument("--rotate-token", action="store_true")
    serve_i.add_argument("--force-reconfigure", action="store_true")
    agent_i = install_sub.add_parser("agent")
    agent_i.add_argument("--binary", required=True)
    agent_i.add_argument("--config-dir", default="/etc/relkit-agent")
    agent_i.add_argument("--state-dir", default="/var/lib/relkit-agent")
    agent_i.add_argument("--product-root", default="/srv/relkit")
    agent_i.add_argument("--user", default="relkit")
    agent_i.add_argument("--prefix", default="/usr/local/bin")

    upgrade = sub.add_parser("upgrade", help="upgrade an already-running host")
    upgrade.add_argument("--host", required=True)
    upgrade.add_argument("--plan", action="store_true")
    upgrade.add_argument("--apply", action="store_true")
    upgrade.add_argument("--restart", action="store_true")
    upgrade.add_argument("--serve-binary", default="dist/relkit-serve-linux-amd64")
    upgrade.add_argument("--agent-binary", default="dist/relkit-agent-linux-amd64")
    upgrade.add_argument("--public-base-url")
    upgrade.add_argument(
        "--public-upload-url",
        help="CI- and agent-reachable relkit-compatible write endpoint",
    )
    upgrade.add_argument("--user", default="relkit")
    upgrade.add_argument("--prefix", default="/usr/local/bin")
    upgrade.add_argument("--serve-only", action="store_true")
    upgrade.add_argument("--agent-only", action="store_true")

    token = sub.add_parser("token", help="explicit operator/product token rotate")
    token.add_argument("--host")
    token.add_argument("--local", dest="target", action="store_const", const="local")
    token.add_argument("--prepare", dest="action", action="store_const", const="prepare")
    token.add_argument("--activate", dest="action", action="store_const", const="activate")
    token.add_argument("--product")
    token.add_argument("--config-dir", default="/etc/relkit-serve")

    remote = sub.add_parser("remote", help=argparse.SUPPRESS)
    remote.add_argument("remote_cmd", choices=["probe", "apply"])
    remote.add_argument("--spec")

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
            else:
                cmd_install_agent(args)
        elif args.cmd == "upgrade":
            cmd_upgrade(args)
        elif args.cmd == "token":
            if not getattr(args, "action", None):
                die("token requires --prepare or --activate")
            if not getattr(args, "target", None):
                args.target = "remote"
            cmd_token(args)
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
