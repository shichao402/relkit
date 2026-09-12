#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import io
import json
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest.mock import patch
from urllib.error import URLError

sys.path.insert(0, str(Path(__file__).parent / "host"))
import relkit_consume as subject


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sdk_zip() -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w") as archive:
        archive.writestr("pubspec.yaml", "name: rup_client\n")
        archive.writestr("lib/rup_client.dart", "library;\n")
    return output.getvalue()


def rust_sdk_zip() -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w") as archive:
        archive.writestr("Cargo.toml", "[package]\nname='relkit-updater'\n")
        archive.writestr("src/lib.rs", "pub const IPC_MIN: u32 = 1;\n")
        archive.writestr("proto/updater/v1/updater.proto", 'syntax = "proto3";\n')
    return output.getvalue()


def host_scripts_zip() -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w") as archive:
        archive.writestr("relkit_consume.py", "# consume\n")
        archive.writestr("relkit_host.py", "# host\n")
    return output.getvalue()


def lock_for(url: str, sdk: bytes, binary: bytes, rust_sdk: bytes | None = None) -> dict:
    spec = lambda data: {"url": url, "sha256": sha256(data)}
    lock = {
        "schema": subject.LOCK_SCHEMA,
        "release": "v1.2.3",
        "commit": "a" * 40,
        "artifacts": {
            "sdk-dart": spec(sdk),
            "cli": {"linux-amd64": spec(binary)},
            "updater": {"linux-amd64": spec(binary)},
        },
    }
    if rust_sdk is not None:
        lock["artifacts"]["sdk-rust"] = spec(rust_sdk)
    return lock


