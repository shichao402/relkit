#!/usr/bin/env python3
"""Tests for scripts/deploy/relkit_ops.py and CLI parsing."""

from __future__ import annotations

import contextlib
import inspect
import io
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path

DEPLOY = Path(__file__).resolve().parent
sys.path.insert(0, str(DEPLOY))

# Tests create temporary trees constantly, and on this Windows host a
# TemporaryDirectory under %TEMP% intermittently cannot be chmod'ed by Python
# 3.13 (tempfile._resetperms -> WinError 5), which fails tests whose logic is
# unrelated, and only when the suite runs as a whole. A scratch directory inside
# the repository is writable by the account running the tests, so the location
# is pinned here rather than left to the ambient TMP setting.
_SCRATCH = DEPLOY.parent.parent / ".tmp-test"
# Leftovers from earlier runs make this host refuse operations on newly created
# directories, so the scratch area starts empty on every run. Leftovers would
# otherwise accumulate, since the temporary directories below are never deleted.
if _SCRATCH.is_dir():
    shutil.rmtree(_SCRATCH, ignore_errors=True)
_SCRATCH.mkdir(parents=True, exist_ok=True)
tempfile.tempdir = str(_SCRATCH)


class _TempDir:
    """A temporary directory that this host can actually use.

    tempfile.TemporaryDirectory is unusable here: Python 3.13 tightens the new
    directory to 0700 and re-scans it on cleanup, and this host intermittently
    refuses both (WinError 5) for a directory it has just created. That failure
    lands on whichever test happens to be running, several at a time, and only
    when the suite runs as a whole, which makes it look like the code under test
    is broken. Creating the directory here under a unique name and never
    deleting it keeps the isolation the tests need without cleanup work that can
    fail; leftovers accumulate only inside the repository's scratch area.
    """

    _counter = 0

    def __init__(self) -> None:
        type(self)._counter += 1
        self.name = str(_SCRATCH / f"t{os.getpid()}-{type(self)._counter:05d}")
        os.makedirs(self.name)

    def __enter__(self) -> str:
        return self.name

    def __exit__(self, *exc_info: object) -> None:
        return None


tempfile.TemporaryDirectory = _TempDir

import relkit_ops as ops  # noqa: E402
import relkit as deploy_cli  # noqa: E402
from components import (
    BY_NAME,
    TARGETS,
)


class RequirementsTests(unittest.TestCase):
    def test_comments_and_blanks_are_ignored(self):
        text = "# pin\n\n# another\n"
        self.assertEqual(ops.parse_requirements(text), [])

    def test_pins_are_kept(self):
        self.assertEqual(ops.parse_requirements("foo==1.0\n# x\nbar\n"), ["foo==1.0", "bar"])


class SsotTests(unittest.TestCase):
    def test_parses_number_and_tag(self):
        out = ops.parse_ssot_document('{"schema":"relkit.version/1","version":"0.3.20+0"}\n')
        self.assertEqual(out["number"], "0.3.20")
        self.assertEqual(out["version"], "0.3.20+0")
        self.assertEqual(out["build"], 0)
        self.assertEqual(out["tag"], "v0.3.20")

    def test_rejects_wrong_schema(self):
        with self.assertRaises(ValueError):
            ops.parse_ssot_document('{"schema":"nope","version":"0.3.20+0"}\n')

    def test_repo_ssot_matches_parser(self):
        text = (deploy_cli.REPO_ROOT / "VERSION.json").read_text(encoding="utf-8")
        out = ops.parse_ssot_document(text)
        self.assertEqual(out["tag"], "v" + out["number"])

    def test_publish_protocol_contract_matches_code(self):
        minimum, maximum = deploy_cli.publish_protocol_window()
        self.assertEqual((minimum, maximum), (2, 3))
        # ADR 0019 §1 retired scripts/host/release-contract.json; the Go source
        # above is the only place the window is declared now.
        self.assertFalse(
            (deploy_cli.REPO_ROOT / "scripts" / "host" / "release-contract.json").exists()
        )


class RedactTests(unittest.TestCase):
    def test_redacts_token_fields_and_signature_urls(self):
        payload = {
            "uploadTokens": [{"file": "a.token"}],
            "url": "http://127.0.0.1:8080/cas/aa?exp=1&size=1&sig=deadbeef",
            "nested": {"secret": "xyz"},
        }
        out = ops.redact_value(payload)
        self.assertEqual(out["uploadTokens"], "<redacted>")
        self.assertEqual(out["nested"]["secret"], "<redacted>")
        self.assertIn("sig=<redacted>", out["url"])
        self.assertNotIn("deadbeef", json.dumps(out))

    def test_redacts_export_and_bearer(self):
        text = "export RELKIT_SERVE_TOKEN='abc'\nAuthorization: Bearer hunter2"
        red = ops.redact_text(text)
        self.assertNotIn("abc", red)
        self.assertNotIn("hunter2", red)
        self.assertIn("<redacted>", red)


class ListenTests(unittest.TestCase):
    def test_port_and_loopback(self):
        self.assertEqual(ops.parse_listen_port("127.0.0.1:8080"), 8080)
        self.assertEqual(ops.parse_listen_port(":30341"), 30341)
        self.assertEqual(ops.loopback_base("127.0.0.1:8080"), "http://127.0.0.1:8080")

    def test_exec_config_from_systemd_show(self):
        raw = (
            "{ path=/usr/local/bin/relkit-store ; "
            "argv[]=/usr/local/bin/relkit-store -config /etc/relkit-store/relkit-store.json ; "
            "ignore_errors=no }"
        )
        self.assertEqual(
            ops.parse_exec_config(raw),
            "/etc/relkit-store/relkit-store.json",
        )
        self.assertEqual(ops.parse_exec_binary(raw), "/usr/local/bin/relkit-store")


class UnitRenderTests(unittest.TestCase):
    def test_read_write_paths_include_tree_and_external_files(self):
        cfg = {
            "dir": "/data/relkit-store",
            "adminStateFile": "/var/lib/relkit-store/admin.json",
        }
        paths = ops.read_write_paths(cfg)
        self.assertEqual(paths[0], "/data/relkit-store")
        self.assertIn("/var/lib/relkit-store/admin.json", paths)

    def test_render_store_unit_uses_live_paths_and_low_port_caps(self):
        template = (DEPLOY / "relkit-store.service").read_text(encoding="utf-8")
        unit = ops.render_store_unit(
            template,
            user="relkit",
            prefix="/usr/local/bin",
            config_path="/etc/relkit-store/relkit-store.json",
            read_write_paths=["/data/relkit-store"],
            addr="127.0.0.1:80",
        )
        self.assertIn("ReadWritePaths=/data/relkit-store", unit)
        self.assertIn("ExecStart=/usr/local/bin/relkit-store -config /etc/relkit-store/relkit-store.json", unit)
        self.assertIn("AmbientCapabilities=CAP_NET_BIND_SERVICE", unit)
        self.assertNotIn("/srv/releases", unit.split("ReadWritePaths=")[1].splitlines()[0])

    def test_render_agent_keeps_environment_file(self):
        template = (DEPLOY / "relkit-agent.service").read_text(encoding="utf-8")
        unit = ops.render_agent_unit(
            template,
            user="relkit",
            prefix="/usr/local/bin",
            config_path="/etc/relkit-agent/relkit-agent.json",
            working_directory="/srv/relkit",
        )
        self.assertIn("EnvironmentFile=-/etc/relkit-agent/env", unit)


