#!/usr/bin/env python3
from __future__ import annotations

import json
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
            (root / "a.py").write_text("x\n", encoding="utf-8")
            first = host.tree_sha256(root)
            second = host.tree_sha256(root)
            self.assertEqual(first, second)
            (root / "a.py").write_text("y\n", encoding="utf-8")
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


class ReconcileTests(unittest.TestCase):
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
