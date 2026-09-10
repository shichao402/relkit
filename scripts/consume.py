#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Canonical consumer sync for relkit.

Host projects keep ``scripts/relkit.lock.json`` and a copy of
``scripts/host/relkit_consume.py`` (often still named ``ensure_relkit_sparse.py``).
This script — from the resolved commit — owns sparse paths, root-file
materialization, CLI / ``relkit-updater`` builds, and CNB token injection.
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import tarfile
import time
import traceback
import zipfile
from pathlib import Path
from typing import Any, List, Optional, Sequence
from urllib.request import urlopen

DEFAULT_URL = "https://github.com/shichao402/relkit.git"
DEFAULT_REF = "main"
LOCK_SCHEMA = "relkit.consume/1"
TOOLCHAIN_DIR_NAME = ".toolchain"

# Cone paths: every language SDK + Go sources needed to build cmd/relkit,
# cmd/relkit-updater, and this script. `sdk` is taken whole: SvnMergeTool needs
# `sdk/dart`, Dec imports `sdk` and the frozen `sdk/apply`, and splitting the
# cone per host is what ADR-007 exists to prevent.
SPARSE_CONE_DIRS = (
    "sdk",
    "cmd/relkit",
    "cmd/relkit-updater",
    "internal",
    "api",
    "embed",
    "version",
    "scripts",
    "proto/updater",
)

# Do not rely on cone mode implicitly retaining repository-root files. The
# BlueShield Linux worker accepted the sparse command in build #110 but left
# both files absent, so a later `go build` could not find the module.
REQUIRED_ROOT_FILES = ("go.mod", "go.sum")

# Same worker, SvnMergeTool build #116: the cone command was accepted but the
# working tree came out with `cmd/relkit` and without `internal`/`version`, and
# `go build` could only report every import as an unprovided module. These
# directories have existed for as long as the CLI has, so their absence always
# means an incomplete checkout rather than an old ref.
REQUIRED_CHECKOUT_DIRS = ("cmd/relkit", "internal", "version")

# LFS-tracked (and/or local) CLI artifacts under tools/bin/.
# name → (GOOS, GOARCH, filename)
CLI_TARGETS = {
    "windows-amd64": ("windows", "amd64", "relkit.exe"),
    "linux-amd64": ("linux", "amd64", "relkit-linux-amd64"),
}

# 网络类 git 操作（clone / fetch）的重试次数，每次之间指数退避。
NETWORK_ATTEMPTS = 3

# TLS 握手失败的特征串。macOS 构建机的 /usr/bin/git 链接 LibreSSL 3.3.6，到
# github.com 会被握手拒绝（`tlsv1 alert protocol version`），而同一台机上
# Homebrew 的 git（新 OpenSSL）能连。命中这些特征时重试同一条命令没有意义，
# 必须换 TLS 参数或换 git 二进制。
TLS_FAILURE_MARKERS = (
    "tlsv1 alert",
    "sslv3 alert",
    "ssl routines",
    "libressl",
    "ssl connect error",
    "gnutls",
    "schannel",
)


class Logger:
    def info(self, message: str) -> None:
        print(message, flush=True)

    def warn(self, message: str) -> None:
        print(f"WARN: {message}", file=sys.stderr, flush=True)

    def error(self, message: str) -> None:
        print(f"ERROR: {message}", file=sys.stderr, flush=True)

    def success(self, message: str) -> None:
        self.info(message)

    def failed(self, message: str) -> None:
        self.error(message)

    def command_failure(self, output: str, tail_lines: int = 40, error_lines: int = 20) -> None:
        lines = [line for line in (output or "").splitlines() if line.strip()]
        for line in lines[-tail_lines:]:
            print(line, file=sys.stderr, flush=True)


def prepend_path(directory: Path) -> None:
    os.environ["PATH"] = str(directory) + os.pathsep + os.environ.get("PATH", "")


def run_probe(command: Sequence[str], timeout_seconds: int = 60) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        list(command),
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        timeout=timeout_seconds,
        check=False,
    )


