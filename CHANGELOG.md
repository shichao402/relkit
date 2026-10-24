# Changelog

All notable changes to relkit are recorded here. The version in `VERSION.json`
is the single source of truth; this file explains what changed and, above all,
what will break.

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
