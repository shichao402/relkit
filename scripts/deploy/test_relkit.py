#!/usr/bin/env python3
"""Tests for scripts/deploy/relkit_ops.py and CLI parsing."""

from __future__ import annotations

import inspect
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

import relkit_ops as ops  # noqa: E402
import relkit as deploy_cli  # noqa: E402
from hostlib.facets import (
    BY_NAME,
    HOST_SOURCE_SUFFIXES,
    Component,
    TARGETS,
    packed_host_surface_errors,
    validate_components,
    with_component,
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
            "statsFile": "/var/lib/stats.json",
            "adminStateFile": "/data/relkit-store/.relkit-serve-admin.json",
        }
        paths = ops.read_write_paths(cfg)
        self.assertEqual(paths[0], "/data/relkit-store")
        self.assertIn("/var/lib/stats.json", paths)

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

    def test_migrate_http_put_needs_public_base(self):
        backend = {"type": "http-put", "baseUrl": "https://update.example/"}
        with self.assertRaises(ValueError):
            ops.migrate_backend(
                {"type": "http-put", "outputDir": "/data"},
                serve_addr="127.0.0.1:8080",
                public_base_url=None,
            )
        migrated, notes = ops.migrate_backend(
            backend,
            serve_addr="127.0.0.1:8080",
            public_base_url=None,
        )
        self.assertEqual(migrated["type"], "relkit-compatible")
        self.assertEqual(migrated["tokenEnv"], "RELKIT_SERVE_TOKEN")
        self.assertTrue(any("http-put" in item for item in notes))

    def test_migrate_rejects_static_http(self):
        with self.assertRaises(ValueError) as raised:
            ops.migrate_backend(
                {"type": "static-http", "baseUrl": "https://example.invalid/"},
                serve_addr="127.0.0.1:8080",
                public_base_url=None,
            )
        self.assertIn("static-http", str(raised.exception))

    def test_migrate_local_with_public_url(self):
        profile = {
            "backends": {
                "intranet": {
                    "type": "local",
                    "outputDir": "/data/relkit-store",
                    "casCredentials": {"mode": "sts"},
                }
            }
        }
        out, notes = ops.migrate_profile(
            profile,
            serve_addr="127.0.0.1:8080",
            public_base_url="https://update.devcloud.woa.com/",
            public_upload_url="http://update.devcloud.woa.com/",
        )
        b = out["backends"]["intranet"]
        self.assertEqual(b["type"], "relkit-compatible")
        self.assertEqual(b["baseUrl"], "https://update.devcloud.woa.com/")
        self.assertEqual(b["uploadUrl"], "http://update.devcloud.woa.com/")
        self.assertNotIn("casCredentials", b)
        self.assertTrue(notes)

    def test_public_upload_url_replaces_compatible_loopback(self):
        backend = {
            "type": "relkit-compatible",
            "baseUrl": "https://update.devcloud.woa.com/",
            "uploadUrl": "http://127.0.0.1:8080/",
            "tokenEnv": "RELKIT_SERVE_TOKEN",
        }
        migrated, notes = ops.migrate_backend(
            backend,
            serve_addr="127.0.0.1:8080",
            public_base_url="http://update.devcloud.woa.com:8080",
            public_upload_url="http://update.devcloud.woa.com:8080",
        )
        self.assertEqual(
            migrated["baseUrl"], "http://update.devcloud.woa.com:8080/"
        )
        self.assertEqual(
            migrated["uploadUrl"], "http://update.devcloud.woa.com:8080/"
        )
        self.assertTrue(any("set baseUrl" in item for item in notes))
        self.assertTrue(any("set uploadUrl" in item for item in notes))

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
            public_base_url=None,
        )
        self.assertNotIn("site", migrated)
        self.assertTrue(any("site.sinks" in item for item in notes))

    def test_unknown_backend_left_alone(self):
        backend = {"type": "s3-compatible", "bucket": "x"}
        out, notes = ops.migrate_backend(
            backend, serve_addr=":8080", public_base_url=None
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

    def test_remote_bootstrap_includes_hostlib_facets(self):
        sources = deploy_cli.remote_bootstrap_sources()
        names = [(path.name, dest) for path, dest in sources]
        self.assertIn(("relkit.py", ""), names)
        self.assertIn(("relkit_ops.py", ""), names)
        self.assertIn(("__init__.py", "hostlib"), names)
        self.assertIn(("facets.py", "hostlib"), names)
        for path, _dest in sources:
            self.assertTrue(path.is_file(), path)


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
        self.assertEqual(BY_NAME["host-scripts"].pack_kind(), "working-tree")
        self.assertEqual(BY_NAME["sdk-go"].pack_kind(), "go-deps")
        self.assertEqual(BY_NAME["sdk-rust"].pack_kind(), "tracked-tree")
        self.assertEqual(BY_NAME["bindings-ts"].pack_kind(), "listed-files")
        self.assertEqual(BY_NAME["sdk-dart"].pack_kind(), "tracked-tree")
        for row in deploy_cli.registry_components("product-tree"):
            names = {name for _, name in deploy_cli.component_entries(row.name)}
            for required in row.required_paths:
                self.assertTrue(
                    required in names or any(
                        name == required or name.startswith(required.rstrip("/") + "/")
                        for name in names
                    ),
                    f"{row.name} packed files missing {required}",
                )

    def test_host_zips_omit_in_process_updater(self):
        dart = {name for _, name in deploy_cli.component_entries("sdk-dart")}
        self.assertNotIn("lib/src/updater.dart", dart)
        self.assertNotIn("lib/src/scheduler.dart", dart)
        self.assertFalse(any(name.startswith("test/") for name in dart))
        go = {name for _, name in deploy_cli.component_entries("sdk-go")}
        self.assertNotIn("sdk/updater.go", go)
        self.assertFalse(any(name.startswith("internal/inprocess/") for name in go))
        self.assertFalse(any(name.startswith("internal/updater/") for name in go))
        self.assertTrue(any(name.startswith("sdk/") for name in go))
        self.assertTrue(any(name.startswith("sdk/updaterfacade/") for name in go))
        self.assertTrue(any(name.startswith("internal/ipc/") for name in go))
        for row in deploy_cli.registry_components("product-tree"):
            if not row.updater_process:
                continue
            entries = deploy_cli.component_entries(row.name)
            names = [name for _, name in entries]
            texts = {
                name: source.read_text(encoding="utf-8", errors="ignore")
                for source, name in entries
                if source.suffix.lower() in HOST_SOURCE_SUFFIXES
                and source.is_file()
            }
            self.assertEqual(packed_host_surface_errors(row, names, texts), ())
            self.assertIn("facade", row.host_api)
            self.assertTrue(row.facade_paths)

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
        self.assertEqual(BY_NAME["cli"].install_name("windows-amd64"), "relkit.exe")
        self.assertEqual(BY_NAME["cli"].install_name("linux-amd64"), "relkit-linux-amd64")
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

    def test_fictional_facade_is_one_validated_registry_row(self):
        row = Component(
            "sdk-zig", "product-tree", "zig-sdk", "relkit-sdk-zig.zip",
            "sdk/zig", "third_party/relkit/sdk/zig", "files",
            ("build.zig",), portable=True, detect=("build.zig",),
            updater_process="zig", import_signals=("@import(\"relkit\")",),
            host_api=("facade",), facade_paths=("src/lib.zig",),
        )
        rows = with_component(row)
        added = rows[-1]
        self.assertEqual(added.build_flag, "zig-sdk")
        self.assertEqual(added.name, "sdk-zig")
        self.assertEqual(added.destination, "third_party/relkit/sdk/zig")
        self.assertEqual(added.updater_process, "zig")
        with self.assertRaisesRegex(ValueError, "updater_process requires facade_paths"):
            validate_components([
                Component(
                    "sdk-nim", "product-tree", "nim-sdk", "relkit-sdk-nim.zip",
                    "sdk/nim", "third_party/relkit/sdk/nim", "files",
                    ("src/lib.nim",), portable=True, updater_process="nim",
                    host_api=("facade",),
                )
            ])
        dart = BY_NAME["sdk-dart"]
        self.assertEqual(
            packed_host_surface_errors(
                dart, dart.facade_paths, {dart.facade_paths[0]: "class Updater {}"},
            ),
            (),
        )
        self.assertTrue(
            packed_host_surface_errors(
                dart, dart.facade_paths, {"lib/src/updater.dart": "class RupUpdater {}"},
            )
        )
        self.assertTrue(
            packed_host_surface_errors(dart, ("lib/src/other.dart",), {})
        )
        with self.assertRaisesRegex(ValueError, "listed-files pack requires pack_files"):
            validate_components([
                Component(
                    "sdk-bad", "product-tree", "bad-sdk", "relkit-sdk-bad.zip",
                    "sdk/bad", "third_party/relkit/sdk/bad", "files",
                    ("x",), portable=True, pack="listed-files",
                )
            ])

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

    def test_host_scripts_archive_has_stable_tree_hash(self):
        args = deploy_cli.build_parser().parse_args(["build", "--host-scripts"])
        self.assertTrue(args.host_scripts)
        entries = deploy_cli.host_script_entries()
        names = [name for _, name in entries]
        self.assertIn("relkit_consume.py", names)
        self.assertIn("relkit_host.py", names)
        self.assertIn("hostlib/facets.py", names)
        self.assertFalse(any("__pycache__" in name for name in names))
        first = deploy_cli.host_scripts_tree_sha256(entries)
        second = deploy_cli.host_scripts_tree_sha256(entries)
        self.assertEqual(first, second)
        self.assertEqual(len(first), 64)

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
    def test_render_console_unit_rewrites_exec_and_paths(self):
        template = (DEPLOY / "relkit-console.service").read_text(encoding="utf-8")
        unit = ops.render_console_unit(
            template,
            user="relkit",
            prefix="/usr/local/bin",
            config_path="/etc/relkit-console/relkit-console.json",
            read_write_paths=["/data/relkit-serve"],
            state_dir="/data/relkit-agent",
        )
        self.assertIn(
            "ExecStart=/usr/local/bin/relkit-console -config /etc/relkit-console/relkit-console.json",
            unit,
        )
        self.assertIn("ReadWritePaths=/data/relkit-serve", unit)
        self.assertIn("User=relkit", unit)
        self.assertNotIn("/srv/releases", unit.split("ReadWritePaths=")[1].splitlines()[0])

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


if __name__ == "__main__":
    unittest.main()