def version_at_least(current: str, required: str) -> bool:
    def parts(raw: str) -> list[int]:
        out: list[int] = []
        for chunk in raw.split("."):
            digits = "".join(ch for ch in chunk if ch.isdigit())
            out.append(int(digits) if digits else 0)
        return out or [0]

    left, right = parts(current), parts(required)
    width = max(len(left), len(right))
    left += [0] * (width - len(left))
    right += [0] * (width - len(right))
    return left >= right


def required_go_version(project_root: Path) -> str:
    dest = relkit_dir(project_root) / "go.mod"
    if dest.is_file():
        for line in dest.read_text(encoding="utf-8").splitlines():
            if line.startswith("go "):
                return line.split()[1]
    return "1.26.3"


def load_lock(path: Path) -> dict[str, Any]:
    data = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise RuntimeError(f"{path} is not a JSON object")
    schema = str(data.get("schema") or "")
    if schema and schema != LOCK_SCHEMA:
        raise RuntimeError(f"{path} schema {schema!r} is not {LOCK_SCHEMA}")
    return data


def lock_ref(lock: dict[str, Any], override: str) -> str:
    if override:
        return override
    commit = str(lock.get("commit") or "").strip()
    if commit:
        return commit
    return str(lock.get("channel") or os.environ.get("RELKIT_REF") or DEFAULT_REF)


def inject_cnb_token(url: str) -> str:
    """Dec / CNB CI: inject CNB_TOKEN into plain https://cnb.cool/ git URLs."""
    token = (os.environ.get("CNB_TOKEN") or "").strip()
    if not token:
        return url
    prefix = "https://cnb.cool/"
    host = url.split("://", 1)[-1].split("/", 1)[0]
    if url.startswith(prefix) and "@" not in host:
        return f"https://cnb:{token}@cnb.cool/" + url[len(prefix) :]
    return url


def lock_urls(lock: dict[str, Any], extra: Sequence[str]) -> list[str]:
    raw: list[str] = []
    value = lock.get("url")
    if isinstance(value, list):
        raw.extend(str(item) for item in value)
    elif value:
        raw.append(str(value))
    raw.extend(extra)
    env = os.environ.get("RELKIT_URL") or ""
    if env:
        raw.append(env)
    return [inject_cnb_token(item) for item in resolve_urls(raw)]


def get_project_root() -> Path:
    return Path(__file__).parent.resolve().parent


def run_capture(
    logger: Logger,
    command: Sequence[str],
    *,
    cwd: Path,
    timeout_seconds: int = 600,
    env: Optional[dict] = None,
) -> tuple[int, str]:
    """执行命令，返回 (exit code, stdout+stderr)；超时按失败处理，不抛异常。"""
    display = subprocess.list2cmdline([str(part) for part in command])
    logger.info(f"$ {display}  (cwd={cwd})")
    try:
        result = subprocess.run(
            list(command),
            cwd=str(cwd),
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            timeout=timeout_seconds,
            env={**os.environ, **(env or {})},
            check=False,
        )
    except subprocess.TimeoutExpired:
        logger.warn(f"命令超时（{timeout_seconds}s）: {display}")
        return 124, f"command timed out after {timeout_seconds}s: {display}"
    if result.stdout:
        for line in result.stdout.splitlines():
            logger.info(line)
    return result.returncode, (result.stdout or "") + "\n" + (result.stderr or "")


def run(
    logger: Logger,
    command: Sequence[str],
    *,
    cwd: Path,
    timeout_seconds: int = 600,
    env: Optional[dict] = None,
) -> None:
    code, output = run_capture(
        logger,
        command,
        cwd=cwd,
        timeout_seconds=timeout_seconds,
        env=env,
    )
    if code != 0:
        logger.command_failure(output, tail_lines=80, error_lines=40)
        display = subprocess.list2cmdline([str(part) for part in command])
        raise RuntimeError(f"command failed ({code}): {display}")


def looks_like_tls_failure(output: str) -> bool:
    lowered = output.lower()
    return any(marker in lowered for marker in TLS_FAILURE_MARKERS)


