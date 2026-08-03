/// Signed envelope verification (SPEC.md sections 4 and 4.1).
///
/// This is the security boundary of the whole protocol. Everything downstream
/// (which version is newest, where to download it, what it should hash to) is
/// read out of the payload, so a client that gets this wrong gains nothing from
/// any of the other checks.
library;

import 'dart:convert';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart' as crypto;

import 'models.dart';

/// An Ed25519 public key the client is willing to trust, by key id.
///
/// The set is embedded in the client build. That is what makes it meaningful:
/// a signature is only evidence if the verifier decided in advance whose
/// signatures count.
class TrustedKeys {
  TrustedKeys(Map<String, List<int>> keys)
      : _keys = {
          for (final entry in keys.entries)
            entry.key: _checkLength(entry.key, entry.value),
        };

  /// Keys given as standard base64, the form they appear in inside
  /// `relkit`-generated public key files.
  factory TrustedKeys.fromBase64(Map<String, String> keys) => TrustedKeys({
        for (final entry in keys.entries) entry.key: base64.decode(entry.value),
      });

  final Map<String, List<int>> _keys;

  static List<int> _checkLength(String keyId, List<int> bytes) {
    if (bytes.length != 32) {
      throw ArgumentError.value(
        bytes.length,
        'keys[$keyId]',
        'an Ed25519 public key is 32 bytes',
      );
    }
    return List.unmodifiable(bytes);
  }

  List<int>? operator [](String keyId) => _keys[keyId];

  Iterable<String> get keyIds => _keys.keys;

  bool get isEmpty => _keys.isEmpty;
}

/// Why an envelope was refused. The distinctions matter for diagnosis, not for
/// behaviour: every one of these means the document must not be used.
enum EnvelopeRejection {
  malformed,

  /// Not `rup.envelope/2`.
  wrongSchema,

  /// An empty signature array. Nothing to check is not the same as nothing
  /// wrong, and reading it as "vacuously fine" would disable the whole
  /// mechanism with a one-character change to a published file.
  noSignatures,

  /// Signatures exist, but none is from a key this client trusts, or none of
  /// the trusted ones verified.
  notVerified,
}

class EnvelopeResult {
  const EnvelopeResult._({
    this.payload,
    this.keyId,
    this.rejection,
    this.detail = '',
  });

  const EnvelopeResult.accepted(Uint8List payload, String keyId)
      : this._(payload: payload, keyId: keyId);

  const EnvelopeResult.rejected(EnvelopeRejection rejection, String detail)
      : this._(rejection: rejection, detail: detail);

  /// The verified bytes. Only non-null when accepted, and only these bytes may
  /// be parsed as an index.
  final Uint8List? payload;

  /// Which trusted key verified it.
  final String? keyId;

  final EnvelopeRejection? rejection;
  final String detail;

  bool get accepted => payload != null;

  @override
  String toString() =>
      accepted ? 'accepted (signed by $keyId)' : '${rejection!.name}: $detail';
}

final _ed25519 = crypto.Ed25519();

/// Verifies an envelope and returns the payload bytes it vouches for.
///
/// Two properties of this function are load-bearing:
///
/// The signature is checked against the raw decoded payload bytes, never
/// against a re-serialisation of the parsed object. Re-serialising would make
/// verification depend on this language's key ordering and number formatting,
/// so a document signed by `relkit` could fail here for reasons that look like
/// a broken signature.
///
/// A signature from an unknown key id is not a reason to stop. Key rotation
/// works by signing with the old and new keys at once, so an older client sees
/// an unknown signature first and a usable one second.
Future<EnvelopeResult> openEnvelope(
  List<int> bytes,
  TrustedKeys trusted,
) async {
  final Envelope env;
  try {
    env = Envelope.fromBuffer(bytes);
  } catch (error) {
    return EnvelopeResult.rejected(
        EnvelopeRejection.malformed, 'not valid protobuf: $error');
  }

  if (env.schema != envelopeSchemaId) {
    return EnvelopeResult.rejected(EnvelopeRejection.wrongSchema,
        'schema is "${env.schema}", expected "$envelopeSchemaId"');
  }
  if (env.signatures.isEmpty) {
    return const EnvelopeResult.rejected(
        EnvelopeRejection.noSignatures, 'signatures is empty');
  }

  for (final entry in env.signatures) {
    if (entry.alg != 'ed25519') continue;
    if (entry.keyId.isEmpty) continue;

    final publicKey = trusted[entry.keyId];
    if (publicKey == null) continue;
    final signature = entry.sig;
    if (signature.length != 64) continue;

    final ok = await _ed25519.verify(
      env.payload,
      signature: crypto.Signature(
        signature,
        publicKey: crypto.SimplePublicKey(
          publicKey,
          type: crypto.KeyPairType.ed25519,
        ),
      ),
    );
    if (ok) {
      return EnvelopeResult.accepted(
        Uint8List.fromList(env.payload),
        entry.keyId,
      );
    }
  }

  return const EnvelopeResult.rejected(EnvelopeRejection.notVerified,
      'no signature from a trusted key verified');
}
