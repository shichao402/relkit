/// Cross-implementation gate: bytes sealed by the Go control plane, read here.
///
/// The shared `conformance/` fixtures prove both implementations agree with the
/// same test data. They cannot prove the two agree with *each other*, because
/// each side could satisfy every fixture while disagreeing about something no
/// fixture pins down. Those defects surface only when real bytes from one side
/// are fed to the other.
///
/// `fixtures/interop-index.json` is produced by `internal/envelope` through the
/// same sealing path `publish.Run` uses, and `internal/envelope`'s
/// `TestSdkInteropFixtureIsNotStale` re-seals on every run so the checked-in
/// bytes cannot drift from what relkit actually emits. This test is the consumer
/// half of that gate: signature verification, index parsing and chain selection
/// all run on those exact bytes.
library;

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

void main() {
  final fixture = jsonDecode(
    File('test/fixtures/interop-index.json').readAsStringSync(),
  ) as Map<String, dynamic>;

  final trustedIds = (fixture['trustedKeys'] as List).cast<String>();
  final envelopeBytes =
      base64.decode(fixture['envelopeBase64'] as String) as Uint8List;

  // The public keys live in the shared conformance key table; the fixture only
  // names which of them this product trusts.
  final keys = jsonDecode(
    File('conformance/signature/keys.json').readAsStringSync(),
  ) as Map<String, dynamic>;
  final trusted = TrustedKeys({
    for (final raw in (keys['keys'] as List).cast<Map<String, dynamic>>())
      if (trustedIds.contains(raw['keyId']))
        raw['keyId'] as String:
            Uint8List.fromList(base64.decode(raw['publicKeyBase64'] as String)),
  });

  group('bytes sealed by the relkit control plane', () {
    test('verify and parse into an index for this product', () async {
      final result = await openEnvelope(envelopeBytes, trusted);
      expect(result.accepted, isTrue,
          reason: 'the client rejected genuinely sealed bytes: $result');

      final index = parseIndex(result.payload!);
      expect(index.product, fixture['expectProduct']);
      expect(index.channel, fixture['expectChannel']);
      expect(index.sequence.toInt(), fixture['expectSequence']);
      expect(index.versions.map((node) => node.version),
          ['1.0.0', '1.5.0']);
    });

    test('walk the chain the way the fixture declares', () async {
      final result = await openEnvelope(envelopeBytes, trusted);
      final index = parseIndex(result.payload!);

      for (final raw in (fixture['cases'] as List).cast<Map<String, dynamic>>()) {
        final currentCode = raw['currentCode'] as int;
        final where = 'currentCode $currentCode';

        final target = selectNextTarget(index, currentCode);
        expect(target?.version, raw['expectTarget'], reason: where);

        final path =
            resolveUpgradePath(index, currentCode).map((n) => n.version);
        expect(path, (raw['expectPath'] as List).cast<String>(),
            reason: where);

        expect(isMandatory(index, currentCode), raw['expectMandatory'],
            reason: where);
      }
    });

    test('reject those same bytes under a key this product does not trust',
        () async {
      final wrongKey = TrustedKeys.fromBase64({
        'k1': base64.encode(List<int>.filled(32, 7)),
      });

      final result = await openEnvelope(envelopeBytes, wrongKey);
      expect(result.accepted, isFalse,
          reason: 'a forged trust root must not accept real bytes');
    });
  });
}
