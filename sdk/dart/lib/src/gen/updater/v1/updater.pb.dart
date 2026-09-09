// This is a generated file - do not edit.
//
// Generated from updater/v1/updater.proto.

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
import 'package:protobuf/well_known_types/google/protobuf/timestamp.pb.dart'
    as $1;

import 'updater.pbenum.dart';

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

export 'updater.pbenum.dart';

class TrustedKey extends $pb.GeneratedMessage {
  factory TrustedKey({
    $core.String? keyId,
    $core.List<$core.int>? publicKey,
  }) {
    final result = create();
    if (keyId != null) result.keyId = keyId;
    if (publicKey != null) result.publicKey = publicKey;
    return result;
  }

  TrustedKey._();

  factory TrustedKey.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory TrustedKey.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'TrustedKey',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'keyId')
    ..a<$core.List<$core.int>>(
        2, _omitFieldNames ? '' : 'publicKey', $pb.PbFieldType.OY)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  TrustedKey clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  TrustedKey copyWith(void Function(TrustedKey) updates) =>
      super.copyWith((message) => updates(message as TrustedKey)) as TrustedKey;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static TrustedKey create() => TrustedKey._();
  @$core.override
  TrustedKey createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static TrustedKey getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<TrustedKey>(create);
  static TrustedKey? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get keyId => $_getSZ(0);
  @$pb.TagNumber(1)
  set keyId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasKeyId() => $_has(0);
  @$pb.TagNumber(1)
  void clearKeyId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.List<$core.int> get publicKey => $_getN(1);
  @$pb.TagNumber(2)
  set publicKey($core.List<$core.int> value) => $_setBytes(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPublicKey() => $_has(1);
  @$pb.TagNumber(2)
  void clearPublicKey() => $_clearField(2);
}

class RecoveryLink extends $pb.GeneratedMessage {
  factory RecoveryLink({
    $core.String? label,
    $core.String? url,
  }) {
    final result = create();
    if (label != null) result.label = label;
    if (url != null) result.url = url;
    return result;
  }

  RecoveryLink._();

