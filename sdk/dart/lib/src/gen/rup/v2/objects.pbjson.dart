// This is a generated file - do not edit.
//
// Generated from rup/v2/objects.proto.

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

@$core.Deprecated('Use selectorDescriptor instead')
const Selector$json = {
  '1': 'Selector',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 9, '10': 'value'},
  ],
};

/// Descriptor for `Selector`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List selectorDescriptor = $convert.base64Decode(
    'CghTZWxlY3RvchIQCgNrZXkYASABKAlSA2tleRIUCgV2YWx1ZRgCIAEoCVIFdmFsdWU=');

@$core.Deprecated('Use metaEntryDescriptor instead')
const MetaEntry$json = {
  '1': 'MetaEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 9, '10': 'value'},
  ],
};

/// Descriptor for `MetaEntry`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List metaEntryDescriptor = $convert.base64Decode(
    'CglNZXRhRW50cnkSEAoDa2V5GAEgASgJUgNrZXkSFAoFdmFsdWUYAiABKAlSBXZhbHVl');

@$core.Deprecated('Use digestRefDescriptor instead')
const DigestRef$json = {
  '1': 'DigestRef',
  '2': [
    {'1': 'sha256', '3': 1, '4': 1, '5': 9, '10': 'sha256'},
    {'1': 'size', '3': 2, '4': 1, '5': 3, '10': 'size'},
    {'1': 'urls', '3': 3, '4': 3, '5': 9, '10': 'urls'},
  ],
};

/// Descriptor for `DigestRef`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List digestRefDescriptor = $convert.base64Decode(
    'CglEaWdlc3RSZWYSFgoGc2hhMjU2GAEgASgJUgZzaGEyNTYSEgoEc2l6ZRgCIAEoA1IEc2l6ZR'
    'ISCgR1cmxzGAMgAygJUgR1cmxz');

@$core.Deprecated('Use versionNodeDescriptor instead')
const VersionNode$json = {
  '1': 'VersionNode',
  '2': [
    {'1': 'version', '3': 1, '4': 1, '5': 9, '10': 'version'},
    {'1': 'code', '3': 2, '4': 1, '5': 3, '10': 'code'},
    {'1': 'min_from', '3': 3, '4': 1, '5': 3, '10': 'minFrom'},
    {'1': 'yanked', '3': 4, '4': 1, '5': 8, '10': 'yanked'},
    {
      '1': 'manifest',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.rup.v2.DigestRef',
      '10': 'manifest'
    },
    {'1': 'released_at', '3': 6, '4': 1, '5': 9, '10': 'releasedAt'},
    {'1': 'notes', '3': 7, '4': 1, '5': 9, '10': 'notes'},
    {'1': 'notes_url', '3': 8, '4': 1, '5': 9, '10': 'notesUrl'},
  ],
};

/// Descriptor for `VersionNode`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List versionNodeDescriptor = $convert.base64Decode(
    'CgtWZXJzaW9uTm9kZRIYCgd2ZXJzaW9uGAEgASgJUgd2ZXJzaW9uEhIKBGNvZGUYAiABKANSBG'
    'NvZGUSGQoIbWluX2Zyb20YAyABKANSB21pbkZyb20SFgoGeWFua2VkGAQgASgIUgZ5YW5rZWQS'
    'LQoIbWFuaWZlc3QYBSABKAsyES5ydXAudjIuRGlnZXN0UmVmUghtYW5pZmVzdBIfCgtyZWxlYX'
    'NlZF9hdBgGIAEoCVIKcmVsZWFzZWRBdBIUCgVub3RlcxgHIAEoCVIFbm90ZXMSGwoJbm90ZXNf'
    'dXJsGAggASgJUghub3Rlc1VybA==');

@$core.Deprecated('Use indexDescriptor instead')
const Index$json = {
  '1': 'Index',
  '2': [
    {'1': 'schema', '3': 1, '4': 1, '5': 9, '10': 'schema'},
    {'1': 'product', '3': 2, '4': 1, '5': 9, '10': 'product'},
    {'1': 'channel', '3': 3, '4': 1, '5': 9, '10': 'channel'},
    {'1': 'sequence', '3': 4, '4': 1, '5': 3, '10': 'sequence'},
    {'1': 'generated_at', '3': 5, '4': 1, '5': 9, '10': 'generatedAt'},
    {'1': 'min_supported', '3': 6, '4': 1, '5': 3, '10': 'minSupported'},
    {'1': 'has_min_supported', '3': 7, '4': 1, '5': 8, '10': 'hasMinSupported'},
    {'1': 'expires_at', '3': 8, '4': 1, '5': 9, '10': 'expiresAt'},
    {
      '1': 'versions',
      '3': 9,
      '4': 3,
      '5': 11,
      '6': '.rup.v2.VersionNode',
      '10': 'versions'
    },
  ],
};

