# Changelog

All notable changes to relkit are recorded here. The version in `VERSION.json`
is the single source of truth; this file explains what changed and, above all,
what will break.

## Unreleased

### Added

- `relkit-updater proto` prints the embedded `relkit.updater.v2` contract, so a
  host generates its types from the very binary it runs (ADR 0026).
- On Windows the apply worker shows a progress window while the host is gone:
  waiting for exit, downloading, installing, relaunching, and failures with
  the host's recovery help. Waiting and downloading can be cancelled; the old
  version is relaunched when the install was left untouched. Hosts that draw
  their own handoff UI set `ApplyOp.hideProgress`; `InstallSpec.displayName`
  names the product (ADR 0026 §6).
- On macOS the apply worker shows the same progress window, drawn with
  AppKit. The darwin `relkit-updater` is now a cgo build from the
  `darwin-updater` GitHub Actions workflow and needs macOS 13 or later; the
  deploy CLI downloads that build for the commit being released
  (`relkit.py build --darwin-cgo` builds it locally on a Mac instead).
- `apply` accepts a plan that was not downloaded yet; the worker downloads it
  after the host exits (new `SESSION_PHASE_DOWNLOADING`). Such a session always
  reports `requiresHostExit: true`.
- `StatusSnapshot.hostMustExit` is the single startup handoff rule: a host that
  sees it true exits at once. The engine computes it (live worker, install root
  still needed), so a relaunched host is never told to quit and a crashed
  worker never locks the host out (ADR 0026 §6).
- Release codes are allocated by the server (ADR 0025). `POST /v1/release-code`
  hands out the next code for a product, above both its counter and every code
  already published on any channel, so products that published before need no
  migration. `relkit release-code claim` prints the number for CI;
  `relkit release-code show` reads the state. Hosts stop deriving codes from git
  history, so the same commit can be released again.
- `relkit release --execute` checks the staged code against the server before
  uploading anything. A stale code now fails in seconds instead of after the
  whole CAS upload.

### Changed

- Registering a product now signs its first Runtime Profile, and the agent signs
  any missing one on start, so `relkit bootstrap build` works right after
  registration. Channels are a server fact (`dev`, `stable`) and publish config
  derives from a machine-level `publish` block in `relkit-agent.json`. See ADR 0024.
- `upgrade` builds that `publish` block from the box's existing
  `products/*.json` and asks the operator for nothing; profiles that disagree
  stop the upgrade. The short-lived `--publish-read-url` flag is removed.

### Breaking

- `relkit-updater` speaks updater protocol 5 (ADR 0026). The request is one
  ProtoJSON `relkit.updater.v2.UpdaterRequest` on stdin; events are ProtoJSON
  `UpdaterEvent` lines on stdout, the first always `Capabilities`. Unknown
  fields are rejected, `ClientProfile.bootstrap` and `expectedProduct` are
  required, and a host on any other protocol gets `UPDATER_TOO_OLD` /
  `UPDATER_TOO_NEW`. The length-prefixed binary frames, `ClientHello`, `CancelOp`
  and the profile fields duplicated by the bootstrap are gone. Hosts on the
  protocol 4 facades must migrate together with the engine; it is not published
  until they do.
- relkit no longer ships `sdk/dart`, `sdk/rust` or `bindings/ts`, and the
  `sdk-dart` / `sdk-rust` / `bindings-ts` components, the facade generator and
  product-tree zip packing are removed. Hosts generate their own types from
  `relkit-updater proto`. `relkit build` loses `--dart-sdk`, `--rust-sdk` and
  `--bindings-ts`.
- The release manifest's `minUpdaterIpc` / `maxUpdaterIpc` become one
  `updaterProtocol`.
- `relkit.project.json` may omit `channels`; any value other than
  `["dev","stable"]` is now rejected, and so is a staged document on another channel.

### Fixed

- After an apply the engine refreshed its sidecar at
  `<installRoot>/relkit-updater[.exe]` regardless of `sidecarRelpath`. On macOS
  that left a stray binary at the `.app` root, outside `Contents`, which breaks
  the bundle's code signature, while the real sidecar under
  `Contents/Resources/runtime/` was never refreshed by this step. The refresh
  now writes `<installRoot>/<sidecarRelpath>`, skips the copy when the payload
  already put identical bytes there, and ignores relpaths that leave the
  install root.

