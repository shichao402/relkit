// This is a generated file - do not edit.
//
// Generated from updater/v1/payload.proto.

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

@$core.Deprecated('Use scriptPhaseDescriptor instead')
const ScriptPhase$json = {
  '1': 'ScriptPhase',
  '2': [
    {'1': 'SCRIPT_PHASE_UNSPECIFIED', '2': 0},
    {'1': 'SCRIPT_PHASE_PRE_APPLY', '2': 1},
    {'1': 'SCRIPT_PHASE_POST_APPLY', '2': 2},
  ],
};

/// Descriptor for `ScriptPhase`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List scriptPhaseDescriptor = $convert.base64Decode(
    'CgtTY3JpcHRQaGFzZRIcChhTQ1JJUFRfUEhBU0VfVU5TUEVDSUZJRUQQABIaChZTQ1JJUFRfUE'
    'hBU0VfUFJFX0FQUExZEAESGwoXU0NSSVBUX1BIQVNFX1BPU1RfQVBQTFkQAg==');

@$core.Deprecated('Use scriptInterpreterDescriptor instead')
const ScriptInterpreter$json = {
  '1': 'ScriptInterpreter',
  '2': [
    {'1': 'SCRIPT_INTERPRETER_UNSPECIFIED', '2': 0},
    {'1': 'SCRIPT_INTERPRETER_DIRECT', '2': 1},
    {'1': 'SCRIPT_INTERPRETER_SH', '2': 2},
    {'1': 'SCRIPT_INTERPRETER_POWERSHELL', '2': 3},
  ],
};

/// Descriptor for `ScriptInterpreter`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List scriptInterpreterDescriptor = $convert.base64Decode(
    'ChFTY3JpcHRJbnRlcnByZXRlchIiCh5TQ1JJUFRfSU5URVJQUkVURVJfVU5TUEVDSUZJRUQQAB'
    'IdChlTQ1JJUFRfSU5URVJQUkVURVJfRElSRUNUEAESGQoVU0NSSVBUX0lOVEVSUFJFVEVSX1NI'
    'EAISIQodU0NSSVBUX0lOVEVSUFJFVEVSX1BPV0VSU0hFTEwQAw==');

@$core.Deprecated('Use fileTableDescriptor instead')
const FileTable$json = {
  '1': 'FileTable',
  '2': [
    {'1': 'schema', '3': 1, '4': 1, '5': 9, '10': 'schema'},
    {
      '1': 'files',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.FileEntry',
      '10': 'files'
    },
    {
      '1': 'scripts',
      '3': 3,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.ScriptEntry',
      '10': 'scripts'
    },
    {'1': 'preserve', '3': 4, '4': 3, '5': 9, '10': 'preserve'},
  ],
};

/// Descriptor for `FileTable`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List fileTableDescriptor = $convert.base64Decode(
    'CglGaWxlVGFibGUSFgoGc2NoZW1hGAEgASgJUgZzY2hlbWESMgoFZmlsZXMYAiADKAsyHC5yZW'
    'xraXQudXBkYXRlci52MS5GaWxlRW50cnlSBWZpbGVzEjgKB3NjcmlwdHMYAyADKAsyHi5yZWxr'
    'aXQudXBkYXRlci52MS5TY3JpcHRFbnRyeVIHc2NyaXB0cxIaCghwcmVzZXJ2ZRgEIAMoCVIIcH'
    'Jlc2VydmU=');

@$core.Deprecated('Use fileEntryDescriptor instead')
const FileEntry$json = {
  '1': 'FileEntry',
  '2': [
    {'1': 'relpath', '3': 1, '4': 1, '5': 9, '10': 'relpath'},
    {'1': 'size', '3': 2, '4': 1, '5': 3, '10': 'size'},
    {'1': 'sha256', '3': 3, '4': 1, '5': 9, '10': 'sha256'},
    {'1': 'mode', '3': 4, '4': 1, '5': 13, '10': 'mode'},
  ],
};

