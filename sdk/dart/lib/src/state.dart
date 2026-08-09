/// Persisted client state and the throttling rules around it
/// (SPEC.md sections 12.2, 12.4, 12.7, and 16.2).
library;

import 'dart:convert';
import 'dart:io';

/// Whether an index with this sequence may be adopted (SPEC.md section 12.4).
///
/// Equal is accepted: re-fetching the same index is the ordinary case, not an
/// attack. Strictly smaller is refused, because a valid signature proves only
/// that the publisher issued the document at some point, never that it is the
/// newest one, so a network attacker could otherwise replay an old index to
/// steer clients onto a version with known holes.
///
/// A refusal here is not an error to report. Mirrors legitimately lag behind
/// one another, and surfacing that to the user as a failure would train them to
/// ignore a message that usually means nothing.
bool acceptsSequence(int sequence, int? lastSeenSequence) =>
    lastSeenSequence == null || sequence >= lastSeenSequence;

/// Stats for one candidate source key (URL or `service:<id>`).
class SourceStat {
  SourceStat({
    this.successes = 0,
    this.failures = 0,
    this.consecutiveFailures = 0,
    this.lastBytesPerSecond,
    this.lastSuccessAt,
    this.lastFailureAt,
  });

  factory SourceStat.fromJson(Map<String, dynamic> json) => SourceStat(
        successes: json['successes'] as int? ?? 0,
        failures: json['failures'] as int? ?? 0,
        consecutiveFailures: json['consecutiveFailures'] as int? ?? 0,
        lastBytesPerSecond: json['lastBytesPerSecond'] as int?,
        lastSuccessAt: switch (json['lastSuccessAt']) {
          final String value => DateTime.tryParse(value),
          _ => null,
        },
        lastFailureAt: switch (json['lastFailureAt']) {
          final String value => DateTime.tryParse(value),
          _ => null,
        },
      );

  int successes;
  int failures;
  int consecutiveFailures;
  int? lastBytesPerSecond;
  DateTime? lastSuccessAt;
  DateTime? lastFailureAt;

  Map<String, dynamic> toJson() => {
        'successes': successes,
        'failures': failures,
        'consecutiveFailures': consecutiveFailures,
        if (lastBytesPerSecond != null)
          'lastBytesPerSecond': lastBytesPerSecond,
        if (lastSuccessAt != null)
          'lastSuccessAt': lastSuccessAt!.toUtc().toIso8601String(),
        if (lastFailureAt != null)
          'lastFailureAt': lastFailureAt!.toUtc().toIso8601String(),
      };
}

/// What the client remembers between runs, per (product, channel).
class UpdateState {
  UpdateState({
    this.lastCheckAt,
    this.lastResult,
    this.lastSeenSequence,
    this.lastSeenFallbackSequence,
    this.lastSeenDirectorySequence,
    this.lastSuccessfulSourceKey,
    Map<String, SourceStat>? sourceStats,
    Set<int>? skipped,
  })  : sourceStats = sourceStats ?? <String, SourceStat>{},
        skipped = skipped ?? <int>{};

  factory UpdateState.fromJson(Map<String, dynamic> json) {
    final rawSkipped = json['skipped'];
    final rawStats = json['sourceStats'];
    final stats = <String, SourceStat>{};
    if (rawStats is Map) {
      for (final entry in rawStats.entries) {
        final value = entry.value;
        if (entry.key is String && value is Map<String, dynamic>) {
          stats[entry.key as String] = SourceStat.fromJson(value);
        } else if (entry.key is String && value is Map) {
          stats[entry.key as String] =
              SourceStat.fromJson(Map<String, dynamic>.from(value));
        }
      }
    }
    return UpdateState(
      lastCheckAt: switch (json['lastCheckAt']) {
        final String value => DateTime.tryParse(value),
        _ => null,
      },
      lastResult: json['lastResult'] as String?,
      lastSeenSequence: json['lastSeenSequence'] as int?,
      lastSeenFallbackSequence: json['lastSeenFallbackSequence'] as int?,
      lastSeenDirectorySequence: json['lastSeenDirectorySequence'] as int?,
      lastSuccessfulSourceKey: json['lastSuccessfulSourceKey'] as String?,
      sourceStats: stats,
      skipped: rawSkipped is List ? rawSkipped.whereType<int>().toSet() : null,
    );
  }

  DateTime? lastCheckAt;
  String? lastResult;

  /// The highest index sequence ever accepted. Never lower it.
  int? lastSeenSequence;

  /// The highest fallback sequence ever accepted (product-scoped anti-rollback).
  int? lastSeenFallbackSequence;

  /// The highest directory sequence ever accepted (product-scoped anti-rollback).
  int? lastSeenDirectorySequence;

  /// Key of the last candidate that fully succeeded (URL or `service:<id>`).
  String? lastSuccessfulSourceKey;

  /// Per-source outcomes from real attempts only (SPEC §12.7).
  final Map<String, SourceStat> sourceStats;

  /// Codes the user chose to skip. Ignored when an update is mandatory.
  final Set<int> skipped;