- The publish-time "code is not greater" error pointed at `CLI.md 4.2`, which
  is now the `doctor` section. It now carries the stable code
  `release.code_not_monotonic` and points at `relkit release-code claim`.

- The console's locked screen blamed a missing agent address in the console
  config. Locked actually means the agent is reachable but has no operator and
  no live bootstrap; the screen now says so and points at
  `relkit-agent init -bootstrap`. Found on publish.firoyang.com after its
  redeploy, where the config was correct and the agent had no admin state.

- Clients installed before the 2026-10-24 epoch kept anti-rollback
  watermarks from the old servers (directory 29, index 94 seen in the field),
  while the epoch restarted counting near 1. Even after a manual reinstall they
  rejected every directory and index as "older than last seen". The next
  directory or index signed over an existing one now jumps to
  `model.SequenceFloor` (1000) and counts on from there; a product's first
  document still starts at 1.

### Removed

- `DecAssets/` and the `assets-version.yml` gate. The `relkit-ops` and
  `relkit-deploy` skills now live only in the Dec cloud `relkit` bundle; edit
  them there. See ADR 0023.

## 0.7.2+4 — console seams, deployment root, cloud skill source

A maintenance line inside the epoch. Nothing changes the wire format, and no
client needs to move; what changes is who is allowed to configure a server and
where the operator's own skills come from.

### Breaking

- `relkit-console` reads the release tree through one config entry,
  `tree: {type, ...that type's own parameters}`, picked by `newAdapter` the same
  way `internal/backends` does. The top-level `store` and `dir` keys and the
  `-store` flag are gone: they put one implementation and its address into the
  config, the command line and the deploy script. `init` now takes `-tree`.
  Configs still carrying the old keys refuse to start and point at `upgrade`,
  whose one-time migration is the only code that still knows them.
- The agent signs tool download URLs with its own publicURL. Callers no longer
  pass a base: `--public-base-url` is deleted, and publishing without a public
  publicURL now fails with 409 instead of handing out unreachable URLs.
  `upgrade` never rewrites a product's baseUrl. `update-internal` serves the
  protocol tree only; the example that forwarded `/v1/tools/` and `/v1/catalog`
  there is removed, and `verify-published-tools` targets publish.

### Added

- `relkit_query.py`, a read-only agent query script in the operator skill, so a
  token never has to pass through the agent's context to answer a question.

### Changed

- `install agent` takes `--product-root` as a required flag and records it as
  `productRootParent`, which is what decides where a blank-root registration
  lands. The flag and the agent no longer disagree. `upgrade` backfills the same
  key from the registered products, falling back to the unit's
  WorkingDirectory when no product votes.
- `upgrade` reads `admin.json` before backfilling `productRootParent`, so an
  operator-set value is not overwritten by an inferred one.
- The human-facing page has one shape on both hosts.
- Retired fields are out of both agent config examples.

### Fixed

- After an apply the engine refreshed its sidecar at
  `<installRoot>/relkit-updater[.exe]` regardless of `sidecarRelpath`. On macOS
  that left a stray binary at the `.app` root, outside `Contents`, which breaks
  the bundle's code signature, while the real sidecar under
  `Contents/Resources/runtime/` was never refreshed by this step. The refresh
  now writes `<installRoot>/<sidecarRelpath>`, skips the copy when the payload
  already put identical bytes there, and ignores relpaths that leave the
  install root.

- The publish-time "code is not greater" error pointed at `CLI.md 4.2`, which
  is now the `doctor` section. It now carries the stable code
  `release.code_not_monotonic` and points at `relkit release-code claim`.

- `install agent` can choose the server root. It never wrote `rootKeyId` or
  `custodyManifest`, so every freshly installed box came up rootless: `publish`
  failed with `server root is required`, and `/v1/catalog` and
  `/v1/tools/root.pub` answered 404, which reads like an empty server rather
  than an unconfigured one. The pair could only be set by a later `upgrade`,
  which is why a rebuilt host needed `upgrade` to finish what `install` should
  have done. Both flags now exist on `install agent` and are validated as one
  setting against the manifest, so a typo dies before anything is signed.