class MigrateTests(unittest.TestCase):
    def test_cas_grace_idempotent(self):
        cfg = {"addr": "127.0.0.1:8080", "dir": "/data/relkit-store"}
        once, notes = ops.ensure_cas_grace(cfg)
        self.assertEqual(once["gc"]["casGrace"], "24h")
        self.assertTrue(notes)
        twice, notes2 = ops.ensure_cas_grace(once)
        self.assertEqual(notes2, [])
        self.assertEqual(twice["addr"], "127.0.0.1:8080")
        self.assertEqual(twice["dir"], "/data/relkit-store")

    def test_strip_cas_credentials(self):
        profile = {"backends": {"x": {"type": "relkit-compatible", "casCredentials": "sts"}}}
        out, n = ops.strip_cas_credentials(profile)
        self.assertEqual(n, 1)
        self.assertNotIn("casCredentials", out["backends"]["x"])

    def test_migrate_http_put_needs_its_own_base(self):
        backend = {"type": "http-put", "baseUrl": "https://update.example/"}
        with self.assertRaises(ValueError):
            ops.migrate_backend(
                {"type": "http-put", "outputDir": "/data"},
                serve_addr="127.0.0.1:8080",
            )
        migrated, notes = ops.migrate_backend(
            backend,
            serve_addr="127.0.0.1:8080",
        )
        self.assertEqual(migrated["type"], "relkit-compatible")
        self.assertEqual(migrated["baseUrl"], "https://update.example/")
        self.assertEqual(migrated["tokenEnv"], "RELKIT_SERVE_TOKEN")
        self.assertTrue(any("http-put" in item for item in notes))

    def test_migrate_rejects_static_http(self):
        with self.assertRaises(ValueError) as raised:
            ops.migrate_backend(
                {"type": "static-http", "baseUrl": "https://example.invalid/"},
                serve_addr="127.0.0.1:8080",
            )
        self.assertIn("static-http", str(raised.exception))

    def test_migrate_local_keeps_declared_base(self):
        profile = {
            "backends": {
                "intranet": {
                    "type": "local",
                    "baseUrl": "http://update-internal.devcloud.woa.com/",
                    "outputDir": "/data/relkit-store",
                    "casCredentials": {"mode": "sts"},
                }
            }
        }
        out, notes = ops.migrate_profile(
            profile,
            serve_addr="127.0.0.1:8080",
            public_upload_url="http://update.devcloud.woa.com/",
        )
        b = out["backends"]["intranet"]
        self.assertEqual(b["type"], "relkit-compatible")
        self.assertEqual(b["baseUrl"], "http://update-internal.devcloud.woa.com/")
        self.assertEqual(b["uploadUrl"], "http://update.devcloud.woa.com/")
        self.assertNotIn("casCredentials", b)
        self.assertTrue(notes)

    def test_upgrade_never_rewrites_compatible_base(self):
        """The read plane is the product's registration, not an upgrade knob."""
        backend = {
            "type": "relkit-compatible",
            "baseUrl": "http://update-internal.devcloud.woa.com/",
            "uploadUrl": "http://127.0.0.1:8080/",
            "tokenEnv": "RELKIT_SERVE_TOKEN",
        }
        migrated, notes = ops.migrate_backend(
            backend,
            serve_addr="127.0.0.1:8080",
            public_upload_url="http://update.devcloud.woa.com:8080",
        )
        self.assertEqual(
            migrated["baseUrl"], "http://update-internal.devcloud.woa.com/"
        )
        self.assertEqual(
            migrated["uploadUrl"], "http://update.devcloud.woa.com:8080/"
        )
        self.assertFalse(any("set baseUrl" in item for item in notes))
        self.assertTrue(any("set uploadUrl" in item for item in notes))

    def test_upgrade_has_no_public_base_url_flag(self):
        with self.assertRaises(SystemExit), contextlib.redirect_stderr(io.StringIO()):
            deploy_cli.build_parser().parse_args(
                ["upgrade", "--host", "box", "--public-base-url", "http://x/"]
            )

    def test_migrate_profile_removes_product_owned_site(self):
        profile = {
            "site": {"makers": {"tokenEnv": "PAGES_TOKEN"}},
            "backends": {
                "prod": {
                    "type": "s3-compatible",
                    "baseUrl": "https://raw.example/",
                }
            },
        }
        migrated, notes = ops.migrate_profile(
            profile,
            serve_addr="127.0.0.1:8080",
        )
        self.assertNotIn("site", migrated)
        self.assertTrue(any("site.sinks" in item for item in notes))

    def test_unknown_backend_left_alone(self):
        backend = {"type": "s3-compatible", "bucket": "x"}
        out, notes = ops.migrate_backend(
            backend, serve_addr=":8080"
        )
        self.assertEqual(out["type"], "s3-compatible")
        self.assertEqual(notes, [])

    def test_agent_instance_token_fields_removed(self):
        cfg, notes = ops.migrate_agent_config(
            {"addr": "127.0.0.1:8787", "uploadToken": "nope", "casCredentials": 1}
        )
        self.assertNotIn("uploadToken", cfg)
        self.assertTrue(notes)

    def test_agent_site_makers_migrated_to_sinks(self):
        cfg, notes = ops.migrate_agent_config(
            {
                "addr": "127.0.0.1:8787",
                "site": {
                    "makers": {
                        "projectId": "relkit-updates-index",
                        "region": "china",
                        "tokenEnv": "EDGEONE_PAGES_API_TOKEN",
                    }
                },
            }
        )
        self.assertEqual(
            cfg["site"]["sinks"],
            [
                {
                    "type": "makers",
                    "projectId": "relkit-updates-index",
                    "region": "china",
                    "tokenEnv": "EDGEONE_PAGES_API_TOKEN",
                }
            ],
        )
        self.assertNotIn("makers", cfg["site"])
        self.assertTrue(any("site.sinks" in item for item in notes))

    def test_agent_site_sinks_left_alone(self):
        sinks = [{"type": "directory", "path": "/srv/relkit-site"}]
        cfg, notes = ops.migrate_agent_config(
            {"addr": "127.0.0.1:8787", "site": {"sinks": sinks}}
        )
        self.assertEqual(cfg["site"]["sinks"], sinks)
        self.assertEqual([n for n in notes if "site" in n], [])

    def test_agent_site_makers_and_sinks_conflict_raises(self):
        with self.assertRaises(ValueError):
            ops.migrate_agent_config(
                {
                    "site": {
                        "makers": {"projectId": "p1"},
                        "sinks": [{"type": "directory", "path": "/srv/site"}],
                    }
                }
            )


class EpochSplitTests(unittest.TestCase):
    """The epoch agent has no reader for `products` or `uploadTokens`.

    The deploy pipeline therefore performs the split on the box: registrations
    and token hashes move into the admin document, signing moves to the server's
    one root. These tests pin the two properties that make the upgrade safe: an
    existing credential keeps working (its hash is derived from the file it was
    issued into, never reissued), and the retired multi-root fields are dropped
    instead of being carried forward as if they still meant something.
    """

    LEGACY_AGENT = {
        "addr": "127.0.0.1:8787",
        "products": {
            "loom": {
                "root": "/data/relkit/loom",
                "profile": "products/loom.json",
                "candidateRootFingerprint": "sha256:aaa",
                "distributionRootFingerprint": "sha256:bbb",
                "lane": "candidate",
                "operatorDeploymentId": "tencent-internal",
            },
            "svn-auto-merge": {"root": "/data/relkit/svn-auto-merge"},
        },
        "uploadTokens": [
            {"file": "tokens/shared.token", "products": ["loom", "svn-auto-merge"]}
        ],
        "stateDir": "/data/relkit-agent",
    }

    def test_products_move_into_admin_with_root_and_profile(self):
        cfg, admin, notes = ops.epoch_split_agent_config(
            self.LEGACY_AGENT, token_hashes={"tokens/shared.token": "f" * 64}
        )
        self.assertNotIn("products", cfg)
        by_id = {entry["id"]: entry for entry in admin["products"]}
        self.assertEqual(by_id["loom"]["root"], "/data/relkit/loom")
        self.assertEqual(by_id["loom"]["profile"], "products/loom.json")
        self.assertEqual(by_id["svn-auto-merge"]["root"], "/data/relkit/svn-auto-merge")
        self.assertNotIn("profile", by_id["svn-auto-merge"])
        self.assertTrue(any("into the admin document" in item for item in notes))

    def test_retired_multi_root_fields_are_dropped(self):
        _, admin, notes = ops.epoch_split_agent_config(
            self.LEGACY_AGENT, token_hashes={"tokens/shared.token": "f" * 64}
        )
        for entry in admin["products"]:
            self.assertEqual(set(entry) & {"candidateRootFingerprint", "lane", "operatorDeploymentId"}, set())
        self.assertTrue(any("dropped retired product fields" in item for item in notes))

    def test_one_token_entry_per_product_authorization(self):
        _, admin, _ = ops.epoch_split_agent_config(
            self.LEGACY_AGENT, token_hashes={"tokens/shared.token": "f" * 64}
        )
        self.assertEqual(len(admin["tokens"]), 2)
        self.assertEqual({entry["product"] for entry in admin["tokens"]}, {"loom", "svn-auto-merge"})
        for entry in admin["tokens"]:
            self.assertEqual(entry["hash"], "f" * 64)

    def test_token_authorization_needs_a_hash(self):
        # Reissuing would break every publisher's CI, so a missing hash is a
        # hard error: the caller must read the token file on the box.
        with self.assertRaises(ValueError) as ctx:
            ops.epoch_split_agent_config(self.LEGACY_AGENT)
        self.assertIn("tokens/shared.token", str(ctx.exception))

    def test_product_without_root_raises(self):
        with self.assertRaises(ValueError):
            ops.epoch_split_agent_config(
                {"products": {"loom": {"profile": "products/loom.json"}}},
                token_hashes={},
            )

    def test_root_key_and_manifest_are_written(self):
        cfg, _, notes = ops.epoch_split_agent_config(
            self.LEGACY_AGENT,
            token_hashes={"tokens/shared.token": "f" * 64},
            root_key_id="intranet-server-root",
            custody_manifest="/etc/relkit-agent/custody-manifest.json",
        )
        self.assertEqual(cfg["rootKeyId"], "intranet-server-root")
        self.assertEqual(cfg["custodyManifest"], "/etc/relkit-agent/custody-manifest.json")
        self.assertTrue(any("set rootKeyId=intranet-server-root" in item for item in notes))
        self.assertTrue(any("set custodyManifest=" in item for item in notes))

    def test_deployment_knobs_survive_the_split(self):
        cfg, _, _ = ops.epoch_split_agent_config(
            self.LEGACY_AGENT,
            token_hashes={"tokens/shared.token": "f" * 64},
            root_key_id="k",
            custody_manifest="/etc/relkit-agent/custody-manifest.json",
        )
        self.assertEqual(cfg["addr"], "127.0.0.1:8787")
        self.assertEqual(cfg["stateDir"], "/data/relkit-agent")

    def test_site_sinks_survive_the_split(self):
        data = dict(self.LEGACY_AGENT)
        data["site"] = {"sinks": [{"type": "backend", "backend": "intranet"}]}
        cfg, _, _ = ops.epoch_split_agent_config(
            data, token_hashes={"tokens/shared.token": "f" * 64}
        )
        self.assertEqual(cfg["site"]["sinks"], [{"type": "backend", "backend": "intranet"}])


