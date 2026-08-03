/// The network boundary (SPEC.md section 3.2).
///
/// An interface rather than direct `dart:io` calls, so the orchestration in
/// `updater.dart` can be tested without a server, and so a host that must route
/// through its own proxy or client certificate can substitute its own.
library;

import 'dart:async';
import 'dart:io';
import 'dart:math' as math;
import 'dart:typed_data';

class FetchException implements Exception {
  FetchException(this.url, this.message, {this.statusCode, this.retryable});

  final Uri url;
  final String message;
  final int? statusCode;

  /// When null, callers infer from [statusCode] / message (network vs 4xx).
  final bool? retryable;

  bool get isRetryable {
    if (retryable != null) return retryable!;
    final code = statusCode;
    if (code == null) return true; // connection / timeout
    if (code == 408 || code == 429) return true;
    if (code >= 500 && code <= 599) return true;
    return false;
  }

  @override
  String toString() => 'FetchException($url): $message';
}

/// Progress of an artifact download, including a smoothed throughput estimate.
class DownloadProgress {
  const DownloadProgress({
    required this.received,
    required this.total,
    required this.bytesPerSecond,
    this.eta,
  });

  final int received;
  final int total;
  final double bytesPerSecond;
  final Duration? eta;

  double? get fraction => total > 0 ? received / total : null;
}

/// Called as bytes arrive. Throughput is a short sliding window, not lifetime average.
typedef ProgressCallback = void Function(DownloadProgress progress);

/// Result of probing a URL for size and Range support.
class ResourceProbe {
  const ResourceProbe({
    required this.acceptsRanges,
    this.contentLength,
  });

  final bool acceptsRanges;
  final int? contentLength;
}

/// Shared helper: sliding-window bytes/sec from incremental [onBytes] reports.
class ThroughputMeter {
  ThroughputMeter({this.window = const Duration(seconds: 1)});

  final Duration window;
  final List<({DateTime at, int bytes})> _samples = [];
  int _received = 0;

  int get received => _received;

  DownloadProgress observe({required int total, int? delta}) {
    if (delta != null && delta > 0) {
      _received += delta;
      final now = DateTime.now();
      _samples.add((at: now, bytes: delta));
      final cutoff = now.subtract(window);
      while (_samples.isNotEmpty && _samples.first.at.isBefore(cutoff)) {
        _samples.removeAt(0);
      }
    }
    final spanMs = _samples.isEmpty
        ? 0
        : math.max(
            1,
            _samples.last.at.difference(_samples.first.at).inMilliseconds,
          );
    final windowBytes =
        _samples.fold<int>(0, (sum, s) => sum + s.bytes);
    // If only one sample, use elapsed since that sample against wall clock.
    final bps = _samples.isEmpty
        ? 0.0
        : _samples.length == 1
            ? windowBytes /
                math.max(
                  0.001,
                  DateTime.now().difference(_samples.first.at).inMilliseconds /
                      1000.0,
                )
            : windowBytes / (spanMs / 1000.0);

    Duration? eta;
    if (bps > 1 && total > _received) {
      final secs = (total - _received) / bps;
      eta = Duration(milliseconds: (secs * 1000).round());
    }
    return DownloadProgress(
      received: _received,
      total: total,
      bytesPerSecond: bps,
      eta: eta,
    );
  }

  void seed(int alreadyReceived) {
    _received = alreadyReceived;
  }
}

abstract class Fetcher {
  /// Fetches a small document whole.
  Future<Uint8List> getBytes(Uri url, {required Duration timeout});

  /// Probes whether the resource supports byte ranges (and optional length).
  Future<ResourceProbe> probe(Uri url, {required Duration timeout});

  /// Streams a large file to disk (single connection).
  ///
  /// When [startOffset] > 0, sends `Range: bytes=startOffset-` and appends to
  /// [destination]. A `200` response means the server ignored Range: the
  /// caller should truncate and restart (this method throws
  /// [FetchException] with message `range not honored`).
  Future<void> download(
    Uri url,
    File destination, {
    required Duration idleTimeout,
    int startOffset = 0,
    int? knownTotal,
    ProgressCallback? onProgress,
  });

  /// Fetches inclusive byte range `[start, endInclusive]` into [destination]
  /// as a contiguous file starting at offset 0 (length = end-start+1).
  /// Requires HTTP 206.
  Future<void> downloadRange(
    Uri url, {
    required File destination,
    required int start,
    required int endInclusive,
    required Duration idleTimeout,
    void Function(int bytesJustReceived)? onBytes,
  });

