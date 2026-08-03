/// The check-for-update flow (SPEC.md section 12.1).
///
/// The ordering here is the specification, not a preference. Each step either
/// establishes something the next one relies on, or rejects a source outright,
/// and reordering them creates gaps that are invisible in a working
/// deployment: verification before parsing, product and channel before
/// anything is acted on, sequence before the index is adopted.
library;

import 'dart:io';
import 'dart:typed_data';

import 'chain.dart';
import 'download.dart';
import 'envelope.dart';
import 'fetch.dart';
import 'models.dart';
import 'selectors.dart';
import 'state.dart';

/// The outcome of a check.
sealed class UpdateCheckResult {
  const UpdateCheckResult();
}

/// Nothing to do: the client is already on the newest reachable version.
class UpToDate extends UpdateCheckResult {
  const UpToDate({required this.sequence, this.currentIsYanked = false});

  final int sequence;

  /// The running version has been withdrawn but nothing newer is reachable.
  /// Worth telling the user, since the fix is out of their hands.
  final bool currentIsYanked;
}

/// A newer version is available and its artifact has been located.
class UpdateAvailable extends UpdateCheckResult {
  const UpdateAvailable({
    required this.target,
    required this.artifact,
    required this.manifest,
    required this.mandatory,
    required this.remainingHops,
    required this.sequence,
  });

  final VersionNode target;
  final Artifact artifact;
  final Manifest manifest;

  /// The client is below `minSupported`; the host must not offer to postpone.
  final bool mandatory;

  /// How many versions still lie between here and the newest one, this one
  /// included. Greater than one means the chain forces an intermediate hop,
  /// which is worth showing rather than surprising the user with a second
  /// update right after the first.
  final int remainingHops;

  final int sequence;

  bool get isFinalHop => remainingHops == 1;
}

/// No source could be used. Every URL failed, or every one that answered was
/// rejected.
class CheckFailed extends UpdateCheckResult {
  const CheckFailed(this.reason, this.attempts);

  final String reason;

  /// One line per source tried, in order. This is the only artefact that
  /// distinguishes "the server is down" from "the server is fine and rejected
  /// us", which are diagnosed in completely different places.
  final List<String> attempts;

  @override
  String toString() => 'CheckFailed: $reason\n  ${attempts.join('\n  ')}';
}

/// Skipped because not enough time has passed (SPEC.md section 12.2).
class CheckThrottled extends UpdateCheckResult {
  const CheckThrottled(this.nextAllowedAt);

  final DateTime nextAllowedAt;
}

/// Checks for, and downloads, updates for one (product, channel).
class RupUpdater {
  RupUpdater({
    required this.product,
    required this.channel,
    required this.currentCode,
    required this.indexUrls,
    required this.trustedKeys,
    required this.clientSelectors,
    required this.stateStore,
    Fetcher? fetcher,
    this.policy = const UpdatePolicy(),
    this.log,
  }) : fetcher = fetcher ?? HttpFetcher() {
    if (indexUrls.isEmpty) {
      throw ArgumentError.value(indexUrls, 'indexUrls', 'must not be empty');
    }
    if (trustedKeys.isEmpty) {
      // Without a key the signature check can only ever fail, and a client
      // built that way would report "update server broken" forever. Failing at
      // construction puts the error where someone can act on it.
      throw ArgumentError.value(
          trustedKeys, 'trustedKeys', 'must contain at least one public key');
    }
  }

  final String product;
  final String channel;

  /// This build's code. Never pass 0 for "unknown" (SPEC.md section 8.1): a
  /// development build that reports 0 will happily replace itself with a
  /// release. Use a value above every published code instead.
  final int currentCode;

  final List<Uri> indexUrls;
  final TrustedKeys trustedKeys;
  final Map<String, String> clientSelectors;
  final UpdateStateStore stateStore;
  final Fetcher fetcher;
  final UpdatePolicy policy;
  final void Function(String message)? log;

