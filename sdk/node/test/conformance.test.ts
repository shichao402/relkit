/**
 * Runs the shared, language-agnostic fixtures in `conformance/`.
 *
 * These are the same fixtures the Go publisher (`relkit`) is checked against,
 * read from their original location rather than copied here. A copy would drift,
 * and a drifted copy is worse than no copy: the suite would stay green while
 * implementations quietly diverged, which is the exact failure the fixtures
 * exist to prevent.
 *
 * `reachability/` is not run here. It is a publishing-side check (SPEC.md
 * section 10) about whether an index may be released at all, and a client has no
 * use for it.
 */

import assert from "node:assert/strict";
import { createPrivateKey, sign as cryptoSign } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { after, describe, test } from "node:test";

import { create, toBinary } from "@bufbuild/protobuf";

import {
  isMandatory,
  resolveUpgradePath,
  selectNextTarget,
} from "../src/chain.js";
import { openEnvelope, describeEnvelopeResult, TrustedKeys } from "../src/envelope.js";
import {
  ArtifactSchema,
  DigestRefSchema,
  EnvelopeSchema,
  IndexSchema,
  ManifestSchema,
  MetaEntrySchema,
  RupFormatError,
  SelectorSchema,
  SignatureSchema,
  VersionNodeSchema,
  indexSchemaId,
  manifestSchemaId,
  envelopeSchemaId,
  parseIndex,
  type Artifact,
  type Index,
  type Manifest,
  type Selector,
  type Signature,
  type MetaEntry,
} from "../src/models.js";
import { selectArtifact } from "../src/selectors.js";
import { acceptsSequence } from "../src/state.js";

/** Walks up from this file to find `conformance/`. */
function findConformanceDir(): string {
  const override = process.env.RUP_CONFORMANCE_DIR;
  if (override !== undefined && override.length > 0) {
    if (!existsSync(override)) {
      throw new Error(
        `RUP_CONFORMANCE_DIR points at ${override}, which does not exist`,
      );
    }
    return override;
  }

  let dir = dirname(fileURLToPath(import.meta.url));
  for (let depth = 0; depth < 8; depth++) {
    const candidate = join(dir, "conformance");
    if (existsSync(candidate)) return candidate;
    const parent = resolve(dir, "..");
    if (parent === dir) break;
    dir = parent;
  }
  throw new Error("could not find conformance/; set RUP_CONFORMANCE_DIR");
}

const root = findConformanceDir();

function readFixture(relative: string): Record<string, unknown> {
  const path = join(root, ...relative.split("/"));
  return JSON.parse(readFileSync(path, "utf8")) as Record<string, unknown>;
}

const asRecord = (value: unknown): Record<string, unknown> =>
  value as Record<string, unknown>;
const asArray = (value: unknown): unknown[] => value as unknown[];

/**
 * Signed messages must not use `map<>`; encode helpers sort by key before
 * serialising, so the fixture bridge does the same.
 */
function selectorsFromFixture(raw: unknown): Selector[] {
  if (typeof raw !== "object" || raw === null) return [];
  return Object.entries(raw as Record<string, string>)
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([key, value]) => create(SelectorSchema, { key, value }));
}

function metaFromFixture(raw: unknown): MetaEntry[] {
  if (typeof raw !== "object" || raw === null) return [];
  return Object.entries(raw as Record<string, string>)
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([key, value]) => create(MetaEntrySchema, { key, value }));
}

