/// Runs the shared, language-agnostic fixtures in `conformance/`.
///
/// These are the same fixtures the Go publisher (`relkit`) is checked against,
/// read from their original location rather than copied here. A copy would
/// drift, and a drifted copy is worse than no copy: the suite would stay green
/// while implementations quietly diverged, which is the exact failure the
/// fixtures exist to prevent.
///
/// `reachability/` is not run here. It is a publishing-side check (SPEC.md
/// section 10) about whether an index may be released at all, and a client has
/// no use for it.
library;

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart' as crypto;
import 'package:fixnum/fixnum.dart';
import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

/// Walks up from the package directory to find `conformance/`.
///
/// Located rather than hard-coded so the test still works when the package is
/// vendored somewhere else in the tree.
Directory findConformanceDir() {
  final override = Platform.environment['RUP_CONFORMANCE_DIR'];
  if (override != null && override.isNotEmpty) {
    final dir = Directory(override);
    if (!dir.existsSync()) {
      throw StateError('RUP_CONFORMANCE_DIR points at $override, which does '
          'not exist');
    }
    return dir;
  }

  var dir = Directory.current.absolute;
  for (var depth = 0; depth < 6; depth++) {
    final candidate =
        Directory('${dir.path}${Platform.pathSeparator}conformance');
    if (candidate.existsSync()) return candidate;
    final parent = dir.parent;
    if (parent.path == dir.path) break;
    dir = parent;
  }
  throw StateError(
      'could not find conformance/ above ${Directory.current.path}; set '
      'RUP_CONFORMANCE_DIR to point at it');
}

Map<String, dynamic> readFixture(Directory root, String relative) {
  final path = '${root.path}${Platform.pathSeparator}'
      '${relative.replaceAll('/', Platform.pathSeparator)}';
  final raw = File(path).readAsStringSync(encoding: utf8);
  return json.decode(raw) as Map<String, dynamic>;
}

List<Selector> selectorsFromFixture(Map<dynamic, dynamic>? selectors) {
  if (selectors == null) return [];
  final entries = selectors.entries
      .map((entry) => MapEntry(entry.key as String, entry.value as String))
      .toList()
    ..sort((a, b) => a.key.compareTo(b.key));
  return [
    for (final entry in entries) Selector(key: entry.key, value: entry.value),
  ];
}

List<MetaEntry> metaFromFixture(Map<dynamic, dynamic>? meta) {
  if (meta == null) return [];
  final entries = meta.entries
      .map((entry) => MapEntry(entry.key as String, entry.value as String))
      .toList()
    ..sort((a, b) => a.key.compareTo(b.key));
  return [
    for (final entry in entries) MetaEntry(key: entry.key, value: entry.value),
  ];
}

DigestRef digestRefFromFixture(Map<String, dynamic> json) => DigestRef(
      sha256: json['sha256'] as String,
      size: Int64(json['size'] as int),
      urls: (json['urls'] as List).cast<String>(),
    );

VersionNode versionNodeFromFixture(Map<String, dynamic> json) => VersionNode(
      version: json['version'] as String,
      code: Int64(json['code'] as int),
      minFrom: Int64((json['minFrom'] as int?) ?? 0),
      yanked: json['yanked'] as bool? ?? false,
      manifest: digestRefFromFixture(json['manifest'] as Map<String, dynamic>),
      releasedAt: json['releasedAt'] as String? ?? '',
      notes: json['notes'] as String? ?? '',
      notesUrl: json['notesUrl'] as String? ?? '',
    );

Index indexFromFixture(Map<String, dynamic> json) => Index(
      schema: indexSchemaId,
      product: json['product'] as String,
      channel: json['channel'] as String,
      sequence: Int64(json['sequence'] as int),
      generatedAt: json['generatedAt'] as String,
      minSupported: json.containsKey('minSupported')
          ? Int64(json['minSupported'] as int)
          : null,
      hasMinSupported_7: json.containsKey('minSupported'),
      expiresAt: json['expiresAt'] as String? ?? '',
      versions: [
        for (final raw in json['versions'] as List)
          versionNodeFromFixture(raw as Map<String, dynamic>),
      ],
    );

Artifact artifactFromFixture(Map<String, dynamic> json) => Artifact(
      id: json['id'] as String,
      filename: json['filename'] as String,
      size: Int64(json['size'] as int),
      sha256: json['sha256'] as String,
      kind: json['kind'] as String? ?? '',
      selectors:
          selectorsFromFixture((json['selectors'] as Map<dynamic, dynamic>?)),
      urls: (json['urls'] as List).cast<String>(),
      meta: metaFromFixture(json['meta'] as Map<dynamic, dynamic>?),
    );

