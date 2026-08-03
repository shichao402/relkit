/// Tests for the parts the shared fixtures cannot reach: the order of
/// operations in a check, mirror fallback, and download verification.
///
/// The conformance suite covers the pure decisions (which version, which
/// artifact, is the signature good). What it cannot cover is everything that
/// only exists once there is a network and a disk: that a bad mirror is skipped
/// rather than fatal, that a truncated download is deleted instead of kept,
/// that a stale index does not become a user-visible error. Those are where
/// this client can go wrong without any fixture noticing.
library;

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';
import 'package:cryptography/cryptography.dart' as crypto;
import 'package:fixnum/fixnum.dart';
import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

/// A fetcher backed by a map, which also records what was asked for.
///
/// The request log is the point: several rules in the spec are about what the
/// client must *not* do (no parallel mirror requests, no second source once one
/// works), and those are only observable from the call sequence.
class FakeFetcher implements Fetcher {
  FakeFetcher(this.responses);

  final Map<String, Object> responses;
  final List<String> requested = [];
  int closeCount = 0;

  @override
  Future<Uint8List> getBytes(Uri url, {required Duration timeout}) async {
    requested.add(url.toString());
    final response = responses[url.toString()];
    if (response == null) {
      throw FetchException(url, 'HTTP 404', statusCode: 404);
    }
    if (response is FetchException) throw response;
    return response as Uint8List;
  }

  @override
  Future<ResourceProbe> probe(Uri url, {required Duration timeout}) async {
    requested.add('HEAD $url');
    final response = responses[url.toString()];
    if (response == null) {
      throw FetchException(url, 'HTTP 404', statusCode: 404);
    }
    if (response is FetchException) throw response;
    final bytes = response as Uint8List;
    return ResourceProbe(acceptsRanges: false, contentLength: bytes.length);
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
    requested.add(url.toString());
    final response = responses[url.toString()];
    if (response == null) {
      throw FetchException(url, 'HTTP 404', statusCode: 404);
    }
    if (response is FetchException) throw response;
    final bytes = response as Uint8List;
    if (startOffset > bytes.length) {
      throw FetchException(url, 'start past EOF', statusCode: 416);
    }
    final slice = bytes.sublist(startOffset);
    await destination.parent.create(recursive: true);
    if (startOffset > 0) {
      await destination.writeAsBytes(
        [...await destination.readAsBytes(), ...slice],
        flush: true,
      );
    } else {
      await destination.writeAsBytes(slice, flush: true);
    }
    final total = knownTotal ?? bytes.length;
    onProgress?.call(DownloadProgress(
      received: startOffset + slice.length,
      total: total,
      bytesPerSecond: slice.length.toDouble(),
      eta: Duration.zero,
    ));
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
    requested.add('RANGE $start-$endInclusive $url');
    final response = responses[url.toString()];
    if (response == null) {
      throw FetchException(url, 'HTTP 404', statusCode: 404);
    }
    if (response is FetchException) throw response;
    final bytes = response as Uint8List;
    final slice = bytes.sublist(start, endInclusive + 1);
    await destination.parent.create(recursive: true);
    await destination.writeAsBytes(slice, flush: true);
    onBytes?.call(slice.length);
  }

  @override
  void close() => closeCount++;
}

/// Builds signed protobuf documents the same way the publisher does, so these
/// tests exercise real signature verification rather than a stub of it.
class Publisher {
  Publisher._(this.keyPair, this.publicKeyBytes, this.keyId);

  static Future<Publisher> create({String keyId = 'test-key'}) async {
    final algorithm = crypto.Ed25519();
    final keyPair = await algorithm.newKeyPair();
    final publicKey = await keyPair.extractPublicKey();
    return Publisher._(keyPair, publicKey.bytes, keyId);
  }

  final crypto.SimpleKeyPair keyPair;
  final List<int> publicKeyBytes;
  final String keyId;

  TrustedKeys get trusted => TrustedKeys({keyId: publicKeyBytes});

