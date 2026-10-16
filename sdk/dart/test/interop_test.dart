/// Cross-implementation test: publish with the real `relkit` binary, consume
/// with this client.
///
/// The shared fixtures prove both implementations agree with the same test
/// data. They cannot prove the two agree with *each other*, because both sides
/// could satisfy every fixture while disagreeing about something no fixture
/// pins down. Those defects surface only when real bytes from one side are fed
/// to the other.
///
/// Skipped when `relkit` is not on PATH, or when the installed `relkit` does
/// not produce protobuf `.pb` outputs yet.
library;

import 'dart:convert';
import 'dart:io';

import 'package:rup_client/rup_client.dart';
import 'package:rup_client/src/updater.dart';
import 'package:test/test.dart';

/// Why the publisher cannot run here, or null when it can.
String? publisherUnavailable() {
  try {
    final probe = Process.runSync('relkit', ['--version']);
    if (probe.exitCode != 0) {
      return 'relkit is not runnable (exit ${probe.exitCode})';
    }
  } catch (error) {
    return 'relkit could not be started ($error); install from '
        'https://github.com/shichao402/relkit/releases or '
        '`go build ./cmd/relkit` from a GitHub clone';
  }

  final project = Directory.systemTemp.createTempSync('rup-relkit-probe-');
  try {
    final init = Process.runSync(
      'relkit',
      ['init', '--product', 'probe'],
      workingDirectory: project.path,
      stdoutEncoding: utf8,
      stderrEncoding: utf8,
    );
    if (init.exitCode != 0) {
      return 'relkit init failed during protobuf-v2 probe\n'
          '${init.stdout}\n${init.stderr}';
    }

    final keygen = Process.runSync(
      'relkit',
      ['keygen', '--key-id', 'probe-2026', '--out', 'keys', '--update-config'],
      workingDirectory: project.path,
      stdoutEncoding: utf8,
      stderrEncoding: utf8,
    );
    if (keygen.exitCode != 0) {
      return 'relkit keygen failed during protobuf-v2 probe\n'
          '${keygen.stdout}\n${keygen.stderr}';
    }

    final sep = Platform.pathSeparator;
    final publicPb =
        File('${project.path}${sep}keys${sep}probe-2026.public.pb');
    final privatePb =
        File('${project.path}${sep}keys${sep}probe-2026.private.pb');
    if (!publicPb.existsSync() || !privatePb.existsSync()) {
      return 'relkit on PATH does not appear to produce protobuf `.pb` key '
          'documents; skipping protobuf-v2 interop';
    }

    return null;
  } finally {
    if (project.existsSync()) project.deleteSync(recursive: true);
  }
}