  void close();
}

/// The default [Fetcher], on `dart:io`.
class HttpFetcher implements Fetcher {
  HttpFetcher({String userAgent = 'rup-client/2'})
      : _client = HttpClient()..userAgent = userAgent;

  final HttpClient _client;

  /// Redirect limit from SPEC.md section 3.2. Object storage and release hosts
  /// redirect routinely, so refusing to follow any would break real
  /// deployments; allowing unbounded ones invites a loop.
  static const maxRedirects = 5;

  @override
  Future<Uint8List> getBytes(Uri url, {required Duration timeout}) async {
    final response = await _open(url, timeout: timeout);
    try {
      final builder = BytesBuilder(copy: false);
      await for (final chunk in response.timeout(timeout)) {
        builder.add(chunk);
      }
      return builder.takeBytes();
    } on TimeoutException {
      throw FetchException(url, 'timed out after ${timeout.inSeconds}s');
    }
  }

  @override
  Future<ResourceProbe> probe(Uri url, {required Duration timeout}) async {
    // Prefer HEAD. Some CDNs mishandle HEAD; fall back to a one-byte Range GET.
    try {
      final response = await _open(
        url,
        timeout: timeout,
        method: 'HEAD',
      );
      await response.drain<void>();
      final accepts = _acceptsBytesRanges(response);
      final length = response.contentLength >= 0 ? response.contentLength : null;
      return ResourceProbe(acceptsRanges: accepts, contentLength: length);
    } on FetchException catch (error) {
      if (error.statusCode == 405 || error.statusCode == 501) {
        // Fall through to Range probe.
      } else if (error.statusCode != null &&
          error.statusCode! >= 400 &&
          error.statusCode! < 500) {
        rethrow;
      }
    }

    final response = await _open(
      url,
      timeout: timeout,
      headers: {HttpHeaders.rangeHeader: 'bytes=0-0'},
    );
    try {
      if (response.statusCode == 206) {
        final total = _totalFromContentRange(response);
        await response.drain<void>();
        return ResourceProbe(acceptsRanges: true, contentLength: total);
      }
      if (response.statusCode == 200) {
        final length =
            response.contentLength >= 0 ? response.contentLength : null;
        await response.drain<void>();
        return ResourceProbe(acceptsRanges: false, contentLength: length);
      }
      await response.drain<void>();
      throw FetchException(
        url,
        'HTTP ${response.statusCode}',
        statusCode: response.statusCode,
      );
    } finally {
      // drained above
    }
  }

  @override
  Future<void> download(
    Uri url,
    File destination, {
    required Duration idleTimeout,
    int startOffset = 0,
    int? knownTotal,
    ProgressCallback? onProgress,
  }) async {
    final headers = <String, String>{};
    if (startOffset > 0) {
      headers[HttpHeaders.rangeHeader] = 'bytes=$startOffset-';
    }

    final response = await _open(
      url,
      timeout: idleTimeout,
      headers: headers.isEmpty ? null : headers,
    );

    if (startOffset > 0) {
      if (response.statusCode == 200) {
        await response.drain<void>();
        throw FetchException(
          url,
          'range not honored',
          statusCode: 200,
          retryable: false,
        );
      }
      if (response.statusCode != 206) {
        await response.drain<void>();
        throw FetchException(
          url,
          'HTTP ${response.statusCode}',
          statusCode: response.statusCode,
        );
      }
    }

    final contentLength =
        response.contentLength >= 0 ? response.contentLength : null;
    final total = knownTotal ??
        (startOffset > 0 && contentLength != null
            ? startOffset + contentLength
            : contentLength);

    await destination.parent.create(recursive: true);
    final sink = destination.openWrite(
      mode: startOffset > 0 ? FileMode.append : FileMode.write,
    );
    final meter = ThroughputMeter()..seed(startOffset);
    try {
      await for (final chunk in response.timeout(idleTimeout)) {
        sink.add(chunk);
        if (onProgress != null) {
          onProgress(meter.observe(total: total ?? 0, delta: chunk.length));
        } else {
          meter.observe(total: total ?? 0, delta: chunk.length);
        }
      }
      await sink.flush();
    } on TimeoutException {
      await sink.close();
      throw FetchException(
        url,
        'stalled for ${idleTimeout.inSeconds}s after ${meter.received} bytes',
      );
    } catch (error) {
      await sink.close();
      if (error is FetchException) rethrow;
      throw FetchException(url, 'download failed: $error');
    }
    await sink.close();
  }

