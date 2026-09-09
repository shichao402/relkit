// This is a generated file - do not edit.
//
// Generated from updater/v1/updater.proto.

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

@$core.Deprecated('Use errorCodeDescriptor instead')
const ErrorCode$json = {
  '1': 'ErrorCode',
  '2': [
    {'1': 'ERROR_CODE_UNSPECIFIED', '2': 0},
    {'1': 'ERROR_CODE_NETWORK', '2': 1},
    {'1': 'ERROR_CODE_SIGNATURE', '2': 2},
    {'1': 'ERROR_CODE_ROLLBACK_REJECTED', '2': 3},
    {'1': 'ERROR_CODE_SELECTOR_NO_MATCH', '2': 4},
    {'1': 'ERROR_CODE_DISK', '2': 5},
    {'1': 'ERROR_CODE_PERMISSION_DENIED', '2': 6},
    {'1': 'ERROR_CODE_OCCUPIED', '2': 7},
    {'1': 'ERROR_CODE_PROTOCOL_MISMATCH', '2': 8},
    {'1': 'ERROR_CODE_UPDATER_TOO_OLD', '2': 9},
    {'1': 'ERROR_CODE_UPDATER_TOO_NEW', '2': 10},
    {'1': 'ERROR_CODE_PLAN_TAMPERED', '2': 11},
    {'1': 'ERROR_CODE_PLAN_EXPIRED', '2': 12},
    {'1': 'ERROR_CODE_PLAN_UNKNOWN', '2': 13},
    {'1': 'ERROR_CODE_PLAN_NOT_DOWNLOADED', '2': 14},
    {'1': 'ERROR_CODE_SKIP_DENIED', '2': 15},
    {'1': 'ERROR_CODE_PROFILE_INVALID', '2': 16},
    {'1': 'ERROR_CODE_CHANNEL_NOT_ALLOWED', '2': 17},
    {'1': 'ERROR_CODE_CANCELED', '2': 18},
    {'1': 'ERROR_CODE_SIDECAR_NOT_FOUND', '2': 19},
    {'1': 'ERROR_CODE_LAYOUT_UNSUPPORTED', '2': 20},
  ],
};

/// Descriptor for `ErrorCode`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List errorCodeDescriptor = $convert.base64Decode(
    'CglFcnJvckNvZGUSGgoWRVJST1JfQ09ERV9VTlNQRUNJRklFRBAAEhYKEkVSUk9SX0NPREVfTk'
    'VUV09SSxABEhgKFEVSUk9SX0NPREVfU0lHTkFUVVJFEAISIAocRVJST1JfQ09ERV9ST0xMQkFD'
    'S19SRUpFQ1RFRBADEiAKHEVSUk9SX0NPREVfU0VMRUNUT1JfTk9fTUFUQ0gQBBITCg9FUlJPUl'
    '9DT0RFX0RJU0sQBRIgChxFUlJPUl9DT0RFX1BFUk1JU1NJT05fREVOSUVEEAYSFwoTRVJST1Jf'
    'Q09ERV9PQ0NVUElFRBAHEiAKHEVSUk9SX0NPREVfUFJPVE9DT0xfTUlTTUFUQ0gQCBIeChpFUl'
    'JPUl9DT0RFX1VQREFURVJfVE9PX09MRBAJEh4KGkVSUk9SX0NPREVfVVBEQVRFUl9UT09fTkVX'
    'EAoSHAoYRVJST1JfQ09ERV9QTEFOX1RBTVBFUkVEEAsSGwoXRVJST1JfQ09ERV9QTEFOX0VYUE'
    'lSRUQQDBIbChdFUlJPUl9DT0RFX1BMQU5fVU5LTk9XThANEiIKHkVSUk9SX0NPREVfUExBTl9O'
    'T1RfRE9XTkxPQURFRBAOEhoKFkVSUk9SX0NPREVfU0tJUF9ERU5JRUQQDxIeChpFUlJPUl9DT0'
    'RFX1BST0ZJTEVfSU5WQUxJRBAQEiIKHkVSUk9SX0NPREVfQ0hBTk5FTF9OT1RfQUxMT1dFRBAR'
    'EhcKE0VSUk9SX0NPREVfQ0FOQ0VMRUQQEhIgChxFUlJPUl9DT0RFX1NJREVDQVJfTk9UX0ZPVU'
    '5EEBMSIQodRVJST1JfQ09ERV9MQVlPVVRfVU5TVVBQT1JURUQQFA==');

@$core.Deprecated('Use lastResultDescriptor instead')
const LastResult$json = {
  '1': 'LastResult',
  '2': [
    {'1': 'LAST_RESULT_UNSPECIFIED', '2': 0},
    {'1': 'LAST_RESULT_UP_TO_DATE', '2': 1},
    {'1': 'LAST_RESULT_UPDATE_AVAILABLE', '2': 2},
    {'1': 'LAST_RESULT_FALLBACK_REQUIRED', '2': 3},
    {'1': 'LAST_RESULT_THROTTLED', '2': 4},
    {'1': 'LAST_RESULT_FAILED', '2': 5},
    {'1': 'LAST_RESULT_APPLIED', '2': 6},
  ],
};

/// Descriptor for `LastResult`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List lastResultDescriptor = $convert.base64Decode(
    'CgpMYXN0UmVzdWx0EhsKF0xBU1RfUkVTVUxUX1VOU1BFQ0lGSUVEEAASGgoWTEFTVF9SRVNVTF'
    'RfVVBfVE9fREFURRABEiAKHExBU1RfUkVTVUxUX1VQREFURV9BVkFJTEFCTEUQAhIhCh1MQVNU'
    'X1JFU1VMVF9GQUxMQkFDS19SRVFVSVJFRBADEhkKFUxBU1RfUkVTVUxUX1RIUk9UVExFRBAEEh'
    'YKEkxBU1RfUkVTVUxUX0ZBSUxFRBAFEhcKE0xBU1RfUkVTVUxUX0FQUExJRUQQBg==');

@$core.Deprecated('Use layoutDescriptor instead')
const Layout$json = {
  '1': 'Layout',
  '2': [
    {'1': 'LAYOUT_UNSPECIFIED', '2': 0},
    {'1': 'LAYOUT_WHOLE_ROOT', '2': 1},
    {'1': 'LAYOUT_VERSIONED_DIR', '2': 2},
    {'1': 'LAYOUT_FILE_SET', '2': 3},
  ],
};

/// Descriptor for `Layout`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List layoutDescriptor = $convert.base64Decode(
    'CgZMYXlvdXQSFgoSTEFZT1VUX1VOU1BFQ0lGSUVEEAASFQoRTEFZT1VUX1dIT0xFX1JPT1QQAR'
    'IYChRMQVlPVVRfVkVSU0lPTkVEX0RJUhACEhMKD0xBWU9VVF9GSUxFX1NFVBAD');

@$core.Deprecated('Use operationDescriptor instead')
const Operation$json = {
  '1': 'Operation',
  '2': [
    {'1': 'OPERATION_UNSPECIFIED', '2': 0},
    {'1': 'OPERATION_CHECK', '2': 1},
    {'1': 'OPERATION_SKIP', '2': 2},
    {'1': 'OPERATION_DOWNLOAD', '2': 3},
    {'1': 'OPERATION_APPLY', '2': 4},
    {'1': 'OPERATION_STATUS', '2': 5},
    {'1': 'OPERATION_CLEANUP', '2': 6},
    {'1': 'OPERATION_CANCEL', '2': 7},
    {'1': 'OPERATION_SCHEDULER', '2': 8},
  ],
};

/// Descriptor for `Operation`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List operationDescriptor = $convert.base64Decode(
    'CglPcGVyYXRpb24SGQoVT1BFUkFUSU9OX1VOU1BFQ0lGSUVEEAASEwoPT1BFUkFUSU9OX0NIRU'
    'NLEAESEgoOT1BFUkFUSU9OX1NLSVAQAhIWChJPUEVSQVRJT05fRE9XTkxPQUQQAxITCg9PUEVS'
    'QVRJT05fQVBQTFkQBBIUChBPUEVSQVRJT05fU1RBVFVTEAUSFQoRT1BFUkFUSU9OX0NMRUFOVV'
    'AQBhIUChBPUEVSQVRJT05fQ0FOQ0VMEAcSFwoTT1BFUkFUSU9OX1NDSEVEVUxFUhAI');

@$core.Deprecated('Use sessionPhaseDescriptor instead')
const SessionPhase$json = {
  '1': 'SessionPhase',
  '2': [
    {'1': 'SESSION_PHASE_UNSPECIFIED', '2': 0},
    {'1': 'SESSION_PHASE_WAITING_FOR_EXIT', '2': 1},
    {'1': 'SESSION_PHASE_COPYING', '2': 2},
    {'1': 'SESSION_PHASE_COMMITTING', '2': 3},
    {'1': 'SESSION_PHASE_RELAUNCHING', '2': 4},
    {'1': 'SESSION_PHASE_NEEDS_ATTENTION', '2': 5},
    {'1': 'SESSION_PHASE_COMPLETED', '2': 6},
    {'1': 'SESSION_PHASE_ROLLED_BACK', '2': 7},
  ],
};

