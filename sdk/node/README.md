# rup-client (Node / TypeScript)

Official Node client SDK for RUP v2. Implements the client half of
[`SPEC.md`](../../SPEC.md): discover a signed index, verify it, pick the next
version, download and verify the artifact.

Agent onboarding: [`AGENT-QUICKSTART.md`](AGENT-QUICKSTART.md).

## Requirements

- Node >= 20 (built-in Ed25519 via `node:crypto`, global `fetch`)
- ESM. CommonJS hosts must use `await import("rup-client")`.

## Layout

| Path | What |
|---|---|
| `src/gen/` | Generated from `proto/rup/v2/*.proto`, checked in |
| `src/models.ts` | Schema ids, parsing, structural validation, `bigint` → `number` |
| `src/envelope.ts` | Ed25519 envelope verification (SPEC §4.1) |
| `src/chain.ts` | `selectNextTarget` / `resolveUpgradePath` / `isMandatory` (§9) |
| `src/selectors.ts` | Artifact matching and tie-break (§11) |
| `src/state.ts` | State store, throttle, `acceptsSequence` (§12.2 / §12.4) |
| `src/preference.ts` | Source learning (§12.7) |
| `src/fetch.ts` | Network boundary: timeouts, redirects, cache-busting (§3.2) |
| `src/download.ts` | Mirror fallback, Range resume, size + sha256 (§12.3) |
| `src/filename.ts` | Artifact filename safety (§14.4) |
| `src/updater.ts` | The check flow (§12.1) and fallback merge (§12.6) |
| `src/scheduler.ts` | Start / periodic checks |

No `apply/`. Installing the verified file is host-specific and deliberately out
of scope; see AGENT-QUICKSTART §N4.

## Build and test

```bash
npm install
npm run build        # tsc -> dist/
npm test             # conformance fixtures + unit tests
npm run generate     # only when proto/ changes (needs buf)
```

`npm run generate` runs `sdk/node/buf.gen.yaml` (`protoc-gen-es` is an npm
devDependency, so it is not in the repo-root `proto/buf.gen.yaml`). Go and Dart
still use `scripts/gen-proto.ps1`. Changing a `.proto` file means running both.

`npm test` reads `../../conformance/` in place rather than a copy. A copy would
drift, and a drifted copy is worse than none: the suite would stay green while
implementations diverged. Override the location with `RUP_CONFORMANCE_DIR`.

`reachability/` is not run: it is a publishing-side check (§10) and a client has
no use for it.

## Alignment notes

Three behaviours are load-bearing and were written against the Dart
implementation, which the fixtures already pin:

- Signatures are verified over the **raw** `Envelope.payload` bytes, never over a
  re-serialisation of the parsed object.
- An unknown `keyId` does not stop the loop; rotation depends on continuing.
- `selectNextTarget` takes the **highest** `code`, never the last array element.

Integers: the generated code uses `bigint` for proto int64. `models.ts` narrows
to `number` at the edge so codes, sizes and sequences can be compared and summed
without mixing the two numeric types at runtime.