  @override
  Future<void> downloadRange(
    Uri url, {
    required File destination,
    required int start,
    required int endInclusive,
    required Duration idleTimeout,
    void Function(int bytesJustReceived)? onBytes,
  }) async {
    if (endInclusive < start) {
      throw ArgumentError('endInclusive < start');
    }
    final expected = endInclusive - start + 1;
    final response = await _open(
      url,
      timeout: idleTimeout,
      headers: {HttpHeaders.rangeHeader: 'bytes=$start-$endInclusive'},
    );

    if (response.statusCode == 200) {
      await response.drain<void>();
      throw FetchException(
        url,
        'range not honored',
        statusCode: 200,
        retryable: false,
      );
    }
    if (response.statusCode != 206) {
      await response.drain<void>();
      throw FetchException(
        url,
        'HTTP ${response.statusCode}',
        statusCode: response.statusCode,
      );
    }

    await destination.parent.create(recursive: true);
    final sink = destination.openWrite(mode: FileMode.write);
    var got = 0;
    try {
      await for (final chunk in response.timeout(idleTimeout)) {
        sink.add(chunk);
        got += chunk.length;
        onBytes?.call(chunk.length);
      }
      await sink.flush();
    } on TimeoutException {
      await sink.close();
      throw FetchException(
        url,
        'stalled for ${idleTimeout.inSeconds}s after $got bytes of range',
      );
    } catch (error) {
      await sink.close();
      if (error is FetchException) rethrow;
      throw FetchException(url, 'range download failed: $error');
    }
    await sink.close();

    if (got != expected) {
      throw FetchException(
        url,
        'range size mismatch: got $got want $expected',
        retryable: true,
      );
    }
  }

  Future<HttpClientResponse> _open(
    Uri url, {
    required Duration timeout,
    String method = 'GET',
    Map<String, String>? headers,
  }) async {
    if (url.scheme != 'http' && url.scheme != 'https') {
      throw FetchException(url, 'unsupported scheme "${url.scheme}"');
    }

    Future<HttpClientResponse> send(Uri target) async {
      try {
        final request = await _client.openUrl(method, target).timeout(timeout);
        request.followRedirects = false;
        headers?.forEach(request.headers.set);
        return await request.close().timeout(timeout);
      } on TimeoutException {
        throw FetchException(target, 'timed out after ${timeout.inSeconds}s');
      } on SocketException catch (error) {
        throw FetchException(target, 'connection failed: ${error.message}');
      }
    }

    var response = await send(url);
    var current = url;
    for (var hop = 0; hop < maxRedirects; hop++) {
      if (!_isRedirect(response.statusCode)) break;

      final location = response.headers.value(HttpHeaders.locationHeader);
      if (location == null) {
        throw FetchException(current, 'redirect without a Location header',
            statusCode: response.statusCode);
      }
      final next = current.resolve(location);

      if (current.scheme == 'https' && next.scheme != 'https') {
        throw FetchException(
            current, 'refusing redirect from https to ${next.scheme}');
      }

      await response.drain<void>();
      response = await send(next);
      current = next;
    }

    if (_isRedirect(response.statusCode)) {
      throw FetchException(current, 'more than $maxRedirects redirects',
          statusCode: response.statusCode);
    }

    // For HEAD/GET with Range we accept 206 as success at the call site.
    // Here only reject obvious error statuses so probe/download can inspect 206/200.
    if (response.statusCode >= 400) {
      final code = response.statusCode;
      await response.drain<void>();
      throw FetchException(current, 'HTTP $code', statusCode: code);
    }
    return response;
  }

  static bool _acceptsBytesRanges(HttpClientResponse response) {
    final raw = response.headers.value(HttpHeaders.acceptRangesHeader);
    if (raw == null) return false;
    return raw.toLowerCase().split(',').any((p) => p.trim() == 'bytes');
  }

  static int? _totalFromContentRange(HttpClientResponse response) {
    final cr = response.headers.value(HttpHeaders.contentRangeHeader);
    if (cr == null) return null;
    final slash = cr.lastIndexOf('/');
    if (slash < 0 || slash + 1 >= cr.length) return null;
    return int.tryParse(cr.substring(slash + 1));
  }

  static bool _isRedirect(int status) =>
      status == 301 ||
      status == 302 ||
      status == 303 ||
      status == 307 ||
      status == 308;

  @override
  void close() => _client.close(force: true);
}