/// Descriptor for `SessionPhase`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List sessionPhaseDescriptor = $convert.base64Decode(
    'CgxTZXNzaW9uUGhhc2USHQoZU0VTU0lPTl9QSEFTRV9VTlNQRUNJRklFRBAAEiIKHlNFU1NJT0'
    '5fUEhBU0VfV0FJVElOR19GT1JfRVhJVBABEhkKFVNFU1NJT05fUEhBU0VfQ09QWUlORxACEhwK'
    'GFNFU1NJT05fUEhBU0VfQ09NTUlUVElORxADEh0KGVNFU1NJT05fUEhBU0VfUkVMQVVOQ0hJTk'
    'cQBBIhCh1TRVNTSU9OX1BIQVNFX05FRURTX0FUVEVOVElPThAFEhsKF1NFU1NJT05fUEhBU0Vf'
    'Q09NUExFVEVEEAYSHQoZU0VTU0lPTl9QSEFTRV9ST0xMRURfQkFDSxAH');

@$core.Deprecated('Use trustedKeyDescriptor instead')
const TrustedKey$json = {
  '1': 'TrustedKey',
  '2': [
    {'1': 'key_id', '3': 1, '4': 1, '5': 9, '10': 'keyId'},
    {'1': 'public_key', '3': 2, '4': 1, '5': 12, '10': 'publicKey'},
  ],
};

/// Descriptor for `TrustedKey`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List trustedKeyDescriptor = $convert.base64Decode(
    'CgpUcnVzdGVkS2V5EhUKBmtleV9pZBgBIAEoCVIFa2V5SWQSHQoKcHVibGljX2tleRgCIAEoDF'
    'IJcHVibGljS2V5');

@$core.Deprecated('Use recoveryLinkDescriptor instead')
const RecoveryLink$json = {
  '1': 'RecoveryLink',
  '2': [
    {'1': 'label', '3': 1, '4': 1, '5': 9, '10': 'label'},
    {'1': 'url', '3': 2, '4': 1, '5': 9, '10': 'url'},
  ],
};

/// Descriptor for `RecoveryLink`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List recoveryLinkDescriptor = $convert.base64Decode(
    'CgxSZWNvdmVyeUxpbmsSFAoFbGFiZWwYASABKAlSBWxhYmVsEhAKA3VybBgCIAEoCVIDdXJs');

@$core.Deprecated('Use recoveryHelpDescriptor instead')
const RecoveryHelp$json = {
  '1': 'RecoveryHelp',
  '2': [
    {'1': 'message', '3': 1, '4': 1, '5': 9, '10': 'message'},
    {
      '1': 'links',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.RecoveryLink',
      '10': 'links'
    },
  ],
};

/// Descriptor for `RecoveryHelp`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List recoveryHelpDescriptor = $convert.base64Decode(
    'CgxSZWNvdmVyeUhlbHASGAoHbWVzc2FnZRgBIAEoCVIHbWVzc2FnZRI1CgVsaW5rcxgCIAMoCz'
    'IfLnJlbGtpdC51cGRhdGVyLnYxLlJlY292ZXJ5TGlua1IFbGlua3M=');

@$core.Deprecated('Use clientProfileDescriptor instead')
const ClientProfile$json = {
  '1': 'ClientProfile',
  '2': [
    {'1': 'product', '3': 1, '4': 1, '5': 9, '10': 'product'},
    {'1': 'allowed_channels', '3': 2, '4': 3, '5': 9, '10': 'allowedChannels'},
    {'1': 'entry_urls', '3': 3, '4': 3, '5': 9, '10': 'entryUrls'},
    {'1': 'index_urls', '3': 4, '4': 3, '5': 9, '10': 'indexUrls'},
    {'1': 'fallback_urls', '3': 5, '4': 3, '5': 9, '10': 'fallbackUrls'},
    {
      '1': 'trusted_keys',
      '3': 6,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.TrustedKey',
      '10': 'trustedKeys'
    },
    {
      '1': 'recovery',
      '3': 7,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.RecoveryHelp',
      '10': 'recovery'
    },
  ],
};

/// Descriptor for `ClientProfile`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientProfileDescriptor = $convert.base64Decode(
    'Cg1DbGllbnRQcm9maWxlEhgKB3Byb2R1Y3QYASABKAlSB3Byb2R1Y3QSKQoQYWxsb3dlZF9jaG'
    'FubmVscxgCIAMoCVIPYWxsb3dlZENoYW5uZWxzEh0KCmVudHJ5X3VybHMYAyADKAlSCWVudHJ5'
    'VXJscxIdCgppbmRleF91cmxzGAQgAygJUglpbmRleFVybHMSIwoNZmFsbGJhY2tfdXJscxgFIA'
    'MoCVIMZmFsbGJhY2tVcmxzEkAKDHRydXN0ZWRfa2V5cxgGIAMoCzIdLnJlbGtpdC51cGRhdGVy'
    'LnYxLlRydXN0ZWRLZXlSC3RydXN0ZWRLZXlzEjsKCHJlY292ZXJ5GAcgASgLMh8ucmVsa2l0Ln'
    'VwZGF0ZXIudjEuUmVjb3ZlcnlIZWxwUghyZWNvdmVyeQ==');

@$core.Deprecated('Use fileSetEntryDescriptor instead')
const FileSetEntry$json = {
  '1': 'FileSetEntry',
  '2': [
    {'1': 'dest_relpath', '3': 1, '4': 1, '5': 9, '10': 'destRelpath'},
    {'1': 'artifact_name', '3': 2, '4': 1, '5': 9, '10': 'artifactName'},
  ],
};

/// Descriptor for `FileSetEntry`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List fileSetEntryDescriptor = $convert.base64Decode(
    'CgxGaWxlU2V0RW50cnkSIQoMZGVzdF9yZWxwYXRoGAEgASgJUgtkZXN0UmVscGF0aBIjCg1hcn'
    'RpZmFjdF9uYW1lGAIgASgJUgxhcnRpZmFjdE5hbWU=');

@$core.Deprecated('Use installSpecDescriptor instead')
const InstallSpec$json = {
  '1': 'InstallSpec',
  '2': [
    {
      '1': 'layout',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.relkit.updater.v1.Layout',
      '10': 'layout'
    },
    {'1': 'install_root', '3': 2, '4': 1, '5': 9, '10': 'installRoot'},
    {
      '1': 'executable_relpath',
      '3': 3,
      '4': 1,
      '5': 9,
      '10': 'executableRelpath'
    },
    {'1': 'sidecar_relpath', '3': 4, '4': 1, '5': 9, '10': 'sidecarRelpath'},
    {'1': 'preserve', '3': 5, '4': 3, '5': 9, '10': 'preserve'},
    {'1': 'retain', '3': 6, '4': 1, '5': 5, '10': 'retain'},
    {'1': 'relaunch', '3': 7, '4': 1, '5': 8, '10': 'relaunch'},
    {
      '1': 'file_set',
      '3': 8,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.FileSetEntry',
      '10': 'fileSet'
    },
  ],
};

/// Descriptor for `InstallSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List installSpecDescriptor = $convert.base64Decode(
    'CgtJbnN0YWxsU3BlYxIxCgZsYXlvdXQYASABKA4yGS5yZWxraXQudXBkYXRlci52MS5MYXlvdX'
    'RSBmxheW91dBIhCgxpbnN0YWxsX3Jvb3QYAiABKAlSC2luc3RhbGxSb290Ei0KEmV4ZWN1dGFi'
    'bGVfcmVscGF0aBgDIAEoCVIRZXhlY3V0YWJsZVJlbHBhdGgSJwoPc2lkZWNhcl9yZWxwYXRoGA'
    'QgASgJUg5zaWRlY2FyUmVscGF0aBIaCghwcmVzZXJ2ZRgFIAMoCVIIcHJlc2VydmUSFgoGcmV0'
    'YWluGAYgASgFUgZyZXRhaW4SGgoIcmVsYXVuY2gYByABKAhSCHJlbGF1bmNoEjoKCGZpbGVfc2'
    'V0GAggAygLMh8ucmVsa2l0LnVwZGF0ZXIudjEuRmlsZVNldEVudHJ5UgdmaWxlU2V0');

@$core.Deprecated('Use runtimeDescriptor instead')
const Runtime$json = {
  '1': 'Runtime',
  '2': [
    {'1': 'channel', '3': 1, '4': 1, '5': 9, '10': 'channel'},
    {'1': 'current_code', '3': 2, '4': 1, '5': 3, '10': 'currentCode'},
    {
      '1': 'client_selectors',
      '3': 3,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.Runtime.ClientSelectorsEntry',
      '10': 'clientSelectors'
    },
    {'1': 'data_dir', '3': 4, '4': 1, '5': 9, '10': 'dataDir'},
    {
      '1': 'install',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.InstallSpec',
      '10': 'install'
    },
    {'1': 'sidecar_path', '3': 6, '4': 1, '5': 9, '10': 'sidecarPath'},
  ],
  '3': [Runtime_ClientSelectorsEntry$json],
};

@$core.Deprecated('Use runtimeDescriptor instead')
const Runtime_ClientSelectorsEntry$json = {
  '1': 'ClientSelectorsEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 9, '10': 'value'},
  ],
  '7': {'7': true},
};

