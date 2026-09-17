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

import 'package:protobuf/protobuf.dart' as $pb;

class ErrorCode extends $pb.ProtobufEnum {
  static const ErrorCode ERROR_CODE_UNSPECIFIED =
      ErrorCode._(0, _omitEnumNames ? '' : 'ERROR_CODE_UNSPECIFIED');
  static const ErrorCode ERROR_CODE_NETWORK =
      ErrorCode._(1, _omitEnumNames ? '' : 'ERROR_CODE_NETWORK');
  static const ErrorCode ERROR_CODE_SIGNATURE =
      ErrorCode._(2, _omitEnumNames ? '' : 'ERROR_CODE_SIGNATURE');
  static const ErrorCode ERROR_CODE_ROLLBACK_REJECTED =
      ErrorCode._(3, _omitEnumNames ? '' : 'ERROR_CODE_ROLLBACK_REJECTED');
  static const ErrorCode ERROR_CODE_SELECTOR_NO_MATCH =
      ErrorCode._(4, _omitEnumNames ? '' : 'ERROR_CODE_SELECTOR_NO_MATCH');
  static const ErrorCode ERROR_CODE_DISK =
      ErrorCode._(5, _omitEnumNames ? '' : 'ERROR_CODE_DISK');
  static const ErrorCode ERROR_CODE_PERMISSION_DENIED =
      ErrorCode._(6, _omitEnumNames ? '' : 'ERROR_CODE_PERMISSION_DENIED');
  static const ErrorCode ERROR_CODE_OCCUPIED =
      ErrorCode._(7, _omitEnumNames ? '' : 'ERROR_CODE_OCCUPIED');
  static const ErrorCode ERROR_CODE_PROTOCOL_MISMATCH =
      ErrorCode._(8, _omitEnumNames ? '' : 'ERROR_CODE_PROTOCOL_MISMATCH');
  static const ErrorCode ERROR_CODE_UPDATER_TOO_OLD =
      ErrorCode._(9, _omitEnumNames ? '' : 'ERROR_CODE_UPDATER_TOO_OLD');
  static const ErrorCode ERROR_CODE_UPDATER_TOO_NEW =
      ErrorCode._(10, _omitEnumNames ? '' : 'ERROR_CODE_UPDATER_TOO_NEW');
  static const ErrorCode ERROR_CODE_PLAN_TAMPERED =
      ErrorCode._(11, _omitEnumNames ? '' : 'ERROR_CODE_PLAN_TAMPERED');
  static const ErrorCode ERROR_CODE_PLAN_EXPIRED =
      ErrorCode._(12, _omitEnumNames ? '' : 'ERROR_CODE_PLAN_EXPIRED');
  static const ErrorCode ERROR_CODE_PLAN_UNKNOWN =
      ErrorCode._(13, _omitEnumNames ? '' : 'ERROR_CODE_PLAN_UNKNOWN');
  static const ErrorCode ERROR_CODE_PLAN_NOT_DOWNLOADED =
      ErrorCode._(14, _omitEnumNames ? '' : 'ERROR_CODE_PLAN_NOT_DOWNLOADED');
  static const ErrorCode ERROR_CODE_SKIP_DENIED =
      ErrorCode._(15, _omitEnumNames ? '' : 'ERROR_CODE_SKIP_DENIED');
  static const ErrorCode ERROR_CODE_PROFILE_INVALID =
      ErrorCode._(16, _omitEnumNames ? '' : 'ERROR_CODE_PROFILE_INVALID');
  static const ErrorCode ERROR_CODE_CHANNEL_NOT_ALLOWED =
      ErrorCode._(17, _omitEnumNames ? '' : 'ERROR_CODE_CHANNEL_NOT_ALLOWED');
  static const ErrorCode ERROR_CODE_CANCELED =
      ErrorCode._(18, _omitEnumNames ? '' : 'ERROR_CODE_CANCELED');
  static const ErrorCode ERROR_CODE_SIDECAR_NOT_FOUND =
      ErrorCode._(19, _omitEnumNames ? '' : 'ERROR_CODE_SIDECAR_NOT_FOUND');
  static const ErrorCode ERROR_CODE_LAYOUT_UNSUPPORTED =
      ErrorCode._(20, _omitEnumNames ? '' : 'ERROR_CODE_LAYOUT_UNSUPPORTED');

