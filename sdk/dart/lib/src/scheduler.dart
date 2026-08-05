/// Background check scheduling (SPEC.md section 12.2 companion).
///
/// Throttling already lives in [UpdatePolicy.shouldCheck] / [RupUpdater.check].
/// This type only wakes on a timer, calls `check(force: false)`, and reschedules
/// from the result — so hosts do not re-implement "wait 24h / 1h" themselves.
library;

import 'dart:async';

import 'state.dart';
import 'updater.dart';

/// Runs throttled update checks on start and on a wake timer.
///
/// The scheduler never bypasses throttling. User-initiated checks stay on
/// [RupUpdater.check] with `force: true` outside this type.
class UpdateScheduler {
  UpdateScheduler({
    required this.check,
    required this.onResult,
    this.policy = const UpdatePolicy(),
    this.log,
    this.now,
    this.scheduleTimer,
  });

  /// Usually `updater.check` or a host wrapper that adds logging / support gates.
  final Future<UpdateCheckResult> Function({bool force}) check;

  /// Called for every completed check except [CheckThrottled] (those only
  /// reschedule). Hosts typically open UI on [UpdateAvailable].
  final void Function(UpdateCheckResult result) onResult;

  final UpdatePolicy policy;
  final void Function(String message)? log;

  /// Injectable clock for tests.
  final DateTime Function()? now;

  /// Injectable timer factory for tests. Defaults to [Timer].
  final Timer Function(Duration delay, void Function() callback)? scheduleTimer;

  bool _running = false;
  bool _inflight = false;
  Timer? _timer;
  int _generation = 0;

  bool get isRunning => _running;

  /// Starts the loop. When [checkOnStart] is true (default), a tick runs
  /// immediately; otherwise the first wake is [UpdatePolicy.afterFailure]
  /// (short) so a newly started host still converges quickly after failures.
  void start({bool checkOnStart = true}) {
    if (_running) return;
    _running = true;
    _generation++;
    log?.call('update scheduler started (checkOnStart=$checkOnStart)');
    if (checkOnStart) {
      unawaited(_tick());
    } else {
      _arm(policy.afterFailure);
    }
  }

  /// Cancels the pending wake. An in-flight check may still finish; its result
  /// is ignored for rescheduling once stopped.
  void stop() {
    if (!_running) return;
    _running = false;
    _generation++;
    _timer?.cancel();
    _timer = null;
    log?.call('update scheduler stopped');
  }

  DateTime _now() => now?.call() ?? DateTime.now();

  void _arm(Duration delay) {
    _timer?.cancel();
    if (!_running) return;
    final safe = delay.isNegative ? Duration.zero : delay;
    final gen = _generation;
    final create = scheduleTimer ?? Timer.new;
    _timer = create(safe, () {
      if (!_running || gen != _generation) return;
      unawaited(_tick());
    });
  }

  void _armUntil(DateTime when) {
    final wait = when.difference(_now());
    _arm(wait < Duration.zero ? Duration.zero : wait);
  }

  Future<void> _tick() async {
    if (!_running || _inflight) return;
    _inflight = true;
    final gen = _generation;
    try {
      final result = await check(force: false);
      if (!_running || gen != _generation) return;

      switch (result) {
        case CheckThrottled(:final nextAllowedAt):
          log?.call('scheduler throttled until $nextAllowedAt');
          _armUntil(nextAllowedAt);
        case UpToDate():
          onResult(result);
          _arm(policy.afterSuccess);
        case UpdateAvailable():
          onResult(result);
          // Surface to the user, but do not poll the network again until the
          // success interval elapses (install / skip happens on the host).
          _arm(policy.afterSuccess);
        case FallbackRequired():
          onResult(result);
          _arm(policy.afterSuccess);
        case CheckFailed():
          onResult(result);
          _arm(policy.afterFailure);
      }
    } catch (error) {
      log?.call('scheduler tick failed: $error');
      if (_running && gen == _generation) {
        _arm(policy.afterFailure);
      }
    } finally {
      _inflight = false;
    }
  }
}