class AdminMergeTests(unittest.TestCase):
    """The migration must not clobber a document the server already owns."""

    def test_registers_missing_products_and_keeps_existing(self):
        existing = {
            "schema": "relkit.agent-admin/1",
            "users": [{"id": "u1", "role": "owner"}],
            "products": [{"id": "loom", "root": "/srv/loom", "createdAt": "2026-01-01T00:00:00Z"}],
            "tokens": [],
        }
        incoming = {
            "schema": "relkit.agent-admin/1",
            "products": [
                {"id": "loom", "root": "/other/loom", "createdAt": "1970-01-01T00:00:00Z"},
                {"id": "svn-auto-merge", "root": "/data/relkit/svn-auto-merge", "createdAt": "1970-01-01T00:00:00Z"},
            ],
            "tokens": [],
        }
        merged, notes = ops.admin_merge(existing, incoming)
        by_id = {entry["id"]: entry for entry in merged["products"]}
        self.assertEqual(by_id["loom"]["root"], "/srv/loom", "the live document is the newer truth")
        self.assertEqual(by_id["svn-auto-merge"]["root"], "/data/relkit/svn-auto-merge")
        self.assertEqual(merged["users"], existing["users"])
        self.assertTrue(any("svn-auto-merge" in item for item in notes))

    def test_dedupes_token_authorizations(self):
        existing = {
            "schema": "relkit.agent-admin/1",
            "products": [],
            "tokens": [{"id": "t1", "product": "loom", "hash": "a" * 64}],
        }
        incoming = {
            "schema": "relkit.agent-admin/1",
            "products": [],
            "tokens": [
                {"id": "t2", "product": "loom", "hash": "a" * 64},
                {"id": "t3", "product": "loom", "hash": "b" * 64},
            ],
        }
        merged, _ = ops.admin_merge(existing, incoming)
        self.assertEqual(len(merged["tokens"]), 2)
        self.assertEqual([entry["id"] for entry in merged["tokens"]], ["t1", "t3"])

    def test_second_run_is_idempotent(self):
        incoming = {
            "schema": "relkit.agent-admin/1",
            "products": [{"id": "loom", "root": "/srv/loom", "createdAt": "1970-01-01T00:00:00Z"}],
            "tokens": [{"id": "t1", "product": "loom", "hash": "a" * 64}],
        }
        once, _ = ops.admin_merge({}, incoming)
        twice, notes = ops.admin_merge(once, incoming)
        self.assertEqual(once, twice)
        self.assertEqual(notes, [])

    def test_rejects_foreign_schema(self):
        with self.assertRaises(ValueError):
            ops.admin_merge({"schema": "something/else"}, {"products": [], "tokens": []})




class MissingTargetTests(unittest.TestCase):
    AGENT_ONLY_HOST = {
        "serve": {
            "unit": {"FragmentPath": "", "ActiveState": "inactive"},
            "binary": "/usr/local/bin/relkit-store",
            "present": False,
        },
        "agent": {
            "unit": {"FragmentPath": "/etc/systemd/system/relkit-agent.service"},
            "binary": "/usr/local/bin/relkit-agent",
            "present": True,
        },
    }

    def test_agent_only_host_is_fine_when_serve_is_skipped(self):
        self.assertEqual(
            ops.missing_upgrade_targets(
                self.AGENT_ONLY_HOST, want_serve=False, want_agent=True
            ),
            [],
        )

    def test_agent_only_host_reports_serve_instead_of_crashing(self):
        missing = ops.missing_upgrade_targets(
            self.AGENT_ONLY_HOST, want_serve=True, want_agent=True
        )
        self.assertEqual(len(missing), 1)
        self.assertIn("relkit-store", missing[0])
        self.assertIn("no systemd unit", missing[0])
        self.assertIn("/usr/local/bin/relkit-store", missing[0])
        self.assertIn("--agent-only", missing[0])

    def test_host_running_both_has_nothing_missing(self):
        probe = {
            "serve": {
                "unit": {"FragmentPath": "/etc/systemd/system/relkit-store.service"},
                "binary": "/usr/local/bin/relkit-store",
                "present": True,
            },
            "agent": {
                "unit": {"FragmentPath": "/etc/systemd/system/relkit-agent.service"},
                "binary": "/usr/local/bin/relkit-agent",
                "present": True,
            },
        }
        self.assertEqual(
            ops.missing_upgrade_targets(probe, want_serve=True, want_agent=True), []
        )

    def test_unit_without_binary_still_reports(self):
        probe = {
            "serve": {
                "unit": {"FragmentPath": "/etc/systemd/system/relkit-store.service"},
                "binary": "/usr/local/bin/relkit-store",
                "present": False,
            }
        }
        missing = ops.missing_upgrade_targets(probe, want_serve=True, want_agent=False)
        self.assertEqual(len(missing), 1)
        self.assertIn("no binary at /usr/local/bin/relkit-store", missing[0])
        self.assertNotIn("no systemd unit", missing[0])

    def test_remote_bootstrap_carries_every_module_the_remote_cli_imports(self):
        sources = deploy_cli.remote_bootstrap_sources()
        names = [(path.name, dest) for path, dest in sources]
        self.assertIn(("relkit.py", ""), names)
        self.assertIn(("relkit_ops.py", ""), names)
        self.assertIn(("components.py", ""), names)
        for path, _dest in sources:
            self.assertTrue(path.is_file(), path)

    def test_remote_bootstrap_carries_the_component_registry(self):
        """components.py reads the registry at import time.

        A remote bootstrap lands in /tmp with no repository above it, so
        `_repo_root()` falls back to /tmp and the canonical
        `internal/vnext/registry/components.json` cannot be found. Shipping the
        table as a sibling is what keeps `relkit.py remote probe` importable.
        """
        names = [(path.name, dest) for path, dest in deploy_cli.remote_bootstrap_sources()]
        self.assertIn(("components.json", ""), names)
        bundled = [path for path, _ in deploy_cli.remote_bootstrap_sources() if path.name == "components.json"]
        self.assertEqual(len(bundled), 1)
        self.assertEqual(bundled[0].read_text(encoding="utf-8")[:1], "{")
        self.assertEqual(bundled[0].name, "components.json")
        self.assertTrue(bundled[0].is_file())


