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
    final result = TrustedKey._();
    if (keyId != null) result.keyId = keyId;
    if (publicKey != null) result.publicKey = publicKey;
    return result;
  }

  TrustedKey._();

  factory TrustedKey.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      TrustedKey()..mergeFromBuffer(data, registry);
  factory TrustedKey.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      TrustedKey()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'TrustedKey',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: TrustedKey.$_createMessage)
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
  @$core.Deprecated('Use TrustedKey() / TrustedKey.new instead')
  static TrustedKey create() => TrustedKey._();
  static $pb.GeneratedMessage $_createMessage() => TrustedKey._();
  @$core.override
  TrustedKey createEmptyInstance() => TrustedKey._();
  @$core.pragma('dart2js:noInline')
  static TrustedKey getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<TrustedKey>(TrustedKey.$_createMessage);
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
    final result = RecoveryLink._();
    if (label != null) result.label = label;
    if (url != null) result.url = url;
    return result;
  }

  RecoveryLink._();

  factory RecoveryLink.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      RecoveryLink()..mergeFromBuffer(data, registry);
  factory RecoveryLink.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      RecoveryLink()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RecoveryLink',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: RecoveryLink.$_createMessage)
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
  @$core.Deprecated('Use RecoveryLink() / RecoveryLink.new instead')
  static RecoveryLink create() => RecoveryLink._();
  static $pb.GeneratedMessage $_createMessage() => RecoveryLink._();
  @$core.override
  RecoveryLink createEmptyInstance() => RecoveryLink._();
  @$core.pragma('dart2js:noInline')
  static RecoveryLink getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<RecoveryLink>(
          RecoveryLink.$_createMessage);
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
    final result = RecoveryHelp._();
    if (message != null) result.message = message;
    if (links != null) result.links.addAll(links);
    return result;
  }

  RecoveryHelp._();

  factory RecoveryHelp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      RecoveryHelp()..mergeFromBuffer(data, registry);
  factory RecoveryHelp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      RecoveryHelp()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RecoveryHelp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: RecoveryHelp.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'message')
    ..pPM<RecoveryLink>(2, _omitFieldNames ? '' : 'links',
        subBuilder: RecoveryLink.$_createMessage)
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
  @$core.Deprecated('Use RecoveryHelp() / RecoveryHelp.new instead')
  static RecoveryHelp create() => RecoveryHelp._();
  static $pb.GeneratedMessage $_createMessage() => RecoveryHelp._();
  @$core.override
  RecoveryHelp createEmptyInstance() => RecoveryHelp._();
  @$core.pragma('dart2js:noInline')
  static RecoveryHelp getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<RecoveryHelp>(
          RecoveryHelp.$_createMessage);
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
    final result = ClientProfile._();
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
      ClientProfile()..mergeFromBuffer(data, registry);
  factory ClientProfile.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientProfile()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientProfile',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: ClientProfile.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'product')
    ..pPS(2, _omitFieldNames ? '' : 'allowedChannels')
    ..pPS(3, _omitFieldNames ? '' : 'entryUrls')
    ..pPS(4, _omitFieldNames ? '' : 'indexUrls')
    ..pPS(5, _omitFieldNames ? '' : 'fallbackUrls')
    ..pPM<TrustedKey>(6, _omitFieldNames ? '' : 'trustedKeys',
        subBuilder: TrustedKey.$_createMessage)
    ..aOM<RecoveryHelp>(7, _omitFieldNames ? '' : 'recovery',
        subBuilder: RecoveryHelp.$_createMessage)
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
  @$core.Deprecated('Use ClientProfile() / ClientProfile.new instead')
  static ClientProfile create() => ClientProfile._();
  static $pb.GeneratedMessage $_createMessage() => ClientProfile._();
  @$core.override
  ClientProfile createEmptyInstance() => ClientProfile._();
  @$core.pragma('dart2js:noInline')
  static ClientProfile getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ClientProfile>(
          ClientProfile.$_createMessage);
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
    final result = FileSetEntry._();
    if (destRelpath != null) result.destRelpath = destRelpath;
    if (artifactName != null) result.artifactName = artifactName;
    return result;
  }

  FileSetEntry._();

  factory FileSetEntry.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileSetEntry()..mergeFromBuffer(data, registry);
  factory FileSetEntry.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FileSetEntry()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FileSetEntry',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: FileSetEntry.$_createMessage)
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
  @$core.Deprecated('Use FileSetEntry() / FileSetEntry.new instead')
  static FileSetEntry create() => FileSetEntry._();
  static $pb.GeneratedMessage $_createMessage() => FileSetEntry._();
  @$core.override
  FileSetEntry createEmptyInstance() => FileSetEntry._();
  @$core.pragma('dart2js:noInline')
  static FileSetEntry getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<FileSetEntry>(
          FileSetEntry.$_createMessage);
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
    $core.Iterable<$fixnum.Int64>? reservedCodes,
  }) {
    final result = InstallSpec._();
    if (layout != null) result.layout = layout;
    if (installRoot != null) result.installRoot = installRoot;
    if (executableRelpath != null) result.executableRelpath = executableRelpath;
    if (sidecarRelpath != null) result.sidecarRelpath = sidecarRelpath;
    if (preserve != null) result.preserve.addAll(preserve);
    if (retain != null) result.retain = retain;
    if (relaunch != null) result.relaunch = relaunch;
    if (fileSet != null) result.fileSet.addAll(fileSet);
    if (reservedCodes != null) result.reservedCodes.addAll(reservedCodes);
    return result;
  }

  InstallSpec._();

  factory InstallSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InstallSpec()..mergeFromBuffer(data, registry);
  factory InstallSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InstallSpec()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'InstallSpec',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: InstallSpec.$_createMessage)
    ..aE<Layout>(1, _omitFieldNames ? '' : 'layout', enumValues: Layout.values)
    ..aOS(2, _omitFieldNames ? '' : 'installRoot')
    ..aOS(3, _omitFieldNames ? '' : 'executableRelpath')
    ..aOS(4, _omitFieldNames ? '' : 'sidecarRelpath')
    ..pPS(5, _omitFieldNames ? '' : 'preserve')
    ..aI(6, _omitFieldNames ? '' : 'retain')
    ..aOB(7, _omitFieldNames ? '' : 'relaunch')
    ..pPM<FileSetEntry>(8, _omitFieldNames ? '' : 'fileSet',
        subBuilder: FileSetEntry.$_createMessage)
    ..p<$fixnum.Int64>(
        9, _omitFieldNames ? '' : 'reservedCodes', $pb.PbFieldType.K6)
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
  @$core.Deprecated('Use InstallSpec() / InstallSpec.new instead')
  static InstallSpec create() => InstallSpec._();
  static $pb.GeneratedMessage $_createMessage() => InstallSpec._();
  @$core.override
  InstallSpec createEmptyInstance() => InstallSpec._();
  @$core.pragma('dart2js:noInline')
  static InstallSpec getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<InstallSpec>(
          InstallSpec.$_createMessage);
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

  /// Codes that prune must never delete (project pins). Engine does not
  /// interpret how the host chose them.
  @$pb.TagNumber(9)
  $pb.PbList<$fixnum.Int64> get reservedCodes => $_getList(8);
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
    final result = Runtime._();
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
      Runtime()..mergeFromBuffer(data, registry);
  factory Runtime.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Runtime()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Runtime',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: Runtime.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'channel')
    ..aInt64(2, _omitFieldNames ? '' : 'currentCode')
    ..m<$core.String, $core.String>(3, _omitFieldNames ? '' : 'clientSelectors',
        entryClassName: 'Runtime.ClientSelectorsEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OS,
        packageName: const $pb.PackageName('relkit.updater.v1'))
    ..aOS(4, _omitFieldNames ? '' : 'dataDir')
    ..aOM<InstallSpec>(5, _omitFieldNames ? '' : 'install',
        subBuilder: InstallSpec.$_createMessage)
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
  @$core.Deprecated('Use Runtime() / Runtime.new instead')
  static Runtime create() => Runtime._();
  static $pb.GeneratedMessage $_createMessage() => Runtime._();
  @$core.override
  Runtime createEmptyInstance() => Runtime._();
  @$core.pragma('dart2js:noInline')
  static Runtime getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Runtime>(Runtime.$_createMessage);
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
    final result = CheckPolicy._();
    if (afterSuccess != null) result.afterSuccess = afterSuccess;
    if (afterFailure != null) result.afterFailure = afterFailure;
    return result;
  }

  CheckPolicy._();

  factory CheckPolicy.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      CheckPolicy()..mergeFromBuffer(data, registry);
  factory CheckPolicy.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      CheckPolicy()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CheckPolicy',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: CheckPolicy.$_createMessage)
    ..aOM<$0.Duration>(1, _omitFieldNames ? '' : 'afterSuccess',
        subBuilder: $0.Duration.$_createMessage)
    ..aOM<$0.Duration>(2, _omitFieldNames ? '' : 'afterFailure',
        subBuilder: $0.Duration.$_createMessage)
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
  @$core.Deprecated('Use CheckPolicy() / CheckPolicy.new instead')
  static CheckPolicy create() => CheckPolicy._();
  static $pb.GeneratedMessage $_createMessage() => CheckPolicy._();
  @$core.override
  CheckPolicy createEmptyInstance() => CheckPolicy._();
  @$core.pragma('dart2js:noInline')
  static CheckPolicy getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<CheckPolicy>(
          CheckPolicy.$_createMessage);
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
    final result = SchedulerConfig._();
    if (checkOnStart != null) result.checkOnStart = checkOnStart;
    if (forceOnStart != null) result.forceOnStart = forceOnStart;
    if (policy != null) result.policy = policy;
    return result;
  }

  SchedulerConfig._();

  factory SchedulerConfig.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SchedulerConfig()..mergeFromBuffer(data, registry);
  factory SchedulerConfig.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SchedulerConfig()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SchedulerConfig',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: SchedulerConfig.$_createMessage)
    ..aOB(1, _omitFieldNames ? '' : 'checkOnStart')
    ..aOB(2, _omitFieldNames ? '' : 'forceOnStart')
    ..aOM<CheckPolicy>(3, _omitFieldNames ? '' : 'policy',
        subBuilder: CheckPolicy.$_createMessage)
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
  @$core.Deprecated('Use SchedulerConfig() / SchedulerConfig.new instead')
  static SchedulerConfig create() => SchedulerConfig._();
  static $pb.GeneratedMessage $_createMessage() => SchedulerConfig._();
  @$core.override
  SchedulerConfig createEmptyInstance() => SchedulerConfig._();
  @$core.pragma('dart2js:noInline')
  static SchedulerConfig getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<SchedulerConfig>(
          SchedulerConfig.$_createMessage);
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
    final result = ClientHello._();
    if (ipcMin != null) result.ipcMin = ipcMin;
    if (ipcMax != null) result.ipcMax = ipcMax;
    return result;
  }

  ClientHello._();

  factory ClientHello.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientHello()..mergeFromBuffer(data, registry);
  factory ClientHello.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ClientHello()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientHello',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: ClientHello.$_createMessage)
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
  @$core.Deprecated('Use ClientHello() / ClientHello.new instead')
  static ClientHello create() => ClientHello._();
  static $pb.GeneratedMessage $_createMessage() => ClientHello._();
  @$core.override
  ClientHello createEmptyInstance() => ClientHello._();
  @$core.pragma('dart2js:noInline')
  static ClientHello getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ClientHello>(
          ClientHello.$_createMessage);
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
    final result = Capabilities._();
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
      Capabilities()..mergeFromBuffer(data, registry);
  factory Capabilities.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Capabilities()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Capabilities',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: Capabilities.$_createMessage)
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
        subBuilder: $0.Duration.$_createMessage)
    ..aOM<$0.Duration>(5, _omitFieldNames ? '' : 'planTtl',
        subBuilder: $0.Duration.$_createMessage)
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
  @$core.Deprecated('Use Capabilities() / Capabilities.new instead')
  static Capabilities create() => Capabilities._();
  static $pb.GeneratedMessage $_createMessage() => Capabilities._();
  @$core.override
  Capabilities createEmptyInstance() => Capabilities._();
  @$core.pragma('dart2js:noInline')
  static Capabilities getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Capabilities>(
          Capabilities.$_createMessage);
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
    final result = CheckOp._();
    if (force != null) result.force = force;
    if (exactCode != null) result.exactCode = exactCode;
    if (policy != null) result.policy = policy;
    return result;
  }

  CheckOp._();

  factory CheckOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      CheckOp()..mergeFromBuffer(data, registry);
  factory CheckOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      CheckOp()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CheckOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: CheckOp.$_createMessage)
    ..aOB(1, _omitFieldNames ? '' : 'force')
    ..aInt64(2, _omitFieldNames ? '' : 'exactCode')
    ..aOM<CheckPolicy>(3, _omitFieldNames ? '' : 'policy',
        subBuilder: CheckPolicy.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CheckOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CheckOp copyWith(void Function(CheckOp) updates) =>
      super.copyWith((message) => updates(message as CheckOp)) as CheckOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use CheckOp() / CheckOp.new instead')
  static CheckOp create() => CheckOp._();
  static $pb.GeneratedMessage $_createMessage() => CheckOp._();
  @$core.override
  CheckOp createEmptyInstance() => CheckOp._();
  @$core.pragma('dart2js:noInline')
  static CheckOp getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CheckOp>(CheckOp.$_createMessage);
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
    final result = SkipOp._();
    if (code != null) result.code = code;
    return result;
  }

  SkipOp._();

  factory SkipOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SkipOp()..mergeFromBuffer(data, registry);
  factory SkipOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SkipOp()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SkipOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: SkipOp.$_createMessage)
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
  @$core.Deprecated('Use SkipOp() / SkipOp.new instead')
  static SkipOp create() => SkipOp._();
  static $pb.GeneratedMessage $_createMessage() => SkipOp._();
  @$core.override
  SkipOp createEmptyInstance() => SkipOp._();
  @$core.pragma('dart2js:noInline')
  static SkipOp getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SkipOp>(SkipOp.$_createMessage);
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
    final result = DownloadOp._();
    if (planId != null) result.planId = planId;
    return result;
  }

  DownloadOp._();

  factory DownloadOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      DownloadOp()..mergeFromBuffer(data, registry);
  factory DownloadOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      DownloadOp()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DownloadOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: DownloadOp.$_createMessage)
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
  @$core.Deprecated('Use DownloadOp() / DownloadOp.new instead')
  static DownloadOp create() => DownloadOp._();
  static $pb.GeneratedMessage $_createMessage() => DownloadOp._();
  @$core.override
  DownloadOp createEmptyInstance() => DownloadOp._();
  @$core.pragma('dart2js:noInline')
  static DownloadOp getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DownloadOp>(DownloadOp.$_createMessage);
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
    $core.bool? installOnly,
  }) {
    final result = ApplyOp._();
    if (planId != null) result.planId = planId;
    if (installOnly != null) result.installOnly = installOnly;
    return result;
  }

  ApplyOp._();

  factory ApplyOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ApplyOp()..mergeFromBuffer(data, registry);
  factory ApplyOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ApplyOp()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ApplyOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: ApplyOp.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'planId')
    ..aOB(2, _omitFieldNames ? '' : 'installOnly')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplyOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ApplyOp copyWith(void Function(ApplyOp) updates) =>
      super.copyWith((message) => updates(message as ApplyOp)) as ApplyOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use ApplyOp() / ApplyOp.new instead')
  static ApplyOp create() => ApplyOp._();
  static $pb.GeneratedMessage $_createMessage() => ApplyOp._();
  @$core.override
  ApplyOp createEmptyInstance() => ApplyOp._();
  @$core.pragma('dart2js:noInline')
  static ApplyOp getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ApplyOp>(ApplyOp.$_createMessage);
  static ApplyOp? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get planId => $_getSZ(0);
  @$pb.TagNumber(1)
  set planId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPlanId() => $_has(0);
  @$pb.TagNumber(1)
  void clearPlanId() => $_clearField(1);

  /// When true, versionedDir copies into versions/ but does not rewrite
  /// active.json. wholeRoot / fileSet ignore this flag.
  @$pb.TagNumber(2)
  $core.bool get installOnly => $_getBF(1);
  @$pb.TagNumber(2)
  set installOnly($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasInstallOnly() => $_has(1);
  @$pb.TagNumber(2)
  void clearInstallOnly() => $_clearField(2);
}