def git_candidates() -> list[str]:
    """git 二进制候选：`RELKIT_GIT` 覆盖 → PATH 上的 git → Homebrew 的 git。

    macOS 构建机上系统 git 的 TLS 后端可能太旧连不上上游，Homebrew 的 git 链接
    新 OpenSSL 通常还能连，所以把它留作同机降级路径。
    """
    candidates: list[str] = []
    override = (os.environ.get("RELKIT_GIT") or "").strip().strip('"').strip("'")
    if override:
        candidates.append(override)
    candidates.append("git")
    if platform_system() == "darwin":
        for path in ("/opt/homebrew/bin/git", "/usr/local/bin/git"):
            if Path(path).is_file() and path not in candidates:
                candidates.append(path)
    return candidates


def git_network_variants() -> list[tuple[str, list[str]]]:
    """(git 二进制, 额外 -c 配置) 的尝试顺序：先原样，再强制 TLS 1.2。"""
    variants: list[tuple[str, list[str]]] = []
    for git_bin in git_candidates():
        variants.append((git_bin, []))
        variants.append((git_bin, ["-c", "http.sslVersion=tlsv1.2"]))
    return variants


def run_git_network(
    logger: Logger,
    args: Sequence[str],
    *,
    cwd: Path,
    timeout_seconds: int = 600,
) -> None:
    """跑一条会走网络的 git 命令：可重试，且能换 TLS 参数 / git 二进制降级。"""
    errors: list[str] = []
    for git_bin, config in git_network_variants():
        command = [git_bin, *config, *args]
        tls_failure = False
        for attempt in range(1, NETWORK_ATTEMPTS + 1):
            code, output = run_capture(
                logger,
                command,
                cwd=cwd,
                timeout_seconds=timeout_seconds,
            )
            if code == 0:
                return
            display = subprocess.list2cmdline(command)
            errors.append(f"{display} -> exit {code}")
            # 每次失败都要留下原始报错：换源/换 TLS 参数的决策依据全在这里，
            # 只报 exit code 等于把 LibreSSL 之类的真因丢掉。
            logger.command_failure(output, tail_lines=40, error_lines=20)
            if looks_like_tls_failure(output):
                tls_failure = True
                logger.warn("git TLS 握手失败，换 TLS 参数 / git 二进制再试")
                break
            if attempt < NETWORK_ATTEMPTS:
                delay = 2**attempt
                logger.warn(
                    f"git 网络操作失败，{delay}s 后重试（{attempt}/{NETWORK_ATTEMPTS}）"
                )
                time.sleep(delay)
        if not tls_failure:
            # DNS / 连接 / 鉴权类失败换 TLS 参数或 git 二进制也救不回来，
            # 早点让上层去试下一个源。
            break
    raise RuntimeError("git 网络操作失败，已尝试:\n  - " + "\n  - ".join(errors))


def go_archive_name(version: str) -> str:
    system = platform_system()
    machine = os.environ.get("PROCESSOR_ARCHITECTURE", "").lower()
    uname = (os.uname().machine.lower() if hasattr(os, "uname") else machine)
    if system == "windows":
        return f"go{version}.windows-amd64.zip"
    if system == "darwin":
        arch = "arm64" if uname in ("arm64", "aarch64") else "amd64"
        return f"go{version}.darwin-{arch}.tar.gz"
    if system == "linux":
        arch = "arm64" if uname in ("arm64", "aarch64") else "amd64"
        return f"go{version}.linux-{arch}.tar.gz"
    raise RuntimeError(f"unsupported OS for Go install: {system}")


def platform_system() -> str:
    import platform as py_platform

    return py_platform.system().lower()


def probe_go_version() -> Optional[str]:
    probe = run_probe(["go", "version"], timeout_seconds=60)
    if probe.returncode != 0:
        return None
    # `go version go1.26.3 windows/amd64`
    parts = (probe.stdout or "").strip().split()
    for part in parts:
        if part.startswith("go") and len(part) > 2 and part[2].isdigit():
            return part[2:]
    return None


