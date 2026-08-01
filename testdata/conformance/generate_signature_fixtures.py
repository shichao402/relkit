#!/usr/bin/env python3
"""Regenerate conformance/signature/*.json deterministically.

Ed25519 signatures are deterministic, and the seeds below are hard-coded, so
running this script twice produces byte-identical output. That is why the
fixtures can live in version control and be diffed meaningfully.

The fixtures were cross-checked against Node's native `crypto.sign('ed25519')`:
identical public keys, byte-identical signatures. See relkit/_ed25519.py.

Usage:
    python generate_signature_fixtures.py
"""

import base64
import json
import pathlib
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent.parent))

from relkit import _ed25519 as ed  # noqa: E402  (needs sys.path above)


# Deterministic test seeds. Trivial on purpose: these must never be mistaken
# for production keys.
SEEDS = {
    "k1": bytes([1]) * 32,
    "k2": bytes([2]) * 32,
    "kx": bytes([9]) * 32,
    "ky": bytes([8]) * 32,
}

TRUSTED = ["k1", "k2"]

OUT_DIR = pathlib.Path(__file__).resolve().parent / "signature"


def b64(data):
    return base64.b64encode(data).decode("ascii")


def canonical(obj):
    return json.dumps(
        obj, separators=(",", ":"), sort_keys=True, ensure_ascii=False
    ).encode("utf-8")


def make_index(sequence):
    return {
        "schema": "rup.index/1",
        "product": "conformance",
        "channel": "stable",
        "sequence": sequence,
        "generatedAt": "2026-07-30T00:00:00Z",
        "versions": [
            {
                "version": "1.5.0",
                "code": 150,
                "minFrom": 0,
                "manifest": {
                    "sha256": "3" * 64,
                    "size": 1024,
                    "urls": ["https://x.invalid/1.5.0.json"],
                },
            }
        ],
    }


def envelope(payload, signers, alg="ed25519"):
    return {
        "schema": "rup.envelope/1",
        "payload": b64(payload),
        "signatures": [
            {
                "keyId": key_id,
                "alg": alg,
                "sig": b64(ed.sign(SEEDS[key_id], payload)),
            }
            for key_id in signers
        ],
    }


def flip_last_byte(data):
    return data[:-1] + bytes([data[-1] ^ 0x01])


def write_keys():
    payload = {
        "warning": "TEST KEYS ONLY. Trivial hard-coded seeds. Never use for a real release.",
        "trustedKeys": TRUSTED,
        "note": "A conforming client embeds only the public keys. privateSeedBase64 is included so implementers can regenerate or extend these fixtures.",
        "keys": [
            {
                "keyId": key_id,
                "alg": "ed25519",
                "publicKeyBase64": b64(ed.public_key(seed)),
                "privateSeedBase64": b64(seed),
                "trusted": key_id in TRUSTED,
            }
            for key_id, seed in SEEDS.items()
        ],
    }
    write_json("keys.json", payload)