  /// Runs a check.
  ///
  /// Pass [force] for a user-initiated check, which ignores throttling.
  Future<UpdateCheckResult> check({bool force = false}) async {
    final state = await stateStore.load();

    if (!force && !policy.shouldCheck(state)) {
      final wait = state.lastResult == 'error'
          ? policy.afterFailure
          : policy.afterSuccess;
      return CheckThrottled(state.lastCheckAt!.add(wait));
    }

    final attempts = <String>[];
    Index? adopted;

    for (final url in indexUrls) {
      final outcome = await _loadIndexFrom(url, state);
      switch (outcome) {
        case _SourceOk(:final index):
          adopted = index;
        case _SourceRejected(:final why):
          attempts.add('$url: $why');
          continue;
      }
      break;
    }

    if (adopted == null) {
      state
        ..lastCheckAt = DateTime.now()
        ..lastResult = 'error';
      await stateStore.save(state);
      return CheckFailed('no usable index source', attempts);
    }

    state.observeSequence(adopted.sequence.toInt());

    final target = selectNextTarget(adopted, currentCode);
    if (target == null) {
      state
        ..lastCheckAt = DateTime.now()
        ..lastResult = 'up-to-date';
      await stateStore.save(state);
      return UpToDate(
        sequence: adopted.sequence.toInt(),
        currentIsYanked: adopted.versions
            .any((node) => node.code.toInt() == currentCode && node.yanked),
      );
    }

    final Manifest manifest;
    try {
      manifest = await _loadManifest(target);
    } on VerificationException catch (error) {
      attempts.add('manifest for ${target.version}: ${error.message}');
      state
        ..lastCheckAt = DateTime.now()
        ..lastResult = 'error';
      await stateStore.save(state);
      return CheckFailed('could not obtain a valid manifest', attempts);
    }

    final artifact = selectArtifact(manifest, clientSelectors);
    if (artifact == null) {
      state
        ..lastCheckAt = DateTime.now()
        ..lastResult = 'no-artifact';
      await stateStore.save(state);
      return CheckFailed(
        'version ${target.version} has no artifact for this platform',
        [
          ...attempts,
          'client selectors: $clientSelectors',
          'offered: ${manifest.artifacts.map((a) => '${a.id} ${selectorsToMap(a.selectors)}').join(', ')}',
        ],
      );
    }

    state
      ..lastCheckAt = DateTime.now()
      ..lastResult = 'update-available';
    await stateStore.save(state);

    return UpdateAvailable(
      target: target,
      artifact: artifact,
      manifest: manifest,
      mandatory: isMandatory(adopted, currentCode),
      remainingHops: resolveUpgradePath(adopted, currentCode).length,
      sequence: adopted.sequence.toInt(),
    );
  }

  /// Downloads the artifact of an available update and verifies its hash.
  Future<VerifiedFile> download(
    UpdateAvailable update, {
    required Directory destinationDir,
    ProgressCallback? onProgress,
  }) =>
      downloadArtifact(
        update.artifact,
        fetcher: fetcher,
        destinationDir: destinationDir,
        policy: policy,
        onProgress: onProgress,
        log: log,
      );

  /// Marks a version as skipped so later checks stay quiet about it.
  ///
  /// Has no effect on a mandatory update: the host must not offer the choice,
  /// and honouring a stale skip would defeat the floor the publisher set.
  Future<void> skip(VersionNode version) async {
    final state = await stateStore.load();
    state.skipped.add(version.code.toInt());
    await stateStore.save(state);
  }

  Future<bool> isSkipped(VersionNode version) async {
    final state = await stateStore.load();
    return state.skipped.contains(version.code.toInt());
  }

  void close() => fetcher.close();

