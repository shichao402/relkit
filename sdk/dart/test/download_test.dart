/// Real-HTTP coverage for resume, Range parallelism, fallback, retries, speed.
library;

import 'dart:async';
import 'dart:io';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

void main() {
  late Uint8List payload;
  late String digest;
  late Directory temp;

  setUp(() async {
    // ~256 KiB — enough for multiple chunks at 32 KiB.
    payload = Uint8List.fromList([
      for (var i = 0; i < 256 * 1024; i++) i & 0xff,
    ]);
    digest = sha256.convert(payload).toString();
    temp = await Directory.systemTemp.createTemp('rup_dl_');
  });

  tearDown(() async {
    if (await temp.exists()) await temp.delete(recursive: true);
  });

  Artifact artifact(List<String> urls) => Artifact(
        filename: 'app.bin',
        size: Int64(payload.length),
        sha256: digest,
        urls: urls,
      );

  group('HttpFetcher download engine', () {
    test('reports non-zero bytesPerSecond while downloading', () async {
      final server = await _RangeServer.bind(payload, supportRange: true);
      addTearDown(server.close);

      final progress = <DownloadProgress>[];
      final fetcher = HttpFetcher();
      addTearDown(fetcher.close);

      final verified = await downloadArtifact(
        artifact([server.url]),
        fetcher: fetcher,
        destinationDir: temp,
        policy: const UpdatePolicy(
          downloadConcurrency: 1,
          downloadChunkSize: 32 * 1024,
        ),
        onProgress: progress.add,
      );

      expect(await verified.file.readAsBytes(), payload);
      expect(progress, isNotEmpty);
      expect(progress.any((p) => p.bytesPerSecond > 0), isTrue);
      expect(progress.last.received, payload.length);
    });

    test('reuses a complete download instead of fetching it again', () async {
      // What this buys: an update whose *install* failed can be retried
      // without paying for the package a second time. The bytes were checked
      // against the signed manifest when they arrived, and are checked again
      // here, so nothing is trusted merely because it is already on disk.
      final server = await _RangeServer.bind(payload, supportRange: true);
      addTearDown(server.close);

      final target = File('${temp.path}${Platform.pathSeparator}app.bin');
      await target.writeAsBytes(payload, flush: true);

      var requests = 0;
      server.onRequest = (_) => requests++;

      final fetcher = HttpFetcher();
      addTearDown(fetcher.close);

      final verified = await downloadArtifact(
        artifact([server.url]),
        fetcher: fetcher,
        destinationDir: temp,
      );

      expect(verified.file.path, target.path);
      expect(requests, 0, reason: 'a verified copy makes the network pointless');
    });

    test('replaces a complete file whose bytes are wrong', () async {
      // The dangerous half of reuse: a file with the right name and the wrong
      // contents must never be handed on as verified.
      final server = await _RangeServer.bind(payload, supportRange: true);
      addTearDown(server.close);

      final target = File('${temp.path}${Platform.pathSeparator}app.bin');
      await target.writeAsBytes(
        Uint8List.fromList(List.filled(payload.length, 0)),
        flush: true,
      );

      final fetcher = HttpFetcher();
      addTearDown(fetcher.close);

      final verified = await downloadArtifact(
        artifact([server.url]),
        fetcher: fetcher,
        destinationDir: temp,
      );

      expect(await verified.file.readAsBytes(), payload);
    });

    test('resumes a partial single-connection download', () async {
      final server = await _RangeServer.bind(payload, supportRange: true);
      addTearDown(server.close);

      final partial = File('${temp.path}${Platform.pathSeparator}app.bin.part');
      final half = payload.sublist(0, payload.length ~/ 2);
      await partial.writeAsBytes(half, flush: true);

      final fetcher = HttpFetcher();
      addTearDown(fetcher.close);

      var rangeResumes = 0;
      server.onRange = (_) => rangeResumes++;

      final verified = await downloadArtifact(
        artifact([server.url]),
        fetcher: fetcher,
        destinationDir: temp,
        policy: const UpdatePolicy(downloadConcurrency: 1),
      );

      expect(await verified.file.readAsBytes(), payload);
      expect(rangeResumes, greaterThan(0));
    });

    test('parallel Range workers reassemble the file', () async {
      final server = await _RangeServer.bind(payload, supportRange: true);
      addTearDown(server.close);

      final fetcher = HttpFetcher();
      addTearDown(fetcher.close);

      var rangeHits = 0;
      server.onRange = (_) => rangeHits++;

      final verified = await downloadArtifact(
        artifact([server.url]),
        fetcher: fetcher,
        destinationDir: temp,
        policy: const UpdatePolicy(
          downloadConcurrency: 8,
          downloadChunkSize: 16 * 1024,
        ),
      );

      expect(await verified.file.readAsBytes(), payload);
      expect(rangeHits, greaterThan(1));
    });

    test('falls back to single GET when server ignores Range', () async {
      final server = await _RangeServer.bind(payload, supportRange: false);
      addTearDown(server.close);

      final fetcher = HttpFetcher();
      addTearDown(fetcher.close);

      final verified = await downloadArtifact(
        artifact([server.url]),
        fetcher: fetcher,
        destinationDir: temp,
        policy: const UpdatePolicy(
          downloadConcurrency: 8,
          downloadChunkSize: 16 * 1024,
        ),
      );

      expect(await verified.file.readAsBytes(), payload);
    });

    test('retries a flaky URL then succeeds', () async {
      final server = await _RangeServer.bind(
        payload,
        supportRange: false,
        failTimes: 2,
      );
      addTearDown(server.close);

      final fetcher = HttpFetcher();
      addTearDown(fetcher.close);

      final verified = await downloadArtifact(
        artifact([server.url]),
        fetcher: fetcher,
        destinationDir: temp,
        policy: const UpdatePolicy(
          downloadConcurrency: 1,
          downloadRetries: 4,
          downloadRetryBackoff: Duration(milliseconds: 10),
        ),
      );

      expect(await verified.file.readAsBytes(), payload);
      expect(server.failuresServed, 2);
    });

    test('moves to the next mirror after retries are exhausted', () async {
      final bad = await _RangeServer.bind(
        payload,
        supportRange: false,
        failTimes: 100,
      );
      addTearDown(bad.close);
      final good = await _RangeServer.bind(payload, supportRange: false);
      addTearDown(good.close);

      final fetcher = HttpFetcher();
      addTearDown(fetcher.close);

      final verified = await downloadArtifact(
        artifact([bad.url, good.url]),
        fetcher: fetcher,
        destinationDir: temp,
        policy: const UpdatePolicy(
          downloadConcurrency: 1,
          downloadRetries: 2,
          downloadRetryBackoff: Duration(milliseconds: 5),
        ),
      );

      expect(await verified.file.readAsBytes(), payload);
    });

    test('resumes parallel download from .part.meta', () async {
      final server = await _RangeServer.bind(payload, supportRange: true);
      addTearDown(server.close);

      final chunk = 32 * 1024;
      final firstEnd = chunk - 1;
      final partial = File('${temp.path}${Platform.pathSeparator}app.bin.part');
      final meta = File('${temp.path}${Platform.pathSeparator}app.bin.part.meta');

      // Pre-size and write first chunk.
      await partial.writeAsBytes(Uint8List(payload.length), flush: true);
      final raf = await partial.open(mode: FileMode.append);
      try {
        await raf.setPosition(0);
        await raf.writeFrom(payload.sublist(0, chunk));
        await raf.flush();
      } finally {
        await raf.close();
      }
      await meta.writeAsString(
        '{"sha256":"$digest","size":${payload.length},'
        '"url":"${server.url}","mode":"parallel",'
        '"completed":[[0,$firstEnd]]}',
        flush: true,
      );

      final fetcher = HttpFetcher();
      addTearDown(fetcher.close);

      final seenStarts = <int>{};
      server.onRange = (header) {
        final m = RegExp(r'bytes=(\d+)-').firstMatch(header);
        if (m != null) seenStarts.add(int.parse(m.group(1)!));
      };

      final verified = await downloadArtifact(
        artifact([server.url]),
        fetcher: fetcher,
        destinationDir: temp,
        policy: UpdatePolicy(
          downloadConcurrency: 4,
          downloadChunkSize: chunk,
        ),
      );

      expect(await verified.file.readAsBytes(), payload);
      expect(seenStarts.contains(0), isFalse,
          reason: 'first chunk should already be complete');
    });
  });
}