def ensure_go(logger: Logger, project_root: Path) -> str:
    """Return path to `go` binary meeting toolchain.json requirement."""
    required = required_go_version(project_root)
    current = probe_go_version()
    if current and version_at_least(current, required):
        which = shutil.which("go")
        logger.info(f"Go 已就绪: {current} ({which})")
        return which or "go"

    toolchain_root = project_root / TOOLCHAIN_DIR_NAME
    go_root = toolchain_root / f"go-{required}"
    go_bin = go_root / "go" / "bin" / ("go.exe" if os.name == "nt" else "go")
    if go_bin.is_file():
        prepend_path(go_bin.parent)
        os.environ["GOROOT"] = str(go_root / "go")
        logger.info(f"复用本地 Go 工具链: {go_bin}")
        return str(go_bin)

    logger.warn(f"缺少 Go >= {required}，下载便携工具链到 {go_root}")
    archive = go_archive_name(required)
    url = f"https://go.dev/dl/{archive}"
    toolchain_root.mkdir(parents=True, exist_ok=True)
    if go_root.exists():
        shutil.rmtree(go_root)
    go_root.mkdir(parents=True)

    archive_path = go_root / archive
    logger.info(f"下载 {url}")
    with urlopen(url, timeout=120) as response, open(archive_path, "wb") as handle:
        shutil.copyfileobj(response, handle)

    if archive.endswith(".zip"):
        with zipfile.ZipFile(archive_path, "r") as zf:
            zf.extractall(go_root)
    else:
        with tarfile.open(archive_path, "r:gz") as tf:
            tf.extractall(go_root)
    archive_path.unlink(missing_ok=True)

    if not go_bin.is_file():
        raise RuntimeError(f"Go 解压后未找到 {go_bin}")
    prepend_path(go_bin.parent)
    os.environ["GOROOT"] = str(go_root / "go")
    logger.info(f"Go {required} 已安装: {go_bin}")
    return str(go_bin)


def relkit_dir(project_root: Path) -> Path:
    return project_root / "third_party" / "relkit"


def dart_sdk_dir(project_root: Path) -> Path:
    return relkit_dir(project_root) / "sdk" / "dart"


def cli_output_path(project_root: Path, target: str = "host") -> Path:
    bin_dir = project_root / "tools" / "bin"
    if target == "host":
        system = platform_system()
        if system == "windows":
            return bin_dir / "relkit.exe"
        if system == "linux":
            return bin_dir / "relkit-linux-amd64"
        return bin_dir / "relkit"
    if target not in CLI_TARGETS:
        raise RuntimeError(
            f"未知 --target={target!r}；可选: host, {', '.join(CLI_TARGETS)}"
        )
    return bin_dir / CLI_TARGETS[target][2]


def resolve_build_targets(raw: Sequence[str]) -> list[str]:
    if not raw:
        return ["host"]
    out: list[str] = []
    for item in raw:
        for part in item.split(","):
            name = part.strip()
            if not name:
                continue
            if name == "host" or name in CLI_TARGETS:
                if name not in out:
                    out.append(name)
                continue
            raise RuntimeError(
                f"未知 --target={name!r}；可选: host, {', '.join(CLI_TARGETS)}"
            )
    return out


