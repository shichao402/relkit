"""Pure deploy helpers: config migrate, unit render, redaction, path derivation.

No SSH, no subprocess side effects. Imported by deploy/relkit.py and tests.
"""

from __future__ import annotations

import json
import re
from copy import deepcopy
from typing import Any, Optional

DEFAULT_CAS_GRACE = "24h"
DELETED_BACKENDS = frozenset({"local", "http-put"})
COMPATIBLE = "relkit-compatible"
TOKEN_ENV = "RELKIT_SERVE_TOKEN"

SECRET_KEYS = frozenset(
    {
        "token",
        "uploadToken",
        "secret",
        "secretKey",
        "privateKey",
        "seedBase64",
        "casCredentials",
        "uploadTokens",
    }
)

SIG_RE = re.compile(r"([?&]sig=)[^&\s\"']+", re.IGNORECASE)
BEARER_RE = re.compile(r"(Bearer\s+)(\S+)", re.IGNORECASE)
EXPORT_TOKEN_RE = re.compile(
    r"(export\s+(?:RELKIT_SERVE_TOKEN|RELKIT_UPLOAD_TOKEN|RELKIT_ADMIN_BOOTSTRAP)=)(['\"]?)([^'\"\s]+)(\2)"
)


def load_json_object(text: str) -> dict[str, Any]:
    data = json.loads(text)
    if not isinstance(data, dict):
        raise ValueError("JSON root must be an object")
    return data


def dump_json(data: dict[str, Any]) -> str:
    return json.dumps(data, ensure_ascii=False, indent=2) + "\n"


def parse_listen_port(addr: str) -> Optional[int]:
    addr = (addr or "").strip()
    if not addr:
        return None
    if addr.startswith("[") and "]:" in addr:
        tail = addr.rsplit("]:", 1)[-1]
    elif ":" in addr:
        tail = addr.rsplit(":", 1)[-1]
    else:
        return None
    if not tail.isdigit():
        return None
    return int(tail)


def loopback_base(addr: str) -> str:
    port = parse_listen_port(addr) or 8080
    return f"http://127.0.0.1:{port}"


def read_write_paths(serve_cfg: dict[str, Any]) -> list[str]:
    paths: list[str] = []
    directory = serve_cfg.get("dir")
    if isinstance(directory, str) and directory.strip():
        paths.append(directory.strip())
    for key in ("statsFile", "adminStateFile"):
        value = serve_cfg.get(key)
        if isinstance(value, str) and value.startswith("/"):
            paths.append(value)
    seen: set[str] = set()
    out: list[str] = []
    for path in paths:
        if path not in seen:
            seen.add(path)
            out.append(path)
    return out


def render_serve_unit(
    template: str,
    *,
    user: str,
    prefix: str,
    config_path: str,
    read_write_paths: list[str],
    addr: str,
) -> str:
    text = template
    text = re.sub(r"^User=.*$", f"User={user}", text, flags=re.M)
    text = re.sub(r"^Group=.*$", f"Group={user}", text, flags=re.M)
    rwp = " ".join(read_write_paths) if read_write_paths else "/srv/releases"
    text = re.sub(r"^ReadWritePaths=.*$", f"ReadWritePaths={rwp}", text, flags=re.M)
    exec_line = f"ExecStart={prefix.rstrip('/')}/relkit-serve -config {config_path}"
    text = re.sub(r"^ExecStart=.*$", exec_line, text, flags=re.M)
    port = parse_listen_port(addr)
    if port is not None and port < 1024 and "AmbientCapabilities=CAP_NET_BIND_SERVICE" not in text:
        insert = (
            "# Required to bind a privileged port as an unprivileged user.\n"
            "# Added by deploy/relkit.py because the configured port is below 1024.\n"
            "AmbientCapabilities=CAP_NET_BIND_SERVICE\n"
            "CapabilityBoundingSet=CAP_NET_BIND_SERVICE\n\n"
        )
        text = re.sub(r"^\[Install\]", insert + "[Install]", text, flags=re.M)
    return text