class StatusOp extends $pb.GeneratedMessage {
  factory StatusOp() => StatusOp._();

  StatusOp._();

  factory StatusOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StatusOp()..mergeFromBuffer(data, registry);
  factory StatusOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StatusOp()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StatusOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: StatusOp.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StatusOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  StatusOp copyWith(void Function(StatusOp) updates) =>
      super.copyWith((message) => updates(message as StatusOp)) as StatusOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use StatusOp() / StatusOp.new instead')
  static StatusOp create() => StatusOp._();
  static $pb.GeneratedMessage $_createMessage() => StatusOp._();
  @$core.override
  StatusOp createEmptyInstance() => StatusOp._();
  @$core.pragma('dart2js:noInline')
  static StatusOp getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<StatusOp>(StatusOp.$_createMessage);
  static StatusOp? _defaultInstance;
}

class CleanupOp extends $pb.GeneratedMessage {
  factory CleanupOp() => CleanupOp._();

  CleanupOp._();

  factory CleanupOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      CleanupOp()..mergeFromBuffer(data, registry);
  factory CleanupOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      CleanupOp()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CleanupOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: CleanupOp.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CleanupOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CleanupOp copyWith(void Function(CleanupOp) updates) =>
      super.copyWith((message) => updates(message as CleanupOp)) as CleanupOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use CleanupOp() / CleanupOp.new instead')
  static CleanupOp create() => CleanupOp._();
  static $pb.GeneratedMessage $_createMessage() => CleanupOp._();
  @$core.override
  CleanupOp createEmptyInstance() => CleanupOp._();
  @$core.pragma('dart2js:noInline')
  static CleanupOp getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CleanupOp>(CleanupOp.$_createMessage);
  static CleanupOp? _defaultInstance;
}

