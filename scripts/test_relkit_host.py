#!/usr/bin/env python3
from __future__ import annotations

import json
import inspect
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent / "host"))
import relkit_host as host


class TreeHashTests(unittest.TestCase):
    def test_stable_for_same_bytes(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "relkit_consume.py").write_text("x\n", encoding="utf-8")
            (root / "relkit_host.py").write_text("host\n", encoding="utf-8")
            first = host.tree_sha256(root)
            second = host.tree_sha256(root)
            self.assertEqual(first, second)
            (root / "README.local").write_text("ignored\n", encoding="utf-8")
            self.assertEqual(first, host.tree_sha256(root))
            (root / "relkit_consume.py").write_text("y\n", encoding="utf-8")
            self.assertNotEqual(first, host.tree_sha256(root))


class StateTests(unittest.TestCase):
    def test_set_marks_later_stale(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            host.set_step(state, "product.id", "verified", "demo")
            host.set_step(state, "serve.register", "verified", "demo")
            host.set_step(state, "product.id", "confirmed", "other", mark_later_stale=True)
            self.assertEqual(state["steps"]["serve.register"]["status"], "stale")

    def test_onboard_start_does_not_confirm_product(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "package.json").write_text("{}\n", encoding="utf-8")
            self.assertEqual(host.cmd_onboard_start(root), 0)
            state = host.load_state(root)
            self.assertEqual(state["steps"]["repo.root"]["status"], "verified")
            self.assertEqual(state["steps"]["product.id"]["status"], "unanswered")
            ignore = (root / ".gitignore").read_text(encoding="utf-8")
            self.assertIn(".relkit/onboarding.local.json", ignore)
            self.assertIn(".relkit/onboarding.md", ignore)
            self.assertTrue(host.projection_path(root).is_file())

    def test_interactive_requires_human_answer_and_can_resume(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "package.json").write_text("{}\n", encoding="utf-8")
            answers = iter(["r", "r", "q"])
            with patch("builtins.print"):
                self.assertEqual(
                    host.cmd_onboard_start(root, interactive=False),
                    0,
                )
                self.assertEqual(
                    host.run_interactive_wizard(root, input_fn=lambda _prompt: next(answers)),
                    0,
                )
            state = host.load_state(root)
            self.assertEqual(state["steps"]["product.id"]["value"], root.name.lower())
            self.assertEqual(state["steps"]["updater.process"]["value"], "node")
            self.assertEqual(state["steps"]["channel.ssot"]["status"], "unanswered")

    def test_recommendation_does_not_choose_backend(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            self.assertIsNone(host.recommended_value(root, state, "backend.kind"))

    def test_nested_tauri_is_detected_as_rust_shell(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "package.json").write_text("{}\n", encoding="utf-8")
            tauri = root / "packages" / "desktop" / "src-tauri"
            tauri.mkdir(parents=True)
            (tauri / "Cargo.toml").write_text("[package]\nname='shell'\n", encoding="utf-8")
            stack = host.detect_stack(root)
            self.assertEqual(stack["languages"], ["rust", "node"])
            self.assertIn("rust-shell", stack["updater"])
            self.assertIsNone(
                host.recommended_value(root, host.default_state(root), "updater.process")
            )
            self.assertEqual(
                host.consume_components(root),
                ["sdk-rust", "cli", "updater"],
            )

    def test_root_src_tauri_selects_rust_sdk(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            tauri = root / "src-tauri"
            tauri.mkdir()
            (tauri / "Cargo.toml").write_text("[package]\nname='loom'\n", encoding="utf-8")
            self.assertEqual(host.detect_stack(root)["languages"], ["rust"])
            self.assertEqual(host.consume_components(root)[0], "sdk-rust")

    def test_updater_process_is_closed_vocab(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            with self.assertRaisesRegex(host.Fail, "must be"):
                host.cmd_onboard_set(root, "updater.process", "electron", None)
            self.assertEqual(host.cmd_onboard_set(root, "updater.process", "rust-shell", None), 0)
            state = host.load_state(root)
            self.assertEqual(state["steps"]["updater.process"]["value"], "rust-shell")
            md = host.projection_path(root).read_text(encoding="utf-8")
            self.assertIn("rust-shell", md)
            self.assertIn("Do not edit", md)

    def test_onboard_reset_requires_yes_and_deletes_relkit_dir(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            host.cmd_onboard_set(root, "product.id", "loom", None)
            self.assertTrue(host.relkit_dir(root).is_dir())
            with self.assertRaisesRegex(host.Fail, "--yes"):
                host.cmd_onboard_reset(root)
            self.assertEqual(host.cmd_onboard_reset(root, yes=True), 0)
            self.assertFalse(host.relkit_dir(root).exists())


class RoutingTests(unittest.TestCase):
    def test_no_args_does_not_mutate(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            code = host.main(["--project-root", str(root)])
            self.assertEqual(code, 0)
            self.assertFalse(host.state_path(root).is_file())

    def test_serve_add_without_execute_fails(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            code = host.main(
                ["--project-root", str(root), "serve", "add", "--product", "loom"]
            )
            self.assertEqual(code, 1)

    def test_keys_gen_without_execute_fails(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            code = host.main(["--project-root", str(root), "keys", "gen"])
            self.assertEqual(code, 1)

    def test_release_checksums_rewrite_rust_sdk_lock(self) -> None:
        artifacts: dict = {}
        host.rewrite_lock_artifacts(
            artifacts,
            "https://example.invalid/v1",
            {"relkit-sdk-rust.zip": "a" * 64},
        )
        self.assertEqual(
            artifacts["sdk-rust"]["url"],
            "https://example.invalid/v1/relkit-sdk-rust.zip",
        )

    def test_serve_restart_requires_execute_and_restart(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            self.assertEqual(
                host.main(["--project-root", str(root), "serve", "restart", "--execute"]),
                1,
            )


class RedactTests(unittest.TestCase):
    def test_redacts_export(self) -> None:
        blob = "export RELKIT_UPLOAD_TOKEN='sekrit'\n"
        self.assertNotIn("sekrit", host.redact_text(blob))
        self.assertEqual(host.extract_export("RELKIT_UPLOAD_TOKEN", blob), "sekrit")

    def test_write_secret_does_not_require_print(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            path = host.write_secret(root, "sekrit")
            self.assertTrue(path.is_file())
            self.assertIn("sekrit", path.read_text(encoding="utf-8"))
            local = host.load_local(root)
            self.assertTrue(local["secretRefs"])


class SshGuardTests(unittest.TestCase):
    def test_ssh_failure_is_fatal(self) -> None:
        fake = subprocess.CompletedProcess(
            args=["ssh"], returncode=255, stdout="", stderr="Permission denied"
        )
        with patch("relkit_host.subprocess.run", return_value=fake):
            with self.assertRaisesRegex(host.Fail, "Do not switch credentials"):
                host.ssh_run("missing-host", ["true"])

    def test_list_products_parser(self) -> None:
        text = (
            "config /etc/relkit-serve\n"
            "uploadTokens\n"
            "  svn-auto-merge            tokens/svn-auto-merge.token\n"
            "  loom                      tokens/loom.token\n"
        )
        self.assertEqual(host.parse_list_products(text), ["svn-auto-merge", "loom"])


class UpgradeManifestTests(unittest.TestCase):
    def test_commit_comes_from_release_manifest_not_github_api(self) -> None:
        commit = "9a3af017b3a3f7ab2f47e3843b9b5297c2775ac1"
        with patch(
            "relkit_host.http_get",
            return_value=json.dumps({"schema": "relkit.release/1", "commit": commit}),
        ) as getter:
            self.assertEqual(
                host.release_commit_from_manifest("https://example.invalid/v0.3.3"),
                commit,
            )
        getter.assert_called_once_with("https://example.invalid/v0.3.3/manifest.json")

    def test_missing_manifest_commit_is_fatal(self) -> None:
        with patch("relkit_host.http_get", return_value=json.dumps({"commit": ""})):
            with self.assertRaisesRegex(host.Fail, "refusing to keep the previous lock commit"):
                host.release_commit_from_manifest("https://example.invalid/v0.3.3")

    def test_upgrade_does_not_call_github_api(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            lock_path = root / "scripts" / "relkit.lock.json"
            lock_path.parent.mkdir(parents=True)
            lock_path.write_text(
                json.dumps(
                    {
                        "schema": host.LOCK_SCHEMA,
                        "release": "v0.3.2",
                        "commit": "4" * 40,
                        "artifacts": {},
                    }
                ),
                encoding="utf-8",
            )
            host_dir = root / "scripts" / "host"
            host_dir.mkdir()
            (host_dir / "relkit_consume.py").write_text("#\n", encoding="utf-8")
            commit = "9a3af017b3a3f7ab2f47e3843b9b5297c2775ac1"

            def fake_get(url: str) -> str:
                if url.endswith("/manifest.json"):
                    return json.dumps(
                        {
                            "commit": commit,
                            "hostScriptsSha256": "a" * 64,
                            "consumerSha256": "b" * 64,
                        }
                    )
                if url.endswith("/SHA256SUMS"):
                    return f"{'c' * 64}  relkit-host-scripts.zip\n"
                raise host.Fail(f"unexpected GET {url}")

            with (
                patch("relkit_host.host_scripts_dir", return_value=host_dir),
                patch("relkit_host.http_get", side_effect=fake_get),
                patch("relkit_host.cmd_install", return_value=0),
            ):
                self.assertEqual(host.cmd_upgrade(root, "v0.3.3"), 0)
            lock = json.loads(lock_path.read_text(encoding="utf-8"))
            self.assertEqual(lock["commit"], commit)
            self.assertEqual(lock["release"], "v0.3.3")
            self.assertEqual(lock["hostScriptsSha256"], "a" * 64)
            self.assertEqual(lock["consumerSha256"], "b" * 64)
            self.assertEqual(
                lock["artifacts"]["host-scripts"]["sha256"],
                "c" * 64,
            )


class InstallGateTests(unittest.TestCase):
    def test_successful_install_leaves_verified_lock_state(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            host_dir = root / "scripts" / "host"
            host_dir.mkdir(parents=True)
            (host_dir / "relkit_consume.py").write_text("# consume\n", encoding="utf-8")
            (host_dir / "relkit_host.py").write_text("# host\n", encoding="utf-8")
            (host_dir / "README.local").write_text("not executable\n", encoding="utf-8")
            lock_path = root / "scripts" / "relkit.lock.json"
            lock_path.write_text(
                json.dumps(
                    {
                        "schema": host.LOCK_SCHEMA,
                        "release": "v1.2.3",
                        "commit": "a" * 40,
                        "hostScriptsSha256": host.tree_sha256(host_dir),
                        "artifacts": {},
                    }
                ),
                encoding="utf-8",
            )
            consume = type("Consume", (), {"main": staticmethod(lambda _argv: 0)})
            with (
                patch("relkit_host.import_consume", return_value=consume),
                patch("relkit_host.consume_components", return_value=[]),
            ):
                self.assertEqual(host.cmd_install(root, []), 0)
            state = host.load_state(root)
            self.assertEqual(state["steps"]["consume.lock"]["status"], "verified")
            self.assertEqual(state["steps"]["consume.lock"]["value"], "v1.2.3")


class AgentProvisionTests(unittest.TestCase):
    def test_rewrites_public_https_to_agent_origin(self) -> None:
        self.assertEqual(
            host.rewrite_agent_backend_url("https://update.devcloud.woa.com/"),
            "http://update.devcloud.woa.com:8080/",
        )

    def test_machine_config_injects_private_key_path(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "relkit.json").write_text(
                json.dumps(
                    {
                        "product": "loom",
                        "backends": {
                            "intranet": {
                                "type": "relkit-compatible",
                                "baseUrl": "https://update.devcloud.woa.com/",
                                "uploadUrl": "https://update.devcloud.woa.com/",
                            }
                        },
                        "publishTo": ["intranet"],
                        "signing": {"keyId": "k1"},
                    }
                ),
                encoding="utf-8",
            )
            cfg = host.machine_publish_config(root, "loom", "k1", ".relkit-keys/k1.private.pb")
            self.assertEqual(cfg["signing"]["privateKeyPath"], ".relkit-keys/k1.private.pb")
            self.assertEqual(
                cfg["backends"]["intranet"]["baseUrl"],
                "https://update.devcloud.woa.com/",
            )
            self.assertEqual(
                cfg["backends"]["intranet"]["uploadUrl"],
                "http://update.devcloud.woa.com:8080/",
            )
            profile = host.extract_publish_profile(cfg)
            self.assertEqual(profile["signing"]["keyId"], "k1")
            self.assertEqual(
                profile["backends"]["intranet"]["uploadUrl"],
                "http://update.devcloud.woa.com:8080/",
            )

    def test_profile_keeps_only_the_makers_token_env_from_site(self) -> None:
        machine = {
            "product": "loom",
            "signing": {"keyId": "k1", "privateKeyPath": ".relkit-keys/k1.private.pb"},
            "backends": {"intranet": {"type": "relkit-compatible"}},
            "publishTo": ["intranet"],
            "directory": {
                "publishTo": ["intranet"],
                "entryUrls": ["https://policy-only.invalid/directory.json"],
            },
            "site": {
                "title": "Loom",
                "description": "blurb",
                "homepage": "https://example.invalid",
            },
        }
        profile = host.extract_publish_profile(machine)
        self.assertNotIn("site", profile)
        self.assertEqual(profile["directory"], {"publishTo": ["intranet"]})

        machine["site"]["makers"] = {"tokenEnv": "MAKERS_TOKEN", "projectId": "x"}
        profile = host.extract_publish_profile(machine)
        self.assertEqual(profile["site"], {"makers": {"tokenEnv": "MAKERS_TOKEN"}})

    def test_provision_refuses_without_execute(self) -> None:
        args = host.build_parser().parse_args(["agent", "provision"])
        with self.assertRaisesRegex(host.Fail, "without --execute"):
            host.cmd_agent_provision(Path("."), args)

    def test_provision_reads_profile_back_and_marks_signing_verified(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / ".relkit-keys").mkdir()
            (root / ".relkit-keys" / "k1.private.pb").write_bytes(b"private")
            (root / "relkit.json").write_text(
                json.dumps(
                    {
                        "product": "loom",
                        "signing": {"keyId": "k1"},
                        "backends": {
                            "intranet": {
                                "type": "relkit-compatible",
                                "baseUrl": "https://example.invalid/",
                                "tokenEnv": "RELKIT_UPLOAD_TOKEN",
                            }
                        },
                        "publishTo": ["intranet"],
                    }
                ),
                encoding="utf-8",
            )
            state = host.default_state(root)
            host.set_step(state, "product.id", "confirmed", "loom", "test")
            host.set_step(state, "signing.keys", "stale", "k1", "test")
            host.save_state(root, state)

            written: dict[str, bytes] = {}

            def fake_write(_host, path, data):
                written[path] = data

            def fake_ssh(_host, command, **_kwargs):
                stdout = ""
                if command[:2] == ["sudo", "cat"]:
                    stdout = written[command[2]].decode("utf-8")
                return subprocess.CompletedProcess(command, 0, stdout, "")

            args = host.build_parser().parse_args(
                ["agent", "provision", "--host", "box", "--execute"]
            )
            with (
                patch("relkit_host.ssh_path_exists", return_value=True),
                patch("relkit_host.ssh_write", side_effect=fake_write),
                patch("relkit_host.ssh_run", side_effect=fake_ssh),
            ):
                self.assertEqual(host.cmd_agent_provision(root, args), 0)

            saved = host.load_state(root)
            self.assertEqual(saved["steps"]["signing.keys"]["status"], "verified")


class SshQuotingTests(unittest.TestCase):
    def test_arguments_with_spaces_survive_the_far_side_shell(self) -> None:
        completed = subprocess.CompletedProcess([], 0, "", "")
        with patch("subprocess.run", return_value=completed) as runner:
            host.ssh_run("box", ["bash", "-lc", "rm -rf /srv/a /srv/b"])
        argv = runner.call_args.args[0]
        self.assertEqual(argv[-1], "'rm -rf /srv/a /srv/b'")
        self.assertEqual(argv[-3:-1], ["bash", "-lc"])

    def test_plain_arguments_are_not_rewritten(self) -> None:
        completed = subprocess.CompletedProcess([], 0, "", "")
        with patch("subprocess.run", return_value=completed) as runner:
            host.ssh_run("box", ["sudo", "systemctl", "restart", "relkit-agent"])
        self.assertEqual(
            runner.call_args.args[0][-4:],
            ["sudo", "systemctl", "restart", "relkit-agent"],
        )


class RemoteClaimTests(unittest.TestCase):
    def claim(self, status: str, products: list[str]) -> tuple[dict[str, Any], list[str]]:
        with tempfile.TemporaryDirectory() as raw:
            state = host.default_state(Path(raw))
        state["steps"]["serve.register"]["status"] = status
        state["steps"]["serve.register"]["value"] = "loom"
        drift: list[str] = []
        host.claim_remote_registration(
            state,
            "serve.register",
            "loom",
            products,
            note="listed on remote",
            missing="serve does not list loom",
            drift=drift,
        )
        return state["steps"]["serve.register"], drift

    def test_stale_local_does_not_hide_remote_evidence(self) -> None:
        step, drift = self.claim("stale", ["loom"])
        self.assertEqual(step["status"], "verified")
        self.assertEqual(drift, [])

    def test_missing_remote_is_drift(self) -> None:
        step, drift = self.claim("verified", ["svn-auto-merge"])
        self.assertEqual(step["status"], "drift")
        self.assertEqual(drift, ["serve does not list loom"])

    def test_unanswered_without_remote_stays_quiet(self) -> None:
        step, drift = self.claim("unanswered", [])
        self.assertEqual(step["status"], "unanswered")
        self.assertEqual(drift, [])


class SigningProfileTests(unittest.TestCase):
    def state_with_key(self, key_id: str) -> dict[str, Any]:
        with tempfile.TemporaryDirectory() as raw:
            state = host.default_state(Path(raw))
        state["steps"]["signing.keys"]["status"] = "stale"
        state["steps"]["signing.keys"]["value"] = key_id
        return state

    def test_matching_key_id_verifies(self) -> None:
        state = self.state_with_key("k1")
        drift: list[str] = []
        completed = subprocess.CompletedProcess([], 0, '{"signing": {"keyId": "k1"}}', "")
        with patch("relkit_host.ssh_run", return_value=completed) as runner:
            self.assertTrue(
                host.reconcile_signing_profile(
                    state, "box", "/etc/relkit-agent/relkit-agent.json", "loom", drift
                )
            )
        runner.assert_called_once_with(
            "box", ["sudo", "cat", "/etc/relkit-agent/products/loom.json"]
        )
        self.assertEqual(state["steps"]["signing.keys"]["status"], "verified")
        self.assertEqual(drift, [])

    def test_key_id_mismatch_is_drift(self) -> None:
        state = self.state_with_key("k1")
        drift: list[str] = []
        completed = subprocess.CompletedProcess([], 0, '{"signing": {"keyId": "k9"}}', "")
        with patch("relkit_host.ssh_run", return_value=completed):
            self.assertFalse(
                host.reconcile_signing_profile(
                    state, "box", "/etc/relkit-agent/relkit-agent.json", "loom", drift
                )
            )
        self.assertEqual(state["steps"]["signing.keys"]["status"], "drift")
        self.assertEqual(len(drift), 1)


class SidecarLayoutTests(unittest.TestCase):
    def test_next_to_shell_verifies(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            state["steps"]["sidecar.layout"]["status"] = "stale"
            (root / "scripts").mkdir()
            (root / "scripts" / "build_desktop_release.mjs").write_text(
                "cpSync(path.join(root, 'tools/bin/relkit-updater.exe'), dest)\n",
                encoding="utf-8",
            )
            bin_dir = root / "tools" / "bin"
            bin_dir.mkdir(parents=True)
            (bin_dir / host.updater_sidecar_name()).write_bytes(b"sidecar")
            dest = root / "dist" / "loom-editor-win"
            dest.mkdir(parents=True)
            (dest / host.updater_sidecar_name()).write_bytes(b"sidecar")
            drift: list[str] = []
            host.reconcile_sidecar_layout(root, state, drift)
            self.assertEqual(state["steps"]["sidecar.layout"]["status"], "verified")
            self.assertEqual(drift, [])

    def test_pack_tree_without_sidecar_is_drift(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            state["steps"]["sidecar.layout"]["status"] = "applied"
            (root / "scripts").mkdir()
            (root / "scripts" / "build_desktop_release.mjs").write_text(
                "tools/bin relkit-updater\n",
                encoding="utf-8",
            )
            bin_dir = root / "tools" / "bin"
            bin_dir.mkdir(parents=True)
            (bin_dir / host.updater_sidecar_name()).write_bytes(b"sidecar")
            (root / "dist" / "loom-editor-win").mkdir(parents=True)
            drift: list[str] = []
            host.reconcile_sidecar_layout(root, state, drift)
            self.assertEqual(state["steps"]["sidecar.layout"]["status"], "drift")
            self.assertEqual(len(drift), 1)


class FakeStageTests(unittest.TestCase):
    def test_staged_tree_restores_applied_not_verified(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            state["steps"]["fake.release"]["status"] = "stale"
            state["steps"]["fake.release"]["value"] = "0.2.2+0"
            tree = root / ".relkit" / "staged" / "0.2.2+0"
            tree.mkdir(parents=True)
            (tree / "staged.pb").write_bytes(b"x")
            host.reconcile_fake_stage(root, state)
            self.assertEqual(state["steps"]["fake.release"]["status"], "applied")
            self.assertEqual(state["steps"]["fake.release"]["value"], "0.2.2+0")

    def test_staged_tree_does_not_downgrade_verified(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            state["steps"]["fake.release"] = {
                "status": "verified",
                "value": "0.2.2+0",
                "note": "simulate passed",
            }
            tree = root / ".relkit" / "staged" / "0.2.2+0"
            tree.mkdir(parents=True)
            (tree / "staged.pb").write_bytes(b"x")
            host.reconcile_fake_stage(root, state)
            self.assertEqual(state["steps"]["fake.release"]["status"], "verified")
            self.assertEqual(state["steps"]["fake.release"]["note"], "simulate passed")


class ReconcileTests(unittest.TestCase):
    def test_release_accepts_confirmed_decisions_but_requires_verified_actions(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            state = host.default_state(Path(raw))
        for step in host.DECISION_STEPS:
            host.set_step(state, step, "confirmed", "x")
        for step in host.ACTION_STEPS:
            host.set_step(state, step, "verified", "x")
        self.assertEqual(host.release_incomplete_steps(state), [])
        state["steps"]["fake.release"]["status"] = "applied"
        self.assertEqual(host.release_incomplete_steps(state), ["fake.release"])
        state["steps"]["pack.ci"]["status"] = "confirmed"
        self.assertIn("pack.ci", host.release_incomplete_steps(state))
        state["steps"]["fake.release"]["status"] = "verified"
        self.assertEqual(host.release_incomplete_steps(state, via_ci=True), [])

    def test_agent_execute_is_ci_only(self) -> None:
        source = inspect.getsource(host.cmd_release)
        self.assertIn("RELKIT_RELEASE_VIA_CI", source)
        self.assertIn("agent publish is CI-only", source)

    def test_fake_verify_runs_simulate_and_marks_verified(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            host.save_state(root, state)
            staged = root / ".relkit" / "staged" / "1.2.3+4"
            staged.mkdir(parents=True)
            (staged / "staged.pb").write_bytes(b"x")
            with (
                patch("relkit_host.relkit_bin", return_value=Path("relkit")),
                patch("relkit_host.run_relkit") as run,
            ):
                self.assertEqual(host.cmd_fake_verify(root, "1.2.3+4"), 0)
            run.assert_called_once_with(
                root,
                Path("relkit"),
                ["simulate", "--with-staged", "1.2.3+4", "--from", "all"],
            )
            state = host.load_state(root)
            self.assertEqual(state["steps"]["fake.release"]["status"], "verified")

    def test_release_refuses_drift(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            for step in host.REQUIRED_FOR_RELEASE:
                host.set_step(state, step, "verified", "x")
            state["product"] = "loom"
            host.save_state(root, state)
            args = host.build_parser().parse_args(["release"])
            with patch(
                "relkit_host.reconcile",
                return_value={**state, "drift": ["serve missing loom"], "unconfirmed": []},
            ):
                with self.assertRaisesRegex(host.Fail, "unresolved drift"):
                    host.cmd_release(root, args)

    def test_release_refuses_unconfirmed_ssh(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            for step in host.REQUIRED_FOR_RELEASE:
                host.set_step(state, step, "verified", "x")
            host.save_state(root, state)
            args = host.build_parser().parse_args(["release"])
            with patch(
                "relkit_host.reconcile",
                return_value={**state, "drift": [], "unconfirmed": ["SSH failed"]},
            ):
                with self.assertRaisesRegex(host.Fail, "cannot confirm"):
                    host.cmd_release(root, args)

    def test_agent_config_publishes_through_agent_without_private_key(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "package.json").write_text("{}\n", encoding="utf-8")
            (root / "relkit.json").write_text(
                json.dumps({"product": "demo", "agent": {"url": "http://agent/v1/"}}),
                encoding="utf-8",
            )
            staged = root / ".relkit" / "staged" / "1.0.0+1"
            staged.mkdir(parents=True)
            self.assertEqual(host.agent_base_url(root), "http://agent/v1/")
            lines: list[str] = []
            with patch("builtins.print", lambda *args: lines.append(" ".join(map(str, args)))):
                self.assertEqual(
                    host.publish_via_agent(
                        root,
                        Path("relkit"),
                        product="demo",
                        version="1.0.0+1",
                        url="http://agent/v1/",
                        execute=False,
                    ),
                    0,
                )
            text = "\n".join(lines)
            self.assertIn("cas-put", text)
            self.assertIn("POST http://agent/v1/publish", text)
            self.assertNotIn("private", text)

    def test_agent_publish_refuses_without_staged_tree(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            with self.assertRaisesRegex(host.Fail, "no staged tree"):
                host.publish_via_agent(
                    root,
                    Path("relkit"),
                    product="demo",
                    version="1.0.0+1",
                    url="http://agent/v1/",
                    execute=True,
                )

    def test_serve_add_does_not_write_json(self) -> None:
        import inspect

        source = inspect.getsource(host.cmd_serve_add)
        self.assertNotIn("json.dumps", source)
        self.assertNotIn("uploadTokens", source)

    def test_agent_add_forwards_share_with(self) -> None:
        import inspect

        source = inspect.getsource(host.cmd_agent_add)
        self.assertIn("-share-with", source)
        self.assertIn("share-with does not print a token", source)

    def test_release_reads_version_via_get(self) -> None:
        import inspect

        source = inspect.getsource(host.cmd_release)
        self.assertIn('"version", "get"', source)
        self.assertNotIn('[str(binary), "version"]', source)


class SshPortTests(unittest.TestCase):
    def write_config(self, root: Path) -> Path:
        included = root / "devcloud_config"
        included.write_text(
            "Host *.devcloud.woa.com\n    User root\n    Port 36000\n",
            encoding="utf-8",
        )
        path = root / "config"
        path.write_text(
            f"Host *.devcloud.woa.com\nInclude {included.as_posix()}\n"
            "Host cvm-gz\n    HostName 10.0.0.1\n",
            encoding="utf-8",
        )
        return path

    def test_port_comes_from_the_included_file(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            path = self.write_config(Path(raw))
            self.assertEqual(
                host.ssh_host_port("update.devcloud.woa.com", path), 36000
            )
            self.assertIsNone(host.ssh_host_port("cvm-gz", path))

    def test_target_spells_out_the_port(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            path = self.write_config(Path(raw))
            lookup = host.ssh_host_port
            with patch(
                "relkit_host.ssh_host_port",
                side_effect=lambda name, config_path=path: lookup(name, config_path),
            ):
                self.assertEqual(
                    host.ssh_target("update.devcloud.woa.com"),
                    "update.devcloud.woa.com:36000",
                )
                self.assertEqual(host.ssh_target("cvm-gz"), "cvm-gz:22 (ssh default)")

    def test_ssh_run_passes_the_port_and_names_it_on_failure(self) -> None:
        completed = subprocess.CompletedProcess([], 0, "", "")
        with patch("subprocess.run", return_value=completed) as runner:
            host.ssh_run("update.devcloud.woa.com", ["true"], port=36000)
        self.assertEqual(runner.call_args.args[0][:6], [
            "ssh",
            "-o",
            "BatchMode=yes",
            "-p",
            "36000",
            "update.devcloud.woa.com",
        ])
        fake = subprocess.CompletedProcess(
            args=["ssh"], returncode=255, stdout="", stderr="Connection refused"
        )
        with patch("relkit_host.subprocess.run", return_value=fake):
            with self.assertRaisesRegex(host.Fail, r"update\.devcloud\.woa\.com:36000"):
                host.ssh_run("update.devcloud.woa.com", ["true"], port=36000)

    def test_recommendation_shows_the_port(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            path = self.write_config(Path(raw))
            self.assertIn("*.devcloud.woa.com (port 36000)", host.ssh_host_recommend(path))


class SshHostParseTests(unittest.TestCase):
    def test_skips_bare_star_from_exact(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "config"
            path.write_text("Host *\nHost update.devcloud.woa.com\n", encoding="utf-8")
            self.assertEqual(host.list_ssh_hosts(path), ["update.devcloud.woa.com"])

    def test_include_and_glob_match(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            included = root / "devcloud"
            included.write_text("Host *.devcloud.woa.com\n    Port 36000\n", encoding="utf-8")
            path = root / "config"
            path.write_text(
                f"Host *.devcloud.woa.com\nInclude {included.as_posix()}\nHost cvm-gz\n",
                encoding="utf-8",
            )
            exact, patterns = host.parse_ssh_config(path)
            self.assertEqual(exact, ["cvm-gz"])
            self.assertIn("*.devcloud.woa.com", patterns)
            self.assertTrue(host.ssh_host_allowed("update.devcloud.woa.com", path))
            self.assertFalse(host.ssh_host_allowed("git.woa.com", path))


if __name__ == "__main__":
    unittest.main()