def render_agent_unit(
    template: str,
    *,
    user: str,
    prefix: str,
    config_path: str,
    working_directory: str,
) -> str:
    text = template
    text = re.sub(r"^User=.*$", f"User={user}", text, flags=re.M)
    text = re.sub(r"^Group=.*$", f"Group={user}", text, flags=re.M)
    text = re.sub(
        r"^WorkingDirectory=.*$",
        f"WorkingDirectory={working_directory}",
        text,
        flags=re.M,
    )
    exec_line = f"ExecStart={prefix.rstrip('/')}/relkit-agent -config {config_path}"
    text = re.sub(r"^ExecStart=.*$", exec_line, text, flags=re.M)
    return text


def ensure_cas_grace(serve_cfg: dict[str, Any]) -> tuple[dict[str, Any], list[str]]:
    cfg = deepcopy(serve_cfg)
    notes: list[str] = []
    gc = cfg.get("gc")
    if not isinstance(gc, dict):
        cfg["gc"] = {"enabled": True, "interval": "1h", "casGrace": DEFAULT_CAS_GRACE}
        notes.append(f"set gc.casGrace={DEFAULT_CAS_GRACE}")
        return cfg, notes
    grace = gc.get("casGrace")
    if not isinstance(grace, str) or not grace.strip():
        gc["casGrace"] = DEFAULT_CAS_GRACE
        notes.append(f"set gc.casGrace={DEFAULT_CAS_GRACE}")
    return cfg, notes


def strip_cas_credentials(obj: Any) -> tuple[Any, int]:
    removed = 0
    if isinstance(obj, dict):
        if "casCredentials" in obj:
            obj = dict(obj)
            obj.pop("casCredentials", None)
            removed += 1
        out = {}
        for key, value in obj.items():
            new_value, n = strip_cas_credentials(value)
            out[key] = new_value
            removed += n
        return out, removed
    if isinstance(obj, list):
        items = []
        for item in obj:
            new_item, n = strip_cas_credentials(item)
            items.append(new_item)
            removed += n
        return items, removed
    return obj, removed


def _guess_upload_url(backend: dict[str, Any], serve_addr: str) -> Optional[str]:
    for key in ("uploadUrl", "baseUrl", "url"):
        value = backend.get(key)
        if isinstance(value, str) and value.strip():
            url = value.strip()
            if not url.endswith("/"):
                url += "/"
            return url
    output_dir = backend.get("outputDir")
    if isinstance(output_dir, str) and output_dir.strip():
        return loopback_base(serve_addr) + "/"
    if serve_addr:
        return loopback_base(serve_addr) + "/"
    return None


def migrate_backend(
    backend: dict[str, Any],
    *,
    serve_addr: str,
    public_base_url: Optional[str],
    public_upload_url: Optional[str] = None,
) -> tuple[dict[str, Any], list[str]]:
    notes: list[str] = []
    kind = backend.get("type")
    cleaned, n = strip_cas_credentials(backend)
    if not isinstance(cleaned, dict):
        raise ValueError("backend must be an object")
    if n:
        notes.append("removed casCredentials")
    if kind not in DELETED_BACKENDS and kind != COMPATIBLE:
        return cleaned, notes
    if kind == COMPATIBLE:
        if not cleaned.get("tokenEnv"):
            cleaned["tokenEnv"] = TOKEN_ENV
            notes.append(f"set tokenEnv={TOKEN_ENV}")
        if public_upload_url:
            upload = public_upload_url.rstrip("/") + "/"
            if cleaned.get("uploadUrl") != upload:
                cleaned["uploadUrl"] = upload
                notes.append(f"set uploadUrl={upload}")
        return cleaned, notes

    base = public_base_url or cleaned.get("baseUrl") or cleaned.get("url")
    if isinstance(base, str) and base.strip():
        base = base.strip()
        if not base.endswith("/"):
            base += "/"
    else:
        raise ValueError(
            f'cannot migrate backend type {kind!r}: need public baseUrl '
            "(pass --public-base-url)"
        )
    upload = public_upload_url.rstrip("/") + "/" if public_upload_url else _guess_upload_url(cleaned, serve_addr)
    if not upload:
        raise ValueError(
            f"cannot migrate backend type {kind!r}: no uploadUrl/outputDir to derive from"
        )
    migrated = {
        "type": COMPATIBLE,
        "baseUrl": base,
        "uploadUrl": upload,
        "tokenEnv": TOKEN_ENV,
        "timeoutSeconds": int(cleaned.get("timeoutSeconds") or 600),
    }
    notes.append(f"migrated {kind} -> {COMPATIBLE}")
    return migrated, notes