def write_envelope_cases():
    payload = canonical(make_index(42))
    other_payload = canonical(make_index(43))

    valid_k1 = envelope(payload, ["k1"])

    tampered_payload = dict(valid_k1)
    tampered_payload["payload"] = b64(flip_last_byte(payload))

    bad_signature = json.loads(json.dumps(valid_k1))
    bad_signature["signatures"][0]["sig"] = b64(
        flip_last_byte(base64.b64decode(valid_k1["signatures"][0]["sig"]))
    )

    cross_payload = json.loads(json.dumps(envelope(other_payload, ["k1"])))
    cross_payload["payload"] = b64(payload)

    wrong_product = canonical(
        dict(make_index(42), product="somethingelse")
    )
    wrong_channel = canonical(dict(make_index(42), channel="beta"))

    cases = [
        {
            "name": "valid-k1",
            "envelope": valid_k1,
            "expectAccepted": True,
        },
        {
            "name": "valid-k2",
            "envelope": envelope(payload, ["k2"]),
            "expectAccepted": True,
            "why": "Any trusted key is sufficient; k2 exists so rotation can be tested.",
        },
        {
            "name": "unknown-key",
            "envelope": envelope(payload, ["kx"]),
            "expectAccepted": False,
            "why": "Signature is cryptographically valid but the keyId is not trusted. Rejecting an untrusted signer is the whole point of pinning keys.",
        },
        {
            "name": "tampered-payload",
            "envelope": tampered_payload,
            "expectAccepted": False,
            "why": "Last payload byte flipped. Signature no longer matches.",
        },
        {
            "name": "bad-signature",
            "envelope": bad_signature,
            "expectAccepted": False,
            "why": "Last signature byte flipped.",
        },
        {
            "name": "unsupported-alg",
            "envelope": envelope(payload, ["k1"], alg="rsa-sha256"),
            "expectAccepted": False,
            "why": "v1 permits only ed25519. An unknown alg must not be treated as trusted, and must not fall back to skipping verification.",
        },
        {
            "name": "cross-payload-replay",
            "envelope": cross_payload,
            "expectAccepted": False,
            "why": "A genuine k1 signature over a different index, pasted onto this payload. Catches implementations that verify the signature against a re-serialized object instead of the payload bytes.",
        },
        {
            "name": "rotation-untrusted-first",
            "envelope": envelope(payload, ["kx", "k1"]),
            "expectAccepted": True,
            "why": "During rotation the publisher signs with several keys. A client that only knows k1 must still accept, so it must not stop at the first unusable signature.",
        },
        {
            "name": "rotation-all-untrusted",
            "envelope": envelope(payload, ["kx", "ky"]),
            "expectAccepted": False,
            "why": "Several signatures, none from a trusted key.",
        },
        {
            "name": "no-signatures",
            "envelope": {
                "schema": "rup.envelope/1",
                "payload": b64(payload),
                "signatures": [],
            },
            "expectAccepted": False,
            "why": "An empty signature list must never be read as 'nothing to check, therefore fine'.",
        },
        {
            "name": "wrong-envelope-schema",
            "envelope": dict(valid_k1, schema="rup.envelope/2"),
            "expectAccepted": False,
            "why": "Unrecognized major version must be rejected outright, never parsed speculatively (SPEC.md 15).",
        },
        {
            "name": "wrong-product",
            "envelope": envelope(wrong_product, ["k1"]),
            "expectAccepted": False,
            "why": "Correctly signed by the publisher, but for a different product. Prevents installing product A's update into product B.",
        },
        {
            "name": "wrong-channel",
            "envelope": envelope(wrong_channel, ["k1"]),
            "expectAccepted": False,
            "why": "Correctly signed, but a beta index served to a stable client.",
        },
    ]

    write_json(
        "envelope.json",
        {
            "name": "envelope",
            "description": "Envelope verification per SPEC.md 4.1, plus the product and channel guards from SPEC.md 12.1 steps 1-3. Public keys are in keys.json; trusted keyIds are k1 and k2.",
            "trustedKeys": TRUSTED,
            "expectProduct": "conformance",
            "expectChannel": "stable",
            "cases": cases,
        },
    )


def write_anti_rollback():
    write_json(
        "anti-rollback.json",
        {
            "name": "anti-rollback",
            "description": "Sequence monotonicity per SPEC.md 12.4. A valid signature only proves the publisher issued this index at some point, not that it is the newest one; without this check an attacker can replay an older correctly-signed index to steer clients onto a version with known vulnerabilities.",
            "cases": [
                {
                    "lastSeenSequence": None,
                    "indexSequence": 1,
                    "expectAccepted": True,
                    "why": "First run, nothing recorded yet.",
                },
                {
                    "lastSeenSequence": 42,
                    "indexSequence": 43,
                    "expectAccepted": True,
                },
                {
                    "lastSeenSequence": 42,
                    "indexSequence": 42,
                    "expectAccepted": True,
                    "why": "Equal is fine: this is the normal case of fetching the same index again.",
                },
                {
                    "lastSeenSequence": 42,
                    "indexSequence": 41,
                    "expectAccepted": False,
                    "why": "Replay of an older index. Must be rejected, and must NOT be surfaced as an error: mirrors legitimately lag behind each other, so the client tries the next source and otherwise reports 'no update'.",
                },
                {
                    "lastSeenSequence": 42,
                    "indexSequence": 1,
                    "expectAccepted": False,
                },
            ],
        },
    )


def write_json(name, payload):
    path = OUT_DIR / name
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="\n") as handle:
        json.dump(payload, handle, indent=2, ensure_ascii=False)
        handle.write("\n")
    print("wrote %s" % path.relative_to(OUT_DIR.parent.parent))


if __name__ == "__main__":
    write_keys()
    write_envelope_cases()
    write_anti_rollback()