/// Descriptor for `Index`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List indexDescriptor = $convert.base64Decode(
    'CgVJbmRleBIWCgZzY2hlbWEYASABKAlSBnNjaGVtYRIYCgdwcm9kdWN0GAIgASgJUgdwcm9kdW'
    'N0EhgKB2NoYW5uZWwYAyABKAlSB2NoYW5uZWwSGgoIc2VxdWVuY2UYBCABKANSCHNlcXVlbmNl'
    'EiEKDGdlbmVyYXRlZF9hdBgFIAEoCVILZ2VuZXJhdGVkQXQSIwoNbWluX3N1cHBvcnRlZBgGIA'
    'EoA1IMbWluU3VwcG9ydGVkEioKEWhhc19taW5fc3VwcG9ydGVkGAcgASgIUg9oYXNNaW5TdXBw'
    'b3J0ZWQSHQoKZXhwaXJlc19hdBgIIAEoCVIJZXhwaXJlc0F0Ei8KCHZlcnNpb25zGAkgAygLMh'
    'MucnVwLnYyLlZlcnNpb25Ob2RlUgh2ZXJzaW9ucw==');

@$core.Deprecated('Use artifactDescriptor instead')
const Artifact$json = {
  '1': 'Artifact',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'filename', '3': 2, '4': 1, '5': 9, '10': 'filename'},
    {'1': 'size', '3': 3, '4': 1, '5': 3, '10': 'size'},
    {'1': 'sha256', '3': 4, '4': 1, '5': 9, '10': 'sha256'},
    {'1': 'kind', '3': 5, '4': 1, '5': 9, '10': 'kind'},
    {
      '1': 'selectors',
      '3': 6,
      '4': 3,
      '5': 11,
      '6': '.rup.v2.Selector',
      '10': 'selectors'
    },
    {'1': 'urls', '3': 7, '4': 3, '5': 9, '10': 'urls'},
    {
      '1': 'meta',
      '3': 8,
      '4': 3,
      '5': 11,
      '6': '.rup.v2.MetaEntry',
      '10': 'meta'
    },
  ],
};

/// Descriptor for `Artifact`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List artifactDescriptor = $convert.base64Decode(
    'CghBcnRpZmFjdBIOCgJpZBgBIAEoCVICaWQSGgoIZmlsZW5hbWUYAiABKAlSCGZpbGVuYW1lEh'
    'IKBHNpemUYAyABKANSBHNpemUSFgoGc2hhMjU2GAQgASgJUgZzaGEyNTYSEgoEa2luZBgFIAEo'
    'CVIEa2luZBIuCglzZWxlY3RvcnMYBiADKAsyEC5ydXAudjIuU2VsZWN0b3JSCXNlbGVjdG9ycx'
    'ISCgR1cmxzGAcgAygJUgR1cmxzEiUKBG1ldGEYCCADKAsyES5ydXAudjIuTWV0YUVudHJ5UgRt'
    'ZXRh');

@$core.Deprecated('Use manifestDescriptor instead')
const Manifest$json = {
  '1': 'Manifest',
  '2': [
    {'1': 'schema', '3': 1, '4': 1, '5': 9, '10': 'schema'},
    {'1': 'product', '3': 2, '4': 1, '5': 9, '10': 'product'},
    {'1': 'version', '3': 3, '4': 1, '5': 9, '10': 'version'},
    {'1': 'code', '3': 4, '4': 1, '5': 3, '10': 'code'},
    {'1': 'released_at', '3': 5, '4': 1, '5': 9, '10': 'releasedAt'},
    {'1': 'notes', '3': 6, '4': 1, '5': 9, '10': 'notes'},
    {
      '1': 'artifacts',
      '3': 7,
      '4': 3,
      '5': 11,
      '6': '.rup.v2.Artifact',
      '10': 'artifacts'
    },
  ],
};