  static const $core.List<ErrorCode> values = <ErrorCode>[
    ERROR_CODE_UNSPECIFIED,
    ERROR_CODE_NETWORK,
    ERROR_CODE_SIGNATURE,
    ERROR_CODE_ROLLBACK_REJECTED,
    ERROR_CODE_SELECTOR_NO_MATCH,
    ERROR_CODE_DISK,
    ERROR_CODE_PERMISSION_DENIED,
    ERROR_CODE_OCCUPIED,
    ERROR_CODE_PROTOCOL_MISMATCH,
    ERROR_CODE_UPDATER_TOO_OLD,
    ERROR_CODE_UPDATER_TOO_NEW,
    ERROR_CODE_PLAN_TAMPERED,
    ERROR_CODE_PLAN_EXPIRED,
    ERROR_CODE_PLAN_UNKNOWN,
    ERROR_CODE_PLAN_NOT_DOWNLOADED,
    ERROR_CODE_SKIP_DENIED,
    ERROR_CODE_PROFILE_INVALID,
    ERROR_CODE_CHANNEL_NOT_ALLOWED,
    ERROR_CODE_CANCELED,
    ERROR_CODE_SIDECAR_NOT_FOUND,
    ERROR_CODE_LAYOUT_UNSUPPORTED,
  ];

  static final $core.List<ErrorCode?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 20);
  static ErrorCode? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const ErrorCode._(super.value, super.name);
}

class LastResult extends $pb.ProtobufEnum {
  static const LastResult LAST_RESULT_UNSPECIFIED =
      LastResult._(0, _omitEnumNames ? '' : 'LAST_RESULT_UNSPECIFIED');
  static const LastResult LAST_RESULT_UP_TO_DATE =
      LastResult._(1, _omitEnumNames ? '' : 'LAST_RESULT_UP_TO_DATE');
  static const LastResult LAST_RESULT_UPDATE_AVAILABLE =
      LastResult._(2, _omitEnumNames ? '' : 'LAST_RESULT_UPDATE_AVAILABLE');
  static const LastResult LAST_RESULT_FALLBACK_REQUIRED =
      LastResult._(3, _omitEnumNames ? '' : 'LAST_RESULT_FALLBACK_REQUIRED');
  static const LastResult LAST_RESULT_THROTTLED =
      LastResult._(4, _omitEnumNames ? '' : 'LAST_RESULT_THROTTLED');
  static const LastResult LAST_RESULT_FAILED =
      LastResult._(5, _omitEnumNames ? '' : 'LAST_RESULT_FAILED');
  static const LastResult LAST_RESULT_APPLIED =
      LastResult._(6, _omitEnumNames ? '' : 'LAST_RESULT_APPLIED');

  static const $core.List<LastResult> values = <LastResult>[
    LAST_RESULT_UNSPECIFIED,
    LAST_RESULT_UP_TO_DATE,
    LAST_RESULT_UPDATE_AVAILABLE,
    LAST_RESULT_FALLBACK_REQUIRED,
    LAST_RESULT_THROTTLED,
    LAST_RESULT_FAILED,
    LAST_RESULT_APPLIED,
  ];

  static final $core.List<LastResult?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 6);
  static LastResult? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const LastResult._(super.value, super.name);
}

class Placement extends $pb.ProtobufEnum {
  static const Placement PLACEMENT_UNSPECIFIED =
      Placement._(0, _omitEnumNames ? '' : 'PLACEMENT_UNSPECIFIED');
  static const Placement PLACEMENT_IN_PLACE =
      Placement._(1, _omitEnumNames ? '' : 'PLACEMENT_IN_PLACE');
  static const Placement PLACEMENT_LIBRARY =
      Placement._(2, _omitEnumNames ? '' : 'PLACEMENT_LIBRARY');

  static const $core.List<Placement> values = <Placement>[
    PLACEMENT_UNSPECIFIED,
    PLACEMENT_IN_PLACE,
    PLACEMENT_LIBRARY,
  ];

  static final $core.List<Placement?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static Placement? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const Placement._(super.value, super.name);
}

class ApplyDisposition extends $pb.ProtobufEnum {
  static const ApplyDisposition APPLY_DISPOSITION_UNSPECIFIED =
      ApplyDisposition._(
          0, _omitEnumNames ? '' : 'APPLY_DISPOSITION_UNSPECIFIED');
  static const ApplyDisposition APPLY_DISPOSITION_INTERNAL =
      ApplyDisposition._(1, _omitEnumNames ? '' : 'APPLY_DISPOSITION_INTERNAL');
  static const ApplyDisposition APPLY_DISPOSITION_FULL_INSTALL =
      ApplyDisposition._(
          2, _omitEnumNames ? '' : 'APPLY_DISPOSITION_FULL_INSTALL');

  static const $core.List<ApplyDisposition> values = <ApplyDisposition>[
    APPLY_DISPOSITION_UNSPECIFIED,
    APPLY_DISPOSITION_INTERNAL,
    APPLY_DISPOSITION_FULL_INSTALL,
  ];