class CancelOp extends $pb.GeneratedMessage {
  factory CancelOp() => CancelOp._();

  CancelOp._();

  factory CancelOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      CancelOp()..mergeFromBuffer(data, registry);
  factory CancelOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      CancelOp()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CancelOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: CancelOp.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CancelOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CancelOp copyWith(void Function(CancelOp) updates) =>
      super.copyWith((message) => updates(message as CancelOp)) as CancelOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use CancelOp() / CancelOp.new instead')
  static CancelOp create() => CancelOp._();
  static $pb.GeneratedMessage $_createMessage() => CancelOp._();
  @$core.override
  CancelOp createEmptyInstance() => CancelOp._();
  @$core.pragma('dart2js:noInline')
  static CancelOp getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CancelOp>(CancelOp.$_createMessage);
  static CancelOp? _defaultInstance;
}

class ListInstalledOp extends $pb.GeneratedMessage {
  factory ListInstalledOp() => ListInstalledOp._();

  ListInstalledOp._();

  factory ListInstalledOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ListInstalledOp()..mergeFromBuffer(data, registry);
  factory ListInstalledOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ListInstalledOp()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ListInstalledOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: ListInstalledOp.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ListInstalledOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ListInstalledOp copyWith(void Function(ListInstalledOp) updates) =>
      super.copyWith((message) => updates(message as ListInstalledOp))
          as ListInstalledOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use ListInstalledOp() / ListInstalledOp.new instead')
  static ListInstalledOp create() => ListInstalledOp._();
  static $pb.GeneratedMessage $_createMessage() => ListInstalledOp._();
  @$core.override
  ListInstalledOp createEmptyInstance() => ListInstalledOp._();
  @$core.pragma('dart2js:noInline')
  static ListInstalledOp getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ListInstalledOp>(
          ListInstalledOp.$_createMessage);
  static ListInstalledOp? _defaultInstance;
}

class SwitchActiveOp extends $pb.GeneratedMessage {
  factory SwitchActiveOp({
    $fixnum.Int64? code,
  }) {
    final result = SwitchActiveOp._();
    if (code != null) result.code = code;
    return result;
  }

  SwitchActiveOp._();

  factory SwitchActiveOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SwitchActiveOp()..mergeFromBuffer(data, registry);
  factory SwitchActiveOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SwitchActiveOp()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SwitchActiveOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: SwitchActiveOp.$_createMessage)
    ..aInt64(1, _omitFieldNames ? '' : 'code')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SwitchActiveOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SwitchActiveOp copyWith(void Function(SwitchActiveOp) updates) =>
      super.copyWith((message) => updates(message as SwitchActiveOp))
          as SwitchActiveOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use SwitchActiveOp() / SwitchActiveOp.new instead')
  static SwitchActiveOp create() => SwitchActiveOp._();
  static $pb.GeneratedMessage $_createMessage() => SwitchActiveOp._();
  @$core.override
  SwitchActiveOp createEmptyInstance() => SwitchActiveOp._();
  @$core.pragma('dart2js:noInline')
  static SwitchActiveOp getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<SwitchActiveOp>(
          SwitchActiveOp.$_createMessage);
  static SwitchActiveOp? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get code => $_getI64(0);
  @$pb.TagNumber(1)
  set code($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCode() => $_has(0);
  @$pb.TagNumber(1)
  void clearCode() => $_clearField(1);
}

class RollbackOp extends $pb.GeneratedMessage {
  factory RollbackOp() => RollbackOp._();

  RollbackOp._();

  factory RollbackOp.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      RollbackOp()..mergeFromBuffer(data, registry);
  factory RollbackOp.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      RollbackOp()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RollbackOp',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: RollbackOp.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RollbackOp clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RollbackOp copyWith(void Function(RollbackOp) updates) =>
      super.copyWith((message) => updates(message as RollbackOp)) as RollbackOp;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use RollbackOp() / RollbackOp.new instead')
  static RollbackOp create() => RollbackOp._();
  static $pb.GeneratedMessage $_createMessage() => RollbackOp._();
  @$core.override
  RollbackOp createEmptyInstance() => RollbackOp._();
  @$core.pragma('dart2js:noInline')
  static RollbackOp getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<RollbackOp>(RollbackOp.$_createMessage);
  static RollbackOp? _defaultInstance;
}

class InstalledVersion extends $pb.GeneratedMessage {
  factory InstalledVersion({
    $fixnum.Int64? code,
    $core.String? version,
    $core.String? path,
    $core.String? executable,
    $core.bool? active,
  }) {
    final result = InstalledVersion._();
    if (code != null) result.code = code;
    if (version != null) result.version = version;
    if (path != null) result.path = path;
    if (executable != null) result.executable = executable;
    if (active != null) result.active = active;
    return result;
  }

  InstalledVersion._();

