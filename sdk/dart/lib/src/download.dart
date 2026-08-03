/// Downloading and verifying (SPEC.md section 12.3).
///
/// Orchestrates mirror fallback, per-URL retries, optional multi-connection
/// Range downloads, and resume via `.part` + `.part.meta`.
library;

import 'dart:convert';
import 'dart:io';
import 'dart:math' as math;
import 'dart:typed_data';

import 'package:convert/convert.dart';
import 'package:crypto/crypto.dart';

import 'fetch.dart';
import 'models.dart';
import 'state.dart';

/// A file that has been fetched and whose bytes match the signed manifest.
class VerifiedFile {
  VerifiedFile({
    required this.file,
    required this.artifact,
    required this.sourceUrl,
  });

  final File file;
  final Artifact artifact;

  /// Which mirror it actually came from. Worth logging: when one mirror is
  /// stale, this is the only thing that says which.
  final Uri sourceUrl;
}

class VerificationException implements Exception {
  VerificationException(this.message);

  final String message;

  @override
  String toString() => 'VerificationException: $message';
}

/// Downloads an artifact, trying each mirror in turn, and returns it only if
/// the bytes match.
///
/// Mirrors are tried strictly one at a time. Racing them would finish sooner
/// and is explicitly forbidden: it multiplies load on every mirror for every
/// client, and the winner is whichever server is fastest rather than whichever
/// is correct.
///
/// Within a single URL, multiple Range connections may run in parallel when
/// the server advertises byte ranges.
Future<VerifiedFile> downloadArtifact(
  Artifact artifact, {
  required Fetcher fetcher,
  required Directory destinationDir,
  UpdatePolicy policy = const UpdatePolicy(),
  ProgressCallback? onProgress,
  void Function(String message)? log,
}) async {
  final target = File('${destinationDir.path}${Platform.pathSeparator}'
      '${artifact.filename}');
  final partial = File('${target.path}.part');
  final metaFile = File('${target.path}.part.meta');
  final expectedSize = artifact.size.toInt();

  final failures = <String>[];
  for (final rawUrl in artifact.urls) {
    final url = Uri.tryParse(rawUrl);
    if (url == null) {
      failures.add('$rawUrl: not a valid URL');
      continue;
    }

    var attempt = 0;
    var backoff = policy.downloadRetryBackoff;
    while (true) {
      attempt++;
      try {
        await _downloadFromUrl(
          fetcher: fetcher,
          url: url,
          rawUrl: rawUrl,
          artifact: artifact,
          expectedSize: expectedSize,
          partial: partial,
          metaFile: metaFile,
          policy: policy,
          onProgress: onProgress,
          log: log,
        );

        final problem = await _verify(partial, artifact);
        if (problem != null) {
          await _deleteQuietly(partial);
          await _deleteQuietly(metaFile);
          failures.add('$rawUrl: $problem');
          log?.call('rejected $rawUrl: $problem');
          break; // next mirror; hash mismatch is not a transient retry
        }

        if (await target.exists()) await target.delete();
        await partial.rename(target.path);
        await _deleteQuietly(metaFile);
        return VerifiedFile(file: target, artifact: artifact, sourceUrl: url);
      } on FetchException catch (error) {
        final msg = error.message;
        failures.add('$rawUrl: $msg');
        log?.call('failed $rawUrl (attempt $attempt): $msg');

        final canRetry =
            error.isRetryable && attempt < math.max(1, policy.downloadRetries);
        if (!canRetry) {
          await _cleanupAbandonedPartial(partial, metaFile);
          break;
        }

        await Future<void>.delayed(backoff);
        backoff *= 2;
      } catch (error) {
        failures.add('$rawUrl: $error');
        log?.call('failed $rawUrl (attempt $attempt): $error');
        final canRetry = attempt < math.max(1, policy.downloadRetries);
        if (!canRetry) {
          await _cleanupAbandonedPartial(partial, metaFile);
          break;
        }
        await Future<void>.delayed(backoff);
        backoff *= 2;
      }
    }
  }

  throw VerificationException(
      'could not obtain ${artifact.filename} from any of '
      '${artifact.urls.length} URL(s): ${failures.join('; ')}');
}

Future<void> _cleanupAbandonedPartial(File partial, File metaFile) async {
  final hasBytes =
      await partial.exists() && await partial.length() > 0;
  if (!hasBytes) {
    await _deleteQuietly(partial);
    await _deleteQuietly(metaFile);
  }
}

