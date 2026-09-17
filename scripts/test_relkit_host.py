#!/usr/bin/env python3
from __future__ import annotations

import io
import json
import inspect
import re
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stdout
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
            self.assertNotEqual(first, host.tree_sha256(root))
            (root / "README.local").unlink()
            self.assertEqual(first, host.tree_sha256(root))
            (root / "relkit_consume.py").write_text("y\n", encoding="utf-8")
            self.assertNotEqual(first, host.tree_sha256(root))


class StateTests(unittest.TestCase):
    def write_answer_batch(
        self,
        root: Path,
        intent: str,
        answers: dict[str, object],
    ) -> Path:
        batch = host.build_question_batch(root, host.load_state(root), intent)
        path = root / ".relkit" / "cache" / "answers.json"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(
            json.dumps(
                {
                    "schema": host.ANSWER_BATCH_SCHEMA,
                    "intent": intent,
                    "revision": batch["revision"],
                    "answers": answers,
                }
            )
            + "\n",
            encoding="utf-8",
        )
        return path

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
            self.assertEqual(state["steps"]["env.inspect"]["status"], "verified")
            self.assertEqual(state["steps"]["product.id"]["status"], "unanswered")
            ignore = (root / ".gitignore").read_text(encoding="utf-8")
            self.assertIn(".relkit/cache/", ignore)
            self.assertTrue(host.projection_path(root).is_file())
            self.assertEqual(
                host.projection_path(root),
                root / ".relkit/cache/onboarding.md",
            )
            self.assertEqual(state["steps"]["ops.retrospect"]["status"], "unanswered")

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

    def test_questions_batch_all_human_decisions_after_inspection(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "package.json").write_text("{}\n", encoding="utf-8")
            host.cmd_onboard_start(root)
            batch = host.build_question_batch(root, host.load_state(root), "fresh")
            self.assertEqual(batch["schema"], host.QUESTION_BATCH_SCHEMA)
            self.assertEqual(
                [item["id"] for item in batch["questions"]],
                list(host.BATCH_DECISION_STEPS),
            )
            self.assertIn("inspection", batch)
            self.assertEqual(
                batch["answerShape"]["revision"], "<copy revision>"
            )

    def test_answer_batch_is_applied_atomically(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "package.json").write_text("{}\n", encoding="utf-8")
            host.cmd_onboard_start(root)
            path = self.write_answer_batch(
                root,
                "fresh",
                {
                    "product.id": "demo",
                    "updater.process": "node",
                    "channel.ssot": "VERSION.json",
                    "backend.kind": "intranet-relkit-compatible",
                    "ssh.host": "update.devcloud.woa.com",
                    "ssh.config_dir": "/etc/relkit-serve",
                    "token.isolation": "exclusive",
                },
            )
            with patch("relkit_host.ssh_host_allowed", return_value=True):
                self.assertEqual(
                    host.cmd_onboard_apply(root, path.relative_to(root)), 0
                )
            state = host.load_state(root)
            self.assertEqual(state["product"], "demo")
            for step in host.BATCH_DECISION_STEPS:
                self.assertEqual(state["steps"][step]["status"], "confirmed")

    def test_answer_conflicts_reject_the_whole_batch(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            host.cmd_onboard_start(root)
            before = host.load_state(root)
            path = self.write_answer_batch(
                root,
                "fresh",
                {
                    "product.id": "demo",
                    "updater.process": "not-a-process",
                    "channel.ssot": "VERSION.json",
                    "backend.kind": "not-a-backend",
                    "ssh.host": "missing-host",
                    "ssh.config_dir": "/etc/relkit-serve",
                    "token.isolation": "share-with:demo",
                },
            )
            with self.assertRaises(host.Fail) as raised:
                host.cmd_onboard_apply(root, path)
            self.assertEqual(raised.exception.code, "onboard-answer-conflict")
            self.assertIn("updater.process", str(raised.exception))
            self.assertIn("backend.kind", str(raised.exception))
            self.assertEqual(host.load_state(root), before)

    def test_changed_facts_reject_stale_answer_revision(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            host.cmd_onboard_start(root)
            batch = host.build_question_batch(root, host.load_state(root), "fresh")
            answers = root / "answers.json"
            answers.write_text(
                json.dumps(
                    {
                        "schema": host.ANSWER_BATCH_SCHEMA,
                        "intent": "fresh",
                        "revision": batch["revision"],
                        "answers": {},
                    }
                ),
                encoding="utf-8",
            )
            (root / "VERSION.json").write_text("{}\n", encoding="utf-8")
            with self.assertRaises(host.Fail) as raised:
                host.cmd_onboard_apply(root, answers)
            self.assertEqual(
                raised.exception.code, "onboard-answer-revision-conflict"
            )

    def test_fresh_intent_rejects_existing_product(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "relkit.json").write_text(
                '{"product":"existing","backends":{}}\n', encoding="utf-8"
            )
            host.cmd_onboard_start(root)
            with self.assertRaises(host.Fail) as raised:
                host.build_question_batch(root, host.load_state(root), "fresh")
            self.assertEqual(
                raised.exception.code, "onboard-batch-intent-conflict"
            )

    def test_direct_backends_make_remote_token_steps_not_applicable(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "relkit.json").write_text(
                json.dumps(
                    {
                        "product": "direct",
                        "backends": {"cos": {"type": "s3-compatible"}},
                        "publishTo": ["cos"],
                    }
                ),
                encoding="utf-8",
            )
            state = host.default_state(root)
            host.set_step(state, "env.inspect", "verified", "clean")
            state["product"] = "direct"
            host.set_step(state, "product.id", "confirmed", "direct")
            batch = host.build_question_batch(root, state, "upgrade")
            asked = {item["id"] for item in batch["questions"]}
            self.assertNotIn("ssh.host", asked)
            self.assertNotIn("ssh.config_dir", asked)
            self.assertNotIn("token.isolation", asked)
            self.assertEqual(batch["evidence"]["topology"]["mode"], "direct")
            self.assertFalse(
                batch["evidence"]["applicable"]["serve.register"]
            )
            self.assertIn(
                "serve/agent product tokens are not on this route",
                "\n".join(batch["evidence"]["implications"]),
            )

    def test_live_products_are_the_only_share_with_options(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "relkit.json").write_text(
                json.dumps(
                    {
                        "product": "demo",
                        "backends": {
                            "intranet": {"type": "relkit-compatible"}
                        },
                        "publishTo": ["intranet"],
                    }
                ),
                encoding="utf-8",
            )
            state = host.default_state(root)
            host.set_step(state, "env.inspect", "verified", "clean")
            state["serve"]["sshHost"] = "publisher"
            responses = [
                subprocess.CompletedProcess([], 0, "relkit-serve 0.3.17\n", ""),
                subprocess.CompletedProcess([], 0, "active\nenabled\n", ""),
                subprocess.CompletedProcess(
                    [],
                    0,
                    (
                        "config /etc/relkit-serve/relkit-serve.json\n"
                        "operator relkit-serve.token (full tree)\n"
                        "products\n"
                        "  loom,atlas tokens/loom.token\n"
                    ),
                    "",
                ),
            ]
            with patch("relkit_host.ssh_run", side_effect=responses):
                batch = host.build_question_batch(root, state, "upgrade")
            token = next(
                item
                for item in batch["questions"]
                if item["id"] == "token.isolation"
            )
            self.assertEqual(
                token["options"],
                ["exclusive", "share-with:loom", "share-with:atlas"],
            )
            remote = batch["evidence"]["remote"]
            self.assertTrue(remote["operatorTokenPresent"])
            self.assertEqual(remote["products"], ["loom", "atlas"])

    def test_operator_only_remote_does_not_offer_share_with(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "relkit.json").write_text(
                json.dumps(
                    {
                        "product": "demo",
                        "backends": {
                            "intranet": {"type": "relkit-compatible"}
                        },
                        "publishTo": ["intranet"],
                    }
                ),
                encoding="utf-8",
            )
            state = host.default_state(root)
            host.set_step(state, "env.inspect", "verified", "clean")
            state["serve"]["sshHost"] = "publisher"
            responses = [
                subprocess.CompletedProcess([], 0, "relkit-serve 0.3.17\n", ""),
                subprocess.CompletedProcess([], 0, "active\nenabled\n", ""),
                subprocess.CompletedProcess(
                    [],
                    0,
                    "operator relkit-serve.token (full tree)\nproducts none\n",
                    "",
                ),
            ]
            with patch("relkit_host.ssh_run", side_effect=responses):
                batch = host.build_question_batch(root, state, "upgrade")
            token = next(
                item
                for item in batch["questions"]
                if item["id"] == "token.isolation"
            )
            self.assertEqual(token["options"], ["exclusive"])
            self.assertIn(
                "operator credentials are not product tokens",
                "\n".join(batch["evidence"]["implications"]),
            )

    def test_token_question_waits_for_live_remote_inventory(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "relkit.json").write_text(
                json.dumps(
                    {
                        "product": "demo",
                        "backends": {
                            "intranet": {"type": "relkit-compatible"}
                        },
                        "publishTo": ["intranet"],
                    }
                ),
                encoding="utf-8",
            )
            state = host.default_state(root)
            host.set_step(state, "env.inspect", "verified", "clean")
            batch = host.build_question_batch(root, state, "upgrade")
            asked = {item["id"] for item in batch["questions"]}
            self.assertIn("ssh.host", asked)
            self.assertNotIn("token.isolation", asked)
            self.assertEqual(
                [item["id"] for item in batch["blocked"]],
                ["token.isolation"],
            )

    def test_direct_release_does_not_require_remote_steps(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "relkit.json").write_text(
                json.dumps(
                    {
                        "backends": {"cos": {"type": "s3-compatible"}},
                        "publishTo": ["cos"],
                    }
                ),
                encoding="utf-8",
            )
            state = host.default_state(root)
            for step in host.REQUIRED_FOR_RELEASE:
                if step not in {
                    "ssh.host",
                    "ssh.config_dir",
                    "token.isolation",
                    "serve.register",
                    "agent.register",
                }:
                    host.set_step(state, step, "verified", "ok")
            self.assertEqual(
                host.release_incomplete_steps(state, root=root), []
            )

    def test_status_skips_remote_steps_for_direct_publish(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "relkit.json").write_text(
                json.dumps(
                    {
                        "product": "direct",
                        "signing": {
                            "keyId": "k1",
                            "publicKeys": [{"keyId": "k1", "publicKeyBase64": "QQ=="}],
                        },
                        "backends": {"cos": {"type": "s3-compatible"}},
                        "publishTo": ["cos"],
                    }
                ),
                encoding="utf-8",
            )
            state = host.default_state(root)
            host.set_step(state, "ssh.host", "stale", "cvm-gz")
            host.set_step(state, "token.isolation", "stale", "exclusive")
            host.save_state(root, state)
            host.cmd_status(root)
            saved = host.load_state(root)
            self.assertEqual(saved["steps"]["ssh.host"]["status"], "skipped")
            self.assertEqual(saved["steps"]["token.isolation"]["status"], "skipped")
            self.assertEqual(saved["steps"]["serve.register"]["status"], "skipped")
            self.assertEqual(saved["steps"]["signing.keys"]["value"], "k1")
            self.assertEqual(saved["steps"]["signing.keys"]["status"], "verified")
            self.assertNotEqual(host.next_unresolved_step(saved), "ssh.host")

    def test_recommendation_does_not_choose_backend(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            self.assertIsNone(host.recommended_value(root, state, "backend.kind"))
            self.assertEqual(
                host.decision_options(root, "backend.kind"),
                ["intranet-relkit-compatible", "s3-compatible"],
            )

    def test_static_http_is_not_an_onboard_backend(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            host.cmd_onboard_start(root)
            with self.assertRaises(host.Fail) as raised:
                host.cmd_onboard_set(root, "backend.kind", "static-http", None)
            self.assertIn("s3-compatible", str(raised.exception))
            self.assertNotIn("static-http", host.decision_options(root, "backend.kind"))

    def test_nested_src_tauri_detects_rust_and_node(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "package.json").write_text("{}\n", encoding="utf-8")
            tauri = root / "packages" / "desktop" / "src-tauri"
            tauri.mkdir(parents=True)
            (tauri / "Cargo.toml").write_text("[package]\nname='shell'\n", encoding="utf-8")
            stack = host.detect_stack(root)
            self.assertEqual(stack["languages"], ["rust", "node"])
            self.assertIn("rust", stack["updater"])
            self.assertIn("node", stack["updater"])
            self.assertNotIn("rust-shell", stack["updater"])
            self.assertIsNone(
                host.recommended_value(root, host.default_state(root), "updater.process")
            )
            self.assertEqual(
                host.consume_components(root),
            ["sdk-rust", "bindings-ts", "cli", "updater"],
            )

    def test_nested_client_src_tauri_consumes_rust_and_ts(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "go.mod").write_text("module example.test\n", encoding="utf-8")
            client = root / "client" / "src-tauri"
            client.mkdir(parents=True)
            (client / "Cargo.toml").write_text("[package]\nname='console'\n", encoding="utf-8")
            (root / "client" / "package.json").write_text("{}\n", encoding="utf-8")
            stack = host.detect_stack(root)
            self.assertEqual(stack["languages"], ["go", "rust", "node"])
            self.assertEqual(
                host.consume_components(root),
                ["sdk-go", "sdk-rust", "bindings-ts", "cli", "updater"],
            )

    def test_root_src_tauri_selects_rust_sdk(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            tauri = root / "src-tauri"
            tauri.mkdir()
            (tauri / "Cargo.toml").write_text("[package]\nname='loom'\n", encoding="utf-8")
            self.assertEqual(host.detect_stack(root)["languages"], ["rust"])
            self.assertEqual(host.detect_stack(root)["updater"], "rust")
            self.assertEqual(host.consume_components(root)[0], "sdk-rust")

    def test_updater_process_explain_forbids_handwritten_bridge(self) -> None:
        text = host.UPDATER_PROCESS_EXPLAIN
        self.assertNotIn("开工窄桥", text)
        self.assertIn("禁止再开工一份 CheckResult", text)
        self.assertIn("check_result_to_json", text)
        self.assertIn("serde(default)", text)

    def test_updater_process_is_closed_vocab(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            with self.assertRaisesRegex(host.Fail, "must be"):
                host.cmd_onboard_set(root, "updater.process", "electron", None)
            with self.assertRaisesRegex(host.Fail, "must be"):
                host.cmd_onboard_set(root, "updater.process", "rust-shell", None)
            self.assertEqual(host.cmd_onboard_set(root, "updater.process", "rust", None), 0)
            state = host.load_state(root)
            self.assertEqual(state["steps"]["updater.process"]["value"], "rust")
            md = host.projection_path(root).read_text(encoding="utf-8")
            self.assertIn("rust", md)
            self.assertIn("Do not edit", md)

    def test_onboard_reset_requires_yes_and_deletes_relkit_dir(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            host.cmd_onboard_start(root)
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

    def test_default_root_comes_from_facade_not_hostlib_module(self) -> None:
        entry = Path(host.__file__).resolve()
        self.assertEqual(host.host_scripts_dir(), entry.parent)
        self.assertEqual(host.host_root(), entry.parent.parent.parent)

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

    def test_routing_names_only_canonical_product_and_deploy_entries(self) -> None:
        text = host.routing_help()
        self.assertIn("relkit_host.py", text)
        self.assertIn("onboard inspect|questions|apply", text)
        self.assertIn("python scripts/deploy/relkit.py build|install|upgrade", text)
        self.assertNotIn("scripts/relkit_host.py", text)
        self.assertNotIn("scripts/deploy/relkit.py serve", text)

    def test_batch_question_and_apply_commands_parse(self) -> None:
        parser = host.build_parser()
        questions = parser.parse_args(
            ["onboard", "questions", "--intent", "upgrade", "--json"]
        )
        self.assertEqual(questions.onboard_cmd, "questions")
        self.assertEqual(questions.intent, "upgrade")
        self.assertTrue(questions.json)
        apply = parser.parse_args(
            ["onboard", "apply", "--answers", ".relkit/cache/answers.json"]
        )
        self.assertEqual(apply.onboard_cmd, "apply")
        self.assertEqual(apply.answers, ".relkit/cache/answers.json")

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

    def test_list_products_parser_share_with_products_block(self) -> None:
        text = (
            "config /etc/relkit-serve/relkit-serve.json\n"
            "operator  relkit-serve.token (full tree)\n"
            "products\n"
            "  loom,svn-auto-merge      tokens/loom.token\n"
        )
        self.assertEqual(host.parse_list_products(text), ["loom", "svn-auto-merge"])


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


    def test_upgrade_rewrites_a_consume_v1_lock(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            lock_path = root / "scripts" / "relkit.lock.json"
            lock_path.parent.mkdir(parents=True)
            lock_path.write_text(
                json.dumps(
                    {
                        "schema": "relkit.consume/1",
                        "url": "https://github.com/shichao402/relkit.git",
                        "channel": "main",
                        "commit": "6" * 40,
                        "protocol": {"min": 2, "max": 2},
                        "updaterIpc": {"min": 1, "max": 1},
                    }
                ),
                encoding="utf-8",
            )
            commit = "b814972e839dbb5eaac9814728e0fe08258c5e2b"

            with (
                patch("relkit_host.http_get", side_effect=self.fake_release(commit)),
                patch("relkit_host.cmd_install", return_value=0),
            ):
                self.assertEqual(host.cmd_upgrade(root, "v0.3.14"), 0)

            lock = json.loads(lock_path.read_text(encoding="utf-8"))
            self.assertEqual(lock["schema"], host.LOCK_SCHEMA)
            self.assertEqual(lock["commit"], commit)
            self.assertEqual(lock["protocol"], {"min": 2, "max": 2})
            self.assertEqual(lock["updaterIpc"], {"min": 1, "max": 1})
            self.assertEqual(lock["artifacts"]["sdk-go"]["sha256"], "d" * 64)
            # Sparse-checkout keys must not survive into an immutable-release lock.
            self.assertNotIn("url", lock)
            self.assertNotIn("channel", lock)

    def test_upgrade_creates_a_lock_when_the_repo_has_none(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            commit = "b814972e839dbb5eaac9814728e0fe08258c5e2b"
            with (
                patch("relkit_host.http_get", side_effect=self.fake_release(commit)),
                patch("relkit_host.cmd_install", return_value=0),
            ):
                self.assertEqual(host.cmd_upgrade(root, "v0.3.14"), 0)
            lock = json.loads(
                (root / "scripts" / "relkit.lock.json").read_text(encoding="utf-8")
            )
            self.assertEqual(lock["schema"], host.LOCK_SCHEMA)
            self.assertEqual(
                lock["updaterIpc"],
                {"min": host.UPDATER_IPC_FALLBACK, "max": host.UPDATER_IPC_FALLBACK},
            )

    def test_upgrade_takes_the_protocol_window_from_the_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            commit = "b814972e839dbb5eaac9814728e0fe08258c5e2b"
            fake = self.fake_release(commit, min_protocol=2, max_protocol=3)
            with (
                patch("relkit_host.http_get", side_effect=fake),
                patch("relkit_host.cmd_install", return_value=0),
            ):
                self.assertEqual(host.cmd_upgrade(root, "v0.3.14"), 0)
            lock = json.loads(
                (root / "scripts" / "relkit.lock.json").read_text(encoding="utf-8")
            )
            self.assertEqual(lock["protocol"], {"min": 2, "max": 3})

    def test_upgrade_takes_updater_ipc_from_the_manifest_not_the_old_lock(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            lock_path = root / "scripts" / "relkit.lock.json"
            lock_path.parent.mkdir(parents=True)
            lock_path.write_text(
                json.dumps(
                    {
                        "schema": "relkit.consume/2",
                        "updaterIpc": {"min": 1, "max": 1},
                    }
                ),
                encoding="utf-8",
            )
            commit = "b814972e839dbb5eaac9814728e0fe08258c5e2b"
            fake = self.fake_release(
                commit,
                min_updater_ipc=3,
                max_updater_ipc=3,
            )
            with (
                patch("relkit_host.http_get", side_effect=fake),
                patch("relkit_host.cmd_install", return_value=0),
            ):
                self.assertEqual(host.cmd_upgrade(root, "v0.4.4"), 0)
            lock = json.loads(lock_path.read_text(encoding="utf-8"))
            self.assertEqual(lock["updaterIpc"], {"min": 3, "max": 3})

    @staticmethod
    def fake_release(
        commit: str,
        min_protocol: int = 2,
        max_protocol: int = 2,
        min_updater_ipc: int | None = None,
        max_updater_ipc: int | None = None,
    ):
        def fake_get(url: str) -> str:
            if url.endswith("/manifest.json"):
                doc: dict = {
                    "commit": commit,
                    "minProtocol": min_protocol,
                    "maxProtocol": max_protocol,
                    "hostScriptsSha256": "a" * 64,
                    "consumerSha256": "b" * 64,
                }
                if min_updater_ipc is not None:
                    doc["minUpdaterIpc"] = min_updater_ipc
                if max_updater_ipc is not None:
                    doc["maxUpdaterIpc"] = max_updater_ipc
                return json.dumps(doc)
            if url.endswith("/SHA256SUMS"):
                return (
                    f"{'c' * 64}  relkit-host-scripts.zip\n"
                    f"{'d' * 64}  relkit-sdk-go.zip\n"
                )
            raise host.Fail(f"unexpected GET {url}")

        return fake_get


class GoSdkWiringTests(unittest.TestCase):
    def test_go_repo_installs_the_go_sdk_component(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "go.mod").write_text("module example.test\n", encoding="utf-8")
            self.assertIn("sdk-go", host.consume_components(root))

    def test_non_go_repo_does_not_install_the_go_sdk(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            self.assertNotIn("sdk-go", host.consume_components(Path(raw)))

    def test_lock_pins_the_go_sdk_archive(self) -> None:
        artifacts: dict = {}
        host.rewrite_lock_artifacts(
            artifacts,
            "https://example.invalid/v0.3.14",
            {"relkit-sdk-go.zip": "e" * 64},
        )
        self.assertEqual(
            artifacts["sdk-go"],
            {
                "url": "https://example.invalid/v0.3.14/relkit-sdk-go.zip",
                "sha256": "e" * 64,
            },
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


    def test_install_skips_sdk_components_absent_from_the_lock(self) -> None:
        forwarded = self.install_with_lock_artifacts({"cli": {}, "updater": {}})
        self.assertNotIn("sdk-go", forwarded)
        self.assertIn("cli", forwarded)

    def test_install_requests_the_go_sdk_once_the_lock_pins_it(self) -> None:
        forwarded = self.install_with_lock_artifacts(
            {"cli": {}, "updater": {}, "sdk-go": {}}
        )
        self.assertIn("sdk-go", forwarded)

    @staticmethod
    def install_with_lock_artifacts(artifacts: dict) -> list:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "go.mod").write_text("module example.test\n", encoding="utf-8")
            host_dir = root / "scripts" / "host"
            host_dir.mkdir(parents=True)
            (host_dir / "relkit_consume.py").write_text("# consume\n", encoding="utf-8")
            (host_dir / "relkit_host.py").write_text("# host\n", encoding="utf-8")
            (root / "scripts" / "relkit.lock.json").write_text(
                json.dumps(
                    {
                        "schema": host.LOCK_SCHEMA,
                        "release": "v0.3.13",
                        "commit": "a" * 40,
                        "hostScriptsSha256": host.tree_sha256(host_dir),
                        "artifacts": artifacts,
                    }
                ),
                encoding="utf-8",
            )
            seen: list[list[str]] = []
            consume = type(
                "Consume",
                (),
                {"main": staticmethod(lambda argv: seen.append(list(argv)) or 0)},
            )
            with patch("relkit_host.import_consume", return_value=consume):
                assert host.cmd_install(root, []) == 0
            return seen[0]


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

    def test_profile_never_carries_site_owner_config(self) -> None:
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
        self.assertNotIn("site", profile)

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
    def test_lock_pinned_updater_verifies_without_pack_script(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            state["steps"]["sidecar.layout"]["status"] = "stale"
            bin_dir = root / "tools" / "bin"
            bin_dir.mkdir(parents=True)
            (bin_dir / host.updater_sidecar_name()).write_bytes(b"sidecar")
            drift: list[str] = []
            host.reconcile_sidecar_layout(root, state, drift)
            self.assertEqual(state["steps"]["sidecar.layout"]["status"], "verified")
            self.assertEqual(state["steps"]["sidecar.layout"]["value"], "tools/bin")
            self.assertEqual(drift, [])

    def test_loom_mjs_pack_script_is_optional_and_generic(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            (root / "scripts").mkdir()
            (root / "scripts" / "build_desktop_release.mjs").write_text(
                "tools/bin relkit-updater\n",
                encoding="utf-8",
            )
            (root / "relkit.json").write_text(
                json.dumps(
                    {"sidecar": {"packScript": "scripts/build_desktop_release.mjs"}}
                ),
                encoding="utf-8",
            )
            bin_dir = root / "tools" / "bin"
            bin_dir.mkdir(parents=True)
            (bin_dir / host.updater_sidecar_name()).write_bytes(b"sidecar")
            drift: list[str] = []
            host.reconcile_sidecar_layout(root, state, drift)
            self.assertEqual(state["steps"]["sidecar.layout"]["status"], "verified")
            self.assertEqual(drift, [])

    def test_missing_updater_stays_unverified(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            state["steps"]["sidecar.layout"]["status"] = "stale"
            drift: list[str] = []
            host.reconcile_sidecar_layout(root, state, drift)
            self.assertEqual(state["steps"]["sidecar.layout"]["status"], "stale")
            self.assertEqual(drift, [])

    def test_missing_configured_pack_script_fails(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            (root / "relkit.json").write_text(
                json.dumps({"sidecar": {"packScript": "scripts/missing.py"}}),
                encoding="utf-8",
            )
            bin_dir = root / "tools" / "bin"
            bin_dir.mkdir(parents=True)
            (bin_dir / host.updater_sidecar_name()).write_bytes(b"sidecar")
            drift: list[str] = []
            host.reconcile_sidecar_layout(root, state, drift)
            self.assertEqual(state["steps"]["sidecar.layout"]["status"], "drift")
            self.assertIn("sidecar.packScript is missing", drift[0])


class FakeStageTests(unittest.TestCase):
    def test_staged_tree_restores_applied_not_verified(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            state["steps"]["fake.release"]["status"] = "stale"
            state["steps"]["fake.release"]["value"] = "0.2.2+0"
            tree = root / ".relkit" / "cache" / "staged" / "0.2.2+0"
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
            tree = root / ".relkit" / "cache" / "staged" / "0.2.2+0"
            tree.mkdir(parents=True)
            (tree / "staged.pb").write_bytes(b"x")
            host.reconcile_fake_stage(root, state)
            self.assertEqual(state["steps"]["fake.release"]["status"], "verified")
            self.assertEqual(state["steps"]["fake.release"]["note"], "simulate passed")

    def test_new_real_stage_does_not_downgrade_verified_dummy(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            state["steps"]["fake.release"] = {
                "status": "verified",
                "value": "0.2.0+112",
                "note": "dummy simulate passed",
            }
            for version in ("0.2.0+112", "0.2.0+135"):
                tree = root / ".relkit" / "cache" / "staged" / version
                tree.mkdir(parents=True)
                (tree / "staged.pb").write_bytes(b"x")
            host.reconcile_fake_stage(root, state)
            self.assertEqual(state["steps"]["fake.release"]["status"], "verified")
            self.assertEqual(state["steps"]["fake.release"]["value"], "0.2.0+112")

    def test_real_publish_cleanup_keeps_only_current_stage(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            for version in ("0.2.0+112", "0.2.0+135"):
                tree = root / ".relkit" / "cache" / "staged" / version
                tree.mkdir(parents=True)
                (tree / "staged.pb").write_bytes(b"x")
            removed = host.clear_stale_staged_trees(root, "0.2.0+135")
            self.assertEqual(removed, ["0.2.0+112"])
            self.assertFalse(
                (root / ".relkit" / "cache" / "staged" / "0.2.0+112").exists()
            )
            self.assertTrue(
                (root / ".relkit" / "cache" / "staged" / "0.2.0+135").is_dir()
            )


class RetrospectTests(unittest.TestCase):
    def test_all_pass_output_has_labeled_groups_and_marks_verified(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            output = io.StringIO()
            with redirect_stdout(output):
                self.assertEqual(host.cmd_retrospect(root), 0)
            text = output.getvalue()
            self.assertIn("已落地\n", text)
            self.assertIn("待办\n- 无", text)
            self.assertIn("跳过\n", text)
            self.assertIn("本次遇到\n- 无", text)
            self.assertIn("未消化\n- 无", text)
            self.assertIn("文件:", text)
            self.assertIn("期望:", text)
            self.assertIn("现状:", text)
            state = host.load_state(root)
            self.assertEqual(host.STEP_IDS[-1], "ops.retrospect")
            self.assertEqual(state["steps"]["ops.retrospect"]["status"], "verified")

    def test_seeded_skill_drift_is_actionable_todo_and_exit_one(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            skill = root / "skills" / "relkit-ops" / "SKILL.md"
            skill.parent.mkdir(parents=True)
            skill.write_text("仅阅读说明。\n", encoding="utf-8")
            output = io.StringIO()
            with redirect_stdout(output):
                code = host.cmd_retrospect(root)
            self.assertEqual(code, 1)
            text = output.getvalue()
            self.assertIn("待办\n", text)
            self.assertIn("文件: skills/relkit-ops/SKILL.md", text)
            self.assertIn("期望: skill requires relkit_host.py retrospect", text)
            self.assertIn("现状: required command relkit_host.py retrospect is missing", text)
            self.assertFalse(host.state_path(root).exists())

    def test_missing_optional_skill_is_classified_as_skipped(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            report = host.retrospect_report(Path(raw))
        skipped = report["groups"]["skipped"]
        self.assertEqual(len(skipped), 1)
        self.assertEqual(skipped[0]["check"], "skill-gate")
        self.assertIn("not applicable", skipped[0]["actual"])

    def test_retrospect_json_follows_host_json_convention(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            output = io.StringIO()
            with redirect_stdout(output):
                self.assertEqual(host.cmd_retrospect(Path(raw), as_json=True), 0)
            report = json.loads(output.getvalue())
        self.assertEqual(report["schema"], "relkit.retrospect/2")
        self.assertIn("landed", report["groups"])
        self.assertIn("todo", report["groups"])
        self.assertIn("skipped", report["groups"])
        self.assertIn("encountered", report["groups"])
        self.assertIn("digested", report["groups"])
        self.assertIn("undigested", report["groups"])

    def test_retrospect_rejects_skill_that_only_points_to_markdown(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            skill = root / "skills" / "relkit-ops" / "SKILL.md"
            skill.parent.mkdir(parents=True)
            skill.write_text(
                "读 RETROSPECT.md 后即可宣称完成。\n",
                encoding="utf-8",
            )
            failures = host.retrospect_failures(root)
            self.assertTrue(any("relkit_host.py retrospect" in item for item in failures))
            self.assertTrue(any("reading RETROSPECT.md" in item for item in failures))

    def test_retrospect_rejects_command_only_skill_without_conversation_review(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            skill = root / "skills" / "relkit-ops" / "SKILL.md"
            skill.parent.mkdir(parents=True)
            skill.write_text(
                "先展示 evidence.topology 与 evidence.remote。\n"
                "`blocked` 中的决策本轮禁止询问；operatorTokenPresent=true 不代表产品 token。\n"
                "最后运行 python scripts/host/relkit_host.py retrospect。\n",
                encoding="utf-8",
            )
            report = host.retrospect_report(root)
        todo = {item["check"]: item for item in report["groups"]["todo"]}
        check = "skill-conversation-retrospect:skills/relkit-ops/SKILL.md"
        self.assertIn(check, todo)
        self.assertIn("command-only retrospect", todo[check]["actual"])

    def test_retrospect_requires_cache_ignore_but_tracks_onboarding(self) -> None:
        self.assertIn(".relkit/cache/", host.GITIGNORE_RELKIT)
        self.assertNotIn(".relkit/onboarding.json", host.GITIGNORE_RELKIT)

    def test_installed_host_scripts_do_not_leave_tracked_bytecode(self) -> None:
        self.assertIn("__pycache__/", host.GITIGNORE_RELKIT)

    def test_remote_sudo_uses_absolute_binaries(self) -> None:
        # sudoers secure_path omits /usr/local/bin, so a bare name is not found.
        source = inspect.getsource(host)
        self.assertIsNone(
            re.search(r'"sudo",\s*"relkit-(?:serve|agent)"', source)
        )
        self.assertEqual(host.SERVE_BIN, "/usr/local/bin/relkit-serve")
        self.assertEqual(host.AGENT_BIN, "/usr/local/bin/relkit-agent")

    def test_retrospect_contract_has_matching_step_keys(self) -> None:
        self.assertEqual(set(host.STEP_IDS), set(host.EXPLAIN_TEXTS))
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            self.assertEqual(
                set(host.STEP_IDS),
                set(host.recommendations(root, host.default_state(root))),
            )
        self.assertIn("rust", host.UPDATER_PROCESS_VALUES)
        self.assertNotIn("rust-shell", host.UPDATER_PROCESS_VALUES)
        self.assertNotIn(
            "build_desktop_release.mjs",
            inspect.getsource(host.reconcile_sidecar_layout),
        )


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

    def test_github_actions_publish_workflow_confirms_pack_ci(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            workflows = root / ".github" / "workflows"
            workflows.mkdir(parents=True)
            (workflows / "release.yml").write_text(
                "run: python3 scripts/host/relkit_host.py install\n"
                "run: ./tools/bin/relkit-linux-amd64 stage \"$VER\"\n"
                "run: ./tools/bin/relkit-linux-amd64 cas-put --product demo\n",
                encoding="utf-8",
            )
            state = host.default_state(root)
            host.reconcile_pack_ci(root, state)
            self.assertEqual(state["steps"]["pack.ci"]["status"], "confirmed")
            self.assertEqual(
                state["steps"]["pack.ci"]["value"],
                ".github/workflows/release.yml",
            )

    def test_install_only_github_workflow_does_not_confirm_pack_ci(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            workflows = root / ".github" / "workflows"
            workflows.mkdir(parents=True)
            (workflows / "ci.yml").write_text(
                "run: python3 scripts/host/relkit_host.py install\n",
                encoding="utf-8",
            )
            state = host.default_state(root)
            host.reconcile_pack_ci(root, state)
            self.assertEqual(state["steps"]["pack.ci"]["status"], "unanswered")

    def test_ci_skips_retrospect_but_local_release_requires_it(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            state = host.default_state(Path(raw))
        for step in host.REQUIRED_FOR_RELEASE:
            host.set_step(state, step, "verified", "x")
        state["steps"]["ops.retrospect"]["status"] = "unanswered"
        self.assertEqual(host.release_incomplete_steps(state), ["ops.retrospect"])
        self.assertEqual(host.release_incomplete_steps(state, via_ci=True), [])

    def test_agent_execute_is_ci_only(self) -> None:
        source = inspect.getsource(host.cmd_release)
        self.assertIn("RELKIT_RELEASE_VIA_CI", source)
        self.assertIn("agent publish is CI-only", source)

    def test_fake_verify_strips_v_prefix_from_version_json(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            host.save_state(root, host.default_state(root))
            (root / "VERSION.json").write_text(
                '{"version": "v1.13.64"}', encoding="utf-8"
            )
            with (
                patch("relkit_host.relkit_bin", return_value=Path("relkit")),
                patch("relkit_host.stage_dummy_release") as stage,
                patch("relkit_host.run_relkit") as run,
            ):
                self.assertEqual(host.cmd_fake_verify(root, None), 0)
            stage.assert_called_once()
            self.assertEqual(stage.call_args.args[1], "1.13.64")
            run.assert_called_once_with(
                root,
                Path("relkit"),
                ["simulate", "--with-staged", "1.13.64", "--from", "all"],
            )

    def test_fake_verify_runs_simulate_and_marks_verified(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            host.save_state(root, state)
            staged = root / ".relkit" / "cache" / "staged" / "1.2.3+4"
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

    def test_fake_verify_stages_dummy_when_tree_missing(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            host.save_state(root, host.default_state(root))
            with (
                patch("relkit_host.relkit_bin", return_value=Path("relkit")),
                patch("relkit_host.run_relkit") as run,
            ):
                self.assertEqual(host.cmd_fake_verify(root, "1.2.3+4"), 0)
            argv_lists = [call.args[2] for call in run.call_args_list]
            stage_argv = argv_lists[0]
            self.assertEqual(stage_argv[0], "stage")
            self.assertEqual(stage_argv[1], "1.2.3+4")
            self.assertIn("--install", stage_argv)
            self.assertNotIn("--add", stage_argv)
            self.assertTrue(
                all("meta.layout" not in part for part in stage_argv)
            )
            self.assertEqual(
                argv_lists[-1],
                ["simulate", "--with-staged", "1.2.3+4", "--from", "all"],
            )
            self.assertTrue(
                (root / ".relkit" / "cache" / "dummy-fake" / "dummy-windows-x64.zip").is_file()
            )

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
            staged = root / ".relkit" / "cache" / "staged" / "1.0.0+1"
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

    def test_cleanup_does_not_mask_missing_current_stage(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            dummy = root / ".relkit" / "cache" / "staged" / "0.2.0+112"
            dummy.mkdir(parents=True)
            (dummy / "staged.pb").write_bytes(b"x")
            host.clear_stale_staged_trees(root, "0.2.0+135")
            with self.assertRaisesRegex(host.Fail, "no staged tree for 0.2.0\\+135"):
                host.publish_via_agent(
                    root,
                    Path("relkit"),
                    product="demo",
                    version="0.2.0+135",
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

    def test_share_with_token_path_is_the_owner_file(self) -> None:
        self.assertEqual(
            host.serve_token_path("/etc/relkit-serve", "svn-auto-merge", "loom"),
            "/etc/relkit-serve/tokens/loom.token",
        )
        self.assertEqual(
            host.serve_token_path("/etc/relkit-serve", "svn-auto-merge", None),
            "/etc/relkit-serve/tokens/svn-auto-merge.token",
        )
        self.assertEqual(
            host.agent_token_path("svn-auto-merge", "loom"),
            "/etc/relkit-agent/tokens/loom.token",
        )

    def test_serve_add_share_with_chowns_owner_token_not_new_product(self) -> None:
        import argparse

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            host.save_state(root, state)
            remotes: list[list[str]] = []

            def fake_ssh(_remote_host: str, remote, **_kwargs):
                argv = list(remote)
                remotes.append(argv)
                stdout = ""
                if "init" in argv:
                    stdout = (
                        "config /etc/relkit-serve/relkit-serve.json\n"
                        "token  tokens/loom.token (shared; products now include svn-auto-merge)\n"
                    )
                return subprocess.CompletedProcess(argv, 0, stdout, "")

            args = argparse.Namespace(
                execute=True,
                restart=True,
                product="svn-auto-merge",
                share_with="loom",
                host="update.devcloud.woa.com",
                config_dir="/etc/relkit-serve",
            )
            with patch("relkit_host.ssh_run", side_effect=fake_ssh), patch(
                "relkit_host.ssh_host_port", return_value=36000
            ), patch("builtins.print"):
                self.assertEqual(host.cmd_serve_add(root, args), 0)

            chowns = [
                cmd
                for cmd in remotes
                if cmd[:3] == ["sudo", "chown", "relkit:relkit"]
            ]
            self.assertEqual(
                chowns,
                [
                    [
                        "sudo",
                        "chown",
                        "relkit:relkit",
                        "/etc/relkit-serve/tokens/loom.token",
                    ]
                ],
            )
            self.assertFalse(
                any("svn-auto-merge.token" in part for cmd in remotes for part in cmd)
            )
            self.assertIn(["sudo", "systemctl", "restart", "relkit-serve"], remotes)
            self.assertFalse((root / host.SECRET_NOTE).is_file())
            saved = host.load_state(root)
            self.assertEqual(saved["serve"]["shareWith"], "loom")
            self.assertEqual(saved["steps"]["serve.register"]["status"], "applied")

    def test_serve_restart_share_with_chowns_owner_token(self) -> None:
        import argparse

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            state = host.default_state(root)
            state["product"] = "svn-auto-merge"
            state["serve"]["sshHost"] = "box"
            state["serve"]["configDir"] = "/etc/relkit-serve"
            state["serve"]["shareWith"] = "loom"
            host.save_state(root, state)
            remotes: list[list[str]] = []

            def fake_ssh(_remote_host: str, remote, **_kwargs):
                argv = list(remote)
                remotes.append(argv)
                return subprocess.CompletedProcess(argv, 0, "", "")

            args = argparse.Namespace(execute=True, restart=True, host=None)
            with patch("relkit_host.ssh_run", side_effect=fake_ssh), patch(
                "builtins.print"
            ):
                self.assertEqual(host.cmd_serve_restart(root, args), 0)
            self.assertIn(
                ["sudo", "chown", "relkit:relkit", "/etc/relkit-serve/tokens/loom.token"],
                remotes,
            )
            self.assertFalse(
                any("svn-auto-merge.token" in part for cmd in remotes for part in cmd)
            )

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
            inventory = host.ssh_inventory(root, path)
            self.assertEqual(inventory["exact"], ["cvm-gz"])
            self.assertIn("*.devcloud.woa.com", inventory["patterns"])

    def test_include_glob_expands_files(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            parent = Path(raw)
            conf_d = parent / "config.d"
            conf_d.mkdir()
            (conf_d / "devcloud").write_text("Host box.devcloud.woa.com\n", encoding="utf-8")
            paths = host._ssh_include_paths(str(conf_d / "*"))
            self.assertEqual([item.name for item in paths], ["devcloud"])


class InspectAndJournalTests(unittest.TestCase):
    def test_http_put_blocks_inspect(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "relkit.json").write_text(
                json.dumps(
                    {
                        "product": "demo",
                        "backends": {"dev": {"type": "http-put"}},
                    }
                )
                + "\n",
                encoding="utf-8",
            )
            with self.assertRaises(host.Fail) as raised:
                host.cmd_onboard_inspect(root)
            self.assertEqual(raised.exception.code, "stale-backend-type")
            state = host.load_state(root)
            self.assertEqual(state["steps"]["env.inspect"]["status"], "blocked")

    def test_static_http_blocks_inspect(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "relkit.json").write_text(
                json.dumps(
                    {
                        "product": "demo",
                        "backends": {"mirror": {"type": "static-http"}},
                    }
                )
                + "\n",
                encoding="utf-8",
            )
            with self.assertRaises(host.Fail) as raised:
                host.cmd_onboard_inspect(root)
            self.assertEqual(raised.exception.code, "stale-backend-type")

    def test_product_id_requires_verified_inspect(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            with self.assertRaises(host.Fail) as raised:
                host.cmd_onboard_set(root, "product.id", "demo", None)
            self.assertEqual(raised.exception.code, "env-inspect-required")

    def test_inspect_cannot_be_set(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            with self.assertRaises(host.Fail) as raised:
                host.cmd_onboard_set(root, "env.inspect", "ok", None)
            self.assertEqual(raised.exception.code, "env-inspect-set-refused")

    def test_undigested_journal_fails_retrospect(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            host.append_ops_journal(
                root, ["release"], host.Fail("boom", code="brand-new-ops-bug")
            )
            output = io.StringIO()
            with redirect_stdout(output):
                code = host.cmd_retrospect(root)
            self.assertEqual(code, 1)
            self.assertIn("未消化", output.getvalue())
            self.assertIn("ops-journal:brand-new-ops-bug", output.getvalue())
            self.assertFalse(host.state_path(root).exists())

    def test_digested_journal_does_not_fail_retrospect(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            host.append_ops_journal(
                root, ["onboard", "inspect"], host.Fail("stale", code="stale-backend-type")
            )
            self.assertEqual(host.cmd_retrospect(root), 0)

    def test_unclassified_fail_is_not_journaled(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            host.append_ops_journal(root, ["status"], host.Fail("typo"))
            self.assertEqual(host.load_ops_journal(root), [])


class UpdaterGateTests(unittest.TestCase):
    def state(self, root: Path, process: str) -> dict:
        state = host.default_state(root)
        host.set_step(state, "updater.process", "confirmed", process)
        return state

    def test_other_requires_declared_existing_entry(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            drift: list[str] = []
            host.run_gates(root, self.state(root, "other"), drift)
            self.assertTrue(any("updater.entry" in item for item in drift))

    def test_nested_webview_requires_generated_projection(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "client").mkdir()
            (root / "client/package.json").write_text("{}", encoding="utf-8")
            drift: list[str] = []
            host.run_gates(root, self.state(root, "rust"), drift)
            self.assertTrue(any("WebView requires" in item for item in drift))

    def test_unrelated_serde_default_is_not_an_updater_shape(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "src").mkdir()
            (root / "src/update.rs").write_text(
                "#[serde(default)]\nstruct Connection { tls: bool }\n",
                encoding="utf-8",
            )
            (root / "relkit.json").write_text(
                json.dumps({"updater": {"entry": "src/update.rs"}}),
                encoding="utf-8",
            )
            drift: list[str] = []
            host.run_gates(root, self.state(root, "other"), drift)
            self.assertFalse(any("handwritten updater shape" in item for item in drift))

    def test_registered_sdk_directory_is_a_sidecar_chokepoint(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "src/update").mkdir(parents=True)
            (root / "src/other").mkdir(parents=True)
            (root / "src/update/facade.rs").write_text(
                "use relkit_updater::Updater;\nconst BIN: &str = \"relkit-updater\";\n",
                encoding="utf-8",
            )
            (root / "src/update/install.rs").write_text(
                'run("relkit-updater");\n', encoding="utf-8"
            )
            (root / "src/other/duplicate.rs").write_text(
                'run("relkit-updater");\n', encoding="utf-8"
            )
            drift: list[str] = []
            host.run_gates(root, self.state(root, "rust"), drift)
            self.assertFalse(any("src/update/" in item and "sidecar name" in item for item in drift))
            self.assertTrue(any("src/other/duplicate.rs" in item for item in drift))

    def test_other_entry_is_only_sidecar_chokepoint(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "src").mkdir()
            (root / "src/update.rs").write_text('run("relkit-updater");\n', encoding="utf-8")
            (root / "src/second.rs").write_text('run("relkit-updater");\n', encoding="utf-8")
            (root / "relkit.json").write_text(
                json.dumps({"updater": {"entry": "src/update.rs"}}), encoding="utf-8"
            )
            drift: list[str] = []
            host.run_gates(root, self.state(root, "other"), drift)
            self.assertTrue(any("src/second.rs" in item for item in drift))
            self.assertFalse(any("src/update.rs" in item and "sidecar name" in item for item in drift))

    def test_url_allowlist_does_not_exempt_shapes(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "src").mkdir()
            entry = root / "src/update.ts"
            entry.write_text(
                'interface UpdateAvailable {}\nconst u="https://x.test/rup/index";\n',
                encoding="utf-8",
            )
            (root / "relkit.json").write_text(
                json.dumps({"updater": {
                    "entry": "src/update.ts",
                    "urlAllowlist": ["src/update.ts"],
                }}),
                encoding="utf-8",
            )
            drift: list[str] = []
            host.run_gates(root, self.state(root, "other"), drift)
            self.assertFalse(any("not allowlisted" in item for item in drift))
            self.assertTrue(any("handwritten updater shape" in item for item in drift))

    def test_in_process_updater_is_not_a_host_calling_surface(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "internal").mkdir()
            (root / "internal/update.go").write_text(
                "package update\n\nvar _ = " + "sdk." + "Updater{}\n",
                encoding="utf-8",
            )
            drift: list[str] = []
            host.run_gates(root, self.state(root, "go"), drift)
            self.assertTrue(any("in-process updater API" in item for item in drift))

    def test_engine_authoring_tree_may_keep_frozen_updater(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "sdk").mkdir()
            (root / "sdk/updater.go").write_text(
                "package sdk\n\ntype Updater struct{}\nvar _ = " + "Rup" + "Updater{}\n",
                encoding="utf-8",
            )
            drift: list[str] = []
            host.run_gates(root, self.state(root, "go"), drift)
            self.assertFalse(any("in-process updater API" in item for item in drift))


class ConsumerEntryGateTests(unittest.TestCase):
    def state(self, root: Path) -> dict:
        state = host.default_state(root)
        host.set_step(state, "consume.lock", "verified", "v0.3.25")
        return state

    def drift_for(self, write: object) -> list[str]:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            write(root)
            drift: list[str] = []
            host.run_gates(root, self.state(root), drift)
            return drift

    def test_product_import_of_the_consumer_is_drift(self) -> None:
        def write(root: Path) -> None:
            (root / "scripts").mkdir()
            (root / "scripts/build.py").write_text(
                "import relkit_consume\n", encoding="utf-8"
            )

        drift = self.drift_for(write)
        self.assertTrue(any("release-internal" in item for item in drift))
        self.assertTrue(any("scripts/build.py" in item for item in drift))

    def test_retired_root_consumer_path_is_drift(self) -> None:
        def write(root: Path) -> None:
            (root / "scripts").mkdir()
            (root / "scripts/ci_release.sh").write_text(
                'python "$DIR/scripts/relkit_consume.py" install\n', encoding="utf-8"
            )

        drift = self.drift_for(write)
        self.assertTrue(any("is retired" in item for item in drift))

    def test_installed_host_tree_is_not_a_product_import(self) -> None:
        def write(root: Path) -> None:
            (root / "scripts/host").mkdir(parents=True)
            (root / "scripts/host/relkit_host.py").write_text(
                "import relkit_consume\n", encoding="utf-8"
            )

        self.assertEqual(self.drift_for(write), [])

    def test_unanswered_lock_keeps_the_gate_quiet(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "scripts").mkdir()
            (root / "scripts/build.py").write_text(
                "import relkit_consume\n", encoding="utf-8"
            )
            drift: list[str] = []
            host.run_gates(root, host.default_state(root), drift)
            self.assertEqual(drift, [])


class HostPythonFloorGateTests(unittest.TestCase):
    def state(self, root: Path) -> dict:
        state = host.default_state(root)
        host.set_step(state, "consume.lock", "verified", "v0.3.25")
        return state

    def drift_for(self, body: str, name: str = "scripts/ci_release.sh") -> list[str]:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(body, encoding="utf-8")
            drift: list[str] = []
            host.run_gates(root, self.state(root), drift)
            return [item for item in drift if "Python 3." in item]

    def test_entry_script_accepting_python38_is_drift(self) -> None:
        drift = self.drift_for(
            "MIN='import sys; sys.exit(0 if sys.version_info >= (3, 8) else 1)'\n"
            'python3 scripts/host/relkit_host.py install\n'
        )
        self.assertTrue(any("accepts Python 3.8" in item for item in drift))

    def test_installing_python38_is_drift(self) -> None:
        drift = self.drift_for(
            "yum install -y python38\n"
            'python3.8 scripts/host/relkit_host.py install\n'
        )
        self.assertTrue(any("accepts Python 3.8" in item for item in drift))

    def test_modern_floor_is_clean(self) -> None:
        drift = self.drift_for(
            "MIN='import sys; sys.exit(0 if sys.version_info >= (3, 9) else 1)'\n"
            "for c in python3 python3.12 python3.9; do :; done\n"
            'python3 scripts/host/relkit_host.py install\n'
        )
        self.assertEqual(drift, [])

    def test_script_that_never_calls_the_host_is_not_scanned(self) -> None:
        drift = self.drift_for(
            "yum install -y python38\npython3.8 scripts/other.py\n",
            name="scripts/unrelated.sh",
        )
        self.assertEqual(drift, [])

    def test_windows_batch_proxy_is_scanned(self) -> None:
        drift = self.drift_for(
            "py -3.8 scripts\\host\\relkit_host.py install\r\n",
            name="scripts/ci_release.bat",
        )
        self.assertTrue(any("ci_release.bat" in item for item in drift))


class SidecarUniversalTests(unittest.TestCase):
    def test_refuses_outside_macos(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            with patch("relkit_host.sys.platform", "win32"):
                with self.assertRaises(host.Fail) as caught:
                    host.cmd_sidecar_universal(Path(raw), Path(raw) / "out")
            self.assertEqual(caught.exception.code, "sidecar-universal-not-darwin")

    def test_fuses_both_darwin_attachments(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            requested: list[str] = []

            class FakeConsume:
                @staticmethod
                def load_lock(path: Path) -> dict:
                    return {"artifacts": {"updater": {}}}

                @staticmethod
                def artifact_spec(lock: dict, component: str, target: str) -> dict:
                    return {"url": f"https://x.test/{target}", "sha256": "0" * 64}

                @staticmethod
                def download_artifact(project: Path, label: str, spec: dict) -> Path:
                    requested.append(label)
                    artifact = root / label
                    artifact.write_bytes(b"binary")
                    return artifact

            out = root / "tools" / "bin" / "relkit-updater"

            def fake_fuse(inputs, destination) -> list[str]:
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_bytes(b"fat")
                return ["x86_64", "arm64"]

            with (
                patch("relkit_host.sys.platform", "darwin"),
                patch("relkit_host.import_consume", return_value=FakeConsume),
                patch("relkit_host._lipo_fuse", side_effect=fake_fuse) as fuse,
            ):
                with redirect_stdout(io.StringIO()):
                    self.assertEqual(host.cmd_sidecar_universal(root, out), 0)
            self.assertEqual(requested, ["updater-darwin-amd64", "updater-darwin-arm64"])
            self.assertEqual(len(fuse.call_args.args[0]), 2)
            self.assertTrue(out.is_file())

    def test_missing_attachment_is_a_classified_failure(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)

            class FakeConsume:
                @staticmethod
                def load_lock(path: Path) -> dict:
                    return {"artifacts": {}}

                @staticmethod
                def artifact_spec(lock: dict, component: str, target: str) -> dict:
                    raise RuntimeError(f"no {component} for {target}")

            with (
                patch("relkit_host.sys.platform", "darwin"),
                patch("relkit_host.import_consume", return_value=FakeConsume),
            ):
                with self.assertRaises(host.Fail) as caught:
                    host.cmd_sidecar_universal(root, root / "out")
            self.assertEqual(
                caught.exception.code, "sidecar-universal-missing-attachment"
            )

    def test_sidecar_universal_parses(self) -> None:
        args = host.build_parser().parse_args(
            ["sidecar", "universal", "--out", "tools/bin/relkit-updater"]
        )
        self.assertEqual(args.cmd, "sidecar")
        self.assertEqual(args.sidecar_cmd, "universal")


class HostPythonFloorDeclarationTests(unittest.TestCase):
    def test_entry_guards_match_the_declared_floor(self) -> None:
        floor = re.compile(r"sys\.version_info\s*<\s*\((\d+),\s*(\d+)\)")
        scripts = Path(__file__).parent / "host"
        for name in ("relkit_host.py", "relkit_consume.py"):
            found = floor.search((scripts / name).read_text(encoding="utf-8"))
            self.assertIsNotNone(found, name)
            self.assertEqual(
                (int(found.group(1)), int(found.group(2))),
                tuple(host.MIN_PYTHON),
                name,
            )


if __name__ == "__main__":
    unittest.main()
