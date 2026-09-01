/**
 * Signed envelope verification (SPEC.md sections 4 and 4.1).
 *
 * This is the security boundary of the whole protocol. Everything downstream
 * (which version is newest, where to download it, what it should hash to) is
 * read out of the payload, so a client that gets this wrong gains nothing from
 * any of the other checks.
 */

import { createPublicKey, verify as cryptoVerify, type KeyObject } from "node:crypto";
import { fromBinary } from "@bufbuild/protobuf";

import { EnvelopeSchema, type Envelope } from "./gen/rup/v2/envelope_pb.js";
import { envelopeSchemaId } from "./models.js";

/**
 * DER prefix for an Ed25519 SubjectPublicKeyInfo.
 *
 * Node's `createPublicKey` will not take the raw 32 bytes that `relkit`
 * publishes, so the key is wrapped here rather than asking every host to carry
 * a base64 SPKI blob it cannot read. The prefix is fixed for Ed25519.
 */
const ED25519_SPKI_PREFIX = Uint8Array.from([
  0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x03, 0x21, 0x00,
]);

function publicKeyFromRaw(keyId: string, raw: Uint8Array): KeyObject {
  if (raw.length !== 32) {
    throw new TypeError(
      `keys[${keyId}]: an Ed25519 public key is 32 bytes, got ${raw.length}`,
    );
  }
  const der = new Uint8Array(ED25519_SPKI_PREFIX.length + raw.length);
  der.set(ED25519_SPKI_PREFIX, 0);
  der.set(raw, ED25519_SPKI_PREFIX.length);
  return createPublicKey({ key: Buffer.from(der), format: "der", type: "spki" });
}

/**
 * An Ed25519 public key the client is willing to trust, by key id.
 *
 * The set is embedded in the client build. That is what makes it meaningful: a
 * signature is only evidence if the verifier decided in advance whose
 * signatures count.
 */
export class TrustedKeys {
  private readonly keys = new Map<string, KeyObject>();

  constructor(keys: Record<string, Uint8Array> | Map<string, Uint8Array>) {
    const entries =
      keys instanceof Map ? [...keys.entries()] : Object.entries(keys);
    for (const [keyId, raw] of entries) {
      this.keys.set(keyId, publicKeyFromRaw(keyId, raw));
    }
  }

  /**
   * Keys given as standard base64, the form they appear in inside
   * `relkit`-generated public key files.
   */
  static fromBase64(
    keys: Record<string, string> | Map<string, string>,
  ): TrustedKeys {
    const entries =
      keys instanceof Map ? [...keys.entries()] : Object.entries(keys);
    const decoded: Record<string, Uint8Array> = {};
    for (const [keyId, value] of entries) {
      decoded[keyId] = new Uint8Array(Buffer.from(value, "base64"));
    }
    return new TrustedKeys(decoded);
  }

  get(keyId: string): KeyObject | undefined {
    return this.keys.get(keyId);
  }

  get keyIds(): string[] {
    return [...this.keys.keys()];
  }

  get isEmpty(): boolean {
    return this.keys.size === 0;
  }
}

/**
 * Anything a host may reasonably hand over as its trusted key set.
 *
 * Accepting the plain object form matters: `{ "my-key": <32 bytes> }` is the
 * obvious way to write this, and rejecting it with a `trusted.get is not a
 * function` crash deep inside verification would blame the wrong line.
 */
export type TrustedKeysInput =
  | TrustedKeys
  | Record<string, Uint8Array | string>
  | Map<string, Uint8Array | string>;

/** Normalises {@link TrustedKeysInput}; strings are read as standard base64. */
export function toTrustedKeys(input: TrustedKeysInput): TrustedKeys {
  if (input instanceof TrustedKeys) return input;
  const entries =
    input instanceof Map ? [...input.entries()] : Object.entries(input);
  const decoded: Record<string, Uint8Array> = {};
  for (const [keyId, value] of entries) {
    decoded[keyId] =
      typeof value === "string"
        ? new Uint8Array(Buffer.from(value, "base64"))
        : value;
  }
  return new TrustedKeys(decoded);
}

/**
 * Why an envelope was refused. The distinctions matter for diagnosis, not for
 * behaviour: every one of these means the document must not be used.
 */
export type EnvelopeRejection =
  | "malformed"
  /** Not `rup.envelope/2`. */
  | "wrongSchema"
  /**
   * An empty signature array. Nothing to check is not the same as nothing
   * wrong, and reading it as "vacuously fine" would disable the whole mechanism
   * with a one-character change to a published file.
   */
  | "noSignatures"
  /**
   * Signatures exist, but none is from a key this client trusts, or none of the
   * trusted ones verified.
   */
  | "notVerified";

export type EnvelopeResult =
  | { accepted: true; payload: Uint8Array; keyId: string }
  | {
      accepted: false;
      rejection: EnvelopeRejection;
      detail: string;
    };

export function describeEnvelopeResult(result: EnvelopeResult): string {
  return result.accepted
    ? `accepted (signed by ${result.keyId})`
    : `${result.rejection}: ${result.detail}`;
}

/**
 * Verifies an envelope and returns the payload bytes it vouches for.
 *
 * Two properties of this function are load-bearing:
 *
 * The signature is checked against the raw decoded payload bytes, never against
 * a re-serialisation of the parsed object. Re-serialising would make
 * verification depend on this language's field ordering and number formatting,
 * so a document signed by `relkit` could fail here for reasons that look like a
 * broken signature.
 *
 * A signature from an unknown key id is not a reason to stop. Key rotation
 * works by signing with the old and new keys at once, so an older client sees
 * an unknown signature first and a usable one second.
 */
export function openEnvelope(
  bytes: Uint8Array,
  trusted: TrustedKeys,
): EnvelopeResult {
  let env: Envelope;
  try {
    env = fromBinary(EnvelopeSchema, bytes);
  } catch (error) {
    return {
      accepted: false,
      rejection: "malformed",
      detail: `not valid protobuf: ${String(error)}`,
    };
  }

  if (env.schema !== envelopeSchemaId) {
    return {
      accepted: false,
      rejection: "wrongSchema",
      detail: `schema is "${env.schema}", expected "${envelopeSchemaId}"`,
    };
  }
  if (env.signatures.length === 0) {
    return {
      accepted: false,
      rejection: "noSignatures",
      detail: "signatures is empty",
    };
  }

  for (const entry of env.signatures) {
    if (entry.alg !== "ed25519") continue;
    if (entry.keyId.length === 0) continue;

    const publicKey = trusted.get(entry.keyId);
    if (publicKey === undefined) continue;
    if (entry.sig.length !== 64) continue;

    let ok = false;
    try {
      ok = cryptoVerify(
        null,
        Buffer.from(env.payload),
        publicKey,
        Buffer.from(entry.sig),
      );
    } catch {
      ok = false;
    }
    if (ok) {
      return {
        accepted: true,
        payload: new Uint8Array(env.payload),
        keyId: entry.keyId,
      };
    }
  }

  return {
    accepted: false,
    rejection: "notVerified",
    detail: "no signature from a trusted key verified",
  };
}
