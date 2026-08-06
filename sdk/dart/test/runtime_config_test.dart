/// Tests for [UpdateRuntimeConfig] JSON parsing.
library;

import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

void main() {
  test('defaults when JSON is empty', () {
    const config = UpdateRuntimeConfig();
    expect(config.checkOnStart, isTrue);
    expect(config.forceOnStart, isTrue);
    expect(config.policy.afterSuccess, const Duration(hours: 24));
    expect(config.policy.afterFailure, const Duration(hours: 1));
  });

  test('fromJson parses scheduler flags and policy hours', () {
    final config = UpdateRuntimeConfig.fromJson({
      'checkOnStart': false,
      'forceOnStart': false,
      'afterSuccessHours': 6,
      'afterFailureHours': 2,
    });

    expect(config.checkOnStart, isFalse);
    expect(config.forceOnStart, isFalse);
    expect(config.policy.afterSuccess, const Duration(hours: 6));
    expect(config.policy.afterFailure, const Duration(hours: 2));
  });
}