def build_cli(
    logger: Logger,
    project_root: Path,
    go_bin: str,
    targets: Sequence[str],
) -> list[Path]:
    dest = relkit_dir(project_root)
    built: list[Path] = []
    for target in targets:
        if target == "host":
            system = platform_system()
            machine = (
                os.uname().machine.lower()
                if hasattr(os, "uname")
                else os.environ.get("PROCESSOR_ARCHITECTURE", "amd64").lower()
            )
            goos = {
                "windows": "windows",
                "linux": "linux",
                "darwin": "darwin",
            }.get(system)
            if goos is None:
                raise RuntimeError(f"不支持为本机 OS 构建 relkit: {system}")
            goarch = "arm64" if machine in ("arm64", "aarch64") else "amd64"
            # Prefer LFS 标准文件名 when host matches a tracked target.
            if goos == "windows" and goarch == "amd64":
                out = project_root / "tools" / "bin" / "relkit.exe"
            elif goos == "linux" and goarch == "amd64":
                out = project_root / "tools" / "bin" / "relkit-linux-amd64"
            else:
                out = project_root / "tools" / "bin" / "relkit"
        else:
            goos, goarch, filename = CLI_TARGETS[target]
            out = project_root / "tools" / "bin" / filename

        out.parent.mkdir(parents=True, exist_ok=True)
        env = {
            "CGO_ENABLED": "0",
            "GOOS": goos,
            "GOARCH": goarch,
        }
        logger.info(f"go build ({goos}/{goarch}) → {out}")
        run(
            logger,
            [
                go_bin,
                "build",
                "-trimpath",
                "-ldflags",
                "-s -w",
                "-o",
                str(out),
                "./cmd/relkit",
            ],
            cwd=dest,
            timeout_seconds=600,
            env=env,
        )
        if not out.is_file():
            raise RuntimeError(f"go build 未产出 {out}")
        if goos != "windows":
            out.chmod(out.stat().st_mode | 0o111)
        logger.info(f"relkit CLI 就绪: {out} ({out.stat().st_size} bytes)")
        built.append(out)

        updater_name = "relkit-updater.exe" if goos == "windows" else "relkit-updater"
        updater_out = project_root / "tools" / "bin" / updater_name
        logger.info(f"go build relkit-updater ({goos}/{goarch}) → {updater_out}")
        run(
            logger,
            [
                go_bin,
                "build",
                "-trimpath",
                "-ldflags",
                "-s -w",
                "-o",
                str(updater_out),
                "./cmd/relkit-updater",
            ],
            cwd=dest,
            timeout_seconds=600,
            env=env,
        )
        if updater_out.is_file() and goos != "windows":
            updater_out.chmod(updater_out.stat().st_mode | 0o111)
        if updater_out.is_file():
            logger.info(
                f"relkit-updater 就绪: {updater_out} ({updater_out.stat().st_size} bytes)"
            )
            built.append(updater_out)
    return built


def sync_from_url(
    logger: Logger,
    project_root: Path,
    dest: Path,
    *,
    url: str,
    ref: str,
) -> None:
    """从单个 URL 把 relkit 检出/更新到 [dest]，并把 ref 落到工作树。"""
    if dest.exists() and not (dest / ".git").exists():
        # leftover non-git directory (e.g. aborted run / 上一个 URL 失败的残留)
        shutil.rmtree(dest)

    if not (dest / ".git").exists():
        logger.info(f"sparse clone {url} → {dest}")
        clone_cmd = ["clone", "--filter=blob:none", "--sparse"]
        # Branch/tag can be checked out during clone; commits need a follow-up fetch.
        if not _looks_like_commit(ref):
            clone_cmd += ["--branch", ref]
        clone_cmd += [url, str(dest)]
        run_git_network(logger, clone_cmd, cwd=project_root, timeout_seconds=600)
        run(
            logger,
            ["git", "sparse-checkout", "set", "--cone", *SPARSE_CONE_DIRS],
            cwd=dest,
        )
    else:
        logger.info(f"更新既有 sparse 仓库: {dest} ← {url}")
        run(logger, ["git", "remote", "set-url", "origin", url], cwd=dest)
        run(
            logger,
            ["git", "sparse-checkout", "set", "--cone", *SPARSE_CONE_DIRS],
            cwd=dest,
        )

    # Resolve branch/tag/commit after (re)fetch — works for all ref kinds.
    run_git_network(
        logger,
        ["fetch", "--filter=blob:none", "--tags", "origin", ref],
        cwd=dest,
        timeout_seconds=600,
    )
    run(logger, ["git", "checkout", "--force", "FETCH_HEAD"], cwd=dest)
    materialize_required_root_files(logger, dest)
    materialize_required_dirs(logger, dest)