class TokenPermTests(unittest.TestCase):
    def test_0600_ok_0640_rejected(self):
        self.assertTrue(ops.token_mode_ok(0o600))
        self.assertFalse(ops.token_mode_ok(0o640))
        self.assertFalse(ops.token_mode_ok(0o644))


class BackupRollbackTests(unittest.TestCase):
    def test_atomic_replace_install(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            src = root / "newbin"
            dest = root / "relkit-store"
            src.write_bytes(b"new")
            dest.write_bytes(b"old")
            tmp_new = dest.with_name(dest.name + ".new")
            shutil.copy2(src, tmp_new)
            os.replace(tmp_new, dest)
            self.assertEqual(dest.read_bytes(), b"new")
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            src = root / "relkit-store.json"
            src.write_text('{"addr":"127.0.0.1:8080"}\n', encoding="utf-8")
            bak = root / "backup"
            bak.mkdir()
            dest = bak / src.name
            dest.write_bytes(src.read_bytes())
            restored = root / "restored.json"
            restored.write_bytes(dest.read_bytes())
            self.assertEqual(restored.read_text(encoding="utf-8"), src.read_text(encoding="utf-8"))


class CliParseTests(unittest.TestCase):
    def test_bindings_archive_uses_precompiled_entrypoints(self):
        names = {name for _, name in deploy_cli.component_entries("bindings-ts")}
        self.assertEqual(names, set(BY_NAME["bindings-ts"].pack_files))
        self.assertNotIn("updater/v1/updater_pb.ts", names)

    def test_component_entries_does_not_branch_on_component_names(self):
        source = inspect.getsource(deploy_cli.component_entries)
        self.assertNotIn("if name ==", source)
        self.assertNotIn("if row.name ==", source)
        self.assertIn("pack_kind()", source)

    def test_registry_pack_covers_required_archive_paths(self):
        # The epoch upgrade ships no Go SDK: the Go packages move into
        # internal/ and host Go code stops being a relkit artifact, so the
        # go-deps pack kind went with it (requirement 135 keeps pack-shape and
        # the component table in step).
        self.assertEqual(BY_NAME["sdk-rust"].pack_kind(), "tracked-tree")
        self.assertEqual(BY_NAME["bindings-ts"].pack_kind(), "listed-files")
        self.assertEqual(BY_NAME["sdk-dart"].pack_kind(), "tracked-tree")
        self.assertNotIn("host-scripts", BY_NAME)
        self.assertNotIn("sdk-go", BY_NAME)
        for row in deploy_cli.registry_components("product-tree"):
            names = {name for _, name in deploy_cli.component_entries(row.name)}
            self.assertTrue(names, f"{row.name} packed no files")

    def test_host_zips_omit_in_process_updater(self):
        dart = {name for _, name in deploy_cli.component_entries("sdk-dart")}
        self.assertNotIn("lib/src/updater.dart", dart)
        self.assertNotIn("lib/src/scheduler.dart", dart)
        self.assertFalse(any(name.startswith("test/") for name in dart))
        # The retired source-grep gate is gone; what remains enforceable is that
        # every declared facade path really is inside its archive.
        for row in deploy_cli.registry_components("product-tree"):
            if not row.updater_process:
                continue
            names = {name for _, name in deploy_cli.component_entries(row.name)}
            self.assertTrue(row.facade_paths)
            for facade in row.facade_paths:
                self.assertIn(facade, names)

    def test_tag_ci_builds_every_registry_component(self):
        workflow = Path(__file__).resolve().parents[2] / ".github" / "workflows" / "release.yml"
        text = workflow.read_text(encoding="utf-8")
        self.assertIn("build --all", text)
        self.assertIn("npm --prefix bindings/ts test", text)
        parser = deploy_cli.build_parser()
        args = parser.parse_args(["build", "--all"])
        self.assertTrue(args.all)
        for row in deploy_cli.registry_components():
            self.assertTrue(hasattr(args, row.build_flag.replace("-", "_")))

    def test_registry_preserves_artifact_and_install_name_goldens(self):
        self.assertEqual(BY_NAME["sdk-rust"].archive, "relkit-sdk-rust.zip")
        self.assertEqual(BY_NAME["bindings-ts"].archive, "relkit-bindings-ts.zip")
        # The vNext registry derives shell names from binaryPrefix alone: one
        # rule for every product binary instead of a per-target override table.
        self.assertEqual(BY_NAME["cli"].install_name("windows-amd64"), "relkit.exe")
        self.assertEqual(BY_NAME["cli"].install_name("linux-amd64"), "relkit")
        self.assertEqual(BY_NAME["cli"].install_name("darwin-arm64"), "relkit")
        self.assertEqual(
            [f"{BY_NAME['updater'].binary_prefix}-{target}" + (".exe" if target.startswith("windows-") else "")
             for target in TARGETS],
            [
                "relkit-updater-linux-amd64",
                "relkit-updater-linux-arm64",
                "relkit-updater-windows-amd64.exe",
                "relkit-updater-darwin-amd64",
                "relkit-updater-darwin-arm64",
            ],
        )

    def test_registry_declares_each_facade_once(self):
        """ADR 0021: one row per facade, and a facade row is one validated unit.

        The retired facets.py validator is gone with the Python registry, so the
        invariants it used to assert are now asserted against the single vNext
        registry that replaced it.
        """
        trees = deploy_cli.registry_components("product-tree")
        self.assertEqual(
            [row.name for row in trees],
            ["sdk-dart", "sdk-rust", "bindings-ts"],
        )
        for row in trees:
            self.assertTrue(row.build_flag)
            self.assertTrue(row.archive)
            self.assertTrue(row.source)
            self.assertEqual(
                bool(row.updater_process), bool(row.facade_paths),
                f"{row.name} must declare updaterProcess and facadePaths together",
            )
        self.assertNotIn("host-scripts", BY_NAME)

    def test_help_and_subcommands(self):
        env = os.environ.copy()
        env["RELKIT_DEPLOY_BOOTSTRAPPED"] = "1"
        result = subprocess.run(
            [sys.executable, str(DEPLOY / "relkit.py"), "-h"],
            capture_output=True,
            text=True,
            env=env,
        )
        self.assertEqual(result.returncode, 0)
        self.assertIn("build", result.stdout)
        self.assertIn("install", result.stdout)
        self.assertIn("upgrade", result.stdout)
        self.assertIn("version", result.stdout)
        self.assertNotIn("0.2.1", result.stdout)
        self.assertNotRegex(result.stdout, r"(?m)^\s+token\s")

    def test_upgrade_requires_host(self):
        env = os.environ.copy()
        env["RELKIT_DEPLOY_BOOTSTRAPPED"] = "1"
        result = subprocess.run(
            [sys.executable, str(DEPLOY / "relkit.py"), "upgrade"],
            capture_output=True,
            text=True,
            env=env,
        )
        self.assertNotEqual(result.returncode, 0)

    def test_version_prints_ssot_number(self):
        env = os.environ.copy()
        env["RELKIT_DEPLOY_BOOTSTRAPPED"] = "1"
        result = subprocess.run(
            [sys.executable, str(DEPLOY / "relkit.py"), "version"],
            capture_output=True,
            text=True,
            env=env,
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), ops.parse_ssot_document(
            (deploy_cli.REPO_ROOT / "VERSION.json").read_text(encoding="utf-8")
        )["number"])

    def test_version_check_tag_rejects_mismatch(self):
        env = os.environ.copy()
        env["RELKIT_DEPLOY_BOOTSTRAPPED"] = "1"
        result = subprocess.run(
            [sys.executable, str(DEPLOY / "relkit.py"), "version", "--check-tag", "v0.0.0"],
            capture_output=True,
            text=True,
            env=env,
        )
        self.assertNotEqual(result.returncode, 0)

    def test_version_probe_avoids_percent(self):
        probe = "python3 --version"
        self.assertNotIn("%", probe)
        text = (DEPLOY / "requirements.txt").read_text(encoding="utf-8")
        self.assertEqual(ops.parse_requirements(text), [])

    def test_rust_sdk_flag_and_deterministic_zip(self):
        args = deploy_cli.build_parser().parse_args(["build", "--rust-sdk"])
        self.assertTrue(args.rust_sdk)
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "Cargo.toml"
            source.write_text("[package]\nname='x'\n", encoding="utf-8")
            first = root / "first.zip"
            second = root / "second.zip"
            entries = [(source, "Cargo.toml")]
            deploy_cli.write_deterministic_zip(first, entries)
            deploy_cli.write_deterministic_zip(second, entries)
            self.assertEqual(first.read_bytes(), second.read_bytes())

    def test_retired_host_scripts_component_is_not_packable(self):
        # ADR 0019 §1 deleted scripts/host; the registry must reject the name
        # rather than quietly pack an empty tree.
        self.assertNotIn("host-scripts", BY_NAME)
        with self.assertRaises(KeyError):
            deploy_cli.component_entries("host-scripts")
        args = deploy_cli.build_parser().parse_args(["build", "--all"])
        self.assertFalse(hasattr(args, "host_scripts"))

    def test_rust_sdk_archive_excludes_ignored_target(self):
        ignore = (deploy_cli.REPO_ROOT / "sdk" / "rust" / ".gitignore").read_text(
            encoding="utf-8"
        )
        self.assertIn("/target/", ignore.splitlines())
        with tempfile.TemporaryDirectory() as tmp:
            archive_path = Path(tmp) / "relkit-sdk-rust.zip"
            deploy_cli.write_deterministic_zip(
                archive_path, deploy_cli.rust_sdk_entries()
            )
            with zipfile.ZipFile(archive_path) as archive:
                names = archive.namelist()
        self.assertIn("Cargo.toml", names)
        self.assertIn("proto/updater/v1/updater.proto", names)
        self.assertFalse(any(name == "target" or name.startswith("target/") for name in names))


