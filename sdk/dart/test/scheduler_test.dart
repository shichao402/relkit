/// Tests for [UpdateScheduler]: the start tick, the reschedule after each
/// outcome, stop, and the re-arm loop that makes "periodic" periodic.
///
/// The timer is injected as a [_ManualTimer] the test fires by hand. Avoiding a
/// real delay is the lesser reason. The earlier version of this suite returned
/// `Timer(const Duration(days: 365), callback)` with a comment saying it must not
/// fire, so every test asserted only the *planned* delay and none asserted that
/// the wake runs another check. Replacing the armed callback with a no-op — which
/// disables background checking completely — left all six tests green. Firing the
/// timer here is what closes that hole: the scheduler's entire job is that the
/// chain keeps going, and a chain is only observable by walking it.
library;

import 'dart:async';

import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

/// A [Timer] the test decides when to fire.
///
/// Unlike a far-future real timer, "does not fire on its own" is a property of
/// the type here rather than a comment, and the time axis can still be advanced.
class _ManualTimer implements Timer {
  _ManualTimer(this.delay, this._callback);

  final Duration delay;
  final void Function() _callback;

  bool _cancelled = false;
  int _tick = 0;

  @override
  bool get isActive => !_cancelled;

  @override
  int get tick => _tick;

  @override
  void cancel() => _cancelled = true;

  /// Advances to this wake. A cancelled timer does nothing, as a real one would.
  void fire() {
    if (_cancelled) return;
    _tick++;
    _callback();
  }
}

/// Records every timer the scheduler arms, so tests can fire them in order.
class _TimerLog {
  final List<_ManualTimer> armed = <_ManualTimer>[];

  List<Duration> get delays => armed.map((timer) => timer.delay).toList();

  _ManualTimer get last => armed.last;

  Timer schedule(Duration delay, void Function() callback) {
    final timer = _ManualTimer(delay, callback);
    armed.add(timer);
    return timer;
  }
}

/// Lets the `await check(...)` inside a tick complete before asserting.
Future<void> settle() => Future<void>.delayed(Duration.zero);

UpdateAvailable _updateAvailable({int code = 110}) => UpdateAvailable(
      target: VersionNode(version: '1.1.0', code: Int64(code)),
      artifact: Artifact(id: 'app', filename: 'app.zip', kind: 'archive'),
      manifest: Manifest(
        schema: manifestSchemaId,
        product: 'demo',
        version: '1.1.0',
        code: Int64(code),
      ),
      mandatory: false,
      remainingHops: 0,
      sequence: 1,
    );

const _fallback = FallbackRequired(
  manualUrl: 'https://example.invalid/download',
  message: 'grab it by hand',
  mandatory: false,
  sequence: 1,
  minCode: 0,
  maxCode: 200,
);

