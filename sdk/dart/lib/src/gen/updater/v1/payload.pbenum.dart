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

import 'package:protobuf/protobuf.dart' as $pb;

class ScriptPhase extends $pb.ProtobufEnum {
  static const ScriptPhase SCRIPT_PHASE_UNSPECIFIED =
      ScriptPhase._(0, _omitEnumNames ? '' : 'SCRIPT_PHASE_UNSPECIFIED');
  static const ScriptPhase SCRIPT_PHASE_PRE_APPLY =
      ScriptPhase._(1, _omitEnumNames ? '' : 'SCRIPT_PHASE_PRE_APPLY');
  static const ScriptPhase SCRIPT_PHASE_POST_APPLY =
      ScriptPhase._(2, _omitEnumNames ? '' : 'SCRIPT_PHASE_POST_APPLY');

  static const $core.List<ScriptPhase> values = <ScriptPhase>[
    SCRIPT_PHASE_UNSPECIFIED,
    SCRIPT_PHASE_PRE_APPLY,
    SCRIPT_PHASE_POST_APPLY,
  ];

  static final $core.List<ScriptPhase?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static ScriptPhase? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const ScriptPhase._(super.value, super.name);
}

class ScriptInterpreter extends $pb.ProtobufEnum {
  static const ScriptInterpreter SCRIPT_INTERPRETER_UNSPECIFIED =
      ScriptInterpreter._(
          0, _omitEnumNames ? '' : 'SCRIPT_INTERPRETER_UNSPECIFIED');
  static const ScriptInterpreter SCRIPT_INTERPRETER_DIRECT =
      ScriptInterpreter._(1, _omitEnumNames ? '' : 'SCRIPT_INTERPRETER_DIRECT');
  static const ScriptInterpreter SCRIPT_INTERPRETER_SH =
      ScriptInterpreter._(2, _omitEnumNames ? '' : 'SCRIPT_INTERPRETER_SH');
  static const ScriptInterpreter SCRIPT_INTERPRETER_POWERSHELL =
      ScriptInterpreter._(
          3, _omitEnumNames ? '' : 'SCRIPT_INTERPRETER_POWERSHELL');

  static const $core.List<ScriptInterpreter> values = <ScriptInterpreter>[
    SCRIPT_INTERPRETER_UNSPECIFIED,
    SCRIPT_INTERPRETER_DIRECT,
    SCRIPT_INTERPRETER_SH,
    SCRIPT_INTERPRETER_POWERSHELL,
  ];

  static final $core.List<ScriptInterpreter?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 3);
  static ScriptInterpreter? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const ScriptInterpreter._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