function indexFromFixture(json: Record<string, unknown>): Index {
  const hasMinSupported = Object.hasOwn(json, "minSupported");
  return create(IndexSchema, {
    schema: indexSchemaId,
    product: String(json.product),
    channel: String(json.channel),
    sequence: BigInt(Number(json.sequence)),
    generatedAt: String(json.generatedAt),
    minSupported: hasMinSupported ? BigInt(Number(json.minSupported)) : 0n,
    hasMinSupported,
    expiresAt: typeof json.expiresAt === "string" ? json.expiresAt : "",
    versions: asArray(json.versions).map((raw) => {
      const node = asRecord(raw);
      const manifest = asRecord(node.manifest);
      return create(VersionNodeSchema, {
        version: String(node.version),
        code: BigInt(Number(node.code)),
        minFrom: BigInt(Number(node.minFrom ?? 0)),
        yanked: node.yanked === true,
        manifest: create(DigestRefSchema, {
          sha256: String(manifest.sha256),
          size: BigInt(Number(manifest.size)),
          urls: asArray(manifest.urls).map(String),
        }),
        releasedAt: typeof node.releasedAt === "string" ? node.releasedAt : "",
        notes: typeof node.notes === "string" ? node.notes : "",
        notesUrl: typeof node.notesUrl === "string" ? node.notesUrl : "",
      });
    }),
  });
}

function manifestFromFixture(json: Record<string, unknown>): Manifest {
  return create(ManifestSchema, {
    schema: manifestSchemaId,
    product: String(json.product),
    version: String(json.version),
    code: BigInt(Number(json.code)),
    releasedAt: typeof json.releasedAt === "string" ? json.releasedAt : "",
    notes: typeof json.notes === "string" ? json.notes : "",
    artifacts: asArray(json.artifacts).map((raw) => {
      const artifact = asRecord(raw);
      return create(ArtifactSchema, {
        id: String(artifact.id),
        filename: String(artifact.filename),
        size: BigInt(Number(artifact.size)),
        sha256: String(artifact.sha256),
        kind: typeof artifact.kind === "string" ? artifact.kind : "",
        selectors: selectorsFromFixture(artifact.selectors),
        urls: asArray(artifact.urls).map(String),
        meta: metaFromFixture(artifact.meta),
      }) satisfies Artifact;
    }),
  });
}

interface FixtureKey {
  keyId: string;
  publicKey: Uint8Array;
  seed: Uint8Array;
}

/** DER prefix for an Ed25519 PKCS#8 private key holding a raw 32-byte seed. */
const ED25519_PKCS8_PREFIX = Uint8Array.from([
  0x30, 0x2e, 0x02, 0x01, 0x00, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x04,
  0x22, 0x04, 0x20,
]);

function signWithSeed(seed: Uint8Array, payload: Uint8Array): Uint8Array {
  const der = new Uint8Array(ED25519_PKCS8_PREFIX.length + seed.length);
  der.set(ED25519_PKCS8_PREFIX, 0);
  der.set(seed, ED25519_PKCS8_PREFIX.length);
  const key = createPrivateKey({
    key: Buffer.from(der),
    format: "der",
    type: "pkcs8",
  });
  return new Uint8Array(cryptoSign(null, Buffer.from(payload), key));
}

function signature(
  keyId: string,
  payload: Uint8Array,
  keys: Map<string, FixtureKey>,
  alg = "ed25519",
): Signature {
  const key = keys.get(keyId);
  assert.ok(key !== undefined, `fixture key ${keyId} missing`);
  return create(SignatureSchema, {
    keyId,
    alg,
    sig: signWithSeed(key.seed, payload),
  });
}

/**
 * Rebuilds each envelope case from the fixture's canonical payload.
 *
 * The checked-in `payload` is base64 JSON of a v1 index and its `sig` was
 * produced over those JSON bytes, so it cannot be verified against a v2
 * protobuf payload directly. Dart's runner re-signs the re-encoded protobuf with
 * the same trivial fixture seeds, and this does the same, case for case, so both
 * implementations are asserting identical semantics.
 */
