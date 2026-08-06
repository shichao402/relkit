/// Tests for the record that makes an in-flight update visible to other
/// processes.
///
/// The reason this exists is a real failure: an update that waited for the
/// installation to be released while the user, seeing nothing at all, started
/// the application again -- which took a lock on that very directory and made
/// the update impossible forever. Every case below is about a process that has
/// to decide, without being able to ask anyone, whether an update is happening
/// right now.
library;

import 'dart:io';

import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

void main() {
  late Directory root;
  late File file;

  setUp(() {
    root = Directory.systemTemp.createTempSync('rup-session-');
    file = File('${root.path}${Platform.pathSeparator}apply.json');
  });

  tearDown(() {
    try {
      if (root.existsSync()) root.deleteSync(recursive: true);
    } on FileSystemException {
      // Nothing useful to do.
    }
  });

  ApplySession running({DateTime? updatedAt, int? code}) {
    final now = DateTime.now();
    return ApplySession(
      state: ApplyState.running,
      pid: 4242,
      startedAt: now,
      updatedAt: updatedAt ?? now,
      installDir: '/opt/app',
      stagedRoot: '/tmp/staged',
      targetCode: code,
      targetVersion: code == null ? null : '1.2.3',
    );
  }

  group('round trip', () {
    test('keeps everything a host needs to explain the update', () {
      writeApplySession(file, running(code: 41));

      final read = readApplySession(file);
      expect(read, isNotNull);
      expect(read!.state, ApplyState.running);
      expect(read.pid, 4242);
      expect(read.installDir, '/opt/app');
      expect(read.stagedRoot, '/tmp/staged');
      expect(read.targetCode, 41);
      expect(read.targetVersion, '1.2.3');
      expect(read.needsAttention, isFalse);
    });

    test('carries the failure reason and whether a human is needed', () {
      writeApplySession(
        file,
        running().copyWith(
          state: ApplyState.failed,
          message: 'the installation is still in use',
          needsAttention: true,
        ),
      );

      final read = readApplySession(file)!;
      expect(read.state, ApplyState.failed);
      expect(read.message, 'the installation is still in use');
      expect(read.needsAttention, isTrue);
    });
  });

  group('reading a file that is not one', () {
    test('missing reads as no update in progress', () {
      expect(readApplySession(file), isNull);
    });

    test('truncated or corrupt reads as no update in progress', () {
      // This runs on the startup path. An exception here would turn a
      // half-written file into an application that does not start, which is a
      // far worse failure than forgetting one update.
      file.writeAsStringSync('{"state":"run');
      expect(readApplySession(file), isNull);

      file.writeAsStringSync('null');
      expect(readApplySession(file), isNull);

      file.writeAsStringSync('{"state":"teleporting","startedAt":"nonsense"}');
      expect(readApplySession(file), isNull);
    });
  });

  group('liveness', () {
    test('a fresh heartbeat means someone is still working on it', () {
      expect(running().isLive(), isTrue);
    });

    test('a heartbeat that stopped means the applier died', () {
      // The case this catches: the apply process was killed, or the machine
      // lost power mid-update. Nothing deletes the file then, so staleness is
      // the only signal there is.
      final abandoned = running(
        updatedAt: DateTime.now().subtract(const Duration(minutes: 5)),
      );
      expect(abandoned.isLive(), isFalse);
    });

    test('a finished update is not live whatever the timestamp says', () {
      final done = running().copyWith(state: ApplyState.succeeded);
      expect(done.isLive(), isFalse);

      final failed = running().copyWith(state: ApplyState.failed);
      expect(failed.isLive(), isFalse);
    });

    test('a timestamp from the future is treated as alive', () {
      // Clock skew must not read as "dead": deciding an update is abandoned
      // when it is not is the expensive mistake here -- it is what leads a
      // host to delete the staging directory out from under a live apply.
      final skewed = running(
        updatedAt: DateTime.now().add(const Duration(minutes: 10)),
      );
      expect(skewed.isLive(), isTrue);
    });
  });

  test('clearing removes the record', () {
    writeApplySession(file, running());
    expect(readApplySession(file), isNotNull);

    clearApplySession(file);
    expect(file.existsSync(), isFalse);
    expect(readApplySession(file), isNull);

    // And clearing what is not there is not an error.
    clearApplySession(file);
  });
}