  factory InstalledVersion.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InstalledVersion()..mergeFromBuffer(data, registry);
  factory InstalledVersion.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InstalledVersion()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'InstalledVersion',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: InstalledVersion.$_createMessage)
    ..aInt64(1, _omitFieldNames ? '' : 'code')
    ..aOS(2, _omitFieldNames ? '' : 'version')
    ..aOS(3, _omitFieldNames ? '' : 'path')
    ..aOS(4, _omitFieldNames ? '' : 'executable')
    ..aOB(5, _omitFieldNames ? '' : 'active')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InstalledVersion clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InstalledVersion copyWith(void Function(InstalledVersion) updates) =>
      super.copyWith((message) => updates(message as InstalledVersion))
          as InstalledVersion;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use InstalledVersion() / InstalledVersion.new instead')
  static InstalledVersion create() => InstalledVersion._();
  static $pb.GeneratedMessage $_createMessage() => InstalledVersion._();
  @$core.override
  InstalledVersion createEmptyInstance() => InstalledVersion._();
  @$core.pragma('dart2js:noInline')
  static InstalledVersion getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<InstalledVersion>(
          InstalledVersion.$_createMessage);
  static InstalledVersion? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get code => $_getI64(0);
  @$pb.TagNumber(1)
  set code($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCode() => $_has(0);
  @$pb.TagNumber(1)
  void clearCode() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get version => $_getSZ(1);
  @$pb.TagNumber(2)
  set version($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasVersion() => $_has(1);
  @$pb.TagNumber(2)
  void clearVersion() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get path => $_getSZ(2);
  @$pb.TagNumber(3)
  set path($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasPath() => $_has(2);
  @$pb.TagNumber(3)
  void clearPath() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get executable => $_getSZ(3);
  @$pb.TagNumber(4)
  set executable($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasExecutable() => $_has(3);
  @$pb.TagNumber(4)
  void clearExecutable() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get active => $_getBF(4);
  @$pb.TagNumber(5)
  set active($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasActive() => $_has(4);
  @$pb.TagNumber(5)
  void clearActive() => $_clearField(5);
}

class InstalledList extends $pb.GeneratedMessage {
  factory InstalledList({
    $core.Iterable<InstalledVersion>? versions,
    $fixnum.Int64? activeCode,
  }) {
    final result = InstalledList._();
    if (versions != null) result.versions.addAll(versions);
    if (activeCode != null) result.activeCode = activeCode;
    return result;
  }

  InstalledList._();

  factory InstalledList.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InstalledList()..mergeFromBuffer(data, registry);
  factory InstalledList.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      InstalledList()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'InstalledList',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: InstalledList.$_createMessage)
    ..pPM<InstalledVersion>(1, _omitFieldNames ? '' : 'versions',
        subBuilder: InstalledVersion.$_createMessage)
    ..aInt64(2, _omitFieldNames ? '' : 'activeCode')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InstalledList clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  InstalledList copyWith(void Function(InstalledList) updates) =>
      super.copyWith((message) => updates(message as InstalledList))
          as InstalledList;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use InstalledList() / InstalledList.new instead')
  static InstalledList create() => InstalledList._();
  static $pb.GeneratedMessage $_createMessage() => InstalledList._();
  @$core.override
  InstalledList createEmptyInstance() => InstalledList._();
  @$core.pragma('dart2js:noInline')
  static InstalledList getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<InstalledList>(
          InstalledList.$_createMessage);
  static InstalledList? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<InstalledVersion> get versions => $_getList(0);

  @$pb.TagNumber(2)
  $fixnum.Int64 get activeCode => $_getI64(1);
  @$pb.TagNumber(2)
  set activeCode($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasActiveCode() => $_has(1);
  @$pb.TagNumber(2)
  void clearActiveCode() => $_clearField(2);
}

enum UpdaterRequest_Op {
  check_10,
  skip,
  download,
  apply,
  status,
  cleanup,
  cancel,
  listInstalled,
  switchActive,
  rollback,
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
    ListInstalledOp? listInstalled,
    SwitchActiveOp? switchActive,
    RollbackOp? rollback,
  }) {
    final result = UpdaterRequest._();
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
    if (listInstalled != null) result.listInstalled = listInstalled;
    if (switchActive != null) result.switchActive = switchActive;
    if (rollback != null) result.rollback = rollback;
    return result;
  }

  UpdaterRequest._();

  factory UpdaterRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      UpdaterRequest()..mergeFromBuffer(data, registry);
  factory UpdaterRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      UpdaterRequest()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, UpdaterRequest_Op> _UpdaterRequest_OpByTag =
      {
    10: UpdaterRequest_Op.check_10,
    11: UpdaterRequest_Op.skip,
    12: UpdaterRequest_Op.download,
    13: UpdaterRequest_Op.apply,
    14: UpdaterRequest_Op.status,
    15: UpdaterRequest_Op.cleanup,
    16: UpdaterRequest_Op.cancel,
    17: UpdaterRequest_Op.listInstalled,
    18: UpdaterRequest_Op.switchActive,
    19: UpdaterRequest_Op.rollback,
    0: UpdaterRequest_Op.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'UpdaterRequest',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: UpdaterRequest.$_createMessage)
    ..oo(0, [10, 11, 12, 13, 14, 15, 16, 17, 18, 19])
    ..aOM<ClientHello>(1, _omitFieldNames ? '' : 'hello',
        subBuilder: ClientHello.$_createMessage)
    ..aOM<ClientProfile>(2, _omitFieldNames ? '' : 'profile',
        subBuilder: ClientProfile.$_createMessage)
    ..aOM<Runtime>(3, _omitFieldNames ? '' : 'runtime',
        subBuilder: Runtime.$_createMessage)
    ..aOM<CheckOp>(10, _omitFieldNames ? '' : 'check',
        subBuilder: CheckOp.$_createMessage)
    ..aOM<SkipOp>(11, _omitFieldNames ? '' : 'skip',
        subBuilder: SkipOp.$_createMessage)
    ..aOM<DownloadOp>(12, _omitFieldNames ? '' : 'download',
        subBuilder: DownloadOp.$_createMessage)
    ..aOM<ApplyOp>(13, _omitFieldNames ? '' : 'apply',
        subBuilder: ApplyOp.$_createMessage)
    ..aOM<StatusOp>(14, _omitFieldNames ? '' : 'status',
        subBuilder: StatusOp.$_createMessage)
    ..aOM<CleanupOp>(15, _omitFieldNames ? '' : 'cleanup',
        subBuilder: CleanupOp.$_createMessage)
    ..aOM<CancelOp>(16, _omitFieldNames ? '' : 'cancel',
        subBuilder: CancelOp.$_createMessage)
    ..aOM<ListInstalledOp>(17, _omitFieldNames ? '' : 'listInstalled',
        subBuilder: ListInstalledOp.$_createMessage)
    ..aOM<SwitchActiveOp>(18, _omitFieldNames ? '' : 'switchActive',
        subBuilder: SwitchActiveOp.$_createMessage)
    ..aOM<RollbackOp>(19, _omitFieldNames ? '' : 'rollback',
        subBuilder: RollbackOp.$_createMessage)
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
  @$core.Deprecated('Use UpdaterRequest() / UpdaterRequest.new instead')
  static UpdaterRequest create() => UpdaterRequest._();
  static $pb.GeneratedMessage $_createMessage() => UpdaterRequest._();
  @$core.override
  UpdaterRequest createEmptyInstance() => UpdaterRequest._();
  @$core.pragma('dart2js:noInline')
  static UpdaterRequest getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<UpdaterRequest>(
          UpdaterRequest.$_createMessage);
  static UpdaterRequest? _defaultInstance;

  @$pb.TagNumber(10)
  @$pb.TagNumber(11)
  @$pb.TagNumber(12)
  @$pb.TagNumber(13)
  @$pb.TagNumber(14)
  @$pb.TagNumber(15)
  @$pb.TagNumber(16)
  @$pb.TagNumber(17)
  @$pb.TagNumber(18)
  @$pb.TagNumber(19)
  UpdaterRequest_Op whichOp() => _UpdaterRequest_OpByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(10)
  @$pb.TagNumber(11)
  @$pb.TagNumber(12)
  @$pb.TagNumber(13)
  @$pb.TagNumber(14)
  @$pb.TagNumber(15)
  @$pb.TagNumber(16)
  @$pb.TagNumber(17)
  @$pb.TagNumber(18)
  @$pb.TagNumber(19)
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

  @$pb.TagNumber(17)
  ListInstalledOp get listInstalled => $_getN(10);
  @$pb.TagNumber(17)
  set listInstalled(ListInstalledOp value) => $_setField(17, value);
  @$pb.TagNumber(17)
  $core.bool hasListInstalled() => $_has(10);
  @$pb.TagNumber(17)
  void clearListInstalled() => $_clearField(17);
  @$pb.TagNumber(17)
  ListInstalledOp ensureListInstalled() => $_ensure(10);

  @$pb.TagNumber(18)
  SwitchActiveOp get switchActive => $_getN(11);
  @$pb.TagNumber(18)
  set switchActive(SwitchActiveOp value) => $_setField(18, value);
  @$pb.TagNumber(18)
  $core.bool hasSwitchActive() => $_has(11);
  @$pb.TagNumber(18)
  void clearSwitchActive() => $_clearField(18);
  @$pb.TagNumber(18)
  SwitchActiveOp ensureSwitchActive() => $_ensure(11);

  @$pb.TagNumber(19)
  RollbackOp get rollback => $_getN(12);
  @$pb.TagNumber(19)
  set rollback(RollbackOp value) => $_setField(19, value);
  @$pb.TagNumber(19)
  $core.bool hasRollback() => $_has(12);
  @$pb.TagNumber(19)
  void clearRollback() => $_clearField(19);
  @$pb.TagNumber(19)
  RollbackOp ensureRollback() => $_ensure(12);
}

class Error extends $pb.GeneratedMessage {
  factory Error({
    ErrorCode? code,
    $core.bool? retryable,
    $core.String? message,
    $core.Iterable<$core.String>? attempts,
    RecoveryHelp? recovery,
  }) {
    final result = Error._();
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
      Error()..mergeFromBuffer(data, registry);
  factory Error.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Error()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Error',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: Error.$_createMessage)
    ..aE<ErrorCode>(1, _omitFieldNames ? '' : 'code',
        enumValues: ErrorCode.values)
    ..aOB(2, _omitFieldNames ? '' : 'retryable')
    ..aOS(3, _omitFieldNames ? '' : 'message')
    ..pPS(4, _omitFieldNames ? '' : 'attempts')
    ..aOM<RecoveryHelp>(5, _omitFieldNames ? '' : 'recovery',
        subBuilder: RecoveryHelp.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Error clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Error copyWith(void Function(Error) updates) =>
      super.copyWith((message) => updates(message as Error)) as Error;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use Error() / Error.new instead')
  static Error create() => Error._();
  static $pb.GeneratedMessage $_createMessage() => Error._();
  @$core.override
  Error createEmptyInstance() => Error._();
  @$core.pragma('dart2js:noInline')
  static Error getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Error>(Error.$_createMessage);
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
    final result = PriorReleaseNotes._();
    if (version != null) result.version = version;
    if (code != null) result.code = code;
    if (notes != null) result.notes = notes;
    if (notesUrl != null) result.notesUrl = notesUrl;
    return result;
  }

  PriorReleaseNotes._();

  factory PriorReleaseNotes.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PriorReleaseNotes()..mergeFromBuffer(data, registry);
  factory PriorReleaseNotes.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PriorReleaseNotes()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PriorReleaseNotes',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: PriorReleaseNotes.$_createMessage)
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
  @$core.Deprecated('Use PriorReleaseNotes() / PriorReleaseNotes.new instead')
  static PriorReleaseNotes create() => PriorReleaseNotes._();
  static $pb.GeneratedMessage $_createMessage() => PriorReleaseNotes._();
  @$core.override
  PriorReleaseNotes createEmptyInstance() => PriorReleaseNotes._();
  @$core.pragma('dart2js:noInline')
  static PriorReleaseNotes getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<PriorReleaseNotes>(
          PriorReleaseNotes.$_createMessage);
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
    final result = ArtifactView._();
    if (name != null) result.name = name;
    if (size != null) result.size = size;
    if (sha256 != null) result.sha256 = sha256;
    return result;
  }

  ArtifactView._();

  factory ArtifactView.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ArtifactView()..mergeFromBuffer(data, registry);
  factory ArtifactView.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ArtifactView()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ArtifactView',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: ArtifactView.$_createMessage)
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
  @$core.Deprecated('Use ArtifactView() / ArtifactView.new instead')
  static ArtifactView create() => ArtifactView._();
  static $pb.GeneratedMessage $_createMessage() => ArtifactView._();
  @$core.override
  ArtifactView createEmptyInstance() => ArtifactView._();
  @$core.pragma('dart2js:noInline')
  static ArtifactView getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ArtifactView>(
          ArtifactView.$_createMessage);
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
    final result = UpToDate._();
    if (sequence != null) result.sequence = sequence;
    if (currentIsYanked != null) result.currentIsYanked = currentIsYanked;
    return result;
  }

  UpToDate._();

  factory UpToDate.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      UpToDate()..mergeFromBuffer(data, registry);
  factory UpToDate.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      UpToDate()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'UpToDate',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: UpToDate.$_createMessage)
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
  @$core.Deprecated('Use UpToDate() / UpToDate.new instead')
  static UpToDate create() => UpToDate._();
  static $pb.GeneratedMessage $_createMessage() => UpToDate._();
  @$core.override
  UpToDate createEmptyInstance() => UpToDate._();
  @$core.pragma('dart2js:noInline')
  static UpToDate getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<UpToDate>(UpToDate.$_createMessage);
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
    final result = UpdateAvailable._();
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
      UpdateAvailable()..mergeFromBuffer(data, registry);
  factory UpdateAvailable.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      UpdateAvailable()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'UpdateAvailable',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: UpdateAvailable.$_createMessage)
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
        subBuilder: PriorReleaseNotes.$_createMessage)
    ..pPM<ArtifactView>(11, _omitFieldNames ? '' : 'artifacts',
        subBuilder: ArtifactView.$_createMessage)
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
  @$core.Deprecated('Use UpdateAvailable() / UpdateAvailable.new instead')
  static UpdateAvailable create() => UpdateAvailable._();
  static $pb.GeneratedMessage $_createMessage() => UpdateAvailable._();
  @$core.override
  UpdateAvailable createEmptyInstance() => UpdateAvailable._();
  @$core.pragma('dart2js:noInline')
  static UpdateAvailable getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<UpdateAvailable>(
          UpdateAvailable.$_createMessage);
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
    final result = FallbackRequired._();
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
      FallbackRequired()..mergeFromBuffer(data, registry);
  factory FallbackRequired.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      FallbackRequired()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FallbackRequired',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: FallbackRequired.$_createMessage)
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
  @$core.Deprecated('Use FallbackRequired() / FallbackRequired.new instead')
  static FallbackRequired create() => FallbackRequired._();
  static $pb.GeneratedMessage $_createMessage() => FallbackRequired._();
  @$core.override
  FallbackRequired createEmptyInstance() => FallbackRequired._();
  @$core.pragma('dart2js:noInline')
  static FallbackRequired getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<FallbackRequired>(
          FallbackRequired.$_createMessage);
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
    final result = Throttled._();
    if (nextAllowedAt != null) result.nextAllowedAt = nextAllowedAt;
    return result;
  }

  Throttled._();

  factory Throttled.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Throttled()..mergeFromBuffer(data, registry);
  factory Throttled.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Throttled()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Throttled',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: Throttled.$_createMessage)
    ..aOM<$1.Timestamp>(1, _omitFieldNames ? '' : 'nextAllowedAt',
        subBuilder: $1.Timestamp.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Throttled clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Throttled copyWith(void Function(Throttled) updates) =>
      super.copyWith((message) => updates(message as Throttled)) as Throttled;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use Throttled() / Throttled.new instead')
  static Throttled create() => Throttled._();
  static $pb.GeneratedMessage $_createMessage() => Throttled._();
  @$core.override
  Throttled createEmptyInstance() => Throttled._();
  @$core.pragma('dart2js:noInline')
  static Throttled getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Throttled>(Throttled.$_createMessage);
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
    final result = Failed._();
    if (error != null) result.error = error;
    return result;
  }

  Failed._();

  factory Failed.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Failed()..mergeFromBuffer(data, registry);
  factory Failed.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Failed()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Failed',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: Failed.$_createMessage)
    ..aOM<Error>(1, _omitFieldNames ? '' : 'error',
        subBuilder: Error.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Failed clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Failed copyWith(void Function(Failed) updates) =>
      super.copyWith((message) => updates(message as Failed)) as Failed;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use Failed() / Failed.new instead')
  static Failed create() => Failed._();
  static $pb.GeneratedMessage $_createMessage() => Failed._();
  @$core.override
  Failed createEmptyInstance() => Failed._();
  @$core.pragma('dart2js:noInline')
  static Failed getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Failed>(Failed.$_createMessage);
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
    final result = CheckResult._();
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
      CheckResult()..mergeFromBuffer(data, registry);
  factory CheckResult.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      CheckResult()..mergeFromJson(json, registry);

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
      createEmptyInstance: CheckResult.$_createMessage)
    ..oo(0, [1, 2, 3, 4, 5])
    ..aOM<UpToDate>(1, _omitFieldNames ? '' : 'upToDate',
        subBuilder: UpToDate.$_createMessage)
    ..aOM<UpdateAvailable>(2, _omitFieldNames ? '' : 'updateAvailable',
        subBuilder: UpdateAvailable.$_createMessage)
    ..aOM<FallbackRequired>(3, _omitFieldNames ? '' : 'fallbackRequired',
        subBuilder: FallbackRequired.$_createMessage)
    ..aOM<Throttled>(4, _omitFieldNames ? '' : 'throttled',
        subBuilder: Throttled.$_createMessage)
    ..aOM<Failed>(5, _omitFieldNames ? '' : 'failed',
        subBuilder: Failed.$_createMessage)
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
  @$core.Deprecated('Use CheckResult() / CheckResult.new instead')
  static CheckResult create() => CheckResult._();
  static $pb.GeneratedMessage $_createMessage() => CheckResult._();
  @$core.override
  CheckResult createEmptyInstance() => CheckResult._();
  @$core.pragma('dart2js:noInline')
  static CheckResult getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<CheckResult>(
          CheckResult.$_createMessage);
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
    final result = Downloaded._();
    if (planId != null) result.planId = planId;
    if (bytes != null) result.bytes = bytes;
    return result;
  }

  Downloaded._();

  factory Downloaded.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Downloaded()..mergeFromBuffer(data, registry);
  factory Downloaded.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Downloaded()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Downloaded',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: Downloaded.$_createMessage)
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
  @$core.Deprecated('Use Downloaded() / Downloaded.new instead')
  static Downloaded create() => Downloaded._();
  static $pb.GeneratedMessage $_createMessage() => Downloaded._();
  @$core.override
  Downloaded createEmptyInstance() => Downloaded._();
  @$core.pragma('dart2js:noInline')
  static Downloaded getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Downloaded>(Downloaded.$_createMessage);
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
    final result = DownloadResult._();
    if (downloaded != null) result.downloaded = downloaded;
    if (failed != null) result.failed = failed;
    return result;
  }

  DownloadResult._();

  factory DownloadResult.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      DownloadResult()..mergeFromBuffer(data, registry);
  factory DownloadResult.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      DownloadResult()..mergeFromJson(json, registry);

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
      createEmptyInstance: DownloadResult.$_createMessage)
    ..oo(0, [1, 2])
    ..aOM<Downloaded>(1, _omitFieldNames ? '' : 'downloaded',
        subBuilder: Downloaded.$_createMessage)
    ..aOM<Failed>(2, _omitFieldNames ? '' : 'failed',
        subBuilder: Failed.$_createMessage)
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
  @$core.Deprecated('Use DownloadResult() / DownloadResult.new instead')
  static DownloadResult create() => DownloadResult._();
  static $pb.GeneratedMessage $_createMessage() => DownloadResult._();
  @$core.override
  DownloadResult createEmptyInstance() => DownloadResult._();
  @$core.pragma('dart2js:noInline')
  static DownloadResult getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<DownloadResult>(
          DownloadResult.$_createMessage);
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
    final result = ApplyAccepted._();
    if (sessionId != null) result.sessionId = sessionId;
    if (planId != null) result.planId = planId;
    if (requiresHostExit != null) result.requiresHostExit = requiresHostExit;
    return result;
  }

  ApplyAccepted._();

  factory ApplyAccepted.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ApplyAccepted()..mergeFromBuffer(data, registry);
  factory ApplyAccepted.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ApplyAccepted()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ApplyAccepted',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: ApplyAccepted.$_createMessage)
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
  @$core.Deprecated('Use ApplyAccepted() / ApplyAccepted.new instead')
  static ApplyAccepted create() => ApplyAccepted._();
  static $pb.GeneratedMessage $_createMessage() => ApplyAccepted._();
  @$core.override
  ApplyAccepted createEmptyInstance() => ApplyAccepted._();
  @$core.pragma('dart2js:noInline')
  static ApplyAccepted getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ApplyAccepted>(
          ApplyAccepted.$_createMessage);
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
    final result = ApplyResult._();
    if (accepted != null) result.accepted = accepted;
    if (failed != null) result.failed = failed;
    return result;
  }

  ApplyResult._();

  factory ApplyResult.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ApplyResult()..mergeFromBuffer(data, registry);
  factory ApplyResult.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ApplyResult()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, ApplyResult_Kind> _ApplyResult_KindByTag = {
    1: ApplyResult_Kind.accepted,
    2: ApplyResult_Kind.failed,
    0: ApplyResult_Kind.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ApplyResult',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: ApplyResult.$_createMessage)
    ..oo(0, [1, 2])
    ..aOM<ApplyAccepted>(1, _omitFieldNames ? '' : 'accepted',
        subBuilder: ApplyAccepted.$_createMessage)
    ..aOM<Failed>(2, _omitFieldNames ? '' : 'failed',
        subBuilder: Failed.$_createMessage)
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
  @$core.Deprecated('Use ApplyResult() / ApplyResult.new instead')
  static ApplyResult create() => ApplyResult._();
  static $pb.GeneratedMessage $_createMessage() => ApplyResult._();
  @$core.override
  ApplyResult createEmptyInstance() => ApplyResult._();
  @$core.pragma('dart2js:noInline')
  static ApplyResult getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ApplyResult>(
          ApplyResult.$_createMessage);
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
  factory Ok() => Ok._();

  Ok._();

  factory Ok.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Ok()..mergeFromBuffer(data, registry);
  factory Ok.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Ok()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Ok',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: Ok.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Ok clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Ok copyWith(void Function(Ok) updates) =>
      super.copyWith((message) => updates(message as Ok)) as Ok;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use Ok() / Ok.new instead')
  static Ok create() => Ok._();
  static $pb.GeneratedMessage $_createMessage() => Ok._();
  @$core.override
  Ok createEmptyInstance() => Ok._();
  @$core.pragma('dart2js:noInline')
  static Ok getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Ok>(Ok.$_createMessage);
  static Ok? _defaultInstance;
}

