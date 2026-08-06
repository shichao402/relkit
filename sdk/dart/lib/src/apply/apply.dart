/// Self-update for an application shipped as a directory of files.
///
/// The problem this solves is specific to Windows and has no way around it: a
/// running executable, and every DLL it has loaded, cannot be deleted or
/// overwritten. An application therefore cannot replace itself. Something else
/// has to do it.
///
/// That "something else" here is the new version. The downloaded package is a
/// complete, working copy of the application, so it can be unpacked and run
/// from a scratch directory, and from there it is an ordinary program
/// replacing files it has no relationship to. This avoids shipping a separate
/// updater executable, and with it the question of what updates the updater.
///
/// The sequence:
///
///  1. the running application unpacks the verified package to a staging
///     directory outside the installation
///  2. it starts the staged copy with [applyFlag] and exits
///  3. the staged copy waits for the installation to be released, replaces it,
///     and starts the installed application again
///  4. the restarted application deletes the staging directory
///
/// Steps 1, 2 and 4 are the host's; call [stageUpdate], [launchApply] and
/// [cleanStagingArea]. Step 3 is [runApplyMode], which the host must call at
/// the very top of `main` -- see its documentation for why that matters.
library;

import 'dart:io';

import 'apply_exception.dart';
import 'session.dart';
import 'swap.dart';
import 'unpack.dart';

export 'apply_exception.dart' show ApplyException;
export 'session.dart';

/// The argument that tells a copy of the application it was started to replace
/// an installation rather than to be used.
const applyFlag = '--rup-apply';

/// A staged copy of a new version, ready to be started.
class StagedUpdate {
  StagedUpdate({required this.directory, required this.executable});

  /// The scratch directory holding the unpacked package.
  final Directory directory;

  /// The application executable inside [directory].
  final File executable;
}

/// Unpacks a verified package into [stagingRoot].
///
/// [archive] must already have been checked against the manifest: this does
/// not verify anything, and unpacking an unverified archive would hand an
/// attacker arbitrary file writes.
///
/// The outer file is a zip. On Windows it holds the portable tree directly.
/// On macOS it holds a single `.dmg`; that image is mounted and reduced to the
/// install root that contains [executableName] (so Applications / Gatekeeper
/// helpers on a manual-install DMG are ignored). A future Windows `.exe`
/// installer would extend the same unpack step.
///
/// [executableName] is the application executable's path within the package,
/// and is required rather than guessed: a package that does not contain it is
/// one this mechanism cannot apply, and finding that out here is much better
/// than finding out after the installation has been moved aside.
Future<StagedUpdate> stageUpdate({
  required File archive,
  required Directory stagingRoot,
  required String executableName,
  Duration clearTimeout = const Duration(seconds: 30),
  void Function(String message)? log,
}) async {
  final note = log ?? (_) {};

  await unpackUpdatePackage(
    archive: archive,
    stagingRoot: stagingRoot,
    executableName: executableName,
    clearTimeout: clearTimeout,
    log: note,
  );

  // Some packages wrap everything in a single top-level directory and some do
  // not. Look for the directory that actually contains [executableName],
  // preferring a `*.app` when several match (DMG helpers can leave siblings).
  var root = stagingRoot;
  var executable = File('${root.path}${Platform.pathSeparator}'
      '${executableName.replaceAll('/', Platform.pathSeparator)}');

  if (!executable.existsSync()) {
    final match = selectInstallRootContainingExecutable(
      stagingRoot,
      executableName,
    );
    if (match != null && match.path != stagingRoot.path) {
      root = match;
      executable = File('${root.path}${Platform.pathSeparator}'
          '${executableName.replaceAll('/', Platform.pathSeparator)}');
    }
  }

  if (!executable.existsSync()) {
    throw ApplyException(
        'the package does not contain $executableName, so it cannot be '
        'applied. Unpacked into ${stagingRoot.path}');
  }

  if (!Platform.isWindows) {
    // Zip files do not have to carry the executable bit, and an unpacked copy
    // that cannot be run would fail at the point of no return.
    await Process.run('chmod', ['+x', executable.path]);
    // Flutter macOS helpers live next to the main binary.
    final macosDir = executable.parent;
    if (macosDir.existsSync()) {
      await Process.run('chmod', ['-R', 'u+x', macosDir.path]);
    }
    // HTTP downloads may be quarantined; clear it so Process.start works.
    if (Platform.isMacOS) {
      await Process.run(
        'xattr',
        ['-dr', 'com.apple.quarantine', root.path],
      );
    }
  }

  return StagedUpdate(directory: root, executable: executable);
}