/// Descriptor for `Runtime`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List runtimeDescriptor = $convert.base64Decode(
    'CgdSdW50aW1lEhgKB2NoYW5uZWwYASABKAlSB2NoYW5uZWwSIQoMY3VycmVudF9jb2RlGAIgAS'
    'gDUgtjdXJyZW50Q29kZRJaChBjbGllbnRfc2VsZWN0b3JzGAMgAygLMi8ucmVsa2l0LnVwZGF0'
    'ZXIudjEuUnVudGltZS5DbGllbnRTZWxlY3RvcnNFbnRyeVIPY2xpZW50U2VsZWN0b3JzEhkKCG'
    'RhdGFfZGlyGAQgASgJUgdkYXRhRGlyEjgKB2luc3RhbGwYBSABKAsyHi5yZWxraXQudXBkYXRl'
    'ci52MS5JbnN0YWxsU3BlY1IHaW5zdGFsbBIhCgxzaWRlY2FyX3BhdGgYBiABKAlSC3NpZGVjYX'
    'JQYXRoGkIKFENsaWVudFNlbGVjdG9yc0VudHJ5EhAKA2tleRgBIAEoCVIDa2V5EhQKBXZhbHVl'
    'GAIgASgJUgV2YWx1ZToCOAE=');

@$core.Deprecated('Use checkPolicyDescriptor instead')
const CheckPolicy$json = {
  '1': 'CheckPolicy',
  '2': [
    {
      '1': 'after_success',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Duration',
      '10': 'afterSuccess'
    },
    {
      '1': 'after_failure',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Duration',
      '10': 'afterFailure'
    },
  ],
};

/// Descriptor for `CheckPolicy`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List checkPolicyDescriptor = $convert.base64Decode(
    'CgtDaGVja1BvbGljeRI+Cg1hZnRlcl9zdWNjZXNzGAEgASgLMhkuZ29vZ2xlLnByb3RvYnVmLk'
    'R1cmF0aW9uUgxhZnRlclN1Y2Nlc3MSPgoNYWZ0ZXJfZmFpbHVyZRgCIAEoCzIZLmdvb2dsZS5w'
    'cm90b2J1Zi5EdXJhdGlvblIMYWZ0ZXJGYWlsdXJl');

@$core.Deprecated('Use schedulerConfigDescriptor instead')
const SchedulerConfig$json = {
  '1': 'SchedulerConfig',
  '2': [
    {'1': 'check_on_start', '3': 1, '4': 1, '5': 8, '10': 'checkOnStart'},
    {'1': 'force_on_start', '3': 2, '4': 1, '5': 8, '10': 'forceOnStart'},
    {
      '1': 'policy',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.CheckPolicy',
      '10': 'policy'
    },
  ],
};

/// Descriptor for `SchedulerConfig`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List schedulerConfigDescriptor = $convert.base64Decode(
    'Cg9TY2hlZHVsZXJDb25maWcSJAoOY2hlY2tfb25fc3RhcnQYASABKAhSDGNoZWNrT25TdGFydB'
    'IkCg5mb3JjZV9vbl9zdGFydBgCIAEoCFIMZm9yY2VPblN0YXJ0EjYKBnBvbGljeRgDIAEoCzIe'
    'LnJlbGtpdC51cGRhdGVyLnYxLkNoZWNrUG9saWN5UgZwb2xpY3k=');

@$core.Deprecated('Use clientHelloDescriptor instead')
const ClientHello$json = {
  '1': 'ClientHello',
  '2': [
    {'1': 'ipc_min', '3': 1, '4': 1, '5': 13, '10': 'ipcMin'},
    {'1': 'ipc_max', '3': 2, '4': 1, '5': 13, '10': 'ipcMax'},
  ],
};

/// Descriptor for `ClientHello`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientHelloDescriptor = $convert.base64Decode(
    'CgtDbGllbnRIZWxsbxIXCgdpcGNfbWluGAEgASgNUgZpcGNNaW4SFwoHaXBjX21heBgCIAEoDV'
    'IGaXBjTWF4');

@$core.Deprecated('Use capabilitiesDescriptor instead')
const Capabilities$json = {
  '1': 'Capabilities',
  '2': [
    {'1': 'ipc', '3': 1, '4': 1, '5': 13, '10': 'ipc'},
    {
      '1': 'operations',
      '3': 2,
      '4': 3,
      '5': 14,
      '6': '.relkit.updater.v1.Operation',
      '10': 'operations'
    },
    {
      '1': 'layouts',
      '3': 3,
      '4': 3,
      '5': 14,
      '6': '.relkit.updater.v1.Layout',
      '10': 'layouts'
    },
    {
      '1': 'min_check_interval',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Duration',
      '10': 'minCheckInterval'
    },
    {
      '1': 'plan_ttl',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Duration',
      '10': 'planTtl'
    },
    {'1': 'engine_version', '3': 6, '4': 1, '5': 9, '10': 'engineVersion'},
  ],
};

/// Descriptor for `Capabilities`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List capabilitiesDescriptor = $convert.base64Decode(
    'CgxDYXBhYmlsaXRpZXMSEAoDaXBjGAEgASgNUgNpcGMSPAoKb3BlcmF0aW9ucxgCIAMoDjIcLn'
    'JlbGtpdC51cGRhdGVyLnYxLk9wZXJhdGlvblIKb3BlcmF0aW9ucxIzCgdsYXlvdXRzGAMgAygO'
    'MhkucmVsa2l0LnVwZGF0ZXIudjEuTGF5b3V0UgdsYXlvdXRzEkcKEm1pbl9jaGVja19pbnRlcn'
    'ZhbBgEIAEoCzIZLmdvb2dsZS5wcm90b2J1Zi5EdXJhdGlvblIQbWluQ2hlY2tJbnRlcnZhbBI0'
    'CghwbGFuX3R0bBgFIAEoCzIZLmdvb2dsZS5wcm90b2J1Zi5EdXJhdGlvblIHcGxhblR0bBIlCg'
    '5lbmdpbmVfdmVyc2lvbhgGIAEoCVINZW5naW5lVmVyc2lvbg==');

@$core.Deprecated('Use checkOpDescriptor instead')
const CheckOp$json = {
  '1': 'CheckOp',
  '2': [
    {'1': 'force', '3': 1, '4': 1, '5': 8, '10': 'force'},
    {'1': 'exact_code', '3': 2, '4': 1, '5': 3, '10': 'exactCode'},
    {
      '1': 'policy',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.CheckPolicy',
      '10': 'policy'
    },
  ],
};

/// Descriptor for `CheckOp`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List checkOpDescriptor = $convert.base64Decode(
    'CgdDaGVja09wEhQKBWZvcmNlGAEgASgIUgVmb3JjZRIdCgpleGFjdF9jb2RlGAIgASgDUglleG'
    'FjdENvZGUSNgoGcG9saWN5GAMgASgLMh4ucmVsa2l0LnVwZGF0ZXIudjEuQ2hlY2tQb2xpY3lS'
    'BnBvbGljeQ==');

@$core.Deprecated('Use skipOpDescriptor instead')
const SkipOp$json = {
  '1': 'SkipOp',
  '2': [
    {'1': 'code', '3': 1, '4': 1, '5': 3, '10': 'code'},
  ],
};

/// Descriptor for `SkipOp`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List skipOpDescriptor =
    $convert.base64Decode('CgZTa2lwT3ASEgoEY29kZRgBIAEoA1IEY29kZQ==');

@$core.Deprecated('Use downloadOpDescriptor instead')
const DownloadOp$json = {
  '1': 'DownloadOp',
  '2': [
    {'1': 'plan_id', '3': 1, '4': 1, '5': 9, '10': 'planId'},
  ],
};

/// Descriptor for `DownloadOp`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List downloadOpDescriptor = $convert
    .base64Decode('CgpEb3dubG9hZE9wEhcKB3BsYW5faWQYASABKAlSBnBsYW5JZA==');

@$core.Deprecated('Use applyOpDescriptor instead')
const ApplyOp$json = {
  '1': 'ApplyOp',
  '2': [
    {'1': 'plan_id', '3': 1, '4': 1, '5': 9, '10': 'planId'},
  ],
};

/// Descriptor for `ApplyOp`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List applyOpDescriptor =
    $convert.base64Decode('CgdBcHBseU9wEhcKB3BsYW5faWQYASABKAlSBnBsYW5JZA==');

@$core.Deprecated('Use statusOpDescriptor instead')
const StatusOp$json = {
  '1': 'StatusOp',
};

/// Descriptor for `StatusOp`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List statusOpDescriptor =
    $convert.base64Decode('CghTdGF0dXNPcA==');

@$core.Deprecated('Use cleanupOpDescriptor instead')
const CleanupOp$json = {
  '1': 'CleanupOp',
};

/// Descriptor for `CleanupOp`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List cleanupOpDescriptor =
    $convert.base64Decode('CglDbGVhbnVwT3A=');

@$core.Deprecated('Use cancelOpDescriptor instead')
const CancelOp$json = {
  '1': 'CancelOp',
};

/// Descriptor for `CancelOp`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List cancelOpDescriptor =
    $convert.base64Decode('CghDYW5jZWxPcA==');