def materialize_required_root_files(logger: Logger, dest: Path) -> None:
    """Materialize build-critical root files directly from the checked-out HEAD.

    Cone-mode sparse checkout normally leaves root files present, but that is
    not a portable enough contract for a publishing path. `git show` works
    independently of sparse patterns and also asks a partial clone's promisor
    remote for a missing blob. Write atomically so an interrupted repair cannot
    leave a truncated module file behind.
    """
    missing = [name for name in REQUIRED_ROOT_FILES if not (dest / name).is_file()]
    if missing:
        logger.warn(
            "sparse checkout omitted required root files; restoring from HEAD: "
            + ", ".join(missing)
        )
    for name in missing:
        try:
            data = subprocess.check_output(
                ["git", "show", f"HEAD:{name}"],
                cwd=str(dest),
            )
        except subprocess.CalledProcessError as error:
            raise RuntimeError(
                f"cannot restore required root file {name} from relkit HEAD"
            ) from error
        if not data:
            raise RuntimeError(f"relkit HEAD contains an empty required file: {name}")
        target = dest / name
        temporary = target.with_name(f".{name}.relkit-tmp")
        temporary.write_bytes(data)
        os.replace(temporary, target)

    absent = [name for name in REQUIRED_ROOT_FILES if not (dest / name).is_file()]
    if absent:
        raise RuntimeError(
            "sparse checkout is incomplete; missing required root files: "
            + ", ".join(absent)
        )


def missing_tracked_paths(dest: Path, relative: str) -> list[str]:
    """Tracked paths under [relative] that HEAD has but the working tree lacks."""
    out = subprocess.check_output(
        ["git", "ls-tree", "-r", "-z", "--name-only", "HEAD", "--", relative],
        cwd=str(dest),
    )
    names = [name.decode("utf-8", "replace") for name in out.split(b"\0") if name]
    return [name for name in names if not (dest / name).exists()]


def materialize_required_dirs(logger: Logger, dest: Path) -> None:
    """Give up on the cone rather than build from a half-checked-out tree.

    Sparseness is a download optimization; a build-complete tree is the
    contract. When a worker leaves build-critical files out, drop the cone
    entirely and check the same commit out again. Compare against HEAD instead
    of testing for directories: an unexpanded sparse directory looks present
    and still fails the build.
    """
    missing: list[str] = []
    for relative in REQUIRED_CHECKOUT_DIRS:
        missing += missing_tracked_paths(dest, relative)
    if not missing:
        return

    logger.warn(
        f"sparse checkout omitted {len(missing)} build-critical files, "
        "e.g. " + ", ".join(sorted(missing)[:5])
    )
    for label, command in (
        ("git", ["git", "--version"]),
        ("active cone", ["git", "sparse-checkout", "list"]),
    ):
        try:
            out = subprocess.check_output(
                command, cwd=str(dest), text=True, errors="replace"
            )
        except (subprocess.CalledProcessError, OSError):
            out = "<command failed>"
        logger.warn(f"{label}: " + " ".join(out.split()))

    logger.warn("dropping the cone and checking the commit out in full")
    run(logger, ["git", "sparse-checkout", "disable"], cwd=dest)
    run(logger, ["git", "checkout", "--force", "HEAD"], cwd=dest)

    absent: list[str] = []
    for relative in REQUIRED_CHECKOUT_DIRS:
        absent += missing_tracked_paths(dest, relative)
    if absent:
        raise RuntimeError(
            "relkit checkout is incomplete even without a sparse cone; "
            f"{len(absent)} files missing, e.g. " + ", ".join(sorted(absent)[:5])
        )


