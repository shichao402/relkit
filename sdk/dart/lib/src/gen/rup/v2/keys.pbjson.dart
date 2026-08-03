// This is a generated file - do not edit.
//
// Generated from rup/v2/keys.proto.

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

@$core.Deprecated('Use publicKeyDocumentDescriptor instead')
const PublicKeyDocument$json = {
  '1': 'PublicKeyDocument',
  '2': [
    {'1': 'schema', '3': 1, '4': 1, '5': 9, '10': 'schema'},
    {'1': 'key_id', '3': 2, '4': 1, '5': 9, '10': 'keyId'},
    {'1': 'alg', '3': 3, '4': 1, '5': 9, '10': 'alg'},
    {'1': 'public_key', '3': 4, '4': 1, '5': 12, '10': 'publicKey'},
  ],
};

/// Descriptor for `PublicKeyDocument`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List publicKeyDocumentDescriptor = $convert.base64Decode(
    'ChFQdWJsaWNLZXlEb2N1bWVudBIWCgZzY2hlbWEYASABKAlSBnNjaGVtYRIVCgZrZXlfaWQYAi'
    'ABKAlSBWtleUlkEhAKA2FsZxgDIAEoCVIDYWxnEh0KCnB1YmxpY19rZXkYBCABKAxSCXB1Ymxp'
    'Y0tleQ==');

@$core.Deprecated('Use privateKeyDocumentDescriptor instead')
const PrivateKeyDocument$json = {
  '1': 'PrivateKeyDocument',
  '2': [
    {'1': 'schema', '3': 1, '4': 1, '5': 9, '10': 'schema'},
    {'1': 'key_id', '3': 2, '4': 1, '5': 9, '10': 'keyId'},
    {'1': 'alg', '3': 3, '4': 1, '5': 9, '10': 'alg'},
    {'1': 'seed', '3': 4, '4': 1, '5': 12, '10': 'seed'},
  ],
};

/// Descriptor for `PrivateKeyDocument`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List privateKeyDocumentDescriptor = $convert.base64Decode(
    'ChJQcml2YXRlS2V5RG9jdW1lbnQSFgoGc2NoZW1hGAEgASgJUgZzY2hlbWESFQoGa2V5X2lkGA'
    'IgASgJUgVrZXlJZBIQCgNhbGcYAyABKAlSA2FsZxISCgRzZWVkGAQgASgMUgRzZWVk');