@$core.Deprecated('Use updaterRequestDescriptor instead')
const UpdaterRequest$json = {
  '1': 'UpdaterRequest',
  '2': [
    {
      '1': 'hello',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.ClientHello',
      '10': 'hello'
    },
    {
      '1': 'profile',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.ClientProfile',
      '10': 'profile'
    },
    {
      '1': 'runtime',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Runtime',
      '10': 'runtime'
    },
    {
      '1': 'check',
      '3': 10,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.CheckOp',
      '9': 0,
      '10': 'check'
    },
    {
      '1': 'skip',
      '3': 11,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.SkipOp',
      '9': 0,
      '10': 'skip'
    },
    {
      '1': 'download',
      '3': 12,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.DownloadOp',
      '9': 0,
      '10': 'download'
    },
    {
      '1': 'apply',
      '3': 13,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.ApplyOp',
      '9': 0,
      '10': 'apply'
    },
    {
      '1': 'status',
      '3': 14,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.StatusOp',
      '9': 0,
      '10': 'status'
    },
    {
      '1': 'cleanup',
      '3': 15,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.CleanupOp',
      '9': 0,
      '10': 'cleanup'
    },
    {
      '1': 'cancel',
      '3': 16,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.CancelOp',
      '9': 0,
      '10': 'cancel'
    },
  ],
  '8': [
    {'1': 'op'},
  ],
};

/// Descriptor for `UpdaterRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List updaterRequestDescriptor = $convert.base64Decode(
    'Cg5VcGRhdGVyUmVxdWVzdBI0CgVoZWxsbxgBIAEoCzIeLnJlbGtpdC51cGRhdGVyLnYxLkNsaW'
    'VudEhlbGxvUgVoZWxsbxI6Cgdwcm9maWxlGAIgASgLMiAucmVsa2l0LnVwZGF0ZXIudjEuQ2xp'
    'ZW50UHJvZmlsZVIHcHJvZmlsZRI0CgdydW50aW1lGAMgASgLMhoucmVsa2l0LnVwZGF0ZXIudj'
    'EuUnVudGltZVIHcnVudGltZRIyCgVjaGVjaxgKIAEoCzIaLnJlbGtpdC51cGRhdGVyLnYxLkNo'
    'ZWNrT3BIAFIFY2hlY2sSLwoEc2tpcBgLIAEoCzIZLnJlbGtpdC51cGRhdGVyLnYxLlNraXBPcE'
    'gAUgRza2lwEjsKCGRvd25sb2FkGAwgASgLMh0ucmVsa2l0LnVwZGF0ZXIudjEuRG93bmxvYWRP'
    'cEgAUghkb3dubG9hZBIyCgVhcHBseRgNIAEoCzIaLnJlbGtpdC51cGRhdGVyLnYxLkFwcGx5T3'
    'BIAFIFYXBwbHkSNQoGc3RhdHVzGA4gASgLMhsucmVsa2l0LnVwZGF0ZXIudjEuU3RhdHVzT3BI'
    'AFIGc3RhdHVzEjgKB2NsZWFudXAYDyABKAsyHC5yZWxraXQudXBkYXRlci52MS5DbGVhbnVwT3'
    'BIAFIHY2xlYW51cBI1CgZjYW5jZWwYECABKAsyGy5yZWxraXQudXBkYXRlci52MS5DYW5jZWxP'
    'cEgAUgZjYW5jZWxCBAoCb3A=');

@$core.Deprecated('Use errorDescriptor instead')
const Error$json = {
  '1': 'Error',
  '2': [
    {
      '1': 'code',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.relkit.updater.v1.ErrorCode',
      '10': 'code'
    },
    {'1': 'retryable', '3': 2, '4': 1, '5': 8, '10': 'retryable'},
    {'1': 'message', '3': 3, '4': 1, '5': 9, '10': 'message'},
    {'1': 'attempts', '3': 4, '4': 3, '5': 9, '10': 'attempts'},
    {
      '1': 'recovery',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.RecoveryHelp',
      '10': 'recovery'
    },
  ],
};

/// Descriptor for `Error`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List errorDescriptor = $convert.base64Decode(
    'CgVFcnJvchIwCgRjb2RlGAEgASgOMhwucmVsa2l0LnVwZGF0ZXIudjEuRXJyb3JDb2RlUgRjb2'
    'RlEhwKCXJldHJ5YWJsZRgCIAEoCFIJcmV0cnlhYmxlEhgKB21lc3NhZ2UYAyABKAlSB21lc3Nh'
    'Z2USGgoIYXR0ZW1wdHMYBCADKAlSCGF0dGVtcHRzEjsKCHJlY292ZXJ5GAUgASgLMh8ucmVsa2'
    'l0LnVwZGF0ZXIudjEuUmVjb3ZlcnlIZWxwUghyZWNvdmVyeQ==');

@$core.Deprecated('Use priorReleaseNotesDescriptor instead')
const PriorReleaseNotes$json = {
  '1': 'PriorReleaseNotes',
  '2': [
    {'1': 'version', '3': 1, '4': 1, '5': 9, '10': 'version'},
    {'1': 'code', '3': 2, '4': 1, '5': 3, '10': 'code'},
    {'1': 'notes', '3': 3, '4': 1, '5': 9, '10': 'notes'},
    {'1': 'notes_url', '3': 4, '4': 1, '5': 9, '10': 'notesUrl'},
  ],
};

/// Descriptor for `PriorReleaseNotes`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List priorReleaseNotesDescriptor = $convert.base64Decode(
    'ChFQcmlvclJlbGVhc2VOb3RlcxIYCgd2ZXJzaW9uGAEgASgJUgd2ZXJzaW9uEhIKBGNvZGUYAi'
    'ABKANSBGNvZGUSFAoFbm90ZXMYAyABKAlSBW5vdGVzEhsKCW5vdGVzX3VybBgEIAEoCVIIbm90'
    'ZXNVcmw=');

@$core.Deprecated('Use artifactViewDescriptor instead')
const ArtifactView$json = {
  '1': 'ArtifactView',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
    {'1': 'size', '3': 2, '4': 1, '5': 3, '10': 'size'},
    {'1': 'sha256', '3': 3, '4': 1, '5': 12, '10': 'sha256'},
  ],
};

/// Descriptor for `ArtifactView`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List artifactViewDescriptor = $convert.base64Decode(
    'CgxBcnRpZmFjdFZpZXcSEgoEbmFtZRgBIAEoCVIEbmFtZRISCgRzaXplGAIgASgDUgRzaXplEh'
    'YKBnNoYTI1NhgDIAEoDFIGc2hhMjU2');

@$core.Deprecated('Use upToDateDescriptor instead')
const UpToDate$json = {
  '1': 'UpToDate',
  '2': [
    {'1': 'sequence', '3': 1, '4': 1, '5': 3, '10': 'sequence'},
    {'1': 'current_is_yanked', '3': 2, '4': 1, '5': 8, '10': 'currentIsYanked'},
  ],
};

/// Descriptor for `UpToDate`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List upToDateDescriptor = $convert.base64Decode(
    'CghVcFRvRGF0ZRIaCghzZXF1ZW5jZRgBIAEoA1IIc2VxdWVuY2USKgoRY3VycmVudF9pc195YW'
    '5rZWQYAiABKAhSD2N1cnJlbnRJc1lhbmtlZA==');

@$core.Deprecated('Use updateAvailableDescriptor instead')
const UpdateAvailable$json = {
  '1': 'UpdateAvailable',
  '2': [
    {'1': 'plan_id', '3': 1, '4': 1, '5': 9, '10': 'planId'},
    {'1': 'prompt_key', '3': 2, '4': 1, '5': 9, '10': 'promptKey'},
    {'1': 'version', '3': 3, '4': 1, '5': 9, '10': 'version'},
    {'1': 'code', '3': 4, '4': 1, '5': 3, '10': 'code'},
    {'1': 'mandatory', '3': 5, '4': 1, '5': 8, '10': 'mandatory'},
    {'1': 'remaining_hops', '3': 6, '4': 1, '5': 5, '10': 'remainingHops'},
    {'1': 'sequence', '3': 7, '4': 1, '5': 3, '10': 'sequence'},
    {
      '1': 'release_notes_markdown',
      '3': 8,
      '4': 1,
      '5': 9,
      '10': 'releaseNotesMarkdown'
    },
    {'1': 'release_notes_url', '3': 9, '4': 1, '5': 9, '10': 'releaseNotesUrl'},
    {
      '1': 'prior_release_notes',
      '3': 10,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.PriorReleaseNotes',
      '10': 'priorReleaseNotes'
    },
    {
      '1': 'artifacts',
      '3': 11,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.ArtifactView',
      '10': 'artifacts'
    },
  ],
};

