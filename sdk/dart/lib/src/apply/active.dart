/// Pointer from a stable install root to the currently active version directory.
///
/// This file is a protocol product of [InstallLayout.versionedDir], not a
/// host-specific config. Hosts read it; they must not invent a second schema.
library;

import 'dart:convert';
import 'dart:io';

const activePointerFileName = 'active.json';

class ActivePointer {
  ActivePointer({
    required this.code,
    required this.version,
    required this.path,
    this.executable,
  });

  /// RUP `code` of the active payload.
  final int code;

  /// Display version (for example `0.1.2+81`).
  final String version;

  /// Directory of the payload, relative to the install root, using `/`.
  final String path;

  /// Optional executable relative to the install root.
  final String? executable;

  Map<String, Object?> toJson() => <String, Object?>{
        'code': code,
        'version': version,
        'path': path,
        if (executable != null) 'executable': executable,
      };

  static ActivePointer? fromJson(Map<String, Object?> json) {
    final code = json['code'];
    final version = json['version'];
    final path = json['path'];
    if (code is! int || version is! String || path is! String) return null;
    if (version.isEmpty || path.isEmpty) return null;
    final executable = json['executable'];
    return ActivePointer(
      code: code,
      version: version,
      path: path,
      executable: executable is String && executable.isNotEmpty
          ? executable
          : null,
    );
  }
}

File activePointerFile(Directory installDir) => File(
      '${installDir.path}${Platform.pathSeparator}$activePointerFileName',
    );

ActivePointer? readActivePointer(Directory installDir) {
  try {
    final file = activePointerFile(installDir);
    if (!file.existsSync()) return null;
    final decoded = jsonDecode(file.readAsStringSync());
    if (decoded is! Map) return null;
    return ActivePointer.fromJson(
      decoded.map((key, value) => MapEntry('$key', value)),
    );
  } on Object {
    return null;
  }
}

/// Writes [pointer] via a sibling temp file then rename, so a crash cannot
/// leave a half-written `active.json`.
void writeActivePointer(Directory installDir, ActivePointer pointer) {
  final file = activePointerFile(installDir);
  final temp = File('${file.path}.tmp');
  file.parent.createSync(recursive: true);
  temp.writeAsStringSync('${jsonEncode(pointer.toJson())}\n');
  if (file.existsSync()) {
    file.deleteSync();
  }
  temp.renameSync(file.path);
}

/// Directory name under `versions/` for a release. `+` is kept: it is legal
/// on Windows and macOS and matches the RUP version string.
String versionDirectoryName(String version) {
  final trimmed = version.trim();
  if (trimmed.isEmpty) {
    throw ArgumentError('version id must not be empty');
  }
  if (trimmed.contains(Platform.pathSeparator) ||
      trimmed.contains('/') ||
      trimmed.contains('\\') ||
      trimmed.contains('..')) {
    throw ArgumentError('version id is not a single path segment: $version');
  }
  return trimmed;
}