def migrate_profile(
    profile: dict[str, Any],
    *,
    serve_addr: str,
    public_base_url: Optional[str],
    public_upload_url: Optional[str] = None,
) -> tuple[dict[str, Any], list[str]]:
    cfg, n = strip_cas_credentials(deepcopy(profile))
    if not isinstance(cfg, dict):
        raise ValueError("profile must be an object")
    notes: list[str] = []
    if n:
        notes.append(f"removed casCredentials x{n}")
    backends = cfg.get("backends")
    if isinstance(backends, dict):
        new_backends = {}
        for name, backend in backends.items():
            if not isinstance(backend, dict):
                raise ValueError(f"backend {name!r} is not an object")
            migrated, more = migrate_backend(
                backend,
                serve_addr=serve_addr,
                public_base_url=public_base_url,
                public_upload_url=public_upload_url,
            )
            new_backends[name] = migrated
            notes.extend(f"{name}: {item}" for item in more)
        cfg["backends"] = new_backends
    return cfg, notes


def migrate_agent_config(agent_cfg: dict[str, Any]) -> tuple[dict[str, Any], list[str]]:
    cfg, n = strip_cas_credentials(deepcopy(agent_cfg))
    if not isinstance(cfg, dict):
        raise ValueError("agent config must be an object")
    notes: list[str] = []
    if n:
        notes.append(f"removed casCredentials x{n}")
    if "uploadToken" in cfg or "uploadTokenFile" in cfg:
        cfg.pop("uploadToken", None)
        cfg.pop("uploadTokenFile", None)
        notes.append("removed instance-wide uploadToken fields")
    return cfg, notes


def redact_value(obj: Any, key: Optional[str] = None) -> Any:
    if key is not None and key in SECRET_KEYS:
        return "<redacted>"
    if isinstance(obj, str):
        text = SIG_RE.sub(r"\1<redacted>", obj)
        text = BEARER_RE.sub(r"\1<redacted>", text)
        text = EXPORT_TOKEN_RE.sub(r"\1\2<redacted>\4", text)
        return text
    if isinstance(obj, dict):
        return {k: redact_value(v, k) for k, v in obj.items()}
    if isinstance(obj, list):
        return [redact_value(item) for item in obj]
    return obj


def redact_text(text: str) -> str:
    text = SIG_RE.sub(r"\1<redacted>", text)
    text = BEARER_RE.sub(r"\1<redacted>", text)
    text = EXPORT_TOKEN_RE.sub(r"\1\2<redacted>\4", text)
    return text


def extract_export(name: str, text: str) -> Optional[str]:
    match = re.search(
        rf"export\s+{re.escape(name)}=(['\"])(.*?)(\1)",
        text,
    )
    if match:
        return match.group(2)
    match = re.search(rf"export\s+{re.escape(name)}=(\S+)", text)
    if match:
        return match.group(1).strip("'\"")
    return None


def parse_exec_config(exec_start: str, flag: str = "-config") -> Optional[str]:
    """Pull the path after -config from a systemd ExecStart value."""
    text = exec_start
    if "argv[]=" in text:
        text = text.split("argv[]=", 1)[1]
        text = text.split(";", 1)[0]
    parts = text.split()
    for i, part in enumerate(parts):
        if part == flag and i + 1 < len(parts):
            return parts[i + 1].rstrip(";")
        if part.startswith(flag + "="):
            return part.split("=", 1)[1]
    return None


def token_mode_ok(mode: int) -> bool:
    return (mode & 0o077) == 0


def parse_requirements(text: str) -> list[str]:
    reqs: list[str] = []
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        reqs.append(line)
    return reqs