- `upgrade` writes the root pair or neither. A box with no root keeps both keys
  absent instead of getting a manifest path with no keyId, which previously
  produced a config the agent refused to start on.
- Both agent config examples leave the root pair empty rather than
  half-filled, so the shape of a complete setting is visible in the file.

### Removed

- The old Dec registry publishing path and the legacy Dec files that went with
  it. The operator skills are published from the cloud source instead: `dec`
  writes straight to the cloud SSOT, and the `DecAssets/` copy in this repo is
  no longer the publishing route. See ADR 0023.

## 0.7.1 — console rebuild, deployment root

### Breaking

- The console's server-rendered pages are replaced by a React SPA over a JSON
  API. `cmd/relkit-console` serves `/-/admin/api/*` (session, products, files,
  users, roles, tokens) plus the SPA shell; the Go templates are gone. Writes
  still go through `relkit-agent` with the operator's own Bearer, so the console
  keeps owning no state.
- The old `/-/admin/panel` and `/-/admin/panel/*` pages no longer exist; those
  URLs and `/-/p/<id>` now 302 to the new routes. `/-/latest/*` and `/-/health`
  are unchanged.
- `mergeIdentity` is gone from artifact sets. It was the host-side proof that it
  held a resolution; the epoch moved that judgement to the server, and keeping
  the engine check made every multi-platform merge impossible. Identity
  agreement between documents is still enforced through
  product/version/releaseCode/channel.

### Added

- The built front end is committed under `internal/webui/dist` so a Go-only
  checkout still builds. CI rebuilds it and fails when the committed bytes are
  stale.

### Fixed

- After an apply the engine refreshed its sidecar at
  `<installRoot>/relkit-updater[.exe]` regardless of `sidecarRelpath`. On macOS
  that left a stray binary at the `.app` root, outside `Contents`, which breaks
  the bundle's code signature, while the real sidecar under
  `Contents/Resources/runtime/` was never refreshed by this step. The refresh
  now writes `<installRoot>/<sidecarRelpath>`, skips the copy when the payload
  already put identical bytes there, and ignores relpaths that leave the
  install root.

- The publish-time "code is not greater" error pointed at `CLI.md 4.2`, which
  is now the `doctor` section. It now carries the stable code
  `release.code_not_monotonic` and points at `relkit release-code claim`.

- The admin panel is reachable. Navigation to it used to exist only inside the
  panel itself, so nothing linked to it from anywhere else. The SPA has a
  persistent sidebar (and a select on narrow screens) reaching products, users,
  roles and tokens from every page, with a skip link and a visible focus ring
  for keyboard users, and a regression test walks the navigation.
- The agent no longer invents a product root. An empty root resolves from
  `productRootParent` in the agent config, else from the common parent of the
  registered products, and is rejected with an actionable message when neither
  exists. The old `/srv/relkit/<id>` default belongs to root on the internal
  machines and deferred the failure to the first publish.
- Writes are JSON mutations that return data, not pages, and the UI invalidates
  queries afterwards. A refresh or a back-navigation can no longer replay a
  rotate-token, which would invalidate the plaintext just shown, or mint an
  extra token.
- Dialogs restore focus correctly. They are controlled by an `open` prop and
  have no trigger, so Radix had no element to hand focus back to on close and
  the next Tab restarted at the top of the page. A document-level focusin
  listener remembers the opener.
- Registering a retirement names the product tree it leaves behind, and the
  panel previews a retirement before committing to it.
- Agent refusals are localised, and golden fixtures stay byte-stable across
  line endings.

### Removed

- `relkit-cos-cert-renew` and its references in the docs.

## 0.7.0+1 — desktop updater epoch

This release is a clean break, not an upgrade path. Hosts and servers on the
other side of the boundary are expected to fail loudly and be replaced, and no
compatibility shim exists in either direction.

### Breaking

- `relkit-updater` is the only update engine. The Dart, Node and Rust libraries
  no longer contain any update logic: no signature verification, no index or
  manifest parsing, no selector matching, no version comparison, no download or
  file-replacement code. A host that called those APIs now talks to the engine
  over IPC instead.