class ExtractExportTests(unittest.TestCase):
    def test_extract_once_token(self):
        blob = "blah\nexport RELKIT_SERVE_TOKEN='sekrit'\n"
        self.assertEqual(ops.extract_export("RELKIT_SERVE_TOKEN", blob), "sekrit")


class ConsoleInstallTests(unittest.TestCase):
    def test_render_console_unit_rewrites_exec_and_keeps_tree_out(self):
        template = (DEPLOY / "relkit-console.service").read_text(encoding="utf-8")
        unit = ops.render_console_unit(
            template,
            user="relkit",
            prefix="/usr/local/bin",
            config_path="/etc/relkit-console/relkit-console.json",
            extra_write_paths=[],
        )
        self.assertIn(
            "ExecStart=/usr/local/bin/relkit-console -config /etc/relkit-console/relkit-console.json",
            unit,
        )
        self.assertIn("User=relkit", unit)
        self.assertIn("StateDirectory=relkit-console", unit)
        # The panel reads the tree through the store; the unit grants it nothing
        # under the tree, on this box or any other.
        self.assertNotIn("ReadWritePaths=", unit)
        self.assertNotIn("/srv/releases", unit)

    def test_render_console_unit_lists_a_deliberate_state_location(self):
        template = (DEPLOY / "relkit-console.service").read_text(encoding="utf-8")
        unit = ops.render_console_unit(
            template,
            user="relkit",
            prefix="/usr/local/bin",
            config_path="/etc/relkit-console/relkit-console.json",
            extra_write_paths=["/opt/panel"],
        )
        self.assertIn("ReadWritePaths=/opt/panel", unit)

    def test_console_template_exists_and_is_panel_only(self):
        template = (DEPLOY / "relkit-console.service").read_text(encoding="utf-8")
        self.assertIn("relkit-console", template)
        # The panel never carries upload traffic, so no LimitNOFILE bump.
        self.assertNotIn("LimitNOFILE", template)
        self.assertIn("ProtectSystem=strict", template)

    def test_migrate_and_console_parsers_exist(self):
        args = deploy_cli.build_parser().parse_args(["migrate-serve", "--host", "box", "--store-binary", "dist/relkit-store-linux-amd64"])
        self.assertEqual(args.host, "box")
        self.assertEqual(args.user, "relkit")
        args = deploy_cli.build_parser().parse_args(["install", "console", "--binary", "dist/relkit-console-linux-amd64"])
        self.assertEqual(args.addr, "127.0.0.1:8081")
        self.assertEqual(args.config_dir, "/etc/relkit-console")

    def test_remote_bootstrap_sources_include_console_template(self):
        names = [path.name for path, _ in deploy_cli.remote_bootstrap_sources()]
        self.assertIn("relkit-console.service", names)
        self.assertIn("relkit-store.service", names)


class AgentInstallTests(unittest.TestCase):
    """`install agent` must agree with the agent about default product roots.

    The flag used to have a /srv/relkit default that was never written into the
    agent config, so on an empty box the two disagreed: the unit ran with one
    WorkingDirectory while productRootParent stayed empty, and the first blank
    root registration was refused outright. The flag is mandatory now, and its
    value lands in the config.
    """

    def test_product_root_is_mandatory(self):
        parser = deploy_cli.build_parser()
        with self.assertRaises(SystemExit):
            parser.parse_args(["install", "agent", "--binary", "dist/relkit-agent-linux-amd64"])
        args = parser.parse_args(
            [
                "install",
                "agent",
                "--binary",
                "dist/relkit-agent-linux-amd64",
                "--product-root",
                "/data/relkit",
            ]
        )
        self.assertEqual(args.product_root, "/data/relkit")
        self.assertEqual(args.state_dir, "/var/lib/relkit-agent")

    def test_no_hardcoded_product_root_remains(self):
        source = inspect.getsource(deploy_cli.build_parser)
        self.assertNotIn('/srv/relkit', source)

    def _drain(self, product_root, config):
        """Run the config-writing half of install agent against a temp tree."""
        tmp = Path(_TempDir().name)
        config_dir = tmp / "etc"
        config_dir.mkdir(parents=True)
        config_path = config_dir / "relkit-agent.json"
        config_path.write_text(json.dumps(config), encoding="utf-8")
        return deploy_cli.write_product_root_parent(config_path, product_root), config_path

    def test_product_root_is_written_as_product_root_parent(self):
        parent, config_path = self._drain("/data/relkit", {"addr": "127.0.0.1:8787"})
        self.assertEqual(parent, "/data/relkit")
        written = json.loads(config_path.read_text(encoding="utf-8"))
        self.assertEqual(written["productRootParent"], "/data/relkit")
        # Everything else in the file survives: the installer patches one key.
        self.assertEqual(written["addr"], "127.0.0.1:8787")

    def test_trailing_slash_is_normalised(self):
        parent, config_path = self._drain("/data/relkit/", {})
        self.assertEqual(parent, "/data/relkit")
        self.assertEqual(
            json.loads(config_path.read_text(encoding="utf-8"))["productRootParent"],
            "/data/relkit",
        )

    def test_relative_product_root_is_rejected(self):
        with self.assertRaises(deploy_cli.Fail):
            self._drain("data/relkit", {})

    def test_configured_parent_must_match_the_flag(self):
        with self.assertRaises(deploy_cli.Fail):
            self._drain("/data/relkit", {"productRootParent": "/srv/relkit"})

    def test_matching_parent_is_idempotent(self):
        parent, config_path = self._drain("/data/relkit", {"productRootParent": "/data/relkit"})
        self.assertEqual(parent, "/data/relkit")

    def test_intranet_example_pins_the_real_tree(self):
        example = json.loads(
            (DEPLOY / "relkit-agent.intranet.example.json").read_text(encoding="utf-8")
        )
        self.assertEqual(example["productRootParent"], "/data/relkit")
        self.assertEqual(example["stateDir"], "/data/relkit-agent")

    def test_generic_example_states_the_key_without_guessing(self):
        example = json.loads(
            (DEPLOY / "relkit-agent.example.json").read_text(encoding="utf-8")
        )
        # The key is present so operators see it exists, empty so a template
        # never decides where somebody else's releases live.
        self.assertIn("productRootParent", example)
        self.assertEqual(example["productRootParent"], "")