/// Starts the staged copy so it can replace [installDir], and returns.
///
/// The caller must exit promptly afterwards. The staged copy is already
/// waiting for the installation to be released, and every second the caller
/// stays alive is a second the update is stalled.
///
/// Deliberately not doing the exiting itself: a host with unsaved state, an
/// open database or a window to close has to be the one to decide how.
///
/// [sessionFile] is where the update announces itself to any other process
/// that starts while it runs -- see `session.dart`. It is written here rather
/// than by the staged copy because the staged copy takes a second or two to
/// boot, and a user who starts the application inside that window would
/// otherwise see nothing to warn them. It must live outside both [installDir]
/// and the staging area, since both are about to be replaced or deleted.
///
/// [renameTimeout] is how long the staged copy waits for the installation to
/// be released. It belongs to the host: how long an update may take before
/// giving up is a product decision, and the host is also the only side that
/// knows whether it has arranged for other copies to step aside.
Future<void> launchApply({
  required StagedUpdate staged,
  required Directory installDir,
  required String executableName,
  List<PreservedEntry> preserve = const [],
  File? logFile,
  File? sessionFile,
  Duration? renameTimeout,
  int? targetCode,
  String? targetVersion,
  bool relaunch = true,
}) async {
  final arguments = <String>[
    applyFlag,
    '--install-dir',
    installDir.absolute.path,
    '--staged-root',
    staged.directory.absolute.path,
    '--executable',
    executableName,
    if (preserve.isNotEmpty) ...['--preserve', preserve.join(',')],
    if (logFile != null) ...['--apply-log', logFile.absolute.path],
    if (sessionFile != null) ...['--apply-session', sessionFile.absolute.path],
    if (renameTimeout != null)
      ...['--apply-timeout', '${renameTimeout.inSeconds}'],
    if (!relaunch) '--no-relaunch',
  ];

  if (sessionFile != null) {
    final now = DateTime.now();
    writeApplySession(
      sessionFile,
      ApplySession(
        state: ApplyState.running,
        pid: pid,
        startedAt: now,
        updatedAt: now,
        installDir: installDir.absolute.path,
        stagedRoot: staged.directory.absolute.path,
        targetCode: targetCode,
        targetVersion: targetVersion,
      ),
    );
  }

  await Process.start(
    staged.executable.absolute.path,
    arguments,
    workingDirectory: staged.directory.absolute.path,
    // Detached, because the parent is about to exit and a child in the same
    // process group would be taken down with it on some shutdown paths.
    mode: ProcessStartMode.detached,
  );
}

/// What [applyFlag] was invoked with.
class ApplyRequest {
  ApplyRequest({
    required this.installDir,
    required this.stagedRoot,
    required this.executableName,
    required this.preserve,
    required this.relaunch,
    required this.logFile,
    this.sessionFile,
    this.renameTimeout,
  });

  final Directory installDir;

  /// Portable root of the staged copy (the directory that replaces
  /// [installDir]). On Windows this is usually the parent of the exe; on
  /// macOS it is the directory that contains `App.app`, not
  /// `App.app/Contents/MacOS`.
  final Directory stagedRoot;

  final String executableName;
  final List<PreservedEntry> preserve;
  final bool relaunch;
  final File? logFile;

  /// Where to report progress and failure so other processes can see it.
  final File? sessionFile;

  /// How long to wait for the installation to be released. Null means the
  /// default in [runApplyMode].
  final Duration? renameTimeout;

