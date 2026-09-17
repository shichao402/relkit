// This is a generated file - do not edit.
//
// Generated from updater/v1/payload.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:fixnum/fixnum.dart' as $fixnum;
import 'package:protobuf/protobuf.dart' as $pb;
import 'package:protobuf/well_known_types/google/protobuf/duration.pb.dart'
    as $0;

import 'payload.pbenum.dart';

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

export 'payload.pbenum.dart';

/// FileTable is stored as files.pb at the root of a payload artifact.
/// It is the complete set of files owned by this release. Deletions are derived
/// by comparing this table with the last successfully applied baseline.
class FileTable extends $pb.GeneratedMessage {
  factory FileTable({
    $core.String? schema,
    $core.Iterable<FileEntry>? files,
    $core.Iterable<ScriptEntry>? scripts,
    $core.Iterable<$core.String>? preserve,
  }) {
    final result = FileTable._();
    if (schema != null) result.schema = schema;
    if (files != null) result.files.addAll(files);
    if (scripts != null) result.scripts.addAll(scripts);
    if (preserve != null) result.preserve.addAll(preserve);
    return result;
  }

  FileTable._();

  factory FileTable.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileTable()..mergeFromBuffer(data, registry);
  factory FileTable.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileTable()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FileTable',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: FileTable.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'schema')
    ..pPM<FileEntry>(2, _omitFieldNames ? '' : 'files',
        subBuilder: FileEntry.$_createMessage)
    ..pPM<ScriptEntry>(3, _omitFieldNames ? '' : 'scripts',
        subBuilder: ScriptEntry.$_createMessage)
    ..pPS(4, _omitFieldNames ? '' : 'preserve')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileTable clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileTable copyWith(void Function(FileTable) updates) =>
      super.copyWith((message) => updates(message as FileTable)) as FileTable;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use FileTable() / FileTable.new instead')
  static FileTable create() => FileTable._();
  static $pb.GeneratedMessage $_createMessage() => FileTable._();
  @$core.override
  FileTable createEmptyInstance() => FileTable._();
  @$core.pragma('dart2js:noInline')
  static FileTable getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FileTable>(FileTable.$_createMessage);
  static FileTable? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get schema => $_getSZ(0);
  @$pb.TagNumber(1)
  set schema($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSchema() => $_has(0);
  @$pb.TagNumber(1)
  void clearSchema() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<FileEntry> get files => $_getList(1);

  @$pb.TagNumber(3)
  $pb.PbList<ScriptEntry> get scripts => $_getList(2);

  @$pb.TagNumber(4)
  $pb.PbList<$core.String> get preserve => $_getList(3);
}

class FileEntry extends $pb.GeneratedMessage {
  factory FileEntry({
    $core.String? relpath,
    $fixnum.Int64? size,
    $core.String? sha256,
    $core.int? mode,
  }) {
    final result = FileEntry._();
    if (relpath != null) result.relpath = relpath;
    if (size != null) result.size = size;
    if (sha256 != null) result.sha256 = sha256;
    if (mode != null) result.mode = mode;
    return result;
  }

  FileEntry._();

