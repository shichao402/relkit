#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import io
import json
import stat
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


def go_sdk_zip() -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w") as archive:
        archive.writestr("go.mod", "module go.firoyang.com/relkit\n")
        archive.writestr("go.sum", "")
        archive.writestr("sdk/doc.go", "package sdk\n")
        archive.writestr("api/updater/v1/updater.pb.go", "package updaterv1\n")
    return output.getvalue()


def host_scripts_zip() -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w") as archive:
        for name in subject.BY_NAME["host-scripts"].required_paths:
            archive.writestr(name, f"# {name}\n")
    return output.getvalue()


def write_host_scripts(directory: Path) -> None:
    for name in subject.BY_NAME["host-scripts"].required_paths:
        path = directory / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(f"# {name}\n", encoding="utf-8")


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

            def fake_download(_root, component, _spec, **_kwargs):
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

    def test_go_sdk_lands_at_the_module_root(self) -> None:
        archive = go_sdk_zip()
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            artifact = root / "sdk-go.zip"
            artifact.write_bytes(archive)
            destination = subject.sdk_destination(root, "sdk-go")
            self.assertEqual(destination, root / "third_party" / "relkit")
            subject.safe_extract_sdk(
                artifact, destination, sha256(archive), "sdk-go"
            )
            # The host's `replace` target must resolve to a buildable module.
            self.assertTrue((destination / "go.mod").is_file())
            self.assertTrue((destination / "sdk" / "doc.go").is_file())
            self.assertTrue(
                (destination / "api" / "updater" / "v1" / "updater.pb.go").is_file()
            )

    def test_go_sdk_install_keeps_sibling_sdk_artifacts(self) -> None:
        archive = go_sdk_zip()
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            artifact = root / "sdk-go.zip"
            artifact.write_bytes(archive)
            destination = subject.sdk_destination(root, "sdk-go")
            dart = destination / "sdk" / "dart"
            dart.mkdir(parents=True)
            (dart / "pubspec.yaml").write_text("name: rup_client\n", encoding="utf-8")
            subject.safe_extract_sdk(
                artifact, destination, sha256(archive), "sdk-go"
            )
            self.assertTrue((dart / "pubspec.yaml").is_file())
            self.assertTrue((destination / "go.mod").is_file())

    def test_go_sdk_replaces_a_read_only_sparse_checkout(self) -> None:
        archive = go_sdk_zip()
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            artifact = root / "sdk-go.zip"
            artifact.write_bytes(archive)
            destination = subject.sdk_destination(root, "sdk-go")
            # A relkit.consume/1 checkout leaves read-only git pack files behind.
            pack = destination / ".git" / "objects" / "pack"
            pack.mkdir(parents=True)
            idx = pack / "pack-cafe.idx"
            idx.write_bytes(b"pack")
            idx.chmod(stat.S_IREAD)
            try:
                subject.safe_extract_sdk(
                    artifact, destination, sha256(archive), "sdk-go"
                )
            finally:
                if idx.is_file():
                    idx.chmod(stat.S_IWRITE)
            self.assertTrue((destination / "go.mod").is_file())
            self.assertFalse(destination.with_name(destination.name + ".relkit-old").exists())

    def test_incomplete_go_sdk_is_rejected(self) -> None:
        output = io.BytesIO()
        with zipfile.ZipFile(output, "w") as archive:
            archive.writestr("go.mod", "module go.firoyang.com/relkit\n")
        payload = output.getvalue()
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            artifact = root / "sdk-go.zip"
            artifact.write_bytes(payload)
            with self.assertRaisesRegex(RuntimeError, "incomplete"):
                subject.safe_extract_sdk(
                    artifact,
                    subject.sdk_destination(root, "sdk-go"),
                    sha256(payload),
                    "sdk-go",
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
            write_host_scripts(expected_dir)
            lock = lock_for("https://example.invalid/artifact", sdk, binary)
            lock["hostScriptsSha256"] = subject.host_scripts_tree_sha256(expected_dir)
            lock["artifacts"]["host-scripts"] = {
                "url": "https://example.invalid/host-scripts",
                "sha256": sha256(scripts),
            }

            def fake_download(_root, component, _spec, **_kwargs):
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
            write_host_scripts(expected_dir)
            expected = subject.host_scripts_tree_sha256(expected_dir)
            real_replace = subject.os.replace
            failed = False

            def flaky_replace(source, target):
                nonlocal failed
                if Path(target).name == "host" and Path(source).name.startswith(".host-scripts-") and not failed:
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
            orphan = root / "third_party/relkit/proto/updater/v1"
            orphan.mkdir(parents=True)
            (orphan / "updater.proto").write_text(
                'syntax = "proto3"; // stale leftover\n', encoding="utf-8"
            )

            def fake_download(_root, component, _spec, **_kwargs):
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
            self.assertFalse((root / "third_party/relkit/proto").exists())


class DownloadTests(unittest.TestCase):
    def setUp(self) -> None:
        subject._windows_ca_bundle = None
        subject._curl_backends.clear()

    def test_uses_system_curl_when_python_tls_fails(self) -> None:
        payload = b"release artifact"
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            spec = {
                "url": "https://example.invalid/x",
                "sha256": sha256(payload),
            }

            def fake_curl(_url, destination, **_kwargs):
                destination.write_bytes(payload)

            with (
                patch.object(subject, "urlopen", side_effect=URLError("bad CA")),
                patch.object(subject, "download_with_curl", side_effect=fake_curl),
                patch.object(
                    subject,
                    "download_with_powershell",
                    side_effect=AssertionError("powershell should not run"),
                ),
                patch.object(subject, "resolve_curl", return_value=r"C:\Windows\System32\curl.exe"),
                patch.object(subject, "detect_curl_backend", return_value="schannel"),
            ):
                installed = subject.download_artifact(root, "cli", spec)
            self.assertEqual(installed.read_bytes(), payload)

    def test_resolve_curl_prefers_system32_over_path_cygwin(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            system = Path(raw) / "System32" / "curl.exe"
            system.parent.mkdir(parents=True)
            system.write_bytes(b"MZ")
            cygwin = Path(raw) / "cygwin" / "curl.exe"
            cygwin.parent.mkdir(parents=True)
            cygwin.write_bytes(b"MZ")

            def fake_backend(path: str) -> str:
                return "schannel" if path == str(system) else "openssl"

            with (
                patch.object(subject.os, "name", "nt"),
                patch.object(subject, "system32_curl", return_value=system),
                patch.object(subject.shutil, "which", return_value=str(cygwin)) as which,
                patch.object(subject, "detect_curl_backend", side_effect=fake_backend),
            ):
                chosen = subject.resolve_curl(environ={})
            self.assertEqual(chosen, str(system))
            which.assert_not_called()

    def test_resolve_curl_respects_relkit_curl_override(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            override = Path(raw) / "custom-curl.exe"
            override.write_bytes(b"MZ")
            with patch.object(subject, "system32_curl", return_value=None):
                chosen = subject.resolve_curl(
                    environ={subject.CURL_OVERRIDE_ENV: str(override)}
                )
            self.assertEqual(chosen, str(override))

    def test_detect_curl_backend_is_per_binary(self) -> None:
        subject._curl_backends.clear()
        with patch.object(
            subject.subprocess,
            "run",
            side_effect=[
                subject.subprocess.CompletedProcess([], 0, "curl ... OpenSSL/1.0.2d", ""),
                subject.subprocess.CompletedProcess([], 0, "curl ... Schannel", ""),
            ],
        ):
            first = subject.detect_curl_backend(r"C:\cygwin\bin\curl.exe")
            second = subject.detect_curl_backend(r"C:\Windows\System32\curl.exe")
        self.assertEqual(first, "openssl")
        self.assertEqual(second, "schannel")
        self.assertEqual(
            subject._curl_backends[r"C:\cygwin\bin\curl.exe"],
            "openssl",
        )

    def test_curl_keeps_tls_verification_enabled(self) -> None:
        with (
            tempfile.TemporaryDirectory() as raw,
            patch.object(subject, "resolve_curl", return_value="curl") as resolve,
            patch.object(subject, "detect_curl_backend", return_value="openssl"),
            patch.object(
                subject.subprocess,
                "run",
                return_value=subject.subprocess.CompletedProcess([], 0, "", ""),
            ) as run,
        ):
            subject.download_with_curl(
                "https://example.invalid/x", Path(raw) / "artifact"
            )
        resolve.assert_called_once()
        command = run.call_args.args[0]
        self.assertIn("--tlsv1.2", command)
        self.assertNotIn("--insecure", command)

    def test_openssl_curl_passes_windows_ca_bundle(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            bundle = Path(raw) / "windows-ca.pem"
            bundle.write_text("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n")
            with (
                patch.object(subject, "resolve_curl", return_value="curl"),
                patch.object(subject, "detect_curl_backend", return_value="openssl"),
                patch.object(
                    subject.subprocess,
                    "run",
                    return_value=subject.subprocess.CompletedProcess([], 0, "", ""),
                ) as run,
            ):
                subject.download_with_curl(
                    "https://example.invalid/x",
                    Path(raw) / "artifact",
                    ca_bundle=bundle,
                )
            command = run.call_args.args[0]
            self.assertIn("--cacert", command)
            self.assertIn(str(bundle), command)
            self.assertNotIn("--insecure", command)

    def test_schannel_curl_skips_cacert(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            bundle = Path(raw) / "windows-ca.pem"
            bundle.write_text("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n")
            with (
                patch.object(subject, "resolve_curl", return_value="curl"),
                patch.object(subject, "detect_curl_backend", return_value="schannel"),
                patch.object(
                    subject.subprocess,
                    "run",
                    return_value=subject.subprocess.CompletedProcess([], 0, "", ""),
                ) as run,
            ):
                subject.download_with_curl(
                    "https://example.invalid/x",
                    Path(raw) / "artifact",
                    ca_bundle=bundle,
                )
            command = run.call_args.args[0]
            self.assertNotIn("--cacert", command)

    def test_transport_log_includes_backend_and_ca_source(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            destination = Path(raw) / "artifact"
            lines: list[str] = []

            def capture(message: str, **_kwargs) -> None:
                lines.append(message)

            with (
                patch.object(subject, "resolve_ca_bundle", return_value=None),
                patch.object(
                    subject,
                    "resolve_curl",
                    return_value=r"C:\Windows\System32\curl.exe",
                ),
                patch.object(subject, "detect_curl_backend", return_value="schannel"),
                patch.object(
                    subject,
                    "download_with_curl",
                    side_effect=lambda *a, **k: destination.write_bytes(b"ok"),
                ),
                patch.object(
                    subject,
                    "download_with_powershell",
                    side_effect=AssertionError("powershell should not run"),
                ),
                patch.object(
                    subject,
                    "download_with_python",
                    side_effect=AssertionError("python should not run"),
                ),
                patch.object(subject.sys, "stderr"),
                patch("builtins.print", side_effect=capture),
            ):
                subject.fetch_url("https://github.example/asset.zip", destination)
            joined = "\n".join(lines)
            self.assertIn("relkit consume: transport attempt", joined)
            self.assertIn("backend=schannel", joined)
            self.assertIn("relkit consume: transport success", joined)
            self.assertIn("method=system curl", joined)

    def test_python_loads_ca_bundle_into_ssl_context(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            bundle = Path(raw) / "corp-ca.pem"
            # Minimal PEM is enough: we only assert load_verify_locations is called.
            bundle.write_text(
                "-----BEGIN CERTIFICATE-----\n"
                "MIIBkTCB+wIJAKHBfJXrfIqEMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBmRv\n"
                "bW15bjAeFw0yNDAxMDEwMDAwMDBaFw0zNDAxMDEwMDAwMDBaMBExDzANBgNVBAMM\n"
                "BmRvbW15bjBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQC5dummy\n"
                "-----END CERTIFICATE-----\n"
            )
            destination = Path(raw) / "artifact"
            loaded: list[str] = []

            class FakeContext:
                def load_verify_locations(self, *, cafile: str) -> None:
                    loaded.append(cafile)

            class Response(io.BytesIO):
                def __enter__(self):
                    return self

                def __exit__(self, *args):
                    self.close()

            with (
                patch.object(
                    subject.ssl, "create_default_context", return_value=FakeContext()
                ),
                patch.object(
                    subject, "urlopen", side_effect=lambda *_a, **_k: Response(b"ok")
                ),
            ):
                # Invalid PEM will fail load_verify_locations on a real context;
                # FakeContext records the path instead.
                subject.download_with_python(
                    "https://example.invalid/x",
                    destination,
                    verify=True,
                    ca_bundle=bundle,
                )
            self.assertEqual(loaded, [str(bundle)])
            self.assertEqual(destination.read_bytes(), b"ok")

    def test_certificate_failure_is_fail_closed_by_default(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            destination = Path(raw) / "artifact"
            verify_calls: list[bool] = []

            def python_download(
                _url: str, path: Path, *, verify: bool, ca_bundle=None
            ) -> None:
                verify_calls.append(verify)
                if verify:
                    raise subject.CertificateVerificationError("expired CA")
                path.write_bytes(b"should-not-run")

            with (
                patch.object(subject, "download_with_python", side_effect=python_download),
                patch.object(
                    subject,
                    "download_with_curl",
                    side_effect=subject.CertificateVerificationError("curl CA fail"),
                ),
                patch.object(
                    subject,
                    "download_with_powershell",
                    side_effect=subject.CertificateVerificationError("ps CA fail"),
                ),
                patch.dict(subject.os.environ, {}, clear=False),
            ):
                subject.os.environ.pop(subject.ALLOW_INSECURE_ENV, None)
                with self.assertRaisesRegex(
                    RuntimeError, "certificate verification failed.*RELKIT_CONSUME_ALLOW_INSECURE"
                ):
                    subject.fetch_url(
                        "https://example.invalid/artifact",
                        destination,
                        allow_insecure=False,
                    )

            self.assertEqual(verify_calls, [True])
            self.assertFalse(destination.exists())

    def test_opt_in_insecure_fallback_still_checksum_guarded(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            destination = Path(raw) / "artifact"

            def python_download(
                _url: str, path: Path, *, verify: bool, ca_bundle=None
            ) -> None:
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
                patch.object(
                    subject,
                    "download_with_powershell",
                    side_effect=subject.CertificateVerificationError("ps CA fail"),
                ),
            ):
                subject.fetch_url(
                    "https://example.invalid/artifact",
                    destination,
                    allow_insecure=True,
                )

            self.assertEqual(destination.read_bytes(), b"pinned")

    def test_network_failure_never_allows_insecure_fallback(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            destination = Path(raw) / "artifact"
            calls: list[bool] = []

            def python_download(
                _url: str, _path: Path, *, verify: bool, ca_bundle=None
            ) -> None:
                calls.append(verify)
                raise TimeoutError("network timeout")

            with (
                patch.object(subject, "download_with_python", side_effect=python_download),
                patch.object(
                    subject,
                    "download_with_curl",
                    side_effect=RuntimeError("network unreachable"),
                ),
                patch.object(
                    subject,
                    "download_with_powershell",
                    side_effect=RuntimeError("powershell network unreachable"),
                ),
            ):
                with self.assertRaisesRegex(RuntimeError, "refusing insecure fallback"):
                    subject.fetch_url(
                        "https://example.invalid/artifact",
                        destination,
                        allow_insecure=True,
                    )

            self.assertEqual(calls, [True])

    def test_resolve_ca_bundle_prefers_explicit_env(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            bundle = Path(raw) / "custom.pem"
            bundle.write_text("pem", encoding="utf-8")
            resolved = subject.resolve_ca_bundle(
                {"SSL_CERT_FILE": str(bundle), "CURL_CA_BUNDLE": str(Path(raw) / "missing")}
            )
            self.assertEqual(resolved, bundle)

    def test_windows_ca_export_uses_root_and_ca_stores(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            captured: dict[str, str] = {}

            def fake_run(command, **kwargs):
                captured["command"] = " ".join(command)
                Path(kwargs["env"]["RELKIT_CA_OUT"]).write_text(
                    "-----BEGIN CERTIFICATE-----\nABC\n-----END CERTIFICATE-----\n",
                    encoding="utf-8",
                )
                return subject.subprocess.CompletedProcess(command, 0, "", "")

            subject._windows_ca_bundle = None
            with (
                patch.object(subject.shutil, "which", return_value="powershell.exe"),
                patch.object(subject.subprocess, "run", side_effect=fake_run),
                patch.object(subject.tempfile, "gettempdir", return_value=raw),
                patch.object(subject.os, "getpid", return_value=4242),
            ):
                path = subject.ensure_windows_ca_bundle()
            self.assertIsNotNone(path)
            assert path is not None
            self.assertTrue(path.is_file())
            self.assertIn("LocalMachine\\Root", captured["command"])
            self.assertIn("LocalMachine\\CA", captured["command"])
            self.assertIn("CurrentUser\\Root", captured["command"])
            self.assertIn("CurrentUser\\CA", captured["command"])
            self.assertIn("BEGIN CERTIFICATE", path.read_text(encoding="utf-8"))

    def test_fetch_url_summary_omits_curl_help_noise(self) -> None:
        help_noise = (
            "curl: (60) SSL certificate problem\n"
            "Usage: curl [options...] <url>\n"
            "curl --help for more information\n"
        )
        with tempfile.TemporaryDirectory() as raw:
            destination = Path(raw) / "artifact"
            with (
                patch.object(
                    subject,
                    "download_with_python",
                    side_effect=subject.CertificateVerificationError(help_noise),
                ),
                patch.object(
                    subject,
                    "download_with_curl",
                    side_effect=subject.CertificateVerificationError(help_noise),
                ),
                patch.object(
                    subject,
                    "download_with_powershell",
                    side_effect=subject.CertificateVerificationError(help_noise),
                ),
            ):
                with self.assertRaises(RuntimeError) as raised:
                    subject.fetch_url(
                        "https://example.invalid/artifact",
                        destination,
                        allow_insecure=False,
                    )
            message = str(raised.exception)
            self.assertIn("certificate verification failed", message)
            self.assertNotIn("Usage:", message)
            self.assertNotIn("curl --help", message)

    def test_enterprise_ca_bundle_lets_python_succeed_before_insecure(self) -> None:
        """HTTPS inspection / corp CA: explicit PEM makes strict Python succeed."""
        payload = b"corp-proxied artifact"
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            bundle = root / "corp-root.pem"
            bundle.write_text("-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----\n")
            spec = {"url": "https://corp.example/x", "sha256": sha256(payload)}
            seen: dict[str, object] = {}

            def fake_python(url, destination, *, verify, ca_bundle=None):
                seen["verify"] = verify
                seen["ca_bundle"] = ca_bundle
                if not verify:
                    raise AssertionError("insecure path must not run")
                if ca_bundle != bundle:
                    raise subject.CertificateVerificationError("missing corp CA")
                destination.write_bytes(payload)

            with (
                patch.object(subject, "resolve_ca_bundle", return_value=bundle),
                patch.object(subject, "download_with_python", side_effect=fake_python),
                patch.object(
                    subject,
                    "download_with_curl",
                    side_effect=RuntimeError("curl should not win"),
                ),
                patch.object(
                    subject,
                    "download_with_powershell",
                    side_effect=RuntimeError("powershell should not win"),
                ),
            ):
                installed = subject.download_artifact(
                    root, "cli", spec, allow_insecure=False
                )
            self.assertEqual(installed.read_bytes(), payload)
            self.assertEqual(seen["verify"], True)
            self.assertEqual(seen["ca_bundle"], bundle)

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
                patch.object(subject, "resolve_curl", side_effect=RuntimeError("no curl")),
                patch.object(
                    subject,
                    "download_with_powershell",
                    side_effect=RuntimeError("no powershell"),
                ),
                patch.object(subject.time, "sleep"),
            ):
                with self.assertRaisesRegex(RuntimeError, "sha256 mismatch"):
                    subject.download_artifact(root, "cli", spec)
            self.assertFalse(
                (root / ".relkit/cache/artifacts" / ("0" * 64)).exists()
            )

    def test_ensure_uses_system32_when_present(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            system = Path(raw) / "System32" / "curl.exe"
            system.parent.mkdir(parents=True)
            system.write_bytes(b"MZ")
            with (
                patch.object(subject.os, "name", "nt"),
                patch.object(subject, "system32_curl", return_value=system),
                patch.object(subject, "detect_curl_backend", return_value="schannel"),
                patch.object(
                    subject,
                    "install_pinned_windows_schannel_curl",
                    side_effect=AssertionError("must not install"),
                ),
            ):
                chosen = subject.ensure_windows_schannel_curl(root, environ={})
            self.assertEqual(chosen, str(system))

    def test_ensure_installs_pinned_curl_when_system32_missing(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            installed = (
                root
                / ".relkit"
                / "cache"
                / "tools"
                / f"curl-schannel-{subject._WINDOWS_CURL_PIN['version']}"
                / subject._WINDOWS_CURL_PIN["extract_dir"]
                / "bin"
                / "curl.exe"
            )
            installed.parent.mkdir(parents=True)
            installed.write_bytes(b"MZ")
            env: dict[str, str] = {}

            def fake_install(project_root: Path) -> Path:
                self.assertEqual(project_root, root)
                return installed

            with (
                patch.object(subject.os, "name", "nt"),
                patch.object(subject, "system32_curl", return_value=None),
                patch.object(subject, "cached_schannel_curl", return_value=None),
                patch.object(
                    subject, "install_pinned_windows_schannel_curl", side_effect=fake_install
                ),
                patch.object(subject, "detect_curl_backend", return_value="schannel"),
            ):
                chosen = subject.ensure_windows_schannel_curl(root, environ=env)
            self.assertEqual(chosen, str(installed))
            self.assertEqual(env[subject.CURL_OVERRIDE_ENV], str(installed))

    def test_ensure_falls_back_to_powershell_when_install_fails(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            with (
                patch.object(subject.os, "name", "nt"),
                patch.object(subject, "system32_curl", return_value=None),
                patch.object(subject, "cached_schannel_curl", return_value=None),
                patch.object(
                    subject,
                    "install_pinned_windows_schannel_curl",
                    side_effect=RuntimeError("403 Forbidden"),
                ),
            ):
                chosen = subject.ensure_windows_schannel_curl(root, environ={})
            self.assertIsNone(chosen)

    def test_fetch_url_uses_powershell_when_curl_missing(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            destination = Path(raw) / "artifact"
            with (
                patch.object(subject.os, "name", "nt"),
                patch.object(
                    subject, "resolve_curl", side_effect=RuntimeError("System32 missing")
                ),
                patch.object(
                    subject,
                    "download_with_powershell",
                    side_effect=lambda url, path: path.write_bytes(b"via-ps"),
                ),
                patch.object(
                    subject,
                    "download_with_python",
                    side_effect=AssertionError("python should not run"),
                ),
            ):
                subject.fetch_url(
                    "https://github.example/asset.zip",
                    destination,
                    allow_insecure=False,
                )
            self.assertEqual(destination.read_bytes(), b"via-ps")

    def test_install_pinned_curl_checks_sha256(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            archive_bytes = b"not-a-real-archive"

            def fake_ps(url: str, destination: Path) -> None:
                destination.write_bytes(archive_bytes)

            with (
                patch.object(subject.os, "name", "nt"),
                patch.object(subject, "download_with_powershell", side_effect=fake_ps),
            ):
                with self.assertRaisesRegex(RuntimeError, "sha256 mismatch"):
                    subject.install_pinned_windows_schannel_curl(root)

    def test_watched_windows_tree_replaces_files_and_removes_stale(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            destination = root / "bindings"
            staging = root / "staging"
            backup = root / "backup"
            (destination / "old").mkdir(parents=True)
            (destination / "old" / "stale.ts").write_text("stale", encoding="utf-8")
            (destination / "same.js").write_text("old", encoding="utf-8")
            (staging / "dist").mkdir(parents=True)
            (staging / "dist" / "updater.js").write_text("new", encoding="utf-8")
            (staging / "same.js").write_text("replaced", encoding="utf-8")

            subject.replace_watched_tree_on_windows(staging, destination, backup)

            self.assertEqual(
                (destination / "dist" / "updater.js").read_text(encoding="utf-8"),
                "new",
            )
            self.assertEqual(
                (destination / "same.js").read_text(encoding="utf-8"), "replaced"
            )
            self.assertFalse((destination / "old" / "stale.ts").exists())
            self.assertEqual(
                (backup / "same.js").read_text(encoding="utf-8"), "old"
            )


if __name__ == "__main__":
    unittest.main()
