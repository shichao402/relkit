/// Replacing an installation directory with a new one, reversibly.
///
/// This is the half of "apply" that touches the disk, kept free of process
/// handling so it can be tested for real: every failure path below is
/// reachable in a test, which is the only way to have any confidence in code
/// whose failure mode is an application that no longer starts.
///
/// The invariant, in one sentence: at every point where this code can fail,
/// either the old installation is still in place or it can be put back.
///
/// It must not run inside the application being replaced. See `apply.dart`.
library;

import 'dart:io';

/// A directory kept from the old installation instead of taking the new
/// package's copy.
///
/// Two kinds of things qualify, and both are things the package cannot supply:
/// data the application wrote (logs), and data that is expensive to recreate
/// (an unpacked toolchain). Anything the package ships a real version of must
/// not be listed here, or the update will silently keep shipping the old one.
typedef PreservedEntry = String;

class SwapException implements Exception {
  SwapException(this.message, {this.rolledBack = true, this.cause});

  final String message;

  /// Whether the old installation is back in place. False here means the
  /// installation is in pieces and needs a human -- the message says where the
  /// old copy is.
  final bool rolledBack;

  final Object? cause;

  @override
  String toString() => 'SwapException: $message'
      '${rolledBack ? '' : ' (INSTALLATION LEFT INCOMPLETE)'}'
      '${cause == null ? '' : '\n  caused by: $cause'}';
}

/// Replaces [installDir] with [stagedDir].
///
/// On success the old installation is deleted, except for [preserve] entries,
/// which are moved across.
///
/// Set [keepStaged] when the caller is running from [stagedDir] -- which is
/// the normal case for a self-update, since the staged copy is the only
/// complete application available while the installed one is being replaced.
/// The staged tree is then copied instead of moved, and the caller is
/// responsible for deleting it later, from a process that is not inside it.
///
/// [placeStaged] overrides how the tree is put in place, and overrides
/// [keepStaged] with it. Its purpose is to let tests exercise the rollback
/// below, which is otherwise only reachable when the disk fills up or a
/// permission is wrong -- neither of which can be arranged reliably, and both
/// of which produce a broken installation if this code is wrong.
///
/// Throws [SwapException] on any failure. Unless the exception says otherwise,
/// the old installation is untouched or restored.
/// How the staged tree gets to its destination. See [swapInstallation].
typedef PlaceStaged = Future<void> Function(Directory from, Directory to);

/// Called on every failed attempt to take the installation directory.
///
/// The wait is the only part of a swap that takes long enough for anyone to
/// notice, so it is also the only part that has to be observable from outside
/// this process. Callers use it to keep a heartbeat fresh.
typedef WaitingForInstall = void Function(Duration waited, int attempts);

Future<void> swapInstallation({
  required Directory installDir,
  required Directory stagedDir,
  List<PreservedEntry> preserve = const [],
  bool keepStaged = false,
  Duration renameTimeout = const Duration(seconds: 60),
  PlaceStaged? placeStaged,
  WaitingForInstall? onWaiting,
  void Function(String message)? log,
}) async {
  final note = log ?? (_) {};

  if (!installDir.existsSync()) {
    throw SwapException('installation directory does not exist: '
        '${installDir.path}');
  }
  if (!stagedDir.existsSync()) {
    throw SwapException('staged directory does not exist: ${stagedDir.path}');
  }

  final retired = Directory('${installDir.path}.rup-old');

  // A leftover from a previous run would make the rename below fail for a
  // reason that has nothing to do with this update.
  if (retired.existsSync()) {
    note('removing a leftover ${retired.path}');
    try {
      retired.deleteSync(recursive: true);
    } on FileSystemException catch (error) {
      throw SwapException(
          'could not remove a leftover directory from an earlier update: '
          '${retired.path}',
          cause: error);
    }
  }

  // Step one, and the reason it comes first: on Windows this rename fails
  // while any file underneath is open, which is exactly the condition that
  // makes the rest unsafe. Checking for a live process would be a guess about
  // the same question; this asks the filesystem directly, and also catches a
  // file held open by something else entirely -- a virus scanner, an explorer
  // preview, a second copy of the app.
  //
  // Nothing has changed on disk if it fails, so failing here is free.
  await _renameWithRetry(
    installDir,
    retired,
    timeout: renameTimeout,
    onWaiting: onWaiting,
    log: note,
  );

  // From here a failure has to be undone.
  try {
    if (placeStaged != null) {
      await placeStaged(stagedDir, installDir);
    } else if (keepStaged) {
      note('copying ${stagedDir.path} into place');
      await _copyDirectory(stagedDir, installDir);
    } else {
      note('moving ${stagedDir.path} into place');
      await _moveDirectory(stagedDir, installDir);
    }
  } on Object catch (error) {
    await _restore(retired, installDir, error);
    throw SwapException('could not move the new version into place',
        cause: error);
  }

  // Preserved entries come last: the new installation is already complete and
  // startable without them, so a failure here is worth reporting but not worth
  // undoing a good update for.
  final carryFailures = <String>[];
  for (final entry in preserve) {
    try {
      await _carryOver(entry, from: retired, to: installDir, log: note);
    } on Object catch (error) {
      carryFailures.add('$entry ($error)');
    }
  }

  try {
    retired.deleteSync(recursive: true);
  } on FileSystemException catch (error) {
    // Harmless: it is out of the way and the next update removes it.
    note('could not delete ${retired.path}, leaving it for later: $error');
  }

  if (carryFailures.isNotEmpty) {
    throw SwapException(
        'the update was applied, but these were not carried over from the old '
        'installation: ${carryFailures.join(', ')}',
        rolledBack: false);
  }
}