class AgentUpgradeBackfillTests(unittest.TestCase):
    """An upgraded box must end up with the same answer install agent writes.

    These boxes carry no productRootParent, so they work only while the agent
    can infer one from their products. That inference is exactly what fails on
    an empty box, which is why the flag became mandatory.
    """

    def test_registered_products_decide_before_the_unit(self):
        # A unit's WorkingDirectory records what an installer was handed, not
        # where the trees ended up. The live box this was checked against runs
        # in /data/relkit, so a stale unit value must never outvote the roots an
        # operator actually registered.
        cfg, note = deploy_cli.backfill_product_root_parent(
            {},
            working_directory="/srv/relkit",
            product_roots={"loom": "/data/relkit/loom", "svn": "/data/relkit/svn-auto-merge"},
        )
        self.assertEqual(cfg["productRootParent"], "/data/relkit")
        self.assertIn("registered products", note)
        self.assertIn("/srv/relkit", note)
        self.assertIn("not there", note)

    def test_working_directory_decides_when_no_product_is_registered(self):
        cfg, note = deploy_cli.backfill_product_root_parent(
            {}, working_directory="/data/relkit", product_roots={}
        )
        self.assertEqual(cfg["productRootParent"], "/data/relkit")
        self.assertIn("WorkingDirectory", note)

    def test_scattered_products_fall_back_to_the_unit(self):
        cfg, note = deploy_cli.backfill_product_root_parent(
            {},
            working_directory="/data/relkit",
            product_roots={"a": "/data/relkit/a", "b": "/srv/b"},
        )
        self.assertEqual(cfg["productRootParent"], "/data/relkit")
        self.assertIn("WorkingDirectory", note)

    def test_configured_value_is_never_overwritten(self):
        cfg, note = deploy_cli.backfill_product_root_parent(
            {"productRootParent": "/data/relkit"},
            working_directory="/srv/relkit",
            product_roots={"loom": "/other/loom"},
        )
        self.assertEqual(cfg["productRootParent"], "/data/relkit")
        self.assertEqual(note, "")

    def test_trailing_slash_is_normalised(self):
        cfg, _ = deploy_cli.backfill_product_root_parent(
            {}, working_directory="/data/relkit/", product_roots={}
        )
        self.assertEqual(cfg["productRootParent"], "/data/relkit")

    def test_nothing_to_go_on_leaves_the_key_absent(self):
        cfg, note = deploy_cli.backfill_product_root_parent(
            {}, working_directory="", product_roots={}
        )
        self.assertNotIn("productRootParent", cfg)
        self.assertTrue(note)

    def test_scattered_products_and_no_unit_write_nothing(self):
        cfg, note = deploy_cli.backfill_product_root_parent(
            {},
            working_directory="",
            product_roots={"a": "/data/relkit/a", "b": "/srv/b"},
        )
        self.assertNotIn("productRootParent", cfg)
        self.assertIn("share no parent", note)

    def test_common_parent_matches_the_agent_rule(self):
        self.assertEqual(
            deploy_cli._common_parent(["/data/relkit/a", "/data/relkit/b"]), "/data/relkit"
        )
        self.assertEqual(deploy_cli._common_parent(["/data/relkit/a", "/srv/b"]), "")
        self.assertEqual(deploy_cli._common_parent([]), "")
        self.assertEqual(deploy_cli._common_parent(["", None]), "")

    def test_products_already_on_disk_are_read_even_with_nothing_to_merge(self):
        """The epoch-finished case: config has no products, so does this run.

        Reading only the merge input therefore finds nothing, and the backfill
        falls back to the unit's WorkingDirectory. On the live box that file is
        /data/relkit while the unit said /srv/relkit, so the regression is
        silent: a plausible path lands in the config, just not the right one.
        """
        roots = deploy_cli.registered_products_for_backfill(
            config_products={},
            existing_admin={
                "products": [
                    {"id": "loom", "root": "/data/relkit/loom"},
                    {"id": "svn-auto-merge", "root": "/data/relkit/svn-auto-merge"},
                ]
            },
            merged_admin={},
        )
        self.assertEqual(
            roots, {"loom": "/data/relkit/loom", "svn-auto-merge": "/data/relkit/svn-auto-merge"}
        )
        cfg, note = deploy_cli.backfill_product_root_parent(
            {}, working_directory="/srv/relkit", product_roots=roots
        )
        self.assertEqual(cfg["productRootParent"], "/data/relkit")
        self.assertIn("registered products", note)

    def test_all_three_sources_contribute_and_later_ones_win(self):
        roots = deploy_cli.registered_products_for_backfill(
            config_products={"loom": "/srv/relkit/loom", "legacy": "/srv/relkit/legacy"},
            existing_admin={"products": [{"id": "loom", "root": "/data/relkit/loom"}]},
            merged_admin={"products": [{"id": "fresh", "root": "/data/relkit/fresh"}]},
        )
        # The admin document is where registration really lives now, so it
        # overrides a stale config entry for the same id instead of losing.
        self.assertEqual(roots["loom"], "/data/relkit/loom")
        self.assertEqual(roots["legacy"], "/srv/relkit/legacy")
        self.assertEqual(roots["fresh"], "/data/relkit/fresh")

    def test_malformed_admin_document_yields_no_products(self):
        for doc in ({}, {"products": None}, {"products": [None, {}, {"id": "x"}]}, []):
            self.assertEqual(deploy_cli.admin_document_products(doc), {})


class AgentUpgradeBackfillWiringTests(unittest.TestCase):
    """The backfill must be fed from disk, not only from this run's merge."""

    def test_apply_agent_upgrade_reads_the_existing_admin_file(self):
        source = inspect.getsource(deploy_cli.apply_agent_upgrade)
        self.assertIn("registered_products_for_backfill", source)
        # Reading it only inside the "is there anything to merge" branch is the
        # exact shape of the bug this guards.
        before_merge = source.split("if admin_doc.get(\"products\") or admin_doc.get(\"tokens\")")[0]
        self.assertIn("existing_admin", before_merge)

    def test_console_upgrade_does_not_read_the_agent_unit(self):
        """It was copied there by accident and never used; it also reads a file
        belonging to a different service, which misleads the next reader."""
        source = inspect.getsource(deploy_cli.apply_console_upgrade)
        self.assertNotIn("live_working_dir", source)
        self.assertNotIn("relkit-agent.service", source)


class ConsoleAdminPathTests(unittest.TestCase):
    """The panel writes one file, in its own state directory, never the tree."""

    def test_unset_setting_is_the_state_directory(self):
        self.assertEqual(
            str(deploy_cli.console_admin_path({})),
            deploy_cli.CONSOLE_STATE_FILE,
        )

    def test_absolute_setting_is_used_as_is(self):
        cfg = {"adminStateFile": "/opt/panel/panel.json"}
        self.assertEqual(str(deploy_cli.console_admin_path(cfg)), "/opt/panel/panel.json")

    def test_default_location_needs_no_extra_write_paths(self):
        self.assertEqual(deploy_cli.console_read_write_paths({}), [])

    def test_deliberate_location_is_writable(self):
        cfg = {"adminStateFile": "/opt/panel/panel.json"}
        self.assertEqual(deploy_cli.console_read_write_paths(cfg), ["/opt/panel"])

    def test_legacy_files_cover_every_old_layout(self):
        cfg = {"dir": "/data/relkit-serve", "adminStateFile": ".relkit-serve-admin.json"}
        files = [
            p.as_posix()
            for p in deploy_cli.console_legacy_panel_files(cfg, "/etc/relkit-console/relkit-console.json")
        ]
        self.assertEqual(files[0], "/etc/relkit-console/.relkit-serve-admin.json")
        self.assertIn("/data/relkit-serve/.relkit-serve-admin.json", files)
        self.assertIn("/data/relkit-serve/.relkit-console-panel.json", files)
        self.assertEqual(len(files), len(set(files)))

    def test_upgrade_parser_exposes_console_switches(self):
        args = deploy_cli.build_parser().parse_args(["upgrade", "--host", "box", "--apply"])
        self.assertEqual(args.console_binary, "dist/relkit-console-linux-amd64")
        self.assertFalse(args.no_console)

    def test_install_console_takes_a_store_not_a_tree(self):
        args = deploy_cli.build_parser().parse_args(
            ["install", "console", "--binary", "b", "--store", "http://127.0.0.1:8080"]
        )
        self.assertEqual(args.store, "http://127.0.0.1:8080")
        self.assertFalse(hasattr(args, "dir"))
        self.assertFalse(hasattr(args, "state_dir"))


