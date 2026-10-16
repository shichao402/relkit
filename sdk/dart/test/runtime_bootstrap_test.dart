import 'dart:convert';

import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

void main() {
  test('projects a verified runtime bootstrap into ClientProfile', () {
    final digest = 'sha256:${'a' * 64}';
    final root = 'sha256:${'b' * 64}';
    final key = base64Encode(List<int>.filled(32, 7));
    final value = runtimeBootstrapFromJson(
      jsonEncode(<String, Object?>{
        'schema': 'relkit.runtime-bootstrap/1',
        'resolutionDigest': digest,
        'profileDigest': digest,
        'rootFingerprint': root,
        'profile': <String, Object?>{
          'schema': 'relkit.runtime-profile/1',
          'product': 'demo',
          'profileVersion': 1,
          'allowedChannels': <String>['stable'],
          'directoryBootstrap': <String, Object?>{
            'entryUrls': <String>[
              'https://updates.example/directory/demo.pb',
            ],
            'recovery': <String, Object?>{
              'message': 'offline',
              'links': <Object?>[
                <String, String>{
                  'label': 'download',
                  'url': 'https://updates.example/',
                },
              ],
            },
          },
          'rupSchemaSet': 'rup.v2',
          'digest': digest,
          'signatures': <Object?>[],
        },
        'trustedKeys': <Object?>[
          <String, String>{'keyId': 'root', 'key': key},
        ],
      }),
    );
    expect(value.clientProfile.product, 'demo');
    expect(value.clientProfile.allowedChannels, <String>['stable']);
    expect(value.clientProfile.indexUrls, isEmpty);
    expect(value.clientProfile.trustedKeys.single.publicKey,
        List<int>.filled(32, 7));
    expect(value.clientProfile.recovery.message, 'offline');
  });

  test('rejects invalid input rather than inventing fallback endpoints', () {
    expect(
      () => runtimeBootstrapFromJson('{}'),
      throwsA(isA<FormatException>()),
    );
  });
}
