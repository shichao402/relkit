// This is a generated file - do not edit.
//
// Generated from rup/v2/envelope.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

/// Signature is one Ed25519 signature over Envelope.payload bytes.
class Signature extends $pb.GeneratedMessage {
  factory Signature({
    $core.String? keyId,
    $core.String? alg,
    $core.List<$core.int>? sig,
  }) {
    final result = create();
    if (keyId != null) result.keyId = keyId;
    if (alg != null) result.alg = alg;
    if (sig != null) result.sig = sig;
    return result;
  }

  Signature._();

  factory Signature.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Signature.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Signature',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'keyId')
    ..aOS(2, _omitFieldNames ? '' : 'alg')
    ..a<$core.List<$core.int>>(
        3, _omitFieldNames ? '' : 'sig', $pb.PbFieldType.OY)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Signature clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Signature copyWith(void Function(Signature) updates) =>
      super.copyWith((message) => updates(message as Signature)) as Signature;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Signature create() => Signature._();
  @$core.override
  Signature createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Signature getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Signature>(create);
  static Signature? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get keyId => $_getSZ(0);
  @$pb.TagNumber(1)
  set keyId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasKeyId() => $_has(0);
  @$pb.TagNumber(1)
  void clearKeyId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get alg => $_getSZ(1);
  @$pb.TagNumber(2)
  set alg($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasAlg() => $_has(1);
  @$pb.TagNumber(2)
  void clearAlg() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.List<$core.int> get sig => $_getN(2);
  @$pb.TagNumber(3)
  set sig($core.List<$core.int> value) => $_setBytes(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSig() => $_has(2);
  @$pb.TagNumber(3)
  void clearSig() => $_clearField(3);
}

/// Envelope wraps a signed payload on the wire (schema rup.envelope/2).
/// payload is the protobuf serialization of Index or Fallback (not JSON).
/// The client knows which message to parse from the logical key / URL it fetched.
class Envelope extends $pb.GeneratedMessage {
  factory Envelope({
    $core.String? schema,
    $core.List<$core.int>? payload,
    $core.Iterable<Signature>? signatures,
  }) {
    final result = create();
    if (schema != null) result.schema = schema;
    if (payload != null) result.payload = payload;
    if (signatures != null) result.signatures.addAll(signatures);
    return result;
  }

  Envelope._();

  factory Envelope.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Envelope.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Envelope',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'schema')
    ..a<$core.List<$core.int>>(
        2, _omitFieldNames ? '' : 'payload', $pb.PbFieldType.OY)
    ..pPM<Signature>(3, _omitFieldNames ? '' : 'signatures',
        subBuilder: Signature.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Envelope clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Envelope copyWith(void Function(Envelope) updates) =>
      super.copyWith((message) => updates(message as Envelope)) as Envelope;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Envelope create() => Envelope._();
  @$core.override
  Envelope createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Envelope getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Envelope>(create);
  static Envelope? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get schema => $_getSZ(0);
  @$pb.TagNumber(1)
  set schema($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSchema() => $_has(0);
  @$pb.TagNumber(1)
  void clearSchema() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.List<$core.int> get payload => $_getN(1);
  @$pb.TagNumber(2)
  set payload($core.List<$core.int> value) => $_setBytes(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPayload() => $_has(1);
  @$pb.TagNumber(2)
  void clearPayload() => $_clearField(2);

  @$pb.TagNumber(3)
  $pb.PbList<Signature> get signatures => $_getList(2);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
