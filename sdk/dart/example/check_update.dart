/// Checks a real RUP endpoint from the command line.
///
/// Useful for answering "is the server actually serving what I think it is"
/// without building an application around it, and as a worked example of the
/// minimum a host has to supply.
///
///   dart run example/check_update.dart \
///     --index http://releases.example.com/index/demo/stable.pb \
///     --product demo --code 0 \
///     --key selftest-key=cBLBeiXZRvip40uhFn9bSN/iopIFzPk1P5Yk8nSDA7c= \
///     --download out/
library;

import 'dart:io';

import 'package:rup_client/rup_client.dart';

void main(List<String> args) async {
  String? valueOf(String name) {
    final at = args.indexOf(name);
    return at < 0 || at + 1 >= args.length ? null : args[at + 1];
  }

  final indexUrl = valueOf('--index');
  final product = valueOf('--product');
  final keys = args
      .asMap()
      .entries
      .where((e) => e.value == '--key' && e.key + 1 < args.length)
      .map((e) => args[e.key + 1])
      .toList();

  if (indexUrl == null || product == null || keys.isEmpty) {
    stderr.writeln('usage: check_update.dart --index URL --product NAME '
        '--key ID=BASE64 [--key ...] [--channel stable] [--code N] '
        '[--os windows] [--arch x64] [--download DIR]');
    exit(2);
  }

  final trusted = TrustedKeys.fromBase64({
    for (final entry in keys)
      entry.split('=').first: entry.substring(entry.indexOf('=') + 1),
  });

  final updater = RupUpdater(
    product: product,
    channel: valueOf('--channel') ?? 'stable',
    currentCode: int.parse(valueOf('--code') ?? '0'),
    indexUrls: [Uri.parse(indexUrl)],
    trustedKeys: trusted,
    clientSelectors: {
      'os': valueOf('--os') ?? _defaultOs,
      'arch': valueOf('--arch') ?? 'x64',
    },
    // Nothing is persisted: a diagnostic tool that remembered a sequence
    // number would start refusing perfectly good answers on the second run.
    stateStore: MemoryUpdateStateStore(),
    log: stdout.writeln,
  );

  // Forced, because throttling exists to protect a server from an application
  // checking on every launch, not to make a tool run by hand do nothing.
  final result = await updater.check(force: true);

  switch (result) {
    case UpToDate(:final sequence, :final currentIsYanked):
      stdout.writeln('up to date (sequence $sequence)'
          '${currentIsYanked ? ', but this version has been withdrawn' : ''}');

    case UpdateAvailable(
        :final target,
        :final artifact,
        :final mandatory,
        :final remainingHops,
        :final sequence,
      ):
      stdout.writeln(
          'update available: ${target.version} (code ${target.code.toInt()})');
      stdout.writeln('  sequence     $sequence');
      stdout.writeln('  mandatory    $mandatory');
      stdout.writeln('  hops to head $remainingHops');
      stdout.writeln('  artifact     ${artifact.filename} '
          '(${artifact.size.toInt()} bytes, '
          'sha256 ${artifact.sha256.substring(0, 12)}…)');
      for (final url in artifact.urls) {
        stdout.writeln('  url          $url');
      }

      final into = valueOf('--download');
      if (into != null) {
        stdout.writeln('downloading…');
        final verified = await updater.download(
          result,
          destinationDir: Directory(into),
          onProgress: (progress) {
            final expected =
                progress.total > 0 ? progress.total : artifact.size.toInt();
            if (expected > 0) {
              final pct = (progress.received * 100 / expected).toStringAsFixed(0);
              final mbps =
                  (progress.bytesPerSecond / (1024 * 1024)).toStringAsFixed(2);
              stdout.write('\r  $pct%  $mbps MB/s');
            }
          },
        );
        stdout.writeln('\r  verified ${verified.file.path}');
      }

    case CheckFailed(:final reason, :final attempts):
      stderr.writeln('check failed: $reason');
      for (final attempt in attempts) {
        stderr.writeln('  $attempt');
      }
      updater.close();
      exit(1);

    case FallbackRequired(:final manualUrl, :final message, :final mandatory):
      stdout.writeln('fallback urge: $message');
      stdout.writeln('  open $manualUrl');
      stdout.writeln('  mandatory=$mandatory');

    case CheckThrottled(:final nextAllowedAt):
      stdout.writeln('throttled until $nextAllowedAt');
  }

  updater.close();
}

String get _defaultOs {
  if (Platform.isWindows) return 'windows';
  if (Platform.isMacOS) return 'macos';
  return 'linux';
}