class ConsoleConfigNormalizeTests(unittest.TestCase):
    """The live config must decode on the new binary, which is strict."""

    LIVE = {
        "addr": "127.0.0.1:8081",
        "dir": "/data/relkit-serve",
        "adminStateFile": "/data/relkit-serve/.relkit-serve-admin.json",
        "stateDir": "/data/relkit-agent",
        "agent": "http://127.0.0.1:8787",
    }

    def test_intranet_config_moves_onto_the_store(self):
        updated, notes = deploy_cli.normalize_console_config(dict(self.LIVE), "http://127.0.0.1:8080")
        self.assertEqual(
            updated,
            {"addr": "127.0.0.1:8081", "agent": "http://127.0.0.1:8787", "store": "http://127.0.0.1:8080"},
        )
        self.assertTrue(notes)

    def test_retired_keys_are_dropped(self):
        updated, _ = deploy_cli.normalize_console_config(dict(self.LIVE), "http://127.0.0.1:8080")
        for key in deploy_cli.CONSOLE_RETIRED_KEYS:
            self.assertNotIn(key, updated)

    def test_chosen_store_is_kept(self):
        cfg = {"store": "https://store.example.invalid"}
        updated, notes = deploy_cli.normalize_console_config(cfg, "http://127.0.0.1:8080")
        self.assertEqual(updated["store"], "https://store.example.invalid")
        self.assertEqual(notes, [])

    def test_relative_state_file_is_dropped(self):
        cfg = {"store": "http://s", "adminStateFile": ".relkit-serve-admin.json"}
        updated, _ = deploy_cli.normalize_console_config(cfg, "")
        self.assertNotIn("adminStateFile", updated)

    def test_state_file_outside_the_tree_is_kept(self):
        cfg = {"store": "http://s", "dir": "/data/t", "adminStateFile": "/opt/panel/panel.json"}
        updated, _ = deploy_cli.normalize_console_config(cfg, "")
        self.assertEqual(updated["adminStateFile"], "/opt/panel/panel.json")

    def test_second_run_is_idempotent(self):
        once, _ = deploy_cli.normalize_console_config(dict(self.LIVE), "http://127.0.0.1:8080")
        twice, notes = deploy_cli.normalize_console_config(once, "http://127.0.0.1:8080")
        self.assertEqual(notes, [])
        self.assertEqual(once, twice)


class ListenerOwnershipTests(unittest.TestCase):
    """A green health check must not come from a different process.

    relkit-store crash-looped on "address already in use" while a retired
    relkit-serve process answered every probe. Curling the port returned 200 the
    whole time, so the upgrade reported success against a dead service.
    """

    def _ss_output(self, lines):
        return subprocess.CompletedProcess(args=["ss"], returncode=0, stdout="\n".join(lines), stderr="")

    def test_listener_pid_reads_the_socket_owner(self):
        fake = self._ss_output(
            [
                'LISTEN 0 4096 *:8080 *:* users:(("relkit-serve",pid=67135,fd=7))',
                'LISTEN 0 511 127.0.0.1:18080 0.0.0.0:* users:(("node",pid=3773622,fd=18))',
            ]
        )
        original = deploy_cli.run
        deploy_cli.run = lambda *a, **k: fake
        try:
            self.assertEqual(deploy_cli.listener_pid("8080"), "67135")
        finally:
            deploy_cli.run = original

    def test_listener_pid_is_empty_when_nothing_listens(self):
        fake = self._ss_output(['LISTEN 0 511 127.0.0.1:18080 0.0.0.0:* users:(("node",pid=1,fd=18))'])
        original = deploy_cli.run
        deploy_cli.run = lambda *a, **k: fake
        try:
            self.assertEqual(deploy_cli.listener_pid("8080"), "")
        finally:
            deploy_cli.run = original

    def test_foreign_process_on_the_port_is_rejected(self):
        original_run = deploy_cli.run
        original_state = deploy_cli.unit_state
        calls = {"ss": 0}

        def fake_run(argv, **kwargs):
            if argv[:3] == ["ss", "-lntpH"]:
                return self._ss_output(['LISTEN 0 4096 *:8080 *:* users:(("relkit-serve",pid=67135,fd=7))'])
            return subprocess.CompletedProcess(args=argv, returncode=0, stdout="/usr/local/bin/relkit-serve -config x", stderr="")

        deploy_cli.run = fake_run
        deploy_cli.unit_state = lambda unit: {"MainPID": "999", "ActiveState": "activating"}
        try:
            with self.assertRaises(ops.Fail if hasattr(ops, "Fail") else Exception) as caught:
                deploy_cli.require_listener_belongs_to("relkit-store", ":8080")
            self.assertIn("67135", str(caught.exception))
        finally:
            deploy_cli.run = original_run
            deploy_cli.unit_state = original_state

    def test_own_process_on_the_port_passes(self):
        original_run = deploy_cli.run
        original_state = deploy_cli.unit_state

        def fake_run(argv, **kwargs):
            return self._ss_output(['LISTEN 0 4096 *:8080 *:* users:(("relkit-store",pid=4242,fd=7))'])

        deploy_cli.run = fake_run
        deploy_cli.unit_state = lambda unit: {"MainPID": "4242", "ActiveState": "active"}
        try:
            deploy_cli.require_listener_belongs_to("relkit-store", ":8080")
        finally:
            deploy_cli.run = original_run
            deploy_cli.unit_state = original_state

    def test_listener_is_polled_until_the_socket_binds(self):
        """A process reaches "active" before it binds; an instant read would lie."""
        original_run = deploy_cli.run
        original_state = deploy_cli.unit_state
        original_sleep = deploy_cli.time.sleep
        reads = {"count": 0}

        def fake_run(argv, **kwargs):
            reads["count"] += 1
            if reads["count"] < 3:
                return self._ss_output([])  # nothing bound yet
            return self._ss_output(['LISTEN 0 4096 *:8080 *:* users:(("relkit-store",pid=4242,fd=7))'])

        deploy_cli.run = fake_run
        deploy_cli.unit_state = lambda unit: {"MainPID": "4242", "ActiveState": "active"}
        deploy_cli.time.sleep = lambda seconds: None
        try:
            deploy_cli.require_listener_belongs_to("relkit-store", ":8080")
            self.assertGreaterEqual(reads["count"], 3)
        finally:
            deploy_cli.run = original_run
            deploy_cli.unit_state = original_state
            deploy_cli.time.sleep = original_sleep

    def test_a_port_that_never_binds_is_reported(self):
        original_run = deploy_cli.run
        original_state = deploy_cli.unit_state
        original_sleep = deploy_cli.time.sleep
        deploy_cli.run = lambda argv, **kwargs: self._ss_output([])
        deploy_cli.unit_state = lambda unit: {"MainPID": "4242", "ActiveState": "active"}
        deploy_cli.time.sleep = lambda seconds: None
        try:
            with self.assertRaises(Exception) as caught:
                deploy_cli.require_listener_belongs_to("relkit-store", ":8080")
            self.assertIn("nothing is listening", str(caught.exception))
        finally:
            deploy_cli.run = original_run
            deploy_cli.unit_state = original_state
            deploy_cli.time.sleep = original_sleep

    def test_crash_looping_unit_is_not_reported_running(self):
        original_state = deploy_cli.unit_state
        original_sleep = deploy_cli.time.sleep
        deploy_cli.unit_state = lambda unit: {
            "ActiveState": "activating",
            "SubState": "auto-restart",
            "MainPID": "0",
            "NRestarts": "30",
        }
        deploy_cli.time.sleep = lambda seconds: None
        try:
            with self.assertRaises(Exception) as caught:
                deploy_cli.require_unit_running("relkit-store")
            self.assertIn("activating", str(caught.exception))
        finally:
            deploy_cli.unit_state = original_state
            deploy_cli.time.sleep = original_sleep