  Future<Uint8List> sealIndex(Index index) async {
    final payload = Uint8List.fromList(index.writeToBuffer());
    final signature = await crypto.Ed25519().sign(payload, keyPair: keyPair);
    final envelope = Envelope(
      schema: envelopeSchemaId,
      payload: payload,
      signatures: [
        Signature(
          keyId: keyId,
          alg: 'ed25519',
          sig: signature.bytes,
        ),
      ],
    );
    return Uint8List.fromList(envelope.writeToBuffer());
  }
}

String sha256Hex(List<int> bytes) => sha256.convert(bytes).toString();

List<Selector> selectorEntries(Map<String, String> selectors) {
  final entries = selectors.entries.toList()
    ..sort((a, b) => a.key.compareTo(b.key));
  return [
    for (final entry in entries) Selector(key: entry.key, value: entry.value),
  ];
}

List<MetaEntry> metaEntries(Map<String, String> meta) {
  final entries = meta.entries.toList()..sort((a, b) => a.key.compareTo(b.key));
  return [
    for (final entry in entries) MetaEntry(key: entry.key, value: entry.value),
  ];
}

Manifest manifestDoc({
  String product = 'demo',
  String version = '1.1.0',
  int code = 110,
  required List<Artifact> artifacts,
}) =>
    Manifest(
      schema: manifestSchemaId,
      product: product,
      version: version,
      code: Int64(code),
      releasedAt: '2026-07-30T00:00:00Z',
      artifacts: artifacts,
    );

Artifact artifactDoc({
  required String id,
  required String filename,
  required List<int> body,
  required List<String> urls,
  Map<String, String> selectors = const {'os': 'windows', 'arch': 'x64'},
  Map<String, String> meta = const {},
}) =>
    Artifact(
      id: id,
      filename: filename,
      size: Int64(body.length),
      sha256: sha256Hex(body),
      kind: 'archive',
      selectors: selectorEntries(selectors),
      urls: urls,
      meta: metaEntries(meta),
    );

Index indexDoc({
  String product = 'demo',
  String channel = 'stable',
  int sequence = 1,
  int? minSupported,
  required List<VersionNode> versions,
}) =>
    Index(
      schema: indexSchemaId,
      product: product,
      channel: channel,
      sequence: Int64(sequence),
      generatedAt: '2026-07-30T00:00:00Z',
      minSupported: minSupported == null ? null : Int64(minSupported),
      hasMinSupported_7: minSupported != null,
      versions: versions,
    );

VersionNode versionDoc({
  required String version,
  required int code,
  int minFrom = 0,
  bool yanked = false,
  required Uint8List manifestBytes,
  required List<String> urls,
}) =>
    VersionNode(
      version: version,
      code: Int64(code),
      minFrom: Int64(minFrom),
      yanked: yanked,
      manifest: DigestRef(
        sha256: sha256Hex(manifestBytes),
        size: Int64(manifestBytes.length),
        urls: urls,
      ),
    );

Uint8List utf8Bytes(String value) => Uint8List.fromList(utf8.encode(value));