Future<void> _downloadFromUrl({
  required Fetcher fetcher,
  required Uri url,
  required String rawUrl,
  required Artifact artifact,
  required int expectedSize,
  required File partial,
  required File metaFile,
  required UpdatePolicy policy,
  ProgressCallback? onProgress,
  void Function(String message)? log,
}) async {
  await _ensurePartialCompatible(
    partial: partial,
    metaFile: metaFile,
    artifact: artifact,
    rawUrl: rawUrl,
    expectedSize: expectedSize,
  );

  final concurrency = math.max(1, policy.downloadConcurrency);
  var useParallel = concurrency > 1;

  if (useParallel) {
    try {
      final probe = await fetcher.probe(
        url,
        timeout: policy.documentTimeout,
      );
      useParallel = probe.acceptsRanges;
      if (probe.contentLength != null &&
          probe.contentLength != expectedSize) {
        log?.call(
          'probe Content-Length ${probe.contentLength} != '
          'manifest $expectedSize for $url (continuing with manifest size)',
        );
      }
    } on FetchException catch (error) {
      log?.call('probe failed ($error); falling back to single connection');
      useParallel = false;
    }
  }

  if (useParallel) {
    try {
      await _downloadParallel(
        fetcher: fetcher,
        url: url,
        rawUrl: rawUrl,
        artifact: artifact,
        expectedSize: expectedSize,
        partial: partial,
        metaFile: metaFile,
        policy: policy,
        onProgress: onProgress,
        log: log,
      );
      return;
    } on FetchException catch (error) {
      if (error.message.contains('range not honored')) {
        log?.call('server ignored Range; falling back to single connection');
        await _deleteQuietly(partial);
        await _deleteQuietly(metaFile);
      } else {
        rethrow;
      }
    }
  }

  await _downloadSingle(
    fetcher: fetcher,
    url: url,
    rawUrl: rawUrl,
    expectedSize: expectedSize,
    partial: partial,
    metaFile: metaFile,
    artifact: artifact,
    policy: policy,
    onProgress: onProgress,
  );
}

Future<void> _ensurePartialCompatible({
  required File partial,
  required File metaFile,
  required Artifact artifact,
  required String rawUrl,
  required int expectedSize,
}) async {
  if (!await metaFile.exists()) {
    if (await partial.exists()) {
      final len = await partial.length();
      if (len <= 0 || len >= expectedSize) {
        await _deleteQuietly(partial);
      }
      // Contiguous prefix without meta is fine for single-connection resume.
    }
    return;
  }

  try {
    final meta = await _PartMeta.load(metaFile);
    final ok = meta.sha256 == artifact.sha256 &&
        meta.size == expectedSize &&
        meta.url == rawUrl;
    if (!ok) {
      await _deleteQuietly(partial);
      await _deleteQuietly(metaFile);
    }
  } catch (_) {
    await _deleteQuietly(partial);
    await _deleteQuietly(metaFile);
  }
}

Future<void> _downloadSingle({
  required Fetcher fetcher,
  required Uri url,
  required String rawUrl,
  required int expectedSize,
  required File partial,
  required File metaFile,
  required Artifact artifact,
  required UpdatePolicy policy,
  ProgressCallback? onProgress,
}) async {
  var startOffset = 0;
  if (await partial.exists()) {
    startOffset = await partial.length();
    if (startOffset >= expectedSize) {
      await _deleteQuietly(partial);
      startOffset = 0;
    }
  }

  // Record single-mode meta so a later parallel attempt can revalidate.
  await _PartMeta(
    sha256: artifact.sha256,
    size: expectedSize,
    url: rawUrl,
    completed: startOffset > 0 ? [[0, startOffset - 1]] : [],
    mode: 'single',
  ).save(metaFile);

  try {
    await fetcher.download(
      url,
      partial,
      idleTimeout: policy.downloadIdleTimeout,
      startOffset: startOffset,
      knownTotal: expectedSize,
      onProgress: onProgress,
    );
  } on FetchException catch (error) {
    if (error.message.contains('range not honored') && startOffset > 0) {
      await _deleteQuietly(partial);
      await _PartMeta(
        sha256: artifact.sha256,
        size: expectedSize,
        url: rawUrl,
        completed: [],
        mode: 'single',
      ).save(metaFile);
      await fetcher.download(
        url,
        partial,
        idleTimeout: policy.downloadIdleTimeout,
        startOffset: 0,
        knownTotal: expectedSize,
        onProgress: onProgress,
      );
    } else {
      rethrow;
    }
  }
}

