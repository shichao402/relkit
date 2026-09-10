#!/usr/bin/env python3
from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

import consume as subject


class _Logger:
    def __init__(self) -> None:
        self.warnings: list[str] = []

    def warn(self, message: str) -> None:
        self.warnings.append(message)

    def info(self, message: str) -> None:
        pass


class RequiredRootFilesTests(unittest.TestCase):
    def test_restores_files_from_head_when_sparse_checkout_omits_them(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw)
            subprocess.run(["git", "init", "-q"], cwd=repo, check=True)
            subprocess.run(
                ["git", "config", "user.email", "test@example.invalid"],
                cwd=repo,
                check=True,
            )
            subprocess.run(
                ["git", "config", "user.name", "test"],
                cwd=repo,
                check=True,
            )
            expected = {
                "go.mod": b"module example.invalid/probe\n\ngo 1.26\n",
                "go.sum": b"example.invalid/module v1.0.0 h1:probe\n",
            }
            for name, data in expected.items():
                (repo / name).write_bytes(data)
            subprocess.run(["git", "add", *expected], cwd=repo, check=True)
            subprocess.run(["git", "commit", "-q", "-m", "fixture"], cwd=repo, check=True)

            for name in expected:
                (repo / name).unlink()

            logger = _Logger()
            subject.materialize_required_root_files(logger, repo)

            self.assertTrue(logger.warnings)
            for name, data in expected.items():
                self.assertEqual((repo / name).read_bytes(), data)

    def test_keeps_files_already_materialized(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw)
            for name in subject.REQUIRED_ROOT_FILES:
                (repo / name).write_text(name, encoding="utf-8")

            logger = _Logger()
            subject.materialize_required_root_files(logger, repo)

            self.assertEqual(logger.warnings, [])


class RequiredCheckoutDirsTests(unittest.TestCase):
    @staticmethod
    def _repo(root: Path) -> None:
        subprocess.run(["git", "init", "-q"], cwd=root, check=True)
        subprocess.run(
            ["git", "config", "user.email", "test@example.invalid"],
            cwd=root,
            check=True,
        )
        subprocess.run(["git", "config", "user.name", "test"], cwd=root, check=True)
        for relative in subject.REQUIRED_CHECKOUT_DIRS:
            target = root / relative / "probe.go"
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text("package probe\n", encoding="utf-8")
        subprocess.run(["git", "add", "-A"], cwd=root, check=True)
        subprocess.run(["git", "commit", "-q", "-m", "fixture"], cwd=root, check=True)

    def test_drops_the_cone_when_a_build_critical_dir_is_missing(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw)
            self._repo(repo)
            subprocess.run(
                ["git", "sparse-checkout", "set", "--cone", "cmd/relkit"],
                cwd=repo,
                check=True,
            )
            self.assertFalse((repo / "internal").is_dir())

            logger = _Logger()
            subject.materialize_required_dirs(logger, repo)

            self.assertTrue(logger.warnings)
            for relative in subject.REQUIRED_CHECKOUT_DIRS:
                self.assertTrue((repo / relative / "probe.go").is_file(), relative)

    def test_repairs_a_directory_left_without_its_files(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw)
            self._repo(repo)
            (repo / "internal" / "probe.go").unlink()

            logger = _Logger()
            subject.materialize_required_dirs(logger, repo)

            self.assertTrue(logger.warnings)
            self.assertTrue((repo / "internal" / "probe.go").is_file())

    def test_keeps_a_complete_checkout_sparse(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw)
            self._repo(repo)
            subprocess.run(
                ["git", "sparse-checkout", "set", "--cone", *subject.REQUIRED_CHECKOUT_DIRS],
                cwd=repo,
                check=True,
            )

            logger = _Logger()
            subject.materialize_required_dirs(logger, repo)

            self.assertEqual(logger.warnings, [])
            self.assertTrue((repo / ".git" / "info" / "sparse-checkout").is_file())


class LockTests(unittest.TestCase):
    def test_commit_wins_over_channel(self) -> None:
        self.assertEqual(
            subject.lock_ref({"channel": "main", "commit": "abc1234"}, ""),
            "abc1234",
        )

    def test_override_wins_over_lock(self) -> None:
        self.assertEqual(
            subject.lock_ref({"channel": "main", "commit": "abc1234"}, "other"),
            "other",
        )

    def test_cnb_token_injected(self) -> None:
        import os
        from unittest.mock import patch

        with patch.dict(os.environ, {"CNB_TOKEN": "secret"}):
            self.assertEqual(
                subject.inject_cnb_token("https://cnb.cool/org/relkit.git"),
                "https://cnb:secret@cnb.cool/org/relkit.git",
            )


if __name__ == "__main__":
    unittest.main()