function buildEnvelopeCase(
  testCase: Record<string, unknown>,
  keys: Map<string, FixtureKey>,
  canonicalPayloadJson: Record<string, unknown>,
): Uint8Array {
  const name = String(testCase.name);
  const payloadJson =
    name === "wrong-product" || name === "wrong-channel"
      ? (JSON.parse(
          Buffer.from(
            String(asRecord(testCase.envelope).payload),
            "base64",
          ).toString("utf8"),
        ) as Record<string, unknown>)
      : canonicalPayloadJson;

  let payload = toBinary(IndexSchema, indexFromFixture(payloadJson));
  let schema = envelopeSchemaId;
  const signatures: Signature[] = [];

  switch (name) {
    case "valid-k1":
      signatures.push(signature("k1", payload, keys));
      break;
    case "valid-k2":
      signatures.push(signature("k2", payload, keys));
      break;
    case "unknown-key":
      signatures.push(signature("kx", payload, keys));
      break;
    case "tampered-payload": {
      signatures.push(signature("k1", payload, keys));
      const tampered = Uint8Array.from(payload);
      const last = tampered.length - 1;
      tampered[last] = tampered[last]! ^ 0x01;
      payload = tampered;
      break;
    }
    case "bad-signature": {
      const good = signature("k1", payload, keys);
      const bad = Uint8Array.from(good.sig);
      const last = bad.length - 1;
      bad[last] = bad[last]! ^ 0x01;
      signatures.push(
        create(SignatureSchema, { keyId: good.keyId, alg: good.alg, sig: bad }),
      );
      break;
    }
    case "unsupported-alg":
      signatures.push(signature("k1", payload, keys, "rsa-sha256"));
      break;
    case "cross-payload-replay": {
      const other = indexFromFixture({
        ...payloadJson,
        sequence: Number(payloadJson.sequence) + 1,
      });
      signatures.push(
        signature("k1", toBinary(IndexSchema, other), keys),
      );
      break;
    }
    case "rotation-untrusted-first":
      signatures.push(signature("kx", payload, keys));
      signatures.push(signature("k1", payload, keys));
      break;
    case "rotation-all-untrusted":
      signatures.push(signature("kx", payload, keys));
      signatures.push(signature("ky", payload, keys));
      break;
    case "no-signatures":
      break;
    case "wrong-envelope-schema":
      schema = "rup.envelope/1";
      signatures.push(signature("k1", payload, keys));
      break;
    case "wrong-product":
    case "wrong-channel":
      signatures.push(signature("k1", payload, keys));
      break;
    default:
      throw new Error(`unhandled signature case "${name}"`);
  }

  return toBinary(
    EnvelopeSchema,
    create(EnvelopeSchema, { schema, payload, signatures }),
  );
}

/**
 * Counts every assertion made, so the suite cannot pass by doing nothing.
 *
 * Each fixture drives a loop over `cases`. If a file failed to load, or its case
 * list were empty, every loop would complete without asserting anything and the
 * run would be green. The total below is checked at the end, which turns that
 * silent success into a failure.
 */
let casesChecked = 0;

/** 8 version-select files, 3 selector files, 2 signature files. */
const EXPECTED_CASES = 65;

after(() => {
  assert.equal(
    casesChecked,
    EXPECTED_CASES,
    `expected ${EXPECTED_CASES} fixture cases, ran ${casesChecked}. ` +
      "A fixture was added, removed, or silently failed to load.",
  );
});

describe("version-select (SPEC.md section 9)", () => {
  const files = [
    "flat-chain",
    "min-supported",
    "required-intermediate",
    "single-version",
    "three-hops",
    "unordered",
    "yanked-head",
    "yanked",
  ];

  for (const name of files) {
    test(name, () => {
      const fixture = readFixture(`version-select/${name}.json`);
      const index = indexFromFixture(asRecord(fixture.index));

      for (const raw of asArray(fixture.cases)) {
        const testCase = asRecord(raw);
        casesChecked++;
        const currentCode = Number(testCase.currentCode);
        const where = `currentCode=${currentCode} in ${name}`;

        const target = selectNextTarget(index, currentCode);
        assert.equal(
          target?.version ?? null,
          testCase.expectTarget ?? null,
          `selectNextTarget for ${where}`,
        );

        assert.deepEqual(
          resolveUpgradePath(index, currentCode).map((node) => node.version),
          testCase.expectPath,
          `resolveUpgradePath for ${where}`,
        );

        assert.equal(
          isMandatory(index, currentCode),
          testCase.expectMandatory === true,
          `isMandatory for ${where}`,
        );
      }
    });
  }
});