/// Minimal static file server with optional Range and injected 503s.
class _RangeServer {
  _RangeServer._(this._server, this._payload, this._supportRange, this._failTimes);

  final HttpServer _server;
  final Uint8List _payload;
  final bool _supportRange;
  final int _failTimes;
  int failuresServed = 0;
  void Function(String rangeHeader)? onRange;

  /// Every request, including the HEAD probe. Lets a test assert that the
  /// network was not touched at all.
  void Function(HttpRequest request)? onRequest;

  String get url => 'http://${_server.address.host}:${_server.port}/app.bin';

  static Future<_RangeServer> bind(
    Uint8List payload, {
    required bool supportRange,
    int failTimes = 0,
  }) async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final wrapper =
        _RangeServer._(server, payload, supportRange, failTimes);
    server.listen(wrapper._handle);
    return wrapper;
  }

  Future<void> close() => _server.close(force: true);

  Future<void> _handle(HttpRequest request) async {
    onRequest?.call(request);
    if (failuresServed < _failTimes) {
      failuresServed++;
      request.response.statusCode = HttpStatus.serviceUnavailable;
      await request.response.close();
      return;
    }

    final range = request.headers.value(HttpHeaders.rangeHeader);
    if (request.method == 'HEAD') {
      request.response.headers.set(
        HttpHeaders.acceptRangesHeader,
        _supportRange ? 'bytes' : 'none',
      );
      request.response.headers.contentLength = _payload.length;
      await request.response.close();
      return;
    }

    if (range != null && _supportRange) {
      onRange?.call(range);
      final match = RegExp(r'bytes=(\d+)-(\d*)').firstMatch(range);
      if (match == null) {
        request.response.statusCode = HttpStatus.badRequest;
        await request.response.close();
        return;
      }
      final start = int.parse(match.group(1)!);
      final endRaw = match.group(2)!;
      final end = endRaw.isEmpty ? _payload.length - 1 : int.parse(endRaw);
      final slice = _payload.sublist(start, end + 1);
      request.response.statusCode = HttpStatus.partialContent;
      request.response.headers.set(
        HttpHeaders.contentRangeHeader,
        'bytes $start-$end/${_payload.length}',
      );
      request.response.headers.contentLength = slice.length;
      request.response.add(slice);
      await request.response.close();
      return;
    }

    // Full body (also the "ignore Range" path when supportRange is false).
    if (range != null) {
      onRange?.call(range);
    }
    request.response.headers.set(HttpHeaders.acceptRangesHeader, 'none');
    request.response.headers.contentLength = _payload.length;
    request.response.statusCode = HttpStatus.ok;
    request.response.add(_payload);
    await request.response.close();
  }
}