/// Descriptor for `UpdateAvailable`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List updateAvailableDescriptor = $convert.base64Decode(
    'Cg9VcGRhdGVBdmFpbGFibGUSFwoHcGxhbl9pZBgBIAEoCVIGcGxhbklkEh0KCnByb21wdF9rZX'
    'kYAiABKAlSCXByb21wdEtleRIYCgd2ZXJzaW9uGAMgASgJUgd2ZXJzaW9uEhIKBGNvZGUYBCAB'
    'KANSBGNvZGUSHAoJbWFuZGF0b3J5GAUgASgIUgltYW5kYXRvcnkSJQoOcmVtYWluaW5nX2hvcH'
    'MYBiABKAVSDXJlbWFpbmluZ0hvcHMSGgoIc2VxdWVuY2UYByABKANSCHNlcXVlbmNlEjQKFnJl'
    'bGVhc2Vfbm90ZXNfbWFya2Rvd24YCCABKAlSFHJlbGVhc2VOb3Rlc01hcmtkb3duEioKEXJlbG'
    'Vhc2Vfbm90ZXNfdXJsGAkgASgJUg9yZWxlYXNlTm90ZXNVcmwSVAoTcHJpb3JfcmVsZWFzZV9u'
    'b3RlcxgKIAMoCzIkLnJlbGtpdC51cGRhdGVyLnYxLlByaW9yUmVsZWFzZU5vdGVzUhFwcmlvcl'
    'JlbGVhc2VOb3RlcxI9CglhcnRpZmFjdHMYCyADKAsyHy5yZWxraXQudXBkYXRlci52MS5BcnRp'
    'ZmFjdFZpZXdSCWFydGlmYWN0cw==');

@$core.Deprecated('Use fallbackRequiredDescriptor instead')
const FallbackRequired$json = {
  '1': 'FallbackRequired',
  '2': [
    {'1': 'prompt_key', '3': 1, '4': 1, '5': 9, '10': 'promptKey'},
    {'1': 'manual_url', '3': 2, '4': 1, '5': 9, '10': 'manualUrl'},
    {'1': 'message', '3': 3, '4': 1, '5': 9, '10': 'message'},
    {'1': 'mandatory', '3': 4, '4': 1, '5': 8, '10': 'mandatory'},
    {'1': 'sequence', '3': 5, '4': 1, '5': 3, '10': 'sequence'},
    {'1': 'min_code', '3': 6, '4': 1, '5': 3, '10': 'minCode'},
    {'1': 'max_code', '3': 7, '4': 1, '5': 3, '10': 'maxCode'},
  ],
};

/// Descriptor for `FallbackRequired`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List fallbackRequiredDescriptor = $convert.base64Decode(
    'ChBGYWxsYmFja1JlcXVpcmVkEh0KCnByb21wdF9rZXkYASABKAlSCXByb21wdEtleRIdCgptYW'
    '51YWxfdXJsGAIgASgJUgltYW51YWxVcmwSGAoHbWVzc2FnZRgDIAEoCVIHbWVzc2FnZRIcCglt'
    'YW5kYXRvcnkYBCABKAhSCW1hbmRhdG9yeRIaCghzZXF1ZW5jZRgFIAEoA1IIc2VxdWVuY2USGQ'
    'oIbWluX2NvZGUYBiABKANSB21pbkNvZGUSGQoIbWF4X2NvZGUYByABKANSB21heENvZGU=');

@$core.Deprecated('Use throttledDescriptor instead')
const Throttled$json = {
  '1': 'Throttled',
  '2': [
    {
      '1': 'next_allowed_at',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Timestamp',
      '10': 'nextAllowedAt'
    },
  ],
};

/// Descriptor for `Throttled`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List throttledDescriptor = $convert.base64Decode(
    'CglUaHJvdHRsZWQSQgoPbmV4dF9hbGxvd2VkX2F0GAEgASgLMhouZ29vZ2xlLnByb3RvYnVmLl'
    'RpbWVzdGFtcFINbmV4dEFsbG93ZWRBdA==');

@$core.Deprecated('Use failedDescriptor instead')
const Failed$json = {
  '1': 'Failed',
  '2': [
    {
      '1': 'error',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Error',
      '10': 'error'
    },
  ],
};

/// Descriptor for `Failed`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List failedDescriptor = $convert.base64Decode(
    'CgZGYWlsZWQSLgoFZXJyb3IYASABKAsyGC5yZWxraXQudXBkYXRlci52MS5FcnJvclIFZXJyb3'
    'I=');

@$core.Deprecated('Use checkResultDescriptor instead')
const CheckResult$json = {
  '1': 'CheckResult',
  '2': [
    {
      '1': 'up_to_date',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.UpToDate',
      '9': 0,
      '10': 'upToDate'
    },
    {
      '1': 'update_available',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.UpdateAvailable',
      '9': 0,
      '10': 'updateAvailable'
    },
    {
      '1': 'fallback_required',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.FallbackRequired',
      '9': 0,
      '10': 'fallbackRequired'
    },
    {
      '1': 'throttled',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Throttled',
      '9': 0,
      '10': 'throttled'
    },
    {
      '1': 'failed',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Failed',
      '9': 0,
      '10': 'failed'
    },
  ],
  '8': [
    {'1': 'kind'},
  ],
};

/// Descriptor for `CheckResult`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List checkResultDescriptor = $convert.base64Decode(
    'CgtDaGVja1Jlc3VsdBI7Cgp1cF90b19kYXRlGAEgASgLMhsucmVsa2l0LnVwZGF0ZXIudjEuVX'
    'BUb0RhdGVIAFIIdXBUb0RhdGUSTwoQdXBkYXRlX2F2YWlsYWJsZRgCIAEoCzIiLnJlbGtpdC51'
    'cGRhdGVyLnYxLlVwZGF0ZUF2YWlsYWJsZUgAUg91cGRhdGVBdmFpbGFibGUSUgoRZmFsbGJhY2'
    'tfcmVxdWlyZWQYAyABKAsyIy5yZWxraXQudXBkYXRlci52MS5GYWxsYmFja1JlcXVpcmVkSABS'
    'EGZhbGxiYWNrUmVxdWlyZWQSPAoJdGhyb3R0bGVkGAQgASgLMhwucmVsa2l0LnVwZGF0ZXIudj'
    'EuVGhyb3R0bGVkSABSCXRocm90dGxlZBIzCgZmYWlsZWQYBSABKAsyGS5yZWxraXQudXBkYXRl'
    'ci52MS5GYWlsZWRIAFIGZmFpbGVkQgYKBGtpbmQ=');

@$core.Deprecated('Use downloadedDescriptor instead')
const Downloaded$json = {
  '1': 'Downloaded',
  '2': [
    {'1': 'plan_id', '3': 1, '4': 1, '5': 9, '10': 'planId'},
    {'1': 'bytes', '3': 2, '4': 1, '5': 3, '10': 'bytes'},
  ],
};

/// Descriptor for `Downloaded`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List downloadedDescriptor = $convert.base64Decode(
    'CgpEb3dubG9hZGVkEhcKB3BsYW5faWQYASABKAlSBnBsYW5JZBIUCgVieXRlcxgCIAEoA1IFYn'
    'l0ZXM=');

@$core.Deprecated('Use downloadResultDescriptor instead')
const DownloadResult$json = {
  '1': 'DownloadResult',
  '2': [
    {
      '1': 'downloaded',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Downloaded',
      '9': 0,
      '10': 'downloaded'
    },
    {
      '1': 'failed',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Failed',
      '9': 0,
      '10': 'failed'
    },
  ],
  '8': [
    {'1': 'kind'},
  ],
};

/// Descriptor for `DownloadResult`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List downloadResultDescriptor = $convert.base64Decode(
    'Cg5Eb3dubG9hZFJlc3VsdBI/Cgpkb3dubG9hZGVkGAEgASgLMh0ucmVsa2l0LnVwZGF0ZXIudj'
    'EuRG93bmxvYWRlZEgAUgpkb3dubG9hZGVkEjMKBmZhaWxlZBgCIAEoCzIZLnJlbGtpdC51cGRh'
    'dGVyLnYxLkZhaWxlZEgAUgZmYWlsZWRCBgoEa2luZA==');

@$core.Deprecated('Use applyAcceptedDescriptor instead')
const ApplyAccepted$json = {
  '1': 'ApplyAccepted',
  '2': [
    {'1': 'session_id', '3': 1, '4': 1, '5': 9, '10': 'sessionId'},
    {'1': 'plan_id', '3': 2, '4': 1, '5': 9, '10': 'planId'},
    {
      '1': 'requires_host_exit',
      '3': 3,
      '4': 1,
      '5': 8,
      '10': 'requiresHostExit'
    },
  ],
};

/// Descriptor for `ApplyAccepted`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List applyAcceptedDescriptor = $convert.base64Decode(
    'Cg1BcHBseUFjY2VwdGVkEh0KCnNlc3Npb25faWQYASABKAlSCXNlc3Npb25JZBIXCgdwbGFuX2'
    'lkGAIgASgJUgZwbGFuSWQSLAoScmVxdWlyZXNfaG9zdF9leGl0GAMgASgIUhByZXF1aXJlc0hv'
    'c3RFeGl0');

@$core.Deprecated('Use applyResultDescriptor instead')
const ApplyResult$json = {
  '1': 'ApplyResult',
  '2': [
    {
      '1': 'accepted',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.ApplyAccepted',
      '9': 0,
      '10': 'accepted'
    },
    {
      '1': 'failed',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Failed',
      '9': 0,
      '10': 'failed'
    },
  ],
  '8': [
    {'1': 'kind'},
  ],
};