  static final $core.List<ApplyDisposition?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static ApplyDisposition? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const ApplyDisposition._(super.value, super.name);
}

class Operation extends $pb.ProtobufEnum {
  static const Operation OPERATION_UNSPECIFIED =
      Operation._(0, _omitEnumNames ? '' : 'OPERATION_UNSPECIFIED');
  static const Operation OPERATION_CHECK =
      Operation._(1, _omitEnumNames ? '' : 'OPERATION_CHECK');
  static const Operation OPERATION_SKIP =
      Operation._(2, _omitEnumNames ? '' : 'OPERATION_SKIP');
  static const Operation OPERATION_DOWNLOAD =
      Operation._(3, _omitEnumNames ? '' : 'OPERATION_DOWNLOAD');
  static const Operation OPERATION_APPLY =
      Operation._(4, _omitEnumNames ? '' : 'OPERATION_APPLY');
  static const Operation OPERATION_STATUS =
      Operation._(5, _omitEnumNames ? '' : 'OPERATION_STATUS');
  static const Operation OPERATION_CLEANUP =
      Operation._(6, _omitEnumNames ? '' : 'OPERATION_CLEANUP');
  static const Operation OPERATION_CANCEL =
      Operation._(7, _omitEnumNames ? '' : 'OPERATION_CANCEL');
  static const Operation OPERATION_SCHEDULER =
      Operation._(8, _omitEnumNames ? '' : 'OPERATION_SCHEDULER');
  static const Operation OPERATION_LIST_INSTALLED =
      Operation._(9, _omitEnumNames ? '' : 'OPERATION_LIST_INSTALLED');
  static const Operation OPERATION_SWITCH_ACTIVE =
      Operation._(10, _omitEnumNames ? '' : 'OPERATION_SWITCH_ACTIVE');
  static const Operation OPERATION_ROLLBACK =
      Operation._(11, _omitEnumNames ? '' : 'OPERATION_ROLLBACK');

  static const $core.List<Operation> values = <Operation>[
    OPERATION_UNSPECIFIED,
    OPERATION_CHECK,
    OPERATION_SKIP,
    OPERATION_DOWNLOAD,
    OPERATION_APPLY,
    OPERATION_STATUS,
    OPERATION_CLEANUP,
    OPERATION_CANCEL,
    OPERATION_SCHEDULER,
    OPERATION_LIST_INSTALLED,
    OPERATION_SWITCH_ACTIVE,
    OPERATION_ROLLBACK,
  ];

  static final $core.List<Operation?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 11);
  static Operation? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const Operation._(super.value, super.name);
}

class SessionPhase extends $pb.ProtobufEnum {
  static const SessionPhase SESSION_PHASE_UNSPECIFIED =
      SessionPhase._(0, _omitEnumNames ? '' : 'SESSION_PHASE_UNSPECIFIED');
  static const SessionPhase SESSION_PHASE_WAITING_FOR_EXIT =
      SessionPhase._(1, _omitEnumNames ? '' : 'SESSION_PHASE_WAITING_FOR_EXIT');
  static const SessionPhase SESSION_PHASE_COPYING =
      SessionPhase._(2, _omitEnumNames ? '' : 'SESSION_PHASE_COPYING');
  static const SessionPhase SESSION_PHASE_COMMITTING =
      SessionPhase._(3, _omitEnumNames ? '' : 'SESSION_PHASE_COMMITTING');
  static const SessionPhase SESSION_PHASE_RELAUNCHING =
      SessionPhase._(4, _omitEnumNames ? '' : 'SESSION_PHASE_RELAUNCHING');
  static const SessionPhase SESSION_PHASE_NEEDS_ATTENTION =
      SessionPhase._(5, _omitEnumNames ? '' : 'SESSION_PHASE_NEEDS_ATTENTION');
  static const SessionPhase SESSION_PHASE_COMPLETED =
      SessionPhase._(6, _omitEnumNames ? '' : 'SESSION_PHASE_COMPLETED');
  static const SessionPhase SESSION_PHASE_ROLLED_BACK =
      SessionPhase._(7, _omitEnumNames ? '' : 'SESSION_PHASE_ROLLED_BACK');

  static const $core.List<SessionPhase> values = <SessionPhase>[
    SESSION_PHASE_UNSPECIFIED,
    SESSION_PHASE_WAITING_FOR_EXIT,
    SESSION_PHASE_COPYING,
    SESSION_PHASE_COMMITTING,
    SESSION_PHASE_RELAUNCHING,
    SESSION_PHASE_NEEDS_ATTENTION,
    SESSION_PHASE_COMPLETED,
    SESSION_PHASE_ROLLED_BACK,
  ];

  static final $core.List<SessionPhase?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 7);
  static SessionPhase? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const SessionPhase._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