Future<void> _downloadParallel({
  required Fetcher fetcher,
  required Uri url,
  required String rawUrl,
  required Artifact artifact,
  required int expectedSize,
  required File partial,
  required File metaFile,
  required UpdatePolicy policy,
  ProgressCallback? onProgress,
  void Function(String message)? log,
}) async {
  final chunkSize = math.max(1024, policy.downloadChunkSize);
  final concurrency = math.max(1, policy.downloadConcurrency);

  var meta = await metaFile.exists()
      ? await _PartMeta.load(metaFile)
      : _PartMeta(
          sha256: artifact.sha256,
          size: expectedSize,
          url: rawUrl,
          completed: [],
          mode: 'parallel',
        );

  // Promote a contiguous single-mode prefix into completed ranges.
  if (meta.mode == 'single' && await partial.exists()) {
    final len = await partial.length();
    if (len > 0 && len < expectedSize) {
      meta = meta.copyWith(
        mode: 'parallel',
        completed: [
          [0, len - 1]
        ],
      );
    } else if (len >= expectedSize) {
      await _deleteQuietly(partial);
      meta = meta.copyWith(mode: 'parallel', completed: []);
    } else {
      meta = meta.copyWith(mode: 'parallel', completed: []);
    }
  } else {
    meta = meta.copyWith(mode: 'parallel');
  }

  // Ensure the file exists at the final size without wiping resume data.
  await partial.parent.create(recursive: true);
  if (!await partial.exists()) {
    await partial.create();
  }
  final resize = await partial.open(mode: FileMode.append);
  try {
    await resize.truncate(expectedSize);
    await resize.flush();
  } finally {
    await resize.close();
  }

  final slices = _planSlices(expectedSize, chunkSize);
  final pending = <List<int>>[
    for (final slice in slices)
      if (!_rangeCovered(meta.completed, slice[0], slice[1])) slice,
  ];

  final meter = ThroughputMeter()
    ..seed(_completedBytes(meta.completed, expectedSize));
  void emit() {
    onProgress?.call(meter.observe(total: expectedSize));
  }

  emit();

  final writeLock = _AsyncMutex();
  final errors = <Object>[];
  var nextIndex = 0;

  Future<void> worker() async {
    while (true) {
      if (errors.isNotEmpty) return;
      final i = nextIndex++;
      if (i >= pending.length) return;
      final slice = pending[i];
      final start = slice[0];
      final end = slice[1];

      // Stage each slice into a side file, then copy under lock so a failed
      // worker cannot leave a torn region in `.part`.
      final chunkFile = File('${partial.path}.chunk.$start-$end');
      try {
        await fetcher.downloadRange(
          url,
          destination: chunkFile,
          start: start,
          endInclusive: end,
          idleTimeout: policy.downloadIdleTimeout,
          onBytes: (n) {
            onProgress?.call(meter.observe(total: expectedSize, delta: n));
          },
        );

        final bytes = await chunkFile.readAsBytes();
        final want = end - start + 1;
        if (bytes.length != want) {
          throw FetchException(
            url,
            'chunk size mismatch: got ${bytes.length} want $want',
            retryable: true,
          );
        }
        await writeLock.synchronized(() async {
          final out = await partial.open(mode: FileMode.append);
          try {
            await out.setPosition(start);
            await out.writeFrom(bytes);
            await out.flush();
          } finally {
            await out.close();
          }
          meta = meta.copyWith(
            completed: _mergeRanges([
              ...meta.completed,
              [start, end]
            ]),
          );
          await meta.save(metaFile);
        });
      } catch (error) {
        errors.add(error);
        rethrow;
      } finally {
        await _deleteQuietly(chunkFile);
      }
    }
  }

  final workers = [
    for (var w = 0; w < math.min(concurrency, pending.length); w++) worker(),
  ];

  try {
    await Future.wait(workers);
  } catch (_) {
    if (errors.isNotEmpty) {
      final first = errors.first;
      if (first is FetchException) rethrow;
      throw FetchException(url, 'parallel download failed: $first');
    }
    rethrow;
  }

  if (pending.isEmpty) {
    log?.call('all ranges already present for $url');
  }
  emit();
}