/// Descriptor for `ApplyResult`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List applyResultDescriptor = $convert.base64Decode(
    'CgtBcHBseVJlc3VsdBI+CghhY2NlcHRlZBgBIAEoCzIgLnJlbGtpdC51cGRhdGVyLnYxLkFwcG'
    'x5QWNjZXB0ZWRIAFIIYWNjZXB0ZWQSMwoGZmFpbGVkGAIgASgLMhkucmVsa2l0LnVwZGF0ZXIu'
    'djEuRmFpbGVkSABSBmZhaWxlZEIGCgRraW5k');

@$core.Deprecated('Use okDescriptor instead')
const Ok$json = {
  '1': 'Ok',
};

/// Descriptor for `Ok`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List okDescriptor = $convert.base64Decode('CgJPaw==');

@$core.Deprecated('Use resultDescriptor instead')
const Result$json = {
  '1': 'Result',
  '2': [
    {
      '1': 'ok',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Ok',
      '9': 0,
      '10': 'ok'
    },
    {
      '1': 'failed',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Failed',
      '9': 0,
      '10': 'failed'
    },
  ],
  '8': [
    {'1': 'kind'},
  ],
};

/// Descriptor for `Result`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List resultDescriptor = $convert.base64Decode(
    'CgZSZXN1bHQSJwoCb2sYASABKAsyFS5yZWxraXQudXBkYXRlci52MS5Pa0gAUgJvaxIzCgZmYW'
    'lsZWQYAiABKAsyGS5yZWxraXQudXBkYXRlci52MS5GYWlsZWRIAFIGZmFpbGVkQgYKBGtpbmQ=');

@$core.Deprecated('Use sidecarInfoDescriptor instead')
const SidecarInfo$json = {
  '1': 'SidecarInfo',
  '2': [
    {'1': 'path', '3': 1, '4': 1, '5': 9, '10': 'path'},
    {'1': 'ipc', '3': 2, '4': 1, '5': 13, '10': 'ipc'},
    {'1': 'version', '3': 3, '4': 1, '5': 9, '10': 'version'},
  ],
};

/// Descriptor for `SidecarInfo`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List sidecarInfoDescriptor = $convert.base64Decode(
    'CgtTaWRlY2FySW5mbxISCgRwYXRoGAEgASgJUgRwYXRoEhAKA2lwYxgCIAEoDVIDaXBjEhgKB3'
    'ZlcnNpb24YAyABKAlSB3ZlcnNpb24=');

@$core.Deprecated('Use sessionViewDescriptor instead')
const SessionView$json = {
  '1': 'SessionView',
  '2': [
    {'1': 'session_id', '3': 1, '4': 1, '5': 9, '10': 'sessionId'},
    {'1': 'plan_id', '3': 2, '4': 1, '5': 9, '10': 'planId'},
    {
      '1': 'phase',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.relkit.updater.v1.SessionPhase',
      '10': 'phase'
    },
    {
      '1': 'started_at',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Timestamp',
      '10': 'startedAt'
    },
    {
      '1': 'error',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Error',
      '10': 'error'
    },
  ],
};

/// Descriptor for `SessionView`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List sessionViewDescriptor = $convert.base64Decode(
    'CgtTZXNzaW9uVmlldxIdCgpzZXNzaW9uX2lkGAEgASgJUglzZXNzaW9uSWQSFwoHcGxhbl9pZB'
    'gCIAEoCVIGcGxhbklkEjUKBXBoYXNlGAMgASgOMh8ucmVsa2l0LnVwZGF0ZXIudjEuU2Vzc2lv'
    'blBoYXNlUgVwaGFzZRI5CgpzdGFydGVkX2F0GAQgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbW'
    'VzdGFtcFIJc3RhcnRlZEF0Ei4KBWVycm9yGAUgASgLMhgucmVsa2l0LnVwZGF0ZXIudjEuRXJy'
    'b3JSBWVycm9y');

@$core.Deprecated('Use statusSnapshotDescriptor instead')
const StatusSnapshot$json = {
  '1': 'StatusSnapshot',
  '2': [
    {
      '1': 'last_check_at',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Timestamp',
      '10': 'lastCheckAt'
    },
    {
      '1': 'last_result',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.relkit.updater.v1.LastResult',
      '10': 'lastResult'
    },
    {
      '1': 'next_allowed_at',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Timestamp',
      '10': 'nextAllowedAt'
    },
    {
      '1': 'last_seen_sequence',
      '3': 4,
      '4': 1,
      '5': 3,
      '10': 'lastSeenSequence'
    },
    {'1': 'skipped_codes', '3': 5, '4': 3, '5': 3, '10': 'skippedCodes'},
    {
      '1': 'active_session',
      '3': 6,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.SessionView',
      '10': 'activeSession'
    },
    {
      '1': 'sidecar',
      '3': 7,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.SidecarInfo',
      '10': 'sidecar'
    },
  ],
};

/// Descriptor for `StatusSnapshot`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List statusSnapshotDescriptor = $convert.base64Decode(
    'Cg5TdGF0dXNTbmFwc2hvdBI+Cg1sYXN0X2NoZWNrX2F0GAEgASgLMhouZ29vZ2xlLnByb3RvYn'
    'VmLlRpbWVzdGFtcFILbGFzdENoZWNrQXQSPgoLbGFzdF9yZXN1bHQYAiABKA4yHS5yZWxraXQu'
    'dXBkYXRlci52MS5MYXN0UmVzdWx0UgpsYXN0UmVzdWx0EkIKD25leHRfYWxsb3dlZF9hdBgDIA'
    'EoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXBSDW5leHRBbGxvd2VkQXQSLAoSbGFzdF9z'
    'ZWVuX3NlcXVlbmNlGAQgASgDUhBsYXN0U2VlblNlcXVlbmNlEiMKDXNraXBwZWRfY29kZXMYBS'
    'ADKANSDHNraXBwZWRDb2RlcxJFCg5hY3RpdmVfc2Vzc2lvbhgGIAEoCzIeLnJlbGtpdC51cGRh'
    'dGVyLnYxLlNlc3Npb25WaWV3Ug1hY3RpdmVTZXNzaW9uEjgKB3NpZGVjYXIYByABKAsyHi5yZW'
    'xraXQudXBkYXRlci52MS5TaWRlY2FySW5mb1IHc2lkZWNhcg==');

@$core.Deprecated('Use progressDescriptor instead')
const Progress$json = {
  '1': 'Progress',
  '2': [
    {'1': 'bytes_received', '3': 1, '4': 1, '5': 3, '10': 'bytesReceived'},
    {'1': 'bytes_total', '3': 2, '4': 1, '5': 3, '10': 'bytesTotal'},
    {'1': 'bytes_per_second', '3': 3, '4': 1, '5': 3, '10': 'bytesPerSecond'},
  ],
};

/// Descriptor for `Progress`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List progressDescriptor = $convert.base64Decode(
    'CghQcm9ncmVzcxIlCg5ieXRlc19yZWNlaXZlZBgBIAEoA1INYnl0ZXNSZWNlaXZlZBIfCgtieX'
    'Rlc190b3RhbBgCIAEoA1IKYnl0ZXNUb3RhbBIoChBieXRlc19wZXJfc2Vjb25kGAMgASgDUg5i'
    'eXRlc1BlclNlY29uZA==');

@$core.Deprecated('Use applyProgressDescriptor instead')
const ApplyProgress$json = {
  '1': 'ApplyProgress',
  '2': [
    {'1': 'session_id', '3': 1, '4': 1, '5': 9, '10': 'sessionId'},
    {
      '1': 'phase',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.relkit.updater.v1.SessionPhase',
      '10': 'phase'
    },
  ],
};

/// Descriptor for `ApplyProgress`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List applyProgressDescriptor = $convert.base64Decode(
    'Cg1BcHBseVByb2dyZXNzEh0KCnNlc3Npb25faWQYASABKAlSCXNlc3Npb25JZBI1CgVwaGFzZR'
    'gCIAEoDjIfLnJlbGtpdC51cGRhdGVyLnYxLlNlc3Npb25QaGFzZVIFcGhhc2U=');

@$core.Deprecated('Use logDescriptor instead')
const Log$json = {
  '1': 'Log',
  '2': [
    {'1': 'message', '3': 1, '4': 1, '5': 9, '10': 'message'},
  ],
};

/// Descriptor for `Log`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List logDescriptor =
    $convert.base64Decode('CgNMb2cSGAoHbWVzc2FnZRgBIAEoCVIHbWVzc2FnZQ==');

@$core.Deprecated('Use updaterEventDescriptor instead')
const UpdaterEvent$json = {
  '1': 'UpdaterEvent',
  '2': [
    {
      '1': 'capabilities',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Capabilities',
      '9': 0,
      '10': 'capabilities'
    },
    {
      '1': 'progress',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Progress',
      '9': 0,
      '10': 'progress'
    },
    {
      '1': 'apply_progress',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.ApplyProgress',
      '9': 0,
      '10': 'applyProgress'
    },
    {
      '1': 'log',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Log',
      '9': 0,
      '10': 'log'
    },
    {
      '1': 'check',
      '3': 10,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.CheckResult',
      '9': 0,
      '10': 'check'
    },
    {
      '1': 'download',
      '3': 11,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.DownloadResult',
      '9': 0,
      '10': 'download'
    },
    {
      '1': 'apply',
      '3': 12,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.ApplyResult',
      '9': 0,
      '10': 'apply'
    },
    {
      '1': 'result',
      '3': 13,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Result',
      '9': 0,
      '10': 'result'
    },
    {
      '1': 'status',
      '3': 14,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.StatusSnapshot',
      '9': 0,
      '10': 'status'
    },
    {
      '1': 'failed',
      '3': 15,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Failed',
      '9': 0,
      '10': 'failed'
    },
  ],
  '8': [
    {'1': 'kind'},
  ],
};

