/// Expand a verified update package into a staging directory.
///
/// The outer transport is always a zip today:
/// - Windows: zip holds the portable tree directly (extract-and-overwrite).
/// - macOS: zip holds a single `.dmg`; after unzip we mount it and copy the
///   install root out. Manual-install DMGs often also contain an `Applications`
///   symlink and Gatekeeper helper shortcuts — those must not be staged for
///   auto-update. When [executableName] is provided, only the directory that
///   contains that relative path is kept (preferring a `*.app` bundle).
///
/// A future Windows `.exe` installer would plug in here; do not treat every
/// `.exe` inside a zip as an installer (the portable payload itself usually
/// contains the main binary).
library;

import 'dart:io';

import 'package:archive/archive_io.dart';

import 'apply_exception.dart';

String _basename(String path) {
  final normalized = path.replaceAll('\\', '/');
  final parts = normalized.split('/');
  return parts.isEmpty ? path : parts.last;
}

String _join(String parent, String child) =>
    '$parent${Platform.pathSeparator}'
    '${child.replaceAll('/', Platform.pathSeparator)}';

/// Unpacks [archive] into [stagingRoot], expanding an inner `.dmg` when present.
///
/// Pass the same [executableName] that [stageUpdate] uses (relative to the
/// install root, e.g. `Contents/MacOS/MyApp` or `MyApp.exe`). On macOS this is
/// what lets the expander ignore DMG install helpers.
///
/// [clearTimeout] bounds the wait for [stagingRoot] to be released before
/// unpacking over it; see `_clearStagingRoot` for what tends to be holding it.
Future<void> unpackUpdatePackage({
  required File archive,
  required Directory stagingRoot,
  String? executableName,
  Duration clearTimeout = const Duration(seconds: 30),
  void Function(String message)? log,
}) async {
  final note = log ?? (_) {};

  await _clearStagingRoot(stagingRoot, timeout: clearTimeout, log: note);
  stagingRoot.createSync(recursive: true);

  note('unpacking ${archive.path} into ${stagingRoot.path}');
  try {
    await extractFileToDisk(archive.path, stagingRoot.path);
  } on Object catch (error) {
    throw ApplyException('could not unpack ${archive.path}', cause: error);
  }

  await expandInnerInstallerIfPresent(
    stagingRoot,
    executableName: executableName,
    log: note,
  );
}

/// Empties [stagingRoot], waiting out whatever is still holding it.
///
/// The thing most likely to be holding it is an apply from a previous attempt
/// that has not finished: the staged copy runs from inside this directory, so
/// on Windows it cannot be deleted until that process exits. Retrying turns
/// "the update is permanently broken until you reboot" into "the update takes
/// a few seconds longer", which is the whole difference for a user who just
/// pressed the button again.
///
/// The raw [FileSystemException] is deliberately not what escapes: "Deletion
/// failed, errno = 5" tells a user nothing they can act on.
Future<void> _clearStagingRoot(
  Directory stagingRoot, {
  Duration timeout = const Duration(seconds: 30),
  required void Function(String message) log,
}) async {
  if (!stagingRoot.existsSync()) return;

  final startedAt = DateTime.now();
  final deadline = startedAt.add(timeout);
  var delay = const Duration(milliseconds: 100);
  var reported = false;
  Object? last;

  while (true) {
    try {
      stagingRoot.deleteSync(recursive: true);
      return;
    } on FileSystemException catch (error) {
      last = error;
      if (!stagingRoot.existsSync()) return;

      if (!reported) {
        log('waiting for ${stagingRoot.path} to be released: $error');
        reported = true;
      }

      if (!DateTime.now().isBefore(deadline)) break;
      await Future<void>.delayed(delay);
      if (delay < const Duration(seconds: 1)) delay *= 2;
    }
  }

  throw ApplyException(
      'could not clear the staging directory ${stagingRoot.path} after '
      '${timeout.inSeconds}s. Something is still using it -- most likely an '
      'earlier update that has not finished. Wait for it to end, or restart '
      'the computer, then try again',
      cause: last);
}

/// If [root] contains a `.dmg`, replace [root]'s contents with the install root.
///
/// When [executableName] is set, only the payload directory that contains that
/// relative path is kept. Without it, the full DMG payload is copied (legacy
/// single-item DMGs); multi-item payloads then need [executableName] or
/// [stageUpdate] will fail to locate the binary.
///
/// No-op when there is no DMG (Windows / legacy flat macOS zip).
Future<void> expandInnerInstallerIfPresent(
  Directory root, {
  String? executableName,
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
  await _expandDmgInto(
    dmg,
    root,
    executableName: executableName,
    log: note,
  );
}

/// Finds the directory under [payload] that contains [executableName].
///
/// Search order:
/// 1. [payload] itself
/// 2. each immediate subdirectory, preferring `*.app` when several match
///
/// Returns null when nothing contains the executable.
Directory? selectInstallRootContainingExecutable(
  Directory payload,
  String executableName,
) {
  final relative = executableName.replaceAll('/', Platform.pathSeparator);
  final atRoot = File(_join(payload.path, relative));
  if (atRoot.existsSync()) {
    return payload;
  }

  Directory? match;
  for (final entry in payload.listSync(followLinks: false)) {
    if (entry is! Directory) {
      continue;
    }
    final candidate = File(_join(entry.path, relative));
    if (!candidate.existsSync()) {
      continue;
    }
    final isAppBundle = entry.path.toLowerCase().endsWith('.app');
    if (match == null || isAppBundle) {
      match = entry;
      if (isAppBundle) {
        return entry;
      }
    }
  }
  return match;
}

Future<void> _expandDmgInto(
  File dmg,
  Directory destination, {
  String? executableName,
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

    Directory stagedPayload = extracted;
    if (executableName != null && executableName.trim().isNotEmpty) {
      final installRoot = selectInstallRootContainingExecutable(
        extracted,
        executableName,
      );
      if (installRoot == null) {
        throw ApplyException(
          'DMG payload does not contain $executableName under ${extracted.path}',
        );
      }

      if (installRoot.path != extracted.path) {
        final isolated = Directory(
          '${parent.path}${Platform.pathSeparator}$baseName.dmg-app',
        );
        if (isolated.existsSync()) {
          isolated.deleteSync(recursive: true);
        }
        isolated.createSync(recursive: true);
        final stagedApp = Directory(
          _join(isolated.path, _basename(installRoot.path)),
        );
        final isolate = await Process.run(
          'ditto',
          [installRoot.path, stagedApp.path],
        );
        if (isolate.exitCode != 0) {
          final detail = isolate.stderr.toString().trim().isEmpty
              ? isolate.stdout
              : isolate.stderr;
          throw ApplyException(
            'ditto install root from DMG payload failed',
            cause: detail,
          );
        }
        stagedPayload = isolated;
        log(
          'DMG install root ${_basename(installRoot.path)} staged '
          '(matched $executableName)',
        );
      }
    }

    if (destination.existsSync()) {
      destination.deleteSync(recursive: true);
    }
    stagedPayload.renameSync(destination.path);
    if (stagedPayload.path == extracted.path) {
      log('DMG payload copied into ${destination.path}');
    } else {
      log('DMG install root ready at ${destination.path}');
    }
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
