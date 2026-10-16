import 'dart:convert';

import 'gen/updater/v1/updater.pb.dart';

const String _runtimeBootstrapSchema = 'relkit.runtime-bootstrap/1';

final class RuntimeBootstrap {
  RuntimeBootstrap({
    required this.resolutionDigest,
    required this.profileDigest,
    required this.rootFingerprint,
    required this.clientProfile,
  });

  final String resolutionDigest;
  final String profileDigest;
  final String rootFingerprint;
  final ClientProfile clientProfile;
}

RuntimeBootstrap runtimeBootstrapFromJson(String source) {
  final Object? decoded;
  try {
    decoded = jsonDecode(source);
  } on FormatException catch (error) {
    throw FormatException('invalid runtime bootstrap JSON: ${error.message}');
  }
  final document = _object(decoded, r'$');
  if (document['schema'] != _runtimeBootstrapSchema) {
    throw FormatException(
      'runtime bootstrap schema is ${document['schema']}, '
      'expected $_runtimeBootstrapSchema',
    );
  }
  final resolutionDigest = _digest(document, 'resolutionDigest');
  final profileDigest = _digest(document, 'profileDigest');
  final rootFingerprint = _digest(document, 'rootFingerprint');
  final profile = _object(document['profile'], r'$.profile');
  if (profile['digest'] != profileDigest) {
    throw const FormatException(
      'embedded profile digest does not match profileDigest',
    );
  }
  final product = _nonEmptyString(profile, 'product', r'$.profile');
  final channels = _stringList(
    profile['allowedChannels'],
    r'$.profile.allowedChannels',
  );
  final directory = _object(
    profile['directoryBootstrap'],
    r'$.profile.directoryBootstrap',
  );
  final entryUrls = _stringList(
    directory['entryUrls'],
    r'$.profile.directoryBootstrap.entryUrls',
  );
  final keys = <TrustedKey>[];
  final keyIds = <String>{};
  final rawKeys = document['trustedKeys'];
  if (rawKeys is! List || rawKeys.isEmpty) {
    throw const FormatException(r'$.trustedKeys must be a non-empty array');
  }
  for (var index = 0; index < rawKeys.length; index += 1) {
    final key = _object(rawKeys[index], r'$.trustedKeys[]');
    final keyId = _nonEmptyString(key, 'keyId', r'$.trustedKeys[]');
    if (!keyIds.add(keyId)) {
      throw FormatException('duplicate trusted key id $keyId');
    }
    final encoded = _nonEmptyString(key, 'key', r'$.trustedKeys[]');
    final List<int> publicKey;
    try {
      publicKey = base64Decode(encoded);
    } on FormatException {
      throw FormatException('trusted key $keyId is not base64');
    }
    if (publicKey.length != 32) {
      throw FormatException(
        'trusted key $keyId must contain a 32-byte ed25519 key',
      );
    }
    keys.add(TrustedKey(keyId: keyId, publicKey: publicKey));
  }
  RecoveryHelp? recovery;
  if (directory['recovery'] case final Object rawRecovery) {
    final value = _object(
      rawRecovery,
      r'$.profile.directoryBootstrap.recovery',
    );
    final links = <RecoveryLink>[];
    final rawLinks = value['links'];
    if (rawLinks != null) {
      if (rawLinks is! List) {
        throw const FormatException('recovery.links must be an array');
      }
      for (final rawLink in rawLinks) {
        final link =
            _object(rawLink, r'$.profile.directoryBootstrap.recovery.links[]');
        links.add(
          RecoveryLink(
            label: _nonEmptyString(link, 'label', 'recovery.links[]'),
            url: _nonEmptyString(link, 'url', 'recovery.links[]'),
          ),
        );
      }
    }
    recovery = RecoveryHelp(
      message: value['message'] is String ? value['message'] as String : '',
      links: links,
    );
  }
  return RuntimeBootstrap(
    resolutionDigest: resolutionDigest,
    profileDigest: profileDigest,
    rootFingerprint: rootFingerprint,
    clientProfile: ClientProfile(
      product: product,
      allowedChannels: channels,
      entryUrls: entryUrls,
      trustedKeys: keys,
      recovery: recovery,
    ),
  );
}

Map<String, Object?> _object(Object? value, String path) {
  if (value is! Map) {
    throw FormatException('$path must be an object');
  }
  return value.map(
    (key, item) => MapEntry(key.toString(), item),
  );
}

String _nonEmptyString(
  Map<String, Object?> value,
  String key,
  String path,
) {
  final result = value[key];
  if (result is! String || result.isEmpty) {
    throw FormatException('$path.$key must be a non-empty string');
  }
  return result;
}

String _digest(Map<String, Object?> value, String key) {
  final digest = _nonEmptyString(value, key, r'$');
  if (!RegExp(r'^sha256:[0-9a-f]{64}$').hasMatch(digest)) {
    throw FormatException(r'$.' '$key must be a sha256 digest');
  }
  return digest;
}

List<String> _stringList(Object? value, String path) {
  if (value is! List || value.isEmpty) {
    throw FormatException('$path must be a non-empty array');
  }
  final result =
      value.whereType<String>().where((item) => item.isNotEmpty).toList();
  if (result.length != value.length) {
    throw FormatException('$path must contain non-empty strings');
  }
  return result;
}
