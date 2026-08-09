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
import 'preference.dart';
import 'release_notes.dart';
import 'selectors.dart';
import 'state.dart';

/// Preference key for a directory service id (SPEC §12.7 / §16).
String directoryServiceKey(String id) => 'service:$id';

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
    this.priorReleaseNotes = const <PriorReleaseNotes>[],
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

  /// Earlier releases: prefer [PriorReleaseNotes.notesUrl] over inlined bodies.
  final List<PriorReleaseNotes> priorReleaseNotes;

  bool get isFinalHop => remainingHops == 1;

  /// Markdown release notes for [target] (falls back to [manifest].notes).
  String get releaseNotesMarkdown => resolveReleaseNotesMarkdown(
        target: target,
        manifest: manifest,
      );

  /// Repository / changelog URL for [target], if the publisher set one.
  String get releaseNotesUrl => target.notesUrl.trim();
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

/// A signed emergency notice matches this build (SPEC.md section 12.6).
///
/// The host should show [message] and open [manualUrl] in a browser. This never
/// carries an auto-download; that is the point of the escape hatch.
class FallbackRequired extends UpdateCheckResult {
  const FallbackRequired({
    required this.manualUrl,
    required this.message,
    required this.mandatory,
    required this.sequence,
    required this.minCode,
    required this.maxCode,
  });

  final String manualUrl;
  final String message;
  final bool mandatory;
  final int sequence;
  final int minCode;
  final int maxCode;
}

