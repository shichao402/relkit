/**
 * Object model for the protobuf RUP documents a client reads (SPEC.md sections
 * 4-6).
 *
 * The generated types use `bigint` for proto int64. Every accessor here narrows
 * to `number` at the edge, because the protocol's integers are codes, sizes and
 * sequences, all of which are compared and arithmetic'd throughout the SDK.
 * Mixing `bigint` and `number` in those comparisons is a TypeError at runtime,
 * not a type error at the call site, so the conversion is done once, here.
 */

import { fromBinary } from "@bufbuild/protobuf";

import {
  ArtifactSchema,
  FallbackSchema,
  IndexSchema,
  ManifestSchema,
  UpdateDirectorySchema,
  type Artifact,
  type DigestRef,
  type Fallback,
  type Index,
  type Manifest,
  type MetaEntry,
  type Selector,
  type UpdateDirectory,
  type VersionNode,
} from "./gen/rup/v2/objects_pb.js";

export {
  ArtifactSchema,
  DigestRefSchema,
  DirectoryServiceSchema,
  FallbackRuleSchema,
  FallbackSchema,
  IndexSchema,
  ManifestSchema,
  MetaEntrySchema,
  SelectorSchema,
  StagedArtifactSchema,
  StagedSchema,
  UpdateDirectorySchema,
  VersionNodeSchema,
} from "./gen/rup/v2/objects_pb.js";
export type {
  Artifact,
  DigestRef,
  DirectoryService,
  Fallback,
  FallbackRule,
  Index,
  Manifest,
  MetaEntry,
  Selector,
  Staged,
  StagedArtifact,
  UpdateDirectory,
  VersionNode,
} from "./gen/rup/v2/objects_pb.js";
export { EnvelopeSchema, SignatureSchema } from "./gen/rup/v2/envelope_pb.js";
export type { Envelope, Signature } from "./gen/rup/v2/envelope_pb.js";
export {
  PrivateKeyDocumentSchema,
  PublicKeyDocumentSchema,
} from "./gen/rup/v2/keys_pb.js";
export type {
  PrivateKeyDocument,
  PublicKeyDocument,
} from "./gen/rup/v2/keys_pb.js";

export const envelopeSchemaId = "rup.envelope/2";
export const indexSchemaId = "rup.index/2";
export const manifestSchemaId = "rup.manifest/2";
export const fallbackSchemaId = "rup.fallback/2";
export const directorySchemaId = "rup.directory/2";
export const publicKeySchemaId = "rup.publickey/2";
export const privateKeySchemaId = "rup.privatekey/2";
export const stagedSchemaId = "rup.staged/2";

/**
 * A document that does not conform to the protocol.
 *
 * Distinct from a network failure: this one will not be fixed by retrying, and
 * on a different mirror it will most likely reproduce, because mirrors carry
 * byte-identical documents (SPEC.md section 5.3).
 */
export class RupFormatError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "RupFormatError";
  }
}

const HEX64 = /^[0-9a-f]{64}$/;

function bad(message: string): never {
  throw new RupFormatError(message);
}

/** int64 fields as `number`. See the note at the top of this file. */
export const num = (value: bigint | number): number =>
  typeof value === "bigint" ? Number(value) : value;

export const nodeCode = (node: VersionNode): number => num(node.code);
export const nodeMinFrom = (node: VersionNode): number => num(node.minFrom);
export const indexSequence = (index: Index): number => num(index.sequence);
export const indexMinSupported = (index: Index): number =>
  num(index.minSupported);

export function selectorsToMap(
  selectors: readonly Selector[],
): Record<string, string> {
  const out: Record<string, string> = Object.create(null);
  for (const selector of selectors) out[selector.key] = selector.value;
  return out;
}

export function metaToMap(
  entries: readonly MetaEntry[],
): Record<string, string> {
  const out: Record<string, string> = Object.create(null);
  for (const entry of entries) out[entry.key] = entry.value;
  return out;
}

export function parseIndex(bytes: Uint8Array): Index {
  let index: Index;
  try {
    index = fromBinary(IndexSchema, bytes);
  } catch (error) {
    throw new RupFormatError(`index is not valid protobuf: ${String(error)}`);
  }
  validateIndex(index);
  return index;
}

export function parseManifest(bytes: Uint8Array): Manifest {
  let manifest: Manifest;
  try {
    manifest = fromBinary(ManifestSchema, bytes);
  } catch (error) {
    throw new RupFormatError(`manifest is not valid protobuf: ${String(error)}`);
  }
  validateManifest(manifest);
  return manifest;
}

export function parseFallback(bytes: Uint8Array): Fallback {
  let doc: Fallback;
  try {
    doc = fromBinary(FallbackSchema, bytes);
  } catch (error) {
    throw new RupFormatError(`fallback is not valid protobuf: ${String(error)}`);
  }
  validateFallback(doc);
  return doc;
}

export function parseDirectory(bytes: Uint8Array): UpdateDirectory {
  let doc: UpdateDirectory;
  try {
    doc = fromBinary(UpdateDirectorySchema, bytes);
  } catch (error) {
    throw new RupFormatError(
      `directory is not valid protobuf: ${String(error)}`,
    );
  }
  validateDirectory(doc);
  return doc;
}

