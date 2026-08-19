# rup_client (Dart RUP SDK)

Official Dart client for [RUP](https://cnb.cool/shichao402/relkit) protobuf v2:
signed index → version chain → hash-verified download (optional apply helpers).

**This directory (`relkit/sdk/dart`) is the source of truth.**  
Go peer SDK: `../` (`cnb.cool/shichao402/relkit/sdk`).  
Agent onboarding: [`AGENT-QUICKSTART.md`](AGENT-QUICKSTART.md) · hub: [`../../docs/agent/README.md`](../../docs/agent/README.md).

Pure Dart (no Flutter SDK dependency).

## Install

From this monorepo path (local):

```yaml
dependencies:
  rup_client:
    path: /path/to/relkit/sdk/dart
```

From Git:

```yaml
dependencies:
  rup_client:
    git:
      url: https://cnb.cool/shichao402/relkit.git
      path: sdk/dart
      ref: main   # or a release tag
```

Internal CI that cannot reach GitHub should **mirror** this tree into the host
repo and document the sync command (do not treat the mirror as upstream).

## Wire format

- Envelope schema `rup.envelope/2`; payload = Index protobuf bytes
- Ed25519 over those payload bytes
- Manifest / artifact integrity via size + sha256
- Selectors are repeated `Selector { key, value }`

Generated types: `lib/src/gen/rup/v2/` (re-exported by `package:rup_client/rup_client.dart`).

## What it does

- `RupUpdater.check()` — verify index, select next version, resolve artifact
- `RupUpdater.download()` — Range multi-connection when possible, resume, retries; `DownloadProgress`
- `UpdateScheduler` — start + periodic throttled checks
- `UpdateRuntimeConfig` — host-injected scheduler config (`forceOnStart`, etc.)
- `src/apply/` — optional install helpers: `wholeRoot` (directory swap) and
  `versionedDir` (`versions/<id>/` + atomic `active.json`). Defaults:
  Windows `versionedDir`, macOS `wholeRoot`. Not protocol; see SPEC appendix B.

The package does not decide UI, prompts, or silent installs. It does not read
configuration files: load JSON in the host and pass [UpdateRuntimeConfig].

### Background checks

```dart
const runtime = UpdateRuntimeConfig(
  checkOnStart: true,
  forceOnStart: true, // cold start bypasses throttle; periodic ticks do not
);

UpdateScheduler(
  runtime: runtime,
  check: updater.check,
  onResult: (result) { /* show UI on UpdateAvailable */ },
).start();
```

Example host JSON (`assets/config/rup_update.json`):

```json
{
  "checkOnStart": true,
  "forceOnStart": true,
  "afterSuccessHours": 24,
  "afterFailureHours": 1
}
```

```dart
final runtime = UpdateRuntimeConfig.fromJson(jsonDecode(await rootBundle.loadString(...)));
```

Manual checks from settings should call `updater.check(force: true)` directly.

## Checking for an update

```dart
final updater = RupUpdater(
  product: 'demo',
  channel: 'stable',
  currentCode: 21,
  indexUrls: [
    Uri.parse('https://releases.example.com/index/demo/stable.pb'),
  ],
  trustedKeys: TrustedKeys.fromBase64({
    'release-2026': 'oKbk9wV4...',
  }),
  clientSelectors: const {'os': 'windows', 'arch': 'x64'},
  stateStore: FileUpdateStateStore(
    directory: Directory(supportDir),
    product: 'demo',
    channel: 'stable',
  ),
);

switch (await updater.check()) {
  case UpToDate():
    break;
  case UpdateAvailable(:final target):
    print('next: ${target.version}');
  case CheckFailed(:final reason):
    print(reason);
  case CheckThrottled():
    break;
}
```

See `example/check_update.dart` and `AGENT-QUICKSTART.md` for full host wiring.
