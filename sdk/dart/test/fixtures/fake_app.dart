/// A stand-in application used by `apply_test.dart`.
///
/// Compiled to a real executable and installed into a real directory, so that
/// the file locking the apply mechanism exists to work around is genuinely
/// present rather than simulated.
///
/// It reports what it is and where it ran from, and on request replaces itself
/// with a newer copy.
library;

import 'dart:io';

import 'package:rup_client/rup_client.dart';

Future<void> main(List<String> args) async {
  // First statement, exactly as a host is meant to call it.
  await runApplyMode(args, renameTimeout: const Duration(seconds: 20));

  final here = File(Platform.resolvedExecutable).parent;
  final version =
      File('${here.path}${Platform.pathSeparator}version.txt').existsSync()
          ? File('${here.path}${Platform.pathSeparator}version.txt')
              .readAsStringSync()
              .trim()
          : 'unknown';

  final journal = File(Platform.environment['FAKE_APP_JOURNAL']!);
  journal.writeAsStringSync('started $version at ${here.path}\n',
      mode: FileMode.append);

  // Runtime state that an update must not destroy.
  final logs = Directory('${here.path}${Platform.pathSeparator}logs')
    ..createSync(recursive: true);
  File('${logs.path}${Platform.pathSeparator}run.log')
      .writeAsStringSync('$version ran\n', mode: FileMode.append);

  String? valueOf(String name) {
    final at = args.indexOf(name);
    return at < 0 || at + 1 >= args.length ? null : args[at + 1];
  }

  final packagePath = valueOf('--self-update');
  if (packagePath == null) {
    journal.writeAsStringSync('idle $version\n', mode: FileMode.append);
    return;
  }

  final staged = await stageUpdate(
    archive: File(packagePath),
    stagingRoot: Directory(valueOf('--staging')!),
    executableName: _executableName,
  );

  final sessionPath = valueOf('--apply-session');
  final timeoutSeconds = int.tryParse(valueOf('--apply-timeout') ?? '');

  await launchApply(
    staged: staged,
    installDir: here,
    executableName: _executableName,
    preserve: const ['logs'],
    logFile: File(valueOf('--apply-log')!),
    sessionFile: sessionPath == null ? null : File(sessionPath),
    renameTimeout:
        timeoutSeconds == null ? null : Duration(seconds: timeoutSeconds),
    targetCode: 2,
    targetVersion: 'v2',
  );

  journal.writeAsStringSync('exiting for update\n', mode: FileMode.append);
  exit(0);
}

String get _executableName => Platform.isWindows ? 'fake_app.exe' : 'fake_app';