class BackupRotationTests(unittest.TestCase):
    def test_backup_skips_regenerable_staged_cache(self):
        """An upgrade snapshot must not carry the publish payload artifacts.

        Every apply used to copy products/.relkit/cache/staged wholesale, which
        made each 12MB snapshot 250MB and grew /var/backups/relkit without
        bound. The cache is rebuilt on demand, so it is not backup material.
        """
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            products = root / "products"
            staged = products / ".relkit" / "cache" / "staged" / "0.1.0+14" / "artifacts"
            staged.mkdir(parents=True)
            (staged / "payload.zip").write_bytes(b"x" * 4096)
            (products / "dec.json").write_text('{"product":"dec"}\n', encoding="utf-8")

            backup = root / "backup"
            deploy_cli.backup_files(backup / "agent", [products])

            copied = backup / "agent" / "products"
            self.assertTrue((copied / "dec.json").is_file(), "config must be backed up")
            self.assertFalse(
                (copied / ".relkit").exists(),
                "regenerable .relkit/cache must be skipped",
            )

    def test_backup_keeps_unrelated_cache_named_directories(self):
        """Ignore rules match the relative path, not the bare directory name."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            products = root / "products"
            (products / "cache").mkdir(parents=True)
            (products / "cache" / "keep.json").write_text("{}\n", encoding="utf-8")
            (products / ".relkit" / "cache").mkdir(parents=True)
            (products / ".relkit" / "cache" / "drop.bin").write_bytes(b"x")

            backup = root / "backup"
            deploy_cli.backup_files(backup / "agent", [products])

            copied = backup / "agent" / "products"
            self.assertTrue((copied / "cache" / "keep.json").is_file())
            self.assertFalse((copied / ".relkit").exists())

    def test_prune_keeps_newest_snapshots(self):
        """In normal use the current snapshot is the newest, so keep=N means N."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            names = ["20260901T000000Z", "20260902T000000Z", "20260903T000000Z", "20260904T000000Z"]
            for name in names:
                (root / name).mkdir()
            current = root / names[-1]

            removed = deploy_cli.prune_backups(root, 2, current=current)

            self.assertEqual(
                sorted(path.name for path in root.iterdir()),
                ["20260903T000000Z", "20260904T000000Z"],
            )
            self.assertEqual(sorted(removed), ["20260901T000000Z", "20260902T000000Z"])

    def test_prune_never_deletes_current(self):
        """A protected snapshot survives even when it is the oldest one.

        Rotation runs right after a successful apply, where current is the
        newest snapshot. Protecting it matters for the failure path: an apply
        that breaks and rolls back must not find its own backup already gone.
        """
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            names = ["20260901T000000Z", "20260902T000000Z", "20260903T000000Z"]
            for name in names:
                (root / name).mkdir()
            current = root / names[0]

            deploy_cli.prune_backups(root, 1, current=current)

            self.assertTrue(current.is_dir(), "the protected snapshot must never be deleted")

    def test_prune_ignores_foreign_entries(self):
        """A hand-made tarball is not a rotation candidate."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "20260901T000000Z").mkdir()
            (root / "20260902T000000Z").mkdir()
            foreign = root / "manual-export.tar.gz"
            foreign.write_bytes(b"keep me")

            removed = deploy_cli.prune_backups(root, 1)

            self.assertTrue(foreign.is_file(), "foreign files must survive rotation")
            self.assertEqual(removed, ["20260901T000000Z"])

    def test_prune_rejects_non_positive_keep(self):
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(deploy_cli.Fail):
                deploy_cli.prune_backups(Path(tmp), 0)

    def test_prune_dry_run_matches_real_run(self):
        """A preview must name exactly what the real run would delete."""
        def build(root: Path) -> None:
            for name in ["20260901T000000Z", "20260902T000000Z", "20260903T000000Z"]:
                (root / name).mkdir()

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            build(root)
            preview = deploy_cli.prune_backups(root, 1, dry_run=True)
            self.assertEqual(len(list(root.iterdir())), 3, "dry run must delete nothing")

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            build(root)
            actual = deploy_cli.prune_backups(root, 1)

        self.assertEqual(preview, actual)

    def test_upgrade_defaults_keep_five_snapshots(self):
        args = deploy_cli.build_parser().parse_args(["upgrade", "--host", "box", "--plan"])
        self.assertEqual(args.keep_backups, 5)
        self.assertFalse(args.no_prune_backups)

    def test_prune_backups_parser(self):
        args = deploy_cli.build_parser().parse_args(["prune-backups", "--host", "box", "--keep", "3", "--dry-run"])
        self.assertEqual(args.keep, 3)
        self.assertTrue(args.dry_run)


class ToolPublishTests(unittest.TestCase):
    """Requirement 12.6: a release ships tools, and the server signs them."""

    def test_upgrade_publishes_tools_by_default(self):
        args = deploy_cli.build_parser().parse_args(["upgrade", "--host", "box", "--plan"])
        self.assertFalse(args.no_tools, "publishing tools must be the default; opting out is the exception")

    def test_no_tools_flag_opts_out(self):
        args = deploy_cli.build_parser().parse_args(["upgrade", "--host", "box", "--no-tools"])
        self.assertTrue(args.no_tools)

    def test_tools_publish_has_no_base_url_flag(self):
        """The agent signs its own publicURL; a caller cannot pick the domain."""
        with self.assertRaises(SystemExit), contextlib.redirect_stderr(io.StringIO()):
            deploy_cli.build_parser().parse_args(
                ["tools", "publish", "--server", "http://127.0.0.1:8787", "--base-url", "http://update-internal.example"]
            )

    def test_token_path_sits_beside_the_agent_config(self):
        path = deploy_cli.tool_publish_token_path(Path("/etc/relkit-agent"))
        self.assertEqual(path.as_posix(), "/etc/relkit-agent/tools-publish.token")

    def test_token_is_minted_once_and_then_reused(self):
        """A working release pipeline must keep working across upgrades."""
        with tempfile.TemporaryDirectory() as tmp:
            config_dir = Path(tmp)
            path, token, minted = deploy_cli.ensure_tool_publish_token(config_dir, user="")
            self.assertTrue(minted)
            self.assertEqual(path.read_text(encoding="utf-8").strip(), token)
            self.assertGreaterEqual(len(token), 32)
            again, token2, minted2 = deploy_cli.ensure_tool_publish_token(config_dir, user="")
            self.assertFalse(minted2)
            self.assertEqual(token2, token, "an existing credential must be reused, not rotated")
            self.assertEqual(again, path)

    def test_rotation_replaces_the_secret(self):
        with tempfile.TemporaryDirectory() as tmp:
            config_dir = Path(tmp)
            _, first, _ = deploy_cli.ensure_tool_publish_token(config_dir, user="")
            _, second, minted = deploy_cli.ensure_tool_publish_token(config_dir, user="", rotate=True)
            self.assertTrue(minted)
            self.assertNotEqual(first, second)

    def test_token_file_is_not_world_readable(self):
        with tempfile.TemporaryDirectory() as tmp:
            path, _, _ = deploy_cli.ensure_tool_publish_token(Path(tmp), user="")
            if os.name == "nt":
                self.skipTest("POSIX modes are not reported on Windows")
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)

    def test_no_leftover_temp_file(self):
        """The write must be atomic: a crash must not leave a half-written token."""
        with tempfile.TemporaryDirectory() as tmp:
            config_dir = Path(tmp)
            deploy_cli.ensure_tool_publish_token(config_dir, user="")
            self.assertEqual([p.name for p in config_dir.iterdir()], ["tools-publish.token"])

    def test_publishing_a_missing_artifact_fails_loudly(self):
        with self.assertRaises(deploy_cli.Fail):
            deploy_cli.publish_release_tools(
                files=[{"path": "/nonexistent/cli-linux-amd64", "component": "cli"}],
                version="0.7.0+abc",
                commit="abc",
                user="",
                restart=False,
            )

    def test_publishing_needs_a_version(self):
        with self.assertRaises(deploy_cli.Fail):
            deploy_cli.publish_release_tools(
                files=[{"path": "/nonexistent/cli-linux-amd64", "component": "cli"}],
                version="",
                commit="abc",
                user="",
                restart=False,
            )

    def test_no_tool_files_means_no_work(self):
        self.assertEqual(
            deploy_cli.publish_release_tools(
                files=[], version="", commit="", user="", restart=False
            ),
            [],
        )

    def test_publish_helper_ships_with_the_remote_cli(self):
        """The helper runs on the server, so it must travel with the CLI."""
        names = [dest for _, dest in deploy_cli.remote_bootstrap_sources()]
        sources = [str(Path(src).name) if not hasattr(src, "name") else src.name for src, _ in deploy_cli.remote_bootstrap_sources()]
        self.assertIn("relkit_tools_publish.py", sources)


class BuildAppendTests(unittest.TestCase):
    def test_build_accepts_append(self):
        args = deploy_cli.build_parser().parse_args(["build", "--cli", "--append", "--os", "linux"])
        self.assertTrue(args.append)
        self.assertTrue(args.cli)


if __name__ == "__main__":
    unittest.main()