/// Checks for, and downloads, updates for one (product, channel).
class RupUpdater {
  RupUpdater({
    required this.product,
    required this.channel,
    required this.currentCode,
    required this.trustedKeys,
    required this.clientSelectors,
    required this.stateStore,
    List<Uri>? indexUrls,
    List<Uri>? entryUrls,
    List<Uri>? fallbackUrls,
    Fetcher? fetcher,
    this.policy = const UpdatePolicy(),
    this.log,
  })  : indexUrls = List.unmodifiable(indexUrls ?? const <Uri>[]),
        entryUrls = List.unmodifiable(entryUrls ?? const <Uri>[]),
        fallbackUrls = List.unmodifiable(fallbackUrls ?? const <Uri>[]),
        fetcher = fetcher ?? HttpFetcher() {
    if (this.indexUrls.isEmpty && this.entryUrls.isEmpty) {
      throw ArgumentError(
          'indexUrls or entryUrls must contain at least one URL');
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

  /// Direct index mirrors (compatibility / hosts that skip directory).
  /// Ignored for the check path when [entryUrls] is non-empty (SPEC §12.1).
  final List<Uri> indexUrls;

  /// Signed directory entry URLs (primary → backups). Preferred bootstrap path.
  final List<Uri> entryUrls;

  /// Optional signed fallback notice URLs (SPEC section 12.6). Empty disables
  /// unless a loaded directory supplies `fallback_url` values.
  final List<Uri> fallbackUrls;

  final TrustedKeys trustedKeys;
  final Map<String, String> clientSelectors;
  final UpdateStateStore stateStore;
  final Fetcher fetcher;
  final UpdatePolicy policy;
  final void Function(String message)? log;

  /// Last directory adopted during [check] / [checkFallback] in this process.
  UpdateDirectory? _lastDirectory;

  /// Runs a check.
  ///
  /// Pass [force] for a user-initiated check, which ignores throttling.
  /// When [fallbackUrls] is set, also evaluates the fallback document and
  /// merges: [UpdateAvailable] > [FallbackRequired] > [UpToDate] / [CheckFailed].
  Future<UpdateCheckResult> check({bool force = false}) async {
    final normal = await _checkIndex(force: force);
    final fallback = await checkFallback();
    if (normal is UpdateAvailable) return normal;
    if (fallback != null) return fallback;
    return normal;
  }

  /// Evaluates only the signed fallback document (SPEC section 12.6).
  ///
  /// Returns null when no rule matches or no source can be used. Hosts should
  /// call this after a download/apply failure to urge a manual update.
  Future<FallbackRequired?> checkFallback() async {
    final state = await stateStore.load();
    final urls = await _resolveFallbackUrls(state);
    if (urls.isEmpty) return null;

    final attempts = <String>[];

    for (final url in urls) {
      final outcome = await _loadFallbackFrom(url, state);
      switch (outcome) {
        case _FallbackSourceOk(:final doc):
          state
            ..recordSourceSuccess(url.toString())
            ..observeFallbackSequence(doc.sequence.toInt());
          await stateStore.save(state);
          final rule = _matchFallbackRule(doc);
          if (rule == null) return null;
          return FallbackRequired(
            manualUrl: rule.manualUrl,
            message: rule.message,
            mandatory: rule.mandatory,
            sequence: doc.sequence.toInt(),
            minCode: rule.minCode.toInt(),
            maxCode: rule.maxCode.toInt(),
          );
        case _FallbackSourceRejected(:final why):
          state.recordSourceFailure(url.toString());
          attempts.add('$url: $why');
          continue;
      }
    }

    await stateStore.save(state);
    log?.call('fallback check failed: ${attempts.join('; ')}');
    return null;
  }

  Future<UpdateCheckResult> _checkIndex({bool force = false}) async {
    final state = await stateStore.load();

    if (!force && !policy.shouldCheck(state)) {
      final wait = state.lastResult == 'error'
          ? policy.afterFailure
          : policy.afterSuccess;
      return CheckThrottled(state.lastCheckAt!.add(wait));
    }

    final attempts = <String>[];
    final plan = await _resolveIndexPlan(state, attempts);
    if (plan == null || plan.isEmpty) {
      state
        ..lastCheckAt = DateTime.now()
        ..lastResult = 'error';
      await stateStore.save(state);
      return CheckFailed('no usable directory or index source', attempts);
    }

    Index? adopted;
    String? adoptedKey;

    for (final candidate in plan) {
      final outcome = await _loadIndexFrom(candidate.url, state);
      switch (outcome) {
        case _SourceOk(:final index):
          adopted = index;
          adoptedKey = candidate.preferenceKey;
          state.recordSourceSuccess(candidate.preferenceKey);
        case _SourceRejected(:final why):
          state.recordSourceFailure(candidate.preferenceKey);
          attempts.add('${candidate.url}: $why');
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
      manifest = await _loadManifest(target, state);
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

    final rankedArtifact = artifact.clone()
      ..urls.clear()
      ..urls.addAll(rankUrlStrings(artifact.urls, state));

    state
      ..lastCheckAt = DateTime.now()
      ..lastResult = 'update-available';
    await stateStore.save(state);

    log?.call(
        'adopted index via $adoptedKey (sequence ${adopted.sequence.toInt()})');

    return UpdateAvailable(
      target: target,
      artifact: rankedArtifact,
      manifest: manifest,
      mandatory: isMandatory(adopted, currentCode),
      remainingHops: resolveUpgradePath(adopted, currentCode).length,
      sequence: adopted.sequence.toInt(),
      priorReleaseNotes: collectPriorReleaseNotes(
        adopted,
        targetCode: target.code.toInt(),
      ),
    );
  }

  /// Downloads the artifact of an available update and verifies its hash.
  Future<VerifiedFile> download(
    UpdateAvailable update, {
    required Directory destinationDir,
    ProgressCallback? onProgress,
  }) async {
    final state = await stateStore.load();
    final ranked = update.artifact.clone()
      ..urls.clear()
      ..urls.addAll(rankUrlStrings(update.artifact.urls, state));

    var lastBps = 0;
    final verified = await downloadArtifact(
      ranked,
      fetcher: fetcher,
      destinationDir: destinationDir,
      policy: policy,
      onProgress: (progress) {
        if (progress.bytesPerSecond > 0) {
          lastBps = progress.bytesPerSecond.round();
        }
        onProgress?.call(progress);
      },
      log: log,
    );

    if (!verified.sourceUrl.isScheme('file')) {
      state.recordSourceSuccess(
        verified.sourceUrl.toString(),
        bytesPerSecond: lastBps > 0 ? lastBps : null,
      );
      await stateStore.save(state);
    }
    return verified;
  }

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

  Future<List<_IndexCandidate>?> _resolveIndexPlan(
    UpdateState state,
    List<String> attempts,
  ) async {
    if (entryUrls.isEmpty) {
      return [
        for (final url in rankUris(indexUrls, state))
          _IndexCandidate(url: url, preferenceKey: url.toString()),
      ];
    }

    for (final entry in rankUris(entryUrls, state)) {
      final outcome = await _loadDirectoryFrom(entry, state);
      switch (outcome) {
        case _DirectorySourceOk(:final doc):
          _lastDirectory = doc;
          state
            ..recordSourceSuccess(entry.toString())
            ..observeDirectorySequence(doc.directorySequence.toInt());
          await stateStore.save(state);
          final services = _servicesForChannel(doc);
          if (services.isEmpty) {
            attempts.add('$entry: no services for channel "$channel"');
            continue;
          }
          final ranked = rankByLearning<DirectoryService>(
            items: services,
            keyOf: (service) => directoryServiceKey(service.id),
            state: state,
          );
          final plan = <_IndexCandidate>[];
          for (final service in ranked) {
            final url = Uri.tryParse(service.indexUrl);
            if (url == null) {
              attempts.add(
                  '$entry service ${service.id}: bad indexUrl ${service.indexUrl}');
              continue;
            }
            plan.add(_IndexCandidate(
              url: url,
              preferenceKey: directoryServiceKey(service.id),
            ));
          }
          if (plan.isEmpty) {
            attempts.add('$entry: no usable indexUrl in directory');
            continue;
          }
          return plan;
        case _DirectorySourceRejected(:final why):
          state.recordSourceFailure(entry.toString());
          attempts.add('$entry: $why');
          continue;
      }
    }
    return null;
  }

  Future<List<Uri>> _resolveFallbackUrls(UpdateState state) async {
    if (fallbackUrls.isNotEmpty) {
      return rankUris(fallbackUrls, state);
    }
    if (entryUrls.isEmpty) return const [];

    var doc = _lastDirectory;
    if (doc == null) {
      for (final entry in rankUris(entryUrls, state)) {
        final outcome = await _loadDirectoryFrom(entry, state);
        if (outcome case _DirectorySourceOk(:final doc)) {
          _lastDirectory = doc;
          state
            ..recordSourceSuccess(entry.toString())
            ..observeDirectorySequence(doc.directorySequence.toInt());
          await stateStore.save(state);
          break;
        }
        if (outcome case _DirectorySourceRejected()) {
          state.recordSourceFailure(entry.toString());
        }
      }
      doc = _lastDirectory;
    }
    if (doc == null) return const [];

    final urls = <Uri>[];
    for (final service in _servicesForChannel(doc)) {
      if (service.fallbackUrl.isEmpty) continue;
      final url = Uri.tryParse(service.fallbackUrl);
      if (url != null) urls.add(url);
    }
    return rankUris(urls, state);
  }

  List<DirectoryService> _servicesForChannel(UpdateDirectory doc) {
    final matched = doc.services
        .where(
            (service) => service.channel.isEmpty || service.channel == channel)
        .toList();
    matched.sort((a, b) {
      final byPriority = a.priority.compareTo(b.priority);
      if (byPriority != 0) return byPriority;
      return a.id.compareTo(b.id);
    });
    return matched;
  }

  Future<_DirectorySourceOutcome> _loadDirectoryFrom(
    Uri url,
    UpdateState state,
  ) async {
    Uint8List bytes;
    try {
      bytes = await fetcher.getBytes(url, timeout: policy.documentTimeout);
    } on FetchException catch (error) {
      return _DirectorySourceRejected(error.message);
    }

    final verified = await openEnvelope(bytes, trustedKeys);
    if (!verified.accepted) {
      return _DirectorySourceRejected(
          'signature check failed (${verified.rejection!.name}: '
          '${verified.detail})');
    }

    final UpdateDirectory doc;
    try {
      doc = parseDirectory(verified.payload!);
    } on RupFormatException catch (error) {
      return _DirectorySourceRejected('malformed directory: ${error.message}');
    }

    if (doc.product != product) {
      return _DirectorySourceRejected(
          'directory is for product "${doc.product}", expected "$product"');
    }

    final sequence = doc.directorySequence.toInt();
    if (!acceptsSequence(sequence, state.lastSeenDirectorySequence)) {
      log?.call('$url directory sequence $sequence < '
          '${state.lastSeenDirectorySequence}, trying the next entry');
      return _DirectorySourceRejected('directory_sequence $sequence is older '
          'than the last accepted ${state.lastSeenDirectorySequence}');
    }

    return _DirectorySourceOk(doc);
  }

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
  Future<Manifest> _loadManifest(VersionNode target, UpdateState state) async {
    final failures = <String>[];

    for (final rawUrl in rankUrlStrings(target.manifest.urls, state)) {
      final url = Uri.tryParse(rawUrl);
      if (url == null) {
        failures.add('$rawUrl: not a valid URL');
        continue;
      }

      Uint8List bytes;
      try {
        bytes = await fetcher.getBytes(url, timeout: policy.documentTimeout);
      } on FetchException catch (error) {
        state.recordSourceFailure(rawUrl);
        failures.add('$rawUrl: ${error.message}');
        continue;
      }

      final expectedSize = target.manifest.size.toInt();
      if (bytes.length != expectedSize) {
        state.recordSourceFailure(rawUrl);
        failures.add('$rawUrl: expected $expectedSize bytes, '
            'got ${bytes.length}');
        continue;
      }
      final digest = sha256OfBytes(bytes);
      if (digest != target.manifest.sha256) {
        state.recordSourceFailure(rawUrl);
        failures.add('$rawUrl: sha256 mismatch');
        continue;
      }

      final Manifest manifest;
      try {
        manifest = parseManifest(bytes);
      } on RupFormatException catch (error) {
        state.recordSourceFailure(rawUrl);
        failures.add('$rawUrl: ${error.message}');
        continue;
      }

      // The hash already proves these bytes are what the index pinned, so a
      // mismatch here means the publisher assembled two documents that
      // disagree. Refusing is the only safe reading: acting on it would install
      // a different version than the chain says.
      if (manifest.product != product) {
        state.recordSourceFailure(rawUrl);
        failures.add('$rawUrl: manifest names product "${manifest.product}"');
        continue;
      }
      final manifestCode = manifest.code.toInt();
      final targetCode = target.code.toInt();
      if (manifestCode != targetCode) {
        state.recordSourceFailure(rawUrl);
        failures.add('$rawUrl: manifest code $manifestCode does not match '
            'index node $targetCode');
        continue;
      }
      if (manifest.version != target.version) {
        state.recordSourceFailure(rawUrl);
        failures.add('$rawUrl: manifest version "${manifest.version}" does not '
            'match index node "${target.version}"');
        continue;
      }

      state.recordSourceSuccess(rawUrl);
      return manifest;
    }

    throw VerificationException(
        'no usable manifest for ${target.version}: ${failures.join('; ')}');
  }

  Future<_FallbackSourceOutcome> _loadFallbackFrom(
      Uri url, UpdateState state) async {
    Uint8List bytes;
    try {
      bytes = await fetcher.getBytes(url, timeout: policy.documentTimeout);
    } on FetchException catch (error) {
      return _FallbackSourceRejected(error.message);
    }

    final verified = await openEnvelope(bytes, trustedKeys);
    if (!verified.accepted) {
      return _FallbackSourceRejected(
          'signature check failed (${verified.rejection!.name}: '
          '${verified.detail})');
    }

    final Fallback doc;
    try {
      doc = parseFallback(verified.payload!);
    } on RupFormatException catch (error) {
      return _FallbackSourceRejected('malformed fallback: ${error.message}');
    }

    if (doc.product != product) {
      return _FallbackSourceRejected(
          'fallback is for product "${doc.product}", expected "$product"');
    }

    final sequence = doc.sequence.toInt();
    if (!acceptsSequence(sequence, state.lastSeenFallbackSequence)) {
      log?.call('$url fallback sequence $sequence < '
          '${state.lastSeenFallbackSequence}, trying the next source');
      return _FallbackSourceRejected('sequence $sequence is older than the '
          'last accepted ${state.lastSeenFallbackSequence}');
    }

    return _FallbackSourceOk(doc);
  }

  FallbackRule? _matchFallbackRule(Fallback doc) {
    for (final rule in doc.rules) {
      final minCode = rule.minCode.toInt();
      final maxCode = rule.maxCode.toInt();
      if (currentCode < minCode || currentCode > maxCode) continue;
      if (!matchesSelectors(selectorsToMap(rule.selectors), clientSelectors)) {
        continue;
      }
      return rule;
    }
    return null;
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

sealed class _FallbackSourceOutcome {
  const _FallbackSourceOutcome();
}

class _FallbackSourceOk extends _FallbackSourceOutcome {
  const _FallbackSourceOk(this.doc);

  final Fallback doc;
}

class _FallbackSourceRejected extends _FallbackSourceOutcome {
  const _FallbackSourceRejected(this.why);

  final String why;
}

class _IndexCandidate {
  const _IndexCandidate({required this.url, required this.preferenceKey});

  final Uri url;
  final String preferenceKey;
}

sealed class _DirectorySourceOutcome {
  const _DirectorySourceOutcome();
}

class _DirectorySourceOk extends _DirectorySourceOutcome {
  const _DirectorySourceOk(this.doc);

  final UpdateDirectory doc;
}

class _DirectorySourceRejected extends _DirectorySourceOutcome {
  const _DirectorySourceRejected(this.why);

  final String why;
}