void main() {
  test('runs an immediate check on start and reschedules after success',
      () async {
    final timers = _TimerLog();
    final results = <UpdateCheckResult>[];
    var checks = 0;

    final scheduler = UpdateScheduler(
      runtime: const UpdateRuntimeConfig(
        forceOnStart: false,
        policy: UpdatePolicy(afterSuccess: Duration(hours: 24)),
      ),
      check: ({bool force = false}) async {
        checks++;
        expect(force, isFalse);
        return const UpToDate(sequence: 1);
      },
      onResult: results.add,
      scheduleTimer: timers.schedule,
    );

    scheduler.start();
    await settle();

    expect(checks, 1);
    expect(results, hasLength(1));
    expect(results.single, isA<UpToDate>());
    expect(timers.delays, [const Duration(hours: 24)]);
    scheduler.stop();
  });

  test('the armed wake runs another check and re-arms', () async {
    final timers = _TimerLog();
    final forces = <bool>[];

    final scheduler = UpdateScheduler(
      runtime: const UpdateRuntimeConfig(
        forceOnStart: true,
        policy: UpdatePolicy(afterSuccess: Duration(hours: 6)),
      ),
      check: ({bool force = false}) async {
        forces.add(force);
        return const UpToDate(sequence: 1);
      },
      onResult: (_) {},
      scheduleTimer: timers.schedule,
    );

    scheduler.start();
    await settle();
    expect(forces, [true]);

    timers.last.fire();
    await settle();

    // The wake must check again, and must not inherit forceOnStart: periodic
    // checks stay subject to throttling.
    expect(forces, [true, false]);
    expect(timers.delays, [
      const Duration(hours: 6),
      const Duration(hours: 6),
    ]);

    // Twice, because a loop that re-arms exactly once looks identical to a
    // working one after a single wake.
    timers.last.fire();
    await settle();
    expect(forces, [true, false, false]);
    expect(timers.delays, hasLength(3));

    scheduler.stop();
  });

  test('CheckThrottled arms until nextAllowedAt without calling onResult',
      () async {
    final timers = _TimerLog();
    final results = <UpdateCheckResult>[];
    final fixedNow = DateTime.utc(2026, 1, 1, 12);
    final next = fixedNow.add(const Duration(hours: 3));
    var checks = 0;

    final scheduler = UpdateScheduler(
      now: () => fixedNow,
      check: ({bool force = false}) async {
        checks++;
        return CheckThrottled(next);
      },
      onResult: results.add,
      scheduleTimer: timers.schedule,
    );

    scheduler.start();
    await settle();

    expect(results, isEmpty);
    expect(timers.delays, [const Duration(hours: 3)]);

    timers.last.fire();
    await settle();

    // Throttling means "not now", not "not ever": the loop has to survive it,
    // or one throttled check ends background checking for the whole run.
    expect(checks, 2);
    expect(results, isEmpty);
    scheduler.stop();
  });

  test('stop prevents reschedule after an in-flight check', () async {
    final timers = _TimerLog();
    final gate = Completer<void>();

    final scheduler = UpdateScheduler(
      check: ({bool force = false}) async {
        await gate.future;
        return const UpToDate(sequence: 1);
      },
      onResult: (_) {},
      scheduleTimer: timers.schedule,
    );

    scheduler.start();
    scheduler.stop();
    gate.complete();
    await settle();

    expect(timers.delays, isEmpty);
  });

  test('stop cancels the armed wake', () async {
    final timers = _TimerLog();
    var checks = 0;

    final scheduler = UpdateScheduler(
      check: ({bool force = false}) async {
        checks++;
        return const UpToDate(sequence: 1);
      },
      onResult: (_) {},
      scheduleTimer: timers.schedule,
    );

    scheduler.start();
    await settle();
    expect(checks, 1);

    final armed = timers.last;
    scheduler.stop();

    // Reaching the network after the host tore the scheduler down is a real bug,
    // so stop has to hold that wake down even if the timer still fires.
    expect(armed.isActive, isFalse);
    armed.fire();
    await settle();
    expect(checks, 1);
  });

  test('CheckFailed reschedules with afterFailure and keeps going', () async {
    final timers = _TimerLog();
    var checks = 0;

    final scheduler = UpdateScheduler(
      runtime: const UpdateRuntimeConfig(
        policy: UpdatePolicy(
          afterSuccess: Duration(hours: 24),
          afterFailure: Duration(minutes: 30),
        ),
      ),
      check: ({bool force = false}) async {
        checks++;
        return const CheckFailed('boom', <String>[]);
      },
      onResult: (_) {},
      scheduleTimer: timers.schedule,
    );

    scheduler.start();
    await settle();
    expect(timers.delays, [const Duration(minutes: 30)]);

    timers.last.fire();
    await settle();

    // An unreachable server must not stop the client from ever asking again.
    expect(checks, 2);
    expect(timers.delays, [
      const Duration(minutes: 30),
      const Duration(minutes: 30),
    ]);
    scheduler.stop();
  });

  test('a throwing check reschedules with afterFailure', () async {
    final timers = _TimerLog();
    final logs = <String>[];
    var checks = 0;

    final scheduler = UpdateScheduler(
      runtime: const UpdateRuntimeConfig(
        policy: UpdatePolicy(
          afterSuccess: Duration(hours: 24),
          afterFailure: Duration(minutes: 30),
        ),
      ),
      check: ({bool force = false}) async {
        checks++;
        throw StateError('exploded');
      },
      onResult: (_) => fail('a thrown check has no result to surface'),
      log: logs.add,
      scheduleTimer: timers.schedule,
    );

    scheduler.start();
    await settle();

    expect(timers.delays, [const Duration(minutes: 30)]);
    expect(logs, contains(contains('exploded')));

    timers.last.fire();
    await settle();
    expect(checks, 2);
    scheduler.stop();
  });

  test('UpdateAvailable surfaces the update and re-arms', () async {
    final timers = _TimerLog();
    final results = <UpdateCheckResult>[];
    var checks = 0;

    final scheduler = UpdateScheduler(
      runtime: const UpdateRuntimeConfig(
        policy: UpdatePolicy(afterSuccess: Duration(hours: 6)),
      ),
      check: ({bool force = false}) async {
        checks++;
        return _updateAvailable();
      },
      onResult: results.add,
      scheduleTimer: timers.schedule,
    );

    scheduler.start();
    await settle();

    expect(results.single, isA<UpdateAvailable>());
    // Install and skip both happen on the host, so the scheduler waits out the
    // success interval rather than re-polling; it must still be waiting for
    // something, otherwise the next release after this one is never seen.
    expect(timers.delays, [const Duration(hours: 6)]);

    timers.last.fire();
    await settle();
    expect(checks, 2);
    scheduler.stop();
  });

  test('FallbackRequired surfaces the notice and re-arms', () async {
    final timers = _TimerLog();
    final results = <UpdateCheckResult>[];
    var checks = 0;

    final scheduler = UpdateScheduler(
      runtime: const UpdateRuntimeConfig(
        policy: UpdatePolicy(afterSuccess: Duration(hours: 6)),
      ),
      check: ({bool force = false}) async {
        checks++;
        return _fallback;
      },
      onResult: results.add,
      scheduleTimer: timers.schedule,
    );

    scheduler.start();
    await settle();

    expect(results.single, isA<FallbackRequired>());
    expect(timers.delays, [const Duration(hours: 6)]);

    timers.last.fire();
    await settle();
    expect(checks, 2);
    scheduler.stop();
  });

  test('checkOnStart false arms the first wake without checking', () async {
    final timers = _TimerLog();
    var checks = 0;

    final scheduler = UpdateScheduler(
      runtime: const UpdateRuntimeConfig(
        checkOnStart: false,
        policy: UpdatePolicy(
          afterSuccess: Duration(hours: 24),
          afterFailure: Duration(minutes: 30),
        ),
      ),
      check: ({bool force = false}) async {
        checks++;
        return const UpToDate(sequence: 1);
      },
      onResult: (_) {},
      scheduleTimer: timers.schedule,
    );

    scheduler.start();
    await settle();

    // Opting out of the start check must not opt out of the loop.
    expect(checks, 0);
    expect(timers.delays, [const Duration(minutes: 30)]);

    timers.last.fire();
    await settle();
    expect(checks, 1);
    scheduler.stop();
  });

  test('forceOnStart runs check(force: true) even when throttled', () async {
    final timers = _TimerLog();
    final forces = <bool>[];

    final scheduler = UpdateScheduler(
      runtime: const UpdateRuntimeConfig(forceOnStart: true),
      check: ({bool force = false}) async {
        forces.add(force);
        return const UpToDate(sequence: 1);
      },
      onResult: (_) {},
      scheduleTimer: timers.schedule,
    );

    scheduler.start();
    await settle();

    expect(forces, [true]);

    timers.last.fire();
    await settle();

    // forceOnStart is about the cold start only.
    expect(forces, [true, false]);
    scheduler.stop();
  });

  test('forceOnStart false allows CheckThrottled on startup', () async {
    final timers = _TimerLog();
    final fixedNow = DateTime.utc(2026, 1, 1, 12);
    final next = fixedNow.add(const Duration(hours: 3));
    var checks = 0;

    final scheduler = UpdateScheduler(
      runtime: const UpdateRuntimeConfig(forceOnStart: false),
      now: () => fixedNow,
      check: ({bool force = false}) async {
        checks++;
        expect(force, isFalse);
        return CheckThrottled(next);
      },
      onResult: (_) {},
      scheduleTimer: timers.schedule,
    );

    scheduler.start();
    await settle();

    expect(checks, 1);
    scheduler.stop();
  });
}