/// Puts the old installation back, turning a failed update into a no-op.
Future<void> _restore(
    Directory retired, Directory installDir, Object original) async {
  // A partial move can leave a stump where the installation was; it has to go
  // before the old one can come back.
  if (installDir.existsSync()) {
    try {
      installDir.deleteSync(recursive: true);
    } on FileSystemException catch (error) {
      throw SwapException(
          'the update failed and the partial installation could not be '
          'removed. The previous version is intact at ${retired.path} and can '
          'be renamed back to ${installDir.path} by hand',
          rolledBack: false,
          cause: error);
    }
  }

  try {
    retired.renameSync(installDir.path);
  } on FileSystemException catch (error) {
    throw SwapException(
        'the update failed and the previous version could not be restored. It '
        'is intact at ${retired.path} and can be renamed back to '
        '${installDir.path} by hand',
        rolledBack: false,
        cause: error);
  }
}

/// Moves one preserved entry from the old installation to the new one.
///
/// The old copy wins: these are entries the package cannot meaningfully
/// provide, so whatever the package shipped (usually an empty placeholder) is
/// discarded.
Future<void> _carryOver(
  String entry, {
  required Directory from,
  required Directory to,
  required void Function(String) log,
}) async {
  final source = Directory('${from.path}${Platform.pathSeparator}$entry');
  final sourceFile = File('${from.path}${Platform.pathSeparator}$entry');
  final destinationPath = '${to.path}${Platform.pathSeparator}$entry';

  final isDirectory = source.existsSync();
  final isFile = !isDirectory && sourceFile.existsSync();
  if (!isDirectory && !isFile) return;

  final existingDir = Directory(destinationPath);
  final existingFile = File(destinationPath);
  if (existingDir.existsSync()) {
    existingDir.deleteSync(recursive: true);
  } else if (existingFile.existsSync()) {
    existingFile.deleteSync();
  }

  log('carrying over $entry');
  if (isDirectory) {
    await _moveDirectory(source, Directory(destinationPath));
  } else {
    await _moveFile(sourceFile, File(destinationPath));
  }
}

/// Renames [from] to [to], retrying while the filesystem says something is
/// still using it.
///
/// The retry is the mechanism that makes an in-place self-update work at all:
/// the process that asked for the update is still shutting down when this
/// starts, and how long that takes is not knowable in advance.
Future<void> _renameWithRetry(
  Directory from,
  Directory to, {
  required Duration timeout,
  WaitingForInstall? onWaiting,
  required void Function(String) log,
}) async {
  final startedAt = DateTime.now();
  final deadline = startedAt.add(timeout);
  var delay = const Duration(milliseconds: 50);
  var attempts = 0;
  // Logging every attempt would bury the rest of the update log in hundreds of
  // identical lines, and logging none is how a wait that should have taken
  // milliseconds went eight seconds without leaving a trace of who was
  // holding the directory. One line per interval keeps both the cause and the
  // duration.
  var nextReport = const Duration(seconds: 2);
  Object? last;

  while (true) {
    attempts++;
    try {
      from.renameSync(to.path);
      if (attempts > 1) {
        final waited = DateTime.now().difference(startedAt);
        log('the installation became free after $attempts attempts '
            '(${waited.inMilliseconds}ms)');
      }
      return;
    } on FileSystemException catch (error) {
      last = error;
      final waited = DateTime.now().difference(startedAt);

      if (waited >= nextReport) {
        log('still waiting for ${from.path} after ${waited.inSeconds}s '
            '($attempts attempts): $error');
        nextReport += const Duration(seconds: 5);
      }

      // The caller uses this to tell the rest of the world it is still alive,
      // which is what stops a user from starting the application again into
      // the middle of an update.
      onWaiting?.call(waited, attempts);

      if (!DateTime.now().isBefore(deadline)) break;
      await Future<void>.delayed(delay);
      if (delay < const Duration(milliseconds: 500)) {
        delay *= 2;
      }
    }
  }

  throw SwapException(
      'the installation at ${from.path} is still in use after '
      '${timeout.inSeconds}s, so it cannot be replaced. Close every copy of '
      'the application and try again',
      cause: last);
}

/// Moves a directory, falling back to copy-and-delete across volumes.
///
/// A rename is atomic and instant, and it is what happens whenever staging and
/// installation share a volume. Across volumes the operating system refuses,
/// and the copy is the only option.
Future<void> _moveDirectory(Directory from, Directory to) async {
  try {
    from.renameSync(to.path);
    return;
  } on FileSystemException {
    // Fall through: almost certainly a cross-volume move.
  }

  await _copyDirectory(from, to);
  from.deleteSync(recursive: true);
}

Future<void> _moveFile(File from, File to) async {
  try {
    from.renameSync(to.path);
    return;
  } on FileSystemException {
    await from.copy(to.path);
    from.deleteSync();
  }
}

Future<void> _copyDirectory(Directory from, Directory to) async {
  to.createSync(recursive: true);

  await for (final entity in from.list(followLinks: false)) {
    final name =
        entity.uri.pathSegments.lastWhere((segment) => segment.isNotEmpty);
    final target = '${to.path}${Platform.pathSeparator}$name';

    if (entity is Directory) {
      await _copyDirectory(entity, Directory(target));
    } else if (entity is File) {
      await entity.copy(target);
    } else if (entity is Link) {
      Link(target).createSync(entity.targetSync());
    }
  }
}