- IPC baseline raised from 3 to 4. An engine and a host from different sides of
  the boundary refuse each other with `UPDATER_TOO_OLD` / `UPDATER_TOO_NEW`
  instead of half-working. There is no negotiation window.
- The runtime bootstrap is parsed by the engine only. A host hands over the
  bootstrap document verbatim plus the product it expects, and the engine
  reports back the allowed channels, the default channel, and the root key it
  trusts. `platform_selectors` is gone from every client.
- One root key per server. Product signing keys, the Distribution Root, the
  Candidate Root, the Runtime Profile Root, and the candidate/certified
  distinction are all deleted, along with their fields, flags and validation
  branches.
- Two new engine operations: `DeleteInstalled` (refuses the active version and
  any version the host declares in use, moves the directory out of `versions/`
  before deleting, shares `.relkit-apply.lock` with apply) and the FULL_INSTALL
  handoff (the engine waits for the host to exit, runs the verified installer
  interactively, decides on its exit code, and relaunches). macOS refuses
  FULL_INSTALL: desktop `.app` products ship the bundle as a payload.
- `relkit-agent init` is down to `-config`, `-bootstrap` and `-reset`. The
  `-product`, `-token-only`, `-share-with`, `-list-products` and `-remove`
  spellings are gone and fail loudly rather than acting as deprecated no-ops.
  Config files that still name `products`, `uploadToken` or `uploadTokenFile`
  refuse to start.
- The control-plane side layers are deleted outright: resolution, offline
  bundle, wrapper, materialize, production escapes, certification, the
  distribution catalog lifecycle, `selectionPolicy`, `requires`/`provides`,
  `relkitw` generation and the candidate bundle chain.
- Project scope is desktop only. Mobile (iOS, Android), sandboxed distribution
  and Linux are out of scope, and the selector fixtures no longer carry values
  reserved for them.
- Dec has left relkit. The Dec-only audience rule in the human-facing page
  projection is gone, and its test fixtures were renamed rather than kept as
  special cases.
- Go SDK entry points moved into `internal/`: `sdk/*.go` became `internal/sdk`,
  `sdk/updaterfacade` became `internal/updaterfacade`. The Go facade is no
  longer published, and `sdk/node` is deleted.

### Added

- Credential custody manifest (`relkit.custody-manifest/1`) plus
  `relkit custody check/get/public`, so a key is looked up by keyId instead of
  being guessed at from a file path. `scripts/check_trust_keys.py` calls it.
- Server-side tool distribution: `relkit.tools/1`, the agent's `/v1/tools`
  endpoints (including a read-only `current` alias and component addressing),
  and the fixed three-step CI template (download, `ssh-keygen -Y verify`,
  execute). Tool versions and file names come from signed server documents, so
  pipeline yaml never pins either.
- Agent admin interface (`/v1/admin/*`): product registration and removal,
  token issue/rotate/revoke, users and roles. Tokens are stored as hashes and
  authorize per product.
- A management panel with user, role and host-product pages; every write goes
  through the admin interface, and a token is shown exactly once.
- `relkit 0.7` conformance fixtures generated from the real signing path: a
  valid runtime bootstrap and a numeric-array `rootPublicKey` regression that
  reproduces the 2026-10-17 outage.

### Changed

- `internal/site` no longer filters artifacts by audience. A page shows exactly
  what the manifest lists, minus payloads.
- `sdk/dart` is a thin client: the facade plus generated `updater/v1` types,
  nothing else. `proto/buf.gen.dart.yaml` generates only the updater surface,
  while the default template still generates the full Go surface.
- The human-facing page, `catalog.json` and channel documents are unchanged in
  shape; what changed is who signs them (the server root) and what a page may
  contain.

### Removed

- `sdk/node` entirely, and the thick Dart implementation (chain, download,
  envelope, fetch, models, preference, release notes, runtime bootstrap,
  runtime config, scheduler, selectors, state, updater) with its tests,
  fixtures and conformance copies.
- `conformance/updater/legacy-lastResult.json` and the legacy state migration
  path, including the `update_apply.json` double write.
- `conformance/selector/target-dimension.json`: the fixture existed to model a
  desktop client sharing a version number with an Android server.
- `cmd/relkit-updater --capabilities` dead code and the `capabilitiesEvent`
  helper it used.