enum Result_Kind { ok, failed, notSet }

class Result extends $pb.GeneratedMessage {
  factory Result({
    Ok? ok,
    Failed? failed,
  }) {
    final result = Result._();
    if (ok != null) result.ok = ok;
    if (failed != null) result.failed = failed;
    return result;
  }

  Result._();

  factory Result.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Result()..mergeFromBuffer(data, registry);
  factory Result.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Result()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, Result_Kind> _Result_KindByTag = {
    1: Result_Kind.ok,
    2: Result_Kind.failed,
    0: Result_Kind.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Result',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: Result.$_createMessage)
    ..oo(0, [1, 2])
    ..aOM<Ok>(1, _omitFieldNames ? '' : 'ok', subBuilder: Ok.$_createMessage)
    ..aOM<Failed>(2, _omitFieldNames ? '' : 'failed',
        subBuilder: Failed.$_createMessage)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Result clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Result copyWith(void Function(Result) updates) =>
      super.copyWith((message) => updates(message as Result)) as Result;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core.Deprecated('Use Result() / Result.new instead')
  static Result create() => Result._();
  static $pb.GeneratedMessage $_createMessage() => Result._();
  @$core.override
  Result createEmptyInstance() => Result._();
  @$core.pragma('dart2js:noInline')
  static Result getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Result>(Result.$_createMessage);
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
    final result = SidecarInfo._();
    if (path != null) result.path = path;
    if (ipc != null) result.ipc = ipc;
    if (version != null) result.version = version;
    return result;
  }

