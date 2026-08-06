import 'dart:io';

import 'package:archive/archive_io.dart';
import 'package:rup_client/src/apply/apply_exception.dart';
import 'package:rup_client/src/apply/unpack.dart';
import 'package:test/test.dart';

void main() {
  late Directory root;

  setUp(() {
    root = Directory.systemTemp.createTempSync('rup-unpack-');
  });

  tearDown(() {
    try {
      if (root.existsSync()) root.deleteSync(recursive: true);
    } on FileSystemException {
      // A test that deliberately holds a handle must not fail the next run.
    }
  });

  test('expandInnerInstallerIfPresent is a no-op without a dmg', () async {
    final payload = Directory('${root.path}${Platform.pathSeparator}stage')
      ..createSync();
    File('${payload.path}${Platform.pathSeparator}app.bin')
        .writeAsStringSync('ok');

    await expandInnerInstallerIfPresent(payload);

    expect(
      File('${payload.path}${Platform.pathSeparator}app.bin').readAsStringSync(),
      'ok',
    );
  });

  test('expandInnerInstallerIfPresent rejects multiple dmgs', () async {
    final payload = Directory('${root.path}${Platform.pathSeparator}stage')
      ..createSync();
    File('${payload.path}${Platform.pathSeparator}a.dmg').writeAsBytesSync(const []);
    File('${payload.path}${Platform.pathSeparator}b.dmg').writeAsBytesSync(const []);

    await expectLater(
      expandInnerInstallerIfPresent(payload),
      throwsA(isA<ApplyException>()),
    );
  });

  test('expandInnerInstallerIfPresent rejects dmg on non-macOS', () async {
    final payload = Directory('${root.path}${Platform.pathSeparator}stage')
      ..createSync();
    File('${payload.path}${Platform.pathSeparator}App.dmg')
        .writeAsBytesSync(const []);

    await expectLater(
      expandInnerInstallerIfPresent(payload),
      throwsA(isA<ApplyException>()),
    );
  }, skip: Platform.isMacOS);

  test('selectInstallRootContainingExecutable prefers .app among helpers', () {
    final payload = Directory('${root.path}${Platform.pathSeparator}payload')
      ..createSync();
    Link('${payload.path}${Platform.pathSeparator}Applications')
        .createSync('/Applications');

    final helper = Directory(
      '${payload.path}${Platform.pathSeparator}OpenPrivacy.app'
      '${Platform.pathSeparator}Contents${Platform.pathSeparator}MacOS',
    )..createSync(recursive: true);
    File('${helper.path}${Platform.pathSeparator}applet').writeAsStringSync('x');

    final mainMacos = Directory(
      '${payload.path}${Platform.pathSeparator}Product.app'
      '${Platform.pathSeparator}Contents${Platform.pathSeparator}MacOS',
    )..createSync(recursive: true);
    File('${mainMacos.path}${Platform.pathSeparator}Product')
        .writeAsStringSync('bin');

    final selected = selectInstallRootContainingExecutable(
      payload,
      'Contents/MacOS/Product',
    );
    expect(selected, isNotNull);
    expect(selected!.path.endsWith('Product.app'), isTrue);
  });

  test('selectInstallRootContainingExecutable returns null when missing', () {
    final payload = Directory('${root.path}${Platform.pathSeparator}payload')
      ..createSync();
    final helper = Directory(
      '${payload.path}${Platform.pathSeparator}helper.app'
      '${Platform.pathSeparator}Contents${Platform.pathSeparator}MacOS',
    )..createSync(recursive: true);
    File('${helper.path}${Platform.pathSeparator}applet').writeAsStringSync('x');

    expect(
      selectInstallRootContainingExecutable(
        payload,
        'Contents/MacOS/Missing',
      ),
      isNull,
    );
  });

  test('a staging directory in use is reported in terms a user can act on',
      () async {
    // What actually holds it: the previous apply attempt, which runs from
    // inside this directory and has not exited yet. The bare filesystem error
    // for that is "Deletion failed, errno = 5", which tells the user nothing.
    final staging = Directory('${root.path}${Platform.pathSeparator}unpacked')
      ..createSync();
    final locked = File('${staging.path}${Platform.pathSeparator}app.exe')
      ..writeAsStringSync('running');
    final held = locked.openSync(mode: FileMode.append);
    addTearDown(held.closeSync);

    final archive = File('${root.path}${Platform.pathSeparator}update.zip')
      ..writeAsBytesSync(const []);

    await expectLater(
      unpackUpdatePackage(
        archive: archive,
        stagingRoot: staging,
        clearTimeout: const Duration(seconds: 1),
      ),
      throwsA(isA<ApplyException>().having(
        (e) => e.message,
        'message',
        allOf(contains('staging directory'), contains('still using it')),
      )),
    );
  }, skip: Platform.isWindows ? null : 'needs mandatory file locking');

  test('a staging directory left by a finished update is simply replaced',
      () async {
    final staging = Directory('${root.path}${Platform.pathSeparator}unpacked')
      ..createSync();
    File('${staging.path}${Platform.pathSeparator}stale.txt')
        .writeAsStringSync('from the last attempt');

    final payload = Directory('${root.path}${Platform.pathSeparator}payload')
      ..createSync();
    File('${payload.path}${Platform.pathSeparator}payload.txt')
        .writeAsStringSync('v2');

    final archive = File('${root.path}${Platform.pathSeparator}update.zip');
    ZipFileEncoder()
      ..create(archive.path)
      ..addDirectorySync(payload, includeDirName: false)
      ..closeSync();

    await unpackUpdatePackage(archive: archive, stagingRoot: staging);

    expect(File('${staging.path}${Platform.pathSeparator}stale.txt').existsSync(),
        isFalse,
        reason: 'leftovers from a previous attempt must not survive');
    expect(
        File('${staging.path}${Platform.pathSeparator}payload.txt')
            .readAsStringSync(),
        'v2');
  });
}