void main() {
  late Publisher publisher;
  late Directory temp;

  setUpAll(() async => publisher = await Publisher.create());
  setUp(() => temp = Directory.systemTemp.createTempSync('rup-test-'));
  tearDown(() {
    if (temp.existsSync()) temp.deleteSync(recursive: true);
  });

  /// A minimal working world: one update from code 100 to 110.
  Future<(FakeFetcher, RupUpdater, Uint8List)> world({
    int currentCode = 100,
    int sequence = 1,
    int? minSupported,
    UpdateState? state,
    Map<String, String> clientSelectors = const {
      'os': 'windows',
      'arch': 'x64'
    },
  }) async {
    final body = utf8Bytes('a release payload' * 64);
    final manifest = Uint8List.fromList(manifestDoc(artifacts: [
      artifactDoc(
        id: 'app-windows-x64',
        filename: 'demo-1.1.0-win-x64.zip',
        body: body,
        urls: ['http://mirror/artifact/demo/1.1.0/demo-1.1.0-win-x64.zip'],
      ),
    ]).writeToBuffer());

    final index = await publisher.sealIndex(indexDoc(
      sequence: sequence,
      minSupported: minSupported,
      versions: [
        versionDoc(
          version: '1.1.0',
          code: 110,
          manifestBytes: manifest,
          urls: ['http://mirror/manifest/demo/1.1.0.pb'],
        ),
      ],
    ));

    final fetcher = FakeFetcher({
      'http://mirror/index/demo/stable.pb': index,
      'http://mirror/manifest/demo/1.1.0.pb': manifest,
      'http://mirror/artifact/demo/1.1.0/demo-1.1.0-win-x64.zip': body,
    });

    final updater = RupUpdater(
      product: 'demo',
      channel: 'stable',
      currentCode: currentCode,
      indexUrls: [Uri.parse('http://mirror/index/demo/stable.pb')],
      trustedKeys: publisher.trusted,
      clientSelectors: clientSelectors,
      stateStore: MemoryUpdateStateStore(state),
      fetcher: fetcher,
    );

    return (fetcher, updater, body);
  }

  group('check', () {
    test('finds an update and downloads a verified artifact', () async {
      final (_, updater, body) = await world();

      final result = await updater.check();
      expect(result, isA<UpdateAvailable>());
      final available = result as UpdateAvailable;
      expect(available.target.version, '1.1.0');
      expect(available.artifact.id, 'app-windows-x64');
      expect(available.mandatory, isFalse);
      expect(available.isFinalHop, isTrue);

      final file = await updater.download(available, destinationDir: temp);
      expect(await file.file.readAsBytes(), body);
      expect(file.file.path, endsWith('demo-1.1.0-win-x64.zip'));
    });

    test('reports up to date when already on the newest version', () async {
      final (_, updater, _) = await world(currentCode: 110);
      expect(await updater.check(), isA<UpToDate>());
    });

    test('flags a mandatory update below minSupported', () async {
      final (_, updater, _) = await world(minSupported: 110);
      final result = await updater.check() as UpdateAvailable;
      expect(result.mandatory, isTrue);
    });

    test('records the sequence it accepted', () async {
      final store = MemoryUpdateStateStore();
      final body = utf8Bytes('payload');
      final manifest = Uint8List.fromList(manifestDoc(artifacts: [
        artifactDoc(
          id: 'a',
          filename: 'a.zip',
          body: body,
          urls: ['http://m/a.zip'],
        ),
      ]).writeToBuffer());
      final index = await publisher.sealIndex(indexDoc(
        sequence: 7,
        versions: [
          versionDoc(
            version: '1.1.0',
            code: 110,
            manifestBytes: manifest,
            urls: ['http://m/m.pb'],
          ),
        ],
      ));

      final updater = RupUpdater(
        product: 'demo',
        channel: 'stable',
        currentCode: 100,
        indexUrls: [Uri.parse('http://m/i.pb')],
        trustedKeys: publisher.trusted,
        clientSelectors: const {'os': 'windows', 'arch': 'x64'},
        stateStore: store,
        fetcher: FakeFetcher({
          'http://m/i.pb': index,
          'http://m/m.pb': manifest,
          'http://m/a.zip': body,
        }),
      );

      await updater.check();
      expect((await store.load()).lastSeenSequence, 7);
    });
  });

  group('source selection', () {
    test('falls through to the next index URL when one is unreachable',
        () async {
      final body = utf8Bytes('payload');
      final manifest = Uint8List.fromList(manifestDoc(artifacts: [
        artifactDoc(
          id: 'a',
          filename: 'a.zip',
          body: body,
          urls: ['http://m/a.zip'],
        ),
      ]).writeToBuffer());
      final index = await publisher.sealIndex(indexDoc(versions: [
        versionDoc(
          version: '1.1.0',
          code: 110,
          manifestBytes: manifest,
          urls: ['http://m/m.pb'],
        ),
      ]));

      final fetcher = FakeFetcher({
        'http://good/i.pb': index,
        'http://m/m.pb': manifest,
        'http://m/a.zip': body,
      });
      final updater = RupUpdater(
        product: 'demo',
        channel: 'stable',
        currentCode: 100,
        indexUrls: [
          Uri.parse('http://dead/i.pb'),
          Uri.parse('http://good/i.pb'),
        ],
        trustedKeys: publisher.trusted,
        clientSelectors: const {'os': 'windows', 'arch': 'x64'},
        stateStore: MemoryUpdateStateStore(),
        fetcher: fetcher,
      );

      expect(await updater.check(), isA<UpdateAvailable>());
      expect(
        fetcher.requested.take(2),
        ['http://dead/i.pb', 'http://good/i.pb'],
      );
    });

    test('stops at the first usable source', () async {
      final (fetcher, updater, _) = await world();
      await updater.check();
      expect(
        fetcher.requested.where((url) => url.contains('/index/')).length,
        1,
        reason: 'a working source must not be followed by a second request',
      );
    });

    test('rejects a source signed by an untrusted key', () async {
      final other = await Publisher.create(keyId: 'someone-else');
      final body = utf8Bytes('payload');
      final manifest = Uint8List.fromList(manifestDoc(artifacts: [
        artifactDoc(
          id: 'a',
          filename: 'a.zip',
          body: body,
          urls: ['http://m/a.zip'],
        ),
      ]).writeToBuffer());
      final index = await other.sealIndex(indexDoc(versions: [
        versionDoc(
          version: '1.1.0',
          code: 110,
          manifestBytes: manifest,
          urls: ['http://m/m.pb'],
        ),
      ]));

      final updater = RupUpdater(
        product: 'demo',
        channel: 'stable',
        currentCode: 100,
        indexUrls: [Uri.parse('http://m/i.pb')],
        trustedKeys: publisher.trusted,
        clientSelectors: const {'os': 'windows', 'arch': 'x64'},
        stateStore: MemoryUpdateStateStore(),
        fetcher: FakeFetcher({'http://m/i.pb': index}),
      );

      final result = await updater.check();
      expect(result, isA<CheckFailed>());
      expect((result as CheckFailed).attempts.single, contains('signature'));
    });

    test('rejects an index for a different product', () async {
      final body = utf8Bytes('payload');
      final manifest = Uint8List.fromList(manifestDoc(
        product: 'other',
        artifacts: [
          artifactDoc(
            id: 'a',
            filename: 'a.zip',
            body: body,
            urls: ['http://m/a.zip'],
          ),
        ],
      ).writeToBuffer());
      final index = await publisher.sealIndex(indexDoc(
        product: 'other',
        versions: [
          versionDoc(
            version: '1.1.0',
            code: 110,
            manifestBytes: manifest,
            urls: ['http://m/m.pb'],
          ),
        ],
      ));

      final updater = RupUpdater(
        product: 'demo',
        channel: 'stable',
        currentCode: 100,
        indexUrls: [Uri.parse('http://m/i.pb')],
        trustedKeys: publisher.trusted,
        clientSelectors: const {'os': 'windows', 'arch': 'x64'},
        stateStore: MemoryUpdateStateStore(),
        fetcher: FakeFetcher({'http://m/i.pb': index}),
      );

      final result = await updater.check() as CheckFailed;
      expect(result.attempts.single, contains('product "other"'));
    });

    test('a stale mirror is skipped, and a fresh one still wins', () async {
      final body = utf8Bytes('payload');
      final manifest = Uint8List.fromList(manifestDoc(artifacts: [
        artifactDoc(
          id: 'a',
          filename: 'a.zip',
          body: body,
          urls: ['http://m/a.zip'],
        ),
      ]).writeToBuffer());
      final versions = [
        versionDoc(
          version: '1.1.0',
          code: 110,
          manifestBytes: manifest,
          urls: ['http://m/m.pb'],
        ),
      ];
      final stale =
          await publisher.sealIndex(indexDoc(sequence: 3, versions: versions));
      final fresh =
          await publisher.sealIndex(indexDoc(sequence: 9, versions: versions));

      final updater = RupUpdater(
        product: 'demo',
        channel: 'stable',
        currentCode: 100,
        indexUrls: [
          Uri.parse('http://stale/i.pb'),
          Uri.parse('http://fresh/i.pb'),
        ],
        trustedKeys: publisher.trusted,
        clientSelectors: const {'os': 'windows', 'arch': 'x64'},
        stateStore: MemoryUpdateStateStore(UpdateState(lastSeenSequence: 5)),
        fetcher: FakeFetcher({
          'http://stale/i.pb': stale,
          'http://fresh/i.pb': fresh,
          'http://m/m.pb': manifest,
          'http://m/a.zip': body,
        }),
      );

      final result = await updater.check();
      expect(result, isA<UpdateAvailable>());
      expect((result as UpdateAvailable).sequence, 9);
    });
  });

  group('manifest verification', () {
    test('refuses a manifest whose hash does not match the index', () async {
      final body = utf8Bytes('payload');
      final real = Uint8List.fromList(manifestDoc(artifacts: [
        artifactDoc(
          id: 'a',
          filename: 'a.zip',
          body: body,
          urls: ['http://m/a.zip'],
        ),
      ]).writeToBuffer());

      final swapped = Uint8List.fromList(manifestDoc(artifacts: [
        artifactDoc(
          id: 'a',
          filename: 'a.zip',
          body: utf8.encode('PAYLOAD'),
          urls: ['http://m/a.zip'],
        ),
      ]).writeToBuffer());
      expect(swapped.length, real.length,
          reason: 'the test needs both manifests to be the same size');

      final index = await publisher.sealIndex(indexDoc(versions: [
        versionDoc(
          version: '1.1.0',
          code: 110,
          manifestBytes: real,
          urls: ['http://m/m.pb'],
        ),
      ]));

      final updater = RupUpdater(
        product: 'demo',
        channel: 'stable',
        currentCode: 100,
        indexUrls: [Uri.parse('http://m/i.pb')],
        trustedKeys: publisher.trusted,
        clientSelectors: const {'os': 'windows', 'arch': 'x64'},
        stateStore: MemoryUpdateStateStore(),
        fetcher: FakeFetcher({
          'http://m/i.pb': index,
          'http://m/m.pb': swapped,
        }),
      );

      final result = await updater.check();
      expect(result, isA<CheckFailed>());
      expect(result.toString(), contains('sha256 mismatch'));
    });

    test('refuses a manifest of the wrong size before hashing it', () async {
      final body = utf8Bytes('payload');
      final real = Uint8List.fromList(manifestDoc(artifacts: [
        artifactDoc(
          id: 'a',
          filename: 'a.zip',
          body: body,
          urls: ['http://m/a.zip'],
        ),
      ]).writeToBuffer());
      final truncated = Uint8List.fromList(real.take(real.length - 5).toList());

      final index = await publisher.sealIndex(indexDoc(versions: [
        versionDoc(
          version: '1.1.0',
          code: 110,
          manifestBytes: real,
          urls: ['http://m/m.pb'],
        ),
      ]));

      final updater = RupUpdater(
        product: 'demo',
        channel: 'stable',
        currentCode: 100,
        indexUrls: [Uri.parse('http://m/i.pb')],
        trustedKeys: publisher.trusted,
        clientSelectors: const {'os': 'windows', 'arch': 'x64'},
        stateStore: MemoryUpdateStateStore(),
        fetcher: FakeFetcher({
          'http://m/i.pb': index,
          'http://m/m.pb': truncated,
        }),
      );

      expect((await updater.check()).toString(), contains('bytes, got'));
    });

    test('refuses a manifest whose code disagrees with the index node',
        () async {
      final body = utf8Bytes('payload');
      final manifest = Uint8List.fromList(manifestDoc(code: 999, artifacts: [
        artifactDoc(
          id: 'a',
          filename: 'a.zip',
          body: body,
          urls: ['http://m/a.zip'],
        ),
      ]).writeToBuffer());
      final index = await publisher.sealIndex(indexDoc(versions: [
        versionDoc(
          version: '1.1.0',
          code: 110,
          manifestBytes: manifest,
          urls: ['http://m/m.pb'],
        ),
      ]));

      final updater = RupUpdater(
        product: 'demo',
        channel: 'stable',
        currentCode: 100,
        indexUrls: [Uri.parse('http://m/i.pb')],
        trustedKeys: publisher.trusted,
        clientSelectors: const {'os': 'windows', 'arch': 'x64'},
        stateStore: MemoryUpdateStateStore(),
        fetcher: FakeFetcher({
          'http://m/i.pb': index,
          'http://m/m.pb': manifest,
        }),
      );

      expect((await updater.check()).toString(), contains('does not match'));
    });

    test('reports a platform with no artifact distinctly from a failure',
        () async {
      final (_, updater, _) =
          await world(clientSelectors: const {'os': 'linux', 'arch': 'x64'});
      final result = await updater.check();
      expect(result, isA<CheckFailed>());
      expect((result as CheckFailed).reason, contains('no artifact'));
      expect(result.attempts.join(' '), contains('client selectors'));
    });
  });

  group('download', () {
    late Artifact artifact;
    late Uint8List body;

    setUp(() {
      body = utf8Bytes('the real bytes' * 32);
      artifact = parseManifest(Uint8List.fromList(manifestDoc(artifacts: [
        artifactDoc(
          id: 'a',
          filename: 'app.zip',
          body: body,
          urls: ['http://one/app.zip', 'http://two/app.zip'],
        ),
      ]).writeToBuffer()))
          .artifacts
          .single;
    });

    test('falls back to the second mirror and leaves no partial file',
        () async {
      final fetcher = FakeFetcher({'http://two/app.zip': body});
      final result = await downloadArtifact(
        artifact,
        fetcher: fetcher,
        destinationDir: temp,
        policy: const UpdatePolicy(downloadConcurrency: 1),
      );

      expect(await result.file.readAsBytes(), body);
      expect(result.sourceUrl.toString(), 'http://two/app.zip');
      expect(File('${result.file.path}.part').existsSync(), isFalse);
    });

    test('deletes a file whose hash is wrong rather than keeping it', () async {
      final wrong = utf8Bytes('x' * body.length);
      final fetcher = FakeFetcher({'http://one/app.zip': wrong});

      await expectLater(
        downloadArtifact(
          artifact,
          fetcher: fetcher,
          destinationDir: temp,
          policy: const UpdatePolicy(downloadConcurrency: 1),
        ),
        throwsA(isA<VerificationException>()),
      );
      expect(temp.listSync(), isEmpty,
          reason: 'a rejected download must not survive under any name');
    });

    test('rejects a truncated file on size before hashing it', () async {
      final short = utf8Bytes('short');
      final fetcher = FakeFetcher({'http://one/app.zip': short});

      await expectLater(
        downloadArtifact(
          artifact,
          fetcher: fetcher,
          destinationDir: temp,
          policy: const UpdatePolicy(downloadConcurrency: 1),
        ),
        throwsA(predicate((e) =>
            e is VerificationException && e.message.contains('bytes, got'))),
      );
    });

    test('tries mirrors one at a time, never in parallel', () async {
      final fetcher = FakeFetcher({'http://two/app.zip': body});
      await downloadArtifact(
        artifact,
        fetcher: fetcher,
        destinationDir: temp,
        policy: const UpdatePolicy(downloadConcurrency: 1),
      );
      expect(fetcher.requested, ['http://one/app.zip', 'http://two/app.zip']);
    });
  });

  group('throttling', () {
    test('skips a check that is too soon, but never a forced one', () async {
      final justChecked = UpdateState(
        lastCheckAt: DateTime.now().subtract(const Duration(minutes: 5)),
        lastResult: 'up-to-date',
      );

      final (_, updater, _) = await world(state: justChecked);
      expect(await updater.check(), isA<CheckThrottled>());
      expect(await updater.check(force: true), isA<UpdateAvailable>());
    });

    test('retries sooner after a failure than after a success', () {
      const policy = UpdatePolicy();
      final twoHoursAgo = DateTime.now().subtract(const Duration(hours: 2));

      expect(
        policy.shouldCheck(
            UpdateState(lastCheckAt: twoHoursAgo, lastResult: 'error')),
        isTrue,
      );
      expect(
        policy.shouldCheck(
            UpdateState(lastCheckAt: twoHoursAgo, lastResult: 'up-to-date')),
        isFalse,
      );
    });
  });

  group('state', () {
    test('survives a round trip through a file', () async {
      final store = FileUpdateStateStore(
          directory: temp, product: 'demo', channel: 'stable');
      final state = UpdateState(lastSeenSequence: 12, skipped: {110, 120})
        ..lastResult = 'up-to-date'
        ..lastCheckAt = DateTime.utc(2026, 7, 30, 12);
      await store.save(state);

      final loaded = await store.load();
      expect(loaded.lastSeenSequence, 12);
      expect(loaded.skipped, {110, 120});
      expect(loaded.lastResult, 'up-to-date');
      expect(loaded.lastCheckAt, DateTime.utc(2026, 7, 30, 12));
    });

    test('a corrupt file degrades to defaults instead of blocking updates',
        () async {
      final store = FileUpdateStateStore(
          directory: temp, product: 'demo', channel: 'stable');
      await store.file.writeAsString('{not json');

      final loaded = await store.load();
      expect(loaded.lastSeenSequence, isNull);
      expect(loaded.skipped, isEmpty);
    });

    test('channels do not share state', () {
      final stable = FileUpdateStateStore(
          directory: temp, product: 'demo', channel: 'stable');
      final beta = FileUpdateStateStore(
          directory: temp, product: 'demo', channel: 'beta');
      expect(stable.file.path, isNot(beta.file.path));
    });

    test('the sequence high-water mark never moves backwards', () {
      final state = UpdateState(lastSeenSequence: 10);
      state.observeSequence(4);
      expect(state.lastSeenSequence, 10);
      state.observeSequence(11);
      expect(state.lastSeenSequence, 11);
    });
  });

  group('parsing', () {
    test('rejects the wrong schema id', () {
      final bytes = Uint8List.fromList(indexDoc(
        versions: [
          versionDoc(
            version: '1.0.0',
            code: 100,
            manifestBytes: utf8Bytes('manifest'),
            urls: ['http://m/m.pb'],
          ),
        ],
      ).copyWith((i) {
        i.schema = 'rup.index/999';
      }).writeToBuffer());

      expect(() => parseIndex(bytes), throwsA(isA<RupFormatException>()));
    });

    test('rejects a malformed sha256', () {
      final bytes = Uint8List.fromList(manifestDoc(artifacts: [
        Artifact(
          id: 'a',
          filename: 'a.zip',
          size: Int64(1),
          sha256: 'ABC123',
          kind: 'archive',
          urls: ['http://m/a.zip'],
        ),
      ]).writeToBuffer());

      expect(() => parseManifest(bytes), throwsA(isA<RupFormatException>()));
    });

    test('rejects an inconsistent minSupported presence bit', () {
      final bytes = Uint8List.fromList(indexDoc(
        versions: [
          versionDoc(
            version: '1.0.0',
            code: 100,
            manifestBytes: utf8Bytes('manifest'),
            urls: ['http://m/m.pb'],
          ),
        ],
      ).copyWith((i) {
        i.minSupported = Int64(123);
        i.hasMinSupported_7 = false;
      }).writeToBuffer());

      expect(() => parseIndex(bytes), throwsA(isA<RupFormatException>()));
    });

    test('converts repeated selectors to a map when matching', () {
      final artifact = Artifact(
        id: 'a',
        filename: 'a.zip',
        size: Int64(1),
        sha256: '1' * 64,
        kind: 'archive',
        selectors: [
          Selector(key: 'os', value: 'windows'),
          Selector(key: 'arch', value: 'x64'),
        ],
        urls: ['http://m/a.zip'],
      );

      expect(selectorsToMap(artifact.selectors), {
        'os': 'windows',
        'arch': 'x64',
      });
    });
  });

  group('construction', () {
    test('refuses to build a client with no trusted keys', () {
      expect(
        () => RupUpdater(
          product: 'demo',
          channel: 'stable',
          currentCode: 1,
          indexUrls: [Uri.parse('http://m/i.pb')],
          trustedKeys: TrustedKeys(const {}),
          clientSelectors: const {},
          stateStore: MemoryUpdateStateStore(),
          fetcher: FakeFetcher(const {}),
        ),
        throwsA(isA<ArgumentError>()),
      );
    });

    test('refuses a public key of the wrong length', () {
      expect(() => TrustedKeys({'k': List.filled(31, 0)}),
          throwsA(isA<ArgumentError>()));
    });
  });
}
