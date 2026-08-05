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
}
