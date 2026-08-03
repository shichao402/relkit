/// Copies this package into an application that cannot depend on it by path.
///
/// Why this exists: `rup_client` lives in the spec repository, and the
/// applications that use it live elsewhere — in at least one case on an
/// internal Git host whose CI cannot reach the spec repository at all. A
/// `git:` dependency would be the right answer if that reachability existed.
/// It does not, so the code is copied.
///
/// Usage:
///
/// ```bash
/// dart run tools/vendor.dart ../../../SvnMergeTool/packages/rup_client
/// ```
library;

import 'dart:io';

const copyTrees = ['lib', 'test', 'example'];
const copyFiles = [
  'pubspec.yaml',
  'README.md',
  'analysis_options.yaml',
  '.gitignore',
];
const skipNames = {'.dart_tool', '.packages', 'pubspec.lock'};

void main(List<String> args) {
  if (args.isEmpty) {
    stderr.writeln('usage: dart run tools/vendor.dart <target-dir> '
        '[--conformance <path>]');
    exit(1);
  }

  final package = File.fromUri(Platform.script).parent.parent;
  Directory? conformanceOverride;
  final positional = <String>[];
  for (var i = 0; i < args.length; i++) {
    if (args[i] == '--conformance') {
      if (i + 1 >= args.length) {
        stderr.writeln('error: --conformance needs a path');
        exit(1);
      }
      conformanceOverride = Directory(args[++i]);
    } else {
      positional.add(args[i]);
    }
  }
  if (positional.length != 1) {
    stderr.writeln('error: exactly one target directory is required');
    exit(1);
  }

  final target = Directory(positional.single).absolute;
  final conformance = (conformanceOverride ??
          Directory('${package.parent.parent.path}${Platform.pathSeparator}conformance'))
      .absolute;

  if (!conformance.existsSync()) {
    stderr.writeln('error: no conformance directory at ${conformance.path}');
    exit(1);
  }
  if (target.path == package.path) {
    stderr.writeln('error: refusing to vendor a package onto itself');
    exit(1);
  }

  target.createSync(recursive: true);

  for (final name in copyTrees) {
    final source = Directory('${package.path}${Platform.pathSeparator}$name');
    if (source.existsSync()) {
      copyTree(source, Directory('${target.path}${Platform.pathSeparator}$name'));
      stdout.writeln('  $name/');
    }
  }

  for (final name in copyFiles) {
    final source = File('${package.path}${Platform.pathSeparator}$name');
    if (source.existsSync()) {
      source.copySync('${target.path}${Platform.pathSeparator}$name');
      stdout.writeln('  $name');
    }
  }

  copyTree(
    conformance,
    Directory('${target.path}${Platform.pathSeparator}conformance'),
  );
  stdout.writeln('  conformance/');

  final (commit, dirty) = gitDescribe(package);
  final today = DateTime.now().toUtc();
  final date =
      '${today.year.toString().padLeft(4, '0')}-${today.month.toString().padLeft(2, '0')}-${today.day.toString().padLeft(2, '0')}';
  File('${target.path}${Platform.pathSeparator}VENDORED.md').writeAsStringSync('''
# Vendored copy of `rup_client`

**Do not edit these files here.** Change them in the source repository
and re-run the vendor script; edits made here are erased by the next
sync, and a fix that lives only in this copy is a fix the protocol
tests in the source repository will never see.

| | |
|---|---|
| Source | `update-spec/clients/dart` in AgentsHelpMe |
| Commit | `$commit`${dirty ? ' (with uncommitted changes)' : ''} |
| Synced | $date |

## Re-syncing

From the source package:

```bash
dart run tools/vendor.dart <path to this directory>
```

## Why a copy

A `git:` dependency would be better, and is not possible: this
application's CI cannot reach the repository the source lives in.

The `conformance/` directory here is a copy of the shared,
language-agnostic fixtures, included so that `dart test` in this
repository still checks the client against the protocol rather than
merely against itself.
''');
  stdout.writeln('  VENDORED.md');
  stdout.writeln('\nvendored into ${target.path}');
  if (dirty) {
    stdout.writeln('WARNING: the source had uncommitted changes; the recorded '
        'commit does not fully describe what was copied');
  }
}

void copyTree(Directory source, Directory target) {
  if (target.existsSync()) {
    target.deleteSync(recursive: true);
  }
  target.createSync(recursive: true);
  for (final entity in source.listSync(recursive: true, followLinks: false)) {
    final relative = entity.path.substring(source.path.length)
        .replaceFirst(RegExp(r'^[\\/]'), '');
    final parts = relative.split(RegExp(r'[\\/]'));
    if (parts.any(skipNames.contains)) continue;
    final destPath = '${target.path}${Platform.pathSeparator}'
        '${parts.join(Platform.pathSeparator)}';
    if (entity is Directory) {
      Directory(destPath).createSync(recursive: true);
    } else if (entity is File) {
      File(destPath).parent.createSync(recursive: true);
      entity.copySync(destPath);
    }
  }
}

(String, bool) gitDescribe(Directory root) {
  String run(List<String> args) {
    try {
      final out = Process.runSync('git', args, workingDirectory: root.path);
      if (out.exitCode != 0) return '';
      return (out.stdout as String).trim();
    } catch (_) {
      return '';
    }
  }

  final commit = run(['rev-parse', 'HEAD']);
  final dirty = run(['status', '--porcelain', '--', '.']).isNotEmpty;
  return (commit.isEmpty ? 'unknown' : commit, dirty);
}