/// Descriptor for `UpdaterEvent`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List updaterEventDescriptor = $convert.base64Decode(
    'CgxVcGRhdGVyRXZlbnQSRQoMY2FwYWJpbGl0aWVzGAEgASgLMh8ucmVsa2l0LnVwZGF0ZXIudj'
    'EuQ2FwYWJpbGl0aWVzSABSDGNhcGFiaWxpdGllcxI5Cghwcm9ncmVzcxgCIAEoCzIbLnJlbGtp'
    'dC51cGRhdGVyLnYxLlByb2dyZXNzSABSCHByb2dyZXNzEkkKDmFwcGx5X3Byb2dyZXNzGAMgAS'
    'gLMiAucmVsa2l0LnVwZGF0ZXIudjEuQXBwbHlQcm9ncmVzc0gAUg1hcHBseVByb2dyZXNzEioK'
    'A2xvZxgEIAEoCzIWLnJlbGtpdC51cGRhdGVyLnYxLkxvZ0gAUgNsb2cSNgoFY2hlY2sYCiABKA'
    'syHi5yZWxraXQudXBkYXRlci52MS5DaGVja1Jlc3VsdEgAUgVjaGVjaxI/Cghkb3dubG9hZBgL'
    'IAEoCzIhLnJlbGtpdC51cGRhdGVyLnYxLkRvd25sb2FkUmVzdWx0SABSCGRvd25sb2FkEjYKBW'
    'FwcGx5GAwgASgLMh4ucmVsa2l0LnVwZGF0ZXIudjEuQXBwbHlSZXN1bHRIAFIFYXBwbHkSMwoG'
    'cmVzdWx0GA0gASgLMhkucmVsa2l0LnVwZGF0ZXIudjEuUmVzdWx0SABSBnJlc3VsdBI7CgZzdG'
    'F0dXMYDiABKAsyIS5yZWxraXQudXBkYXRlci52MS5TdGF0dXNTbmFwc2hvdEgAUgZzdGF0dXMS'
    'MwoGZmFpbGVkGA8gASgLMhkucmVsa2l0LnVwZGF0ZXIudjEuRmFpbGVkSABSBmZhaWxlZEIGCg'
    'RraW5k');

@$core.Deprecated('Use artifactTargetDescriptor instead')
const ArtifactTarget$json = {
  '1': 'ArtifactTarget',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
    {
      '1': 'selectors',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.ArtifactTarget.SelectorsEntry',
      '10': 'selectors'
    },
  ],
  '3': [ArtifactTarget_SelectorsEntry$json],
};

@$core.Deprecated('Use artifactTargetDescriptor instead')
const ArtifactTarget_SelectorsEntry$json = {
  '1': 'SelectorsEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 9, '10': 'value'},
  ],
  '7': {'7': true},
};

/// Descriptor for `ArtifactTarget`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List artifactTargetDescriptor = $convert.base64Decode(
    'Cg5BcnRpZmFjdFRhcmdldBISCgRuYW1lGAEgASgJUgRuYW1lEk4KCXNlbGVjdG9ycxgCIAMoCz'
    'IwLnJlbGtpdC51cGRhdGVyLnYxLkFydGlmYWN0VGFyZ2V0LlNlbGVjdG9yc0VudHJ5UglzZWxl'
    'Y3RvcnMaPAoOU2VsZWN0b3JzRW50cnkSEAoDa2V5GAEgASgJUgNrZXkSFAoFdmFsdWUYAiABKA'
    'lSBXZhbHVlOgI4AQ==');

@$core.Deprecated('Use plannedFileDescriptor instead')
const PlannedFile$json = {
  '1': 'PlannedFile',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
    {'1': 'size', '3': 2, '4': 1, '5': 3, '10': 'size'},
    {'1': 'sha256_hex', '3': 3, '4': 1, '5': 9, '10': 'sha256Hex'},
    {'1': 'urls', '3': 4, '4': 3, '5': 9, '10': 'urls'},
    {'1': 'dest_relpath', '3': 5, '4': 1, '5': 9, '10': 'destRelpath'},
    {'1': 'local_path', '3': 6, '4': 1, '5': 9, '10': 'localPath'},
    {'1': 'downloaded', '3': 7, '4': 1, '5': 8, '10': 'downloaded'},
  ],
};

/// Descriptor for `PlannedFile`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List plannedFileDescriptor = $convert.base64Decode(
    'CgtQbGFubmVkRmlsZRISCgRuYW1lGAEgASgJUgRuYW1lEhIKBHNpemUYAiABKANSBHNpemUSHQ'
    'oKc2hhMjU2X2hleBgDIAEoCVIJc2hhMjU2SGV4EhIKBHVybHMYBCADKAlSBHVybHMSIQoMZGVz'
    'dF9yZWxwYXRoGAUgASgJUgtkZXN0UmVscGF0aBIdCgpsb2NhbF9wYXRoGAYgASgJUglsb2NhbF'
    'BhdGgSHgoKZG93bmxvYWRlZBgHIAEoCFIKZG93bmxvYWRlZA==');

@$core.Deprecated('Use updatePlanDescriptor instead')
const UpdatePlan$json = {
  '1': 'UpdatePlan',
  '2': [
    {'1': 'plan_id', '3': 1, '4': 1, '5': 9, '10': 'planId'},
    {'1': 'product', '3': 2, '4': 1, '5': 9, '10': 'product'},
    {'1': 'channel', '3': 3, '4': 1, '5': 9, '10': 'channel'},
    {'1': 'version', '3': 4, '4': 1, '5': 9, '10': 'version'},
    {'1': 'code', '3': 5, '4': 1, '5': 3, '10': 'code'},
    {'1': 'sequence', '3': 6, '4': 1, '5': 3, '10': 'sequence'},
    {'1': 'mandatory', '3': 7, '4': 1, '5': 8, '10': 'mandatory'},
    {'1': 'remaining_hops', '3': 8, '4': 1, '5': 5, '10': 'remainingHops'},
    {
      '1': 'release_notes_markdown',
      '3': 9,
      '4': 1,
      '5': 9,
      '10': 'releaseNotesMarkdown'
    },
    {
      '1': 'release_notes_url',
      '3': 10,
      '4': 1,
      '5': 9,
      '10': 'releaseNotesUrl'
    },
    {
      '1': 'prior_release_notes',
      '3': 11,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.PriorReleaseNotes',
      '10': 'priorReleaseNotes'
    },
    {
      '1': 'files',
      '3': 12,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.PlannedFile',
      '10': 'files'
    },
    {
      '1': 'created_at',
      '3': 13,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Timestamp',
      '10': 'createdAt'
    },
    {
      '1': 'expires_at',
      '3': 14,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Timestamp',
      '10': 'expiresAt'
    },
    {'1': 'plan_hmac', '3': 15, '4': 1, '5': 12, '10': 'planHmac'},
  ],
};

/// Descriptor for `UpdatePlan`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List updatePlanDescriptor = $convert.base64Decode(
    'CgpVcGRhdGVQbGFuEhcKB3BsYW5faWQYASABKAlSBnBsYW5JZBIYCgdwcm9kdWN0GAIgASgJUg'
    'dwcm9kdWN0EhgKB2NoYW5uZWwYAyABKAlSB2NoYW5uZWwSGAoHdmVyc2lvbhgEIAEoCVIHdmVy'
    'c2lvbhISCgRjb2RlGAUgASgDUgRjb2RlEhoKCHNlcXVlbmNlGAYgASgDUghzZXF1ZW5jZRIcCg'
    'ltYW5kYXRvcnkYByABKAhSCW1hbmRhdG9yeRIlCg5yZW1haW5pbmdfaG9wcxgIIAEoBVINcmVt'
    'YWluaW5nSG9wcxI0ChZyZWxlYXNlX25vdGVzX21hcmtkb3duGAkgASgJUhRyZWxlYXNlTm90ZX'
    'NNYXJrZG93bhIqChFyZWxlYXNlX25vdGVzX3VybBgKIAEoCVIPcmVsZWFzZU5vdGVzVXJsElQK'
    'E3ByaW9yX3JlbGVhc2Vfbm90ZXMYCyADKAsyJC5yZWxraXQudXBkYXRlci52MS5QcmlvclJlbG'
    'Vhc2VOb3Rlc1IRcHJpb3JSZWxlYXNlTm90ZXMSNAoFZmlsZXMYDCADKAsyHi5yZWxraXQudXBk'
    'YXRlci52MS5QbGFubmVkRmlsZVIFZmlsZXMSOQoKY3JlYXRlZF9hdBgNIAEoCzIaLmdvb2dsZS'
    '5wcm90b2J1Zi5UaW1lc3RhbXBSCWNyZWF0ZWRBdBI5CgpleHBpcmVzX2F0GA4gASgLMhouZ29v'
    'Z2xlLnByb3RvYnVmLlRpbWVzdGFtcFIJZXhwaXJlc0F0EhsKCXBsYW5faG1hYxgPIAEoDFIIcG'
    'xhbkhtYWM=');