def ensure_sparse_clone(
    logger: Logger,
    project_root: Path,
    *,
    urls: Sequence[str],
    ref: str,
    allow_stale: bool = False,
) -> Path:
    """把 relkit monorepo 稀疏检出到 third_party/relkit。

    [urls] 按顺序尝试（内网镜像优先、GitHub 垫底）：构建机到 github.com 未必通，
    典型症状是系统 git 的 TLS 后端太旧（`LibreSSL ... tlsv1 alert protocol
    version`），换源比在同一个源上干等更有用。

    [allow_stale] 为 True 时，**已有本地检出**的情况下所有源都拉不动只警告并沿用
    当前 HEAD（末尾的 Dart SDK 完整性检查仍然会拦住残缺的检出）。这一档是给本地
    构建用的：没道理因为拉不到 SDK 更新就不能构建本地已经能跑的代码。CI 与发布
    路径不传本参数，保持"必须是最新 SDK"。
    """
    dest = relkit_dir(project_root)
    dest.parent.mkdir(parents=True, exist_ok=True)
    # 只有"本来就有检出"才允许降级；首次 clone 失败时目录是空的，没有可沿用的东西。
    can_reuse_existing = (dest / ".git").exists()

    errors: list[str] = []
    for url in urls:
        try:
            sync_from_url(logger, project_root, dest, url=url, ref=ref)
            break
        except RuntimeError as error:
            errors.append(f"{url} -> {error}")
            logger.warn(f"从 {url} 同步 relkit 失败: {error}")
    else:
        detail = "已尝试:\n  - " + "\n  - ".join(errors)
        if not (allow_stale and can_reuse_existing):
            raise RuntimeError(f"无法同步 relkit 仓库。{detail}")
        logger.warn(f"拉取 relkit 失败，沿用本地已有检出。{detail}")
        logger.warn("如果需要最新 SDK，请恢复到上游的网络后重跑本脚本")

    head = subprocess.check_output(
        ["git", "rev-parse", "HEAD"],
        cwd=str(dest),
        text=True,
        encoding="utf-8",
    ).strip()
    logger.info(f"relkit HEAD = {head} (requested {ref})")

    pubspec = dart_sdk_dir(project_root) / "pubspec.yaml"
    if not pubspec.is_file():
        raise RuntimeError(f"sparse 后缺少 Dart SDK: {pubspec}")
    return dest


def _looks_like_commit(ref: str) -> bool:
    if len(ref) < 7:
        return False
    return all(ch in "0123456789abcdef" for ch in ref.lower())


def split_url_list(raw: str) -> list[str]:
    """把 `a, b; c` 这类写法拆成 URL 列表，顺带去掉引号。"""
    text = (raw or "").strip()
    if not text:
        return []
    for separator in ",;":
        text = text.replace(separator, " ")
    return [
        item
        for item in (chunk.strip().strip('"').strip("'") for chunk in text.split())
        if item
    ]


def resolve_urls(raw_urls: Sequence[str]) -> list[str]:
    """候选源顺序：调用方给的（镜像优先），GitHub 垫底。"""
    urls: list[str] = []
    for item in raw_urls:
        for url in split_url_list(item):
            if url not in urls:
                urls.append(url)
    if DEFAULT_URL not in urls:
        urls.append(DEFAULT_URL)
    return urls


def create_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Sparse-checkout relkit monorepo (SDK + CLI sources)",
    )
    parser.add_argument(
        "--lock",
        help="relkit.consume/1 lock JSON (host project)",
    )
    parser.add_argument(
        "--project-root",
        help="host project root (default: cwd)",
    )
    parser.add_argument(
        "--resolved-out",
        help="write the resolved SHA and protocol window as JSON",
    )
    parser.add_argument(
        "--url",
        action="append",
        default=[],
        metavar="URL",
        help=(
            "relkit git URL，可重复或用逗号分隔；按顺序尝试，GitHub 垫底。"
            f"默认读 env RELKIT_URL，再退回 {DEFAULT_URL}"
        ),
    )
    parser.add_argument(
        "--ref",
        default="",
        help=f"branch/tag/commit override (default: lock commit/channel or {DEFAULT_REF})",
    )
    parser.add_argument(
        "--sdk-only",
        action="store_true",
        help="只同步 Dart SDK / Go 源码，不构建 CLI",
    )
    parser.add_argument(
        "--allow-stale",
        action="store_true",
        help=(
            "已有本地检出时，拉取失败只警告并沿用当前 HEAD（本地构建用；"
            "CI / 发布路径不要传，保证 SDK 是最新的）"
        ),
    )
    parser.add_argument(
        "--build-cli",
        action="store_true",
        help="强制构建 CLI（覆盖 --sdk-only）",
    )
    parser.add_argument(
        "--build-cli-if-missing",
        action="store_true",
        help="只有本机缺少 relkit 二进制时才构建 CLI",
    )
    parser.add_argument(
        "--skip-update",
        action="store_true",
        help="不 fetch/checkout，沿用 third_party/relkit 当前树（含未提交改动）只构建 CLI",
    )
    parser.add_argument(
        "--target",
        action="append",
        default=[],
        metavar="NAME",
        help=(
            "构建目标，可重复或逗号分隔："
            f"host, {', '.join(CLI_TARGETS)}。"
            "默认 host；检入 LFS 时常用 --target windows-amd64 --target linux-amd64"
        ),
    )
    return parser


