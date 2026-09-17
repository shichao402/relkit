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

import 'package:protobuf/protobuf.dart' as $pb;

class ArtifactKind extends $pb.ProtobufEnum {
  static const ArtifactKind ARTIFACT_KIND_UNSPECIFIED =
      ArtifactKind._(0, _omitEnumNames ? '' : 'ARTIFACT_KIND_UNSPECIFIED');
  static const ArtifactKind ARTIFACT_KIND_ARCHIVE =
      ArtifactKind._(1, _omitEnumNames ? '' : 'ARTIFACT_KIND_ARCHIVE');
  static const ArtifactKind ARTIFACT_KIND_INSTALLER =
      ArtifactKind._(2, _omitEnumNames ? '' : 'ARTIFACT_KIND_INSTALLER');
  static const ArtifactKind ARTIFACT_KIND_BINARY =
      ArtifactKind._(3, _omitEnumNames ? '' : 'ARTIFACT_KIND_BINARY');
  static const ArtifactKind ARTIFACT_KIND_BLOB =
      ArtifactKind._(4, _omitEnumNames ? '' : 'ARTIFACT_KIND_BLOB');
  static const ArtifactKind ARTIFACT_KIND_PAYLOAD =
      ArtifactKind._(5, _omitEnumNames ? '' : 'ARTIFACT_KIND_PAYLOAD');

  static const $core.List<ArtifactKind> values = <ArtifactKind>[
    ARTIFACT_KIND_UNSPECIFIED,
    ARTIFACT_KIND_ARCHIVE,
    ARTIFACT_KIND_INSTALLER,
    ARTIFACT_KIND_BINARY,
    ARTIFACT_KIND_BLOB,
    ARTIFACT_KIND_PAYLOAD,
  ];

  static final $core.List<ArtifactKind?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 5);
  static ArtifactKind? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const ArtifactKind._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
