/// End-to-end test of an application replacing itself.
///
/// Everything here is real: a compiled executable, installed in a directory,
/// started as a process, which unpacks a package and hands over to it. Nothing
/// about the Windows locking behaviour this mechanism exists for survives being
/// mocked -- a fake that lets you overwrite a running executable would make
/// the broken version of this code pass.
library;

import 'dart:io';

import 'package:archive/archive_io.dart';
import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

/// Compiling takes a while, so it happens once and every test reuses the
/// result.
late final File compiledApp;

void main() {
  late Directory root;
  late Directory install;
  late File journal;
  late File applyLog;

  final exeName = Platform.isWindows ? 'fake_app.exe' : 'fake_app';

  setUpAll(() async {
    final build = Directory.systemTemp.createTempSync('rup-apply-build-');
    addTearDown(() {
      try {
        build.deleteSync(recursive: true);
      } on FileSystemException {
        // Windows sometimes holds the binary briefly after the last run.
      }
    });

    final output = '${build.path}${Platform.pathSeparator}$exeName';
    final result = await Process.run(
      Platform.resolvedExecutable,
      ['compile', 'exe', 'test/fixtures/fake_app.dart', '-o', output],
    );
    if (result.exitCode != 0) {
      fail('could not compile the test application\n'
          '${result.stdout}\n${result.stderr}');
    }
    compiledApp = File(output);
  });

  setUp(() {
    root = Directory.systemTemp.createTempSync('rup-apply-');
    install = Directory('${root.path}${Platform.pathSeparator}app')
      ..createSync();
    journal = File('${root.path}${Platform.pathSeparator}journal.txt')
      ..writeAsStringSync('');
    applyLog = File('${root.path}${Platform.pathSeparator}apply.log');
  });

  tearDown(() {
    try {
      if (root.existsSync()) root.deleteSync(recursive: true);
    } on FileSystemException {
      // A relaunched copy may still be shutting down.
    }
  });

  /// Lays out an installation: the executable, a version marker, and a file
  /// that only that version ships.
  void installVersion(Directory dir, String version) {
    dir.createSync(recursive: true);
    compiledApp.copySync('${dir.path}${Platform.pathSeparator}$exeName');
    File('${dir.path}${Platform.pathSeparator}version.txt')
        .writeAsStringSync(version);
    File('${dir.path}${Platform.pathSeparator}$version-only.txt')
        .writeAsStringSync('shipped with $version');
  }

  /// Zips [tree] into [zipPath]. Synchronous throughout: the async encoder
  /// methods do not compose with `closeSync`, and closing while a write is
  /// still in flight produces a truncated archive rather than an error.
  File zipOf(Directory tree, String zipPath) {
    ZipFileEncoder()
      ..create(zipPath)
      ..addDirectorySync(tree, includeDirName: false)
      ..closeSync();
    return File(zipPath);
  }

  /// Builds a package of [version], the way a release would.
  File packageOf(String version) {
    final tree =
        Directory('${root.path}${Platform.pathSeparator}build-$version')
          ..createSync();
    installVersion(tree, version);

    return zipOf(tree, '${root.path}${Platform.pathSeparator}$version.zip');
  }

  Future<ProcessResult> run(Directory dir, List<String> args) => Process.run(
        '${dir.path}${Platform.pathSeparator}$exeName',
        args,
        environment: {'FAKE_APP_JOURNAL': journal.path},
      );

  /// Waits for the relaunched copy to record that it started.
  Future<String> waitForJournal(String needle,
      {Duration timeout = const Duration(seconds: 30)}) async {
    final deadline = DateTime.now().add(timeout);
    while (DateTime.now().isBefore(deadline)) {
      final text = journal.readAsStringSync();
      if (text.contains(needle)) return text;
      await Future<void>.delayed(const Duration(milliseconds: 100));
    }
    fail('timed out waiting for "$needle".\n'
        'journal:\n${journal.readAsStringSync()}\n'
        'apply log:\n'
        '${applyLog.existsSync() ? applyLog.readAsStringSync() : '(none)'}');
  }

  test('the running application is replaced and restarted by its successor',
      () async {
    installVersion(install, 'v1');
    final package = packageOf('v2');

    // Runtime state from before the update. This is the thing a naive
    // directory swap destroys.
    final logs = Directory('${install.path}${Platform.pathSeparator}logs')
      ..createSync();
    File('${logs.path}${Platform.pathSeparator}history.log')
        .writeAsStringSync('from v1\n');

    final first = await run(install, [
      '--self-update',
      package.path,
      '--staging',
      '${root.path}${Platform.pathSeparator}staging',
      '--apply-log',
      applyLog.path,
    ]);
    expect(first.exitCode, 0, reason: '${first.stdout}\n${first.stderr}');

    await waitForJournal('started v2');

    expect(
        File('${install.path}${Platform.pathSeparator}version.txt')
            .readAsStringSync(),
        'v2');
    expect(
        File('${install.path}${Platform.pathSeparator}v2-only.txt')
            .existsSync(),
        isTrue);
    expect(
        File('${install.path}${Platform.pathSeparator}v1-only.txt')
            .existsSync(),
        isFalse,
        reason: 'the old version\'s files should be gone, not merged');

    expect(
        File('${logs.path}${Platform.pathSeparator}history.log')
            .readAsStringSync(),
        contains('from v1'),
        reason: 'preserved runtime state must survive the replacement');

    expect(Directory('${install.path}.rup-old').existsSync(), isFalse);

    // And the replaced installation is a working application, not just the
    // right bytes on disk.
    final after = await run(install, []);
    expect(after.exitCode, 0);
    expect(journal.readAsStringSync(), contains('idle v2'));
  }, timeout: const Timeout(Duration(minutes: 2)));

  test('the staging directory holds a complete, runnable copy', () async {
    // The mechanism depends on this: the staged copy has to be able to run,
    // because it is the process that performs the replacement.
    installVersion(install, 'v1');
    final package = packageOf('v2');
    final staging = Directory('${root.path}${Platform.pathSeparator}staging');

    await run(install, [
      '--self-update',
      package.path,
      '--staging',
      staging.path,
      '--apply-log',
      applyLog.path,
    ]);
    await waitForJournal('started v2');

    final startedFromStaging = journal
        .readAsStringSync()
        .split('\n')
        .any((line) => line.contains(staging.path));
    expect(startedFromStaging, isFalse,
        reason: 'the staged copy applies the update; it must not start the UI');
  }, timeout: const Timeout(Duration(minutes: 2)));

  test('a package without the executable is refused before anything moves',
      () async {
    installVersion(install, 'v1');

    final empty = Directory('${root.path}${Platform.pathSeparator}empty')
      ..createSync();
    File('${empty.path}${Platform.pathSeparator}readme.txt')
        .writeAsStringSync('no executable here');
    final broken =
        zipOf(empty, '${root.path}${Platform.pathSeparator}broken.zip');

    final result = await run(install, [
      '--self-update',
      broken.path,
      '--staging',
      '${root.path}${Platform.pathSeparator}staging',
      '--apply-log',
      applyLog.path,
    ]);

    expect(result.exitCode, isNot(0));
    expect(result.stderr.toString(), contains(exeName));
    expect(
        File('${install.path}${Platform.pathSeparator}version.txt')
            .readAsStringSync(),
        'v1',
        reason: 'a package that cannot be applied must not disturb anything');
  }, timeout: const Timeout(Duration(minutes: 2)));

  test('an update in flight is announced, and says so when it lands', () async {
    // The session file is the only way a process that starts during an update
    // can find out that one is happening. Without it the user, seeing the
    // application vanish, starts it again and locks the update out for good.
    installVersion(install, 'v1');
    final package = packageOf('v2');
    final session = File('${root.path}${Platform.pathSeparator}session.json');

    await run(install, [
      '--self-update',
      package.path,
      '--staging',
      '${root.path}${Platform.pathSeparator}staging',
      '--apply-log',
      applyLog.path,
      '--apply-session',
      session.path,
    ]);

    await waitForJournal('started v2');

    final record = readApplySession(session);
    expect(record, isNotNull, reason: 'the outcome has to outlive the apply');
    expect(record!.state, ApplyState.succeeded);
    expect(record.targetVersion, 'v2',
        reason: 'the host writes which version it is installing, and the '
            'applying process must not lose it');
  }, timeout: const Timeout(Duration(minutes: 2)));

  test('a failed apply leaves behind why, not just a log nobody opens',
      () async {
    // A blocked update has no window and no application to report through.
    // If it does not write down what happened, the next start cannot tell the
    // user anything at all -- which is exactly the case that sent someone
    // hunting through log files by hand.
    installVersion(install, 'v1');
    final package = packageOf('v2');
    final session = File('${root.path}${Platform.pathSeparator}session.json');

    final held = File('${install.path}${Platform.pathSeparator}version.txt')
        .openSync(mode: FileMode.append);
    addTearDown(held.closeSync);

    await run(install, [
      '--self-update',
      package.path,
      '--staging',
      '${root.path}${Platform.pathSeparator}staging',
      '--apply-log',
      applyLog.path,
      '--apply-session',
      session.path,
      '--apply-timeout',
      '2',
    ]);

    ApplySession? record;
    final deadline = DateTime.now().add(const Duration(seconds: 30));
    while (DateTime.now().isBefore(deadline)) {
      record = readApplySession(session);
      if (record != null && record.state != ApplyState.running) break;
      await Future<void>.delayed(const Duration(milliseconds: 100));
    }

    expect(record, isNotNull);
    expect(record!.state, ApplyState.failed);
    expect(record.message, contains('still in use'));
    expect(record.needsAttention, isFalse,
        reason: 'a blocked rename changes nothing, so a retry is all it needs');
    expect(
        File('${install.path}${Platform.pathSeparator}version.txt')
            .readAsStringSync(),
        'v1');
  },
      timeout: const Timeout(Duration(minutes: 2)),
      skip: Platform.isWindows ? null : 'needs mandatory file locking');

  group('cleaning up the staging area', () {
    test('keeps the verified package so a retry does not download it again',
        () async {
      final staging = Directory('${root.path}${Platform.pathSeparator}staging')
        ..createSync();
      final download = Directory(
          '${staging.path}${Platform.pathSeparator}download')
        ..createSync();
      final zip = File('${download.path}${Platform.pathSeparator}v2.zip')
        ..writeAsStringSync('verified bytes');
      final unpacked =
          Directory('${staging.path}${Platform.pathSeparator}unpacked')
            ..createSync();
      File('${unpacked.path}${Platform.pathSeparator}app.exe')
          .writeAsStringSync('unpacked');

      await cleanStagingArea(staging, keep: const ['download']);

      expect(zip.readAsStringSync(), 'verified bytes');
      expect(
          Directory('${staging.path}${Platform.pathSeparator}unpacked')
              .existsSync(),
          isFalse);
    });

    test('removes everything once nothing needs keeping', () async {
      final staging = Directory('${root.path}${Platform.pathSeparator}staging')
        ..createSync();
      final download =
          Directory('${staging.path}${Platform.pathSeparator}download')
            ..createSync();
      File('${download.path}${Platform.pathSeparator}v2.zip')
          .writeAsStringSync('done with this');

      await cleanStagingArea(staging);

      expect(staging.existsSync(), isFalse);
    });
  });

  test('a copy running inside the installation refuses to replace it',
      () async {
    // Belt and braces: this should be unreachable, and if it ever happens the
    // swap would delete the running process out from under itself.
    installVersion(install, 'v1');

    final result = await run(install, [
      '--rup-apply',
      '--install-dir',
      install.path,
      '--executable',
      exeName,
      '--apply-log',
      applyLog.path,
    ]);

    expect(result.exitCode, isNot(0));
    expect(
        File('${install.path}${Platform.pathSeparator}version.txt')
            .readAsStringSync(),
        'v1');
  }, timeout: const Timeout(Duration(minutes: 2)));
}