@$core.Deprecated('Use persistedStateDescriptor instead')
const PersistedState$json = {
  '1': 'PersistedState',
  '2': [
    {
      '1': 'last_check_at',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Timestamp',
      '10': 'lastCheckAt'
    },
    {
      '1': 'last_result',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.relkit.updater.v1.LastResult',
      '10': 'lastResult'
    },
    {
      '1': 'last_seen_sequence',
      '3': 3,
      '4': 1,
      '5': 3,
      '10': 'lastSeenSequence'
    },
    {
      '1': 'last_seen_directory_sequence',
      '3': 4,
      '4': 1,
      '5': 3,
      '10': 'lastSeenDirectorySequence'
    },
    {
      '1': 'last_seen_fallback_sequence',
      '3': 5,
      '4': 1,
      '5': 3,
      '10': 'lastSeenFallbackSequence'
    },
    {'1': 'skipped_codes', '3': 6, '4': 3, '5': 3, '10': 'skippedCodes'},
  ],
};

/// Descriptor for `PersistedState`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List persistedStateDescriptor = $convert.base64Decode(
    'Cg5QZXJzaXN0ZWRTdGF0ZRI+Cg1sYXN0X2NoZWNrX2F0GAEgASgLMhouZ29vZ2xlLnByb3RvYn'
    'VmLlRpbWVzdGFtcFILbGFzdENoZWNrQXQSPgoLbGFzdF9yZXN1bHQYAiABKA4yHS5yZWxraXQu'
    'dXBkYXRlci52MS5MYXN0UmVzdWx0UgpsYXN0UmVzdWx0EiwKEmxhc3Rfc2Vlbl9zZXF1ZW5jZR'
    'gDIAEoA1IQbGFzdFNlZW5TZXF1ZW5jZRI/ChxsYXN0X3NlZW5fZGlyZWN0b3J5X3NlcXVlbmNl'
    'GAQgASgDUhlsYXN0U2VlbkRpcmVjdG9yeVNlcXVlbmNlEj0KG2xhc3Rfc2Vlbl9mYWxsYmFja1'
    '9zZXF1ZW5jZRgFIAEoA1IYbGFzdFNlZW5GYWxsYmFja1NlcXVlbmNlEiMKDXNraXBwZWRfY29k'
    'ZXMYBiADKANSDHNraXBwZWRDb2Rlcw==');

@$core.Deprecated('Use applySessionRecordDescriptor instead')
const ApplySessionRecord$json = {
  '1': 'ApplySessionRecord',
  '2': [
    {'1': 'session_id', '3': 1, '4': 1, '5': 9, '10': 'sessionId'},
    {'1': 'plan_id', '3': 2, '4': 1, '5': 9, '10': 'planId'},
    {
      '1': 'phase',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.relkit.updater.v1.SessionPhase',
      '10': 'phase'
    },
    {
      '1': 'started_at',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Timestamp',
      '10': 'startedAt'
    },
    {
      '1': 'heartbeat_at',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Timestamp',
      '10': 'heartbeatAt'
    },
    {'1': 'install_root', '3': 6, '4': 1, '5': 9, '10': 'installRoot'},
    {'1': 'staged_root', '3': 7, '4': 1, '5': 9, '10': 'stagedRoot'},
    {'1': 'target_code', '3': 8, '4': 1, '5': 3, '10': 'targetCode'},
    {'1': 'target_version', '3': 9, '4': 1, '5': 9, '10': 'targetVersion'},
    {
      '1': 'error',
      '3': 10,
      '4': 1,
      '5': 11,
      '6': '.relkit.updater.v1.Error',
      '10': 'error'
    },
    {'1': 'pid', '3': 11, '4': 1, '5': 5, '10': 'pid'},
    {
      '1': 'layout',
      '3': 12,
      '4': 1,
      '5': 14,
      '6': '.relkit.updater.v1.Layout',
      '10': 'layout'
    },
    {'1': 'relaunch', '3': 13, '4': 1, '5': 8, '10': 'relaunch'},
    {
      '1': 'executable_relpath',
      '3': 14,
      '4': 1,
      '5': 9,
      '10': 'executableRelpath'
    },
    {'1': 'preserve', '3': 15, '4': 3, '5': 9, '10': 'preserve'},
    {'1': 'retain', '3': 16, '4': 1, '5': 5, '10': 'retain'},
    {
      '1': 'file_set',
      '3': 17,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.FileSetEntry',
      '10': 'fileSet'
    },
    {'1': 'sidecar_relpath', '3': 18, '4': 1, '5': 9, '10': 'sidecarRelpath'},
  ],
};

/// Descriptor for `ApplySessionRecord`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List applySessionRecordDescriptor = $convert.base64Decode(
    'ChJBcHBseVNlc3Npb25SZWNvcmQSHQoKc2Vzc2lvbl9pZBgBIAEoCVIJc2Vzc2lvbklkEhcKB3'
    'BsYW5faWQYAiABKAlSBnBsYW5JZBI1CgVwaGFzZRgDIAEoDjIfLnJlbGtpdC51cGRhdGVyLnYx'
    'LlNlc3Npb25QaGFzZVIFcGhhc2USOQoKc3RhcnRlZF9hdBgEIAEoCzIaLmdvb2dsZS5wcm90b2'
    'J1Zi5UaW1lc3RhbXBSCXN0YXJ0ZWRBdBI9CgxoZWFydGJlYXRfYXQYBSABKAsyGi5nb29nbGUu'
    'cHJvdG9idWYuVGltZXN0YW1wUgtoZWFydGJlYXRBdBIhCgxpbnN0YWxsX3Jvb3QYBiABKAlSC2'
    'luc3RhbGxSb290Eh8KC3N0YWdlZF9yb290GAcgASgJUgpzdGFnZWRSb290Eh8KC3RhcmdldF9j'
    'b2RlGAggASgDUgp0YXJnZXRDb2RlEiUKDnRhcmdldF92ZXJzaW9uGAkgASgJUg10YXJnZXRWZX'
    'JzaW9uEi4KBWVycm9yGAogASgLMhgucmVsa2l0LnVwZGF0ZXIudjEuRXJyb3JSBWVycm9yEhAK'
    'A3BpZBgLIAEoBVIDcGlkEjEKBmxheW91dBgMIAEoDjIZLnJlbGtpdC51cGRhdGVyLnYxLkxheW'
    '91dFIGbGF5b3V0EhoKCHJlbGF1bmNoGA0gASgIUghyZWxhdW5jaBItChJleGVjdXRhYmxlX3Jl'
    'bHBhdGgYDiABKAlSEWV4ZWN1dGFibGVSZWxwYXRoEhoKCHByZXNlcnZlGA8gAygJUghwcmVzZX'
    'J2ZRIWCgZyZXRhaW4YECABKAVSBnJldGFpbhI6CghmaWxlX3NldBgRIAMoCzIfLnJlbGtpdC51'
    'cGRhdGVyLnYxLkZpbGVTZXRFbnRyeVIHZmlsZVNldBInCg9zaWRlY2FyX3JlbHBhdGgYEiABKA'
    'lSDnNpZGVjYXJSZWxwYXRo');

@$core.Deprecated('Use journalEntryDescriptor instead')
const JournalEntry$json = {
  '1': 'JournalEntry',
  '2': [
    {'1': 'dest_path', '3': 1, '4': 1, '5': 9, '10': 'destPath'},
    {'1': 'backup_path', '3': 2, '4': 1, '5': 9, '10': 'backupPath'},
    {'1': 'source_path', '3': 3, '4': 1, '5': 9, '10': 'sourcePath'},
  ],
};

/// Descriptor for `JournalEntry`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List journalEntryDescriptor = $convert.base64Decode(
    'CgxKb3VybmFsRW50cnkSGwoJZGVzdF9wYXRoGAEgASgJUghkZXN0UGF0aBIfCgtiYWNrdXBfcG'
    'F0aBgCIAEoCVIKYmFja3VwUGF0aBIfCgtzb3VyY2VfcGF0aBgDIAEoCVIKc291cmNlUGF0aA==');

@$core.Deprecated('Use applyJournalDescriptor instead')
const ApplyJournal$json = {
  '1': 'ApplyJournal',
  '2': [
    {'1': 'session_id', '3': 1, '4': 1, '5': 9, '10': 'sessionId'},
    {
      '1': 'entries',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.JournalEntry',
      '10': 'entries'
    },
    {'1': 'committed', '3': 3, '4': 1, '5': 8, '10': 'committed'},
  ],
};

/// Descriptor for `ApplyJournal`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List applyJournalDescriptor = $convert.base64Decode(
    'CgxBcHBseUpvdXJuYWwSHQoKc2Vzc2lvbl9pZBgBIAEoCVIJc2Vzc2lvbklkEjkKB2VudHJpZX'
    'MYAiADKAsyHy5yZWxraXQudXBkYXRlci52MS5Kb3VybmFsRW50cnlSB2VudHJpZXMSHAoJY29t'
    'bWl0dGVkGAMgASgIUgljb21taXR0ZWQ=');