  SidecarInfo._();

  factory SidecarInfo.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SidecarInfo()..mergeFromBuffer(data, registry);
  factory SidecarInfo.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SidecarInfo()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SidecarInfo',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: SidecarInfo.$_createMessage)
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
  @$core.Deprecated('Use SidecarInfo() / SidecarInfo.new instead')
  static SidecarInfo create() => SidecarInfo._();
  static $pb.GeneratedMessage $_createMessage() => SidecarInfo._();
  @$core.override
  SidecarInfo createEmptyInstance() => SidecarInfo._();
  @$core.pragma('dart2js:noInline')
  static SidecarInfo getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<SidecarInfo>(
          SidecarInfo.$_createMessage);
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
    final result = SessionView._();
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
      SessionView()..mergeFromBuffer(data, registry);
  factory SessionView.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      SessionView()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SessionView',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: SessionView.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..aOS(2, _omitFieldNames ? '' : 'planId')
    ..aE<SessionPhase>(3, _omitFieldNames ? '' : 'phase',
        enumValues: SessionPhase.values)
    ..aOM<$1.Timestamp>(4, _omitFieldNames ? '' : 'startedAt',
        subBuilder: $1.Timestamp.$_createMessage)
    ..aOM<Error>(5, _omitFieldNames ? '' : 'error',
        subBuilder: Error.$_createMessage)
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
  @$core.Deprecated('Use SessionView() / SessionView.new instead')
  static SessionView create() => SessionView._();
  static $pb.GeneratedMessage $_createMessage() => SessionView._();
  @$core.override
  SessionView createEmptyInstance() => SessionView._();
  @$core.pragma('dart2js:noInline')
  static SessionView getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<SessionView>(
          SessionView.$_createMessage);
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
    final result = StatusSnapshot._();
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
      StatusSnapshot()..mergeFromBuffer(data, registry);
  factory StatusSnapshot.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      StatusSnapshot()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'StatusSnapshot',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: StatusSnapshot.$_createMessage)
    ..aOM<$1.Timestamp>(1, _omitFieldNames ? '' : 'lastCheckAt',
        subBuilder: $1.Timestamp.$_createMessage)
    ..aE<LastResult>(2, _omitFieldNames ? '' : 'lastResult',
        enumValues: LastResult.values)
    ..aOM<$1.Timestamp>(3, _omitFieldNames ? '' : 'nextAllowedAt',
        subBuilder: $1.Timestamp.$_createMessage)
    ..aInt64(4, _omitFieldNames ? '' : 'lastSeenSequence')
    ..p<$fixnum.Int64>(
        5, _omitFieldNames ? '' : 'skippedCodes', $pb.PbFieldType.K6)
    ..aOM<SessionView>(6, _omitFieldNames ? '' : 'activeSession',
        subBuilder: SessionView.$_createMessage)
    ..aOM<SidecarInfo>(7, _omitFieldNames ? '' : 'sidecar',
        subBuilder: SidecarInfo.$_createMessage)
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
  @$core.Deprecated('Use StatusSnapshot() / StatusSnapshot.new instead')
  static StatusSnapshot create() => StatusSnapshot._();
  static $pb.GeneratedMessage $_createMessage() => StatusSnapshot._();
  @$core.override
  StatusSnapshot createEmptyInstance() => StatusSnapshot._();
  @$core.pragma('dart2js:noInline')
  static StatusSnapshot getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<StatusSnapshot>(
          StatusSnapshot.$_createMessage);
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
    final result = Progress._();
    if (bytesReceived != null) result.bytesReceived = bytesReceived;
    if (bytesTotal != null) result.bytesTotal = bytesTotal;
    if (bytesPerSecond != null) result.bytesPerSecond = bytesPerSecond;
    return result;
  }

  Progress._();

  factory Progress.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Progress()..mergeFromBuffer(data, registry);
  factory Progress.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Progress()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Progress',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: Progress.$_createMessage)
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
  @$core.Deprecated('Use Progress() / Progress.new instead')
  static Progress create() => Progress._();
  static $pb.GeneratedMessage $_createMessage() => Progress._();
  @$core.override
  Progress createEmptyInstance() => Progress._();
  @$core.pragma('dart2js:noInline')
  static Progress getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Progress>(Progress.$_createMessage);
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
    final result = ApplyProgress._();
    if (sessionId != null) result.sessionId = sessionId;
    if (phase != null) result.phase = phase;
    return result;
  }

  ApplyProgress._();

  factory ApplyProgress.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ApplyProgress()..mergeFromBuffer(data, registry);
  factory ApplyProgress.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ApplyProgress()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ApplyProgress',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: ApplyProgress.$_createMessage)
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
  @$core.Deprecated('Use ApplyProgress() / ApplyProgress.new instead')
  static ApplyProgress create() => ApplyProgress._();
  static $pb.GeneratedMessage $_createMessage() => ApplyProgress._();
  @$core.override
  ApplyProgress createEmptyInstance() => ApplyProgress._();
  @$core.pragma('dart2js:noInline')
  static ApplyProgress getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ApplyProgress>(
          ApplyProgress.$_createMessage);
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
    final result = Log._();
    if (message != null) result.message = message;
    return result;
  }

  Log._();

  factory Log.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Log()..mergeFromBuffer(data, registry);
  factory Log.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      Log()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Log',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: Log.$_createMessage)
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
  @$core.Deprecated('Use Log() / Log.new instead')
  static Log create() => Log._();
  static $pb.GeneratedMessage $_createMessage() => Log._();
  @$core.override
  Log createEmptyInstance() => Log._();
  @$core.pragma('dart2js:noInline')
  static Log getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Log>(Log.$_createMessage);
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
  installed,
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
    InstalledList? installed,
  }) {
    final result$ = UpdaterEvent._();
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
    if (installed != null) result$.installed = installed;
    return result$;
  }

  UpdaterEvent._();

  factory UpdaterEvent.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      UpdaterEvent()..mergeFromBuffer(data, registry);
  factory UpdaterEvent.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      UpdaterEvent()..mergeFromJson(json, registry);

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
    16: UpdaterEvent_Kind.installed,
    0: UpdaterEvent_Kind.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'UpdaterEvent',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: UpdaterEvent.$_createMessage)
    ..oo(0, [1, 2, 3, 4, 10, 11, 12, 13, 14, 15, 16])
    ..aOM<Capabilities>(1, _omitFieldNames ? '' : 'capabilities',
        subBuilder: Capabilities.$_createMessage)
    ..aOM<Progress>(2, _omitFieldNames ? '' : 'progress',
        subBuilder: Progress.$_createMessage)
    ..aOM<ApplyProgress>(3, _omitFieldNames ? '' : 'applyProgress',
        subBuilder: ApplyProgress.$_createMessage)
    ..aOM<Log>(4, _omitFieldNames ? '' : 'log', subBuilder: Log.$_createMessage)
    ..aOM<CheckResult>(10, _omitFieldNames ? '' : 'check',
        subBuilder: CheckResult.$_createMessage)
    ..aOM<DownloadResult>(11, _omitFieldNames ? '' : 'download',
        subBuilder: DownloadResult.$_createMessage)
    ..aOM<ApplyResult>(12, _omitFieldNames ? '' : 'apply',
        subBuilder: ApplyResult.$_createMessage)
    ..aOM<Result>(13, _omitFieldNames ? '' : 'result',
        subBuilder: Result.$_createMessage)
    ..aOM<StatusSnapshot>(14, _omitFieldNames ? '' : 'status',
        subBuilder: StatusSnapshot.$_createMessage)
    ..aOM<Failed>(15, _omitFieldNames ? '' : 'failed',
        subBuilder: Failed.$_createMessage)
    ..aOM<InstalledList>(16, _omitFieldNames ? '' : 'installed',
        subBuilder: InstalledList.$_createMessage)
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
  @$core.Deprecated('Use UpdaterEvent() / UpdaterEvent.new instead')
  static UpdaterEvent create() => UpdaterEvent._();
  static $pb.GeneratedMessage $_createMessage() => UpdaterEvent._();
  @$core.override
  UpdaterEvent createEmptyInstance() => UpdaterEvent._();
  @$core.pragma('dart2js:noInline')
  static UpdaterEvent getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<UpdaterEvent>(
          UpdaterEvent.$_createMessage);
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
  @$pb.TagNumber(16)
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
  @$pb.TagNumber(16)
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

  @$pb.TagNumber(16)
  InstalledList get installed => $_getN(10);
  @$pb.TagNumber(16)
  set installed(InstalledList value) => $_setField(16, value);
  @$pb.TagNumber(16)
  $core.bool hasInstalled() => $_has(10);
  @$pb.TagNumber(16)
  void clearInstalled() => $_clearField(16);
  @$pb.TagNumber(16)
  InstalledList ensureInstalled() => $_ensure(10);
}

