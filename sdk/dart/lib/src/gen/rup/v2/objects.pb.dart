// This is a generated file - do not edit.
//
// Generated from rup/v2/objects.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:fixnum/fixnum.dart' as $fixnum;
import 'package:protobuf/protobuf.dart' as $pb;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

/// Selector is one key/value match constraint.
/// Signed messages MUST NOT use map<>; encode helpers sort by key before Marshal.
class Selector extends $pb.GeneratedMessage {
  factory Selector({
    $core.String? key,
    $core.String? value,
  }) {
    final result = create();
    if (key != null) result.key = key;
    if (value != null) result.value = value;
    return result;
  }

  Selector._();

  factory Selector.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Selector.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Selector',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'key')
    ..aOS(2, _omitFieldNames ? '' : 'value')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Selector clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Selector copyWith(void Function(Selector) updates) =>
      super.copyWith((message) => updates(message as Selector)) as Selector;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Selector create() => Selector._();
  @$core.override
  Selector createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Selector getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Selector>(create);
  static Selector? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get key => $_getSZ(0);
  @$pb.TagNumber(1)
  set key($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearKey() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get value => $_getSZ(1);
  @$pb.TagNumber(2)
  set value($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasValue() => $_has(1);
  @$pb.TagNumber(2)
  void clearValue() => $_clearField(2);
}

/// MetaEntry is opaque host-defined metadata (string values only in v2).
class MetaEntry extends $pb.GeneratedMessage {
  factory MetaEntry({
    $core.String? key,
    $core.String? value,
  }) {
    final result = create();
    if (key != null) result.key = key;
    if (value != null) result.value = value;
    return result;
  }

  MetaEntry._();

  factory MetaEntry.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory MetaEntry.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'MetaEntry',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'key')
    ..aOS(2, _omitFieldNames ? '' : 'value')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MetaEntry clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MetaEntry copyWith(void Function(MetaEntry) updates) =>
      super.copyWith((message) => updates(message as MetaEntry)) as MetaEntry;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static MetaEntry create() => MetaEntry._();
  @$core.override
  MetaEntry createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static MetaEntry getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<MetaEntry>(create);
  static MetaEntry? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get key => $_getSZ(0);
  @$pb.TagNumber(1)
  set key($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearKey() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get value => $_getSZ(1);
  @$pb.TagNumber(2)
  set value($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasValue() => $_has(1);
  @$pb.TagNumber(2)
  void clearValue() => $_clearField(2);
}

/// DigestRef points at immutable bytes with mirrors.
class DigestRef extends $pb.GeneratedMessage {
  factory DigestRef({
    $core.String? sha256,
    $fixnum.Int64? size,
    $core.Iterable<$core.String>? urls,
  }) {
    final result = create();
    if (sha256 != null) result.sha256 = sha256;
    if (size != null) result.size = size;
    if (urls != null) result.urls.addAll(urls);
    return result;
  }

  DigestRef._();

  factory DigestRef.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DigestRef.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DigestRef',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'sha256')
    ..aInt64(2, _omitFieldNames ? '' : 'size')
    ..pPS(3, _omitFieldNames ? '' : 'urls')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DigestRef clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DigestRef copyWith(void Function(DigestRef) updates) =>
      super.copyWith((message) => updates(message as DigestRef)) as DigestRef;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DigestRef create() => DigestRef._();
  @$core.override
  DigestRef createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DigestRef getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<DigestRef>(create);
  static DigestRef? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sha256 => $_getSZ(0);
  @$pb.TagNumber(1)
  set sha256($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSha256() => $_has(0);
  @$pb.TagNumber(1)
  void clearSha256() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get size => $_getI64(1);
  @$pb.TagNumber(2)
  set size($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSize() => $_has(1);
  @$pb.TagNumber(2)
  void clearSize() => $_clearField(2);

  @$pb.TagNumber(3)
  $pb.PbList<$core.String> get urls => $_getList(2);
}

/// VersionNode is one release in an Index chain.
class VersionNode extends $pb.GeneratedMessage {
  factory VersionNode({
    $core.String? version,
    $fixnum.Int64? code,
    $fixnum.Int64? minFrom,
    $core.bool? yanked,
    DigestRef? manifest,
    $core.String? releasedAt,
    $core.String? notes,
    $core.String? notesUrl,
  }) {
    final result = create();
    if (version != null) result.version = version;
    if (code != null) result.code = code;
    if (minFrom != null) result.minFrom = minFrom;
    if (yanked != null) result.yanked = yanked;
    if (manifest != null) result.manifest = manifest;
    if (releasedAt != null) result.releasedAt = releasedAt;
    if (notes != null) result.notes = notes;
    if (notesUrl != null) result.notesUrl = notesUrl;
    return result;
  }

  VersionNode._();

  factory VersionNode.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory VersionNode.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'VersionNode',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'version')
    ..aInt64(2, _omitFieldNames ? '' : 'code')
    ..aInt64(3, _omitFieldNames ? '' : 'minFrom')
    ..aOB(4, _omitFieldNames ? '' : 'yanked')
    ..aOM<DigestRef>(5, _omitFieldNames ? '' : 'manifest',
        subBuilder: DigestRef.create)
    ..aOS(6, _omitFieldNames ? '' : 'releasedAt')
    ..aOS(7, _omitFieldNames ? '' : 'notes')
    ..aOS(8, _omitFieldNames ? '' : 'notesUrl')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VersionNode clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VersionNode copyWith(void Function(VersionNode) updates) =>
      super.copyWith((message) => updates(message as VersionNode))
          as VersionNode;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static VersionNode create() => VersionNode._();
  @$core.override
  VersionNode createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static VersionNode getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<VersionNode>(create);
  static VersionNode? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get version => $_getSZ(0);
  @$pb.TagNumber(1)
  set version($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasVersion() => $_has(0);
  @$pb.TagNumber(1)
  void clearVersion() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get code => $_getI64(1);
  @$pb.TagNumber(2)
  set code($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCode() => $_has(1);
  @$pb.TagNumber(2)
  void clearCode() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get minFrom => $_getI64(2);
  @$pb.TagNumber(3)
  set minFrom($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMinFrom() => $_has(2);
  @$pb.TagNumber(3)
  void clearMinFrom() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get yanked => $_getBF(3);
  @$pb.TagNumber(4)
  set yanked($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasYanked() => $_has(3);
  @$pb.TagNumber(4)
  void clearYanked() => $_clearField(4);

  @$pb.TagNumber(5)
  DigestRef get manifest => $_getN(4);
  @$pb.TagNumber(5)
  set manifest(DigestRef value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasManifest() => $_has(4);
  @$pb.TagNumber(5)
  void clearManifest() => $_clearField(5);
  @$pb.TagNumber(5)
  DigestRef ensureManifest() => $_ensure(4);

  @$pb.TagNumber(6)
  $core.String get releasedAt => $_getSZ(5);
  @$pb.TagNumber(6)
  set releasedAt($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasReleasedAt() => $_has(5);
  @$pb.TagNumber(6)
  void clearReleasedAt() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get notes => $_getSZ(6);
  @$pb.TagNumber(7)
  set notes($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasNotes() => $_has(6);
  @$pb.TagNumber(7)
  void clearNotes() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get notesUrl => $_getSZ(7);
  @$pb.TagNumber(8)
  set notesUrl($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasNotesUrl() => $_has(7);
  @$pb.TagNumber(8)
  void clearNotesUrl() => $_clearField(8);
}

/// Index is the signed channel pointer (schema rup.index/2).
class Index extends $pb.GeneratedMessage {
  factory Index({
    $core.String? schema,
    $core.String? product,
    $core.String? channel,
    $fixnum.Int64? sequence,
    $core.String? generatedAt,
    $fixnum.Int64? minSupported,
    $core.bool? hasMinSupported_7,
    $core.String? expiresAt,
    $core.Iterable<VersionNode>? versions,
  }) {
    final result = create();
    if (schema != null) result.schema = schema;
    if (product != null) result.product = product;
    if (channel != null) result.channel = channel;
    if (sequence != null) result.sequence = sequence;
    if (generatedAt != null) result.generatedAt = generatedAt;
    if (minSupported != null) result.minSupported = minSupported;
    if (hasMinSupported_7 != null) result.hasMinSupported_7 = hasMinSupported_7;
    if (expiresAt != null) result.expiresAt = expiresAt;
    if (versions != null) result.versions.addAll(versions);
    return result;
  }

  Index._();

  factory Index.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Index.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Index',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'schema')
    ..aOS(2, _omitFieldNames ? '' : 'product')
    ..aOS(3, _omitFieldNames ? '' : 'channel')
    ..aInt64(4, _omitFieldNames ? '' : 'sequence')
    ..aOS(5, _omitFieldNames ? '' : 'generatedAt')
    ..aInt64(6, _omitFieldNames ? '' : 'minSupported')
    ..aOB(7, _omitFieldNames ? '' : 'hasMinSupported')
    ..aOS(8, _omitFieldNames ? '' : 'expiresAt')
    ..pPM<VersionNode>(9, _omitFieldNames ? '' : 'versions',
        subBuilder: VersionNode.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Index clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Index copyWith(void Function(Index) updates) =>
      super.copyWith((message) => updates(message as Index)) as Index;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Index create() => Index._();
  @$core.override
  Index createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Index getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Index>(create);
  static Index? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get schema => $_getSZ(0);
  @$pb.TagNumber(1)
  set schema($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSchema() => $_has(0);
  @$pb.TagNumber(1)
  void clearSchema() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get product => $_getSZ(1);
  @$pb.TagNumber(2)
  set product($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasProduct() => $_has(1);
  @$pb.TagNumber(2)
  void clearProduct() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get channel => $_getSZ(2);
  @$pb.TagNumber(3)
  set channel($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasChannel() => $_has(2);
  @$pb.TagNumber(3)
  void clearChannel() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get sequence => $_getI64(3);
  @$pb.TagNumber(4)
  set sequence($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSequence() => $_has(3);
  @$pb.TagNumber(4)
  void clearSequence() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get generatedAt => $_getSZ(4);
  @$pb.TagNumber(5)
  set generatedAt($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasGeneratedAt() => $_has(4);
  @$pb.TagNumber(5)
  void clearGeneratedAt() => $_clearField(5);

  @$pb.TagNumber(6)
  $fixnum.Int64 get minSupported => $_getI64(5);
  @$pb.TagNumber(6)
  set minSupported($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(6)
  $core.bool hasMinSupported() => $_has(5);
  @$pb.TagNumber(6)
  void clearMinSupported() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get hasMinSupported_7 => $_getBF(6);
  @$pb.TagNumber(7)
  set hasMinSupported_7($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasHasMinSupported_7() => $_has(6);
  @$pb.TagNumber(7)
  void clearHasMinSupported_7() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get expiresAt => $_getSZ(7);
  @$pb.TagNumber(8)
  set expiresAt($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasExpiresAt() => $_has(7);
  @$pb.TagNumber(8)
  void clearExpiresAt() => $_clearField(8);

  @$pb.TagNumber(9)
  $pb.PbList<VersionNode> get versions => $_getList(8);
}

/// Artifact describes one downloadable file inside a Manifest.
class Artifact extends $pb.GeneratedMessage {
  factory Artifact({
    $core.String? id,
    $core.String? filename,
    $fixnum.Int64? size,
    $core.String? sha256,
    $core.String? kind,
    $core.Iterable<Selector>? selectors,
    $core.Iterable<$core.String>? urls,
    $core.Iterable<MetaEntry>? meta,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (filename != null) result.filename = filename;
    if (size != null) result.size = size;
    if (sha256 != null) result.sha256 = sha256;
    if (kind != null) result.kind = kind;
    if (selectors != null) result.selectors.addAll(selectors);
    if (urls != null) result.urls.addAll(urls);
    if (meta != null) result.meta.addAll(meta);
    return result;
  }

  Artifact._();

  factory Artifact.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Artifact.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Artifact',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOS(2, _omitFieldNames ? '' : 'filename')
    ..aInt64(3, _omitFieldNames ? '' : 'size')
    ..aOS(4, _omitFieldNames ? '' : 'sha256')
    ..aOS(5, _omitFieldNames ? '' : 'kind')
    ..pPM<Selector>(6, _omitFieldNames ? '' : 'selectors',
        subBuilder: Selector.create)
    ..pPS(7, _omitFieldNames ? '' : 'urls')
    ..pPM<MetaEntry>(8, _omitFieldNames ? '' : 'meta',
        subBuilder: MetaEntry.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Artifact clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Artifact copyWith(void Function(Artifact) updates) =>
      super.copyWith((message) => updates(message as Artifact)) as Artifact;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Artifact create() => Artifact._();
  @$core.override
  Artifact createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Artifact getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Artifact>(create);
  static Artifact? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get filename => $_getSZ(1);
  @$pb.TagNumber(2)
  set filename($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasFilename() => $_has(1);
  @$pb.TagNumber(2)
  void clearFilename() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get size => $_getI64(2);
  @$pb.TagNumber(3)
  set size($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSize() => $_has(2);
  @$pb.TagNumber(3)
  void clearSize() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get sha256 => $_getSZ(3);
  @$pb.TagNumber(4)
  set sha256($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSha256() => $_has(3);
  @$pb.TagNumber(4)
  void clearSha256() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get kind => $_getSZ(4);
  @$pb.TagNumber(5)
  set kind($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasKind() => $_has(4);
  @$pb.TagNumber(5)
  void clearKind() => $_clearField(5);

  @$pb.TagNumber(6)
  $pb.PbList<Selector> get selectors => $_getList(5);

  @$pb.TagNumber(7)
  $pb.PbList<$core.String> get urls => $_getList(6);

  @$pb.TagNumber(8)
  $pb.PbList<MetaEntry> get meta => $_getList(7);
}

/// Manifest describes all artifacts for one version (schema rup.manifest/2).
class Manifest extends $pb.GeneratedMessage {
  factory Manifest({
    $core.String? schema,
    $core.String? product,
    $core.String? version,
    $fixnum.Int64? code,
    $core.String? releasedAt,
    $core.String? notes,
    $core.Iterable<Artifact>? artifacts,
  }) {
    final result = create();
    if (schema != null) result.schema = schema;
    if (product != null) result.product = product;
    if (version != null) result.version = version;
    if (code != null) result.code = code;
    if (releasedAt != null) result.releasedAt = releasedAt;
    if (notes != null) result.notes = notes;
    if (artifacts != null) result.artifacts.addAll(artifacts);
    return result;
  }

  Manifest._();

  factory Manifest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Manifest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Manifest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'schema')
    ..aOS(2, _omitFieldNames ? '' : 'product')
    ..aOS(3, _omitFieldNames ? '' : 'version')
    ..aInt64(4, _omitFieldNames ? '' : 'code')
    ..aOS(5, _omitFieldNames ? '' : 'releasedAt')
    ..aOS(6, _omitFieldNames ? '' : 'notes')
    ..pPM<Artifact>(7, _omitFieldNames ? '' : 'artifacts',
        subBuilder: Artifact.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Manifest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Manifest copyWith(void Function(Manifest) updates) =>
      super.copyWith((message) => updates(message as Manifest)) as Manifest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Manifest create() => Manifest._();
  @$core.override
  Manifest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Manifest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Manifest>(create);
  static Manifest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get schema => $_getSZ(0);
  @$pb.TagNumber(1)
  set schema($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSchema() => $_has(0);
  @$pb.TagNumber(1)
  void clearSchema() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get product => $_getSZ(1);
  @$pb.TagNumber(2)
  set product($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasProduct() => $_has(1);
  @$pb.TagNumber(2)
  void clearProduct() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get version => $_getSZ(2);
  @$pb.TagNumber(3)
  set version($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasVersion() => $_has(2);
  @$pb.TagNumber(3)
  void clearVersion() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get code => $_getI64(3);
  @$pb.TagNumber(4)
  set code($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasCode() => $_has(3);
  @$pb.TagNumber(4)
  void clearCode() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get releasedAt => $_getSZ(4);
  @$pb.TagNumber(5)
  set releasedAt($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasReleasedAt() => $_has(4);
  @$pb.TagNumber(5)
  void clearReleasedAt() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get notes => $_getSZ(5);
  @$pb.TagNumber(6)
  set notes($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasNotes() => $_has(5);
  @$pb.TagNumber(6)
  void clearNotes() => $_clearField(6);

  @$pb.TagNumber(7)
  $pb.PbList<Artifact> get artifacts => $_getList(6);
}

/// StagedArtifact is local-only; never published with urls.
class StagedArtifact extends $pb.GeneratedMessage {
  factory StagedArtifact({
    $core.String? id,
    $core.String? filename,
    $fixnum.Int64? size,
    $core.String? sha256,
    $core.String? kind,
    $core.Iterable<Selector>? selectors,
    $core.Iterable<MetaEntry>? meta,
    $core.String? sourcePath,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (filename != null) result.filename = filename;
    if (size != null) result.size = size;
    if (sha256 != null) result.sha256 = sha256;
    if (kind != null) result.kind = kind;
    if (selectors != null) result.selectors.addAll(selectors);
    if (meta != null) result.meta.addAll(meta);
    if (sourcePath != null) result.sourcePath = sourcePath;
    return result;
  }

  StagedArtifact._();

  factory StagedArtifact.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory StagedArtifact.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StagedArtifact',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOS(2, _omitFieldNames ? '' : 'filename')
    ..aInt64(3, _omitFieldNames ? '' : 'size')
    ..aOS(4, _omitFieldNames ? '' : 'sha256')
    ..aOS(5, _omitFieldNames ? '' : 'kind')
    ..pPM<Selector>(6, _omitFieldNames ? '' : 'selectors',
        subBuilder: Selector.create)
    ..pPM<MetaEntry>(7, _omitFieldNames ? '' : 'meta',
        subBuilder: MetaEntry.create)
    ..aOS(8, _omitFieldNames ? '' : 'sourcePath')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StagedArtifact clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StagedArtifact copyWith(void Function(StagedArtifact) updates) =>
      super.copyWith((message) => updates(message as StagedArtifact))
          as StagedArtifact;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static StagedArtifact create() => StagedArtifact._();
  @$core.override
  StagedArtifact createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static StagedArtifact getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<StagedArtifact>(create);
  static StagedArtifact? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get filename => $_getSZ(1);
  @$pb.TagNumber(2)
  set filename($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasFilename() => $_has(1);
  @$pb.TagNumber(2)
  void clearFilename() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get size => $_getI64(2);
  @$pb.TagNumber(3)
  set size($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSize() => $_has(2);
  @$pb.TagNumber(3)
  void clearSize() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get sha256 => $_getSZ(3);
  @$pb.TagNumber(4)
  set sha256($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSha256() => $_has(3);
  @$pb.TagNumber(4)
  void clearSha256() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get kind => $_getSZ(4);
  @$pb.TagNumber(5)
  set kind($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasKind() => $_has(4);
  @$pb.TagNumber(5)
  void clearKind() => $_clearField(5);

  @$pb.TagNumber(6)
  $pb.PbList<Selector> get selectors => $_getList(5);

  @$pb.TagNumber(7)
  $pb.PbList<MetaEntry> get meta => $_getList(6);

  @$pb.TagNumber(8)
  $core.String get sourcePath => $_getSZ(7);
  @$pb.TagNumber(8)
  set sourcePath($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasSourcePath() => $_has(7);
  @$pb.TagNumber(8)
  void clearSourcePath() => $_clearField(8);
}

/// Staged is the offline stage product (schema rup.staged/2).
class Staged extends $pb.GeneratedMessage {
  factory Staged({
    $core.String? schema,
    $core.String? product,
    $core.String? version,
    $fixnum.Int64? code,
    $fixnum.Int64? minFrom,
    $core.String? channel,
    $core.String? createdAt,
    $core.String? notes,
    $core.Iterable<StagedArtifact>? artifacts,
  }) {
    final result = create();
    if (schema != null) result.schema = schema;
    if (product != null) result.product = product;
    if (version != null) result.version = version;
    if (code != null) result.code = code;
    if (minFrom != null) result.minFrom = minFrom;
    if (channel != null) result.channel = channel;
    if (createdAt != null) result.createdAt = createdAt;
    if (notes != null) result.notes = notes;
    if (artifacts != null) result.artifacts.addAll(artifacts);
    return result;
  }

  Staged._();

  factory Staged.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Staged.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Staged',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'schema')
    ..aOS(2, _omitFieldNames ? '' : 'product')
    ..aOS(3, _omitFieldNames ? '' : 'version')
    ..aInt64(4, _omitFieldNames ? '' : 'code')
    ..aInt64(5, _omitFieldNames ? '' : 'minFrom')
    ..aOS(6, _omitFieldNames ? '' : 'channel')
    ..aOS(7, _omitFieldNames ? '' : 'createdAt')
    ..aOS(8, _omitFieldNames ? '' : 'notes')
    ..pPM<StagedArtifact>(9, _omitFieldNames ? '' : 'artifacts',
        subBuilder: StagedArtifact.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Staged clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Staged copyWith(void Function(Staged) updates) =>
      super.copyWith((message) => updates(message as Staged)) as Staged;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Staged create() => Staged._();
  @$core.override
  Staged createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Staged getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Staged>(create);
  static Staged? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get schema => $_getSZ(0);
  @$pb.TagNumber(1)
  set schema($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSchema() => $_has(0);
  @$pb.TagNumber(1)
  void clearSchema() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get product => $_getSZ(1);
  @$pb.TagNumber(2)
  set product($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasProduct() => $_has(1);
  @$pb.TagNumber(2)
  void clearProduct() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get version => $_getSZ(2);
  @$pb.TagNumber(3)
  set version($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasVersion() => $_has(2);
  @$pb.TagNumber(3)
  void clearVersion() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get code => $_getI64(3);
  @$pb.TagNumber(4)
  set code($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasCode() => $_has(3);
  @$pb.TagNumber(4)
  void clearCode() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get minFrom => $_getI64(4);
  @$pb.TagNumber(5)
  set minFrom($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasMinFrom() => $_has(4);
  @$pb.TagNumber(5)
  void clearMinFrom() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get channel => $_getSZ(5);
  @$pb.TagNumber(6)
  set channel($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasChannel() => $_has(5);
  @$pb.TagNumber(6)
  void clearChannel() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get createdAt => $_getSZ(6);
  @$pb.TagNumber(7)
  set createdAt($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasCreatedAt() => $_has(6);
  @$pb.TagNumber(7)
  void clearCreatedAt() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get notes => $_getSZ(7);
  @$pb.TagNumber(8)
  set notes($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasNotes() => $_has(7);
  @$pb.TagNumber(8)
  void clearNotes() => $_clearField(8);

  @$pb.TagNumber(9)
  $pb.PbList<StagedArtifact> get artifacts => $_getList(8);
}

/// FallbackRule matches a client code range and urges a manual update.
/// Match when min_code <= currentCode <= max_code (inclusive).
class FallbackRule extends $pb.GeneratedMessage {
  factory FallbackRule({
    $fixnum.Int64? minCode,
    $fixnum.Int64? maxCode,
    $core.String? manualUrl,
    $core.String? message,
    $core.bool? mandatory,
    $core.Iterable<Selector>? selectors,
  }) {
    final result = create();
    if (minCode != null) result.minCode = minCode;
    if (maxCode != null) result.maxCode = maxCode;
    if (manualUrl != null) result.manualUrl = manualUrl;
    if (message != null) result.message = message;
    if (mandatory != null) result.mandatory = mandatory;
    if (selectors != null) result.selectors.addAll(selectors);
    return result;
  }

  FallbackRule._();

  factory FallbackRule.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FallbackRule.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FallbackRule',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'minCode')
    ..aInt64(2, _omitFieldNames ? '' : 'maxCode')
    ..aOS(3, _omitFieldNames ? '' : 'manualUrl')
    ..aOS(4, _omitFieldNames ? '' : 'message')
    ..aOB(5, _omitFieldNames ? '' : 'mandatory')
    ..pPM<Selector>(6, _omitFieldNames ? '' : 'selectors',
        subBuilder: Selector.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FallbackRule clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FallbackRule copyWith(void Function(FallbackRule) updates) =>
      super.copyWith((message) => updates(message as FallbackRule))
          as FallbackRule;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FallbackRule create() => FallbackRule._();
  @$core.override
  FallbackRule createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FallbackRule getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FallbackRule>(create);
  static FallbackRule? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get minCode => $_getI64(0);
  @$pb.TagNumber(1)
  set minCode($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasMinCode() => $_has(0);
  @$pb.TagNumber(1)
  void clearMinCode() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get maxCode => $_getI64(1);
  @$pb.TagNumber(2)
  set maxCode($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasMaxCode() => $_has(1);
  @$pb.TagNumber(2)
  void clearMaxCode() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get manualUrl => $_getSZ(2);
  @$pb.TagNumber(3)
  set manualUrl($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasManualUrl() => $_has(2);
  @$pb.TagNumber(3)
  void clearManualUrl() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get message => $_getSZ(3);
  @$pb.TagNumber(4)
  set message($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasMessage() => $_has(3);
  @$pb.TagNumber(4)
  void clearMessage() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get mandatory => $_getBF(4);
  @$pb.TagNumber(5)
  set mandatory($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasMandatory() => $_has(4);
  @$pb.TagNumber(5)
  void clearMandatory() => $_clearField(5);

  @$pb.TagNumber(6)
  $pb.PbList<Selector> get selectors => $_getList(5);
}

/// Fallback is a signed emergency notice (schema rup.fallback/2).
/// Logical key: fallback/<product>.pb (product-scoped, not channel).
class Fallback extends $pb.GeneratedMessage {
  factory Fallback({
    $core.String? schema,
    $core.String? product,
    $fixnum.Int64? sequence,
    $core.String? generatedAt,
    $core.Iterable<FallbackRule>? rules,
  }) {
    final result = create();
    if (schema != null) result.schema = schema;
    if (product != null) result.product = product;
    if (sequence != null) result.sequence = sequence;
    if (generatedAt != null) result.generatedAt = generatedAt;
    if (rules != null) result.rules.addAll(rules);
    return result;
  }

  Fallback._();

  factory Fallback.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Fallback.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Fallback',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'rup.v2'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'schema')
    ..aOS(2, _omitFieldNames ? '' : 'product')
    ..aInt64(3, _omitFieldNames ? '' : 'sequence')
    ..aOS(4, _omitFieldNames ? '' : 'generatedAt')
    ..pPM<FallbackRule>(5, _omitFieldNames ? '' : 'rules',
        subBuilder: FallbackRule.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Fallback clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Fallback copyWith(void Function(Fallback) updates) =>
      super.copyWith((message) => updates(message as Fallback)) as Fallback;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Fallback create() => Fallback._();
  @$core.override
  Fallback createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Fallback getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Fallback>(create);
  static Fallback? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get schema => $_getSZ(0);
  @$pb.TagNumber(1)
  set schema($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSchema() => $_has(0);
  @$pb.TagNumber(1)
  void clearSchema() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get product => $_getSZ(1);
  @$pb.TagNumber(2)
  set product($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasProduct() => $_has(1);
  @$pb.TagNumber(2)
  void clearProduct() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get sequence => $_getI64(2);
  @$pb.TagNumber(3)
  set sequence($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSequence() => $_has(2);
  @$pb.TagNumber(3)
  void clearSequence() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get generatedAt => $_getSZ(3);
  @$pb.TagNumber(4)
  set generatedAt($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasGeneratedAt() => $_has(3);
  @$pb.TagNumber(4)
  void clearGeneratedAt() => $_clearField(4);

  @$pb.TagNumber(5)
  $pb.PbList<FallbackRule> get rules => $_getList(4);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
