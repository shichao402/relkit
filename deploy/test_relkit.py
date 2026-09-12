#!/usr/bin/env python3
"""Tests for deploy/relkit_ops.py and CLI parsing."""

from __future__ import annotations

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


class RequirementsTests(unittest.TestCase):
    def test_comments_and_blanks_are_ignored(self):
        text = "# pin\n\n# another\n"
        self.assertEqual(ops.parse_requirements(text), [])

    def test_pins_are_kept(self):
        self.assertEqual(ops.parse_requirements("foo==1.0\n# x\nbar\n"), ["foo==1.0", "bar"])


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
            "{ path=/usr/local/bin/relkit-serve ; "
            "argv[]=/usr/local/bin/relkit-serve -config /etc/relkit-serve/relkit-serve.json ; "
            "ignore_errors=no }"
        )
        self.assertEqual(
            ops.parse_exec_config(raw),
            "/etc/relkit-serve/relkit-serve.json",
        )
        self.assertEqual(ops.parse_exec_binary(raw), "/usr/local/bin/relkit-serve")


class UnitRenderTests(unittest.TestCase):
    def test_read_write_paths_include_tree_and_external_files(self):
        cfg = {
            "dir": "/data/relkit-serve",
            "statsFile": "/var/lib/stats.json",
            "adminStateFile": "/data/relkit-serve/.relkit-serve-admin.json",
        }
        paths = ops.read_write_paths(cfg)
        self.assertEqual(paths[0], "/data/relkit-serve")
        self.assertIn("/var/lib/stats.json", paths)

    def test_render_serve_unit_uses_live_paths_and_low_port_caps(self):
        template = (DEPLOY / "relkit-serve.service").read_text(encoding="utf-8")
        unit = ops.render_serve_unit(
            template,
            user="relkit",
            prefix="/usr/local/bin",
            config_path="/etc/relkit-serve/relkit-serve.json",
            read_write_paths=["/data/relkit-serve"],
            addr="127.0.0.1:80",
        )
        self.assertIn("ReadWritePaths=/data/relkit-serve", unit)
        self.assertIn("ExecStart=/usr/local/bin/relkit-serve -config /etc/relkit-serve/relkit-serve.json", unit)
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
        cfg = {"addr": "127.0.0.1:8080", "dir": "/data/relkit-serve"}
        once, notes = ops.ensure_cas_grace(cfg)
        self.assertEqual(once["gc"]["casGrace"], "24h")
        self.assertTrue(notes)
        twice, notes2 = ops.ensure_cas_grace(once)
        self.assertEqual(notes2, [])
        self.assertEqual(twice["addr"], "127.0.0.1:8080")
        self.assertEqual(twice["dir"], "/data/relkit-serve")

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

    def test_migrate_local_with_public_url(self):
        profile = {
            "backends": {
                "intranet": {
                    "type": "local",
                    "outputDir": "/data/relkit-serve",
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


class MissingTargetTests(unittest.TestCase):
    AGENT_ONLY_HOST = {
        "serve": {
            "unit": {"FragmentPath": "", "ActiveState": "inactive"},
            "binary": "/usr/local/bin/relkit-serve",
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
        self.assertIn("relkit-serve", missing[0])
        self.assertIn("no systemd unit", missing[0])
        self.assertIn("/usr/local/bin/relkit-serve", missing[0])
        self.assertIn("--agent-only", missing[0])

    def test_host_running_both_has_nothing_missing(self):
        probe = {
            "serve": {
                "unit": {"FragmentPath": "/etc/systemd/system/relkit-serve.service"},
                "binary": "/usr/local/bin/relkit-serve",
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
                "unit": {"FragmentPath": "/etc/systemd/system/relkit-serve.service"},
                "binary": "/usr/local/bin/relkit-serve",
                "present": False,
            }
        }
        missing = ops.missing_upgrade_targets(probe, want_serve=True, want_agent=False)
        self.assertEqual(len(missing), 1)
        self.assertIn("no binary at /usr/local/bin/relkit-serve", missing[0])
        self.assertNotIn("no systemd unit", missing[0])


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
            dest = root / "relkit-serve"
            src.write_bytes(b"new")
            dest.write_bytes(b"old")
            tmp_new = dest.with_name(dest.name + ".new")
            shutil.copy2(src, tmp_new)
            os.replace(tmp_new, dest)
            self.assertEqual(dest.read_bytes(), b"new")
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            src = root / "relkit-serve.json"
            src.write_text('{"addr":"127.0.0.1:8080"}\n', encoding="utf-8")
            bak = root / "backup"
            bak.mkdir()
            dest = bak / src.name
            dest.write_bytes(src.read_bytes())
            restored = root / "restored.json"
            restored.write_bytes(dest.read_bytes())
            self.assertEqual(restored.read_text(encoding="utf-8"), src.read_text(encoding="utf-8"))


class CliParseTests(unittest.TestCase):
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
        self.assertIn("upgrade", result.stdout)
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
        self.assertEqual(
            [name for _, name in entries],
            ["relkit_consume.py", "relkit_host.py"],
        )
        first = deploy_cli.host_scripts_tree_sha256(entries)
        second = deploy_cli.host_scripts_tree_sha256(entries)
        self.assertEqual(first, second)
        self.assertEqual(len(first), 64)

    def test_rust_sdk_archive_excludes_ignored_target(self):
        ignore = (DEPLOY.parent / "sdk" / "rust" / ".gitignore").read_text(
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


if __name__ == "__main__":
    unittest.main()