  /// Reads a request out of `main`'s arguments, or returns null when this is
  /// an ordinary start.
  static ApplyRequest? tryParse(List<String> args) {
    if (!args.contains(applyFlag)) return null;

    String? value(String name) {
      final at = args.indexOf(name);
      if (at < 0 || at + 1 >= args.length) return null;
      return args[at + 1];
    }

    final installDir = value('--install-dir');
    final executable = value('--executable');
    if (installDir == null || executable == null) {
      throw ApplyException(
          '$applyFlag needs --install-dir and --executable; got: '
          '${args.join(' ')}');
    }

    // Older hosts did not pass --staged-root; fall back to the executable's
    // parent, which is correct for flat Windows layouts.
    final stagedRoot = value('--staged-root') ??
        File(Platform.resolvedExecutable).parent.path;

    final preserve = value('--preserve');
    final logFile = value('--apply-log');
    final sessionFile = value('--apply-session');
    final timeoutSeconds = int.tryParse(value('--apply-timeout') ?? '');

    return ApplyRequest(
      installDir: Directory(installDir),
      stagedRoot: Directory(stagedRoot),
      executableName: executable,
      preserve: preserve == null || preserve.isEmpty
          ? const []
          : preserve.split(',').where((e) => e.isNotEmpty).toList(),
      relaunch: !args.contains('--no-relaunch'),
      logFile: logFile == null ? null : File(logFile),
      sessionFile: sessionFile == null ? null : File(sessionFile),
      renameTimeout: timeoutSeconds == null || timeoutSeconds <= 0
          ? null
          : Duration(seconds: timeoutSeconds),
    );
  }
}

/// Handles [applyFlag] if present, and otherwise returns and lets the
/// application start normally.
///
/// Call this as the first statement of `main`, before the UI framework is
/// initialised and before anything opens a file:
///
/// ```dart
/// void main(List<String> args) async {
///   await runApplyMode(args);
///   // ... normal startup
/// }
/// ```
///
/// **In apply mode this never returns: it ends the process.** That is not a
/// convenience. On Windows the native runner has already created the window
/// and entered its message loop by the time Dart `main` runs, so a `main` that
/// simply returns leaves a process spinning in that loop forever, holding a
/// window the user cannot see and never will -- the window is only shown when
/// the first frame renders, and no frame renders without `runApp`. Calling
/// `exit` is the only way out.
///
/// Never throws, for the same reason it never returns: a self-update that
/// crashes leaves the user with no application and no explanation. Failures go
/// to the log file and the exit code.
Future<void> runApplyMode(
  List<String> args, {
  Duration renameTimeout = const Duration(seconds: 60),
}) async {
  final ApplyRequest? request;
  try {
    request = ApplyRequest.tryParse(args);
  } on ApplyException catch (error) {
    stderr.writeln(error);
    exit(2);
  }
  if (request == null) return;

  final messages = <String>[];
  void note(String message) {
    final line = '${DateTime.now().toIso8601String()} $message';
    messages.add(line);
    request!.logFile?.parent.createSync(recursive: true);
    try {
      request.logFile?.writeAsStringSync('$line\n', mode: FileMode.append);
    } on FileSystemException {
      // The log is a convenience. Losing it must not stop the update.
    }
  }

  final sessionFile = request.sessionFile;
  final startedAt = DateTime.now();

  ApplySession session = ApplySession(
    state: ApplyState.running,
    pid: pid,
    startedAt: startedAt,
    updatedAt: startedAt,
    installDir: request.installDir.absolute.path,
    stagedRoot: request.stagedRoot.absolute.path,
  );
  // The host wrote the record before starting this process; keep whatever it
  // knew that this process does not, such as which version is being installed.
  if (sessionFile != null) {
    final handed = readApplySession(sessionFile);
    if (handed != null) {
      session = ApplySession(
        state: ApplyState.running,
        pid: pid,
        startedAt: handed.startedAt,
        updatedAt: startedAt,
        installDir: session.installDir,
        stagedRoot: session.stagedRoot,
        targetCode: handed.targetCode,
        targetVersion: handed.targetVersion,
      );
    }
    writeApplySession(sessionFile, session);
  }

  var lastBeat = startedAt;
  void beat() {
    if (sessionFile == null) return;
    final now = DateTime.now();
    if (now.difference(lastBeat) < heartbeatInterval) return;
    lastBeat = now;
    session = session.copyWith(updatedAt: now);
    writeApplySession(sessionFile, session);
  }

  try {
    final stagedDir = request.stagedRoot;

    // If this is somehow the installed copy, the swap would delete the ground
    // it is standing on. That should be impossible, which is exactly why it is
    // worth checking.
    if (_samePath(stagedDir, request.installDir)) {
      throw ApplyException(
          'refusing to replace ${request.installDir.path} from a copy running '
          'inside it');
    }

    note('replacing ${request.installDir.path} from ${stagedDir.path}');

    await swapInstallation(
      installDir: request.installDir,
      stagedDir: stagedDir,
      preserve: request.preserve,
      keepStaged: true,
      renameTimeout: request.renameTimeout ?? renameTimeout,
      onWaiting: (_, __) => beat(),
      log: note,
    );

    note('replaced');

    if (sessionFile != null) {
      writeApplySession(
        sessionFile,
        session.copyWith(
          state: ApplyState.succeeded,
          updatedAt: DateTime.now(),
        ),
      );
    }

    if (request.relaunch) {
      final installed = File('${request.installDir.path}'
          '${Platform.pathSeparator}'
          '${request.executableName.replaceAll('/', Platform.pathSeparator)}');
      note('starting ${installed.path}');
      await Process.start(
        installed.path,
        const [],
        workingDirectory: request.installDir.path,
        mode: ProcessStartMode.detached,
      );
    }

    exit(0);
  } on Object catch (error, stack) {
    note('FAILED: $error');
    note(stack.toString());

    // Without this the only trace of a failed update is a log file nobody
    // opens: this process has no window, and the application it was updating
    // is not running. The next start reads it and can finally say what
    // happened.
    if (sessionFile != null) {
      writeApplySession(
        sessionFile,
        session.copyWith(
          state: ApplyState.failed,
          updatedAt: DateTime.now(),
          message: _describe(error),
          needsAttention: error is SwapException && !error.rolledBack,
        ),
      );
    }

    stderr.writeln(error);
    exit(1);
  }
}

