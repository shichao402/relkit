#!/usr/bin/env python3
"""RUP v1 conformance runner.

These fixtures check the code that actually ships: the runner imports
relkit.chain, relkit.selectors and relkit.envelope from the parent directory
instead of keeping its own copy of the normative logic. A second copy would
drift, and the point of the suite is that the publisher and the client reach
one and the same verdict.

No installation step and no third-party packages: the parent directory is put
on sys.path so that `cd conformance && python run.py` works in a fresh clone.

Output is deliberately ASCII-only so it renders correctly on Windows consoles
regardless of the active code page.

Usage:
    python run.py [fixture_root]

Exit code is non-zero if any fixture fails.
"""

import base64
import json
import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent.parent))

from relkit import chain, envelope, selectors  # noqa: E402  (needs sys.path above)


class Report:
    def __init__(self):
        self.passed = 0
        self.failed = 0
        self.failures = []

    def check(self, fixture, label, expected, actual):
        if expected == actual:
            self.passed += 1
        else:
            self.failed += 1
            self.failures.append(
                "  %s :: %s\n      expected %r\n      actual   %r"
                % (fixture, label, expected, actual)
            )


def _version_name(node):
    return None if node is None else node["version"]


def run_version_select(path, data, report, root):
    index = data["index"]
    for case in data["cases"]:
        code = case["currentCode"]
        label = "currentCode=%s" % code
        report.check(
            path, label + " target",
            case["expectTarget"],
            _version_name(chain.select_next_target(index, code)),
        )
        report.check(
            path, label + " path",
            case["expectPath"],
            [n["version"] for n in chain.resolve_upgrade_path(index, code)],
        )
        if "expectMandatory" in case:
            report.check(
                path, label + " mandatory",
                case["expectMandatory"],
                chain.is_mandatory(index, code),
            )


def run_reachability(path, data, report, root):
    errors, warnings = chain.validate_reachability(data["index"])
    report.check(path, "errors", sorted(data["expectErrors"]), errors)
    report.check(path, "warnings", sorted(data["expectWarnings"]), warnings)
    report.check(path, "valid", data["expectValid"], not errors)


def run_selector(path, data, report, root):
    manifest = data["manifest"]
    for case in data["cases"]:
        client_selectors = case["clientSelectors"]
        chosen = selectors.select_artifact(manifest, client_selectors)
        report.check(
            path,
            "selectors=%s" % json.dumps(client_selectors, sort_keys=True),
            case["expectArtifactId"],
            None if chosen is None else chosen["id"],
        )


def load_trusted_keys(root, trusted_ids):
    with (root / "signature" / "keys.json").open(encoding="utf-8") as handle:
        entries = json.load(handle)["keys"]
    return {
        entry["keyId"]: base64.b64decode(entry["publicKeyBase64"])
        for entry in entries
        if entry["keyId"] in trusted_ids
    }


def run_signature(path, data, report, root):
    """Dispatches on fixture name: the signature suite holds two shapes."""
    if data["name"] == "envelope":
        trusted = load_trusted_keys(root, data["trustedKeys"])
        for case in data["cases"]:
            report.check(
                path, case["name"],
                case["expectAccepted"],
                envelope.accept_envelope(
                    case["envelope"],
                    trusted,
                    data["expectProduct"],
                    data["expectChannel"],
                ),
            )
    elif data["name"] == "anti-rollback":
        for case in data["cases"]:
            label = "lastSeen=%s index=%s" % (
                case["lastSeenSequence"], case["indexSequence"],
            )
            report.check(
                path, label,
                case["expectAccepted"],
                envelope.accept_sequence(
                    case["lastSeenSequence"], case["indexSequence"]
                ),
            )
    else:
        raise ValueError("unknown signature fixture: %s" % data["name"])


SUITES = {
    "version-select": run_version_select,
    "reachability": run_reachability,
    "selector": run_selector,
    "signature": run_signature,
}

# Not fixtures: supporting data consumed by the runner itself.
NON_FIXTURES = {"keys.json"}


def main(argv):
    root = pathlib.Path(argv[1] if len(argv) > 1 else __file__).resolve()
    if root.is_file():
        root = root.parent

    report = Report()
    total_files = 0

    for suite, runner in sorted(SUITES.items()):
        directory = root / suite
        if not directory.is_dir():
            print("SKIP  %s (directory not found)" % suite)
            continue
        for fixture in sorted(directory.glob("*.json")):
            if fixture.name in NON_FIXTURES:
                continue
            total_files += 1
            rel = "%s/%s" % (suite, fixture.name)
            before = report.failed
            with fixture.open(encoding="utf-8") as handle:
                data = json.load(handle)
            runner(rel, data, report, root)
            status = "PASS" if report.failed == before else "FAIL"
            print("%s  %s" % (status, rel))

    print("")
    print("%d files, %d checks passed, %d failed"
          % (total_files, report.passed, report.failed))

    if report.failures:
        print("")
        print("Failures:")
        for failure in report.failures:
            print(failure)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