class LockTests(unittest.TestCase):
    def test_rejects_source_build_lock(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "lock.json"
            path.write_text(
                json.dumps({"schema": "relkit.consume/1", "commit": "a" * 40}),
                encoding="utf-8",
            )
            with self.assertRaisesRegex(RuntimeError, "source-build locks"):
                subject.load_lock(path)

    def test_requires_pinned_artifact_hash(self) -> None:
        lock = {
            "schema": subject.LOCK_SCHEMA,
            "release": "v1",
            "commit": "a" * 40,
            "artifacts": {"cli": {"linux-amd64": {"url": "https://x"}}},
        }
        with self.assertRaisesRegex(RuntimeError, "invalid sha256"):
            subject.artifact_spec(lock, "cli", "linux-amd64")


class InstallTests(unittest.TestCase):
    def test_installs_sdk_and_binary_without_git_or_go(self) -> None:
        sdk = sdk_zip()
        binary = b"executable"
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            lock = lock_for("https://example.invalid/artifact", sdk, binary)

            def fake_download(_root, component, _spec):
                path = root / f"{component}.artifact"
                path.write_bytes(sdk if component == "sdk-dart" else binary)
                return path

            with (
                patch.object(subject, "host_root", return_value=root),
                patch.object(subject, "host_target", return_value="linux-amd64"),
                patch.object(subject, "load_lock", return_value=lock),
                patch.object(subject, "download_artifact", side_effect=fake_download),
                patch.object(subject, "verify_binary"),
            ):
                self.assertEqual(subject.main(["install"]), 0)

            self.assertTrue(
                (root / "third_party/relkit/sdk/dart/pubspec.yaml").is_file()
            )
            self.assertEqual(
                (root / "tools/bin/relkit-linux-amd64").read_bytes(), binary
            )
            self.assertEqual(
                (root / "tools/bin/relkit-updater").read_bytes(), binary
            )

    def test_rejects_zip_path_traversal(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            archive_path = root / "bad.zip"
            with zipfile.ZipFile(archive_path, "w") as archive:
                archive.writestr("../escape", "bad")
            with self.assertRaisesRegex(RuntimeError, "unsafe SDK archive"):
                subject.safe_extract_sdk(
                    archive_path,
                    root / "third_party/relkit/sdk/dart",
                    sha256(archive_path.read_bytes()),
                )

    def test_install_self_repairs_host_scripts_before_other_components(self) -> None:
        scripts = host_scripts_zip()
        sdk = sdk_zip()
        binary = b"executable"
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            host_dir = root / "scripts" / "host"
            host_dir.mkdir(parents=True)
            (host_dir / "relkit_consume.py").write_text("# stale\n", encoding="utf-8")
            (host_dir / "relkit_host.py").write_text("# stale\n", encoding="utf-8")

            expected_dir = root / "expected"
            expected_dir.mkdir()
            (expected_dir / "relkit_consume.py").write_text("# consume\n", encoding="utf-8")
            (expected_dir / "relkit_host.py").write_text("# host\n", encoding="utf-8")
            lock = lock_for("https://example.invalid/artifact", sdk, binary)
            lock["hostScriptsSha256"] = subject.host_scripts_tree_sha256(expected_dir)
            lock["artifacts"]["host-scripts"] = {
                "url": "https://example.invalid/host-scripts",
                "sha256": sha256(scripts),
            }

            def fake_download(_root, component, _spec):
                path = root / f"{component}.artifact"
                if component == "host-scripts":
                    path.write_bytes(scripts)
                elif component == "sdk-dart":
                    path.write_bytes(sdk)
                else:
                    path.write_bytes(binary)
                return path

            with (
                patch.object(subject, "host_target", return_value="linux-amd64"),
                patch.object(subject, "load_lock", return_value=lock),
                patch.object(subject, "download_artifact", side_effect=fake_download),
                patch.object(subject, "verify_binary"),
            ):
                self.assertEqual(
                    subject.main(["install", "--project-root", str(root)]),
                    0,
                )

            self.assertEqual(
                subject.host_scripts_tree_sha256(host_dir),
                lock["hostScriptsSha256"],
            )

    def test_host_script_update_rolls_back_when_second_replace_fails(self) -> None:
        scripts = host_scripts_zip()
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            artifact = root / "host-scripts.zip"
            artifact.write_bytes(scripts)
            destination = root / "scripts" / "host"
            destination.mkdir(parents=True)
            old = {
                "relkit_consume.py": b"# old consume\n",
                "relkit_host.py": b"# old host\n",
            }
            for name, data in old.items():
                (destination / name).write_bytes(data)

            expected_dir = root / "expected"
            expected_dir.mkdir()
            (expected_dir / "relkit_consume.py").write_text("# consume\n", encoding="utf-8")
            (expected_dir / "relkit_host.py").write_text("# host\n", encoding="utf-8")
            expected = subject.host_scripts_tree_sha256(expected_dir)
            real_replace = subject.os.replace
            failed = False

            def flaky_replace(source, target):
                nonlocal failed
                if Path(target).name == "relkit_host.py" and not failed:
                    failed = True
                    raise PermissionError("locked")
                return real_replace(source, target)

            with (
                patch.object(subject.os, "replace", side_effect=flaky_replace),
                self.assertRaises(PermissionError),
            ):
                subject.safe_extract_host_scripts(artifact, destination, expected)

            for name, data in old.items():
                self.assertEqual((destination / name).read_bytes(), data)

    def test_installs_rust_sdk_to_stable_path_with_proto(self) -> None:
        sdk = rust_sdk_zip()
        binary = b"executable"
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            lock = lock_for("https://example.invalid/artifact", sdk_zip(), binary, sdk)

            def fake_download(_root, component, _spec):
                path = root / f"{component}.artifact"
                path.write_bytes(sdk if component == "sdk-rust" else binary)
                return path

            with (
                patch.object(subject, "host_target", return_value="linux-amd64"),
                patch.object(subject, "load_lock", return_value=lock),
                patch.object(subject, "download_artifact", side_effect=fake_download),
            ):
                self.assertEqual(
                    subject.main(
                        [
                            "install",
                            "--project-root",
                            str(root),
                            "--component",
                            "sdk-rust",
                        ]
                    ),
                    0,
                )

            installed = root / "third_party/relkit/sdk/rust"
            self.assertTrue((installed / "Cargo.toml").is_file())
            self.assertTrue((installed / "proto/updater/v1/updater.proto").is_file())


class DownloadTests(unittest.TestCase):
    def test_uses_system_curl_when_python_tls_fails(self) -> None:
        payload = b"release artifact"
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            spec = {
                "url": "https://example.invalid/x",
                "sha256": sha256(payload),
            }

            def fake_curl(_url, destination):
                destination.write_bytes(payload)

            with (
                patch.object(subject, "urlopen", side_effect=URLError("bad CA")),
                patch.object(subject, "download_with_curl", side_effect=fake_curl),
            ):
                installed = subject.download_artifact(root, "cli", spec)
            self.assertEqual(installed.read_bytes(), payload)

    def test_curl_keeps_tls_verification_enabled(self) -> None:
        with (
            tempfile.TemporaryDirectory() as raw,
            patch.object(subject.shutil, "which", return_value="curl") as which,
            patch.object(
                subject.subprocess,
                "run",
                return_value=subject.subprocess.CompletedProcess([], 0, "", ""),
            ) as run,
        ):
            subject.download_with_curl(
                "https://example.invalid/x", Path(raw) / "artifact"
            )
        which.assert_called_once()
        command = run.call_args.args[0]
        self.assertIn("--tlsv1.2", command)
        self.assertNotIn("--insecure", command)

    def test_certificate_failure_allows_checksum_guarded_fallback(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            destination = Path(raw) / "artifact"

            def python_download(_url: str, path: Path, *, verify: bool) -> None:
                if verify:
                    raise subject.CertificateVerificationError("expired CA")
                path.write_bytes(b"pinned")

            with (
                patch.object(subject, "download_with_python", side_effect=python_download),
                patch.object(
                    subject,
                    "download_with_curl",
                    side_effect=RuntimeError("curl unavailable"),
                ),
            ):
                subject.fetch_url("https://example.invalid/artifact", destination)

            self.assertEqual(destination.read_bytes(), b"pinned")

    def test_network_failure_never_allows_insecure_fallback(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            destination = Path(raw) / "artifact"
            calls: list[bool] = []

            def python_download(_url: str, _path: Path, *, verify: bool) -> None:
                calls.append(verify)
                raise TimeoutError("network timeout")

            with (
                patch.object(subject, "download_with_python", side_effect=python_download),
                patch.object(
                    subject,
                    "download_with_curl",
                    side_effect=RuntimeError("network unreachable"),
                ),
            ):
                with self.assertRaisesRegex(RuntimeError, "refusing insecure fallback"):
                    subject.fetch_url("https://example.invalid/artifact", destination)

            self.assertEqual(calls, [True])

    def test_hash_mismatch_never_enters_cache(self) -> None:
        class Response(io.BytesIO):
            def __enter__(self):
                return self

            def __exit__(self, *args):
                self.close()

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            spec = {"url": "https://example.invalid/x", "sha256": "0" * 64}
            with (
                patch.object(
                    subject, "urlopen", side_effect=lambda *_args, **_kwargs: Response(b"wrong")
                ),
                patch.object(subject.time, "sleep"),
            ):
                with self.assertRaisesRegex(RuntimeError, "sha256 mismatch"):
                    subject.download_artifact(root, "cli", spec)
            self.assertFalse((root / ".relkit/artifacts" / ("0" * 64)).exists())


if __name__ == "__main__":
    unittest.main()
