/// Host-injected runtime configuration for background update scheduling.
///
/// The SDK does not read configuration files. The host loads JSON (or any other
/// source), constructs [UpdateRuntimeConfig], and passes it to [UpdateScheduler].
library;

import 'state.dart';

/// Scheduler and throttling knobs for a running client.
///
/// [forceOnStart] controls whether the first tick after [UpdateScheduler.start]
/// calls `check(force: true)`, bypassing [UpdatePolicy.shouldCheck]. Periodic
/// ticks always use `force: false`.
class UpdateRuntimeConfig {
  const UpdateRuntimeConfig({
    this.checkOnStart = true,
    this.forceOnStart = true,
    this.policy = const UpdatePolicy(),
  });

  /// Whether [UpdateScheduler.start] runs an immediate tick.
  final bool checkOnStart;

  /// When true, the startup tick ignores throttling (`check(force: true)`).
  final bool forceOnStart;

  /// Throttle intervals and download behaviour for [RupUpdater].
  final UpdatePolicy policy;

  /// Parses host JSON. Unknown keys are ignored.
  ///
  /// Supported keys:
  /// - `checkOnStart` (bool, default true)
  /// - `forceOnStart` (bool, default true)
  /// - `afterSuccessHours` (num, default 24)
  /// - `afterFailureHours` (num, default 1)
  factory UpdateRuntimeConfig.fromJson(Map<String, dynamic> json) {
    Duration hours(num? value, Duration fallback) {
      if (value == null) return fallback;
      return Duration(hours: value.toInt());
    }

    return UpdateRuntimeConfig(
      checkOnStart: json['checkOnStart'] as bool? ?? true,
      forceOnStart: json['forceOnStart'] as bool? ?? true,
      policy: UpdatePolicy(
        afterSuccess: hours(
          json['afterSuccessHours'] as num?,
          const Duration(hours: 24),
        ),
        afterFailure: hours(
          json['afterFailureHours'] as num?,
          const Duration(hours: 1),
        ),
      ),
    );
  }
}
