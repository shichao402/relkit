import 'dart:io';

import 'package:rup_client/src/apply/apply_exception.dart';
import 'package:rup_client/src/apply/unpack.dart';
import 'package:test/test.dart';

void main() {
  late Directory root;

  setUp(() {
    root = Directory.systemTemp.createTempSync('rup-unpack-');
  });

  tearDown(() {
    if (root.existsSync()) {
      root.deleteSync(recursive: true);
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
    File('${payload.path}${Platform.pathSeparator}SvnAutoMerge.dmg')
        .writeAsBytesSync(const []);

    await expectLater(
      expandInnerInstallerIfPresent(payload),
      throwsA(isA<ApplyException>()),
    );
  }, skip: Platform.isMacOS);
}
