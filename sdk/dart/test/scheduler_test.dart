/// Tests for [UpdateScheduler]: start tick, throttle reschedule, stop.
library;

import 'dart:async';

import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

void main() {
  test('runs an immediate check on start and reschedules after success',
      () async {
    final delays = <Duration>[];
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
      scheduleTimer: (delay, callback) {
        delays.add(delay);
        // Do not fire: we only assert the planned delay.
        return Timer(const Duration(days: 365), callback);
      },
    );

    scheduler.start();
    await Future<void>.delayed(Duration.zero);
    await Future<void>.delayed(const Duration(milliseconds: 10));

    expect(checks, 1);
    expect(results, hasLength(1));
    expect(results.single, isA<UpToDate>());
    expect(delays, [const Duration(hours: 24)]);
    scheduler.stop();
  });

  test('CheckThrottled arms until nextAllowedAt without calling onResult',
      () async {
    final delays = <Duration>[];
    final results = <UpdateCheckResult>[];
    final fixedNow = DateTime.utc(2026, 1, 1, 12);
    final next = fixedNow.add(const Duration(hours: 3));

    final scheduler = UpdateScheduler(
      now: () => fixedNow,
      check: ({bool force = false}) async => CheckThrottled(next),
      onResult: results.add,
      scheduleTimer: (delay, callback) {
        delays.add(delay);
        return Timer(const Duration(days: 365), callback);
      },
    );

    scheduler.start();
    await Future<void>.delayed(const Duration(milliseconds: 10));

    expect(results, isEmpty);
    expect(delays, [const Duration(hours: 3)]);
    scheduler.stop();
  });

  test('stop prevents reschedule after an in-flight check', () async {
    final delays = <Duration>[];
    final gate = Completer<void>();

    final scheduler = UpdateScheduler(
      check: ({bool force = false}) async {
        await gate.future;
        return const UpToDate(sequence: 1);
      },
      onResult: (_) {},
      scheduleTimer: (delay, callback) {
        delays.add(delay);
        return Timer(const Duration(days: 365), callback);
      },
    );

    scheduler.start();
    scheduler.stop();
    gate.complete();
    await Future<void>.delayed(const Duration(milliseconds: 10));

    expect(delays, isEmpty);
  });

  test('CheckFailed reschedules with afterFailure', () async {
    final delays = <Duration>[];

    final scheduler = UpdateScheduler(
      policy: const UpdatePolicy(afterFailure: Duration(minutes: 30)),
      check: ({bool force = false}) async =>
          const CheckFailed('boom', <String>[]),
      onResult: (_) {},
      scheduleTimer: (delay, callback) {
        delays.add(delay);
        return Timer(const Duration(days: 365), callback);
      },
    );

    scheduler.start();
    await Future<void>.delayed(const Duration(milliseconds: 10));

    expect(delays, [const Duration(minutes: 30)]);
    scheduler.stop();
  });

  test('forceOnStart runs check(force: true) even when throttled', () async {
    var checks = 0;
    bool? firstForce;

    final scheduler = UpdateScheduler(
      runtime: const UpdateRuntimeConfig(forceOnStart: true),
      check: ({bool force = false}) async {
        checks++;
        firstForce ??= force;
        return const UpToDate(sequence: 1);
      },
      onResult: (_) {},
      scheduleTimer: (delay, callback) =>
          Timer(const Duration(days: 365), callback),
    );

    scheduler.start();
    await Future<void>.delayed(const Duration(milliseconds: 10));

    expect(checks, 1);
    expect(firstForce, isTrue);
    scheduler.stop();
  });

  test('forceOnStart false allows CheckThrottled on startup', () async {
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
      scheduleTimer: (delay, callback) =>
          Timer(const Duration(days: 365), callback),
    );

    scheduler.start();
    await Future<void>.delayed(const Duration(milliseconds: 10));

    expect(checks, 1);
    scheduler.stop();
  });
}
