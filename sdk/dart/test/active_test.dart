import 'dart:io';

import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

void main() {
  late Directory root;

  setUp(() {
    root = Directory.systemTemp.createTempSync('rup-active-');
  });

  tearDown(() {
    try {
      root.deleteSync(recursive: true);
    } on FileSystemException {
      // temp
    }
  });

  test('round-trips through an atomic write', () {
    writeActivePointer(
      root,
      ActivePointer(
        code: 81,
        version: '0.1.2+81',
        path: 'versions/0.1.2+81',
        executable: 'versions/0.1.2+81/SvnAutoMerge.exe',
      ),
    );
    final read = readActivePointer(root);
    expect(read, isNotNull);
    expect(read!.code, 81);
    expect(read.version, '0.1.2+81');
    expect(read.path, 'versions/0.1.2+81');
    expect(read.executable, 'versions/0.1.2+81/SvnAutoMerge.exe');
    expect(File('${root.path}${Platform.pathSeparator}active.json.tmp').existsSync(),
        isFalse);
  });

  test('rejects a version id that would escape versions/', () {
    expect(() => versionDirectoryName('../x'), throwsArgumentError);
    expect(versionDirectoryName('0.1.2+81'), '0.1.2+81');
  });
}
