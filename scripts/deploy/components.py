"""Read the vNext component registry from the deploy CLI.

ADR 0019 §1 retired the Python `scripts/host/hostlib/facets.py` registry, and
ADR 0021 made `internal/vnext/registry/components.json` the single declarative
table that owns both distribution identity and release asset naming. This
module is the only bridge the deploy CLI needs: it reads that same JSON file so
there is one registry, not two.

Every registered component is a Go binary: ADR 0026 retired the facade and
projection product trees (sdk-dart, sdk-rust, bindings-ts) together with their
zip packing.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Optional, Sequence
import json

DEPLOY_DIR = Path(__file__).resolve().parent


def _repo_root() -> Path:
    for candidate in (DEPLOY_DIR, *DEPLOY_DIR.parents):
        if (candidate / "go.mod").is_file():
            return candidate
    return DEPLOY_DIR.parent


REPO_ROOT = _repo_root()


def _registry_path() -> Path:
    """Locate the component registry for both supported layouts.

    Locally the registry lives at `<repo>/internal/vnext/registry/components.json`
    and the module is imported from `<repo>/scripts/deploy`. A remote deploy
    bootstrap copies this module next to the remote CLI, where nothing above it
    has a go.mod, so `_repo_root()` cannot find the repository and the registry
    is shipped as a sibling instead.
    """
    canonical = REPO_ROOT / "internal" / "vnext" / "registry" / "components.json"
    if canonical.is_file():
        return canonical
    sibling = DEPLOY_DIR / "components.json"
    if sibling.is_file():
        return sibling
    return canonical


REGISTRY_PATH = _registry_path()
SCHEMA = "relkit.component-registry/1"

# Role names the deploy CLI has always used, mapped onto the vNext registry's
# roles.
ROLE_ALIASES = {
    "infrastructure-binary": "host-binary",
    "product-binary": "product-binary",
}


@dataclass(frozen=True)
class Component:
    name: str
    role: str
    build_flag: str
    destination: str = ""
    binary_prefix: str = ""
    go_package: str = ""
    install_names: tuple[tuple[str, str], ...] = ()
    default: bool = False
    transport: str = "release-asset"
    targets: tuple[str, ...] = ()
    requires: tuple[dict[str, Any], ...] = ()
    provides: tuple[dict[str, Any], ...] = ()

    def install_name(self, target: str) -> str:
        overrides = dict(self.install_names)
        if target in overrides:
            return overrides[target]
        if "*" in overrides:
            return overrides["*"].replace(
                "{exe}", ".exe" if target.startswith("windows-") else ""
            )
        if not self.binary_prefix:
            raise ValueError(f"{self.name} has no install name for {target}")
        return self.binary_prefix + (".exe" if target.startswith("windows-") else "")


def _strings(value: Any) -> tuple[str, ...]:
    if not value:
        return ()
    return tuple(str(item) for item in value)


def _component(raw: dict[str, Any]) -> Component:
    build = raw.get("build") or {}
    role = str(raw.get("role") or "")
    if role not in ROLE_ALIASES:
        raise SystemExit(f"{REGISTRY_PATH}: component {raw.get('name')!r} has unknown role {role!r}")
    return Component(
        name=str(raw.get("name") or ""),
        role=ROLE_ALIASES.get(role, role),
        build_flag=str(build.get("flag") or ""),
        destination=str(raw.get("destination") or ""),
        binary_prefix=str(build.get("binaryPrefix") or ""),
        go_package=str(build.get("goPackage") or ""),
        install_names=tuple(
            (str(pair[0]), str(pair[1])) for pair in (build.get("installNames") or [])
        ),
        default=bool(raw.get("default")),
        transport=str(raw.get("transport") or "release-asset"),
        targets=_strings(raw.get("targets")),
        requires=tuple(raw.get("requires") or ()),
        provides=tuple(raw.get("provides") or ()),
    )


def _load() -> tuple[tuple[str, ...], tuple[Component, ...]]:
    try:
        document = json.loads(REGISTRY_PATH.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise SystemExit(f"cannot read {REGISTRY_PATH}: {error}")
    if not isinstance(document, dict) or document.get("schema") != SCHEMA:
        raise SystemExit(f"{REGISTRY_PATH} must use {SCHEMA}")
    build_targets = _strings(document.get("buildTargets"))
    if not build_targets:
        raise SystemExit(f"{REGISTRY_PATH} declares no buildTargets")
    rows = tuple(_component(raw) for raw in document.get("components") or ())
    if not rows:
        raise SystemExit(f"{REGISTRY_PATH} declares no components")
    if any(row.name == "host-scripts" for row in rows):
        raise SystemExit(
            f"{REGISTRY_PATH} must not contain the retired host-scripts component"
        )
    return build_targets, rows


TARGETS, COMPONENTS = _load()
BY_NAME = {row.name: row for row in COMPONENTS}


def components(role: Optional[str] = None) -> tuple[Component, ...]:
    return tuple(row for row in COMPONENTS if role is None or row.role == role)


def product_components() -> tuple[Component, ...]:
    return components("product-binary")