Manifest manifestFromFixture(Map<String, dynamic> json) => Manifest(
      schema: manifestSchemaId,
      product: json['product'] as String,
      version: json['version'] as String,
      code: Int64(json['code'] as int),
      releasedAt: json['releasedAt'] as String? ?? '',
      notes: json['notes'] as String? ?? '',
      artifacts: [
        for (final raw in json['artifacts'] as List)
          artifactFromFixture(raw as Map<String, dynamic>),
      ],
    );

class FixtureKey {
  FixtureKey({
    required this.keyId,
    required this.publicKey,
    required this.seed,
  });

  final String keyId;
  final Uint8List publicKey;
  final Uint8List seed;
}

Future<Signature> _signFixturePayload(
  String keyId,
  Uint8List payload,
  Map<String, FixtureKey> keys, {
  String alg = 'ed25519',
}) async {
  final key = keys[keyId]!;
  final keyPair = await crypto.Ed25519().newKeyPairFromSeed(key.seed);
  final signature = await crypto.Ed25519().sign(payload, keyPair: keyPair);
  return Signature(
    keyId: keyId,
    alg: alg,
    sig: signature.bytes,
  );
}

Future<Uint8List> buildEnvelopeCase(
  Map<String, dynamic> testCase,
  Map<String, FixtureKey> keys,
  Map<String, dynamic> canonicalPayloadJson,
) async {
  final name = testCase['name'] as String;
  final payloadJson = name == 'wrong-product' || name == 'wrong-channel'
      ? json.decode(utf8.decode(base64.decode(
          (testCase['envelope'] as Map<String, dynamic>)['payload'] as String,
        ))) as Map<String, dynamic>
      : canonicalPayloadJson;

  Uint8List payload = Uint8List.fromList(
    indexFromFixture(payloadJson).writeToBuffer(),
  );
  var schema = envelopeSchemaId;
  final signatures = <Signature>[];

  switch (name) {
    case 'valid-k1':
      signatures.add(await _signFixturePayload('k1', payload, keys));
      break;
    case 'valid-k2':
      signatures.add(await _signFixturePayload('k2', payload, keys));
      break;
    case 'unknown-key':
      signatures.add(await _signFixturePayload('kx', payload, keys));
      break;
    case 'tampered-payload':
      signatures.add(await _signFixturePayload('k1', payload, keys));
      final tampered = Uint8List.fromList(payload);
      tampered[tampered.length - 1] ^= 0x01;
      payload = tampered;
      break;
    case 'bad-signature':
      final signature = await _signFixturePayload('k1', payload, keys);
      final badBytes = Uint8List.fromList(signature.sig);
      badBytes[badBytes.length - 1] ^= 0x01;
      signatures.add(Signature(
        keyId: signature.keyId,
        alg: signature.alg,
        sig: badBytes,
      ));
      break;
    case 'unsupported-alg':
      signatures.add(
          await _signFixturePayload('k1', payload, keys, alg: 'rsa-sha256'));
      break;
    case 'cross-payload-replay':
      final baseIndex = indexFromFixture(payloadJson);
      final other = baseIndex.copyWith((index) {
        index.sequence = Int64(baseIndex.sequence.toInt() + 1);
      });
      signatures.add(await _signFixturePayload(
        'k1',
        Uint8List.fromList(other.writeToBuffer()),
        keys,
      ));
      break;
    case 'rotation-untrusted-first':
      signatures.add(await _signFixturePayload('kx', payload, keys));
      signatures.add(await _signFixturePayload('k1', payload, keys));
      break;
    case 'rotation-all-untrusted':
      signatures.add(await _signFixturePayload('kx', payload, keys));
      signatures.add(await _signFixturePayload('ky', payload, keys));
      break;
    case 'no-signatures':
      break;
    case 'wrong-envelope-schema':
      schema = 'rup.envelope/1';
      signatures.add(await _signFixturePayload('k1', payload, keys));
      break;
    case 'wrong-product':
      signatures.add(await _signFixturePayload('k1', payload, keys));
      break;
    case 'wrong-channel':
      signatures.add(await _signFixturePayload('k1', payload, keys));
      break;
    default:
      throw StateError('unhandled signature case "$name"');
  }

  final envelope = Envelope(
    schema: schema,
    payload: payload,
    signatures: signatures,
  );
  return Uint8List.fromList(envelope.writeToBuffer());
}

/// Counts every assertion made, so the suite cannot pass by doing nothing.
///
/// Each fixture drives a loop over `cases`. If a file failed to load, or its
/// case list were empty, every loop would complete without asserting anything
/// and the run would be green. The totals below are checked at the end, which
/// turns that silent success into a failure.
var _casesChecked = 0;

