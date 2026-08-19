import 'dart:io';

import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

void main() {
  test('windows defaults to versionedDir, macOS to wholeRoot', () {
    expect(defaultInstallLayoutFor('windows'), InstallLayout.versionedDir);
    expect(defaultInstallLayoutFor('macos'), InstallLayout.wholeRoot);
  });

  test('macOS rejects versionedDir', () {
    expect(
      isInstallLayoutSupported(
        operatingSystem: 'macos',
        layout: InstallLayout.versionedDir,
      ),
      isFalse,
    );
    expect(
      () => ensureInstallLayoutSupported(
        operatingSystem: 'macos',
        layout: InstallLayout.versionedDir,
      ),
      throwsArgumentError,
    );
  });

  test('session path depends on layout', () {
    final install = Directory('/apps/tool');
    final support = Directory('/users/me/support');
    final versioned = resolveApplySessionFile(
      layout: InstallLayout.versionedDir,
      installDir: install,
      appSupportDir: support,
    );
    final whole = resolveApplySessionFile(
      layout: InstallLayout.wholeRoot,
      installDir: install,
      appSupportDir: support,
    );
    expect(versioned.path.replaceAll('\\', '/'), endsWith('/tool/update_apply.json'));
    expect(whole.path.replaceAll('\\', '/'), endsWith('/support/update_apply.json'));
    expect(whole.path, isNot(equals(versioned.path)));
  });

  test('versionedDir without relkit-apply starts the payload exe, not the launcher',
      () {
    final root = Directory.systemTemp.createTempSync('rup-apply-process-');
    addTearDown(() {
      try {
        if (root.existsSync()) root.deleteSync(recursive: true);
      } on FileSystemException {}
    });
    final stagedRoot = Directory('${root.path}${Platform.pathSeparator}staged')
      ..createSync();
    final launcher = File(
        '${stagedRoot.path}${Platform.pathSeparator}SvnAutoMerge.exe')
      ..writeAsStringSync('launcher');
    final payloadDir = Directory(
        '${stagedRoot.path}${Platform.pathSeparator}versions'
        '${Platform.pathSeparator}0.1.2+81')
      ..createSync(recursive: true);
    final payload = File(
        '${payloadDir.path}${Platform.pathSeparator}SvnAutoMerge.exe')
      ..writeAsStringSync('payload');

    final process = resolveApplyProcess(
      staged: StagedUpdate(directory: stagedRoot, executable: launcher),
      layout: InstallLayout.versionedDir,
      executableName: 'SvnAutoMerge.exe',
      targetVersion: '0.1.2+81',
    );
    expect(process.path, payload.path);
  });

  test('versionedDir prefers an explicit relkit-apply', () {
    final root = Directory.systemTemp.createTempSync('rup-apply-bin-');
    addTearDown(() {
      try {
        if (root.existsSync()) root.deleteSync(recursive: true);
      } on FileSystemException {}
    });
    final stagedRoot = Directory('${root.path}${Platform.pathSeparator}staged')
      ..createSync();
    final launcher = File(
        '${stagedRoot.path}${Platform.pathSeparator}SvnAutoMerge.exe')
      ..writeAsStringSync('launcher');
    final apply = File('${root.path}${Platform.pathSeparator}relkit-apply.exe')
      ..writeAsStringSync('apply');

    final process = resolveApplyProcess(
      staged: StagedUpdate(directory: stagedRoot, executable: launcher),
      layout: InstallLayout.versionedDir,
      executableName: 'SvnAutoMerge.exe',
      applyExecutable: apply,
      targetVersion: '0.1.2+81',
    );
    expect(process.path, apply.path);
  });

  test('versionedRootFiles only lists zip-root files when versions/ exists', () {
    final root = Directory.systemTemp.createTempSync('rup-root-files-');
    addTearDown(() {
      try {
        if (root.existsSync()) root.deleteSync(recursive: true);
      } on FileSystemException {}
    });
    final stagedRoot = Directory('${root.path}${Platform.pathSeparator}staged')
      ..createSync();
    File('${stagedRoot.path}${Platform.pathSeparator}SvnAutoMerge.exe')
        .writeAsStringSync('launcher');
    final applyName =
        Platform.isWindows ? 'relkit-apply.exe' : 'relkit-apply';
    File('${stagedRoot.path}${Platform.pathSeparator}$applyName')
        .writeAsStringSync('apply');

    expect(
      versionedRootFiles(
        stagedRoot: stagedRoot,
        version: '0.1.2+81',
        executableName: 'SvnAutoMerge.exe',
      ),
      isEmpty,
    );

    Directory(
      '${stagedRoot.path}${Platform.pathSeparator}versions'
      '${Platform.pathSeparator}0.1.2+81',
    ).createSync(recursive: true);
    final files = versionedRootFiles(
      stagedRoot: stagedRoot,
      version: '0.1.2+81',
      executableName: 'SvnAutoMerge.exe',
    );
    expect(
      files.map((file) => file.uri.pathSegments.last).toList(),
      containsAll(<String>['SvnAutoMerge.exe', applyName]),
    );
  });
}
