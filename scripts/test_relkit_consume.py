#!/usr/bin/env python3
from __future__ import annotations

import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent / "host"))

import relkit_consume as subject


class ArgvTests(unittest.TestCase):
    def test_injects_root_and_lock(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            lock = root / "scripts" / "relkit.lock.json"
            lock.parent.mkdir()
            lock.write_text("{}", encoding="utf-8")
            out = subject.prepare_argv(["--sdk-only"], root, lock)
            self.assertEqual(
                out[:4],
                ["--lock", str(lock), "--project-root", str(root)],
            )
            self.assertEqual(out[-1], "--sdk-only")

    def test_does_not_duplicate_flags(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            lock = root / "scripts" / "relkit.lock.json"
            lock.parent.mkdir()
            lock.write_text("{}", encoding="utf-8")
            out = subject.prepare_argv(
                ["--project-root", "/x", "--lock", "/y", "--sdk-only"],
                root,
                lock,
            )
            self.assertEqual(out, ["--project-root", "/x", "--lock", "/y", "--sdk-only"])


class RawUrlTests(unittest.TestCase):
    def test_github_raw(self) -> None:
        urls = subject.raw_consume_urls(
            "https://github.com/shichao402/relkit.git",
            "abc1234",
        )
        self.assertIn(
            "https://raw.githubusercontent.com/shichao402/relkit/abc1234/scripts/consume.py",
            urls,
        )


class SelectConsumeTests(unittest.TestCase):
    def test_existing_checkout_wins_over_download(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            dest = root / "third_party" / "relkit" / "scripts"
            dest.mkdir(parents=True)
            consume = dest / "consume.py"
            consume.write_text("local", encoding="utf-8")
            with patch.object(subject, "download_bytes") as download:
                self.assertEqual(subject.select_consume_py(root, {}, "main"), consume)
            download.assert_not_called()

    def test_bootstrap_downloads_when_no_checkout(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            with patch.object(subject, "download_bytes", return_value=b"downloaded"):
                path = subject.select_consume_py(root, {}, "main")
            self.assertEqual(path.read_bytes(), b"downloaded")

    def test_env_override(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "consume.py"
            path.write_text("x", encoding="utf-8")
            with patch.dict(os.environ, {"RELKIT_CONSUME_PY": str(path)}):
                self.assertEqual(
                    subject.select_consume_py(Path(raw), {}, "main"),
                    path,
                )


if __name__ == "__main__":
    unittest.main()