  Future<_SourceOutcome> _loadIndexFrom(Uri url, UpdateState state) async {
    Uint8List bytes;
    try {
      bytes = await fetcher.getBytes(url, timeout: policy.documentTimeout);
    } on FetchException catch (error) {
      return _SourceRejected(error.message);
    }

    final verified = await openEnvelope(bytes, trustedKeys);
    if (!verified.accepted) {
      // A source that fails verification is unusable, full stop. There is no
      // fallback to reading it unsigned: that fallback is exactly what an
      // attacker who can serve bytes would aim for.
      return _SourceRejected(
          'signature check failed (${verified.rejection!.name}: '
          '${verified.detail})');
    }

    final Index index;
    try {
      index = parseIndex(verified.payload!);
    } on RupFormatException catch (error) {
      return _SourceRejected('malformed index: ${error.message}');
    }

    if (index.product != product) {
      return _SourceRejected(
          'index is for product "${index.product}", expected "$product"');
    }
    if (index.channel != channel) {
      return _SourceRejected(
          'index is for channel "${index.channel}", expected "$channel"');
    }

    final sequence = index.sequence.toInt();
    if (!acceptsSequence(sequence, state.lastSeenSequence)) {
      // Not an error the user should see. A mirror lagging behind another is
      // ordinary, and it resolves itself once replication catches up.
      log?.call('$url is behind (sequence $sequence < '
          '${state.lastSeenSequence}), trying the next source');
      return _SourceRejected('sequence $sequence is older than the '
          'last accepted ${state.lastSeenSequence}');
    }

    return _SourceOk(index);
  }

  /// Fetches the manifest, trying mirrors in order.
  ///
  /// The digest comes from the signed index, so a manifest that matches it is
  /// as trustworthy as the index itself. That is why the manifest carries no
  /// signature of its own: one signature over a document that pins the hash of
  /// everything else is both cheaper and harder to get wrong.
  Future<Manifest> _loadManifest(VersionNode target) async {
    final failures = <String>[];

    for (final rawUrl in target.manifest.urls) {
      final url = Uri.tryParse(rawUrl);
      if (url == null) {
        failures.add('$rawUrl: not a valid URL');
        continue;
      }

      Uint8List bytes;
      try {
        bytes = await fetcher.getBytes(url, timeout: policy.documentTimeout);
      } on FetchException catch (error) {
        failures.add('$rawUrl: ${error.message}');
        continue;
      }

      final expectedSize = target.manifest.size.toInt();
      if (bytes.length != expectedSize) {
        failures.add('$rawUrl: expected $expectedSize bytes, '
            'got ${bytes.length}');
        continue;
      }
      final digest = sha256OfBytes(bytes);
      if (digest != target.manifest.sha256) {
        failures.add('$rawUrl: sha256 mismatch');
        continue;
      }

      final Manifest manifest;
      try {
        manifest = parseManifest(bytes);
      } on RupFormatException catch (error) {
        failures.add('$rawUrl: ${error.message}');
        continue;
      }

      // The hash already proves these bytes are what the index pinned, so a
      // mismatch here means the publisher assembled two documents that
      // disagree. Refusing is the only safe reading: acting on it would install
      // a different version than the chain says.
      if (manifest.product != product) {
        failures.add('$rawUrl: manifest names product "${manifest.product}"');
        continue;
      }
      final manifestCode = manifest.code.toInt();
      final targetCode = target.code.toInt();
      if (manifestCode != targetCode) {
        failures.add('$rawUrl: manifest code $manifestCode does not match '
            'index node $targetCode');
        continue;
      }
      if (manifest.version != target.version) {
        failures.add('$rawUrl: manifest version "${manifest.version}" does not '
            'match index node "${target.version}"');
        continue;
      }

      return manifest;
    }

    throw VerificationException(
        'no usable manifest for ${target.version}: ${failures.join('; ')}');
  }
}

sealed class _SourceOutcome {
  const _SourceOutcome();
}

class _SourceOk extends _SourceOutcome {
  const _SourceOk(this.index);

  final Index index;
}

class _SourceRejected extends _SourceOutcome {
  const _SourceRejected(this.why);

  final String why;
}