describe("selector (SPEC.md section 11)", () => {
  const files = ["ambiguous", "os-arch", "target-dimension"];

  for (const name of files) {
    test(name, () => {
      const fixture = readFixture(`selector/${name}.json`);
      const manifest = manifestFromFixture(asRecord(fixture.manifest));

      for (const raw of asArray(fixture.cases)) {
        const testCase = asRecord(raw);
        casesChecked++;
        const selectors = asRecord(testCase.clientSelectors) as Record<
          string,
          string
        >;
        const chosen = selectArtifact(manifest, selectors);
        assert.equal(
          chosen?.id ?? null,
          testCase.expectArtifactId ?? null,
          `selectors ${JSON.stringify(selectors)} in ${name}`,
        );
      }
    });
  }
});

describe("signature (SPEC.md sections 4.1, 12.1, 12.4)", () => {
  test("envelope", () => {
    const keysFixture = readFixture("signature/keys.json");
    const fixture = readFixture("signature/envelope.json");

    const fixtureKeys = new Map<string, FixtureKey>();
    for (const raw of asArray(keysFixture.keys)) {
      const entry = asRecord(raw);
      const keyId = String(entry.keyId);
      fixtureKeys.set(keyId, {
        keyId,
        publicKey: new Uint8Array(
          Buffer.from(String(entry.publicKeyBase64), "base64"),
        ),
        seed: new Uint8Array(
          Buffer.from(String(entry.privateSeedBase64), "base64"),
        ),
      });
    }

    const trustedIds = asArray(fixture.trustedKeys).map(String);
    const trusted = new TrustedKeys(
      Object.fromEntries(
        [...fixtureKeys.values()]
          .filter((key) => trustedIds.includes(key.keyId))
          .map((key) => [key.keyId, key.publicKey]),
      ),
    );

    const expectProduct = String(fixture.expectProduct);
    const expectChannel = String(fixture.expectChannel);

    const validCase = asArray(fixture.cases)
      .map(asRecord)
      .find((entry) => entry.name === "valid-k1");
    assert.ok(validCase !== undefined, "fixture must contain valid-k1");
    const canonicalPayloadJson = JSON.parse(
      Buffer.from(
        String(asRecord(validCase.envelope).payload),
        "base64",
      ).toString("utf8"),
    ) as Record<string, unknown>;

    for (const raw of asArray(fixture.cases)) {
      const testCase = asRecord(raw);
      casesChecked++;
      const name = String(testCase.name);
      const bytes = buildEnvelopeCase(testCase, fixtureKeys, canonicalPayloadJson);

      let accepted = false;
      const result = openEnvelope(bytes, trusted);
      if (result.accepted) {
        try {
          const index = parseIndex(result.payload);
          accepted =
            index.product === expectProduct && index.channel === expectChannel;
        } catch (error) {
          if (!(error instanceof RupFormatError)) throw error;
          accepted = false;
        }
      }

      assert.equal(
        accepted,
        testCase.expectAccepted,
        `case "${name}" (verification said: ${describeEnvelopeResult(result)})`,
      );
    }
  });

  test("anti-rollback", () => {
    const fixture = readFixture("signature/anti-rollback.json");

    for (const raw of asArray(fixture.cases)) {
      const testCase = asRecord(raw);
      casesChecked++;
      const lastSeen =
        testCase.lastSeenSequence === null
          ? null
          : Number(testCase.lastSeenSequence);
      const sequence = Number(testCase.indexSequence);

      assert.equal(
        acceptsSequence(sequence, lastSeen),
        testCase.expectAccepted,
        `lastSeen=${lastSeen} index=${sequence}`,
      );
    }
  });
});
