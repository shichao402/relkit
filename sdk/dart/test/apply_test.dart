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