def should_build_cli(args: argparse.Namespace, project_root: Path) -> bool:
    if args.build_cli or args.target:
        return True
    if args.sdk_only:
        return False
    if args.build_cli_if_missing:
        return not cli_output_path(project_root).is_file()
    # Default for standalone runs: build CLI so publish scripts work.
    return True


def main(argv: Optional[List[str]] = None) -> int:
    logger = Logger()
    try:
        args = create_parser().parse_args(argv)
        project_root = Path(args.project_root).resolve() if args.project_root else Path.cwd()
        lock: dict[str, Any] = {}
        lock_path = Path(args.lock) if args.lock else project_root / "scripts" / "relkit.lock.json"
        if lock_path.is_file():
            lock = load_lock(lock_path)
        ref = lock_ref(lock, args.ref or os.environ.get("RELKIT_REF", ""))
        urls = lock_urls(lock, args.url or [])
        protocol = lock.get("protocol") if isinstance(lock.get("protocol"), dict) else {}
        updater_ipc = (
            lock.get("updaterIpc") if isinstance(lock.get("updaterIpc"), dict) else {}
        )
        logger.info("========== relkit consume 开始 ==========")
        logger.info(f"urls={' '.join(urls)} ref={ref}")

        if args.skip_update:
            dest = relkit_dir(project_root)
            pubspec = dart_sdk_dir(project_root) / "pubspec.yaml"
            if not pubspec.is_file():
                raise RuntimeError(
                    f"--skip-update 需要已有 Dart SDK: {pubspec}"
                )
            logger.info(f"跳过拉取，沿用本地树: {dest}")
        else:
            dest = ensure_sparse_clone(
                logger,
                project_root,
                urls=urls,
                ref=ref,
                allow_stale=args.allow_stale,
            )

        head = subprocess.check_output(
            ["git", "rev-parse", "HEAD"],
            cwd=str(relkit_dir(project_root)),
            text=True,
            encoding="utf-8",
        ).strip()
        if _looks_like_commit(ref) and not head.startswith(ref) and not ref.startswith(head):
            raise RuntimeError(f"relkit HEAD {head} does not match requested {ref}")
        resolved = {
            "schema": LOCK_SCHEMA,
            "sha": head,
            "ref": ref,
            "minProtocol": int(protocol.get("min") or 2),
            "maxProtocol": int(protocol.get("max") or 2),
            "updaterIpcMin": int(updater_ipc.get("min") or 1),
            "updaterIpcMax": int(updater_ipc.get("max") or 1),
        }
        logger.info(f"resolved SHA={head}")
        if args.resolved_out:
            out = Path(args.resolved_out)
            out.parent.mkdir(parents=True, exist_ok=True)
            out.write_text(json.dumps(resolved, indent=2) + "\n", encoding="utf-8")

        if should_build_cli(args, project_root):
            targets = resolve_build_targets(args.target)
            logger.info(f"CLI targets: {', '.join(targets)}")
            go_bin = ensure_go(logger, project_root)
            build_cli(logger, project_root, go_bin, targets)
        else:
            logger.info("跳过 CLI 构建（--sdk-only 或二进制已存在）")

        logger.info(f"Dart SDK path: {dart_sdk_dir(project_root)}")
        logger.success("relkit sparse 就绪")
        return 0
    except Exception as error:
        logger.error(f"relkit consume 失败: {error}")
        logger.error(traceback.format_exc())
        logger.failed(str(error))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
