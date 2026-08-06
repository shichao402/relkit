/// A record of the update that is being applied, readable from other processes.
///
/// Applying an update spans three processes -- the old application, the staged
/// copy doing the replacement, and the new application it starts -- and until
/// this file existed none of them could see the others. That gap is not
/// cosmetic. While the staged copy waits for the installation to be released,
/// the application has vanished from the screen with no explanation, so the
/// user starts it again; that new process takes a lock on the very directory
/// being replaced, and the update can then never succeed. The same blindness
/// lets a freshly started application delete the staging directory out from
/// under a still-running apply.
///
/// So the state is written where anyone can read it: a small JSON file in a
/// directory that survives the swap (the staging area does not -- see
/// [ApplySession]). A host reads it at startup and decides what to do; see
/// [readApplySession] and [ApplySession.isLive].
///
/// Liveness is decided by a heartbeat rather than by looking up the process id.
/// There is no portable way to ask whether a pid is alive without also being
/// able to signal it, and a pid that has been recycled answers the wrong
/// question anyway. A timestamp that stops advancing says what the caller
/// actually wants to know: whether anyone is still working on this.
library;

import 'dart:convert';
import 'dart:io';

/// Where an apply is in its life.
enum ApplyState {
  /// A staged copy is replacing the installation right now.
  running,

  /// The installation was replaced. The record is kept only long enough for
  /// the new version to notice it started because of an update.
  succeeded,

  /// The apply gave up. The installation is untouched unless the record says
  /// it needs attention.
  failed,
}

/// What the process replacing an installation is doing, and how it ended.
class ApplySession {
  ApplySession({
    required this.state,
    required this.pid,
    required this.startedAt,
    required this.updatedAt,
    required this.installDir,
    required this.stagedRoot,
    this.targetCode,
    this.targetVersion,
    this.message,
    this.needsAttention = false,
  });

  final ApplyState state;

  /// Recorded for the log only. Liveness comes from [updatedAt]; see the
  /// library comment for why.
  final int pid;

  final DateTime startedAt;

  /// Last sign of life. The applying process rewrites this while it waits.
  final DateTime updatedAt;

  final String installDir;
  final String stagedRoot;

  /// The version being installed, for a host that wants to say which update
  /// is in progress or which one failed.
  final int? targetCode;
  final String? targetVersion;

  /// Why it failed, in a form a host can show a user.
  final String? message;

  /// Set when a failure left the installation in pieces, which is the one case
  /// a user cannot recover from by retrying.
  final bool needsAttention;

  /// Whether someone is still working on this update.
  ///
  /// [staleAfter] has to outlast a heartbeat interval by enough that a machine
  /// under load does not look dead. The default assumes the second-long
  /// heartbeat that [heartbeatInterval] describes.
  bool isLive({
    Duration staleAfter = const Duration(seconds: 20),
    DateTime? now,
  }) {
    if (state != ApplyState.running) return false;
    final at = now ?? DateTime.now();
    final since = at.difference(updatedAt);
    // A clock that jumped backwards, or a file written by a machine with a
    // different idea of the time, must not read as "dead".
    if (since.isNegative) return true;
    return since < staleAfter;
  }

  ApplySession copyWith({
    ApplyState? state,
    DateTime? updatedAt,
    String? message,
    bool? needsAttention,
  }) =>
      ApplySession(
        state: state ?? this.state,
        pid: pid,
        startedAt: startedAt,
        updatedAt: updatedAt ?? this.updatedAt,
        installDir: installDir,
        stagedRoot: stagedRoot,
        targetCode: targetCode,
        targetVersion: targetVersion,
        message: message ?? this.message,
        needsAttention: needsAttention ?? this.needsAttention,
      );

  Map<String, Object?> toJson() => <String, Object?>{
        'state': state.name,
        'pid': pid,
        'startedAt': startedAt.toIso8601String(),
        'updatedAt': updatedAt.toIso8601String(),
        'installDir': installDir,
        'stagedRoot': stagedRoot,
        if (targetCode != null) 'targetCode': targetCode,
        if (targetVersion != null) 'targetVersion': targetVersion,
        if (message != null) 'message': message,
        if (needsAttention) 'needsAttention': true,
      };

  static ApplySession? fromJson(Map<String, Object?> json) {
    final stateName = json['state'];
    final startedAt = DateTime.tryParse('${json['startedAt']}');
    final updatedAt = DateTime.tryParse('${json['updatedAt']}');
    if (stateName is! String || startedAt == null || updatedAt == null) {
      return null;
    }

    ApplyState? state;
    for (final candidate in ApplyState.values) {
      if (candidate.name == stateName) state = candidate;
    }
    if (state == null) return null;

    return ApplySession(
      state: state,
      pid: json['pid'] is int ? json['pid'] as int : 0,
      startedAt: startedAt,
      updatedAt: updatedAt,
      installDir: '${json['installDir'] ?? ''}',
      stagedRoot: '${json['stagedRoot'] ?? ''}',
      targetCode: json['targetCode'] is int ? json['targetCode'] as int : null,
      targetVersion:
          json['targetVersion'] is String ? json['targetVersion'] as String : null,
      message: json['message'] is String ? json['message'] as String : null,
      needsAttention: json['needsAttention'] == true,
    );
  }
}

/// How often a running apply should refresh [ApplySession.updatedAt].
///
/// Exported so a host that reads the file can size its own staleness window
/// against the same number instead of guessing.
const heartbeatInterval = Duration(seconds: 2);

/// Reads the session record, or null when there is none or it is unreadable.
///
/// Never throws. A host calls this on the startup path, where an exception
/// would turn "the update file is corrupt" into "the application does not
/// start" -- a much worse outcome than forgetting one update.
ApplySession? readApplySession(File file) {
  try {
    if (!file.existsSync()) return null;
    final decoded = jsonDecode(file.readAsStringSync());
    if (decoded is! Map<String, Object?>) return null;
    return ApplySession.fromJson(decoded);
  } on Object {
    return null;
  }
}

/// Writes the session record.
///
/// Never throws, for the same reason [readApplySession] does not: losing this
/// file costs the host a warning it would have shown, and must not cost the
/// update itself.
void writeApplySession(File file, ApplySession session) {
  try {
    file.parent.createSync(recursive: true);
    file.writeAsStringSync(jsonEncode(session.toJson()));
  } on Object {
    // Nothing to fall back to, and nothing that depends on it.
  }
}

/// Removes the session record.
void clearApplySession(File file) {
  try {
    if (file.existsSync()) file.deleteSync();
  } on Object {
    // A stale record reads as dead once the heartbeat expires.
  }
}