String _describe(Object error) {
  if (error is SwapException) return error.message;
  if (error is ApplyException) return error.message;
  return error.toString();
}

/// Deletes a staging directory left behind by a completed update.
///
/// Call on startup. Failure is not worth reporting: the space is reclaimed by
/// the next update, which clears the directory before using it.
///
/// [keep] names entries directly under [stagingRoot] to leave alone. Its
/// purpose is the verified package: it is the expensive part of an update, it
/// is already known to match the signed manifest, and throwing it away means a
/// user whose update failed pays for the download a second time. A host that
/// keeps it must also decide when to stop -- see the caller.
///
/// **Do not call this while an apply is running.** The staged copy is
/// executing from inside this directory, so deleting it either fails outright
/// or removes files the update still needs. `session.dart` is how a host that
/// might have just started during an update finds out.
Future<void> cleanStagingArea(
  Directory stagingRoot, {
  List<String> keep = const [],
}) async {
  if (!stagingRoot.existsSync()) return;

  if (keep.isEmpty) {
    try {
      stagingRoot.deleteSync(recursive: true);
    } on FileSystemException {
      // Still locked, or gone already. Either way there is nothing to do.
    }
    return;
  }

  final kept = keep.map(_normaliseName).toSet();
  for (final entry in stagingRoot.listSync(followLinks: false)) {
    final name = entry.path.split(Platform.pathSeparator).last;
    if (kept.contains(_normaliseName(name))) continue;
    try {
      entry.deleteSync(recursive: true);
    } on FileSystemException {
      // Same as above: the next update clears what is in its way.
    }
  }
}

String _normaliseName(String name) =>
    Platform.isWindows ? name.toLowerCase() : name;

bool _samePath(Directory a, Directory b) {
  String normalise(Directory dir) {
    var path = dir.absolute.path.replaceAll('/', Platform.pathSeparator);
    while (path.length > 1 && path.endsWith(Platform.pathSeparator)) {
      path = path.substring(0, path.length - 1);
    }
    return Platform.isWindows ? path.toLowerCase() : path;
  }

  return normalise(a) == normalise(b);
}
