/// Object model for the protobuf RUP documents a client reads (SPEC.md sections
/// 4-6).
library;

import 'dart:typed_data';

import 'gen/rup/v2/objects.pb.dart';

export 'gen/rup/v2/envelope.pb.dart' show Envelope, Signature;
export 'gen/rup/v2/keys.pb.dart' show PrivateKeyDocument, PublicKeyDocument;
export 'gen/rup/v2/objects.pb.dart'
    show
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
        VersionNode;

/// A document that does not conform to the protocol.
///
/// Distinct from a network failure: this one will not be fixed by retrying, and
/// on a different mirror it will most likely reproduce, because mirrors carry
/// byte-identical documents (SPEC.md section 5.3).
class RupFormatException implements Exception {
  RupFormatException(this.message);

  final String message;

  @override
  String toString() => 'RupFormatException: $message';
}

const envelopeSchemaId = 'rup.envelope/2';
const indexSchemaId = 'rup.index/2';
const manifestSchemaId = 'rup.manifest/2';
const fallbackSchemaId = 'rup.fallback/2';
const directorySchemaId = 'rup.directory/2';
const publicKeySchemaId = 'rup.publickey/2';
const privateKeySchemaId = 'rup.privatekey/2';
const stagedSchemaId = 'rup.staged/2';

final _hex64 = RegExp(r'^[0-9a-f]{64}$');

Never _bad(String message) => throw RupFormatException(message);

Index parseIndex(Uint8List bytes) {
  final Index index;
  try {
    index = Index.fromBuffer(bytes);
  } catch (error) {
    throw RupFormatException('index is not valid protobuf: $error');
  }
  _validateIndex(index);
  return index;
}

Manifest parseManifest(Uint8List bytes) {
  final Manifest manifest;
  try {
    manifest = Manifest.fromBuffer(bytes);
  } catch (error) {
    throw RupFormatException('manifest is not valid protobuf: $error');
  }
  _validateManifest(manifest);
  return manifest;
}


Fallback parseFallback(Uint8List bytes) {
  final Fallback doc;
  try {
    doc = Fallback.fromBuffer(bytes);
  } catch (error) {
    throw RupFormatException('fallback is not valid protobuf: $error');
  }
  _validateFallback(doc);
  return doc;
}

UpdateDirectory parseDirectory(Uint8List bytes) {
  final UpdateDirectory doc;
  try {
    doc = UpdateDirectory.fromBuffer(bytes);
  } catch (error) {
    throw RupFormatException('directory is not valid protobuf: $error');
  }
  _validateDirectory(doc);
  return doc;
}

Map<String, String> selectorsToMap(List<Selector> selectors) =>
    Map.unmodifiable({
      for (final selector in selectors) selector.key: selector.value,
    });


void _validateFallback(Fallback doc) {
  const where = 'fallback';
  _requireSchema(doc.schema, fallbackSchemaId, where);
  _requireNonEmptyString(doc.product, '$where.product');
  _requireNonEmptyString(doc.generatedAt, '$where.generatedAt');
  _requireAtLeast(doc.sequence.toInt(), 1, '$where.sequence');
  for (var i = 0; i < doc.rules.length; i++) {
    final rule = doc.rules[i];
    final ruleWhere = '$where.rules[$i]';
    _requireAtLeast(rule.minCode.toInt(), 0, '$ruleWhere.minCode');
    _requireAtLeast(rule.maxCode.toInt(), 1, '$ruleWhere.maxCode');
    if (rule.minCode.toInt() > rule.maxCode.toInt()) {
      _bad('$ruleWhere.minCode must be <= maxCode');
    }
    _requireNonEmptyString(rule.manualUrl, '$ruleWhere.manualUrl');
    for (var j = 0; j < rule.selectors.length; j++) {
      _requireNonEmptyString(
          rule.selectors[j].key, '$ruleWhere.selectors[$j].key');
    }
  }
}