void main() {
  final root = findConformanceDir();

  // 8 version-select files, 3 selector files, 2 signature files.
  const expectedCases = 65;

  tearDownAll(() {
    expect(_casesChecked, expectedCases,
        reason: 'expected $expectedCases fixture cases, ran $_casesChecked. '
            'A fixture was added, removed, or silently failed to load.');
  });

  group('version-select (SPEC.md section 9)', () {
    const files = [
      'flat-chain',
      'min-supported',
      'required-intermediate',
      'single-version',
      'three-hops',
      'unordered',
      'yanked-head',
      'yanked',
    ];

    for (final name in files) {
      test(name, () {
        final fixture = readFixture(root, 'version-select/$name.json');
        final index =
            indexFromFixture(fixture['index'] as Map<String, dynamic>);

        for (final raw in fixture['cases'] as List) {
          final testCase = raw as Map<String, dynamic>;
          _casesChecked++;
          final currentCode = testCase['currentCode'] as int;
          final reason = 'currentCode=$currentCode in $name';

          final target = selectNextTarget(index, currentCode);
          expect(target?.version, testCase['expectTarget'],
              reason: 'selectNextTarget for $reason');

          final path = resolveUpgradePath(index, currentCode)
              .map((node) => node.version)
              .toList();
          expect(path, testCase['expectPath'],
              reason: 'resolveUpgradePath for $reason');

          final expectMandatory = testCase['expectMandatory'] as bool? ?? false;
          expect(isMandatory(index, currentCode), expectMandatory,
              reason: 'isMandatory for $reason');
        }
      });
    }
  });

  group('selector (SPEC.md section 11)', () {
    const files = ['ambiguous', 'os-arch', 'target-dimension'];

    for (final name in files) {
      test(name, () {
        final fixture = readFixture(root, 'selector/$name.json');
        final manifest =
            manifestFromFixture(fixture['manifest'] as Map<String, dynamic>);

        for (final raw in fixture['cases'] as List) {
          final testCase = raw as Map<String, dynamic>;
          _casesChecked++;
          final selectors = (testCase['clientSelectors'] as Map)
              .map((key, value) => MapEntry(key as String, value as String));

          final chosen = selectArtifact(manifest, selectors);
          expect(chosen?.id, testCase['expectArtifactId'],
              reason: 'selectors $selectors in $name');
        }
      });
    }
  });

  group('signature (SPEC.md sections 4.1, 12.1, 12.4)', () {
    test('envelope', () async {
      final keysFixture = readFixture(root, 'signature/keys.json');
      final fixture = readFixture(root, 'signature/envelope.json');

      final fixtureKeys = <String, FixtureKey>{
        for (final raw in keysFixture['keys'] as List)
          (raw as Map<String, dynamic>)['keyId'] as String: FixtureKey(
            keyId: raw['keyId'] as String,
            publicKey: Uint8List.fromList(
                base64.decode(raw['publicKeyBase64'] as String)),
            seed: Uint8List.fromList(
                base64.decode(raw['privateSeedBase64'] as String)),
          ),
      };

      final trustedIds = (fixture['trustedKeys'] as List).cast<String>();
      final trusted = TrustedKeys({
        for (final entry in fixtureKeys.entries)
          if (trustedIds.contains(entry.key)) entry.key: entry.value.publicKey,
      });

      final expectProduct = fixture['expectProduct'] as String;
      final expectChannel = fixture['expectChannel'] as String;
      final canonicalPayloadJson = json.decode(
        utf8.decode(base64.decode(
          ((fixture['cases'] as List).cast<Map<String, dynamic>>().firstWhere(
                  (entry) => entry['name'] == 'valid-k1')['envelope']
              as Map<String, dynamic>)['payload'] as String,
        )),
      ) as Map<String, dynamic>;

      for (final raw in fixture['cases'] as List) {
        final testCase = raw as Map<String, dynamic>;
        _casesChecked++;
        final name = testCase['name'] as String;
        final bytes = await buildEnvelopeCase(
          testCase,
          fixtureKeys,
          canonicalPayloadJson,
        );

        var accepted = false;
        final result = await openEnvelope(bytes, trusted);
        if (result.accepted) {
          try {
            final index = parseIndex(result.payload!);
            accepted = index.product == expectProduct &&
                index.channel == expectChannel;
          } on RupFormatException {
            accepted = false;
          }
        }

        expect(accepted, testCase['expectAccepted'],
            reason: 'case "$name" (verification said: $result)');
      }
    });

    test('anti-rollback', () {
      final fixture = readFixture(root, 'signature/anti-rollback.json');

      for (final raw in fixture['cases'] as List) {
        final testCase = raw as Map<String, dynamic>;
        _casesChecked++;
        final lastSeen = testCase['lastSeenSequence'] as int?;
        final sequence = testCase['indexSequence'] as int;

        expect(acceptsSequence(sequence, lastSeen), testCase['expectAccepted'],
            reason: 'lastSeen=$lastSeen index=$sequence');
      }
    });
  });
}
