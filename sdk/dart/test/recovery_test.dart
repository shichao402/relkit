import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

void main() {
  test('CheckFailed can carry compile-time recovery', () {
    const help = RecoveryHelp(
      message: 'install manually',
      links: [RecoveryLink(label: 'GitHub', url: 'https://github.com/example/app/releases')],
    );
    const failed = CheckFailed('offline', <String>['entry: fail'], recovery: help);
    expect(failed.recovery?.message, 'install manually');
  });
}
