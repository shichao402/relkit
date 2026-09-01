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

Uint8List envelopeBytesFromFixture(Map<String, dynamic> testCase) {
  final envelope = testCase['envelope'] as Map<String, dynamic>;
  return Uint8List.fromList(Envelope(
    schema: envelope['schema'] as String,
    payload: base64.decode(envelope['payload'] as String),
    signatures: [
      for (final raw in (envelope['signatures'] as List?) ?? const [])
        Signature(
          keyId: (raw as Map<String, dynamic>)['keyId'] as String,
          alg: raw['alg'] as String,
          sig: base64.decode(raw['sig'] as String),
        ),
    ],
  ).writeToBuffer());
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

      final trustedIds = (fixture['trustedKeys'] as List).cast<String>();
      final trusted = TrustedKeys({
        for (final raw in keysFixture['keys'] as List)
          if (trustedIds.contains((raw as Map<String, dynamic>)['keyId']))
            raw['keyId'] as String: Uint8List.fromList(
                base64.decode(raw['publicKeyBase64'] as String)),
      });

      final expectProduct = fixture['expectProduct'] as String;
      final expectChannel = fixture['expectChannel'] as String;

      for (final raw in fixture['cases'] as List) {
        final testCase = raw as Map<String, dynamic>;
        _casesChecked++;
        final name = testCase['name'] as String;
        final bytes = envelopeBytesFromFixture(testCase);

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
