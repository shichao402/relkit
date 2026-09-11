#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Product-repo ops gate for relkit.

Copy the whole ``scripts/host/`` tree byte-for-byte. This file is the only
command people and agents run in a product repo. It never writes serve/agent
JSON; machine config is owned by the binaries' ``init`` (internal).
"""

from __future__ import annotations

import argparse
import fnmatch
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import traceback
from pathlib import Path
from typing import Any, Callable, Optional, Sequence
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

SCHEMA = "relkit.onboarding/1"
LOCAL_SCHEMA = "relkit.onboarding.local/1"
LOCK_SCHEMA = "relkit.consume/2"
DEFAULT_SERVE_DIR = "/etc/relkit-serve"
DEFAULT_AGENT_CONFIG = "/etc/relkit-agent/relkit-agent.json"
GITHUB_REPO = os.environ.get("RELKIT_RELEASE_REPO", "shichao402/relkit")
TOKEN_ENV = "RELKIT_UPLOAD_TOKEN"
SECRET_NOTE = Path(".secrets") / "project" / "relkit-upload-token"
AGENT_SECRET_NOTE = Path(".secrets") / "project" / "relkit-agent-upload-token"
GITIGNORE_RELKIT = (
    ".relkit/onboarding.local.json",
    ".relkit/onboarding.md",
    ".relkit/artifacts/",
    ".relkit-keys/*.private.pb",
)
PUBLISH_PROTOCOL_FALLBACK = 2
UPDATER_PROCESS_VALUES = ("rust-shell", "node", "dart", "go", "other")
UPDATER_PROCESS_EXPLAIN = (
    "谁调用 Updater.open。只记封闭词，不要另写决策备忘。"
    " rust-shell：壳拥有更新生命周期，WebView 只走产品仓窄桥；relkit 到 sidecar 阶段才补 Rust facade。"
    " node/dart/go：该语言进程直连 sidecar；壳若只画 UI 选 node。"
    " other：先口头说明再记。"
    " 开工窄桥的时机是 sidecar.layout 已定、fake.release 之前。"
)

STATUSES = (
    "unanswered",
    "confirmed",
    "applied",
    "verified",
    "blocked",
    "stale",
    "drift",
)

STEP_IDS = (
    "repo.root",
    "product.id",
    "updater.process",
    "channel.ssot",
    "backend.kind",
    "ssh.host",
    "ssh.config_dir",
    "token.isolation",
    "serve.register",
    "agent.register",
    "signing.keys",
    "consume.lock",
    "sidecar.layout",
    "fake.release",
    "pack.ci",
)

REQUIRED_FOR_RELEASE = STEP_IDS[:-1]
DECISION_STEPS = STEP_IDS[:8]
ACTION_STEPS = STEP_IDS[8:]


class Fail(Exception):
    pass


def force_utf8_stdio() -> None:
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if reconfigure is not None:
            try:
                reconfigure(encoding="utf-8", errors="replace")
            except (ValueError, OSError):
                pass


def host_root(script_file: Optional[Path] = None) -> Path:
    path = (script_file or Path(__file__)).resolve()
    parent = path.parent
    if parent.name == "host" and parent.parent.name == "scripts":
        return parent.parent.parent
    if parent.name == "scripts":
        return parent.parent
    return parent.parent


def host_scripts_dir(script_file: Optional[Path] = None) -> Path:
    return (script_file or Path(__file__)).resolve().parent


def tree_sha256(directory: Path) -> str:
    digest = hashlib.sha256()
    for path in sorted(directory.rglob("*")):
        if not path.is_file():
            continue
        if "__pycache__" in path.parts or path.suffix == ".pyc":
            continue
        rel = path.relative_to(directory).as_posix().encode("utf-8")
        digest.update(rel)
        digest.update(b"\0")
        digest.update(path.read_bytes())
        digest.update(b"\0")
    return digest.hexdigest()


def relkit_dir(root: Path) -> Path:
    return root / ".relkit"


def state_path(root: Path) -> Path:
    return relkit_dir(root) / "onboarding.json"


def projection_path(root: Path) -> Path:
    return relkit_dir(root) / "onboarding.md"


def local_path(root: Path) -> Path:
    return relkit_dir(root) / "onboarding.local.json"


def empty_steps() -> dict[str, Any]:
    return {step: {"status": "unanswered", "value": None, "note": ""} for step in STEP_IDS}


def default_state(root: Path) -> dict[str, Any]:
    return {
        "schema": SCHEMA,
        "product": None,
        "steps": empty_steps(),
        "serve": {
            "sshHost": None,
            "configDir": DEFAULT_SERVE_DIR,
            "shareWith": None,
            "remoteVersion": None,
        },
        "agent": {
            "sshHost": None,
            "configPath": DEFAULT_AGENT_CONFIG,
            "remoteVersion": None,
        },
        "root": str(root),
    }


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def dump_json(data: dict[str, Any]) -> str:
    return json.dumps(data, indent=2, ensure_ascii=False) + "\n"


def load_state(root: Path) -> dict[str, Any]:
    path = state_path(root)
    if not path.is_file():
        return default_state(root)
    data = load_json(path)
    if data.get("schema") != SCHEMA:
        raise Fail(f"{path} must use schema {SCHEMA!r}")
    steps = empty_steps()
    raw = data.get("steps") or {}
    if isinstance(raw, dict):
        for key, value in raw.items():
            if key in steps and isinstance(value, dict):
                steps[key] = {
                    "status": value.get("status") or "unanswered",
                    "value": value.get("value"),
                    "note": value.get("note") or "",
                }
    data["steps"] = steps
    data.setdefault("serve", default_state(root)["serve"])
    data.setdefault("agent", default_state(root)["agent"])
    return data


def render_onboarding_md(state: dict[str, Any]) -> str:
    product = state.get("product") or "(unset)"
    lines = [
        "# relkit onboarding",
        "",
        "Generated from `.relkit/onboarding.json`. Do not edit.",
        "Regenerate: `python scripts/host/relkit_host.py status`.",
        "Reset: `python scripts/host/relkit_host.py onboard reset --yes`.",
        "",
        f"product: `{product}`",
        "",
        "| step | status | value | note |",
        "| --- | --- | --- | --- |",
    ]
    for step_id in STEP_IDS:
        item = (state.get("steps") or {}).get(step_id) or {}
        value = item.get("value")
        if value in (None, ""):
            value = "—"
        note = item.get("note") or "—"
        status = item.get("status") or "unanswered"
        lines.append(f"| `{step_id}` | {status} | {value} | {note} |")
    unresolved = next_unresolved_step(state) if state.get("steps") else None
    lines.extend(["", "## next", ""])
    if unresolved:
        lines.append(f"`{unresolved}`")
        lines.append("")
        lines.append(explain_step(unresolved))
    else:
        lines.append("no unresolved step; run verify")
    lines.append("")
    return "\n".join(lines)


def write_onboarding_md(root: Path, state: dict[str, Any]) -> None:
    relkit_dir(root).mkdir(parents=True, exist_ok=True)
    projection_path(root).write_text(render_onboarding_md(state), encoding="utf-8")


def save_state(root: Path, data: dict[str, Any]) -> None:
    relkit_dir(root).mkdir(parents=True, exist_ok=True)
    state_path(root).write_text(dump_json(data), encoding="utf-8")
    write_onboarding_md(root, data)


def load_local(root: Path) -> dict[str, Any]:
    path = local_path(root)
    if not path.is_file():
        return {"schema": LOCAL_SCHEMA, "evidence": {}, "secretRefs": []}
    data = load_json(path)
    data.setdefault("schema", LOCAL_SCHEMA)
    data.setdefault("evidence", {})
    data.setdefault("secretRefs", [])
    return data


def save_local(root: Path, data: dict[str, Any]) -> None:
    relkit_dir(root).mkdir(parents=True, exist_ok=True)
    local_path(root).write_text(dump_json(data), encoding="utf-8")


def ensure_gitignore(root: Path) -> None:
    path = root / ".gitignore"
    existing = path.read_text(encoding="utf-8") if path.is_file() else ""
    lines = existing.splitlines()
    missing = [item for item in GITIGNORE_RELKIT if item not in lines and item not in existing]
    if not missing:
        return
    with path.open("a", encoding="utf-8") as handle:
        if existing and not existing.endswith("\n"):
            handle.write("\n")
        for item in missing:
            handle.write(item + "\n")


def redact_text(text: str) -> str:
    text = re.sub(
        r"(export\s+(?:RELKIT_UPLOAD_TOKEN|RELKIT_SERVE_TOKEN|RELKIT_ADMIN_BOOTSTRAP)=)(['\"]?)([^'\"\s]+)\2",
        r"\1\2<redacted>\2",
        text,
    )
    text = re.sub(r"(Bearer\s+)\S+", r"\1<redacted>", text, flags=re.I)
    return text


def extract_export(name: str, text: str) -> Optional[str]:
    match = re.search(
        rf"export\s+{re.escape(name)}=(['\"]?)([^'\"\s]+)\1",
        text,
    )
    if not match:
        return None
    return match.group(2)


def set_step(
    state: dict[str, Any],
    step_id: str,
    status: str,
    value: Any = None,
    note: str = "",
    mark_later_stale: bool = False,
) -> None:
    if step_id not in STEP_IDS:
        raise Fail(f"unknown step {step_id}")
    if status not in STATUSES:
        raise Fail(f"unknown status {status}")
    previous = state["steps"][step_id]
    if value is None:
        value = previous.get("value")
    state["steps"][step_id] = {"status": status, "value": value, "note": note}
    if mark_later_stale:
        started = False
        for later in STEP_IDS:
            if later == step_id:
                started = True
                continue
            if started and state["steps"][later]["status"] in (
                "confirmed",
                "applied",
                "verified",
            ):
                state["steps"][later]["status"] = "stale"


def step_value(state: dict[str, Any], step_id: str) -> Any:
    return state["steps"][step_id].get("value")


def product_id(state: dict[str, Any]) -> str:
    value = state.get("product") or step_value(state, "product.id")
    if not value:
        raise Fail("product.id is unanswered; run onboard set product.id <id>")
    return str(value)


def detect_stack(root: Path) -> dict[str, Any]:
    rust_manifests = list(root.glob("Cargo.toml"))
    if not rust_manifests:
        rust_manifests = list(root.glob("src-tauri/Cargo.toml"))
    if not rust_manifests:
        rust_manifests = list(root.glob("packages/*/src-tauri/Cargo.toml"))
    rust = bool(rust_manifests)
    node = (root / "package.json").is_file()
    dart = (root / "pubspec.yaml").is_file()
    go = (root / "go.mod").is_file()
    kind = []
    if rust:
        kind.append("rust")
    if node:
        kind.append("node")
    if dart:
        kind.append("dart")
    if go:
        kind.append("go")
    updater = None
    if rust and node:
        updater = "choose rust-shell (shell owns Updater.open) or node (shell is UI-only)"
    elif node:
        updater = "node"
    elif dart:
        updater = "dart"
    elif go:
        updater = "go"
    return {"languages": kind, "updater": updater, "root": str(root)}


def recommend(root: Path, state: dict[str, Any], step_id: str) -> str:
    stack = detect_stack(root)
    relkit_json = root / "relkit.json"
    product = None
    if relkit_json.is_file():
        try:
            product = load_json(relkit_json).get("product")
        except (OSError, json.JSONDecodeError, TypeError):
            product = None
    mapping = {
        "repo.root": f"{root}  stack={','.join(stack['languages']) or 'unknown'}",
        "product.id": str(product or re.sub(r"[^a-z0-9._-]+", "-", root.name.lower()).strip("-")),
        "updater.process": stack["updater"] or "ask the user which process owns Updater.open",
        "channel.ssot": (
            "migrate VERSION to VERSION.json via relkit version"
            if (root / "VERSION").is_file() and not (root / "VERSION.json").is_file()
            else "VERSION.json via relkit version"
        ),
        "backend.kind": "choose: intranet-relkit-compatible / s3-compatible / static-http",
        "ssh.host": ssh_host_recommend(),
        "ssh.config_dir": DEFAULT_SERVE_DIR + " (confirm against journal config: line)",
        "token.isolation": "exclusive (pass --share-with only when the user names an existing id)",
        "serve.register": "relkit_host.py serve add --execute after ssh.host is confirmed",
        "agent.register": "relkit_host.py agent add --execute after serve.register",
        "signing.keys": "relkit keygen on this repo; private key stays out of git",
        "consume.lock": "relkit_host.py install after scripts/relkit.lock.json is pinned",
        "sidecar.layout": "tools/bin updater sidecar next to the process that calls Updater.open",
        "fake.release": "stage a dummy zip, simulate, verify; do not publish until pack.ci",
        "pack.ci": "product packaging + CI calling this script's release --execute",
    }
    return mapping[step_id]


def explain_step(step_id: str) -> str:
    texts = {
        "repo.root": "Confirm the product repository root and languages. No mutation.",
        "product.id": "Stable product id used by serve/agent tokens and relkit.json.",
        "updater.process": UPDATER_PROCESS_EXPLAIN,
        "channel.ssot": "VERSION.json is the version SSOT; host.py/CI call relkit version, people do not.",
        "backend.kind": "Where bits live. Intranet products share a serve host with a new product id.",
        "ssh.host": "OpenSSH Host name from ~/.ssh/config. This script never picks one.",
        "ssh.config_dir": "Directory the running unit actually reads (journal config: line).",
        "token.isolation": "exclusive mints a new token. share-with must be explicit.",
        "serve.register": "SSH to the already-installed binary's init. This script does not edit JSON.",
        "agent.register": "Register a publish profile for this product on the agent host.",
        "signing.keys": "Keygen + embed public keys. Private keys are not committed.",
        "consume.lock": "relkit.consume/2 lock pins release URLs, hashes, and scripts/host tree hash.",
        "sidecar.layout": "relkit-updater binary layout the host pack must keep.",
        "fake.release": "Prove stage/simulate/verify before a real pack.",
        "pack.ci": "Real artifacts and CI. Optional for the first wiring, required for shipping.",
    }
    if step_id not in texts:
        raise Fail(f"unknown step {step_id}")
    return texts[step_id]


def choice_hint(root: Path, step_id: str) -> str:
    hints = {
        "product.id": "稳定的小写 ID，例如 loom；发布后不要随意改",
        "updater.process": " / ".join(UPDATER_PROCESS_VALUES),
        "channel.ssot": "migrate:VERSION->VERSION.json / VERSION.json / custom",
        "backend.kind": "intranet-relkit-compatible / s3-compatible / static-http",
        "ssh.host": ssh_host_recommend(),
        "ssh.config_dir": "运行中服务日志实际打印的配置目录",
        "token.isolation": "exclusive / share-with:<existing-product>",
    }
    return hints.get(step_id, "")


def recommended_value(root: Path, state: dict[str, Any], step_id: str) -> Optional[str]:
    stack = detect_stack(root)
    values = {
        "product.id": re.sub(r"[^a-z0-9._-]+", "-", root.name.lower()).strip("-"),
        "updater.process": (
            None
            if "rust" in stack["languages"] and "node" in stack["languages"]
            else stack["updater"]
        ),
        "channel.ssot": (
            "migrate:VERSION->VERSION.json"
            if (root / "VERSION").is_file() and not (root / "VERSION.json").is_file()
            else "VERSION.json"
        ),
        "ssh.config_dir": DEFAULT_SERVE_DIR,
        "token.isolation": "exclusive",
    }
    value = values.get(step_id)
    return str(value) if value else None


def next_unresolved_step(state: dict[str, Any]) -> Optional[str]:
    return next(
        (
            step
            for step in STEP_IDS
            if state["steps"][step]["status"] in ("unanswered", "stale", "blocked", "drift")
        ),
        None,
    )


def print_decision(root: Path, state: dict[str, Any], step_id: str) -> None:
    print(f"\n待决策: {step_id}")
    print(f"说明: {explain_step(step_id)}")
    print(f"推荐: {recommend(root, state, step_id)}")
    hint = choice_hint(root, step_id)
    if hint:
        print(f"可选: {hint}")


def normalize_interactive_value(step_id: str, raw: str) -> tuple[str, Optional[str]]:
    value = raw.strip()
    if step_id == "token.isolation" and value.startswith("share-with:"):
        shared = value.split(":", 1)[1].strip()
        if not shared:
            raise Fail("share-with 后必须给已有 product id")
        return "share-with", shared
    return value, None


def run_interactive_wizard(
    root: Path,
    *,
    input_fn: Callable[[str], str] = input,
) -> int:
    """Ask every policy question; never silently accept a recommendation."""
    while True:
        state = load_state(root)
        step_id = next_unresolved_step(state)
        if step_id is None:
            print("开箱状态没有未决项。运行 verify 对账后再 release。")
            return 0
        if step_id in ACTION_STEPS:
            print_decision(root, state, step_id)
            print("这是执行步骤，不用文字确认冒充完成。")
            print("运行上面建议的显式子命令；有远端写入时还需 --execute，重启还需 --restart。")
            return 0
        print_decision(root, state, step_id)
        answer = input_fn("输入选择；r=采用推荐，?=重看说明，q=保存并退出: ").strip()
        if answer.lower() == "q":
            print("已保存当前进度。")
            return 0
        if answer == "?":
            continue
        if answer.lower() == "r":
            selected = recommended_value(root, state, step_id)
            if selected is None:
                print("这一项没有可安全代选的单值推荐，请输入你的选择。")
                continue
            answer = selected
        if not answer:
            print("空输入不会确认。")
            continue
        value, share_with = normalize_interactive_value(step_id, answer)
        cmd_onboard_set(root, step_id, value, share_with)


def _expand_ssh_path(raw: str) -> Path:
    return Path(os.path.expanduser(os.path.expandvars(raw)))


def parse_ssh_config(
    config_path: Optional[Path] = None,
    *,
    _seen: Optional[set[Path]] = None,
) -> tuple[list[str], list[str]]:
    path = (config_path or (Path.home() / ".ssh" / "config")).resolve()
    seen = _seen if _seen is not None else set()
    if path in seen or not path.is_file():
        return [], []
    seen.add(path)
    exact: list[str] = []
    patterns: list[str] = []
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        lower = stripped.lower()
        if lower.startswith("include "):
            for item in stripped.split()[1:]:
                nested = _expand_ssh_path(item)
                more_exact, more_patterns = parse_ssh_config(nested, _seen=seen)
                for name in more_exact:
                    if name not in exact:
                        exact.append(name)
                for name in more_patterns:
                    if name not in patterns:
                        patterns.append(name)
            continue
        if not lower.startswith("host "):
            continue
        for token in stripped.split()[1:]:
            if any(ch in token for ch in "*?!"):
                if token not in patterns:
                    patterns.append(token)
            elif token not in exact:
                exact.append(token)
    return exact, patterns


def list_ssh_hosts(config_path: Optional[Path] = None) -> list[str]:
    exact, _patterns = parse_ssh_config(config_path)
    return exact


def ssh_host_allowed(name: str, config_path: Optional[Path] = None) -> bool:
    exact, patterns = parse_ssh_config(config_path)
    if name in exact:
        return True
    for pattern in patterns:
        if pattern == "*":
            continue
        if fnmatch.fnmatch(name, pattern):
            return True
    return False


def ssh_host_recommend(config_path: Optional[Path] = None) -> str:
    exact, patterns = parse_ssh_config(config_path)
    bits: list[str] = []
    if exact:
        bits.append("candidates: " + ", ".join(exact))
    useful = [item for item in patterns if item != "*"]
    if useful:
        bits.append("patterns: " + ", ".join(useful))
    return "; ".join(bits) if bits else "no Host entries in ~/.ssh/config"


def ssh_run(
    host: str,
    remote: Sequence[str],
    *,
    timeout: int = 60,
) -> subprocess.CompletedProcess[str]:
    if not host:
        raise Fail("SSH Host is required")
    argv = ["ssh", "-o", "BatchMode=yes", host, "--", *remote]
    try:
        result = subprocess.run(
            argv,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            timeout=timeout,
            check=False,
        )
    except FileNotFoundError as error:
        raise Fail("ssh is not installed") from error
    except subprocess.TimeoutExpired as error:
        raise Fail(f"SSH to {host} timed out") from error
    if result.returncode != 0:
        stderr = redact_text((result.stderr or result.stdout or "").strip())
        raise Fail(
            f"SSH to {host} failed (exit {result.returncode}). "
            f"Stop. Do not switch credentials. {stderr}"
        )
    return result


def parse_list_products(text: str) -> list[str]:
    products: list[str] = []
    in_tokens = False
    for line in text.splitlines():
        stripped = line.strip()
        if stripped.startswith("uploadTokens"):
            in_tokens = True
            continue
        if stripped.startswith("products") and not stripped.startswith("uploadTokens"):
            in_tokens = False
            continue
        if not in_tokens:
            continue
        if not stripped or stripped.lower().endswith("none"):
            continue
        first = stripped.split()[0]
        for item in first.split(","):
            if item and item not in products:
                products.append(item)
    if not products:
        for match in re.finditer(r"^\s+([A-Za-z0-9._-]+)\s+\S+", text, re.M):
            name = match.group(1)
            if name not in ("config", "uploadTokens", "products") and name not in products:
                products.append(name)
    return products


def parse_agent_products(text: str) -> list[str]:
    products: list[str] = []
    in_products = False
    for line in text.splitlines():
        stripped = line.strip()
        if stripped.startswith("products"):
            in_products = True
            if stripped.lower().endswith("none"):
                return []
            continue
        if not in_products:
            continue
        if not stripped:
            continue
        name = stripped.split()[0]
        if name not in products:
            products.append(name)
    return products


def require_execute(args: argparse.Namespace, action: str) -> None:
    if not getattr(args, "execute", False):
        raise Fail(f"refusing to {action} without --execute")


def require_restart_flag(args: argparse.Namespace) -> None:
    if not getattr(args, "restart", False):
        raise Fail(
            "config changed but the process was not restarted. "
            "Pass --restart only after the user said the machine may restart."
        )


def write_secret(root: Path, token: str, *, note: Path = SECRET_NOTE) -> Path:
    path = root / note
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(f"export {TOKEN_ENV}='{token}'\n", encoding="utf-8")
    try:
        path.chmod(0o600)
    except OSError:
        pass
    local = load_local(root)
    ref = str(note).replace("\\", "/")
    if ref not in local["secretRefs"]:
        local["secretRefs"].append(ref)
    save_local(root, local)
    return path


def archive_secret(root: Path) -> None:
    path = root / SECRET_NOTE
    if not path.is_file():
        return
    archived = path.with_name(path.name + ".revoked")
    path.replace(archived)


def relkit_bin(root: Path, explicit: Optional[str] = None) -> Path:
    if explicit:
        path = Path(explicit)
        if not path.is_file():
            raise Fail(f"relkit binary not found: {path}")
        return path
    env = os.environ.get("RELKIT_BIN")
    if env:
        path = Path(env)
        if path.is_file():
            return path
    tools = root / "tools" / "bin"
    for name in ("relkit.exe", "relkit", "relkit-linux-amd64"):
        candidate = tools / name
        if candidate.is_file():
            return candidate
    found = shutil.which("relkit")
    if found:
        return Path(found)
    raise Fail("relkit CLI is not installed; run install first or pass --bin")


def cmd_status(root: Path, as_json: bool = False) -> int:
    state = load_state(root)
    report = reconcile(root, state, write=False)
    if as_json:
        print(dump_json(report))
        return 0
    print(f"schema {report['schema']}")
    print(f"product {report.get('product') or '(unset)'}")
    print("steps")
    for step_id in STEP_IDS:
        item = report["steps"][step_id]
        value = item.get("value")
        suffix = "" if value in (None, "") else f"  {value}"
        extra = ""
        if item.get("note"):
            extra = f"  ({item['note']})"
        print(f"  {step_id:18} {item['status']}{suffix}{extra}")
    write_onboarding_md(root, report)
    unresolved = next_unresolved_step(report)
    if unresolved:
        print("next")
        print_decision(root, report, unresolved)
    drift = report.get("drift") or []
    if drift:
        print("drift")
        for item in drift:
            print(f"  {item}")
    unconfirmed = report.get("unconfirmed") or []
    if unconfirmed:
        print("unconfirmed")
        for item in unconfirmed:
            print(f"  {item}")
    return 1 if drift else 0


def reconcile(root: Path, state: dict[str, Any], *, write: bool) -> dict[str, Any]:
    drift: list[str] = []
    unconfirmed: list[str] = []
    serve = state.get("serve") or {}
    host = serve.get("sshHost")
    config_dir = serve.get("configDir") or DEFAULT_SERVE_DIR
    if host:
        try:
            version = ssh_run(host, ["relkit-serve", "-version"]).stdout.strip()
            serve["remoteVersion"] = version
            listed = ssh_run(
                host,
                ["sudo", "relkit-serve", "init", "-out", str(config_dir), "-list-products"],
            ).stdout
            products = parse_list_products(listed)
            expected = None
            try:
                expected = product_id(state)
            except Fail:
                expected = None
            local = load_local(root)
            local["evidence"]["serve.list-products"] = redact_text(listed)
            local["evidence"]["serve.version"] = version
            save_local(root, local)
            registered = state["steps"]["serve.register"]["status"] in (
                "applied",
                "verified",
            )
            if expected and registered and expected not in products:
                drift.append(
                    f"serve {host} does not list {expected}; local state says registered"
                )
                state["steps"]["serve.register"]["status"] = "drift"
            elif expected and expected in products and registered:
                state["steps"]["serve.register"]["status"] = "verified"
                state["steps"]["serve.register"]["note"] = "listed on remote"
        except Fail as error:
            unconfirmed.append(str(error))
    elif state["steps"]["serve.register"]["status"] in ("applied", "verified"):
        unconfirmed.append("serve.register is applied but ssh.host is empty")

    agent = state.get("agent") or {}
    agent_host = agent.get("sshHost") or host
    config_path = agent.get("configPath") or DEFAULT_AGENT_CONFIG
    if agent_host:
        try:
            version = ssh_run(agent_host, ["relkit-agent", "-version"]).stdout.strip()
            agent["remoteVersion"] = version
            listed = ssh_run(
                agent_host,
                ["sudo", "relkit-agent", "init", "-config", str(config_path), "-list-products"],
            ).stdout
            products = parse_agent_products(listed)
            local = load_local(root)
            local["evidence"]["agent.list-products"] = redact_text(listed)
            local["evidence"]["agent.version"] = version
            save_local(root, local)
            expected = None
            try:
                expected = product_id(state)
            except Fail:
                expected = None
            registered = state["steps"]["agent.register"]["status"] in (
                "applied",
                "verified",
            )
            if expected and registered and expected not in products:
                drift.append(
                    f"agent {agent_host} does not list {expected}; local state says registered"
                )
                state["steps"]["agent.register"]["status"] = "drift"
            elif expected and expected in products and registered:
                state["steps"]["agent.register"]["status"] = "verified"
        except Fail as error:
            unconfirmed.append(str(error))

    lock_path = root / "scripts" / "relkit.lock.json"
    if lock_path.is_file():
        try:
            lock = load_json(lock_path)
            if lock.get("schema") != LOCK_SCHEMA:
                drift.append(f"{lock_path} is not {LOCK_SCHEMA}")
            pinned = str(lock.get("hostScriptsSha256") or "")
            actual = tree_sha256(host_scripts_dir())
            if pinned and pinned != actual:
                drift.append("scripts/host tree does not match lock hostScriptsSha256")
            else:
                set_step(state, "consume.lock", "verified", lock.get("release"), "lock present")
        except (OSError, json.JSONDecodeError) as error:
            unconfirmed.append(f"cannot read lock: {error}")

    if write:
        save_state(root, state)
    report = dict(state)
    report["drift"] = drift
    report["unconfirmed"] = unconfirmed
    return report


def cmd_onboard_start(root: Path, *, interactive: bool = False) -> int:
    ensure_gitignore(root)
    state = load_state(root)
    stack = detect_stack(root)
    set_step(state, "repo.root", "verified", ".", ",".join(stack["languages"]))
    save_state(root, state)
    print("已检测仓库事实；尚未替你确认任何策略。")
    print(f"仓根: {root}")
    print(f"技术栈: {','.join(stack['languages']) or 'unknown'}")
    if interactive:
        return run_interactive_wizard(root)
    unresolved = next_unresolved_step(state)
    if unresolved:
        print_decision(root, state, unresolved)
        print(f"非交互模式写入: onboard set {unresolved} <你的选择>")
    return 0


def cmd_onboard_resume(root: Path, *, interactive: bool = False) -> int:
    state = load_state(root)
    if interactive:
        return run_interactive_wizard(root)
    step_id = next_unresolved_step(state)
    if step_id:
        print_decision(root, state, step_id)
        print(f"非交互模式写入: onboard set {step_id} <你的选择>")
        return 0
    print("开箱状态没有未决项。运行 verify 对账。")
    return 0


def cmd_onboard_explain(step_id: str) -> int:
    print(explain_step(step_id))
    return 0


def cmd_onboard_reset(root: Path, *, yes: bool = False) -> int:
    path = relkit_dir(root)
    if not yes:
        raise Fail("onboard reset deletes .relkit/; pass --yes")
    if path.exists():
        shutil.rmtree(path)
    print(f"deleted {path.as_posix()}")
    print("product onboarding is unanswered; no extra markdown was kept")
    return 0


def cmd_onboard_set(
    root: Path,
    step_id: str,
    value: Optional[str],
    share_with: Optional[str],
    note: Optional[str] = None,
) -> int:
    state = load_state(root)
    previous_note = str(state["steps"].get(step_id, {}).get("note") or "")
    kept_note = previous_note if note is None else note
    if step_id == "product.id":
        if not value:
            raise Fail("onboard set product.id needs an id")
        state["product"] = value
        set_step(state, step_id, "confirmed", value, kept_note, mark_later_stale=True)
    elif step_id == "ssh.host":
        if not value:
            raise Fail("onboard set ssh.host needs a Host name from ~/.ssh/config")
        if not ssh_host_allowed(value):
            raise Fail(f"{value} does not match ~/.ssh/config. {ssh_host_recommend()}")
        state["serve"]["sshHost"] = value
        set_step(state, step_id, "confirmed", value, kept_note, mark_later_stale=True)
    elif step_id == "ssh.config_dir":
        if not value:
            raise Fail("onboard set ssh.config_dir needs the live config directory")
        state["serve"]["configDir"] = value
        set_step(state, step_id, "confirmed", value, kept_note, mark_later_stale=True)
    elif step_id == "token.isolation":
        if value in (None, "exclusive"):
            state["serve"]["shareWith"] = None
            set_step(state, step_id, "confirmed", "exclusive", kept_note, mark_later_stale=True)
        elif value == "share-with":
            if not share_with:
                raise Fail("share-with requires --share-with <existing-id>")
            state["serve"]["shareWith"] = share_with
            set_step(
                state,
                step_id,
                "confirmed",
                f"share-with:{share_with}",
                kept_note,
                mark_later_stale=True,
            )
        else:
            raise Fail("token.isolation is exclusive or share-with")
    elif step_id == "updater.process":
        if not value:
            raise Fail("onboard set updater.process needs a process name")
        if value not in UPDATER_PROCESS_VALUES:
            raise Fail("updater.process must be " + " / ".join(UPDATER_PROCESS_VALUES))
        set_step(state, step_id, "confirmed", value, kept_note, mark_later_stale=True)
    elif step_id == "backend.kind":
        set_step(
            state,
            step_id,
            "confirmed",
            value or "intranet-relkit-compatible",
            kept_note,
            mark_later_stale=True,
        )
    else:
        if value is None:
            raise Fail(f"onboard set {step_id} needs a value")
        set_step(state, step_id, "confirmed", value, kept_note, mark_later_stale=True)
    save_state(root, state)
    print(f"recorded {step_id}={state['steps'][step_id]['value']} status=confirmed")
    print("no remote mutation happened")
    return 0


def import_consume() -> Any:
    scripts = host_scripts_dir()
    if str(scripts) not in sys.path:
        sys.path.insert(0, str(scripts))
    import relkit_consume

    return relkit_consume


def consume_components(root: Path) -> list[str]:
    stack = detect_stack(root)
    components = ["cli", "updater"]
    if "dart" in stack["languages"]:
        components.insert(0, "sdk-dart")
    if "rust" in stack["languages"]:
        components.insert(0, "sdk-rust")
    return components


def cmd_install(root: Path, consume_argv: Sequence[str]) -> int:
    consume = import_consume()
    extra = list(consume_argv)
    if not any(item == "--component" for item in extra):
        for component in consume_components(root):
            extra.extend(["--component", component])
    code = consume.main(["install", "--project-root", str(root), *extra])
    if code == 0:
        state = load_state(root)
        set_step(state, "consume.lock", "applied", None, "install complete")
        save_state(root, state)
    return code


def cmd_upgrade(root: Path, release: str) -> int:
    if not re.fullmatch(r"v\d+\.\d+\.\d+", release):
        raise Fail("upgrade expects vX.Y.Z")
    lock_path = root / "scripts" / "relkit.lock.json"
    if not lock_path.is_file():
        raise Fail(f"missing {lock_path}")
    lock = load_json(lock_path)
    if lock.get("schema") != LOCK_SCHEMA:
        raise Fail(f"{lock_path} must use {LOCK_SCHEMA}")
    base = f"https://github.com/{GITHUB_REPO}/releases/download/{release}"
    # Commit 只从同目录的 immutable 附件读。不要打 api.github.com：
    # 匿名 REST 每小时 60 次，和 Releases 下载不共用配额。
    commit = release_commit_from_manifest(base)
    sums_text = http_get(f"{base}/SHA256SUMS")
    sums = parse_sha256sums(sums_text)
    artifacts = lock.setdefault("artifacts", {})
    rewrite_lock_artifacts(artifacts, base, sums)
    lock["release"] = release
    lock["commit"] = commit
    consume_script = host_scripts_dir() / "relkit_consume.py"
    if consume_script.is_file():
        lock["consumerSha256"] = hashlib.sha256(consume_script.read_bytes()).hexdigest()
    lock["hostScriptsSha256"] = tree_sha256(host_scripts_dir())
    lock_path.write_text(dump_json(lock), encoding="utf-8")
    print(f"updated {lock_path} to {release}")
    return cmd_install(root, [])


def http_get(url: str) -> str:
    request = Request(url, headers={"User-Agent": "relkit-host"})
    try:
        with urlopen(request, timeout=30) as response:
            return response.read().decode("utf-8")
    except (HTTPError, URLError, TimeoutError, OSError) as error:
        raise Fail(f"GET {url} failed: {error}") from error


def parse_sha256sums(text: str) -> dict[str, str]:
    mapping: dict[str, str] = {}
    for line in text.splitlines():
        parts = line.split()
        if len(parts) >= 2:
            mapping[parts[-1].lstrip("*")] = parts[0].lower()
    return mapping


def release_commit_from_manifest(base: str) -> str:
    """Read the build commit from release manifest.json (CDN), never the REST API."""
    raw = http_get(f"{base}/manifest.json")
    try:
        manifest = json.loads(raw)
    except json.JSONDecodeError as error:
        raise Fail(f"{base}/manifest.json is not JSON: {error}") from error
    commit = str(manifest.get("commit") or "").strip().lower()
    if not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise Fail(
            f"{base}/manifest.json must pin a 40-char commit; refusing to keep the previous lock commit"
        )
    return commit


def rewrite_lock_artifacts(artifacts: dict[str, Any], base: str, sums: dict[str, str]) -> None:
    dart = "relkit-sdk-dart.zip"
    if dart in sums:
        artifacts["sdk-dart"] = {"url": f"{base}/{dart}", "sha256": sums[dart]}
    rust = "relkit-sdk-rust.zip"
    if rust in sums:
        artifacts["sdk-rust"] = {"url": f"{base}/{rust}", "sha256": sums[rust]}
    cli = artifacts.setdefault("cli", {})
    updater = artifacts.setdefault("updater", {})
    mapping = {
        ("cli", "linux-amd64"): "relkit-linux-amd64",
        ("cli", "linux-arm64"): "relkit-linux-arm64",
        ("cli", "windows-amd64"): "relkit-windows-amd64.exe",
        ("cli", "darwin-amd64"): "relkit-darwin-amd64",
        ("cli", "darwin-arm64"): "relkit-darwin-arm64",
        ("updater", "linux-amd64"): "relkit-updater-linux-amd64",
        ("updater", "linux-arm64"): "relkit-updater-linux-arm64",
        ("updater", "windows-amd64"): "relkit-updater-windows-amd64.exe",
        ("updater", "darwin-amd64"): "relkit-updater-darwin-amd64",
        ("updater", "darwin-arm64"): "relkit-updater-darwin-arm64",
    }
    buckets = {"cli": cli, "updater": updater}
    for (component, target), filename in mapping.items():
        if filename not in sums:
            continue
        buckets[component][target] = {
            "url": f"{base}/{filename}",
            "sha256": sums[filename],
        }


def relkit_config(root: Path) -> dict[str, Any]:
    path = root / "relkit.json"
    if not path.is_file():
        raise Fail("relkit.json not found; run keys gen --execute first")
    try:
        data = json.loads(path.read_text(encoding="utf-8-sig"))
    except json.JSONDecodeError as exc:
        raise Fail(f"relkit.json is not valid JSON: {exc}") from exc
    if not isinstance(data, dict):
        raise Fail("relkit.json must be a JSON object")
    return data


def agent_base_url(root: Path) -> Optional[str]:
    url = str((relkit_config(root).get("agent") or {}).get("url") or "").strip()
    return url or None


def publish_protocol_window(root: Path) -> tuple[int, int]:
    """Publisher handshake窗口。lock 钉的 CLI 与 agent 是同一份契约。"""
    lock = root / "scripts" / "relkit.lock.json"
    if lock.is_file():
        try:
            protocol = json.loads(lock.read_text(encoding="utf-8")).get("protocol")
            minimum = int((protocol or {}).get("min") or PUBLISH_PROTOCOL_FALLBACK)
            maximum = int((protocol or {}).get("max") or minimum)
            return minimum, maximum
        except (OSError, TypeError, ValueError, AttributeError, json.JSONDecodeError):
            pass
    return PUBLISH_PROTOCOL_FALLBACK, PUBLISH_PROTOCOL_FALLBACK


def upload_token(root: Path, note: Path) -> str:
    token = os.environ.get(TOKEN_ENV, "").strip()
    if token:
        return token
    path = root / note
    if path.is_file():
        found = extract_export(TOKEN_ENV, path.read_text(encoding="utf-8"))
        if found:
            return found
    raise Fail(
        f"{TOKEN_ENV} is unset. CI injects it as a pipeline secret; "
        f"locally it lives in {str(note).replace(chr(92), '/')}"
    )


def publish_via_agent(
    root: Path,
    binary: Path,
    *,
    product: str,
    version: str,
    url: str,
    execute: bool,
) -> int:
    """CI 侧发布：传 CAS + 瘦 staged 树，再让发布机持钥写 index。"""
    staged = root / ".relkit" / "staged" / version
    if not staged.is_dir():
        raise Fail(f"no staged tree for {version}; run relkit stage first ({staged})")
    publish = url.rstrip("/") + "/publish"
    minimum, maximum = publish_protocol_window(root)
    print(f"agent {url}")
    print(f"staged {staged}")
    if not execute:
        print(f"plan: relkit cas-put --version {version} --product {product}")
        print(f"plan: POST {publish}")
        print("pass --execute to upload and publish; the signing key stays on the publish host")
        return 0
    token = upload_token(root, AGENT_SECRET_NOTE)
    env = dict(os.environ)
    env[TOKEN_ENV] = token
    env["RELKIT_AGENT_URL"] = url
    result = subprocess.run(
        [str(binary), "cas-put", "--version", version, "--product", product],
        cwd=str(root),
        env=env,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        check=False,
    )
    text = redact_text((result.stdout or "") + (result.stderr or ""))
    if text.strip():
        print(text.strip())
    if result.returncode != 0:
        raise Fail("relkit cas-put failed")
    staged_sha = re.search(r"sha256=([0-9a-f]{64})", text)
    if not staged_sha:
        raise Fail("cas-put did not report the staged sha256")
    payload = json.dumps(
        {"product": product, "version": version, "stagedSha256": staged_sha.group(1)}
    ).encode("utf-8")
    request = Request(
        publish,
        data=payload,
        method="POST",
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
            "X-Relkit-Publish-Protocol": str(minimum),
            "X-Relkit-Publish-Protocol-Min": str(minimum),
            "X-Relkit-Publish-Protocol-Max": str(maximum),
            "X-Relkit-Version": "relkit-host",
        },
    )
    print(f"POST {publish}")
    try:
        with urlopen(request, timeout=600) as response:
            body = response.read().decode("utf-8", "replace")
    except HTTPError as exc:
        detail = redact_text(exc.read().decode("utf-8", "replace")).strip()
        raise Fail(f"agent publish failed: HTTP {exc.code} {publish}: {detail or exc.reason}") from exc
    except URLError as exc:
        raise Fail(f"agent publish failed: {publish}: {exc.reason}") from exc
    if body.strip():
        print(redact_text(body).strip())
    return 0


def cmd_release(root: Path, args: argparse.Namespace) -> int:
    state = load_state(root)
    report = reconcile(root, state, write=True)
    if report.get("drift"):
        raise Fail("release refused: unresolved drift\n  " + "\n  ".join(report["drift"]))
    if report.get("unconfirmed"):
        raise Fail(
            "release refused: cannot confirm remote state\n  "
            + "\n  ".join(report["unconfirmed"])
        )
    missing = [
        step
        for step in REQUIRED_FOR_RELEASE
        if state["steps"][step]["status"] != "verified"
    ]
    if missing:
        raise Fail("release refused: unverified steps: " + ", ".join(missing))
    binary = relkit_bin(root)
    version_argv = [str(binary), "version"]
    current = subprocess.run(
        version_argv,
        cwd=str(root),
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        check=False,
    )
    if current.returncode != 0:
        raise Fail(redact_text(current.stderr or current.stdout or "relkit version failed"))
    version = (current.stdout or "").strip().splitlines()[-1] if current.stdout else ""
    print(f"relkit {binary}")
    print(f"version {version}")
    agent = agent_base_url(root)
    if agent:
        return publish_via_agent(
            root,
            binary,
            product=product_id(state),
            version=version,
            url=agent,
            execute=bool(args.execute),
        )
    print("dry-run publish")
    dry = subprocess.run(
        [str(binary), "publish", "--dry-run"],
        cwd=str(root),
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        check=False,
    )
    print(redact_text(dry.stdout or dry.stderr or ""))
    if dry.returncode != 0:
        raise Fail("relkit publish --dry-run failed")
    if not args.execute:
        print("pass --execute to publish for real; index write still happens only then")
        return 0
    print("publishing (index is written last)")
    real = subprocess.run(
        [str(binary), "publish"],
        cwd=str(root),
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        check=False,
    )
    print(redact_text(real.stdout or real.stderr or ""))
    if real.returncode != 0:
        raise Fail("relkit publish failed")
    print("verify")
    verify = subprocess.run(
        [str(binary), "verify"],
        cwd=str(root),
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        check=False,
    )
    print(redact_text(verify.stdout or verify.stderr or ""))
    if verify.returncode != 0:
        raise Fail("relkit verify failed after publish")
    return 0


def chown_serve_product_token(host: str, config_dir: str, product: str) -> None:
    token_path = str(Path(config_dir) / "tokens" / f"{product}.token").replace("\\", "/")
    ssh_run(host, ["sudo", "chown", "relkit:relkit", token_path])
    ssh_run(host, ["sudo", "chmod", "0600", token_path])
    print(f"chown relkit:relkit {token_path}")


def cmd_serve_list(root: Path) -> int:
    state = load_state(root)
    host = (state.get("serve") or {}).get("sshHost")
    config_dir = (state.get("serve") or {}).get("configDir") or DEFAULT_SERVE_DIR
    if not host:
        print("no ssh.host recorded")
        print(ssh_host_recommend())
        print("set it with: onboard set ssh.host <Host>")
        return 1
    out = ssh_run(
        host,
        ["sudo", "relkit-serve", "init", "-out", str(config_dir), "-list-products"],
    )
    print(redact_text(out.stdout))
    return 0


def cmd_serve_add(root: Path, args: argparse.Namespace) -> int:
    require_execute(args, "serve add")
    state = load_state(root)
    product = args.product or product_id(state)
    host = args.host or (state.get("serve") or {}).get("sshHost")
    if not host:
        raise Fail("pass --host or onboard set ssh.host first")
    config_dir = args.config_dir or (state.get("serve") or {}).get("configDir") or DEFAULT_SERVE_DIR
    share_with = args.share_with or (state.get("serve") or {}).get("shareWith")
    remote = [
        "sudo",
        "relkit-serve",
        "init",
        "-out",
        str(config_dir),
        "-product",
        product,
    ]
    if share_with:
        remote.extend(["-share-with", share_with])
    print(f"SSH {host} (config {config_dir}) product {product}")
    result = ssh_run(host, remote)
    stdout = result.stdout or ""
    token = extract_export(TOKEN_ENV, stdout) or extract_export("RELKIT_SERVE_TOKEN", stdout)
    print(redact_text(stdout))
    if token:
        write_secret(root, token)
        print(f"token written to project note {SECRET_NOTE.as_posix()} (plaintext not printed)")
    elif share_with:
        print("share-with does not print a token")
    else:
        raise Fail("init did not print a token; not guessing")
    chown_serve_product_token(host, config_dir, product)
    state["product"] = product
    state["serve"]["sshHost"] = host
    state["serve"]["configDir"] = config_dir
    state["serve"]["shareWith"] = share_with
    set_step(state, "ssh.host", "verified", host)
    set_step(state, "ssh.config_dir", "verified", config_dir)
    set_step(state, "product.id", "verified", product)
    set_step(
        state,
        "token.isolation",
        "verified",
        f"share-with:{share_with}" if share_with else "exclusive",
    )
    set_step(state, "serve.register", "applied", product, "waiting for restart" if not args.restart else "")
    save_state(root, state)
    if args.restart:
        ssh_run(host, ["sudo", "systemctl", "restart", "relkit-serve"])
        set_step(state, "serve.register", "applied", product, "restarted")
        save_state(root, state)
        print("relkit-serve restarted")
    else:
        print("not restarted; old tokens still work until --restart")
    return 0


def cmd_serve_restart(root: Path, args: argparse.Namespace) -> int:
    require_execute(args, "serve restart")
    require_restart_flag(args)
    state = load_state(root)
    host = args.host or (state.get("serve") or {}).get("sshHost")
    if not host:
        raise Fail("pass --host or onboard set ssh.host first")
    product = None
    try:
        product = product_id(state)
    except Fail:
        product = state["steps"]["serve.register"].get("value")
    config_dir = (state.get("serve") or {}).get("configDir") or DEFAULT_SERVE_DIR
    if product:
        chown_serve_product_token(host, str(config_dir), str(product))
    ssh_run(host, ["sudo", "systemctl", "reset-failed", "relkit-serve"])
    ssh_run(host, ["sudo", "systemctl", "restart", "relkit-serve"])
    if product:
        set_step(state, "serve.register", "applied", product, "restarted")
        save_state(root, state)
    print("relkit-serve restarted")
    return 0


def cmd_agent_restart(root: Path, args: argparse.Namespace) -> int:
    require_execute(args, "agent restart")
    require_restart_flag(args)
    state = load_state(root)
    host = args.host or (state.get("agent") or {}).get("sshHost") or (state.get("serve") or {}).get("sshHost")
    if not host:
        raise Fail("pass --host or onboard set ssh.host first")
    product = args.product or None
    if not product:
        try:
            product = product_id(state)
        except Fail:
            product = state["steps"]["agent.register"].get("value")
    if product:
        root_dir = args.root_path or f"/srv/relkit/{product}"
        token_path = "/etc/relkit-agent/tokens/" + str(product) + ".token"
        ssh_run(host, ["sudo", "chown", "-R", "relkit:relkit", str(root_dir)])
        ssh_run(host, ["sudo", "chown", "relkit:relkit", token_path])
        print(f"chown relkit:relkit {root_dir} and {token_path}")
    ssh_run(host, ["sudo", "systemctl", "restart", "relkit-agent"])
    if product:
        set_step(state, "agent.register", "applied", product, "restarted")
        save_state(root, state)
    print("relkit-agent restarted")
    return 0


def cmd_serve_rotate(root: Path, args: argparse.Namespace) -> int:
    require_execute(args, "serve rotate")
    state = load_state(root)
    product = args.product or product_id(state)
    host = args.host or (state.get("serve") or {}).get("sshHost")
    config_dir = args.config_dir or (state.get("serve") or {}).get("configDir") or DEFAULT_SERVE_DIR
    if not host:
        raise Fail("pass --host or onboard set ssh.host first")
    result = ssh_run(
        host,
        [
            "sudo",
            "relkit-serve",
            "init",
            "-out",
            str(config_dir),
            "-product",
            product,
            "-token-only",
        ],
    )
    stdout = result.stdout or ""
    token = extract_export(TOKEN_ENV, stdout) or extract_export("RELKIT_SERVE_TOKEN", stdout)
    print(redact_text(stdout))
    if not token:
        raise Fail("rotate did not print a token")
    write_secret(root, token)
    print(f"new token written to {SECRET_NOTE.as_posix()} (plaintext not printed)")
    print("deliver this to publishers before --restart")
    if args.restart:
        ssh_run(host, ["sudo", "systemctl", "restart", "relkit-serve"])
        print("relkit-serve restarted; previous token is invalid")
    else:
        print("not restarted; previous token still works")
    return 0


def cmd_serve_remove(root: Path, args: argparse.Namespace) -> int:
    require_execute(args, "serve remove")
    state = load_state(root)
    product = args.product or product_id(state)
    host = args.host or (state.get("serve") or {}).get("sshHost")
    config_dir = args.config_dir or (state.get("serve") or {}).get("configDir") or DEFAULT_SERVE_DIR
    if not host:
        raise Fail("pass --host or onboard set ssh.host first")
    if args.restart:
        ssh_run(host, ["sudo", "systemctl", "restart", "relkit-serve"])
    result = ssh_run(
        host,
        [
            "sudo",
            "relkit-serve",
            "init",
            "-out",
            str(config_dir),
            "-product",
            product,
            "-remove",
        ],
    )
    print(redact_text(result.stdout or ""))
    archive_secret(root)
    set_step(state, "serve.register", "stale", product, "removed remotely")
    save_state(root, state)
    print(f"archived local note {SECRET_NOTE.as_posix()}.revoked if it existed")
    if not args.restart:
        print("pass --restart as soon as possible so the revoked id stops being writable")
    return 0


def cmd_agent_list(root: Path) -> int:
    state = load_state(root)
    host = (state.get("agent") or {}).get("sshHost") or (state.get("serve") or {}).get("sshHost")
    config_path = (state.get("agent") or {}).get("configPath") or DEFAULT_AGENT_CONFIG
    if not host:
        raise Fail("no agent/serve ssh host recorded")
    out = ssh_run(
        host,
        ["sudo", "relkit-agent", "init", "-config", str(config_path), "-list-products"],
    )
    print(redact_text(out.stdout))
    return 0


def cmd_agent_add(root: Path, args: argparse.Namespace) -> int:
    require_execute(args, "agent add")
    state = load_state(root)
    product = args.product or product_id(state)
    host = args.host or (state.get("agent") or {}).get("sshHost") or (state.get("serve") or {}).get("sshHost")
    config_path = args.config or (state.get("agent") or {}).get("configPath") or DEFAULT_AGENT_CONFIG
    if not host:
        raise Fail("pass --host")
    remote = [
        "sudo",
        "relkit-agent",
        "init",
        "-config",
        str(config_path),
        "-product",
        product,
    ]
    if args.root_path:
        remote.extend(["-root", args.root_path])
    result = ssh_run(host, remote)
    print(redact_text(result.stdout or ""))
    token = extract_export(TOKEN_ENV, result.stdout or "")
    if token:
        write_secret(root, token, note=AGENT_SECRET_NOTE)
        print(f"agent token written to {AGENT_SECRET_NOTE.as_posix()} (plaintext not printed)")
    state["agent"]["sshHost"] = host
    state["agent"]["configPath"] = config_path
    set_step(state, "agent.register", "applied", product)
    save_state(root, state)
    if args.restart:
        ssh_run(host, ["sudo", "systemctl", "restart", "relkit-agent"])
        print("relkit-agent restarted")
    else:
        print("not restarted; pass --restart after the user allows it")
    return 0


def cmd_agent_remove(root: Path, args: argparse.Namespace) -> int:
    require_execute(args, "agent remove")
    state = load_state(root)
    product = args.product or product_id(state)
    host = args.host or (state.get("agent") or {}).get("sshHost") or (state.get("serve") or {}).get("sshHost")
    config_path = args.config or (state.get("agent") or {}).get("configPath") or DEFAULT_AGENT_CONFIG
    if not host:
        raise Fail("pass --host")
    result = ssh_run(
        host,
        [
            "sudo",
            "relkit-agent",
            "init",
            "-config",
            str(config_path),
            "-product",
            product,
            "-remove",
        ],
    )
    print(redact_text(result.stdout or ""))
    set_step(state, "agent.register", "stale", product, "removed remotely")
    save_state(root, state)
    return 0


def run_relkit(root: Path, binary: Path, argv: Sequence[str]) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        [str(binary), *argv],
        cwd=str(root),
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        check=False,
    )
    text = redact_text((result.stdout or "") + (result.stderr or ""))
    if text.strip():
        print(text.strip())
    if result.returncode != 0:
        raise Fail(text.strip() or f"relkit {' '.join(argv)} failed")
    return result


def cmd_keys_gen(root: Path, args: argparse.Namespace) -> int:
    require_execute(args, "keys gen")
    ensure_gitignore(root)
    state = load_state(root)
    product = args.product or product_id(state)
    key_id = args.key_id or "k1"
    out_dir = args.out or ".relkit-keys"
    binary = relkit_bin(root, getattr(args, "bin", None))
    config = root / "relkit.json"
    if not config.is_file():
        run_relkit(root, binary, ["init", "--product", product])
    plain = root / "VERSION"
    if plain.is_file():
        raw = plain.read_text(encoding="utf-8").strip().splitlines()[0].strip()
        if raw:
            run_relkit(root, binary, ["version", "set", raw])
            set_step(
                state,
                "channel.ssot",
                "applied",
                "migrate:VERSION->VERSION.json",
                f"VERSION.json={raw}",
            )
    run_relkit(
        root,
        binary,
        ["keygen", "--key-id", key_id, "--out", out_dir, "--update-config"],
    )
    pub = Path(out_dir) / f"{key_id}.public.pb"
    set_step(state, "signing.keys", "applied", key_id, pub.as_posix())
    save_state(root, state)
    print(f"recorded signing.keys={key_id}; private key is gitignored")
    return 0


def routing_help() -> str:
    return """relkit_host.py — product-repo ops gate. Prints routing only; confirms nothing.

  onboard start|resume [--interactive|--non-interactive]
  onboard explain|set|reset
  install
  upgrade vX.Y.Z
  release [--execute]
  serve list|add|restart|rotate|remove
  agent list|add|restart|remove
  keys gen
  status [--json]
  verify [--all]