  Map<String, dynamic> toJson() => {
        if (lastCheckAt != null)
          'lastCheckAt': lastCheckAt!.toUtc().toIso8601String(),
        if (lastResult != null) 'lastResult': lastResult,
        if (lastSeenSequence != null) 'lastSeenSequence': lastSeenSequence,
        if (lastSeenFallbackSequence != null)
          'lastSeenFallbackSequence': lastSeenFallbackSequence,
        if (lastSeenDirectorySequence != null)
          'lastSeenDirectorySequence': lastSeenDirectorySequence,
        if (lastSuccessfulSourceKey != null)
          'lastSuccessfulSourceKey': lastSuccessfulSourceKey,
        if (sourceStats.isNotEmpty)
          'sourceStats': {
            for (final entry in sourceStats.entries)
              entry.key: entry.value.toJson(),
          },
        'skipped': skipped.toList()..sort(),
      };

  /// Records a newly accepted sequence, never moving the high-water mark down.
  void observeSequence(int sequence) {
    final seen = lastSeenSequence;
    if (seen == null || sequence > seen) lastSeenSequence = sequence;
  }

  void observeFallbackSequence(int sequence) {
    final seen = lastSeenFallbackSequence;
    if (seen == null || sequence > seen) lastSeenFallbackSequence = sequence;
  }

  void observeDirectorySequence(int sequence) {
    final seen = lastSeenDirectorySequence;
    if (seen == null || sequence > seen) lastSeenDirectorySequence = sequence;
  }

  void recordSourceSuccess(String key, {int? bytesPerSecond}) {
    if (key.isEmpty) return;
    final stat = sourceStats.putIfAbsent(key, SourceStat.new);
    stat
      ..successes += 1
      ..consecutiveFailures = 0
      ..lastSuccessAt = DateTime.now().toUtc();
    if (bytesPerSecond != null && bytesPerSecond > 0) {
      stat.lastBytesPerSecond = bytesPerSecond;
    }
    lastSuccessfulSourceKey = key;
  }

  void recordSourceFailure(String key) {
    if (key.isEmpty) return;
    final stat = sourceStats.putIfAbsent(key, SourceStat.new);
    stat
      ..failures += 1
      ..consecutiveFailures += 1
      ..lastFailureAt = DateTime.now().toUtc();
  }
}

abstract class UpdateStateStore {
  Future<UpdateState> load();

  Future<void> save(UpdateState state);
}

/// Keeps state in a JSON file, one per (product, channel).
///
/// Separate files rather than one shared document: two channels of the same app
/// have independent sequences, and mixing them would let a beta index hold back
/// a stable one.
class FileUpdateStateStore implements UpdateStateStore {
  FileUpdateStateStore({
    required Directory directory,
    required String product,
    required String channel,
  }) : file = File('${directory.path}${Platform.pathSeparator}'
            'rup-state-${_sanitize(product)}-${_sanitize(channel)}.json');

  final File file;

  static String _sanitize(String value) =>
      value.replaceAll(RegExp(r'[^A-Za-z0-9._-]'), '_');

  @override
  Future<UpdateState> load() async {
    try {
      if (!await file.exists()) return UpdateState();
      final decoded = json.decode(await file.readAsString());
      if (decoded is! Map<String, dynamic>) return UpdateState();
      return UpdateState.fromJson(decoded);
    } catch (_) {
      // Corrupt state must not block updating. The only thing lost is the
      // rollback high-water mark, which the next successful check restores;
      // refusing to run would be the worse failure, since it disables updates
      // permanently until someone deletes a file they do not know about.
      return UpdateState();
    }
  }

  @override
  Future<void> save(UpdateState state) async {
    await file.parent.create(recursive: true);
    final temp = File('${file.path}.tmp');
    await temp.writeAsString(json.encode(state.toJson()), flush: true);
    await temp.rename(file.path);
  }
}

/// In-memory store, for tests and for hosts that do not want a file.
class MemoryUpdateStateStore implements UpdateStateStore {
  MemoryUpdateStateStore([UpdateState? initial])
      : _state = initial ?? UpdateState();

  UpdateState _state;

  @override
  Future<UpdateState> load() async => _state;

  @override
  Future<void> save(UpdateState state) async => _state = state;
}

/// The intervals from SPEC.md section 12.2. Hosts may override them.
class UpdatePolicy {
  const UpdatePolicy({
    this.afterSuccess = const Duration(hours: 24),
    this.afterFailure = const Duration(hours: 1),
    this.documentTimeout = const Duration(seconds: 10),
    this.downloadIdleTimeout = const Duration(seconds: 60),
    this.downloadRetries = 3,
    this.downloadRetryBackoff = const Duration(milliseconds: 500),
    this.downloadConcurrency = 8,
    this.downloadChunkSize = 4 * 1024 * 1024,
  });

  final Duration afterSuccess;
  final Duration afterFailure;

  /// Applies to index and manifest fetches, which are small.
  final Duration documentTimeout;

  /// Applies between chunks of an artifact download, never to the whole
  /// transfer: a large package on a slow link legitimately takes minutes, so
  /// any total timeout is either useless or actively harmful.
  final Duration downloadIdleTimeout;

  /// Attempts per mirror URL for transient failures (network / 5xx / 408 / 429).
  final int downloadRetries;

  /// Initial backoff between retries; doubles after each failed attempt.
  final Duration downloadRetryBackoff;

  /// Parallel Range workers for one URL. `1` forces single-connection mode.
  final int downloadConcurrency;

  /// Preferred Range slice size in bytes (last slice is truncated to EOF).
  final int downloadChunkSize;

  /// Whether enough time has passed since the last check.
  bool shouldCheck(UpdateState state, {DateTime? now}) {
    final last = state.lastCheckAt;
    if (last == null) return true;
    final wait = state.lastResult == 'error' ? afterFailure : afterSuccess;
    return (now ?? DateTime.now()).difference(last) >= wait;
  }
}