class ArtifactTarget extends $pb.GeneratedMessage {
  factory ArtifactTarget({
    $core.String? name,
    $core.Iterable<$core.MapEntry<$core.String, $core.String>>? selectors,
  }) {
    final result = ArtifactTarget._();
    if (name != null) result.name = name;
    if (selectors != null) result.selectors.addEntries(selectors);
    return result;
  }

  ArtifactTarget._();

  factory ArtifactTarget.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ArtifactTarget()..mergeFromBuffer(data, registry);
  factory ArtifactTarget.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ArtifactTarget()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ArtifactTarget',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: ArtifactTarget.$_createMessage)
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
  @$core.Deprecated('Use ArtifactTarget() / ArtifactTarget.new instead')
  static ArtifactTarget create() => ArtifactTarget._();
  static $pb.GeneratedMessage $_createMessage() => ArtifactTarget._();
  @$core.override
  ArtifactTarget createEmptyInstance() => ArtifactTarget._();
  @$core.pragma('dart2js:noInline')
  static ArtifactTarget getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ArtifactTarget>(
          ArtifactTarget.$_createMessage);
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
    final result = PlannedFile._();
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
      PlannedFile()..mergeFromBuffer(data, registry);
  factory PlannedFile.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PlannedFile()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PlannedFile',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: PlannedFile.$_createMessage)
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
  @$core.Deprecated('Use PlannedFile() / PlannedFile.new instead')
  static PlannedFile create() => PlannedFile._();
  static $pb.GeneratedMessage $_createMessage() => PlannedFile._();
  @$core.override
  PlannedFile createEmptyInstance() => PlannedFile._();
  @$core.pragma('dart2js:noInline')
  static PlannedFile getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<PlannedFile>(
          PlannedFile.$_createMessage);
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
    final result = UpdatePlan._();
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
      UpdatePlan()..mergeFromBuffer(data, registry);
  factory UpdatePlan.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      UpdatePlan()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'UpdatePlan',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: UpdatePlan.$_createMessage)
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
        subBuilder: PriorReleaseNotes.$_createMessage)
    ..pPM<PlannedFile>(12, _omitFieldNames ? '' : 'files',
        subBuilder: PlannedFile.$_createMessage)
    ..aOM<$1.Timestamp>(13, _omitFieldNames ? '' : 'createdAt',
        subBuilder: $1.Timestamp.$_createMessage)
    ..aOM<$1.Timestamp>(14, _omitFieldNames ? '' : 'expiresAt',
        subBuilder: $1.Timestamp.$_createMessage)
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
  @$core.Deprecated('Use UpdatePlan() / UpdatePlan.new instead')
  static UpdatePlan create() => UpdatePlan._();
  static $pb.GeneratedMessage $_createMessage() => UpdatePlan._();
  @$core.override
  UpdatePlan createEmptyInstance() => UpdatePlan._();
  @$core.pragma('dart2js:noInline')
  static UpdatePlan getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<UpdatePlan>(UpdatePlan.$_createMessage);
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
    final result = PersistedState._();
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
      PersistedState()..mergeFromBuffer(data, registry);
  factory PersistedState.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      PersistedState()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PersistedState',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: PersistedState.$_createMessage)
    ..aOM<$1.Timestamp>(1, _omitFieldNames ? '' : 'lastCheckAt',
        subBuilder: $1.Timestamp.$_createMessage)
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
  @$core.Deprecated('Use PersistedState() / PersistedState.new instead')
  static PersistedState create() => PersistedState._();
  static $pb.GeneratedMessage $_createMessage() => PersistedState._();
  @$core.override
  PersistedState createEmptyInstance() => PersistedState._();
  @$core.pragma('dart2js:noInline')
  static PersistedState getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<PersistedState>(
          PersistedState.$_createMessage);
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
    $core.bool? installOnly,
    $core.Iterable<$fixnum.Int64>? reservedCodes,
  }) {
    final result = ApplySessionRecord._();
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
    if (installOnly != null) result.installOnly = installOnly;
    if (reservedCodes != null) result.reservedCodes.addAll(reservedCodes);
    return result;
  }

  ApplySessionRecord._();

  factory ApplySessionRecord.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ApplySessionRecord()..mergeFromBuffer(data, registry);
  factory ApplySessionRecord.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ApplySessionRecord()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ApplySessionRecord',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: ApplySessionRecord.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..aOS(2, _omitFieldNames ? '' : 'planId')
    ..aE<SessionPhase>(3, _omitFieldNames ? '' : 'phase',
        enumValues: SessionPhase.values)
    ..aOM<$1.Timestamp>(4, _omitFieldNames ? '' : 'startedAt',
        subBuilder: $1.Timestamp.$_createMessage)
    ..aOM<$1.Timestamp>(5, _omitFieldNames ? '' : 'heartbeatAt',
        subBuilder: $1.Timestamp.$_createMessage)
    ..aOS(6, _omitFieldNames ? '' : 'installRoot')
    ..aOS(7, _omitFieldNames ? '' : 'stagedRoot')
    ..aInt64(8, _omitFieldNames ? '' : 'targetCode')
    ..aOS(9, _omitFieldNames ? '' : 'targetVersion')
    ..aOM<Error>(10, _omitFieldNames ? '' : 'error',
        subBuilder: Error.$_createMessage)
    ..aI(11, _omitFieldNames ? '' : 'pid')
    ..aE<Layout>(12, _omitFieldNames ? '' : 'layout', enumValues: Layout.values)
    ..aOB(13, _omitFieldNames ? '' : 'relaunch')
    ..aOS(14, _omitFieldNames ? '' : 'executableRelpath')
    ..pPS(15, _omitFieldNames ? '' : 'preserve')
    ..aI(16, _omitFieldNames ? '' : 'retain')
    ..pPM<FileSetEntry>(17, _omitFieldNames ? '' : 'fileSet',
        subBuilder: FileSetEntry.$_createMessage)
    ..aOS(18, _omitFieldNames ? '' : 'sidecarRelpath')
    ..aOB(19, _omitFieldNames ? '' : 'installOnly')
    ..p<$fixnum.Int64>(
        20, _omitFieldNames ? '' : 'reservedCodes', $pb.PbFieldType.K6)
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
  @$core.Deprecated('Use ApplySessionRecord() / ApplySessionRecord.new instead')
  static ApplySessionRecord create() => ApplySessionRecord._();
  static $pb.GeneratedMessage $_createMessage() => ApplySessionRecord._();
  @$core.override
  ApplySessionRecord createEmptyInstance() => ApplySessionRecord._();
  @$core.pragma('dart2js:noInline')
  static ApplySessionRecord getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ApplySessionRecord>(
          ApplySessionRecord.$_createMessage);
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

  @$pb.TagNumber(19)
  $core.bool get installOnly => $_getBF(18);
  @$pb.TagNumber(19)
  set installOnly($core.bool value) => $_setBool(18, value);
  @$pb.TagNumber(19)
  $core.bool hasInstallOnly() => $_has(18);
  @$pb.TagNumber(19)
  void clearInstallOnly() => $_clearField(19);

  @$pb.TagNumber(20)
  $pb.PbList<$fixnum.Int64> get reservedCodes => $_getList(19);
}