/// Plans inclusive [start, end] slices covering `[0, size)`.
List<List<int>> _planSlices(int size, int chunkSize) {
  if (size <= 0) return [];
  final out = <List<int>>[];
  for (var start = 0; start < size; start += chunkSize) {
    final end = math.min(size - 1, start + chunkSize - 1);
    out.add([start, end]);
  }
  return out;
}

bool _rangeCovered(List<List<int>> completed, int start, int end) {
  for (final r in completed) {
    if (r[0] <= start && r[1] >= end) return true;
  }
  return false;
}

int _completedBytes(List<List<int>> completed, int size) {
  var n = 0;
  for (final r in completed) {
    final a = math.max(0, r[0]);
    final b = math.min(size - 1, r[1]);
    if (b >= a) n += b - a + 1;
  }
  return n;
}

List<List<int>> _mergeRanges(List<List<int>> input) {
  if (input.isEmpty) return [];
  final sorted = [...input]..sort((a, b) => a[0].compareTo(b[0]));
  final out = <List<int>>[
    [sorted.first[0], sorted.first[1]]
  ];
  for (var i = 1; i < sorted.length; i++) {
    final cur = sorted[i];
    final last = out.last;
    if (cur[0] <= last[1] + 1) {
      last[1] = math.max(last[1], cur[1]);
    } else {
      out.add([cur[0], cur[1]]);
    }
  }
  return out;
}

/// Size first, then hash. Size is free and rules out the common case of a
/// truncated transfer or an error page served with status 200, so the expensive
/// check only runs on plausible input.
Future<String?> _verify(File file, Artifact artifact) async {
  final expectedSize = artifact.size.toInt();
  final length = await file.length();
  if (length != expectedSize) {
    return 'expected $expectedSize bytes, got $length';
  }

  final actual = await sha256OfFile(file);
  if (actual != artifact.sha256) {
    return 'sha256 mismatch (expected ${artifact.sha256}, got $actual)';
  }
  return null;
}

/// Streams the file through the hash rather than reading it whole.
Future<String> sha256OfFile(File file) async {
  final digest = AccumulatorSink<Digest>();
  final input = sha256.startChunkedConversion(digest);
  await for (final chunk in file.openRead()) {
    input.add(chunk);
  }
  input.close();
  return digest.events.single.toString();
}

/// Hashes bytes already in memory. For manifests, which are small by design.
String sha256OfBytes(Uint8List bytes) => sha256.convert(bytes).toString();

Future<void> _deleteQuietly(File file) async {
  try {
    if (await file.exists()) await file.delete();
  } catch (_) {
    // Best effort. A leftover .part file is untidy but harmless, and failing
    // the update because cleanup failed would be worse.
  }
}

class _PartMeta {
  _PartMeta({
    required this.sha256,
    required this.size,
    required this.url,
    required this.completed,
    required this.mode,
  });

  final String sha256;
  final int size;
  final String url;
  final List<List<int>> completed;
  final String mode;

  _PartMeta copyWith({
    List<List<int>>? completed,
    String? mode,
  }) =>
      _PartMeta(
        sha256: sha256,
        size: size,
        url: url,
        completed: completed ?? this.completed,
        mode: mode ?? this.mode,
      );

  static Future<_PartMeta> load(File file) async {
    final json = jsonDecode(await file.readAsString()) as Map<String, dynamic>;
    final raw = json['completed'];
    final completed = <List<int>>[];
    if (raw is List) {
      for (final item in raw) {
        if (item is List && item.length >= 2) {
          completed.add([(item[0] as num).toInt(), (item[1] as num).toInt()]);
        }
      }
    }
    return _PartMeta(
      sha256: json['sha256'] as String,
      size: (json['size'] as num).toInt(),
      url: json['url'] as String,
      completed: completed,
      mode: (json['mode'] as String?) ?? 'parallel',
    );
  }

  Future<void> save(File file) async {
    await file.parent.create(recursive: true);
    final temp = File('${file.path}.tmp');
    await temp.writeAsString(
      jsonEncode({
        'sha256': sha256,
        'size': size,
        'url': url,
        'mode': mode,
        'completed': completed,
      }),
      flush: true,
    );
    if (await file.exists()) await file.delete();
    await temp.rename(file.path);
  }
}

/// Chains async critical sections without an external lock package.
class _AsyncMutex {
  Future<void> _tail = Future<void>.value();

  Future<T> synchronized<T>(Future<T> Function() action) {
    final result = _tail.then((_) => action());
    _tail = result.then<void>((_) {}, onError: (_) {});
    return result;
  }
}