void main() {
  final unavailable = publisherUnavailable();

  group('a release published by relkit', () {
    late Directory project;
    late HttpServer server;
    late String baseUrl;

    String at(String relative) => '${project.path}${Platform.pathSeparator}'
        '${relative.replaceAll('/', Platform.pathSeparator)}';

    /// Runs relkit, failing loudly with its output when it exits non-zero.
    Future<void> relkit(List<String> args) async {
      final result = await Process.run(
        'relkit',
        args,
        workingDirectory: project.path,
        environment: {
          ...Platform.environment,
          'RELKIT_DART_TEST_TOKEN': 'test-token',
        },
        stdoutEncoding: utf8,
        stderrEncoding: utf8,
      );
      if (result.exitCode != 0) {
        fail('relkit ${args.join(' ')} failed with ${result.exitCode}\n'
            '${result.stdout}\n${result.stderr}');
      }
    }

    setUpAll(() async {
      project = Directory.systemTemp.createTempSync('rup-interop-');

      // Bind before publishing: the base URL is signed into the index, so it
      // has to be right at publish time rather than patched in afterwards.
      server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      baseUrl = 'http://127.0.0.1:${server.port}/';
      final published = <String, List<int>>{};
      server.listen((request) async {
        final key = Uri.decodeComponent(
          request.uri.path.startsWith('/')
              ? request.uri.path.substring(1)
              : request.uri.path,
        );
        if (request.method == 'POST') {
          request.response.statusCode = HttpStatus.notFound;
        } else if (request.method == 'PUT') {
          final source = request.headers.value('X-Relkit-Copy-Source');
          final bytes = source == null
              ? await request.fold<List<int>>(
                  <int>[],
                  (all, chunk) => all..addAll(chunk),
                )
              : published[source];
          if (bytes == null) {
            request.response.statusCode = HttpStatus.notFound;
          } else {
            published[key] = bytes;
            request.response.statusCode = HttpStatus.created;
          }
        } else if (request.method == 'HEAD') {
          final bytes = published[key];
          if (bytes == null) {
            request.response.statusCode = HttpStatus.notFound;
          } else {
            request.response.contentLength = bytes.length;
          }
        } else if (request.method == 'DELETE') {
          published.remove(key);
          request.response.statusCode = HttpStatus.noContent;
        } else {
          final bytes = published[key];
          if (bytes == null) {
            request.response.statusCode = HttpStatus.notFound;
          } else {
            request.response.headers.contentType = ContentType.binary;
            request.response.contentLength = bytes.length;
            request.response.add(bytes);
          }
        }
        await request.response.close();
      });

      await relkit(['init', '--product', 'demo']);
      await relkit([
        'keygen',
        '--key-id',
        'release-2026',
        '--out',
        'keys',
        '--update-config',
      ]);

      final configFile = File(at('relkit.json'));
      final config =
          json.decode(configFile.readAsStringSync()) as Map<String, dynamic>;
      config['codeStrategy'] = 'explicit';
      (config['backends'] as Map)['local'] = {
        'type': 'relkit-compatible',
        'baseUrl': baseUrl,
        'tokenEnv': 'RELKIT_DART_TEST_TOKEN',
      };
      config['publishTo'] = ['local'];
      (config['signing'] as Map)['privateKeyPath'] =
          'keys/release-2026.private.pb';
      configFile.writeAsStringSync(json.encode(config));

      Directory(at('dist')).createSync(recursive: true);

      File(at('dist/demo-1.0.0-win-x64.zip'))
          .writeAsStringSync('release one payload' * 200);
      await relkit([
        'stage',
        '1.0.0',
        '--code',
        '100',
        '--install',
        'dist/demo-1.0.0-win-x64.zip',
        'os=windows,arch=x64',
      ]);
      await relkit(['publish', '1.0.0']);

      File(at('dist/demo-1.5.0-win-x64.zip'))
          .writeAsStringSync('release two payload' * 300);
      await relkit([
        'stage',
        '1.5.0',
        '--code',
        '150',
        '--min-from',
        '100',
        '--install',
        'dist/demo-1.5.0-win-x64.zip',
        'os=windows,arch=x64',
      ]);
      await relkit(['publish', '1.5.0']);

      if (!published.containsKey('index/demo/stable.pb')) {
        fail('relkit did not publish index/demo/stable.pb. '
            'Install a protobuf-v2 relkit build to run interop_test.dart.');
      }
    });

    tearDownAll(() async {
      await server.close(force: true);
      if (project.existsSync()) project.deleteSync(recursive: true);
    });

    TrustedKeys embeddedKeys() {
      final key = PublicKeyDocument.fromBuffer(
          File(at('keys/release-2026.public.pb')).readAsBytesSync());
      return TrustedKeys({key.keyId: key.publicKey});
    }

    RupUpdater updaterAt(int currentCode, {TrustedKeys? keys}) {
      final updater = RupUpdater(
        product: 'demo',
        channel: 'stable',
        currentCode: currentCode,
        indexUrls: [Uri.parse('${baseUrl}index/demo/stable.pb')],
        trustedKeys: keys ?? embeddedKeys(),
        clientSelectors: const {'os': 'windows', 'arch': 'x64'},
        stateStore: MemoryUpdateStateStore(),
      );
      addTearDown(updater.close);
      return updater;
    }

    test('verifies a signature produced by the relkit publisher', () async {
      final result = await updaterAt(0).check();
      expect(result, isA<UpdateAvailable>(),
          reason: 'the client rejected a genuine release: $result');
    });

    test('walks the chain instead of jumping to the newest version', () async {
      final first = await updaterAt(0).check() as UpdateAvailable;
      expect(first.target.version, '1.0.0');
      expect(first.remainingHops, 2);
      expect(first.isFinalHop, isFalse);

      final second = await updaterAt(100).check() as UpdateAvailable;
      expect(second.target.version, '1.5.0');
      expect(second.isFinalHop, isTrue);
    });

    test('downloads over real HTTP and the bytes verify', () async {
      final updater = updaterAt(100);
      final available = await updater.check() as UpdateAvailable;

      final verified = await updater.download(
        available,
        destinationDir: Directory(at('downloads')),
      );

      expect(verified.file.existsSync(), isTrue);
      expect(await verified.file.length(), available.artifact.size.toInt());
      expect(await sha256OfFile(verified.file), available.artifact.sha256);
      expect(
        await verified.file.readAsBytes(),
        await File(at('dist/demo-1.5.0-win-x64.zip')).readAsBytes(),
        reason: 'the delivered bytes must be the staged bytes',
      );
    });

    test('reports up to date at the head of the chain', () async {
      expect(await updaterAt(150).check(), isA<UpToDate>());
    });

    test('rejects a genuine release when the key is not the embedded one',
        () async {
      final wrongKey = TrustedKeys.fromBase64({
        'release-2026': base64.encode(List.filled(32, 7)),
      });

      final result = await updaterAt(0, keys: wrongKey).check();
      expect(result, isA<CheckFailed>());
      expect((result as CheckFailed).attempts.single, contains('signature'));
    });
  }, skip: unavailable);
}