/// Descriptor for `FileEntry`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List fileEntryDescriptor = $convert.base64Decode(
    'CglGaWxlRW50cnkSGAoHcmVscGF0aBgBIAEoCVIHcmVscGF0aBISCgRzaXplGAIgASgDUgRzaX'
    'plEhYKBnNoYTI1NhgDIAEoCVIGc2hhMjU2EhIKBG1vZGUYBCABKA1SBG1vZGU=');

@$core.Deprecated('Use scriptEntryDescriptor instead')
const ScriptEntry$json = {
  '1': 'ScriptEntry',
  '2': [
    {
      '1': 'phase',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.relkit.updater.v1.ScriptPhase',
      '10': 'phase'
    },
    {'1': 'relpath', '3': 2, '4': 1, '5': 9, '10': 'relpath'},
    {'1': 'size', '3': 3, '4': 1, '5': 3, '10': 'size'},
    {'1': 'sha256', '3': 4, '4': 1, '5': 9, '10': 'sha256'},
    {
      '1': 'timeout',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Duration',
      '10': 'timeout'
    },
    {
      '1': 'interpreter',
      '3': 6,
      '4': 1,
      '5': 14,
      '6': '.relkit.updater.v1.ScriptInterpreter',
      '10': 'interpreter'
    },
    {'1': 'args', '3': 7, '4': 3, '5': 9, '10': 'args'},
  ],
};

/// Descriptor for `ScriptEntry`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List scriptEntryDescriptor = $convert.base64Decode(
    'CgtTY3JpcHRFbnRyeRI0CgVwaGFzZRgBIAEoDjIeLnJlbGtpdC51cGRhdGVyLnYxLlNjcmlwdF'
    'BoYXNlUgVwaGFzZRIYCgdyZWxwYXRoGAIgASgJUgdyZWxwYXRoEhIKBHNpemUYAyABKANSBHNp'
    'emUSFgoGc2hhMjU2GAQgASgJUgZzaGEyNTYSMwoHdGltZW91dBgFIAEoCzIZLmdvb2dsZS5wcm'
    '90b2J1Zi5EdXJhdGlvblIHdGltZW91dBJGCgtpbnRlcnByZXRlchgGIAEoDjIkLnJlbGtpdC51'
    'cGRhdGVyLnYxLlNjcmlwdEludGVycHJldGVyUgtpbnRlcnByZXRlchISCgRhcmdzGAcgAygJUg'
    'Rhcmdz');

@$core.Deprecated('Use baselineDescriptor instead')
const Baseline$json = {
  '1': 'Baseline',
  '2': [
    {'1': 'schema', '3': 1, '4': 1, '5': 9, '10': 'schema'},
    {'1': 'code', '3': 2, '4': 1, '5': 3, '10': 'code'},
    {'1': 'version', '3': 3, '4': 1, '5': 9, '10': 'version'},
    {
      '1': 'files',
      '3': 4,
      '4': 3,
      '5': 11,
      '6': '.relkit.updater.v1.BaselineEntry',
      '10': 'files'
    },
  ],
};

/// Descriptor for `Baseline`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List baselineDescriptor = $convert.base64Decode(
    'CghCYXNlbGluZRIWCgZzY2hlbWEYASABKAlSBnNjaGVtYRISCgRjb2RlGAIgASgDUgRjb2RlEh'
    'gKB3ZlcnNpb24YAyABKAlSB3ZlcnNpb24SNgoFZmlsZXMYBCADKAsyIC5yZWxraXQudXBkYXRl'
    'ci52MS5CYXNlbGluZUVudHJ5UgVmaWxlcw==');

@$core.Deprecated('Use baselineEntryDescriptor instead')
const BaselineEntry$json = {
  '1': 'BaselineEntry',
  '2': [
    {'1': 'relpath', '3': 1, '4': 1, '5': 9, '10': 'relpath'},
    {'1': 'sha256', '3': 2, '4': 1, '5': 9, '10': 'sha256'},
  ],
};

/// Descriptor for `BaselineEntry`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List baselineEntryDescriptor = $convert.base64Decode(
    'Cg1CYXNlbGluZUVudHJ5EhgKB3JlbHBhdGgYASABKAlSB3JlbHBhdGgSFgoGc2hhMjU2GAIgAS'
    'gJUgZzaGEyNTY=');
