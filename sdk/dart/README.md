# rup_client (Dart RUP SDK)

Official Dart client for [RUP](https://github.com/shichao402/relkit) protobuf v2:
signed index → version chain → hash-verified download (optional apply helpers).

**This directory (`relkit/sdk/dart`) is the source of truth.**  
Go peer SDK: `../` (`github.com/shichao402/relkit/sdk`).  
Host wiring: `relkit materialize` projects this component to
`.relkit/components/sdk/dart`; product repos no longer materialize host scripts
(ADR 0019 §1).

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
      url: https://github.com/shichao402/relkit.git
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

## Cross-implementation gate

`test/interop_test.dart` reads `test/fixtures/interop-index.json`, which holds an
envelope sealed by the Go control plane through the same path `publish.Run` uses.
The Dart side verifies the signature, parses the index and runs chain selection on
those exact bytes; `internal/envelope` re-seals on every `go test` run so the
checked-in bytes cannot drift from what relkit actually emits.

Do not edit the fixture by hand. After an intentional wire change:

```
go test ./internal/envelope -run TestSdkInteropFixtureIsNotStale -update-sdk-interop-fixture
```


## What it does

- `Updater` (facade) — spawn `relkit-updater` for check/download/apply
- protocol helpers — envelope, chain, selectors, hash-verified download

In-process `RupUpdater` / `UpdateScheduler` stay in this repo for engine tests
and are not exported or packed into host zips.

The package does not decide UI, prompts, or silent installs.

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

See `example/check_update.dart` for a check-flow sample.
