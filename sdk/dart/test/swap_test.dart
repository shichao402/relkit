/// Tests for replacing an installation directory.
///
/// Weighted towards failure: the success path is visible the first time anyone
/// runs an update, while the failure paths are what decide whether a bad
/// update costs a retry or costs the application.
library;

import 'dart:io';

import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

void main() {
  late Directory root;
  late Directory install;
  late Directory staged;

  setUp(() {
    root = Directory.systemTemp.createTempSync('rup-swap-');
    install = Directory('${root.path}${Platform.pathSeparator}app')
      ..createSync();
    staged = Directory('${root.path}${Platform.pathSeparator}staged')
      ..createSync();
  });

  tearDown(() {
    // A test that leaves a locked handle behind would otherwise fail the next
    // run instead of itself.
    try {
      if (root.existsSync()) root.deleteSync(recursive: true);
    } on FileSystemException {
      // Nothing useful to do; the temp directory gets cleaned up eventually.
    }
  });

  void write(Directory dir, String relative, String contents) {
    final file = File('${dir.path}${Platform.pathSeparator}'
        '${relative.replaceAll('/', Platform.pathSeparator)}');
    file.parent.createSync(recursive: true);
    file.writeAsStringSync(contents);
  }

  String? read(Directory dir, String relative) {
    final file = File('${dir.path}${Platform.pathSeparator}'
        '${relative.replaceAll('/', Platform.pathSeparator)}');
    return file.existsSync() ? file.readAsStringSync() : null;
  }

  Directory retiredDir() => Directory('${install.path}.rup-old');

  group('the ordinary case', () {
    test('replaces every file and removes ones the new version dropped',
        () async {
      write(install, 'app.exe', 'v1');
      write(install, 'data/assets.bin', 'old assets');
      write(install, 'gone.dll', 'removed in v2');

      write(staged, 'app.exe', 'v2');
      write(staged, 'data/assets.bin', 'new assets');
      write(staged, 'added.dll', 'new in v2');

      await swapInstallation(installDir: install, stagedDir: staged);

      expect(read(install, 'app.exe'), 'v2');
      expect(read(install, 'data/assets.bin'), 'new assets');
      expect(read(install, 'added.dll'), 'new in v2');
      expect(read(install, 'gone.dll'), isNull,
          reason: 'whole-package replacement means removals take effect too');
      expect(retiredDir().existsSync(), isFalse);
    });

    test('consumes the staged directory when told to move it', () async {
      write(install, 'app.exe', 'v1');
      write(staged, 'app.exe', 'v2');

      await swapInstallation(installDir: install, stagedDir: staged);

      expect(staged.existsSync(), isFalse);
    });

    test('leaves the staged directory alone when told to keep it', () async {
      // The applying process is running from there, so deleting it would pull
      // the floor out from under the code doing the deleting.
      write(install, 'app.exe', 'v1');
      write(staged, 'app.exe', 'v2');

      await swapInstallation(
          installDir: install, stagedDir: staged, keepStaged: true);

      expect(read(install, 'app.exe'), 'v2');
      expect(read(staged, 'app.exe'), 'v2');
    });

    test('clears a leftover .rup-old from an earlier update', () async {
      retiredDir().createSync();
      write(retiredDir(), 'junk.txt', 'from a previous run');
      write(install, 'app.exe', 'v1');
      write(staged, 'app.exe', 'v2');

      await swapInstallation(installDir: install, stagedDir: staged);

      expect(read(install, 'app.exe'), 'v2');
      expect(retiredDir().existsSync(), isFalse);
    });

    test('copies nested trees in full', () async {
      write(install, 'app.exe', 'v1');
      write(staged, 'app.exe', 'v2');
      write(staged, 'data/flutter_assets/fonts/a.ttf', 'font');
      write(staged, 'data/flutter_assets/deep/deeper/x.json', '{}');

      await swapInstallation(
          installDir: install, stagedDir: staged, keepStaged: true);

      expect(read(install, 'data/flutter_assets/fonts/a.ttf'), 'font');
      expect(read(install, 'data/flutter_assets/deep/deeper/x.json'), '{}');
    });
  });

  group('preserved entries', () {
    test('survive the replacement', () async {
      write(install, 'app.exe', 'v1');
      write(install, 'logs/app.log', 'yesterday');
      write(staged, 'app.exe', 'v2');

      await swapInstallation(
          installDir: install, stagedDir: staged, preserve: ['logs']);

      expect(read(install, 'app.exe'), 'v2');
      expect(read(install, 'logs/app.log'), 'yesterday');
    });

    test('beat the version shipped in the package', () async {
      // Packages usually ship an empty placeholder. Taking it would delete the
      // logs of the run that most likely prompted the update.
      write(install, 'logs/app.log', 'yesterday');
      write(staged, 'app.exe', 'v2');
      write(staged, 'logs/.keep', '');

      await swapInstallation(
          installDir: install, stagedDir: staged, preserve: ['logs']);

      expect(read(install, 'logs/app.log'), 'yesterday');
      expect(read(install, 'logs/.keep'), isNull);
    });

    test('are skipped without complaint when absent', () async {
      // A fresh installation that has never been run has no logs and no
      // unpacked toolchain, and that is not a problem.
      write(install, 'app.exe', 'v1');
      write(staged, 'app.exe', 'v2');

      await swapInstallation(
        installDir: install,
        stagedDir: staged,
        preserve: ['logs', 'python', 'svn'],
      );

      expect(read(install, 'app.exe'), 'v2');
    });

    test('can be single files, not only directories', () async {
      write(install, 'app.exe', 'v1');
      write(install, 'machine.id', 'abc123');
      write(staged, 'app.exe', 'v2');

      await swapInstallation(
          installDir: install, stagedDir: staged, preserve: ['machine.id']);

      expect(read(install, 'machine.id'), 'abc123');
    });

    test('carry over large unpacked trees intact', () async {
      write(install, 'app.exe', 'v1');
      write(install, 'python/python.exe', 'runtime');
      write(install, 'python/Lib/os.py', 'import sys');
      write(staged, 'app.exe', 'v2');

      await swapInstallation(
          installDir: install, stagedDir: staged, preserve: ['python']);

      expect(read(install, 'python/python.exe'), 'runtime');
      expect(read(install, 'python/Lib/os.py'), 'import sys');
    });
  });

  group('refusals that change nothing', () {
    test('a missing installation directory', () async {
      install.deleteSync();
      write(staged, 'app.exe', 'v2');

      await expectLater(
        swapInstallation(installDir: install, stagedDir: staged),
        throwsA(isA<SwapException>()),
      );
      expect(staged.existsSync(), isTrue, reason: 'nothing should be consumed');
    });

    test('a missing staged directory', () async {
      write(install, 'app.exe', 'v1');
      staged.deleteSync();

      await expectLater(
        swapInstallation(installDir: install, stagedDir: staged),
        throwsA(isA<SwapException>()),
      );
      expect(read(install, 'app.exe'), 'v1');
    });

    test('an installation still in use', () async {
      // The real reason this happens: the process that asked for the update
      // has not finished exiting. On Windows an open handle blocks the rename,
      // which is the signal the swap waits on.
      write(install, 'app.exe', 'v1');
      write(staged, 'app.exe', 'v2');

      final held = File('${install.path}${Platform.pathSeparator}app.exe')
          .openSync(mode: FileMode.append);
      addTearDown(held.closeSync);

      final swap = swapInstallation(
        installDir: install,
        stagedDir: staged,
        renameTimeout: const Duration(milliseconds: 300),
      );

      if (Platform.isWindows) {
        await expectLater(
          swap,
          throwsA(isA<SwapException>()
              .having((e) => e.rolledBack, 'rolledBack', isTrue)
              .having((e) => e.message, 'message', contains('still in use'))),
        );
        expect(read(install, 'app.exe'), 'v1',
            reason: 'a blocked rename must leave the old version running');
        expect(staged.existsSync(), isTrue);
      } else {
        // POSIX renames a directory regardless of open handles, so there is
        // nothing to block on and the update simply succeeds.
        await swap;
        expect(read(install, 'app.exe'), 'v2');
      }
    });

    test('waiting is reported so the outside world can see it is alive',
        () async {
      // Without this callback the wait is invisible: no window, no log until
      // it is over. That silence is what makes a user start the application
      // again and lock the update out permanently.
      write(install, 'app.exe', 'v1');
      write(staged, 'app.exe', 'v2');

      final held = File('${install.path}${Platform.pathSeparator}app.exe')
          .openSync(mode: FileMode.append);
      addTearDown(held.closeSync);

      final waits = <Duration>[];
      final lines = <String>[];

      await swapInstallation(
        installDir: install,
        stagedDir: staged,
        renameTimeout: const Duration(seconds: 3),
        onWaiting: (waited, _) => waits.add(waited),
        log: lines.add,
      ).then<void>((_) {}, onError: (Object _) {});

      if (Platform.isWindows) {
        expect(waits, isNotEmpty,
            reason: 'every blocked attempt has to be observable');
        expect(lines.where((line) => line.contains('still waiting')), isNotEmpty,
            reason: 'a wait that leaves no trace cannot be diagnosed later');
      } else {
        expect(waits, isEmpty, reason: 'POSIX never blocks on the rename');
      }
    });
  });

  group('rollback', () {
    test('puts the old version back when the new one cannot be placed',
        () async {
      write(install, 'app.exe', 'v1');
      write(install, 'data/assets.bin', 'old assets');
      write(staged, 'app.exe', 'v2');

      await expectLater(
        swapInstallation(
          installDir: install,
          stagedDir: staged,
          placeStaged: (from, to) async =>
              throw const FileSystemException('no space left on device'),
        ),
        throwsA(isA<SwapException>()
            .having((e) => e.rolledBack, 'rolledBack', isTrue)),
      );

      expect(read(install, 'app.exe'), 'v1');
      expect(read(install, 'data/assets.bin'), 'old assets');
      expect(retiredDir().existsSync(), isFalse,
          reason: 'a rolled-back update should leave no debris');
    });

    test('clears a half-written installation before restoring', () async {
      write(install, 'app.exe', 'v1');
      write(staged, 'app.exe', 'v2');

      await expectLater(
        swapInstallation(
          installDir: install,
          stagedDir: staged,
          placeStaged: (from, to) async {
            // Half the files land, then the disk gives out.
            to.createSync(recursive: true);
            File('${to.path}${Platform.pathSeparator}app.exe')
                .writeAsStringSync('v2 (truncated)');
            throw const FileSystemException('device failure');
          },
        ),
        throwsA(isA<SwapException>()
            .having((e) => e.rolledBack, 'rolledBack', isTrue)),
      );

      expect(read(install, 'app.exe'), 'v1',
          reason: 'the truncated copy must not survive the rollback');
    });

    test('says so plainly when it cannot restore', () async {
      // The one case with no good outcome. What matters is that the message
      // names the directory holding the intact old version, because at this
      // point a person has to finish the job.
      write(install, 'app.exe', 'v1');
      write(staged, 'app.exe', 'v2');

      late Directory blocked;
      RandomAccessFile? handle;
      addTearDown(() => handle?.closeSync());

      await expectLater(
        swapInstallation(
          installDir: install,
          stagedDir: staged,
          placeStaged: (from, to) async {
            to.createSync(recursive: true);
            final stuck = File('${to.path}${Platform.pathSeparator}stuck.bin')
              ..writeAsStringSync('locked');
            handle = stuck.openSync(mode: FileMode.append);
            blocked = to;
            throw const FileSystemException('device failure');
          },
        ),
        throwsA(isA<SwapException>()
            .having((e) => e.message, 'message', contains(retiredDir().path))),
      );

      expect(blocked.existsSync(), isTrue);
      expect(read(retiredDir(), 'app.exe'), 'v1',
          reason: 'the old version must still be recoverable by hand');
    }, skip: Platform.isWindows ? null : 'needs mandatory file locking');
  });

  group('versionedDir', () {
    test('copies payload beside the launcher and switches active.json',
        () async {
      write(install, 'SvnAutoMerge.exe', 'launcher');
      write(install, 'versions/0.1.2+80/app.exe', 'v80');
      writeActivePointer(
        install,
        ActivePointer(
          code: 80,
          version: '0.1.2+80',
          path: 'versions/0.1.2+80',
          executable: 'versions/0.1.2+80/app.exe',
        ),
      );
      write(staged, 'app.exe', 'v81');
      write(staged, 'data/x.txt', 'payload');

      await swapVersionedInstallation(
        installDir: install,
        payloadDir: staged,
        code: 81,
        version: '0.1.2+81',
        executableName: 'app.exe',
      );

      expect(read(install, 'SvnAutoMerge.exe'), 'launcher');
      expect(read(install, 'versions/0.1.2+81/app.exe'), 'v81');
      expect(read(install, 'versions/0.1.2+80/app.exe'), 'v80');
      final pointer = readActivePointer(install);
      expect(pointer!.version, '0.1.2+81');
      expect(pointer.path, 'versions/0.1.2+81');
    });

    test('prunes versions beyond the retain window of two', () async {
      write(install, 'versions/0.1.2+79/app.exe', 'v79');
      write(install, 'versions/0.1.2+80/app.exe', 'v80');
      writeActivePointer(
        install,
        ActivePointer(
          code: 80,
          version: '0.1.2+80',
          path: 'versions/0.1.2+80',
        ),
      );
      write(staged, 'app.exe', 'v81');

      await swapVersionedInstallation(
        installDir: install,
        payloadDir: staged,
        code: 81,
        version: '0.1.2+81',
        retainVersions: 2,
      );

      expect(read(install, 'versions/0.1.2+81/app.exe'), 'v81');
      expect(read(install, 'versions/0.1.2+80/app.exe'), 'v80');
      expect(read(install, 'versions/0.1.2+79/app.exe'), isNull);
    });

    test('refreshes install-root files when they differ', () async {
      write(install, 'SvnAutoMerge.exe', 'old-launcher');
      write(install, 'relkit-apply.exe', 'old-apply');
      write(staged, 'app.exe', 'v81');
      final zipRoot =
          Directory('${root.path}${Platform.pathSeparator}zip')..createSync();
      write(zipRoot, 'SvnAutoMerge.exe', 'new-launcher');
      write(zipRoot, 'relkit-apply.exe', 'new-apply');

      await swapVersionedInstallation(
        installDir: install,
        payloadDir: staged,
        code: 81,
        version: '0.1.2+81',
        executableName: 'app.exe',
        rootFiles: [
          File('${zipRoot.path}${Platform.pathSeparator}SvnAutoMerge.exe'),
          File('${zipRoot.path}${Platform.pathSeparator}relkit-apply.exe'),
        ],
      );

      expect(read(install, 'SvnAutoMerge.exe'), 'new-launcher');
      expect(read(install, 'relkit-apply.exe'), 'new-apply');
      expect(read(install, 'SvnAutoMerge.exe.old-81'), isNull);
    });

    test('leaves identical root files alone', () async {
      write(install, 'SvnAutoMerge.exe', 'same');
      write(staged, 'app.exe', 'v81');
      final zipRoot =
          Directory('${root.path}${Platform.pathSeparator}zip')..createSync();
      write(zipRoot, 'SvnAutoMerge.exe', 'same');

      await swapVersionedInstallation(
        installDir: install,
        payloadDir: staged,
        code: 81,
        version: '0.1.2+81',
        rootFiles: [
          File('${zipRoot.path}${Platform.pathSeparator}SvnAutoMerge.exe'),
        ],
      );

      expect(read(install, 'SvnAutoMerge.exe'), 'same');
      expect(read(install, 'SvnAutoMerge.exe.old-81'), isNull);
    });
  });
}