CI must name a subcommand. Mutations need --execute. Restarts need --restart.

Empty-machine install / binary replace lives in the relkit repo:
  python deploy/relkit.py build|install|upgrade
"""


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="relkit_host.py",
        description="Product-repo relkit ops. Does not install systemd units.",
        add_help=True,
    )
    parser.add_argument("--project-root", help="product repository root")
    sub = parser.add_subparsers(dest="cmd")

    onboard = sub.add_parser("onboard", help="decision wizard; does not mutate remotes")
    onboard_sub = onboard.add_subparsers(dest="onboard_cmd", required=True)
    start = onboard_sub.add_parser("start")
    start_mode = start.add_mutually_exclusive_group()
    start_mode.add_argument("--interactive", action="store_true")
    start_mode.add_argument("--non-interactive", action="store_true")
    resume = onboard_sub.add_parser("resume")
    resume_mode = resume.add_mutually_exclusive_group()
    resume_mode.add_argument("--interactive", action="store_true")
    resume_mode.add_argument("--non-interactive", action="store_true")
    explain = onboard_sub.add_parser("explain")
    explain.add_argument("step")
    setter = onboard_sub.add_parser("set")
    setter.add_argument("step")
    setter.add_argument("value", nargs="?")
    setter.add_argument("--share-with")
    setter.add_argument("--note", help="short product fact only; not an architecture essay")
    reset = onboard_sub.add_parser("reset", help="delete product .relkit/; does not write docs")
    reset.add_argument("--yes", action="store_true")

    sub.add_parser("install", help="install lock-pinned artifacts via relkit_consume.py")
    upgrade = sub.add_parser("upgrade", help="rewrite lock to a GitHub release and install")
    upgrade.add_argument("release")

    release = sub.add_parser("release", help="publish if onboard is verified and there is no drift")
    release.add_argument("--execute", action="store_true")

    serve = sub.add_parser("serve")
    serve_sub = serve.add_subparsers(dest="serve_cmd", required=True)
    serve_sub.add_parser("list")
    serve_restart = serve_sub.add_parser("restart")
    serve_restart.add_argument("--host")
    serve_restart.add_argument("--execute", action="store_true")
    serve_restart.add_argument("--restart", action="store_true")
    serve_add = serve_sub.add_parser("add")
    serve_add.add_argument("--product")
    serve_add.add_argument("--share-with")
    serve_add.add_argument("--host")
    serve_add.add_argument("--config-dir")
    serve_add.add_argument("--execute", action="store_true")
    serve_add.add_argument("--restart", action="store_true")
    serve_rot = serve_sub.add_parser("rotate")
    serve_rot.add_argument("--product")
    serve_rot.add_argument("--host")
    serve_rot.add_argument("--config-dir")
    serve_rot.add_argument("--execute", action="store_true")
    serve_rot.add_argument("--restart", action="store_true")
    serve_rm = serve_sub.add_parser("remove")
    serve_rm.add_argument("--product")
    serve_rm.add_argument("--host")
    serve_rm.add_argument("--config-dir")
    serve_rm.add_argument("--execute", action="store_true")
    serve_rm.add_argument("--restart", action="store_true")

    agent = sub.add_parser("agent")
    agent_sub = agent.add_subparsers(dest="agent_cmd", required=True)
    agent_sub.add_parser("list")
    agent_restart = agent_sub.add_parser("restart")
    agent_restart.add_argument("--product")
    agent_restart.add_argument("--host")
    agent_restart.add_argument("--root-path")
    agent_restart.add_argument("--execute", action="store_true")
    agent_restart.add_argument("--restart", action="store_true")
    agent_add = agent_sub.add_parser("add")
    agent_add.add_argument("--product")
    agent_add.add_argument("--host")
    agent_add.add_argument("--config")
    agent_add.add_argument("--root-path")
    agent_add.add_argument("--execute", action="store_true")
    agent_add.add_argument("--restart", action="store_true")
    agent_rm = agent_sub.add_parser("remove")
    agent_rm.add_argument("--product")
    agent_rm.add_argument("--host")
    agent_rm.add_argument("--config")
    agent_rm.add_argument("--execute", action="store_true")

    keys = sub.add_parser("keys")
    keys_sub = keys.add_subparsers(dest="keys_cmd", required=True)
    keys_gen = keys_sub.add_parser("gen")
    keys_gen.add_argument("--product")
    keys_gen.add_argument("--key-id", default="k1")
    keys_gen.add_argument("--out", default=".relkit-keys")
    keys_gen.add_argument("--bin")
    keys_gen.add_argument("--execute", action="store_true")

    status = sub.add_parser("status")
    status.add_argument("--json", action="store_true")
    verify = sub.add_parser("verify")
    verify.add_argument("--all", action="store_true")
    verify.add_argument("--json", action="store_true")
    return parser


def dispatch(root: Path, args: argparse.Namespace) -> int:
    if not args.cmd:
        print(routing_help())
        return 0
    if args.cmd == "onboard":
        interactive = bool(
            getattr(args, "interactive", False)
            or (
                sys.stdin.isatty()
                and not getattr(args, "non_interactive", False)
            )
        )
        if args.onboard_cmd == "start":
            return cmd_onboard_start(root, interactive=interactive)
        if args.onboard_cmd == "resume":
            return cmd_onboard_resume(root, interactive=interactive)
        if args.onboard_cmd == "explain":
            return cmd_onboard_explain(args.step)
        if args.onboard_cmd == "reset":
            return cmd_onboard_reset(root, yes=bool(getattr(args, "yes", False)))
        return cmd_onboard_set(
            root,
            args.step,
            args.value,
            args.share_with,
            getattr(args, "note", None),
        )
    if args.cmd == "install":
        return cmd_install(root, [])
    if args.cmd == "upgrade":
        return cmd_upgrade(root, args.release)
    if args.cmd == "release":
        return cmd_release(root, args)
    if args.cmd == "serve":
        if args.serve_cmd == "list":
            return cmd_serve_list(root)
        if args.serve_cmd == "restart":
            return cmd_serve_restart(root, args)
        if args.serve_cmd == "add":
            return cmd_serve_add(root, args)
        if args.serve_cmd == "rotate":
            return cmd_serve_rotate(root, args)
        return cmd_serve_remove(root, args)
    if args.cmd == "agent":
        if args.agent_cmd == "list":
            return cmd_agent_list(root)
        if args.agent_cmd == "restart":
            return cmd_agent_restart(root, args)
        if args.agent_cmd == "add":
            return cmd_agent_add(root, args)
        return cmd_agent_remove(root, args)
    if args.cmd == "keys":
        return cmd_keys_gen(root, args)
    if args.cmd == "status":
        return cmd_status(root, args.json)
    if args.cmd == "verify":
        state = load_state(root)
        report = reconcile(root, state, write=True)
        if args.json:
            print(dump_json(report))
        else:
            cmd_status(root, False)
        if report.get("drift") or report.get("unconfirmed"):
            return 1
        return 0
    raise Fail(f"unknown command {args.cmd}")


def main(argv: Optional[Sequence[str]] = None) -> int:
    force_utf8_stdio()
    try:
        parser = build_parser()
        args = parser.parse_args(list(argv) if argv is not None else None)
        root = (
            Path(args.project_root).resolve()
            if getattr(args, "project_root", None)
            else host_root()
        )
        return dispatch(root, args)
    except Fail as error:
        print(f"ERROR: {error}", file=sys.stderr)
        return 1
    except Exception as error:
        print(f"ERROR: relkit_host failed: {error}", file=sys.stderr)
        print(traceback.format_exc(), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
