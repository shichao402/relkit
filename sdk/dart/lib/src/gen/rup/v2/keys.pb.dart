// This is a generated file - do not edit.
//
// Generated from rup/v2/keys.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

/// PublicKeyDocument is the on-disk / embeddable public key record.
class PublicKeyDocument extends $pb.GeneratedMessage {
  factory PublicKeyDocument({
    $core.String? schema,
    $core.String? keyId,
    $core.String? alg,
    $core.List<$core.int>? publicKey,
  }) {
    final result = create();
    if (schema != null) result.schema = schema;
    if (keyId != null) result.keyId = keyId;
    if (alg != null) result.alg = alg;
    if (publicKey != null) result.publicKey = publicKey;
    return result;
  }

  PublicKeyDocument._();

  factory PublicKeyDocument.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PublicKeyDocument.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PublicKeyDocument',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'schema')
    ..aOS(2, _omitFieldNames ? '' : 'keyId')
    ..aOS(3, _omitFieldNames ? '' : 'alg')
    ..a<$core.List<$core.int>>(
        4, _omitFieldNames ? '' : 'publicKey', $pb.PbFieldType.OY)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PublicKeyDocument clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PublicKeyDocument copyWith(void Function(PublicKeyDocument) updates) =>
      super.copyWith((message) => updates(message as PublicKeyDocument))
          as PublicKeyDocument;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PublicKeyDocument create() => PublicKeyDocument._();
  @$core.override
  PublicKeyDocument createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PublicKeyDocument getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PublicKeyDocument>(create);
  static PublicKeyDocument? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get schema => $_getSZ(0);
  @$pb.TagNumber(1)
  set schema($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSchema() => $_has(0);
  @$pb.TagNumber(1)
  void clearSchema() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get keyId => $_getSZ(1);
  @$pb.TagNumber(2)
  set keyId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasKeyId() => $_has(1);
  @$pb.TagNumber(2)
  void clearKeyId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get alg => $_getSZ(2);
  @$pb.TagNumber(3)
  set alg($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasAlg() => $_has(2);
  @$pb.TagNumber(3)
  void clearAlg() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.List<$core.int> get publicKey => $_getN(3);
  @$pb.TagNumber(4)
  set publicKey($core.List<$core.int> value) => $_setBytes(3, value);
  @$pb.TagNumber(4)
  $core.bool hasPublicKey() => $_has(3);
  @$pb.TagNumber(4)
  void clearPublicKey() => $_clearField(4);
}

/// PrivateKeyDocument is local-only; never publish.
class PrivateKeyDocument extends $pb.GeneratedMessage {
  factory PrivateKeyDocument({
    $core.String? schema,
    $core.String? keyId,
    $core.String? alg,
    $core.List<$core.int>? seed,
  }) {
    final result = create();
    if (schema != null) result.schema = schema;
    if (keyId != null) result.keyId = keyId;
    if (alg != null) result.alg = alg;
    if (seed != null) result.seed = seed;
    return result;
  }

  PrivateKeyDocument._();

  factory PrivateKeyDocument.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PrivateKeyDocument.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PrivateKeyDocument',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'schema')
    ..aOS(2, _omitFieldNames ? '' : 'keyId')
    ..aOS(3, _omitFieldNames ? '' : 'alg')
    ..a<$core.List<$core.int>>(
        4, _omitFieldNames ? '' : 'seed', $pb.PbFieldType.OY)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PrivateKeyDocument clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PrivateKeyDocument copyWith(void Function(PrivateKeyDocument) updates) =>
      super.copyWith((message) => updates(message as PrivateKeyDocument))
          as PrivateKeyDocument;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PrivateKeyDocument create() => PrivateKeyDocument._();
  @$core.override
  PrivateKeyDocument createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PrivateKeyDocument getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PrivateKeyDocument>(create);
  static PrivateKeyDocument? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get schema => $_getSZ(0);
  @$pb.TagNumber(1)
  set schema($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSchema() => $_has(0);
  @$pb.TagNumber(1)
  void clearSchema() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get keyId => $_getSZ(1);
  @$pb.TagNumber(2)
  set keyId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasKeyId() => $_has(1);
  @$pb.TagNumber(2)
  void clearKeyId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get alg => $_getSZ(2);
  @$pb.TagNumber(3)
  set alg($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasAlg() => $_has(2);
  @$pb.TagNumber(3)
  void clearAlg() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.List<$core.int> get seed => $_getN(3);
  @$pb.TagNumber(4)
  set seed($core.List<$core.int> value) => $_setBytes(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSeed() => $_has(3);
  @$pb.TagNumber(4)
  void clearSeed() => $_clearField(4);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
