// This is a generated file - do not edit.
//
// Generated from rup/v2/envelope.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports
// ignore_for_file: unused_import

import 'dart:convert' as $convert;
import 'dart:core' as $core;
import 'dart:typed_data' as $typed_data;

@$core.Deprecated('Use signatureDescriptor instead')
const Signature$json = {
  '1': 'Signature',
  '2': [
    {'1': 'key_id', '3': 1, '4': 1, '5': 9, '10': 'keyId'},
    {'1': 'alg', '3': 2, '4': 1, '5': 9, '10': 'alg'},
    {'1': 'sig', '3': 3, '4': 1, '5': 12, '10': 'sig'},
  ],
};

/// Descriptor for `Signature`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List signatureDescriptor = $convert.base64Decode(
    'CglTaWduYXR1cmUSFQoGa2V5X2lkGAEgASgJUgVrZXlJZBIQCgNhbGcYAiABKAlSA2FsZxIQCg'
    'NzaWcYAyABKAxSA3NpZw==');

@$core.Deprecated('Use envelopeDescriptor instead')
const Envelope$json = {
  '1': 'Envelope',
  '2': [
    {'1': 'schema', '3': 1, '4': 1, '5': 9, '10': 'schema'},
    {'1': 'payload', '3': 2, '4': 1, '5': 12, '10': 'payload'},
    {
      '1': 'signatures',
      '3': 3,
      '4': 3,
      '5': 11,
      '6': '.rup.v2.Signature',
      '10': 'signatures'
    },
  ],
};

/// Descriptor for `Envelope`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List envelopeDescriptor = $convert.base64Decode(
    'CghFbnZlbG9wZRIWCgZzY2hlbWEYASABKAlSBnNjaGVtYRIYCgdwYXlsb2FkGAIgASgMUgdwYX'
    'lsb2FkEjEKCnNpZ25hdHVyZXMYAyADKAsyES5ydXAudjIuU2lnbmF0dXJlUgpzaWduYXR1cmVz');
