#!/usr/bin/env python3
"""Read-only queries against a relkit agent, for use from the relkit-ops skill.

The point of this script is that the upload token never enters the agent's
context: the script reads it out of the project's Dec secrets landing zone and
puts it straight into an Authorization header. Nothing here prints it, and
nothing here accepts it as an argument.

Only safe verbs are implemented. Write operations (publish, unpublish, token
rotation) belong to the relkit CLI or the panel, where the server enforces
idempotency -- that is deliberately not re-implemented here.

Two different credentials are in play and they are not interchangeable:

  product token  RELKIT_UPLOAD_TOKEN   /v1/catalog, /v1/staged/, /v1/tools/
  admin session  admin login           /v1/admin/*

The product token is scoped to exactly one product; the agent answers 403 with
both product names when you present the wrong one. Admin calls need a session
obtained from username/password, so they are only available when the caller has
operator credentials -- the script reports that plainly rather than failing with
a bare 401.

Server address and product come from relkit.project.json at the repository root
unless overridden, so a skill consumer does not restate them.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any

TIMEOUT = 30.0

# Dec lands project-plane secrets here; see Dec internal/secrets LoadEnvForP.
# Read from the project root upward is unnecessary: skills run at the root.
SECRETS_ENV_DIRS = (".secrets/relkit/env", ".secrets/relkit")


class QueryError(RuntimeError):
    """A query failed for a reason the caller can act on."""


# --- project declaration -------------------------------------------------


def find_project_root(start: Path) -> Path | None:
    for candidate in (start, *start.parents):
        if (candidate / "relkit.project.json").is_file():
            return candidate
    return None


def load_project(root: Path) -> dict[str, Any]:
    path = root / "relkit.project.json"
    try:
        doc = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise QueryError(f"{path} not found; pass --server and --product explicitly") from exc
    except json.JSONDecodeError as exc:
        raise QueryError(f"{path} is not valid JSON: {exc}") from exc
    if not isinstance(doc, dict):
        raise QueryError(f"{path} must be a JSON object")
    return doc


# --- credentials ---------------------------------------------------------


def parse_env_file(path: Path) -> dict[str, str]:
    out: dict[str, str] = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        key = key.strip()
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in ("'", '"'):
            value = value[1:-1]
        if key:
            out[key] = value
    return out


def product_token(root: Path, env_var: str) -> str:
    """Resolve the product upload token without ever echoing it.

    Precedence is environment first, because CI already exports it, then the
    Dec secrets landing zone. Mirrors how the agent CLI itself resolves it.
    """
    inline = os.environ.get(env_var, "").strip()
    if inline:
        return inline
    for rel in SECRETS_ENV_DIRS:
        directory = root / rel
        if not directory.is_dir():
            continue
        for path in sorted(directory.glob("*.env")):
            if env_var in parse_env_file(path):
                return parse_env_file(path)[env_var].strip()
        direct = directory / ".env"
        if direct.is_file() and env_var in parse_env_file(direct):
            return parse_env_file(direct)[env_var].strip()
    raise QueryError(
        f"{env_var} is not set and no {rel} file in this project carries it; "
        "run 'dec pull' in the project, or export it in the environment"
    )


# --- transport -----------------------------------------------------------


def request(
    method: str,
    url: str,
    *,
    token: str = "",
    body: bytes | None = None,
    content_type: str = "",
) -> tuple[int, Any]:
    headers: dict[str, str] = {}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    if content_type:
        headers["Content-Type"] = content_type
    req = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as response:
            raw = response.read()
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        try:
            return exc.code, json.loads(raw.decode("utf-8") or "{}")
        except (json.JSONDecodeError, UnicodeDecodeError):
            return exc.code, raw.decode("utf-8", "replace").strip()
    except urllib.error.URLError as exc:
        raise QueryError(f"{method} {url} unreachable: {exc.reason}") from exc
    if not raw:
        return 200, None
    try:
        return 200, json.loads(raw.decode("utf-8"))
    except (json.JSONDecodeError, UnicodeDecodeError):
        return 200, raw.decode("utf-8", "replace")


def expect(status: int, body: Any, ok: tuple[int, ...] = (200,)) -> Any:
    if status in ok:
        return body
    detail = body if isinstance(body, str) else json.dumps(body, ensure_ascii=False)
    raise QueryError(f"HTTP {status}: {detail}")


def emit(value: Any) -> None:
    if value is None:
        return
    if isinstance(value, str):
        print(value)
    else:
        print(json.dumps(value, ensure_ascii=False, indent=2))


# --- read-only queries ---------------------------------------------------


def cmd_health(base: str) -> Any:
    status, body = request("GET", f"{base}/-/health")
    return expect(status, body)


def cmd_version(base: str) -> Any:
    status, body = request("GET", f"{base}/-/version")
    return expect(status, body)


def cmd_root_pub(base: str) -> Any:
    """The server trust root. Public, and the anchor of every host declaration."""
    status, body = request("GET", f"{base}/v1/tools/root.pub")
    return expect(status, body)


def cmd_catalog(base: str) -> Any:
    """The signed tool catalog. Public: host CI reads it to bootstrap tools."""
    status, body = request("GET", f"{base}/v1/catalog")
    return expect(status, body)


def cmd_products(base: str, session: str) -> Any:
    status, body = request("GET", f"{base}/v1/admin/products", token=session)
    doc = expect(status, body)
    if not isinstance(doc, dict):
        return doc
    # Trim to what an operator scans for: identity, root, and the connect block.
    # The token is never part of connectConfig, so this is safe to print.
    items = []
    for entry in doc.get("products") or []:
        if not isinstance(entry, dict):
            continue
        product = entry.get("product") or {}
        items.append(
            {
                "id": product.get("id") if isinstance(product, dict) else product,
                "root": product.get("root") if isinstance(product, dict) else None,
                "connectConfig": entry.get("connectConfig"),
            }
        )
    return {"count": len(items), "products": items}


def cmd_tokens(base: str, session: str, product: str) -> Any:
    status, body = request("GET", f"{base}/v1/admin/products/{urllib.parse.quote(product, safe='')}/tokens", token=session)
    return expect(status, body)


def cmd_whoami(base: str, session: str) -> Any:
    status, body = request("GET", f"{base}/v1/admin/whoami", token=session)
    return expect(status, body)





def login(base: str, username: str, password: str) -> str:
    payload = json.dumps({"username": username, "password": password}).encode("utf-8")
    status, body = request("POST", f"{base}/v1/admin/login", body=payload, content_type="application/json")
    doc = expect(status, body)
    if not isinstance(doc, dict) or not doc.get("session"):
        raise QueryError("login succeeded but returned no session")
    return str(doc["session"])


# --- entrypoint ----------------------------------------------------------


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(
        prog="relkit_query.py",
        description="read-only queries against a relkit agent",
    )
    parser.add_argument("command", choices=[
        "health", "version", "root-pub", "catalog", "products", "tokens", "whoami",
    ])
    parser.add_argument("--server", help="agent base URL; default from relkit.project.json")
    parser.add_argument("--product", help="product id; default from relkit.project.json")
    parser.add_argument("--root", help="project root; default: nearest ancestor with relkit.project.json")
    parser.add_argument("--admin-user", help="admin username, for /v1/admin/* commands")
    parser.add_argument("--admin-password", help="admin password, for /v1/admin/* commands")
    args = parser.parse_args(argv)

    root = Path(args.root).resolve() if args.root else (find_project_root(Path.cwd()) or Path.cwd())
    project: dict[str, Any] = {}
    if (root / "relkit.project.json").is_file():
        project = load_project(root)

    base = (args.server or str(project.get("server") or "")).rstrip("/")
    product = args.product or str(project.get("product") or "")
    if not base:
        print("no server: pass --server or run inside a project with relkit.project.json", file=sys.stderr)
        return 2

    admin_only = {"products", "tokens", "whoami"}
    try:
        if args.command in admin_only:
            session = ""
            if args.admin_user and args.admin_password:
                session = login(base, args.admin_user, args.admin_password)
            elif args.admin_user:
                print("--admin-password is required with --admin-user", file=sys.stderr)
                return 2
            else:
                print(
                    f"{args.command} needs an admin session; pass --admin-user and --admin-password, "
                    "or use the panel at <server>/-/admin/",
                    file=sys.stderr,
                )
                return 2
            if args.command == "products":
                emit(cmd_products(base, session))
            elif args.command == "tokens":
                if not product:
                    print("tokens needs --product", file=sys.stderr)
                    return 2
                emit(cmd_tokens(base, session, product))
            else:
                emit(cmd_whoami(base, session))
            return 0

        if args.command == "health":
            emit(cmd_health(base))
            return 0
        if args.command == "version":
            emit(cmd_version(base))
            return 0
        if args.command == "root-pub":
            emit(cmd_root_pub(base))
            return 0

        if args.command == "catalog":
            emit(cmd_catalog(base))
    except QueryError as exc:
        print(str(exc), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
