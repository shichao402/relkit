/// Expand a verified update package into a staging directory.
///
/// The outer transport is always a zip today:
/// - Windows: zip holds the portable tree directly (extract-and-overwrite).
/// - macOS: zip holds a single `.dmg`; after unzip we mount it and copy the
///   portable tree out so the rest of apply sees the same layout as Windows.
///
/// A future Windows `.exe` installer would plug in here; do not treat every
/// `.exe` inside a zip as an installer (the portable payload itself contains
/// `SvnAutoMerge.exe`).
library;

import 'dart:io';

import 'package:archive/archive_io.dart';

import 'apply_exception.dart';

/// File name of the macOS disk image inside the outer zip.
const macosDmgInnerName = 'SvnAutoMerge.dmg';

/// Unpacks [archive] into [stagingRoot], expanding an inner `.dmg` when present.
Future<void> unpackUpdatePackage({
  required File archive,
  required Directory stagingRoot,
  void Function(String message)? log,
}) async {
  final note = log ?? (_) {};

  if (stagingRoot.existsSync()) {
    stagingRoot.deleteSync(recursive: true);
  }
  stagingRoot.createSync(recursive: true);

  note('unpacking ${archive.path} into ${stagingRoot.path}');
  try {
    await extractFileToDisk(archive.path, stagingRoot.path);
  } on Object catch (error) {
    throw ApplyException('could not unpack ${archive.path}', cause: error);
  }

  await expandInnerInstallerIfPresent(stagingRoot, log: note);
}

/// If [root] contains a `.dmg`, replace [root]'s contents with the DMG payload.
///
/// No-op when there is no DMG (Windows / legacy flat macOS zip).
Future<void> expandInnerInstallerIfPresent(
  Directory root, {
  void Function(String message)? log,
}) async {
  final note = log ?? (_) {};
  if (!root.existsSync()) return;

  final dmgs = root
      .listSync(followLinks: false)
      .whereType<File>()
      .where((file) => file.path.toLowerCase().endsWith('.dmg'))
      .toList();

  if (dmgs.isEmpty) {
    return;
  }
  if (dmgs.length > 1) {
    throw ApplyException(
      'update package contains ${dmgs.length} .dmg files; expected one',
    );
  }
  if (!Platform.isMacOS) {
    throw ApplyException(
      'update package contains a .dmg but this host is not macOS',
    );
  }

  final dmg = dmgs.single;
  note('expanding inner DMG ${dmg.path}');
  await _expandDmgInto(dmg, root, log: note);
}

Future<void> _expandDmgInto(
  File dmg,
  Directory destination, {
  required void Function(String message) log,
}) async {
  final parent = destination.parent;
  final baseName = destination.uri.pathSegments
      .where((segment) => segment.isNotEmpty)
      .last;
  final mountPoint = Directory(
    '${parent.path}${Platform.pathSeparator}$baseName.dmg-mount',
  );
  final extracted = Directory(
    '${parent.path}${Platform.pathSeparator}$baseName.dmg-payload',
  );

  if (mountPoint.existsSync()) {
    mountPoint.deleteSync(recursive: true);
  }
  if (extracted.existsSync()) {
    extracted.deleteSync(recursive: true);
  }
  mountPoint.createSync(recursive: true);
  extracted.createSync(recursive: true);

  var attached = false;
  try {
    final attach = await Process.run(
      'hdiutil',
      [
        'attach',
        dmg.absolute.path,
        '-nobrowse',
        '-readonly',
        '-mountpoint',
        mountPoint.path,
      ],
    );
    if (attach.exitCode != 0) {
      final detail = attach.stderr.toString().trim().isEmpty
          ? attach.stdout
          : attach.stderr;
      throw ApplyException(
        'hdiutil attach failed for ${dmg.path}',
        cause: detail,
      );
    }
    attached = true;

    // ditto preserves symlinks inside .app frameworks.
    final copy = await Process.run(
      'ditto',
      [mountPoint.path, extracted.path],
    );
    if (copy.exitCode != 0) {
      final detail = copy.stderr.toString().trim().isEmpty
          ? copy.stdout
          : copy.stderr;
      throw ApplyException('ditto from DMG mount failed', cause: detail);
    }

    if (destination.existsSync()) {
      destination.deleteSync(recursive: true);
    }
    extracted.renameSync(destination.path);
    log('DMG payload copied into ${destination.path}');
  } finally {
    if (attached) {
      await Process.run(
        'hdiutil',
        ['detach', mountPoint.path, '-force'],
      );
    }
    if (mountPoint.existsSync()) {
      try {
        mountPoint.deleteSync(recursive: true);
      } on FileSystemException {
        // Detach may leave the mount point busy briefly.
      }
    }
    if (extracted.existsSync()) {
      try {
        extracted.deleteSync(recursive: true);
      } on FileSystemException {
        // Renamed away on the success path.
      }
    }
  }
}