  factory FileEntry.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileEntry()..mergeFromBuffer(data, registry);
  factory FileEntry.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileEntry()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FileEntry',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: FileEntry.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'relpath')
    ..aInt64(2, _omitFieldNames ? '' : 'size')
    ..aOS(3, _omitFieldNames ? '' : 'sha256')
    ..aI(4, _omitFieldNames ? '' : 'mode', fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileEntry clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileEntry copyWith(void Function(FileEntry) updates) =>
      super.copyWith((message) => updates(message as FileEntry)) as FileEntry;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use FileEntry() / FileEntry.new instead')
  static FileEntry create() => FileEntry._();
  static $pb.GeneratedMessage $_createMessage() => FileEntry._();
  @$core.override
  FileEntry createEmptyInstance() => FileEntry._();
  @$core.pragma('dart2js:noInline')
  static FileEntry getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FileEntry>(FileEntry.$_createMessage);
  static FileEntry? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get relpath => $_getSZ(0);
  @$pb.TagNumber(1)
  set relpath($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasRelpath() => $_has(0);
  @$pb.TagNumber(1)
  void clearRelpath() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get size => $_getI64(1);
  @$pb.TagNumber(2)
  set size($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSize() => $_has(1);
  @$pb.TagNumber(2)
  void clearSize() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get sha256 => $_getSZ(2);
  @$pb.TagNumber(3)
  set sha256($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSha256() => $_has(2);
  @$pb.TagNumber(3)
  void clearSha256() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.int get mode => $_getIZ(3);
  @$pb.TagNumber(4)
  set mode($core.int value) => $_setUnsignedInt32(3, value);
  @$pb.TagNumber(4)
  $core.bool hasMode() => $_has(3);
  @$pb.TagNumber(4)
  void clearMode() => $_clearField(4);
}

class ScriptEntry extends $pb.GeneratedMessage {
  factory ScriptEntry({
    ScriptPhase? phase,
    $core.String? relpath,
    $fixnum.Int64? size,
    $core.String? sha256,
    $0.Duration? timeout,
    ScriptInterpreter? interpreter,
    $core.Iterable<$core.String>? args,
  }) {
    final result = ScriptEntry._();
    if (phase != null) result.phase = phase;
    if (relpath != null) result.relpath = relpath;
    if (size != null) result.size = size;
    if (sha256 != null) result.sha256 = sha256;
    if (timeout != null) result.timeout = timeout;
    if (interpreter != null) result.interpreter = interpreter;
    if (args != null) result.args.addAll(args);
    return result;
  }

  ScriptEntry._();

  factory ScriptEntry.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ScriptEntry()..mergeFromBuffer(data, registry);
  factory ScriptEntry.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ScriptEntry()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ScriptEntry',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: ScriptEntry.$_createMessage)
    ..aE<ScriptPhase>(1, _omitFieldNames ? '' : 'phase',
        enumValues: ScriptPhase.values)
    ..aOS(2, _omitFieldNames ? '' : 'relpath')
    ..aInt64(3, _omitFieldNames ? '' : 'size')
    ..aOS(4, _omitFieldNames ? '' : 'sha256')
    ..aOM<$0.Duration>(5, _omitFieldNames ? '' : 'timeout',
        subBuilder: $0.Duration.$_createMessage)
    ..aE<ScriptInterpreter>(6, _omitFieldNames ? '' : 'interpreter',
        enumValues: ScriptInterpreter.values)
    ..pPS(7, _omitFieldNames ? '' : 'args')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ScriptEntry clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ScriptEntry copyWith(void Function(ScriptEntry) updates) =>
      super.copyWith((message) => updates(message as ScriptEntry))
          as ScriptEntry;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use ScriptEntry() / ScriptEntry.new instead')
  static ScriptEntry create() => ScriptEntry._();
  static $pb.GeneratedMessage $_createMessage() => ScriptEntry._();
  @$core.override
  ScriptEntry createEmptyInstance() => ScriptEntry._();
  @$core.pragma('dart2js:noInline')
  static ScriptEntry getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ScriptEntry>(
          ScriptEntry.$_createMessage);
  static ScriptEntry? _defaultInstance;

  @$pb.TagNumber(1)
  ScriptPhase get phase => $_getN(0);
  @$pb.TagNumber(1)
  set phase(ScriptPhase value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasPhase() => $_has(0);
  @$pb.TagNumber(1)
  void clearPhase() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get relpath => $_getSZ(1);
  @$pb.TagNumber(2)
  set relpath($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasRelpath() => $_has(1);
  @$pb.TagNumber(2)
  void clearRelpath() => $_clearField(2);

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
  $0.Duration get timeout => $_getN(4);
  @$pb.TagNumber(5)
  set timeout($0.Duration value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasTimeout() => $_has(4);
  @$pb.TagNumber(5)
  void clearTimeout() => $_clearField(5);
  @$pb.TagNumber(5)
  $0.Duration ensureTimeout() => $_ensure(4);

  @$pb.TagNumber(6)
  ScriptInterpreter get interpreter => $_getN(5);
  @$pb.TagNumber(6)
  set interpreter(ScriptInterpreter value) => $_setField(6, value);
  @$pb.TagNumber(6)
  $core.bool hasInterpreter() => $_has(5);
  @$pb.TagNumber(6)
  void clearInterpreter() => $_clearField(6);

  @$pb.TagNumber(7)
  $pb.PbList<$core.String> get args => $_getList(6);
}

/// Baseline is local engine state and is never published.
class Baseline extends $pb.GeneratedMessage {
  factory Baseline({
    $core.String? schema,
    $fixnum.Int64? code,
    $core.String? version,
    $core.Iterable<BaselineEntry>? files,
  }) {
    final result = Baseline._();
    if (schema != null) result.schema = schema;
    if (code != null) result.code = code;
    if (version != null) result.version = version;
    if (files != null) result.files.addAll(files);
    return result;
  }

  Baseline._();

  factory Baseline.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Baseline()..mergeFromBuffer(data, registry);
  factory Baseline.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Baseline()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Baseline',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: Baseline.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'schema')
    ..aInt64(2, _omitFieldNames ? '' : 'code')
    ..aOS(3, _omitFieldNames ? '' : 'version')
    ..pPM<BaselineEntry>(4, _omitFieldNames ? '' : 'files',
        subBuilder: BaselineEntry.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Baseline clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Baseline copyWith(void Function(Baseline) updates) =>
      super.copyWith((message) => updates(message as Baseline)) as Baseline;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use Baseline() / Baseline.new instead')
  static Baseline create() => Baseline._();
  static $pb.GeneratedMessage $_createMessage() => Baseline._();
  @$core.override
  Baseline createEmptyInstance() => Baseline._();
  @$core.pragma('dart2js:noInline')
  static Baseline getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Baseline>(Baseline.$_createMessage);
  static Baseline? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get schema => $_getSZ(0);
  @$pb.TagNumber(1)
  set schema($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSchema() => $_has(0);
  @$pb.TagNumber(1)
  void clearSchema() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get code => $_getI64(1);
  @$pb.TagNumber(2)
  set code($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCode() => $_has(1);
  @$pb.TagNumber(2)
  void clearCode() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get version => $_getSZ(2);
  @$pb.TagNumber(3)
  set version($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasVersion() => $_has(2);
  @$pb.TagNumber(3)
  void clearVersion() => $_clearField(3);

  @$pb.TagNumber(4)
  $pb.PbList<BaselineEntry> get files => $_getList(3);
}

class BaselineEntry extends $pb.GeneratedMessage {
  factory BaselineEntry({
    $core.String? relpath,
    $core.String? sha256,
  }) {
    final result = BaselineEntry._();
    if (relpath != null) result.relpath = relpath;
    if (sha256 != null) result.sha256 = sha256;
    return result;
  }

  BaselineEntry._();

  factory BaselineEntry.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      BaselineEntry()..mergeFromBuffer(data, registry);
  factory BaselineEntry.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      BaselineEntry()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'BaselineEntry',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: BaselineEntry.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'relpath')
    ..aOS(2, _omitFieldNames ? '' : 'sha256')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BaselineEntry clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BaselineEntry copyWith(void Function(BaselineEntry) updates) =>
      super.copyWith((message) => updates(message as BaselineEntry))
          as BaselineEntry;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use BaselineEntry() / BaselineEntry.new instead')
  static BaselineEntry create() => BaselineEntry._();
  static $pb.GeneratedMessage $_createMessage() => BaselineEntry._();
  @$core.override
  BaselineEntry createEmptyInstance() => BaselineEntry._();
  @$core.pragma('dart2js:noInline')
  static BaselineEntry getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<BaselineEntry>(
          BaselineEntry.$_createMessage);
  static BaselineEntry? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get relpath => $_getSZ(0);
  @$pb.TagNumber(1)
  set relpath($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasRelpath() => $_has(0);
  @$pb.TagNumber(1)
  void clearRelpath() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get sha256 => $_getSZ(1);
  @$pb.TagNumber(2)
  set sha256($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSha256() => $_has(1);
  @$pb.TagNumber(2)
  void clearSha256() => $_clearField(2);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