  factory RecoveryLink.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory RecoveryLink.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RecoveryLink',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'label')
    ..aOS(2, _omitFieldNames ? '' : 'url')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RecoveryLink clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RecoveryLink copyWith(void Function(RecoveryLink) updates) =>
      super.copyWith((message) => updates(message as RecoveryLink))
          as RecoveryLink;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static RecoveryLink create() => RecoveryLink._();
  @$core.override
  RecoveryLink createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static RecoveryLink getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<RecoveryLink>(create);
  static RecoveryLink? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get label => $_getSZ(0);
  @$pb.TagNumber(1)
  set label($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasLabel() => $_has(0);
  @$pb.TagNumber(1)
  void clearLabel() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get url => $_getSZ(1);
  @$pb.TagNumber(2)
  set url($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasUrl() => $_has(1);
  @$pb.TagNumber(2)
  void clearUrl() => $_clearField(2);
}

class RecoveryHelp extends $pb.GeneratedMessage {
  factory RecoveryHelp({
    $core.String? message,
    $core.Iterable<RecoveryLink>? links,
  }) {
    final result = create();
    if (message != null) result.message = message;
    if (links != null) result.links.addAll(links);
    return result;
  }

  RecoveryHelp._();

  factory RecoveryHelp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory RecoveryHelp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RecoveryHelp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'message')
    ..pPM<RecoveryLink>(2, _omitFieldNames ? '' : 'links',
        subBuilder: RecoveryLink.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RecoveryHelp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RecoveryHelp copyWith(void Function(RecoveryHelp) updates) =>
      super.copyWith((message) => updates(message as RecoveryHelp))
          as RecoveryHelp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static RecoveryHelp create() => RecoveryHelp._();
  @$core.override
  RecoveryHelp createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static RecoveryHelp getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<RecoveryHelp>(create);
  static RecoveryHelp? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get message => $_getSZ(0);
  @$pb.TagNumber(1)
  set message($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasMessage() => $_has(0);
  @$pb.TagNumber(1)
  void clearMessage() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<RecoveryLink> get links => $_getList(1);
}

class ClientProfile extends $pb.GeneratedMessage {
  factory ClientProfile({
    $core.String? product,
    $core.Iterable<$core.String>? allowedChannels,
    $core.Iterable<$core.String>? entryUrls,
    $core.Iterable<$core.String>? indexUrls,
    $core.Iterable<$core.String>? fallbackUrls,
    $core.Iterable<TrustedKey>? trustedKeys,
    RecoveryHelp? recovery,
  }) {
    final result = create();
    if (product != null) result.product = product;
    if (allowedChannels != null) result.allowedChannels.addAll(allowedChannels);
    if (entryUrls != null) result.entryUrls.addAll(entryUrls);
    if (indexUrls != null) result.indexUrls.addAll(indexUrls);
    if (fallbackUrls != null) result.fallbackUrls.addAll(fallbackUrls);
    if (trustedKeys != null) result.trustedKeys.addAll(trustedKeys);
    if (recovery != null) result.recovery = recovery;
    return result;
  }

  ClientProfile._();

  factory ClientProfile.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientProfile.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientProfile',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'product')
    ..pPS(2, _omitFieldNames ? '' : 'allowedChannels')
    ..pPS(3, _omitFieldNames ? '' : 'entryUrls')
    ..pPS(4, _omitFieldNames ? '' : 'indexUrls')
    ..pPS(5, _omitFieldNames ? '' : 'fallbackUrls')
    ..pPM<TrustedKey>(6, _omitFieldNames ? '' : 'trustedKeys',
        subBuilder: TrustedKey.create)
    ..aOM<RecoveryHelp>(7, _omitFieldNames ? '' : 'recovery',
        subBuilder: RecoveryHelp.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientProfile clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientProfile copyWith(void Function(ClientProfile) updates) =>
      super.copyWith((message) => updates(message as ClientProfile))
          as ClientProfile;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientProfile create() => ClientProfile._();
  @$core.override
  ClientProfile createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientProfile getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientProfile>(create);
  static ClientProfile? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get product => $_getSZ(0);
  @$pb.TagNumber(1)
  set product($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasProduct() => $_has(0);
  @$pb.TagNumber(1)
  void clearProduct() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<$core.String> get allowedChannels => $_getList(1);

  @$pb.TagNumber(3)
  $pb.PbList<$core.String> get entryUrls => $_getList(2);

  @$pb.TagNumber(4)
  $pb.PbList<$core.String> get indexUrls => $_getList(3);

  @$pb.TagNumber(5)
  $pb.PbList<$core.String> get fallbackUrls => $_getList(4);

  @$pb.TagNumber(6)
  $pb.PbList<TrustedKey> get trustedKeys => $_getList(5);

  @$pb.TagNumber(7)
  RecoveryHelp get recovery => $_getN(6);
  @$pb.TagNumber(7)
  set recovery(RecoveryHelp value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasRecovery() => $_has(6);
  @$pb.TagNumber(7)
  void clearRecovery() => $_clearField(7);
  @$pb.TagNumber(7)
  RecoveryHelp ensureRecovery() => $_ensure(6);
}

class FileSetEntry extends $pb.GeneratedMessage {
  factory FileSetEntry({
    $core.String? destRelpath,
    $core.String? artifactName,
  }) {
    final result = create();
    if (destRelpath != null) result.destRelpath = destRelpath;
    if (artifactName != null) result.artifactName = artifactName;
    return result;
  }

  FileSetEntry._();

  factory FileSetEntry.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FileSetEntry.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FileSetEntry',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'destRelpath')
    ..aOS(2, _omitFieldNames ? '' : 'artifactName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileSetEntry clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FileSetEntry copyWith(void Function(FileSetEntry) updates) =>
      super.copyWith((message) => updates(message as FileSetEntry))
          as FileSetEntry;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FileSetEntry create() => FileSetEntry._();
  @$core.override
  FileSetEntry createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FileSetEntry getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FileSetEntry>(create);
  static FileSetEntry? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get destRelpath => $_getSZ(0);
  @$pb.TagNumber(1)
  set destRelpath($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDestRelpath() => $_has(0);
  @$pb.TagNumber(1)
  void clearDestRelpath() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get artifactName => $_getSZ(1);
  @$pb.TagNumber(2)
  set artifactName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasArtifactName() => $_has(1);
  @$pb.TagNumber(2)
  void clearArtifactName() => $_clearField(2);
}

class InstallSpec extends $pb.GeneratedMessage {
  factory InstallSpec({
    Layout? layout,
    $core.String? installRoot,
    $core.String? executableRelpath,
    $core.String? sidecarRelpath,
    $core.Iterable<$core.String>? preserve,
    $core.int? retain,
    $core.bool? relaunch,
    $core.Iterable<FileSetEntry>? fileSet,
  }) {
    final result = create();
    if (layout != null) result.layout = layout;
    if (installRoot != null) result.installRoot = installRoot;
    if (executableRelpath != null) result.executableRelpath = executableRelpath;
    if (sidecarRelpath != null) result.sidecarRelpath = sidecarRelpath;
    if (preserve != null) result.preserve.addAll(preserve);
    if (retain != null) result.retain = retain;
    if (relaunch != null) result.relaunch = relaunch;
    if (fileSet != null) result.fileSet.addAll(fileSet);
    return result;
  }

  InstallSpec._();

  factory InstallSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory InstallSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'InstallSpec',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aE<Layout>(1, _omitFieldNames ? '' : 'layout', enumValues: Layout.values)
    ..aOS(2, _omitFieldNames ? '' : 'installRoot')
    ..aOS(3, _omitFieldNames ? '' : 'executableRelpath')
    ..aOS(4, _omitFieldNames ? '' : 'sidecarRelpath')
    ..pPS(5, _omitFieldNames ? '' : 'preserve')
    ..aI(6, _omitFieldNames ? '' : 'retain')
    ..aOB(7, _omitFieldNames ? '' : 'relaunch')
    ..pPM<FileSetEntry>(8, _omitFieldNames ? '' : 'fileSet',
        subBuilder: FileSetEntry.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InstallSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InstallSpec copyWith(void Function(InstallSpec) updates) =>
      super.copyWith((message) => updates(message as InstallSpec))
          as InstallSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static InstallSpec create() => InstallSpec._();
  @$core.override
  InstallSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static InstallSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<InstallSpec>(create);
  static InstallSpec? _defaultInstance;

  @$pb.TagNumber(1)
  Layout get layout => $_getN(0);
  @$pb.TagNumber(1)
  set layout(Layout value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasLayout() => $_has(0);
  @$pb.TagNumber(1)
  void clearLayout() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get installRoot => $_getSZ(1);
  @$pb.TagNumber(2)
  set installRoot($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasInstallRoot() => $_has(1);
  @$pb.TagNumber(2)
  void clearInstallRoot() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get executableRelpath => $_getSZ(2);
  @$pb.TagNumber(3)
  set executableRelpath($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasExecutableRelpath() => $_has(2);
  @$pb.TagNumber(3)
  void clearExecutableRelpath() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get sidecarRelpath => $_getSZ(3);
  @$pb.TagNumber(4)
  set sidecarRelpath($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSidecarRelpath() => $_has(3);
  @$pb.TagNumber(4)
  void clearSidecarRelpath() => $_clearField(4);

  @$pb.TagNumber(5)
  $pb.PbList<$core.String> get preserve => $_getList(4);

  @$pb.TagNumber(6)
  $core.int get retain => $_getIZ(5);
  @$pb.TagNumber(6)
  set retain($core.int value) => $_setSignedInt32(5, value);
  @$pb.TagNumber(6)
  $core.bool hasRetain() => $_has(5);
  @$pb.TagNumber(6)
  void clearRetain() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get relaunch => $_getBF(6);
  @$pb.TagNumber(7)
  set relaunch($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasRelaunch() => $_has(6);
  @$pb.TagNumber(7)
  void clearRelaunch() => $_clearField(7);

  @$pb.TagNumber(8)
  $pb.PbList<FileSetEntry> get fileSet => $_getList(7);
}

class Runtime extends $pb.GeneratedMessage {
  factory Runtime({
    $core.String? channel,
    $fixnum.Int64? currentCode,
    $core.Iterable<$core.MapEntry<$core.String, $core.String>>? clientSelectors,
    $core.String? dataDir,
    InstallSpec? install,
    $core.String? sidecarPath,
  }) {
    final result = create();
    if (channel != null) result.channel = channel;
    if (currentCode != null) result.currentCode = currentCode;
    if (clientSelectors != null)
      result.clientSelectors.addEntries(clientSelectors);
    if (dataDir != null) result.dataDir = dataDir;
    if (install != null) result.install = install;
    if (sidecarPath != null) result.sidecarPath = sidecarPath;
    return result;
  }

  Runtime._();

  factory Runtime.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Runtime.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Runtime',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'channel')
    ..aInt64(2, _omitFieldNames ? '' : 'currentCode')
    ..m<$core.String, $core.String>(3, _omitFieldNames ? '' : 'clientSelectors',
        entryClassName: 'Runtime.ClientSelectorsEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OS,
        packageName: const $pb.PackageName('relkit.updater.v1'))
    ..aOS(4, _omitFieldNames ? '' : 'dataDir')
    ..aOM<InstallSpec>(5, _omitFieldNames ? '' : 'install',
        subBuilder: InstallSpec.create)
    ..aOS(6, _omitFieldNames ? '' : 'sidecarPath')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Runtime clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Runtime copyWith(void Function(Runtime) updates) =>
      super.copyWith((message) => updates(message as Runtime)) as Runtime;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Runtime create() => Runtime._();
  @$core.override
  Runtime createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Runtime getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Runtime>(create);
  static Runtime? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get channel => $_getSZ(0);
  @$pb.TagNumber(1)
  set channel($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasChannel() => $_has(0);
  @$pb.TagNumber(1)
  void clearChannel() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get currentCode => $_getI64(1);
  @$pb.TagNumber(2)
  set currentCode($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCurrentCode() => $_has(1);
  @$pb.TagNumber(2)
  void clearCurrentCode() => $_clearField(2);

  @$pb.TagNumber(3)
  $pb.PbMap<$core.String, $core.String> get clientSelectors => $_getMap(2);

  @$pb.TagNumber(4)
  $core.String get dataDir => $_getSZ(3);
  @$pb.TagNumber(4)
  set dataDir($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasDataDir() => $_has(3);
  @$pb.TagNumber(4)
  void clearDataDir() => $_clearField(4);

  @$pb.TagNumber(5)
  InstallSpec get install => $_getN(4);
  @$pb.TagNumber(5)
  set install(InstallSpec value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasInstall() => $_has(4);
  @$pb.TagNumber(5)
  void clearInstall() => $_clearField(5);
  @$pb.TagNumber(5)
  InstallSpec ensureInstall() => $_ensure(4);

  @$pb.TagNumber(6)
  $core.String get sidecarPath => $_getSZ(5);
  @$pb.TagNumber(6)
  set sidecarPath($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasSidecarPath() => $_has(5);
  @$pb.TagNumber(6)
  void clearSidecarPath() => $_clearField(6);
}

class CheckPolicy extends $pb.GeneratedMessage {
  factory CheckPolicy({
    $0.Duration? afterSuccess,
    $0.Duration? afterFailure,
  }) {
    final result = create();
    if (afterSuccess != null) result.afterSuccess = afterSuccess;
    if (afterFailure != null) result.afterFailure = afterFailure;
    return result;
  }

  CheckPolicy._();

  factory CheckPolicy.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CheckPolicy.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CheckPolicy',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOM<$0.Duration>(1, _omitFieldNames ? '' : 'afterSuccess',
        subBuilder: $0.Duration.create)
    ..aOM<$0.Duration>(2, _omitFieldNames ? '' : 'afterFailure',
        subBuilder: $0.Duration.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CheckPolicy clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CheckPolicy copyWith(void Function(CheckPolicy) updates) =>
      super.copyWith((message) => updates(message as CheckPolicy))
          as CheckPolicy;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CheckPolicy create() => CheckPolicy._();
  @$core.override
  CheckPolicy createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CheckPolicy getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CheckPolicy>(create);
  static CheckPolicy? _defaultInstance;

  @$pb.TagNumber(1)
  $0.Duration get afterSuccess => $_getN(0);
  @$pb.TagNumber(1)
  set afterSuccess($0.Duration value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAfterSuccess() => $_has(0);
  @$pb.TagNumber(1)
  void clearAfterSuccess() => $_clearField(1);
  @$pb.TagNumber(1)
  $0.Duration ensureAfterSuccess() => $_ensure(0);

  @$pb.TagNumber(2)
  $0.Duration get afterFailure => $_getN(1);
  @$pb.TagNumber(2)
  set afterFailure($0.Duration value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasAfterFailure() => $_has(1);
  @$pb.TagNumber(2)
  void clearAfterFailure() => $_clearField(2);
  @$pb.TagNumber(2)
  $0.Duration ensureAfterFailure() => $_ensure(1);
}

class SchedulerConfig extends $pb.GeneratedMessage {
  factory SchedulerConfig({
    $core.bool? checkOnStart,
    $core.bool? forceOnStart,
    CheckPolicy? policy,
  }) {
    final result = create();
    if (checkOnStart != null) result.checkOnStart = checkOnStart;
    if (forceOnStart != null) result.forceOnStart = forceOnStart;
    if (policy != null) result.policy = policy;
    return result;
  }

  SchedulerConfig._();

  factory SchedulerConfig.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SchedulerConfig.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SchedulerConfig',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'checkOnStart')
    ..aOB(2, _omitFieldNames ? '' : 'forceOnStart')
    ..aOM<CheckPolicy>(3, _omitFieldNames ? '' : 'policy',
        subBuilder: CheckPolicy.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SchedulerConfig clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SchedulerConfig copyWith(void Function(SchedulerConfig) updates) =>
      super.copyWith((message) => updates(message as SchedulerConfig))
          as SchedulerConfig;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SchedulerConfig create() => SchedulerConfig._();
  @$core.override
  SchedulerConfig createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SchedulerConfig getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SchedulerConfig>(create);
  static SchedulerConfig? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get checkOnStart => $_getBF(0);
  @$pb.TagNumber(1)
  set checkOnStart($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCheckOnStart() => $_has(0);
  @$pb.TagNumber(1)
  void clearCheckOnStart() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get forceOnStart => $_getBF(1);
  @$pb.TagNumber(2)
  set forceOnStart($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasForceOnStart() => $_has(1);
  @$pb.TagNumber(2)
  void clearForceOnStart() => $_clearField(2);

  @$pb.TagNumber(3)
  CheckPolicy get policy => $_getN(2);
  @$pb.TagNumber(3)
  set policy(CheckPolicy value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasPolicy() => $_has(2);
  @$pb.TagNumber(3)
  void clearPolicy() => $_clearField(3);
  @$pb.TagNumber(3)
  CheckPolicy ensurePolicy() => $_ensure(2);
}

class ClientHello extends $pb.GeneratedMessage {
  factory ClientHello({
    $core.int? ipcMin,
    $core.int? ipcMax,
  }) {
    final result = create();
    if (ipcMin != null) result.ipcMin = ipcMin;
    if (ipcMax != null) result.ipcMax = ipcMax;
    return result;
  }

  ClientHello._();

  factory ClientHello.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientHello.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientHello',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aI(1, _omitFieldNames ? '' : 'ipcMin', fieldType: $pb.PbFieldType.OU3)
    ..aI(2, _omitFieldNames ? '' : 'ipcMax', fieldType: $pb.PbFieldType.OU3)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientHello clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientHello copyWith(void Function(ClientHello) updates) =>
      super.copyWith((message) => updates(message as ClientHello))
          as ClientHello;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientHello create() => ClientHello._();
  @$core.override
  ClientHello createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientHello getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientHello>(create);
  static ClientHello? _defaultInstance;

  @$pb.TagNumber(1)
  $core.int get ipcMin => $_getIZ(0);
  @$pb.TagNumber(1)
  set ipcMin($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasIpcMin() => $_has(0);
  @$pb.TagNumber(1)
  void clearIpcMin() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.int get ipcMax => $_getIZ(1);
  @$pb.TagNumber(2)
  set ipcMax($core.int value) => $_setUnsignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasIpcMax() => $_has(1);
  @$pb.TagNumber(2)
  void clearIpcMax() => $_clearField(2);
}

class Capabilities extends $pb.GeneratedMessage {
  factory Capabilities({
    $core.int? ipc,
    $core.Iterable<Operation>? operations,
    $core.Iterable<Layout>? layouts,
    $0.Duration? minCheckInterval,
    $0.Duration? planTtl,
    $core.String? engineVersion,
  }) {
    final result = create();
    if (ipc != null) result.ipc = ipc;
    if (operations != null) result.operations.addAll(operations);
    if (layouts != null) result.layouts.addAll(layouts);
    if (minCheckInterval != null) result.minCheckInterval = minCheckInterval;
    if (planTtl != null) result.planTtl = planTtl;
    if (engineVersion != null) result.engineVersion = engineVersion;
    return result;
  }

  Capabilities._();

  factory Capabilities.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Capabilities.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Capabilities',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aI(1, _omitFieldNames ? '' : 'ipc', fieldType: $pb.PbFieldType.OU3)
    ..pc<Operation>(2, _omitFieldNames ? '' : 'operations', $pb.PbFieldType.KE,
        valueOf: Operation.valueOf,
        enumValues: Operation.values,
        defaultEnumValue: Operation.OPERATION_UNSPECIFIED)
    ..pc<Layout>(3, _omitFieldNames ? '' : 'layouts', $pb.PbFieldType.KE,
        valueOf: Layout.valueOf,
        enumValues: Layout.values,
        defaultEnumValue: Layout.LAYOUT_UNSPECIFIED)
    ..aOM<$0.Duration>(4, _omitFieldNames ? '' : 'minCheckInterval',
        subBuilder: $0.Duration.create)
    ..aOM<$0.Duration>(5, _omitFieldNames ? '' : 'planTtl',
        subBuilder: $0.Duration.create)
    ..aOS(6, _omitFieldNames ? '' : 'engineVersion')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Capabilities clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Capabilities copyWith(void Function(Capabilities) updates) =>
      super.copyWith((message) => updates(message as Capabilities))
          as Capabilities;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Capabilities create() => Capabilities._();
  @$core.override
  Capabilities createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Capabilities getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Capabilities>(create);
  static Capabilities? _defaultInstance;

  @$pb.TagNumber(1)
  $core.int get ipc => $_getIZ(0);
  @$pb.TagNumber(1)
  set ipc($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasIpc() => $_has(0);
  @$pb.TagNumber(1)
  void clearIpc() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<Operation> get operations => $_getList(1);

  @$pb.TagNumber(3)
  $pb.PbList<Layout> get layouts => $_getList(2);

  @$pb.TagNumber(4)
  $0.Duration get minCheckInterval => $_getN(3);
  @$pb.TagNumber(4)
  set minCheckInterval($0.Duration value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasMinCheckInterval() => $_has(3);
  @$pb.TagNumber(4)
  void clearMinCheckInterval() => $_clearField(4);
  @$pb.TagNumber(4)
  $0.Duration ensureMinCheckInterval() => $_ensure(3);

  @$pb.TagNumber(5)
  $0.Duration get planTtl => $_getN(4);
  @$pb.TagNumber(5)
  set planTtl($0.Duration value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasPlanTtl() => $_has(4);
  @$pb.TagNumber(5)
  void clearPlanTtl() => $_clearField(5);
  @$pb.TagNumber(5)
  $0.Duration ensurePlanTtl() => $_ensure(4);

  @$pb.TagNumber(6)
  $core.String get engineVersion => $_getSZ(5);
  @$pb.TagNumber(6)
  set engineVersion($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasEngineVersion() => $_has(5);
  @$pb.TagNumber(6)
  void clearEngineVersion() => $_clearField(6);
}

class CheckOp extends $pb.GeneratedMessage {
  factory CheckOp({
    $core.bool? force,
    $fixnum.Int64? exactCode,
    CheckPolicy? policy,
  }) {
    final result = create();
    if (force != null) result.force = force;
    if (exactCode != null) result.exactCode = exactCode;
    if (policy != null) result.policy = policy;
    return result;
  }

  CheckOp._();

  factory CheckOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CheckOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CheckOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'force')
    ..aInt64(2, _omitFieldNames ? '' : 'exactCode')
    ..aOM<CheckPolicy>(3, _omitFieldNames ? '' : 'policy',
        subBuilder: CheckPolicy.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CheckOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CheckOp copyWith(void Function(CheckOp) updates) =>
      super.copyWith((message) => updates(message as CheckOp)) as CheckOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CheckOp create() => CheckOp._();
  @$core.override
  CheckOp createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CheckOp getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<CheckOp>(create);
  static CheckOp? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get force => $_getBF(0);
  @$pb.TagNumber(1)
  set force($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasForce() => $_has(0);
  @$pb.TagNumber(1)
  void clearForce() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get exactCode => $_getI64(1);
  @$pb.TagNumber(2)
  set exactCode($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasExactCode() => $_has(1);
  @$pb.TagNumber(2)
  void clearExactCode() => $_clearField(2);

  @$pb.TagNumber(3)
  CheckPolicy get policy => $_getN(2);
  @$pb.TagNumber(3)
  set policy(CheckPolicy value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasPolicy() => $_has(2);
  @$pb.TagNumber(3)
  void clearPolicy() => $_clearField(3);
  @$pb.TagNumber(3)
  CheckPolicy ensurePolicy() => $_ensure(2);
}

class SkipOp extends $pb.GeneratedMessage {
  factory SkipOp({
    $fixnum.Int64? code,
  }) {
    final result = create();
    if (code != null) result.code = code;
    return result;
  }

  SkipOp._();

  factory SkipOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SkipOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SkipOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'code')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SkipOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SkipOp copyWith(void Function(SkipOp) updates) =>
      super.copyWith((message) => updates(message as SkipOp)) as SkipOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SkipOp create() => SkipOp._();
  @$core.override
  SkipOp createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SkipOp getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<SkipOp>(create);
  static SkipOp? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get code => $_getI64(0);
  @$pb.TagNumber(1)
  set code($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCode() => $_has(0);
  @$pb.TagNumber(1)
  void clearCode() => $_clearField(1);
}

class DownloadOp extends $pb.GeneratedMessage {
  factory DownloadOp({
    $core.String? planId,
  }) {
    final result = create();
    if (planId != null) result.planId = planId;
    return result;
  }

  DownloadOp._();

  factory DownloadOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DownloadOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DownloadOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'planId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DownloadOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DownloadOp copyWith(void Function(DownloadOp) updates) =>
      super.copyWith((message) => updates(message as DownloadOp)) as DownloadOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DownloadOp create() => DownloadOp._();
  @$core.override
  DownloadOp createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DownloadOp getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DownloadOp>(create);
  static DownloadOp? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get planId => $_getSZ(0);
  @$pb.TagNumber(1)
  set planId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPlanId() => $_has(0);
  @$pb.TagNumber(1)
  void clearPlanId() => $_clearField(1);
}

class ApplyOp extends $pb.GeneratedMessage {
  factory ApplyOp({
    $core.String? planId,
  }) {
    final result = create();
    if (planId != null) result.planId = planId;
    return result;
  }

  ApplyOp._();

  factory ApplyOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ApplyOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ApplyOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'planId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplyOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplyOp copyWith(void Function(ApplyOp) updates) =>
      super.copyWith((message) => updates(message as ApplyOp)) as ApplyOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ApplyOp create() => ApplyOp._();
  @$core.override
  ApplyOp createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ApplyOp getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ApplyOp>(create);
  static ApplyOp? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get planId => $_getSZ(0);
  @$pb.TagNumber(1)
  set planId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPlanId() => $_has(0);
  @$pb.TagNumber(1)
  void clearPlanId() => $_clearField(1);
}

class StatusOp extends $pb.GeneratedMessage {
  factory StatusOp() => create();

  StatusOp._();

  factory StatusOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory StatusOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StatusOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StatusOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StatusOp copyWith(void Function(StatusOp) updates) =>
      super.copyWith((message) => updates(message as StatusOp)) as StatusOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static StatusOp create() => StatusOp._();
  @$core.override
  StatusOp createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static StatusOp getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<StatusOp>(create);
  static StatusOp? _defaultInstance;
}

class CleanupOp extends $pb.GeneratedMessage {
  factory CleanupOp() => create();

  CleanupOp._();

  factory CleanupOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CleanupOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CleanupOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CleanupOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CleanupOp copyWith(void Function(CleanupOp) updates) =>
      super.copyWith((message) => updates(message as CleanupOp)) as CleanupOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CleanupOp create() => CleanupOp._();
  @$core.override
  CleanupOp createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CleanupOp getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<CleanupOp>(create);
  static CleanupOp? _defaultInstance;
}

class CancelOp extends $pb.GeneratedMessage {
  factory CancelOp() => create();

  CancelOp._();

  factory CancelOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CancelOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CancelOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CancelOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CancelOp copyWith(void Function(CancelOp) updates) =>
      super.copyWith((message) => updates(message as CancelOp)) as CancelOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CancelOp create() => CancelOp._();
  @$core.override
  CancelOp createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CancelOp getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<CancelOp>(create);
  static CancelOp? _defaultInstance;
}

enum UpdaterRequest_Op {
  check_10,
  skip,
  download,
  apply,
  status,
  cleanup,
  cancel,
  notSet
}

class UpdaterRequest extends $pb.GeneratedMessage {
  factory UpdaterRequest({
    ClientHello? hello,
    ClientProfile? profile,
    Runtime? runtime,
    CheckOp? check_10,
    SkipOp? skip,
    DownloadOp? download,
    ApplyOp? apply,
    StatusOp? status,
    CleanupOp? cleanup,
    CancelOp? cancel,
  }) {
    final result = create();
    if (hello != null) result.hello = hello;
    if (profile != null) result.profile = profile;
    if (runtime != null) result.runtime = runtime;
    if (check_10 != null) result.check_10 = check_10;
    if (skip != null) result.skip = skip;
    if (download != null) result.download = download;
    if (apply != null) result.apply = apply;
    if (status != null) result.status = status;
    if (cleanup != null) result.cleanup = cleanup;
    if (cancel != null) result.cancel = cancel;
    return result;
  }

  UpdaterRequest._();

  factory UpdaterRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory UpdaterRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, UpdaterRequest_Op> _UpdaterRequest_OpByTag =
      {
    10: UpdaterRequest_Op.check_10,
    11: UpdaterRequest_Op.skip,
    12: UpdaterRequest_Op.download,
    13: UpdaterRequest_Op.apply,
    14: UpdaterRequest_Op.status,
    15: UpdaterRequest_Op.cleanup,
    16: UpdaterRequest_Op.cancel,
    0: UpdaterRequest_Op.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'UpdaterRequest',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..oo(0, [10, 11, 12, 13, 14, 15, 16])
    ..aOM<ClientHello>(1, _omitFieldNames ? '' : 'hello',
        subBuilder: ClientHello.create)
    ..aOM<ClientProfile>(2, _omitFieldNames ? '' : 'profile',
        subBuilder: ClientProfile.create)
    ..aOM<Runtime>(3, _omitFieldNames ? '' : 'runtime',
        subBuilder: Runtime.create)
    ..aOM<CheckOp>(10, _omitFieldNames ? '' : 'check',
        subBuilder: CheckOp.create)
    ..aOM<SkipOp>(11, _omitFieldNames ? '' : 'skip', subBuilder: SkipOp.create)
    ..aOM<DownloadOp>(12, _omitFieldNames ? '' : 'download',
        subBuilder: DownloadOp.create)
    ..aOM<ApplyOp>(13, _omitFieldNames ? '' : 'apply',
        subBuilder: ApplyOp.create)
    ..aOM<StatusOp>(14, _omitFieldNames ? '' : 'status',
        subBuilder: StatusOp.create)
    ..aOM<CleanupOp>(15, _omitFieldNames ? '' : 'cleanup',
        subBuilder: CleanupOp.create)
    ..aOM<CancelOp>(16, _omitFieldNames ? '' : 'cancel',
        subBuilder: CancelOp.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  UpdaterRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  UpdaterRequest copyWith(void Function(UpdaterRequest) updates) =>
      super.copyWith((message) => updates(message as UpdaterRequest))
          as UpdaterRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static UpdaterRequest create() => UpdaterRequest._();
  @$core.override
  UpdaterRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static UpdaterRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<UpdaterRequest>(create);
  static UpdaterRequest? _defaultInstance;

  @$pb.TagNumber(10)
  @$pb.TagNumber(11)
  @$pb.TagNumber(12)
  @$pb.TagNumber(13)
  @$pb.TagNumber(14)
  @$pb.TagNumber(15)
  @$pb.TagNumber(16)
  UpdaterRequest_Op whichOp() => _UpdaterRequest_OpByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(10)
  @$pb.TagNumber(11)
  @$pb.TagNumber(12)
  @$pb.TagNumber(13)
  @$pb.TagNumber(14)
  @$pb.TagNumber(15)
  @$pb.TagNumber(16)
  void clearOp() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  ClientHello get hello => $_getN(0);
  @$pb.TagNumber(1)
  set hello(ClientHello value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasHello() => $_has(0);
  @$pb.TagNumber(1)
  void clearHello() => $_clearField(1);
  @$pb.TagNumber(1)
  ClientHello ensureHello() => $_ensure(0);

  @$pb.TagNumber(2)
  ClientProfile get profile => $_getN(1);
  @$pb.TagNumber(2)
  set profile(ClientProfile value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasProfile() => $_has(1);
  @$pb.TagNumber(2)
  void clearProfile() => $_clearField(2);
  @$pb.TagNumber(2)
  ClientProfile ensureProfile() => $_ensure(1);

  @$pb.TagNumber(3)
  Runtime get runtime => $_getN(2);
  @$pb.TagNumber(3)
  set runtime(Runtime value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasRuntime() => $_has(2);
  @$pb.TagNumber(3)
  void clearRuntime() => $_clearField(3);
  @$pb.TagNumber(3)
  Runtime ensureRuntime() => $_ensure(2);

  @$pb.TagNumber(10)
  CheckOp get check_10 => $_getN(3);
  @$pb.TagNumber(10)
  set check_10(CheckOp value) => $_setField(10, value);
  @$pb.TagNumber(10)
  $core.bool hasCheck_10() => $_has(3);
  @$pb.TagNumber(10)
  void clearCheck_10() => $_clearField(10);
  @$pb.TagNumber(10)
  CheckOp ensureCheck_10() => $_ensure(3);

  @$pb.TagNumber(11)
  SkipOp get skip => $_getN(4);
  @$pb.TagNumber(11)
  set skip(SkipOp value) => $_setField(11, value);
  @$pb.TagNumber(11)
  $core.bool hasSkip() => $_has(4);
  @$pb.TagNumber(11)
  void clearSkip() => $_clearField(11);
  @$pb.TagNumber(11)
  SkipOp ensureSkip() => $_ensure(4);

  @$pb.TagNumber(12)
  DownloadOp get download => $_getN(5);
  @$pb.TagNumber(12)
  set download(DownloadOp value) => $_setField(12, value);
  @$pb.TagNumber(12)
  $core.bool hasDownload() => $_has(5);
  @$pb.TagNumber(12)
  void clearDownload() => $_clearField(12);
  @$pb.TagNumber(12)
  DownloadOp ensureDownload() => $_ensure(5);

  @$pb.TagNumber(13)
  ApplyOp get apply => $_getN(6);
  @$pb.TagNumber(13)
  set apply(ApplyOp value) => $_setField(13, value);
  @$pb.TagNumber(13)
  $core.bool hasApply() => $_has(6);
  @$pb.TagNumber(13)
  void clearApply() => $_clearField(13);
  @$pb.TagNumber(13)
  ApplyOp ensureApply() => $_ensure(6);

  @$pb.TagNumber(14)
  StatusOp get status => $_getN(7);
  @$pb.TagNumber(14)
  set status(StatusOp value) => $_setField(14, value);
  @$pb.TagNumber(14)
  $core.bool hasStatus() => $_has(7);
  @$pb.TagNumber(14)
  void clearStatus() => $_clearField(14);
  @$pb.TagNumber(14)
  StatusOp ensureStatus() => $_ensure(7);

  @$pb.TagNumber(15)
  CleanupOp get cleanup => $_getN(8);
  @$pb.TagNumber(15)
  set cleanup(CleanupOp value) => $_setField(15, value);
  @$pb.TagNumber(15)
  $core.bool hasCleanup() => $_has(8);
  @$pb.TagNumber(15)
  void clearCleanup() => $_clearField(15);
  @$pb.TagNumber(15)
  CleanupOp ensureCleanup() => $_ensure(8);

  @$pb.TagNumber(16)
  CancelOp get cancel => $_getN(9);
  @$pb.TagNumber(16)
  set cancel(CancelOp value) => $_setField(16, value);
  @$pb.TagNumber(16)
  $core.bool hasCancel() => $_has(9);
  @$pb.TagNumber(16)
  void clearCancel() => $_clearField(16);
  @$pb.TagNumber(16)
  CancelOp ensureCancel() => $_ensure(9);
}

class Error extends $pb.GeneratedMessage {
  factory Error({
    ErrorCode? code,
    $core.bool? retryable,
    $core.String? message,
    $core.Iterable<$core.String>? attempts,
    RecoveryHelp? recovery,
  }) {
    final result = create();
    if (code != null) result.code = code;
    if (retryable != null) result.retryable = retryable;
    if (message != null) result.message = message;
    if (attempts != null) result.attempts.addAll(attempts);
    if (recovery != null) result.recovery = recovery;
    return result;
  }

  Error._();

  factory Error.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Error.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Error',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aE<ErrorCode>(1, _omitFieldNames ? '' : 'code',
        enumValues: ErrorCode.values)
    ..aOB(2, _omitFieldNames ? '' : 'retryable')
    ..aOS(3, _omitFieldNames ? '' : 'message')
    ..pPS(4, _omitFieldNames ? '' : 'attempts')
    ..aOM<RecoveryHelp>(5, _omitFieldNames ? '' : 'recovery',
        subBuilder: RecoveryHelp.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Error clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Error copyWith(void Function(Error) updates) =>
      super.copyWith((message) => updates(message as Error)) as Error;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Error create() => Error._();
  @$core.override
  Error createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Error getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Error>(create);
  static Error? _defaultInstance;

  @$pb.TagNumber(1)
  ErrorCode get code => $_getN(0);
  @$pb.TagNumber(1)
  set code(ErrorCode value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasCode() => $_has(0);
  @$pb.TagNumber(1)
  void clearCode() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get retryable => $_getBF(1);
  @$pb.TagNumber(2)
  set retryable($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasRetryable() => $_has(1);
  @$pb.TagNumber(2)
  void clearRetryable() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get message => $_getSZ(2);
  @$pb.TagNumber(3)
  set message($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMessage() => $_has(2);
  @$pb.TagNumber(3)
  void clearMessage() => $_clearField(3);

  @$pb.TagNumber(4)
  $pb.PbList<$core.String> get attempts => $_getList(3);

  @$pb.TagNumber(5)
  RecoveryHelp get recovery => $_getN(4);
  @$pb.TagNumber(5)
  set recovery(RecoveryHelp value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasRecovery() => $_has(4);
  @$pb.TagNumber(5)
  void clearRecovery() => $_clearField(5);
  @$pb.TagNumber(5)
  RecoveryHelp ensureRecovery() => $_ensure(4);
}

class PriorReleaseNotes extends $pb.GeneratedMessage {
  factory PriorReleaseNotes({
    $core.String? version,
    $fixnum.Int64? code,
    $core.String? notes,
    $core.String? notesUrl,
  }) {
    final result = create();
    if (version != null) result.version = version;
    if (code != null) result.code = code;
    if (notes != null) result.notes = notes;
    if (notesUrl != null) result.notesUrl = notesUrl;
    return result;
  }

  PriorReleaseNotes._();

  factory PriorReleaseNotes.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PriorReleaseNotes.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PriorReleaseNotes',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'version')
    ..aInt64(2, _omitFieldNames ? '' : 'code')
    ..aOS(3, _omitFieldNames ? '' : 'notes')
    ..aOS(4, _omitFieldNames ? '' : 'notesUrl')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PriorReleaseNotes clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PriorReleaseNotes copyWith(void Function(PriorReleaseNotes) updates) =>
      super.copyWith((message) => updates(message as PriorReleaseNotes))
          as PriorReleaseNotes;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PriorReleaseNotes create() => PriorReleaseNotes._();
  @$core.override
  PriorReleaseNotes createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PriorReleaseNotes getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PriorReleaseNotes>(create);
  static PriorReleaseNotes? _defaultInstance;

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
  $core.String get notes => $_getSZ(2);
  @$pb.TagNumber(3)
  set notes($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNotes() => $_has(2);
  @$pb.TagNumber(3)
  void clearNotes() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get notesUrl => $_getSZ(3);
  @$pb.TagNumber(4)
  set notesUrl($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasNotesUrl() => $_has(3);
  @$pb.TagNumber(4)
  void clearNotesUrl() => $_clearField(4);
}

class ArtifactView extends $pb.GeneratedMessage {
  factory ArtifactView({
    $core.String? name,
    $fixnum.Int64? size,
    $core.List<$core.int>? sha256,
  }) {
    final result = create();
    if (name != null) result.name = name;
    if (size != null) result.size = size;
    if (sha256 != null) result.sha256 = sha256;
    return result;
  }

  ArtifactView._();

  factory ArtifactView.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ArtifactView.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ArtifactView',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..aInt64(2, _omitFieldNames ? '' : 'size')
    ..a<$core.List<$core.int>>(
        3, _omitFieldNames ? '' : 'sha256', $pb.PbFieldType.OY)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ArtifactView clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ArtifactView copyWith(void Function(ArtifactView) updates) =>
      super.copyWith((message) => updates(message as ArtifactView))
          as ArtifactView;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ArtifactView create() => ArtifactView._();
  @$core.override
  ArtifactView createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ArtifactView getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ArtifactView>(create);
  static ArtifactView? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get size => $_getI64(1);
  @$pb.TagNumber(2)
  set size($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSize() => $_has(1);
  @$pb.TagNumber(2)
  void clearSize() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.List<$core.int> get sha256 => $_getN(2);
  @$pb.TagNumber(3)
  set sha256($core.List<$core.int> value) => $_setBytes(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSha256() => $_has(2);
  @$pb.TagNumber(3)
  void clearSha256() => $_clearField(3);
}

class UpToDate extends $pb.GeneratedMessage {
  factory UpToDate({
    $fixnum.Int64? sequence,
    $core.bool? currentIsYanked,
  }) {
    final result = create();
    if (sequence != null) result.sequence = sequence;
    if (currentIsYanked != null) result.currentIsYanked = currentIsYanked;
    return result;
  }

  UpToDate._();

  factory UpToDate.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory UpToDate.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'UpToDate',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'sequence')
    ..aOB(2, _omitFieldNames ? '' : 'currentIsYanked')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  UpToDate clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  UpToDate copyWith(void Function(UpToDate) updates) =>
      super.copyWith((message) => updates(message as UpToDate)) as UpToDate;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static UpToDate create() => UpToDate._();
  @$core.override
  UpToDate createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static UpToDate getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<UpToDate>(create);
  static UpToDate? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get sequence => $_getI64(0);
  @$pb.TagNumber(1)
  set sequence($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSequence() => $_has(0);
  @$pb.TagNumber(1)
  void clearSequence() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get currentIsYanked => $_getBF(1);
  @$pb.TagNumber(2)
  set currentIsYanked($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCurrentIsYanked() => $_has(1);
  @$pb.TagNumber(2)
  void clearCurrentIsYanked() => $_clearField(2);
}

class UpdateAvailable extends $pb.GeneratedMessage {
  factory UpdateAvailable({
    $core.String? planId,
    $core.String? promptKey,
    $core.String? version,
    $fixnum.Int64? code,
    $core.bool? mandatory,
    $core.int? remainingHops,
    $fixnum.Int64? sequence,
    $core.String? releaseNotesMarkdown,
    $core.String? releaseNotesUrl,
    $core.Iterable<PriorReleaseNotes>? priorReleaseNotes,
    $core.Iterable<ArtifactView>? artifacts,
  }) {
    final result = create();
    if (planId != null) result.planId = planId;
    if (promptKey != null) result.promptKey = promptKey;
    if (version != null) result.version = version;
    if (code != null) result.code = code;
    if (mandatory != null) result.mandatory = mandatory;
    if (remainingHops != null) result.remainingHops = remainingHops;
    if (sequence != null) result.sequence = sequence;
    if (releaseNotesMarkdown != null)
      result.releaseNotesMarkdown = releaseNotesMarkdown;
    if (releaseNotesUrl != null) result.releaseNotesUrl = releaseNotesUrl;
    if (priorReleaseNotes != null)
      result.priorReleaseNotes.addAll(priorReleaseNotes);
    if (artifacts != null) result.artifacts.addAll(artifacts);
    return result;
  }

  UpdateAvailable._();

  factory UpdateAvailable.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory UpdateAvailable.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'UpdateAvailable',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'planId')
    ..aOS(2, _omitFieldNames ? '' : 'promptKey')
    ..aOS(3, _omitFieldNames ? '' : 'version')
    ..aInt64(4, _omitFieldNames ? '' : 'code')
    ..aOB(5, _omitFieldNames ? '' : 'mandatory')
    ..aI(6, _omitFieldNames ? '' : 'remainingHops')
    ..aInt64(7, _omitFieldNames ? '' : 'sequence')
    ..aOS(8, _omitFieldNames ? '' : 'releaseNotesMarkdown')
    ..aOS(9, _omitFieldNames ? '' : 'releaseNotesUrl')
    ..pPM<PriorReleaseNotes>(10, _omitFieldNames ? '' : 'priorReleaseNotes',
        subBuilder: PriorReleaseNotes.create)
    ..pPM<ArtifactView>(11, _omitFieldNames ? '' : 'artifacts',
        subBuilder: ArtifactView.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  UpdateAvailable clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  UpdateAvailable copyWith(void Function(UpdateAvailable) updates) =>
      super.copyWith((message) => updates(message as UpdateAvailable))
          as UpdateAvailable;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static UpdateAvailable create() => UpdateAvailable._();
  @$core.override
  UpdateAvailable createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static UpdateAvailable getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<UpdateAvailable>(create);
  static UpdateAvailable? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get planId => $_getSZ(0);
  @$pb.TagNumber(1)
  set planId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPlanId() => $_has(0);
  @$pb.TagNumber(1)
  void clearPlanId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get promptKey => $_getSZ(1);
  @$pb.TagNumber(2)
  set promptKey($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPromptKey() => $_has(1);
  @$pb.TagNumber(2)
  void clearPromptKey() => $_clearField(2);

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
  $core.bool get mandatory => $_getBF(4);
  @$pb.TagNumber(5)
  set mandatory($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasMandatory() => $_has(4);
  @$pb.TagNumber(5)
  void clearMandatory() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.int get remainingHops => $_getIZ(5);
  @$pb.TagNumber(6)
  set remainingHops($core.int value) => $_setSignedInt32(5, value);
  @$pb.TagNumber(6)
  $core.bool hasRemainingHops() => $_has(5);
  @$pb.TagNumber(6)
  void clearRemainingHops() => $_clearField(6);

  @$pb.TagNumber(7)
  $fixnum.Int64 get sequence => $_getI64(6);
  @$pb.TagNumber(7)
  set sequence($fixnum.Int64 value) => $_setInt64(6, value);
  @$pb.TagNumber(7)
  $core.bool hasSequence() => $_has(6);
  @$pb.TagNumber(7)
  void clearSequence() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get releaseNotesMarkdown => $_getSZ(7);
  @$pb.TagNumber(8)
  set releaseNotesMarkdown($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasReleaseNotesMarkdown() => $_has(7);
  @$pb.TagNumber(8)
  void clearReleaseNotesMarkdown() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get releaseNotesUrl => $_getSZ(8);
  @$pb.TagNumber(9)
  set releaseNotesUrl($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasReleaseNotesUrl() => $_has(8);
  @$pb.TagNumber(9)
  void clearReleaseNotesUrl() => $_clearField(9);

  @$pb.TagNumber(10)
  $pb.PbList<PriorReleaseNotes> get priorReleaseNotes => $_getList(9);

  @$pb.TagNumber(11)
  $pb.PbList<ArtifactView> get artifacts => $_getList(10);
}

class FallbackRequired extends $pb.GeneratedMessage {
  factory FallbackRequired({
    $core.String? promptKey,
    $core.String? manualUrl,
    $core.String? message,
    $core.bool? mandatory,
    $fixnum.Int64? sequence,
    $fixnum.Int64? minCode,
    $fixnum.Int64? maxCode,
  }) {
    final result = create();
    if (promptKey != null) result.promptKey = promptKey;
    if (manualUrl != null) result.manualUrl = manualUrl;
    if (message != null) result.message = message;
    if (mandatory != null) result.mandatory = mandatory;
    if (sequence != null) result.sequence = sequence;
    if (minCode != null) result.minCode = minCode;
    if (maxCode != null) result.maxCode = maxCode;
    return result;
  }

  FallbackRequired._();

  factory FallbackRequired.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FallbackRequired.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FallbackRequired',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'promptKey')
    ..aOS(2, _omitFieldNames ? '' : 'manualUrl')
    ..aOS(3, _omitFieldNames ? '' : 'message')
    ..aOB(4, _omitFieldNames ? '' : 'mandatory')
    ..aInt64(5, _omitFieldNames ? '' : 'sequence')
    ..aInt64(6, _omitFieldNames ? '' : 'minCode')
    ..aInt64(7, _omitFieldNames ? '' : 'maxCode')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FallbackRequired clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FallbackRequired copyWith(void Function(FallbackRequired) updates) =>
      super.copyWith((message) => updates(message as FallbackRequired))
          as FallbackRequired;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FallbackRequired create() => FallbackRequired._();
  @$core.override
  FallbackRequired createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FallbackRequired getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FallbackRequired>(create);
  static FallbackRequired? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get promptKey => $_getSZ(0);
  @$pb.TagNumber(1)
  set promptKey($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPromptKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearPromptKey() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get manualUrl => $_getSZ(1);
  @$pb.TagNumber(2)
  set manualUrl($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasManualUrl() => $_has(1);
  @$pb.TagNumber(2)
  void clearManualUrl() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get message => $_getSZ(2);
  @$pb.TagNumber(3)
  set message($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMessage() => $_has(2);
  @$pb.TagNumber(3)
  void clearMessage() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get mandatory => $_getBF(3);
  @$pb.TagNumber(4)
  set mandatory($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasMandatory() => $_has(3);
  @$pb.TagNumber(4)
  void clearMandatory() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get sequence => $_getI64(4);
  @$pb.TagNumber(5)
  set sequence($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSequence() => $_has(4);
  @$pb.TagNumber(5)
  void clearSequence() => $_clearField(5);

  @$pb.TagNumber(6)
  $fixnum.Int64 get minCode => $_getI64(5);
  @$pb.TagNumber(6)
  set minCode($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(6)
  $core.bool hasMinCode() => $_has(5);
  @$pb.TagNumber(6)
  void clearMinCode() => $_clearField(6);

  @$pb.TagNumber(7)
  $fixnum.Int64 get maxCode => $_getI64(6);
  @$pb.TagNumber(7)
  set maxCode($fixnum.Int64 value) => $_setInt64(6, value);
  @$pb.TagNumber(7)
  $core.bool hasMaxCode() => $_has(6);
  @$pb.TagNumber(7)
  void clearMaxCode() => $_clearField(7);
}

class Throttled extends $pb.GeneratedMessage {
  factory Throttled({
    $1.Timestamp? nextAllowedAt,
  }) {
    final result = create();
    if (nextAllowedAt != null) result.nextAllowedAt = nextAllowedAt;
    return result;
  }

  Throttled._();

  factory Throttled.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Throttled.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Throttled',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOM<$1.Timestamp>(1, _omitFieldNames ? '' : 'nextAllowedAt',
        subBuilder: $1.Timestamp.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Throttled clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Throttled copyWith(void Function(Throttled) updates) =>
      super.copyWith((message) => updates(message as Throttled)) as Throttled;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Throttled create() => Throttled._();
  @$core.override
  Throttled createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Throttled getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Throttled>(create);
  static Throttled? _defaultInstance;

  @$pb.TagNumber(1)
  $1.Timestamp get nextAllowedAt => $_getN(0);
  @$pb.TagNumber(1)
  set nextAllowedAt($1.Timestamp value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasNextAllowedAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearNextAllowedAt() => $_clearField(1);
  @$pb.TagNumber(1)
  $1.Timestamp ensureNextAllowedAt() => $_ensure(0);
}

class Failed extends $pb.GeneratedMessage {
  factory Failed({
    Error? error,
  }) {
    final result = create();
    if (error != null) result.error = error;
    return result;
  }

  Failed._();

  factory Failed.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Failed.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Failed',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOM<Error>(1, _omitFieldNames ? '' : 'error', subBuilder: Error.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Failed clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Failed copyWith(void Function(Failed) updates) =>
      super.copyWith((message) => updates(message as Failed)) as Failed;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Failed create() => Failed._();
  @$core.override
  Failed createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Failed getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Failed>(create);
  static Failed? _defaultInstance;

  @$pb.TagNumber(1)
  Error get error => $_getN(0);
  @$pb.TagNumber(1)
  set error(Error value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasError() => $_has(0);
  @$pb.TagNumber(1)
  void clearError() => $_clearField(1);
  @$pb.TagNumber(1)
  Error ensureError() => $_ensure(0);
}

enum CheckResult_Kind {
  upToDate,
  updateAvailable,
  fallbackRequired,
  throttled,
  failed,
  notSet
}

class CheckResult extends $pb.GeneratedMessage {
  factory CheckResult({
    UpToDate? upToDate,
    UpdateAvailable? updateAvailable,
    FallbackRequired? fallbackRequired,
    Throttled? throttled,
    Failed? failed,
  }) {
    final result = create();
    if (upToDate != null) result.upToDate = upToDate;
    if (updateAvailable != null) result.updateAvailable = updateAvailable;
    if (fallbackRequired != null) result.fallbackRequired = fallbackRequired;
    if (throttled != null) result.throttled = throttled;
    if (failed != null) result.failed = failed;
    return result;
  }

  CheckResult._();

  factory CheckResult.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CheckResult.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, CheckResult_Kind> _CheckResult_KindByTag = {
    1: CheckResult_Kind.upToDate,
    2: CheckResult_Kind.updateAvailable,
    3: CheckResult_Kind.fallbackRequired,
    4: CheckResult_Kind.throttled,
    5: CheckResult_Kind.failed,
    0: CheckResult_Kind.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CheckResult',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..oo(0, [1, 2, 3, 4, 5])
    ..aOM<UpToDate>(1, _omitFieldNames ? '' : 'upToDate',
        subBuilder: UpToDate.create)
    ..aOM<UpdateAvailable>(2, _omitFieldNames ? '' : 'updateAvailable',
        subBuilder: UpdateAvailable.create)
    ..aOM<FallbackRequired>(3, _omitFieldNames ? '' : 'fallbackRequired',
        subBuilder: FallbackRequired.create)
    ..aOM<Throttled>(4, _omitFieldNames ? '' : 'throttled',
        subBuilder: Throttled.create)
    ..aOM<Failed>(5, _omitFieldNames ? '' : 'failed', subBuilder: Failed.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CheckResult clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CheckResult copyWith(void Function(CheckResult) updates) =>
      super.copyWith((message) => updates(message as CheckResult))
          as CheckResult;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CheckResult create() => CheckResult._();
  @$core.override
  CheckResult createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CheckResult getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CheckResult>(create);
  static CheckResult? _defaultInstance;

  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  @$pb.TagNumber(5)
  CheckResult_Kind whichKind() => _CheckResult_KindByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  @$pb.TagNumber(5)
  void clearKind() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  UpToDate get upToDate => $_getN(0);
  @$pb.TagNumber(1)
  set upToDate(UpToDate value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasUpToDate() => $_has(0);
  @$pb.TagNumber(1)
  void clearUpToDate() => $_clearField(1);
  @$pb.TagNumber(1)
  UpToDate ensureUpToDate() => $_ensure(0);

  @$pb.TagNumber(2)
  UpdateAvailable get updateAvailable => $_getN(1);
  @$pb.TagNumber(2)
  set updateAvailable(UpdateAvailable value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasUpdateAvailable() => $_has(1);
  @$pb.TagNumber(2)
  void clearUpdateAvailable() => $_clearField(2);
  @$pb.TagNumber(2)
  UpdateAvailable ensureUpdateAvailable() => $_ensure(1);

  @$pb.TagNumber(3)
  FallbackRequired get fallbackRequired => $_getN(2);
  @$pb.TagNumber(3)
  set fallbackRequired(FallbackRequired value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasFallbackRequired() => $_has(2);
  @$pb.TagNumber(3)
  void clearFallbackRequired() => $_clearField(3);
  @$pb.TagNumber(3)
  FallbackRequired ensureFallbackRequired() => $_ensure(2);

  @$pb.TagNumber(4)
  Throttled get throttled => $_getN(3);
  @$pb.TagNumber(4)
  set throttled(Throttled value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasThrottled() => $_has(3);
  @$pb.TagNumber(4)
  void clearThrottled() => $_clearField(4);
  @$pb.TagNumber(4)
  Throttled ensureThrottled() => $_ensure(3);

  @$pb.TagNumber(5)
  Failed get failed => $_getN(4);
  @$pb.TagNumber(5)
  set failed(Failed value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasFailed() => $_has(4);
  @$pb.TagNumber(5)
  void clearFailed() => $_clearField(5);
  @$pb.TagNumber(5)
  Failed ensureFailed() => $_ensure(4);
}

class Downloaded extends $pb.GeneratedMessage {
  factory Downloaded({
    $core.String? planId,
    $fixnum.Int64? bytes,
  }) {
    final result = create();
    if (planId != null) result.planId = planId;
    if (bytes != null) result.bytes = bytes;
    return result;
  }

  Downloaded._();

  factory Downloaded.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Downloaded.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Downloaded',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'planId')
    ..aInt64(2, _omitFieldNames ? '' : 'bytes')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Downloaded clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Downloaded copyWith(void Function(Downloaded) updates) =>
      super.copyWith((message) => updates(message as Downloaded)) as Downloaded;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Downloaded create() => Downloaded._();
  @$core.override
  Downloaded createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Downloaded getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Downloaded>(create);
  static Downloaded? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get planId => $_getSZ(0);
  @$pb.TagNumber(1)
  set planId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPlanId() => $_has(0);
  @$pb.TagNumber(1)
  void clearPlanId() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get bytes => $_getI64(1);
  @$pb.TagNumber(2)
  set bytes($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBytes() => $_has(1);
  @$pb.TagNumber(2)
  void clearBytes() => $_clearField(2);
}

enum DownloadResult_Kind { downloaded, failed, notSet }

class DownloadResult extends $pb.GeneratedMessage {
  factory DownloadResult({
    Downloaded? downloaded,
    Failed? failed,
  }) {
    final result = create();
    if (downloaded != null) result.downloaded = downloaded;
    if (failed != null) result.failed = failed;
    return result;
  }

  DownloadResult._();

  factory DownloadResult.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DownloadResult.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, DownloadResult_Kind>
      _DownloadResult_KindByTag = {
    1: DownloadResult_Kind.downloaded,
    2: DownloadResult_Kind.failed,
    0: DownloadResult_Kind.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DownloadResult',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..oo(0, [1, 2])
    ..aOM<Downloaded>(1, _omitFieldNames ? '' : 'downloaded',
        subBuilder: Downloaded.create)
    ..aOM<Failed>(2, _omitFieldNames ? '' : 'failed', subBuilder: Failed.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DownloadResult clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DownloadResult copyWith(void Function(DownloadResult) updates) =>
      super.copyWith((message) => updates(message as DownloadResult))
          as DownloadResult;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DownloadResult create() => DownloadResult._();
  @$core.override
  DownloadResult createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DownloadResult getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DownloadResult>(create);
  static DownloadResult? _defaultInstance;

  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  DownloadResult_Kind whichKind() =>
      _DownloadResult_KindByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  void clearKind() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  Downloaded get downloaded => $_getN(0);
  @$pb.TagNumber(1)
  set downloaded(Downloaded value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasDownloaded() => $_has(0);
  @$pb.TagNumber(1)
  void clearDownloaded() => $_clearField(1);
  @$pb.TagNumber(1)
  Downloaded ensureDownloaded() => $_ensure(0);

  @$pb.TagNumber(2)
  Failed get failed => $_getN(1);
  @$pb.TagNumber(2)
  set failed(Failed value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasFailed() => $_has(1);
  @$pb.TagNumber(2)
  void clearFailed() => $_clearField(2);
  @$pb.TagNumber(2)
  Failed ensureFailed() => $_ensure(1);
}

class ApplyAccepted extends $pb.GeneratedMessage {
  factory ApplyAccepted({
    $core.String? sessionId,
    $core.String? planId,
    $core.bool? requiresHostExit,
  }) {
    final result = create();
    if (sessionId != null) result.sessionId = sessionId;
    if (planId != null) result.planId = planId;
    if (requiresHostExit != null) result.requiresHostExit = requiresHostExit;
    return result;
  }

  ApplyAccepted._();

  factory ApplyAccepted.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ApplyAccepted.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ApplyAccepted',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..aOS(2, _omitFieldNames ? '' : 'planId')
    ..aOB(3, _omitFieldNames ? '' : 'requiresHostExit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplyAccepted clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplyAccepted copyWith(void Function(ApplyAccepted) updates) =>
      super.copyWith((message) => updates(message as ApplyAccepted))
          as ApplyAccepted;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ApplyAccepted create() => ApplyAccepted._();
  @$core.override
  ApplyAccepted createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ApplyAccepted getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ApplyAccepted>(create);
  static ApplyAccepted? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sessionId => $_getSZ(0);
  @$pb.TagNumber(1)
  set sessionId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSessionId() => $_has(0);
  @$pb.TagNumber(1)
  void clearSessionId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get planId => $_getSZ(1);
  @$pb.TagNumber(2)
  set planId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPlanId() => $_has(1);
  @$pb.TagNumber(2)
  void clearPlanId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get requiresHostExit => $_getBF(2);
  @$pb.TagNumber(3)
  set requiresHostExit($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasRequiresHostExit() => $_has(2);
  @$pb.TagNumber(3)
  void clearRequiresHostExit() => $_clearField(3);
}

enum ApplyResult_Kind { accepted, failed, notSet }

class ApplyResult extends $pb.GeneratedMessage {
  factory ApplyResult({
    ApplyAccepted? accepted,
    Failed? failed,
  }) {
    final result = create();
    if (accepted != null) result.accepted = accepted;
    if (failed != null) result.failed = failed;
    return result;
  }

  ApplyResult._();

  factory ApplyResult.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ApplyResult.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, ApplyResult_Kind> _ApplyResult_KindByTag = {
    1: ApplyResult_Kind.accepted,
    2: ApplyResult_Kind.failed,
    0: ApplyResult_Kind.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ApplyResult',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..oo(0, [1, 2])
    ..aOM<ApplyAccepted>(1, _omitFieldNames ? '' : 'accepted',
        subBuilder: ApplyAccepted.create)
    ..aOM<Failed>(2, _omitFieldNames ? '' : 'failed', subBuilder: Failed.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplyResult clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplyResult copyWith(void Function(ApplyResult) updates) =>
      super.copyWith((message) => updates(message as ApplyResult))
          as ApplyResult;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ApplyResult create() => ApplyResult._();
  @$core.override
  ApplyResult createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ApplyResult getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ApplyResult>(create);
  static ApplyResult? _defaultInstance;

  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  ApplyResult_Kind whichKind() => _ApplyResult_KindByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  void clearKind() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  ApplyAccepted get accepted => $_getN(0);
  @$pb.TagNumber(1)
  set accepted(ApplyAccepted value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAccepted() => $_has(0);
  @$pb.TagNumber(1)
  void clearAccepted() => $_clearField(1);
  @$pb.TagNumber(1)
  ApplyAccepted ensureAccepted() => $_ensure(0);

  @$pb.TagNumber(2)
  Failed get failed => $_getN(1);
  @$pb.TagNumber(2)
  set failed(Failed value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasFailed() => $_has(1);
  @$pb.TagNumber(2)
  void clearFailed() => $_clearField(2);
  @$pb.TagNumber(2)
  Failed ensureFailed() => $_ensure(1);
}

class Ok extends $pb.GeneratedMessage {
  factory Ok() => create();

  Ok._();

  factory Ok.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Ok.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Ok',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Ok clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Ok copyWith(void Function(Ok) updates) =>
      super.copyWith((message) => updates(message as Ok)) as Ok;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Ok create() => Ok._();
  @$core.override
  Ok createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Ok getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Ok>(create);
  static Ok? _defaultInstance;
}

enum Result_Kind { ok, failed, notSet }

class Result extends $pb.GeneratedMessage {
  factory Result({
    Ok? ok,
    Failed? failed,
  }) {
    final result = create();
    if (ok != null) result.ok = ok;
    if (failed != null) result.failed = failed;
    return result;
  }

  Result._();

  factory Result.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Result.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, Result_Kind> _Result_KindByTag = {
    1: Result_Kind.ok,
    2: Result_Kind.failed,
    0: Result_Kind.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Result',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..oo(0, [1, 2])
    ..aOM<Ok>(1, _omitFieldNames ? '' : 'ok', subBuilder: Ok.create)
    ..aOM<Failed>(2, _omitFieldNames ? '' : 'failed', subBuilder: Failed.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Result clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Result copyWith(void Function(Result) updates) =>
      super.copyWith((message) => updates(message as Result)) as Result;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Result create() => Result._();
  @$core.override
  Result createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Result getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Result>(create);
  static Result? _defaultInstance;

  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  Result_Kind whichKind() => _Result_KindByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  void clearKind() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  Ok get ok => $_getN(0);
  @$pb.TagNumber(1)
  set ok(Ok value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasOk() => $_has(0);
  @$pb.TagNumber(1)
  void clearOk() => $_clearField(1);
  @$pb.TagNumber(1)
  Ok ensureOk() => $_ensure(0);

  @$pb.TagNumber(2)
  Failed get failed => $_getN(1);
  @$pb.TagNumber(2)
  set failed(Failed value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasFailed() => $_has(1);
  @$pb.TagNumber(2)
  void clearFailed() => $_clearField(2);
  @$pb.TagNumber(2)
  Failed ensureFailed() => $_ensure(1);
}

class SidecarInfo extends $pb.GeneratedMessage {
  factory SidecarInfo({
    $core.String? path,
    $core.int? ipc,
    $core.String? version,
  }) {
    final result = create();
    if (path != null) result.path = path;
    if (ipc != null) result.ipc = ipc;
    if (version != null) result.version = version;
    return result;
  }

  SidecarInfo._();

  factory SidecarInfo.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SidecarInfo.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SidecarInfo',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'path')
    ..aI(2, _omitFieldNames ? '' : 'ipc', fieldType: $pb.PbFieldType.OU3)
    ..aOS(3, _omitFieldNames ? '' : 'version')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SidecarInfo clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SidecarInfo copyWith(void Function(SidecarInfo) updates) =>
      super.copyWith((message) => updates(message as SidecarInfo))
          as SidecarInfo;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SidecarInfo create() => SidecarInfo._();
  @$core.override
  SidecarInfo createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SidecarInfo getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SidecarInfo>(create);
  static SidecarInfo? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get path => $_getSZ(0);
  @$pb.TagNumber(1)
  set path($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPath() => $_has(0);
  @$pb.TagNumber(1)
  void clearPath() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.int get ipc => $_getIZ(1);
  @$pb.TagNumber(2)
  set ipc($core.int value) => $_setUnsignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasIpc() => $_has(1);
  @$pb.TagNumber(2)
  void clearIpc() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get version => $_getSZ(2);
  @$pb.TagNumber(3)
  set version($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasVersion() => $_has(2);
  @$pb.TagNumber(3)
  void clearVersion() => $_clearField(3);
}

class SessionView extends $pb.GeneratedMessage {
  factory SessionView({
    $core.String? sessionId,
    $core.String? planId,
    SessionPhase? phase,
    $1.Timestamp? startedAt,
    Error? error,
  }) {
    final result = create();
    if (sessionId != null) result.sessionId = sessionId;
    if (planId != null) result.planId = planId;
    if (phase != null) result.phase = phase;
    if (startedAt != null) result.startedAt = startedAt;
    if (error != null) result.error = error;
    return result;
  }

  SessionView._();

  factory SessionView.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SessionView.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SessionView',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..aOS(2, _omitFieldNames ? '' : 'planId')
    ..aE<SessionPhase>(3, _omitFieldNames ? '' : 'phase',
        enumValues: SessionPhase.values)
    ..aOM<$1.Timestamp>(4, _omitFieldNames ? '' : 'startedAt',
        subBuilder: $1.Timestamp.create)
    ..aOM<Error>(5, _omitFieldNames ? '' : 'error', subBuilder: Error.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SessionView clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SessionView copyWith(void Function(SessionView) updates) =>
      super.copyWith((message) => updates(message as SessionView))
          as SessionView;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SessionView create() => SessionView._();
  @$core.override
  SessionView createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SessionView getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SessionView>(create);
  static SessionView? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sessionId => $_getSZ(0);
  @$pb.TagNumber(1)
  set sessionId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSessionId() => $_has(0);
  @$pb.TagNumber(1)
  void clearSessionId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get planId => $_getSZ(1);
  @$pb.TagNumber(2)
  set planId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPlanId() => $_has(1);
  @$pb.TagNumber(2)
  void clearPlanId() => $_clearField(2);

  @$pb.TagNumber(3)
  SessionPhase get phase => $_getN(2);
  @$pb.TagNumber(3)
  set phase(SessionPhase value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasPhase() => $_has(2);
  @$pb.TagNumber(3)
  void clearPhase() => $_clearField(3);

  @$pb.TagNumber(4)
  $1.Timestamp get startedAt => $_getN(3);
  @$pb.TagNumber(4)
  set startedAt($1.Timestamp value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasStartedAt() => $_has(3);
  @$pb.TagNumber(4)
  void clearStartedAt() => $_clearField(4);
  @$pb.TagNumber(4)
  $1.Timestamp ensureStartedAt() => $_ensure(3);

  @$pb.TagNumber(5)
  Error get error => $_getN(4);
  @$pb.TagNumber(5)
  set error(Error value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasError() => $_has(4);
  @$pb.TagNumber(5)
  void clearError() => $_clearField(5);
  @$pb.TagNumber(5)
  Error ensureError() => $_ensure(4);
}

class StatusSnapshot extends $pb.GeneratedMessage {
  factory StatusSnapshot({
    $1.Timestamp? lastCheckAt,
    LastResult? lastResult,
    $1.Timestamp? nextAllowedAt,
    $fixnum.Int64? lastSeenSequence,
    $core.Iterable<$fixnum.Int64>? skippedCodes,
    SessionView? activeSession,
    SidecarInfo? sidecar,
  }) {
    final result = create();
    if (lastCheckAt != null) result.lastCheckAt = lastCheckAt;
    if (lastResult != null) result.lastResult = lastResult;
    if (nextAllowedAt != null) result.nextAllowedAt = nextAllowedAt;
    if (lastSeenSequence != null) result.lastSeenSequence = lastSeenSequence;
    if (skippedCodes != null) result.skippedCodes.addAll(skippedCodes);
    if (activeSession != null) result.activeSession = activeSession;
    if (sidecar != null) result.sidecar = sidecar;
    return result;
  }

  StatusSnapshot._();

  factory StatusSnapshot.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory StatusSnapshot.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StatusSnapshot',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOM<$1.Timestamp>(1, _omitFieldNames ? '' : 'lastCheckAt',
        subBuilder: $1.Timestamp.create)
    ..aE<LastResult>(2, _omitFieldNames ? '' : 'lastResult',
        enumValues: LastResult.values)
    ..aOM<$1.Timestamp>(3, _omitFieldNames ? '' : 'nextAllowedAt',
        subBuilder: $1.Timestamp.create)
    ..aInt64(4, _omitFieldNames ? '' : 'lastSeenSequence')
    ..p<$fixnum.Int64>(
        5, _omitFieldNames ? '' : 'skippedCodes', $pb.PbFieldType.K6)
    ..aOM<SessionView>(6, _omitFieldNames ? '' : 'activeSession',
        subBuilder: SessionView.create)
    ..aOM<SidecarInfo>(7, _omitFieldNames ? '' : 'sidecar',
        subBuilder: SidecarInfo.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StatusSnapshot clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StatusSnapshot copyWith(void Function(StatusSnapshot) updates) =>
      super.copyWith((message) => updates(message as StatusSnapshot))
          as StatusSnapshot;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static StatusSnapshot create() => StatusSnapshot._();
  @$core.override
  StatusSnapshot createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static StatusSnapshot getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<StatusSnapshot>(create);
  static StatusSnapshot? _defaultInstance;

  @$pb.TagNumber(1)
  $1.Timestamp get lastCheckAt => $_getN(0);
  @$pb.TagNumber(1)
  set lastCheckAt($1.Timestamp value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasLastCheckAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearLastCheckAt() => $_clearField(1);
  @$pb.TagNumber(1)
  $1.Timestamp ensureLastCheckAt() => $_ensure(0);

  @$pb.TagNumber(2)
  LastResult get lastResult => $_getN(1);
  @$pb.TagNumber(2)
  set lastResult(LastResult value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasLastResult() => $_has(1);
  @$pb.TagNumber(2)
  void clearLastResult() => $_clearField(2);

  @$pb.TagNumber(3)
  $1.Timestamp get nextAllowedAt => $_getN(2);
  @$pb.TagNumber(3)
  set nextAllowedAt($1.Timestamp value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasNextAllowedAt() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextAllowedAt() => $_clearField(3);
  @$pb.TagNumber(3)
  $1.Timestamp ensureNextAllowedAt() => $_ensure(2);

  @$pb.TagNumber(4)
  $fixnum.Int64 get lastSeenSequence => $_getI64(3);
  @$pb.TagNumber(4)
  set lastSeenSequence($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasLastSeenSequence() => $_has(3);
  @$pb.TagNumber(4)
  void clearLastSeenSequence() => $_clearField(4);

  @$pb.TagNumber(5)
  $pb.PbList<$fixnum.Int64> get skippedCodes => $_getList(4);

  @$pb.TagNumber(6)
  SessionView get activeSession => $_getN(5);
  @$pb.TagNumber(6)
  set activeSession(SessionView value) => $_setField(6, value);
  @$pb.TagNumber(6)
  $core.bool hasActiveSession() => $_has(5);
  @$pb.TagNumber(6)
  void clearActiveSession() => $_clearField(6);
  @$pb.TagNumber(6)
  SessionView ensureActiveSession() => $_ensure(5);

  @$pb.TagNumber(7)
  SidecarInfo get sidecar => $_getN(6);
  @$pb.TagNumber(7)
  set sidecar(SidecarInfo value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasSidecar() => $_has(6);
  @$pb.TagNumber(7)
  void clearSidecar() => $_clearField(7);
  @$pb.TagNumber(7)
  SidecarInfo ensureSidecar() => $_ensure(6);
}

class Progress extends $pb.GeneratedMessage {
  factory Progress({
    $fixnum.Int64? bytesReceived,
    $fixnum.Int64? bytesTotal,
    $fixnum.Int64? bytesPerSecond,
  }) {
    final result = create();
    if (bytesReceived != null) result.bytesReceived = bytesReceived;
    if (bytesTotal != null) result.bytesTotal = bytesTotal;
    if (bytesPerSecond != null) result.bytesPerSecond = bytesPerSecond;
    return result;
  }

  Progress._();

  factory Progress.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Progress.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Progress',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'bytesReceived')
    ..aInt64(2, _omitFieldNames ? '' : 'bytesTotal')
    ..aInt64(3, _omitFieldNames ? '' : 'bytesPerSecond')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Progress clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Progress copyWith(void Function(Progress) updates) =>
      super.copyWith((message) => updates(message as Progress)) as Progress;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Progress create() => Progress._();
  @$core.override
  Progress createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Progress getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Progress>(create);
  static Progress? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get bytesReceived => $_getI64(0);
  @$pb.TagNumber(1)
  set bytesReceived($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasBytesReceived() => $_has(0);
  @$pb.TagNumber(1)
  void clearBytesReceived() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get bytesTotal => $_getI64(1);
  @$pb.TagNumber(2)
  set bytesTotal($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBytesTotal() => $_has(1);
  @$pb.TagNumber(2)
  void clearBytesTotal() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get bytesPerSecond => $_getI64(2);
  @$pb.TagNumber(3)
  set bytesPerSecond($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasBytesPerSecond() => $_has(2);
  @$pb.TagNumber(3)
  void clearBytesPerSecond() => $_clearField(3);
}

class ApplyProgress extends $pb.GeneratedMessage {
  factory ApplyProgress({
    $core.String? sessionId,
    SessionPhase? phase,
  }) {
    final result = create();
    if (sessionId != null) result.sessionId = sessionId;
    if (phase != null) result.phase = phase;
    return result;
  }

  ApplyProgress._();

  factory ApplyProgress.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ApplyProgress.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ApplyProgress',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..aE<SessionPhase>(2, _omitFieldNames ? '' : 'phase',
        enumValues: SessionPhase.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplyProgress clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplyProgress copyWith(void Function(ApplyProgress) updates) =>
      super.copyWith((message) => updates(message as ApplyProgress))
          as ApplyProgress;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ApplyProgress create() => ApplyProgress._();
  @$core.override
  ApplyProgress createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ApplyProgress getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ApplyProgress>(create);
  static ApplyProgress? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sessionId => $_getSZ(0);
  @$pb.TagNumber(1)
  set sessionId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSessionId() => $_has(0);
  @$pb.TagNumber(1)
  void clearSessionId() => $_clearField(1);

  @$pb.TagNumber(2)
  SessionPhase get phase => $_getN(1);
  @$pb.TagNumber(2)
  set phase(SessionPhase value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasPhase() => $_has(1);
  @$pb.TagNumber(2)
  void clearPhase() => $_clearField(2);
}

class Log extends $pb.GeneratedMessage {
  factory Log({
    $core.String? message,
  }) {
    final result = create();
    if (message != null) result.message = message;
    return result;
  }

  Log._();

  factory Log.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Log.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Log',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'message')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Log clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Log copyWith(void Function(Log) updates) =>
      super.copyWith((message) => updates(message as Log)) as Log;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Log create() => Log._();
  @$core.override
  Log createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Log getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Log>(create);
  static Log? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get message => $_getSZ(0);
  @$pb.TagNumber(1)
  set message($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasMessage() => $_has(0);
  @$pb.TagNumber(1)
  void clearMessage() => $_clearField(1);
}

enum UpdaterEvent_Kind {
  capabilities,
  progress,
  applyProgress,
  log,
  check_10,
  download,
  apply,
  result,
  status,
  failed,
  notSet
}

class UpdaterEvent extends $pb.GeneratedMessage {
  factory UpdaterEvent({
    Capabilities? capabilities,
    Progress? progress,
    ApplyProgress? applyProgress,
    Log? log,
    CheckResult? check_10,
    DownloadResult? download,
    ApplyResult? apply,
    Result? result,
    StatusSnapshot? status,
    Failed? failed,
  }) {
    final result$ = create();
    if (capabilities != null) result$.capabilities = capabilities;
    if (progress != null) result$.progress = progress;
    if (applyProgress != null) result$.applyProgress = applyProgress;
    if (log != null) result$.log = log;
    if (check_10 != null) result$.check_10 = check_10;
    if (download != null) result$.download = download;
    if (apply != null) result$.apply = apply;
    if (result != null) result$.result = result;
    if (status != null) result$.status = status;
    if (failed != null) result$.failed = failed;
    return result$;
  }

  UpdaterEvent._();

  factory UpdaterEvent.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory UpdaterEvent.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, UpdaterEvent_Kind> _UpdaterEvent_KindByTag =
      {
    1: UpdaterEvent_Kind.capabilities,
    2: UpdaterEvent_Kind.progress,
    3: UpdaterEvent_Kind.applyProgress,
    4: UpdaterEvent_Kind.log,
    10: UpdaterEvent_Kind.check_10,
    11: UpdaterEvent_Kind.download,
    12: UpdaterEvent_Kind.apply,
    13: UpdaterEvent_Kind.result,
    14: UpdaterEvent_Kind.status,
    15: UpdaterEvent_Kind.failed,
    0: UpdaterEvent_Kind.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'UpdaterEvent',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..oo(0, [1, 2, 3, 4, 10, 11, 12, 13, 14, 15])
    ..aOM<Capabilities>(1, _omitFieldNames ? '' : 'capabilities',
        subBuilder: Capabilities.create)
    ..aOM<Progress>(2, _omitFieldNames ? '' : 'progress',
        subBuilder: Progress.create)
    ..aOM<ApplyProgress>(3, _omitFieldNames ? '' : 'applyProgress',
        subBuilder: ApplyProgress.create)
    ..aOM<Log>(4, _omitFieldNames ? '' : 'log', subBuilder: Log.create)
    ..aOM<CheckResult>(10, _omitFieldNames ? '' : 'check',
        subBuilder: CheckResult.create)
    ..aOM<DownloadResult>(11, _omitFieldNames ? '' : 'download',
        subBuilder: DownloadResult.create)
    ..aOM<ApplyResult>(12, _omitFieldNames ? '' : 'apply',
        subBuilder: ApplyResult.create)
    ..aOM<Result>(13, _omitFieldNames ? '' : 'result',
        subBuilder: Result.create)
    ..aOM<StatusSnapshot>(14, _omitFieldNames ? '' : 'status',
        subBuilder: StatusSnapshot.create)
    ..aOM<Failed>(15, _omitFieldNames ? '' : 'failed',
        subBuilder: Failed.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  UpdaterEvent clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  UpdaterEvent copyWith(void Function(UpdaterEvent) updates) =>
      super.copyWith((message) => updates(message as UpdaterEvent))
          as UpdaterEvent;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static UpdaterEvent create() => UpdaterEvent._();
  @$core.override
  UpdaterEvent createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static UpdaterEvent getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<UpdaterEvent>(create);
  static UpdaterEvent? _defaultInstance;

  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  @$pb.TagNumber(10)
  @$pb.TagNumber(11)
  @$pb.TagNumber(12)
  @$pb.TagNumber(13)
  @$pb.TagNumber(14)
  @$pb.TagNumber(15)
  UpdaterEvent_Kind whichKind() => _UpdaterEvent_KindByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  @$pb.TagNumber(10)
  @$pb.TagNumber(11)
  @$pb.TagNumber(12)
  @$pb.TagNumber(13)
  @$pb.TagNumber(14)
  @$pb.TagNumber(15)
  void clearKind() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  Capabilities get capabilities => $_getN(0);
  @$pb.TagNumber(1)
  set capabilities(Capabilities value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasCapabilities() => $_has(0);
  @$pb.TagNumber(1)
  void clearCapabilities() => $_clearField(1);
  @$pb.TagNumber(1)
  Capabilities ensureCapabilities() => $_ensure(0);

  @$pb.TagNumber(2)
  Progress get progress => $_getN(1);
  @$pb.TagNumber(2)
  set progress(Progress value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasProgress() => $_has(1);
  @$pb.TagNumber(2)
  void clearProgress() => $_clearField(2);
  @$pb.TagNumber(2)
  Progress ensureProgress() => $_ensure(1);

  @$pb.TagNumber(3)
  ApplyProgress get applyProgress => $_getN(2);
  @$pb.TagNumber(3)
  set applyProgress(ApplyProgress value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasApplyProgress() => $_has(2);
  @$pb.TagNumber(3)
  void clearApplyProgress() => $_clearField(3);
  @$pb.TagNumber(3)
  ApplyProgress ensureApplyProgress() => $_ensure(2);

  @$pb.TagNumber(4)
  Log get log => $_getN(3);
  @$pb.TagNumber(4)
  set log(Log value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasLog() => $_has(3);
  @$pb.TagNumber(4)
  void clearLog() => $_clearField(4);
  @$pb.TagNumber(4)
  Log ensureLog() => $_ensure(3);

  @$pb.TagNumber(10)
  CheckResult get check_10 => $_getN(4);
  @$pb.TagNumber(10)
  set check_10(CheckResult value) => $_setField(10, value);
  @$pb.TagNumber(10)
  $core.bool hasCheck_10() => $_has(4);
  @$pb.TagNumber(10)
  void clearCheck_10() => $_clearField(10);
  @$pb.TagNumber(10)
  CheckResult ensureCheck_10() => $_ensure(4);

  @$pb.TagNumber(11)
  DownloadResult get download => $_getN(5);
  @$pb.TagNumber(11)
  set download(DownloadResult value) => $_setField(11, value);
  @$pb.TagNumber(11)
  $core.bool hasDownload() => $_has(5);
  @$pb.TagNumber(11)
  void clearDownload() => $_clearField(11);
  @$pb.TagNumber(11)
  DownloadResult ensureDownload() => $_ensure(5);

  @$pb.TagNumber(12)
  ApplyResult get apply => $_getN(6);
  @$pb.TagNumber(12)
  set apply(ApplyResult value) => $_setField(12, value);
  @$pb.TagNumber(12)
  $core.bool hasApply() => $_has(6);
  @$pb.TagNumber(12)
  void clearApply() => $_clearField(12);
  @$pb.TagNumber(12)
  ApplyResult ensureApply() => $_ensure(6);

  @$pb.TagNumber(13)
  Result get result => $_getN(7);
  @$pb.TagNumber(13)
  set result(Result value) => $_setField(13, value);
  @$pb.TagNumber(13)
  $core.bool hasResult() => $_has(7);
  @$pb.TagNumber(13)
  void clearResult() => $_clearField(13);
  @$pb.TagNumber(13)
  Result ensureResult() => $_ensure(7);

  @$pb.TagNumber(14)
  StatusSnapshot get status => $_getN(8);
  @$pb.TagNumber(14)
  set status(StatusSnapshot value) => $_setField(14, value);
  @$pb.TagNumber(14)
  $core.bool hasStatus() => $_has(8);
  @$pb.TagNumber(14)
  void clearStatus() => $_clearField(14);
  @$pb.TagNumber(14)
  StatusSnapshot ensureStatus() => $_ensure(8);

  @$pb.TagNumber(15)
  Failed get failed => $_getN(9);
  @$pb.TagNumber(15)
  set failed(Failed value) => $_setField(15, value);
  @$pb.TagNumber(15)
  $core.bool hasFailed() => $_has(9);
  @$pb.TagNumber(15)
  void clearFailed() => $_clearField(15);
  @$pb.TagNumber(15)
  Failed ensureFailed() => $_ensure(9);
}

class ArtifactTarget extends $pb.GeneratedMessage {
  factory ArtifactTarget({
    $core.String? name,
    $core.Iterable<$core.MapEntry<$core.String, $core.String>>? selectors,
  }) {
    final result = create();
    if (name != null) result.name = name;
    if (selectors != null) result.selectors.addEntries(selectors);
    return result;
  }

  ArtifactTarget._();

  factory ArtifactTarget.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ArtifactTarget.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ArtifactTarget',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..m<$core.String, $core.String>(2, _omitFieldNames ? '' : 'selectors',
        entryClassName: 'ArtifactTarget.SelectorsEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OS,
        packageName: const $pb.PackageName('relkit.updater.v1'))
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ArtifactTarget clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ArtifactTarget copyWith(void Function(ArtifactTarget) updates) =>
      super.copyWith((message) => updates(message as ArtifactTarget))
          as ArtifactTarget;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ArtifactTarget create() => ArtifactTarget._();
  @$core.override
  ArtifactTarget createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ArtifactTarget getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ArtifactTarget>(create);
  static ArtifactTarget? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbMap<$core.String, $core.String> get selectors => $_getMap(1);
}

class PlannedFile extends $pb.GeneratedMessage {
  factory PlannedFile({
    $core.String? name,
    $fixnum.Int64? size,
    $core.String? sha256Hex,
    $core.Iterable<$core.String>? urls,
    $core.String? destRelpath,
    $core.String? localPath,
    $core.bool? downloaded,
  }) {
    final result = create();
    if (name != null) result.name = name;
    if (size != null) result.size = size;
    if (sha256Hex != null) result.sha256Hex = sha256Hex;
    if (urls != null) result.urls.addAll(urls);
    if (destRelpath != null) result.destRelpath = destRelpath;
    if (localPath != null) result.localPath = localPath;
    if (downloaded != null) result.downloaded = downloaded;
    return result;
  }

  PlannedFile._();

  factory PlannedFile.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PlannedFile.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PlannedFile',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..aInt64(2, _omitFieldNames ? '' : 'size')
    ..aOS(3, _omitFieldNames ? '' : 'sha256Hex')
    ..pPS(4, _omitFieldNames ? '' : 'urls')
    ..aOS(5, _omitFieldNames ? '' : 'destRelpath')
    ..aOS(6, _omitFieldNames ? '' : 'localPath')
    ..aOB(7, _omitFieldNames ? '' : 'downloaded')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PlannedFile clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PlannedFile copyWith(void Function(PlannedFile) updates) =>
      super.copyWith((message) => updates(message as PlannedFile))
          as PlannedFile;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PlannedFile create() => PlannedFile._();
  @$core.override
  PlannedFile createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PlannedFile getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PlannedFile>(create);
  static PlannedFile? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get size => $_getI64(1);
  @$pb.TagNumber(2)
  set size($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSize() => $_has(1);
  @$pb.TagNumber(2)
  void clearSize() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get sha256Hex => $_getSZ(2);
  @$pb.TagNumber(3)
  set sha256Hex($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSha256Hex() => $_has(2);
  @$pb.TagNumber(3)
  void clearSha256Hex() => $_clearField(3);

  @$pb.TagNumber(4)
  $pb.PbList<$core.String> get urls => $_getList(3);

  @$pb.TagNumber(5)
  $core.String get destRelpath => $_getSZ(4);
  @$pb.TagNumber(5)
  set destRelpath($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasDestRelpath() => $_has(4);
  @$pb.TagNumber(5)
  void clearDestRelpath() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get localPath => $_getSZ(5);
  @$pb.TagNumber(6)
  set localPath($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasLocalPath() => $_has(5);
  @$pb.TagNumber(6)
  void clearLocalPath() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get downloaded => $_getBF(6);
  @$pb.TagNumber(7)
  set downloaded($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasDownloaded() => $_has(6);
  @$pb.TagNumber(7)
  void clearDownloaded() => $_clearField(7);
}

class UpdatePlan extends $pb.GeneratedMessage {
  factory UpdatePlan({
    $core.String? planId,
    $core.String? product,
    $core.String? channel,
    $core.String? version,
    $fixnum.Int64? code,
    $fixnum.Int64? sequence,
    $core.bool? mandatory,
    $core.int? remainingHops,
    $core.String? releaseNotesMarkdown,
    $core.String? releaseNotesUrl,
    $core.Iterable<PriorReleaseNotes>? priorReleaseNotes,
    $core.Iterable<PlannedFile>? files,
    $1.Timestamp? createdAt,
    $1.Timestamp? expiresAt,
    $core.List<$core.int>? planHmac,
  }) {
    final result = create();
    if (planId != null) result.planId = planId;
    if (product != null) result.product = product;
    if (channel != null) result.channel = channel;
    if (version != null) result.version = version;
    if (code != null) result.code = code;
    if (sequence != null) result.sequence = sequence;
    if (mandatory != null) result.mandatory = mandatory;
    if (remainingHops != null) result.remainingHops = remainingHops;
    if (releaseNotesMarkdown != null)
      result.releaseNotesMarkdown = releaseNotesMarkdown;
    if (releaseNotesUrl != null) result.releaseNotesUrl = releaseNotesUrl;
    if (priorReleaseNotes != null)
      result.priorReleaseNotes.addAll(priorReleaseNotes);
    if (files != null) result.files.addAll(files);
    if (createdAt != null) result.createdAt = createdAt;
    if (expiresAt != null) result.expiresAt = expiresAt;
    if (planHmac != null) result.planHmac = planHmac;
    return result;
  }

  UpdatePlan._();

  factory UpdatePlan.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory UpdatePlan.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'UpdatePlan',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'planId')
    ..aOS(2, _omitFieldNames ? '' : 'product')
    ..aOS(3, _omitFieldNames ? '' : 'channel')
    ..aOS(4, _omitFieldNames ? '' : 'version')
    ..aInt64(5, _omitFieldNames ? '' : 'code')
    ..aInt64(6, _omitFieldNames ? '' : 'sequence')
    ..aOB(7, _omitFieldNames ? '' : 'mandatory')
    ..aI(8, _omitFieldNames ? '' : 'remainingHops')
    ..aOS(9, _omitFieldNames ? '' : 'releaseNotesMarkdown')
    ..aOS(10, _omitFieldNames ? '' : 'releaseNotesUrl')
    ..pPM<PriorReleaseNotes>(11, _omitFieldNames ? '' : 'priorReleaseNotes',
        subBuilder: PriorReleaseNotes.create)
    ..pPM<PlannedFile>(12, _omitFieldNames ? '' : 'files',
        subBuilder: PlannedFile.create)
    ..aOM<$1.Timestamp>(13, _omitFieldNames ? '' : 'createdAt',
        subBuilder: $1.Timestamp.create)
    ..aOM<$1.Timestamp>(14, _omitFieldNames ? '' : 'expiresAt',
        subBuilder: $1.Timestamp.create)
    ..a<$core.List<$core.int>>(
        15, _omitFieldNames ? '' : 'planHmac', $pb.PbFieldType.OY)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  UpdatePlan clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  UpdatePlan copyWith(void Function(UpdatePlan) updates) =>
      super.copyWith((message) => updates(message as UpdatePlan)) as UpdatePlan;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static UpdatePlan create() => UpdatePlan._();
  @$core.override
  UpdatePlan createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static UpdatePlan getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<UpdatePlan>(create);
  static UpdatePlan? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get planId => $_getSZ(0);
  @$pb.TagNumber(1)
  set planId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPlanId() => $_has(0);
  @$pb.TagNumber(1)
  void clearPlanId() => $_clearField(1);

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
  $core.String get version => $_getSZ(3);
  @$pb.TagNumber(4)
  set version($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasVersion() => $_has(3);
  @$pb.TagNumber(4)
  void clearVersion() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get code => $_getI64(4);
  @$pb.TagNumber(5)
  set code($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasCode() => $_has(4);
  @$pb.TagNumber(5)
  void clearCode() => $_clearField(5);

  @$pb.TagNumber(6)
  $fixnum.Int64 get sequence => $_getI64(5);
  @$pb.TagNumber(6)
  set sequence($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(6)
  $core.bool hasSequence() => $_has(5);
  @$pb.TagNumber(6)
  void clearSequence() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get mandatory => $_getBF(6);
  @$pb.TagNumber(7)
  set mandatory($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasMandatory() => $_has(6);
  @$pb.TagNumber(7)
  void clearMandatory() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.int get remainingHops => $_getIZ(7);
  @$pb.TagNumber(8)
  set remainingHops($core.int value) => $_setSignedInt32(7, value);
  @$pb.TagNumber(8)
  $core.bool hasRemainingHops() => $_has(7);
  @$pb.TagNumber(8)
  void clearRemainingHops() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get releaseNotesMarkdown => $_getSZ(8);
  @$pb.TagNumber(9)
  set releaseNotesMarkdown($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasReleaseNotesMarkdown() => $_has(8);
  @$pb.TagNumber(9)
  void clearReleaseNotesMarkdown() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.String get releaseNotesUrl => $_getSZ(9);
  @$pb.TagNumber(10)
  set releaseNotesUrl($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasReleaseNotesUrl() => $_has(9);
  @$pb.TagNumber(10)
  void clearReleaseNotesUrl() => $_clearField(10);

  @$pb.TagNumber(11)
  $pb.PbList<PriorReleaseNotes> get priorReleaseNotes => $_getList(10);

  @$pb.TagNumber(12)
  $pb.PbList<PlannedFile> get files => $_getList(11);

  @$pb.TagNumber(13)
  $1.Timestamp get createdAt => $_getN(12);
  @$pb.TagNumber(13)
  set createdAt($1.Timestamp value) => $_setField(13, value);
  @$pb.TagNumber(13)
  $core.bool hasCreatedAt() => $_has(12);
  @$pb.TagNumber(13)
  void clearCreatedAt() => $_clearField(13);
  @$pb.TagNumber(13)
  $1.Timestamp ensureCreatedAt() => $_ensure(12);

  @$pb.TagNumber(14)
  $1.Timestamp get expiresAt => $_getN(13);
  @$pb.TagNumber(14)
  set expiresAt($1.Timestamp value) => $_setField(14, value);
  @$pb.TagNumber(14)
  $core.bool hasExpiresAt() => $_has(13);
  @$pb.TagNumber(14)
  void clearExpiresAt() => $_clearField(14);
  @$pb.TagNumber(14)
  $1.Timestamp ensureExpiresAt() => $_ensure(13);

  @$pb.TagNumber(15)
  $core.List<$core.int> get planHmac => $_getN(14);
  @$pb.TagNumber(15)
  set planHmac($core.List<$core.int> value) => $_setBytes(14, value);
  @$pb.TagNumber(15)
  $core.bool hasPlanHmac() => $_has(14);
  @$pb.TagNumber(15)
  void clearPlanHmac() => $_clearField(15);
}

class PersistedState extends $pb.GeneratedMessage {
  factory PersistedState({
    $1.Timestamp? lastCheckAt,
    LastResult? lastResult,
    $fixnum.Int64? lastSeenSequence,
    $fixnum.Int64? lastSeenDirectorySequence,
    $fixnum.Int64? lastSeenFallbackSequence,
    $core.Iterable<$fixnum.Int64>? skippedCodes,
  }) {
    final result = create();
    if (lastCheckAt != null) result.lastCheckAt = lastCheckAt;
    if (lastResult != null) result.lastResult = lastResult;
    if (lastSeenSequence != null) result.lastSeenSequence = lastSeenSequence;
    if (lastSeenDirectorySequence != null)
      result.lastSeenDirectorySequence = lastSeenDirectorySequence;
    if (lastSeenFallbackSequence != null)
      result.lastSeenFallbackSequence = lastSeenFallbackSequence;
    if (skippedCodes != null) result.skippedCodes.addAll(skippedCodes);
    return result;
  }

  PersistedState._();

  factory PersistedState.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PersistedState.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PersistedState',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOM<$1.Timestamp>(1, _omitFieldNames ? '' : 'lastCheckAt',
        subBuilder: $1.Timestamp.create)
    ..aE<LastResult>(2, _omitFieldNames ? '' : 'lastResult',
        enumValues: LastResult.values)
    ..aInt64(3, _omitFieldNames ? '' : 'lastSeenSequence')
    ..aInt64(4, _omitFieldNames ? '' : 'lastSeenDirectorySequence')
    ..aInt64(5, _omitFieldNames ? '' : 'lastSeenFallbackSequence')
    ..p<$fixnum.Int64>(
        6, _omitFieldNames ? '' : 'skippedCodes', $pb.PbFieldType.K6)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PersistedState clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PersistedState copyWith(void Function(PersistedState) updates) =>
      super.copyWith((message) => updates(message as PersistedState))
          as PersistedState;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PersistedState create() => PersistedState._();
  @$core.override
  PersistedState createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PersistedState getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PersistedState>(create);
  static PersistedState? _defaultInstance;

  @$pb.TagNumber(1)
  $1.Timestamp get lastCheckAt => $_getN(0);
  @$pb.TagNumber(1)
  set lastCheckAt($1.Timestamp value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasLastCheckAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearLastCheckAt() => $_clearField(1);
  @$pb.TagNumber(1)
  $1.Timestamp ensureLastCheckAt() => $_ensure(0);

  @$pb.TagNumber(2)
  LastResult get lastResult => $_getN(1);
  @$pb.TagNumber(2)
  set lastResult(LastResult value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasLastResult() => $_has(1);
  @$pb.TagNumber(2)
  void clearLastResult() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get lastSeenSequence => $_getI64(2);
  @$pb.TagNumber(3)
  set lastSeenSequence($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasLastSeenSequence() => $_has(2);
  @$pb.TagNumber(3)
  void clearLastSeenSequence() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get lastSeenDirectorySequence => $_getI64(3);
  @$pb.TagNumber(4)
  set lastSeenDirectorySequence($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasLastSeenDirectorySequence() => $_has(3);
  @$pb.TagNumber(4)
  void clearLastSeenDirectorySequence() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get lastSeenFallbackSequence => $_getI64(4);
  @$pb.TagNumber(5)
  set lastSeenFallbackSequence($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasLastSeenFallbackSequence() => $_has(4);
  @$pb.TagNumber(5)
  void clearLastSeenFallbackSequence() => $_clearField(5);

  @$pb.TagNumber(6)
  $pb.PbList<$fixnum.Int64> get skippedCodes => $_getList(5);
}

class ApplySessionRecord extends $pb.GeneratedMessage {
  factory ApplySessionRecord({
    $core.String? sessionId,
    $core.String? planId,
    SessionPhase? phase,
    $1.Timestamp? startedAt,
    $1.Timestamp? heartbeatAt,
    $core.String? installRoot,
    $core.String? stagedRoot,
    $fixnum.Int64? targetCode,
    $core.String? targetVersion,
    Error? error,
    $core.int? pid,
    Layout? layout,
    $core.bool? relaunch,
    $core.String? executableRelpath,
    $core.Iterable<$core.String>? preserve,
    $core.int? retain,
    $core.Iterable<FileSetEntry>? fileSet,
    $core.String? sidecarRelpath,
  }) {
    final result = create();
    if (sessionId != null) result.sessionId = sessionId;
    if (planId != null) result.planId = planId;
    if (phase != null) result.phase = phase;
    if (startedAt != null) result.startedAt = startedAt;
    if (heartbeatAt != null) result.heartbeatAt = heartbeatAt;
    if (installRoot != null) result.installRoot = installRoot;
    if (stagedRoot != null) result.stagedRoot = stagedRoot;
    if (targetCode != null) result.targetCode = targetCode;
    if (targetVersion != null) result.targetVersion = targetVersion;
    if (error != null) result.error = error;
    if (pid != null) result.pid = pid;
    if (layout != null) result.layout = layout;
    if (relaunch != null) result.relaunch = relaunch;
    if (executableRelpath != null) result.executableRelpath = executableRelpath;
    if (preserve != null) result.preserve.addAll(preserve);
    if (retain != null) result.retain = retain;
    if (fileSet != null) result.fileSet.addAll(fileSet);
    if (sidecarRelpath != null) result.sidecarRelpath = sidecarRelpath;
    return result;
  }

  ApplySessionRecord._();

  factory ApplySessionRecord.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ApplySessionRecord.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ApplySessionRecord',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..aOS(2, _omitFieldNames ? '' : 'planId')
    ..aE<SessionPhase>(3, _omitFieldNames ? '' : 'phase',
        enumValues: SessionPhase.values)
    ..aOM<$1.Timestamp>(4, _omitFieldNames ? '' : 'startedAt',
        subBuilder: $1.Timestamp.create)
    ..aOM<$1.Timestamp>(5, _omitFieldNames ? '' : 'heartbeatAt',
        subBuilder: $1.Timestamp.create)
    ..aOS(6, _omitFieldNames ? '' : 'installRoot')
    ..aOS(7, _omitFieldNames ? '' : 'stagedRoot')
    ..aInt64(8, _omitFieldNames ? '' : 'targetCode')
    ..aOS(9, _omitFieldNames ? '' : 'targetVersion')
    ..aOM<Error>(10, _omitFieldNames ? '' : 'error', subBuilder: Error.create)
    ..aI(11, _omitFieldNames ? '' : 'pid')
    ..aE<Layout>(12, _omitFieldNames ? '' : 'layout', enumValues: Layout.values)
    ..aOB(13, _omitFieldNames ? '' : 'relaunch')
    ..aOS(14, _omitFieldNames ? '' : 'executableRelpath')
    ..pPS(15, _omitFieldNames ? '' : 'preserve')
    ..aI(16, _omitFieldNames ? '' : 'retain')
    ..pPM<FileSetEntry>(17, _omitFieldNames ? '' : 'fileSet',
        subBuilder: FileSetEntry.create)
    ..aOS(18, _omitFieldNames ? '' : 'sidecarRelpath')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplySessionRecord clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplySessionRecord copyWith(void Function(ApplySessionRecord) updates) =>
      super.copyWith((message) => updates(message as ApplySessionRecord))
          as ApplySessionRecord;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ApplySessionRecord create() => ApplySessionRecord._();
  @$core.override
  ApplySessionRecord createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ApplySessionRecord getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ApplySessionRecord>(create);
  static ApplySessionRecord? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sessionId => $_getSZ(0);
  @$pb.TagNumber(1)
  set sessionId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSessionId() => $_has(0);
  @$pb.TagNumber(1)
  void clearSessionId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get planId => $_getSZ(1);
  @$pb.TagNumber(2)
  set planId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPlanId() => $_has(1);
  @$pb.TagNumber(2)
  void clearPlanId() => $_clearField(2);

  @$pb.TagNumber(3)
  SessionPhase get phase => $_getN(2);
  @$pb.TagNumber(3)
  set phase(SessionPhase value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasPhase() => $_has(2);
  @$pb.TagNumber(3)
  void clearPhase() => $_clearField(3);

  @$pb.TagNumber(4)
  $1.Timestamp get startedAt => $_getN(3);
  @$pb.TagNumber(4)
  set startedAt($1.Timestamp value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasStartedAt() => $_has(3);
  @$pb.TagNumber(4)
  void clearStartedAt() => $_clearField(4);
  @$pb.TagNumber(4)
  $1.Timestamp ensureStartedAt() => $_ensure(3);

  @$pb.TagNumber(5)
  $1.Timestamp get heartbeatAt => $_getN(4);
  @$pb.TagNumber(5)
  set heartbeatAt($1.Timestamp value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasHeartbeatAt() => $_has(4);
  @$pb.TagNumber(5)
  void clearHeartbeatAt() => $_clearField(5);
  @$pb.TagNumber(5)
  $1.Timestamp ensureHeartbeatAt() => $_ensure(4);

  @$pb.TagNumber(6)
  $core.String get installRoot => $_getSZ(5);
  @$pb.TagNumber(6)
  set installRoot($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasInstallRoot() => $_has(5);
  @$pb.TagNumber(6)
  void clearInstallRoot() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get stagedRoot => $_getSZ(6);
  @$pb.TagNumber(7)
  set stagedRoot($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasStagedRoot() => $_has(6);
  @$pb.TagNumber(7)
  void clearStagedRoot() => $_clearField(7);

  @$pb.TagNumber(8)
  $fixnum.Int64 get targetCode => $_getI64(7);
  @$pb.TagNumber(8)
  set targetCode($fixnum.Int64 value) => $_setInt64(7, value);
  @$pb.TagNumber(8)
  $core.bool hasTargetCode() => $_has(7);
  @$pb.TagNumber(8)
  void clearTargetCode() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get targetVersion => $_getSZ(8);
  @$pb.TagNumber(9)
  set targetVersion($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasTargetVersion() => $_has(8);
  @$pb.TagNumber(9)
  void clearTargetVersion() => $_clearField(9);

  @$pb.TagNumber(10)
  Error get error => $_getN(9);
  @$pb.TagNumber(10)
  set error(Error value) => $_setField(10, value);
  @$pb.TagNumber(10)
  $core.bool hasError() => $_has(9);
  @$pb.TagNumber(10)
  void clearError() => $_clearField(10);
  @$pb.TagNumber(10)
  Error ensureError() => $_ensure(9);

  @$pb.TagNumber(11)
  $core.int get pid => $_getIZ(10);
  @$pb.TagNumber(11)
  set pid($core.int value) => $_setSignedInt32(10, value);
  @$pb.TagNumber(11)
  $core.bool hasPid() => $_has(10);
  @$pb.TagNumber(11)
  void clearPid() => $_clearField(11);

  @$pb.TagNumber(12)
  Layout get layout => $_getN(11);
  @$pb.TagNumber(12)
  set layout(Layout value) => $_setField(12, value);
  @$pb.TagNumber(12)
  $core.bool hasLayout() => $_has(11);
  @$pb.TagNumber(12)
  void clearLayout() => $_clearField(12);

  @$pb.TagNumber(13)
  $core.bool get relaunch => $_getBF(12);
  @$pb.TagNumber(13)
  set relaunch($core.bool value) => $_setBool(12, value);
  @$pb.TagNumber(13)
  $core.bool hasRelaunch() => $_has(12);
  @$pb.TagNumber(13)
  void clearRelaunch() => $_clearField(13);

  @$pb.TagNumber(14)
  $core.String get executableRelpath => $_getSZ(13);
  @$pb.TagNumber(14)
  set executableRelpath($core.String value) => $_setString(13, value);
  @$pb.TagNumber(14)
  $core.bool hasExecutableRelpath() => $_has(13);
  @$pb.TagNumber(14)
  void clearExecutableRelpath() => $_clearField(14);

  @$pb.TagNumber(15)
  $pb.PbList<$core.String> get preserve => $_getList(14);

  @$pb.TagNumber(16)
  $core.int get retain => $_getIZ(15);
  @$pb.TagNumber(16)
  set retain($core.int value) => $_setSignedInt32(15, value);
  @$pb.TagNumber(16)
  $core.bool hasRetain() => $_has(15);
  @$pb.TagNumber(16)
  void clearRetain() => $_clearField(16);

  @$pb.TagNumber(17)
  $pb.PbList<FileSetEntry> get fileSet => $_getList(16);

  @$pb.TagNumber(18)
  $core.String get sidecarRelpath => $_getSZ(17);
  @$pb.TagNumber(18)
  set sidecarRelpath($core.String value) => $_setString(17, value);
  @$pb.TagNumber(18)
  $core.bool hasSidecarRelpath() => $_has(17);
  @$pb.TagNumber(18)
  void clearSidecarRelpath() => $_clearField(18);
}

class JournalEntry extends $pb.GeneratedMessage {
  factory JournalEntry({
    $core.String? destPath,
    $core.String? backupPath,
    $core.String? sourcePath,
  }) {
    final result = create();
    if (destPath != null) result.destPath = destPath;
    if (backupPath != null) result.backupPath = backupPath;
    if (sourcePath != null) result.sourcePath = sourcePath;
    return result;
  }

  JournalEntry._();

  factory JournalEntry.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory JournalEntry.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'JournalEntry',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'destPath')
    ..aOS(2, _omitFieldNames ? '' : 'backupPath')
    ..aOS(3, _omitFieldNames ? '' : 'sourcePath')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  JournalEntry clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  JournalEntry copyWith(void Function(JournalEntry) updates) =>
      super.copyWith((message) => updates(message as JournalEntry))
          as JournalEntry;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static JournalEntry create() => JournalEntry._();
  @$core.override
  JournalEntry createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static JournalEntry getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<JournalEntry>(create);
  static JournalEntry? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get destPath => $_getSZ(0);
  @$pb.TagNumber(1)
  set destPath($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDestPath() => $_has(0);
  @$pb.TagNumber(1)
  void clearDestPath() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get backupPath => $_getSZ(1);
  @$pb.TagNumber(2)
  set backupPath($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBackupPath() => $_has(1);
  @$pb.TagNumber(2)
  void clearBackupPath() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get sourcePath => $_getSZ(2);
  @$pb.TagNumber(3)
  set sourcePath($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSourcePath() => $_has(2);
  @$pb.TagNumber(3)
  void clearSourcePath() => $_clearField(3);
}

class ApplyJournal extends $pb.GeneratedMessage {
  factory ApplyJournal({
    $core.String? sessionId,
    $core.Iterable<JournalEntry>? entries,
    $core.bool? committed,
  }) {
    final result = create();
    if (sessionId != null) result.sessionId = sessionId;
    if (entries != null) result.entries.addAll(entries);
    if (committed != null) result.committed = committed;
    return result;
  }

  ApplyJournal._();

  factory ApplyJournal.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ApplyJournal.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ApplyJournal',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..pPM<JournalEntry>(2, _omitFieldNames ? '' : 'entries',
        subBuilder: JournalEntry.create)
    ..aOB(3, _omitFieldNames ? '' : 'committed')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplyJournal clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplyJournal copyWith(void Function(ApplyJournal) updates) =>
      super.copyWith((message) => updates(message as ApplyJournal))
          as ApplyJournal;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ApplyJournal create() => ApplyJournal._();
  @$core.override
  ApplyJournal createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ApplyJournal getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ApplyJournal>(create);
  static ApplyJournal? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get sessionId => $_getSZ(0);
  @$pb.TagNumber(1)
  set sessionId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasSessionId() => $_has(0);
  @$pb.TagNumber(1)
  void clearSessionId() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<JournalEntry> get entries => $_getList(1);

  @$pb.TagNumber(3)
  $core.bool get committed => $_getBF(2);
  @$pb.TagNumber(3)
  set committed($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasCommitted() => $_has(2);
  @$pb.TagNumber(3)
  void clearCommitted() => $_clearField(3);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