void _validateDirectory(UpdateDirectory doc) {
  const where = 'directory';
  _requireSchema(doc.schema, directorySchemaId, where);
  _requireNonEmptyString(doc.product, '$where.product');
  _requireAtLeast(doc.directorySequence.toInt(), 1, '$where.directorySequence');
  if (doc.services.isEmpty) {
    _bad('$where.services must be a non-empty array');
  }
  final ids = <String>{};
  for (var i = 0; i < doc.services.length; i++) {
    final service = doc.services[i];
    final serviceWhere = '$where.services[$i]';
    _requireNonEmptyString(service.id, '$serviceWhere.id');
    if (!ids.add(service.id)) {
      _bad('$serviceWhere.id "${service.id}" is duplicated');
    }
    _requireNonEmptyString(service.indexUrl, '$serviceWhere.indexUrl');
    final indexUri = Uri.tryParse(service.indexUrl);
    if (indexUri == null || !indexUri.hasScheme || indexUri.host.isEmpty) {
      _bad('$serviceWhere.indexUrl must be an absolute URL');
    }
    if (service.fallbackUrl.isNotEmpty) {
      final fallbackUri = Uri.tryParse(service.fallbackUrl);
      if (fallbackUri == null ||
          !fallbackUri.hasScheme ||
          fallbackUri.host.isEmpty) {
        _bad('$serviceWhere.fallbackUrl must be an absolute URL');
      }
    }
  }
}

void _validateIndex(Index index) {
  const where = 'index';

  _requireSchema(index.schema, indexSchemaId, where);
  _requireNonEmptyString(index.product, '$where.product');
  _requireNonEmptyString(index.channel, '$where.channel');
  _requireNonEmptyString(index.generatedAt, '$where.generatedAt');
  _requireAtLeast(index.sequence.toInt(), 1, '$where.sequence');

  if (!index.hasMinSupported_7 && index.minSupported.toInt() != 0) {
    _bad('$where.minSupported is set but $where.hasMinSupported is false');
  }
  if (index.hasMinSupported_7) {
    _requireAtLeast(index.minSupported.toInt(), 0, '$where.minSupported');
  }

  if (index.versions.isEmpty) {
    _bad('$where.versions must be a non-empty array');
  }
  for (var i = 0; i < index.versions.length; i++) {
    _validateVersionNode(index.versions[i], '$where.versions[$i]');
  }
}

void _validateVersionNode(VersionNode node, String where) {
  _requireNonEmptyString(node.version, '$where.version');
  _requireAtLeast(node.code.toInt(), 0, '$where.code');
  _requireAtLeast(node.minFrom.toInt(), 0, '$where.minFrom');

  if (!node.hasManifest()) {
    _bad('$where.manifest must be present');
  }
  _validateDigestRef(node.manifest, '$where.manifest');
}

void _validateDigestRef(DigestRef digest, String where) {
  _requireSha256(digest.sha256, '$where.sha256');
  _requireAtLeast(digest.size.toInt(), 0, '$where.size');
  _requireUrls(digest.urls, '$where.urls');
}

void _validateManifest(Manifest manifest) {
  const where = 'manifest';

  _requireSchema(manifest.schema, manifestSchemaId, where);
  _requireNonEmptyString(manifest.product, '$where.product');
  _requireNonEmptyString(manifest.version, '$where.version');
  _requireAtLeast(manifest.code.toInt(), 0, '$where.code');

  if (manifest.artifacts.isEmpty) {
    _bad('$where.artifacts must be a non-empty array');
  }
  for (var i = 0; i < manifest.artifacts.length; i++) {
    _validateArtifact(manifest.artifacts[i], '$where.artifacts[$i]');
  }
}

void _validateArtifact(Artifact artifact, String where) {
  _requireNonEmptyString(artifact.id, '$where.id');
  _requireNonEmptyString(artifact.filename, '$where.filename');
  _requireAtLeast(artifact.size.toInt(), 0, '$where.size');
  _requireSha256(artifact.sha256, '$where.sha256');
  _requireUrls(artifact.urls, '$where.urls');

  for (var i = 0; i < artifact.selectors.length; i++) {
    final selector = artifact.selectors[i];
    _requireNonEmptyString(selector.key, '$where.selectors[$i].key');
  }
  for (var i = 0; i < artifact.meta.length; i++) {
    final entry = artifact.meta[i];
    _requireNonEmptyString(entry.key, '$where.meta[$i].key');
  }
}

void _requireSchema(String actual, String expected, String where) {
  _requireNonEmptyString(actual, '$where.schema');
  if (actual != expected) {
    _bad('$where.schema is "$actual", expected "$expected"');
  }
}

void _requireNonEmptyString(String value, String where) {
  if (value.isEmpty) _bad('$where must be a non-empty string');
}

void _requireAtLeast(int value, int min, String where) {
  if (value < min) _bad('$where must be at least $min');
}

void _requireSha256(String value, String where) {
  _requireNonEmptyString(value, where);
  if (!_hex64.hasMatch(value)) {
    _bad('$where must be 64 lowercase hex characters');
  }
}

void _requireUrls(List<String> urls, String where) {
  if (urls.isEmpty) _bad('$where must be a non-empty array');
  for (final url in urls) {
    _requireNonEmptyString(url, where);
  }
}