/// Descriptor for `Manifest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List manifestDescriptor = $convert.base64Decode(
    'CghNYW5pZmVzdBIWCgZzY2hlbWEYASABKAlSBnNjaGVtYRIYCgdwcm9kdWN0GAIgASgJUgdwcm'
    '9kdWN0EhgKB3ZlcnNpb24YAyABKAlSB3ZlcnNpb24SEgoEY29kZRgEIAEoA1IEY29kZRIfCgty'
    'ZWxlYXNlZF9hdBgFIAEoCVIKcmVsZWFzZWRBdBIUCgVub3RlcxgGIAEoCVIFbm90ZXMSLgoJYX'
    'J0aWZhY3RzGAcgAygLMhAucnVwLnYyLkFydGlmYWN0UglhcnRpZmFjdHM=');

@$core.Deprecated('Use stagedArtifactDescriptor instead')
const StagedArtifact$json = {
  '1': 'StagedArtifact',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'filename', '3': 2, '4': 1, '5': 9, '10': 'filename'},
    {'1': 'size', '3': 3, '4': 1, '5': 3, '10': 'size'},
    {'1': 'sha256', '3': 4, '4': 1, '5': 9, '10': 'sha256'},
    {'1': 'kind', '3': 5, '4': 1, '5': 9, '10': 'kind'},
    {
      '1': 'selectors',
      '3': 6,
      '4': 3,
      '5': 11,
      '6': '.rup.v2.Selector',
      '10': 'selectors'
    },
    {
      '1': 'meta',
      '3': 7,
      '4': 3,
      '5': 11,
      '6': '.rup.v2.MetaEntry',
      '10': 'meta'
    },
    {'1': 'source_path', '3': 8, '4': 1, '5': 9, '10': 'sourcePath'},
  ],
};

/// Descriptor for `StagedArtifact`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List stagedArtifactDescriptor = $convert.base64Decode(
    'Cg5TdGFnZWRBcnRpZmFjdBIOCgJpZBgBIAEoCVICaWQSGgoIZmlsZW5hbWUYAiABKAlSCGZpbG'
    'VuYW1lEhIKBHNpemUYAyABKANSBHNpemUSFgoGc2hhMjU2GAQgASgJUgZzaGEyNTYSEgoEa2lu'
    'ZBgFIAEoCVIEa2luZBIuCglzZWxlY3RvcnMYBiADKAsyEC5ydXAudjIuU2VsZWN0b3JSCXNlbG'
    'VjdG9ycxIlCgRtZXRhGAcgAygLMhEucnVwLnYyLk1ldGFFbnRyeVIEbWV0YRIfCgtzb3VyY2Vf'
    'cGF0aBgIIAEoCVIKc291cmNlUGF0aA==');

@$core.Deprecated('Use stagedDescriptor instead')
const Staged$json = {
  '1': 'Staged',
  '2': [
    {'1': 'schema', '3': 1, '4': 1, '5': 9, '10': 'schema'},
    {'1': 'product', '3': 2, '4': 1, '5': 9, '10': 'product'},
    {'1': 'version', '3': 3, '4': 1, '5': 9, '10': 'version'},
    {'1': 'code', '3': 4, '4': 1, '5': 3, '10': 'code'},
    {'1': 'min_from', '3': 5, '4': 1, '5': 3, '10': 'minFrom'},
    {'1': 'channel', '3': 6, '4': 1, '5': 9, '10': 'channel'},
    {'1': 'created_at', '3': 7, '4': 1, '5': 9, '10': 'createdAt'},
    {'1': 'notes', '3': 8, '4': 1, '5': 9, '10': 'notes'},
    {
      '1': 'artifacts',
      '3': 9,
      '4': 3,
      '5': 11,
      '6': '.rup.v2.StagedArtifact',
      '10': 'artifacts'
    },
  ],
};

/// Descriptor for `Staged`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List stagedDescriptor = $convert.base64Decode(
    'CgZTdGFnZWQSFgoGc2NoZW1hGAEgASgJUgZzY2hlbWESGAoHcHJvZHVjdBgCIAEoCVIHcHJvZH'
    'VjdBIYCgd2ZXJzaW9uGAMgASgJUgd2ZXJzaW9uEhIKBGNvZGUYBCABKANSBGNvZGUSGQoIbWlu'
    'X2Zyb20YBSABKANSB21pbkZyb20SGAoHY2hhbm5lbBgGIAEoCVIHY2hhbm5lbBIdCgpjcmVhdG'
    'VkX2F0GAcgASgJUgljcmVhdGVkQXQSFAoFbm90ZXMYCCABKAlSBW5vdGVzEjQKCWFydGlmYWN0'
    'cxgJIAMoCzIWLnJ1cC52Mi5TdGFnZWRBcnRpZmFjdFIJYXJ0aWZhY3Rz');