class JournalEntry extends $pb.GeneratedMessage {
  factory JournalEntry({
    $core.String? destPath,
    $core.String? backupPath,
    $core.String? sourcePath,
  }) {
    final result = JournalEntry._();
    if (destPath != null) result.destPath = destPath;
    if (backupPath != null) result.backupPath = backupPath;
    if (sourcePath != null) result.sourcePath = sourcePath;
    return result;
  }

  JournalEntry._();

  factory JournalEntry.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      JournalEntry()..mergeFromBuffer(data, registry);
  factory JournalEntry.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      JournalEntry()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'JournalEntry',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: JournalEntry.$_createMessage)
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
  @$core.Deprecated('Use JournalEntry() / JournalEntry.new instead')
  static JournalEntry create() => JournalEntry._();
  static $pb.GeneratedMessage $_createMessage() => JournalEntry._();
  @$core.override
  JournalEntry createEmptyInstance() => JournalEntry._();
  @$core.pragma('dart2js:noInline')
  static JournalEntry getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<JournalEntry>(
          JournalEntry.$_createMessage);
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
    final result = ApplyJournal._();
    if (sessionId != null) result.sessionId = sessionId;
    if (entries != null) result.entries.addAll(entries);
    if (committed != null) result.committed = committed;
    return result;
  }

  ApplyJournal._();

  factory ApplyJournal.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ApplyJournal()..mergeFromBuffer(data, registry);
  factory ApplyJournal.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      ApplyJournal()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ApplyJournal',
      package:
          const $pb.PackageName(_omitMessageNames ? '' : 'relkit.updater.v1'),
      createEmptyInstance: ApplyJournal.$_createMessage)
    ..aOS(1, _omitFieldNames ? '' : 'sessionId')
    ..pPM<JournalEntry>(2, _omitFieldNames ? '' : 'entries',
        subBuilder: JournalEntry.$_createMessage)
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
  @$core.Deprecated('Use ApplyJournal() / ApplyJournal.new instead')
  static ApplyJournal create() => ApplyJournal._();
  static $pb.GeneratedMessage $_createMessage() => ApplyJournal._();
  @$core.override
  ApplyJournal createEmptyInstance() => ApplyJournal._();
  @$core.pragma('dart2js:noInline')
  static ApplyJournal getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<ApplyJournal>(
          ApplyJournal.$_createMessage);
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