function validateIndex(index: Index): void {
  const where = "index";
  requireSchema(index.schema, indexSchemaId, where);
  requireNonEmpty(index.product, `${where}.product`);
  requireNonEmpty(index.channel, `${where}.channel`);
  requireNonEmpty(index.generatedAt, `${where}.generatedAt`);
  requireAtLeast(num(index.sequence), 1, `${where}.sequence`);

  if (!index.hasMinSupported && num(index.minSupported) !== 0) {
    bad(`${where}.minSupported is set but ${where}.hasMinSupported is false`);
  }
  if (index.hasMinSupported) {
    requireAtLeast(num(index.minSupported), 0, `${where}.minSupported`);
  }

  if (index.versions.length === 0) {
    bad(`${where}.versions must be a non-empty array`);
  }
  index.versions.forEach((node, i) =>
    validateVersionNode(node, `${where}.versions[${i}]`),
  );
}

function validateVersionNode(node: VersionNode, where: string): void {
  requireNonEmpty(node.version, `${where}.version`);
  requireAtLeast(num(node.code), 0, `${where}.code`);
  requireAtLeast(num(node.minFrom), 0, `${where}.minFrom`);
  if (!node.manifest) bad(`${where}.manifest must be present`);
  validateDigestRef(node.manifest, `${where}.manifest`);
}

function validateDigestRef(digest: DigestRef, where: string): void {
  requireSha256(digest.sha256, `${where}.sha256`);
  requireAtLeast(num(digest.size), 0, `${where}.size`);
  requireUrls(digest.urls, `${where}.urls`);
}

function validateManifest(manifest: Manifest): void {
  const where = "manifest";
  requireSchema(manifest.schema, manifestSchemaId, where);
  requireNonEmpty(manifest.product, `${where}.product`);
  requireNonEmpty(manifest.version, `${where}.version`);
  requireAtLeast(num(manifest.code), 0, `${where}.code`);
  if (manifest.artifacts.length === 0) {
    bad(`${where}.artifacts must be a non-empty array`);
  }
  manifest.artifacts.forEach((artifact, i) =>
    validateArtifact(artifact, `${where}.artifacts[${i}]`),
  );
}

function validateArtifact(artifact: Artifact, where: string): void {
  requireNonEmpty(artifact.id, `${where}.id`);
  requireNonEmpty(artifact.filename, `${where}.filename`);
  requireAtLeast(num(artifact.size), 0, `${where}.size`);
  requireSha256(artifact.sha256, `${where}.sha256`);
  requireUrls(artifact.urls, `${where}.urls`);
  artifact.selectors.forEach((selector, i) =>
    requireNonEmpty(selector.key, `${where}.selectors[${i}].key`),
  );
  artifact.meta.forEach((entry, i) =>
    requireNonEmpty(entry.key, `${where}.meta[${i}].key`),
  );
}

function validateFallback(doc: Fallback): void {
  const where = "fallback";
  requireSchema(doc.schema, fallbackSchemaId, where);
  requireNonEmpty(doc.product, `${where}.product`);
  requireNonEmpty(doc.generatedAt, `${where}.generatedAt`);
  requireAtLeast(num(doc.sequence), 1, `${where}.sequence`);
  doc.rules.forEach((rule, i) => {
    const ruleWhere = `${where}.rules[${i}]`;
    requireAtLeast(num(rule.minCode), 0, `${ruleWhere}.minCode`);
    requireAtLeast(num(rule.maxCode), 1, `${ruleWhere}.maxCode`);
    if (num(rule.minCode) > num(rule.maxCode)) {
      bad(`${ruleWhere}.minCode must be <= maxCode`);
    }
    requireNonEmpty(rule.manualUrl, `${ruleWhere}.manualUrl`);
    rule.selectors.forEach((selector, j) =>
      requireNonEmpty(selector.key, `${ruleWhere}.selectors[${j}].key`),
    );
  });
}

function validateDirectory(doc: UpdateDirectory): void {
  const where = "directory";
  requireSchema(doc.schema, directorySchemaId, where);
  requireNonEmpty(doc.product, `${where}.product`);
  requireAtLeast(num(doc.directorySequence), 1, `${where}.directorySequence`);
  if (doc.services.length === 0) {
    bad(`${where}.services must be a non-empty array`);
  }
  const ids = new Set<string>();
  doc.services.forEach((service, i) => {
    const serviceWhere = `${where}.services[${i}]`;
    requireNonEmpty(service.id, `${serviceWhere}.id`);
    if (ids.has(service.id)) {
      bad(`${serviceWhere}.id "${service.id}" is duplicated`);
    }
    ids.add(service.id);
    requireNonEmpty(service.indexUrl, `${serviceWhere}.indexUrl`);
    requireAbsoluteUrl(service.indexUrl, `${serviceWhere}.indexUrl`);
    if (service.fallbackUrl.length > 0) {
      requireAbsoluteUrl(service.fallbackUrl, `${serviceWhere}.fallbackUrl`);
    }
  });
}

function requireSchema(actual: string, expected: string, where: string): void {
  requireNonEmpty(actual, `${where}.schema`);
  if (actual !== expected) {
    bad(`${where}.schema is "${actual}", expected "${expected}"`);
  }
}

function requireNonEmpty(value: string, where: string): void {
  if (value.length === 0) bad(`${where} must be a non-empty string`);
}

function requireAtLeast(value: number, min: number, where: string): void {
  if (value < min) bad(`${where} must be at least ${min}`);
}

function requireSha256(value: string, where: string): void {
  requireNonEmpty(value, where);
  if (!HEX64.test(value)) {
    bad(`${where} must be 64 lowercase hex characters`);
  }
}

function requireUrls(urls: readonly string[], where: string): void {
  if (urls.length === 0) bad(`${where} must be a non-empty array`);
  for (const url of urls) requireNonEmpty(url, where);
}

function requireAbsoluteUrl(value: string, where: string): void {
  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    bad(`${where} must be an absolute URL`);
  }
  if (parsed.protocol.length === 0 || parsed.host.length === 0) {
    bad(`${where} must be an absolute URL`);
  }
}
