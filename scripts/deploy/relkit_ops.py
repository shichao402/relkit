"""Pure deploy helpers: config migrate, unit render, redaction, path derivation.

No SSH, no subprocess side effects. Imported by scripts/deploy/relkit.py and tests.
"""

from __future__ import annotations

import json
import re
from copy import deepcopy
from typing import Any, Optional

DEFAULT_CAS_GRACE = "24h"
DELETED_BACKENDS = frozenset({"local", "http-put"})
REMOVED_BACKENDS = frozenset({"static-http"})
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
    for key in ("adminStateFile",):
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


def render_store_unit(
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
    exec_line = f"ExecStart={prefix.rstrip('/')}/relkit-store -config {config_path}"
    text = re.sub(r"^ExecStart=.*$", exec_line, text, flags=re.M)
    port = parse_listen_port(addr)
    if port is not None and port < 1024 and "AmbientCapabilities=CAP_NET_BIND_SERVICE" not in text:
        insert = (
            "# Required to bind a privileged port as an unprivileged user.\n"
            "# Added by scripts/deploy/relkit.py because the configured port is below 1024.\n"
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


def render_console_unit(
    template: str,
    *,
    user: str,
    prefix: str,
    config_path: str,
    extra_write_paths: list[str],
) -> str:
    """Render relkit-console.service.

    The panel reads the release tree through relkit-store over HTTP and never
    opens it, so the unit grants no access to it. Its only write is the cookie
    key under StateDirectory; extra_write_paths is non-empty only when an
    operator pointed adminStateFile somewhere else on purpose.
    """
    text = template
    text = re.sub(r"^User=.*$", f"User={user}", text, flags=re.M)
    text = re.sub(r"^Group=.*$", f"Group={user}", text, flags=re.M)
    text = re.sub(r"^ReadWritePaths=.*\n", "", text, flags=re.M)
    if extra_write_paths:
        rwp = " ".join(extra_write_paths)
        text = re.sub(
            r"^(StateDirectoryMode=.*)$",
            lambda m: f"{m.group(1)}\nReadWritePaths={rwp}",
            text,
            count=1,
            flags=re.M,
        )
    exec_line = f"ExecStart={prefix.rstrip('/')}/relkit-console -config {config_path}"
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
    public_upload_url: Optional[str] = None,
) -> tuple[dict[str, Any], list[str]]:
    # A profile's baseUrl is the updater read plane the product was registered
    # with. Upgrades never rewrite it: the only flag that used to was also the
    # tool-signing origin, and the two jobs pointed at different domains.
    notes: list[str] = []
    kind = backend.get("type")
    cleaned, n = strip_cas_credentials(backend)
    if not isinstance(cleaned, dict):
        raise ValueError("backend must be an object")
    if n:
        notes.append("removed casCredentials")
    if kind in REMOVED_BACKENDS:
        raise ValueError(
            f"backend type {kind!r} was removed; use s3-compatible or relkit-compatible"
        )
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

    base = cleaned.get("baseUrl") or cleaned.get("url")
    if isinstance(base, str) and base.strip():
        base = base.strip()
        if not base.endswith("/"):
            base += "/"
    else:
        raise ValueError(
            f"cannot migrate backend type {kind!r}: it declares no baseUrl; "
            "set the product's read-plane baseUrl in its profile first"
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
    public_upload_url: Optional[str] = None,
) -> tuple[dict[str, Any], list[str]]:
    cfg, n = strip_cas_credentials(deepcopy(profile))
    if not isinstance(cfg, dict):
        raise ValueError("profile must be an object")
    notes: list[str] = []
    if n:
        notes.append(f"removed casCredentials x{n}")
    if "signing" in cfg:
        cfg.pop("signing", None)
        notes.append(
            "removed signing: a server signs every document with its one root "
            "(requirement 10.5); the profile keeps backends only"
        )
    if "site" in cfg:
        cfg.pop("site", None)
        notes.append("removed product-owned site config; configure site.sinks in relkit-agent.json")
    backends = cfg.get("backends")
    if isinstance(backends, dict):
        new_backends = {}
        for name, backend in backends.items():
            if not isinstance(backend, dict):
                raise ValueError(f"backend {name!r} is not an object")
            migrated, more = migrate_backend(
                backend,
                serve_addr=serve_addr,
                public_upload_url=public_upload_url,
            )
            new_backends[name] = migrated
            notes.extend(f"{name}: {item}" for item in more)
        cfg["backends"] = new_backends
    return cfg, notes


def epoch_split_agent_config(
    agent_cfg: dict[str, Any],
    *,
    product_roots: Optional[dict[str, str]] = None,
    token_hashes: Optional[dict[str, str]] = None,
    root_key_id: str = "",
    custody_manifest: str = "",
) -> tuple[dict[str, Any], dict[str, Any], list[str]]:
    """Split a pre-epoch agent config into (config file, admin document, notes).

    Before the epoch the config file carried `products` and `uploadTokens`, and
    each product profile carried its own signing key. The epoch moved product
    registration and token hashes into the admin document, and moved signing to
    the server's one root (requirements 11.3, 13.6, 10.1). There is no reader
    for the old spelling any more: the agent refuses to start rather than
    silently authorize nobody, so the deploy pipeline has to perform the split
    instead of leaving an operator to hand-edit a server.

    Everything here is derived from data already on the box. A token's plaintext
    exists only in the file it was issued into, so the caller passes the sha256
    of that file's trimmed content; nothing is regenerated, which is what keeps
    an existing publisher working across the upgrade.
    """
    cfg, notes = migrate_agent_config(agent_cfg)
    product_roots = product_roots or {}
    token_hashes = token_hashes or {}
    admin: dict[str, Any] = {"schema": "relkit.agent-admin/1", "products": [], "tokens": []}
    created = "1970-01-01T00:00:00Z"

    products = cfg.pop("products", None)
    if isinstance(products, dict):
        for product_id in sorted(products):
            entry = products[product_id]
            if not isinstance(entry, dict):
                raise ValueError(f"agent products.{product_id} is not an object")
            root = str(entry.get("root") or product_roots.get(product_id) or "").strip()
            if not root:
                raise ValueError(
                    f"agent products.{product_id} has no root; pass the tree so the "
                    "registration can be migrated"
                )
            profile = str(entry.get("profile") or "").strip()
            registered: dict[str, Any] = {"id": product_id, "root": root, "createdAt": created}
            if profile:
                registered["profile"] = profile
            admin["products"].append(registered)
        notes.append(
            f"moved products ({', '.join(sorted(products))}) out of the config file "
            "into the admin document (requirements 11.3, 13.6)"
        )
        dropped = sorted(
            key
            for entry in products.values()
            if isinstance(entry, dict)
            for key in entry
            if key not in {"root", "profile"}
        )
        if dropped:
            # candidateRootFingerprint / distributionRootFingerprint / lane /
            # operatorDeploymentId were the multi-root control plane. The epoch
            # deleted it (requirement 10.2, 10.3); keeping the values would
            # suggest they still mean something.
            notes.append(
                "dropped retired product fields: " + ", ".join(sorted(set(dropped)))
            )

    tokens = cfg.pop("uploadTokens", None)
    if isinstance(tokens, list):
        for index, entry in enumerate(tokens):
            if not isinstance(entry, dict):
                raise ValueError(f"agent uploadTokens[{index}] is not an object")
            carrier = str(entry.get("file") or "").strip()
            digest = token_hashes.get(carrier) or token_hashes.get(str(index)) or ""
            if not digest:
                raise ValueError(
                    f"agent uploadTokens[{index}] ({carrier or 'no file'}) has no token hash; "
                    "read the token file on the host and pass its sha256 so the existing "
                    "credential keeps working"
                )
            for product_id in entry.get("products") or []:
                admin["tokens"].append(
                    {
                        "id": f"tok_{index}_{product_id}",
                        "product": product_id,
                        "label": carrier or "migrated",
                        "hash": digest,
                        "createdAt": created,
                    }
                )
        notes.append(
            f"moved {len(admin['tokens'])} product token authorization(s) out of the config "
            "file into the admin document; the plaintext and its hash are unchanged"
        )

    if root_key_id:
        cfg["rootKeyId"] = root_key_id
        notes.append(f"set rootKeyId={root_key_id}")
    if custody_manifest:
        cfg["custodyManifest"] = custody_manifest
        notes.append(f"set custodyManifest={custody_manifest}")
    return cfg, admin, notes


def admin_merge(existing: dict[str, Any], incoming: dict[str, Any]) -> tuple[dict[str, Any], list[str]]:
    """Merge a migrated admin document into whatever the server already has.

    A server that already finished the epoch has real accounts and possibly
    hand-issued tokens; the migration must add registrations without dropping
    them. Existing entries win, because the live document is the newer truth.
    """
    notes: list[str] = []
    merged = deepcopy(existing)
    merged.setdefault("schema", "relkit.agent-admin/1")
    if merged.get("schema") != "relkit.agent-admin/1":
        raise ValueError(f"admin document schema is {merged.get('schema')!r}, expected relkit.agent-admin/1")

    products = {entry.get("id"): entry for entry in merged.get("products") or [] if isinstance(entry, dict)}
    added_products = []
    for entry in incoming.get("products") or []:
        product_id = entry.get("id")
        if product_id in products:
            continue
        products[product_id] = entry
        added_products.append(product_id)
    merged["products"] = [products[key] for key in sorted(products)]
    if added_products:
        notes.append("registered: " + ", ".join(added_products))

    seen = {(entry.get("product"), entry.get("hash")) for entry in merged.get("tokens") or [] if isinstance(entry, dict)}
    added_tokens = 0
    for entry in incoming.get("tokens") or []:
        if (entry.get("product"), entry.get("hash")) in seen:
            continue
        seen.add((entry.get("product"), entry.get("hash")))
        merged.setdefault("tokens", []).append(entry)
        added_tokens += 1
    if added_tokens:
        notes.append(f"authorized {added_tokens} token(s)")
    return merged, notes


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
    site = cfg.get("site")
    if isinstance(site, dict) and "makers" in site:
        if site.get("sinks"):
            raise ValueError(
                "agent site has both site.makers and site.sinks; keep only site.sinks"
            )
        makers_cfg = site.get("makers") or {}
        if not isinstance(makers_cfg, dict) or not str(makers_cfg.get("projectId") or "").strip():
            raise ValueError(
                "agent site.makers.projectId is empty; migrate it to site.sinks manually"
            )
        sink = {"type": "makers", "projectId": makers_cfg["projectId"]}
        for key in ("region", "tokenEnv"):
            value = makers_cfg.get(key)
            if value:
                sink[key] = value
        cfg["site"] = {"sinks": [sink]}
        notes.append(
            "migrated site.makers to site.sinks [{\"type\":\"makers\"}]; site.makers is deprecated"
        )
    return cfg, notes


def missing_upgrade_targets(
    probe: dict[str, Any], *, want_serve: bool, want_agent: bool
) -> list[str]:
    """Components an upgrade would touch that the host does not actually run.

    A host may legitimately run only one of the two (the public publisher has no
    relkit-store; its data plane is COS). Returned strings say what is missing
    and how to proceed.
    """
    flags = {"serve": "--agent-only", "agent": "--serve-only"}
    missing: list[str] = []
    for kind, wanted in (("serve", want_serve), ("agent", want_agent)):
        if not wanted:
            continue
        entry = probe.get(kind) or {}
        unit = entry.get("unit") or {}
        reasons: list[str] = []
        if not unit.get("FragmentPath"):
            reasons.append("no systemd unit")
        if not entry.get("present"):
            reasons.append(f"no binary at {entry.get('binary') or 'unknown path'}")
        if not reasons:
            continue
        missing.append(
            f"relkit-{kind} ({', '.join(reasons)}); pass {flags[kind]} to skip it, "
            f"or run 'install {kind}' to add it"
        )
    return missing


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


def parse_exec_binary(exec_start: str) -> Optional[str]:
    """First argv of a systemd ExecStart value — the live binary, not PATH."""
    text = exec_start
    if "argv[]=" in text:
        text = text.split("argv[]=", 1)[1]
        text = text.split(";", 1)[0]
    parts = text.split()
    if not parts:
        return None
    return parts[0].rstrip(";")


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


SSOT_SCHEMA = "relkit.version/1"
SSOT_VERSION_RE = re.compile(r"^(\d+)\.(\d+)\.(\d+)\+(\d+)$")


def parse_ssot_document(text: str) -> dict[str, Any]:
    """Parse a relkit.version/1 VERSION.json body. Number is x.y.z; tag is v{number}."""
    try:
        raw = json.loads(text)
    except json.JSONDecodeError as exc:
        raise ValueError(f"VERSION.json is not valid JSON: {exc}") from exc
    if not isinstance(raw, dict):
        raise ValueError("VERSION.json must be an object")
    if raw.get("schema") != SSOT_SCHEMA:
        raise ValueError(f"VERSION.json schema must be {SSOT_SCHEMA}")
    version = str(raw.get("version") or "")
    match = SSOT_VERSION_RE.match(version)
    if not match:
        raise ValueError("VERSION.json version must be x.y.z+build")
    number = f"{match.group(1)}.{match.group(2)}.{match.group(3)}"
    return {
        "version": version,
        "number": number,
        "build": int(match.group(4)),
        "tag": "v" + number,
    }
