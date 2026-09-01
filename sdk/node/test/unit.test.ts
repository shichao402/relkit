/**
 * Unit tests for behaviour the shared fixtures do not cover.
 *
 * `conformance/` pins the four normative areas. These cover the parts that are
 * this SDK's own responsibility: filename safety, throttling, source learning,
 * and the state store's failure mode.
 */

import assert from "node:assert/strict";
import { mkdtemp, open, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, test } from "node:test";
import { create } from "@bufbuild/protobuf";

import { downloadArtifact } from "../src/download.js";
import { toTrustedKeys, TrustedKeys } from "../src/envelope.js";
import { type Fetcher } from "../src/fetch.js";
import { checkArtifactFilename } from "../src/filename.js";
import { ArtifactSchema } from "../src/models.js";
import {
  FileUpdateStateStore,
  UpdateState,
  defaultUpdatePolicy,
  resolvePolicy,
  shouldCheck,
} from "../src/state.js";
import { rankUrlStrings } from "../src/preference.js";

describe("filename safety (SPEC.md section 14.4)", () => {
  test("accepts an ordinary artifact filename", () => {
    assert.equal(checkArtifactFilename("cronkit-0.1.0-win-x64.zip"), null);
  });

  for (const bad of [
    "",
    ".",
    "..",
    "a/b.zip",
    "a\\b.zip",
    "..\\evil.zip",
    "../evil.zip",
    "con",
    "CON.zip",
    "lpt9.bin",
    "nul.tar.gz",
  ]) {
    test(`rejects ${JSON.stringify(bad)}`, () => {
      assert.notEqual(
        checkArtifactFilename(bad),
        null,
        `expected ${JSON.stringify(bad)} to be refused`,
      );
    });
  }

  test("rejects control characters and NUL", () => {
    assert.notEqual(checkArtifactFilename("a\u0000b.zip"), null);
    assert.notEqual(checkArtifactFilename("a\u001fb.zip"), null);
  });
});

describe("throttling (SPEC.md section 12.2)", () => {
  test("first run always checks", () => {
    assert.equal(shouldCheck(defaultUpdatePolicy, new UpdateState()), true);
  });

  test("waits 24h after success and 1h after error", () => {
    const policy = resolvePolicy();
    const now = new Date("2026-08-31T12:00:00Z");

    const ok = new UpdateState();
    ok.lastResult = "up-to-date";
    ok.lastCheckAt = new Date(now.getTime() - 23 * 60 * 60 * 1000);
    assert.equal(shouldCheck(policy, ok, now), false);
    ok.lastCheckAt = new Date(now.getTime() - 25 * 60 * 60 * 1000);
    assert.equal(shouldCheck(policy, ok, now), true);

    const failed = new UpdateState();
    failed.lastResult = "error";
    failed.lastCheckAt = new Date(now.getTime() - 30 * 60 * 1000);
    assert.equal(shouldCheck(policy, failed, now), false);
    failed.lastCheckAt = new Date(now.getTime() - 61 * 60 * 1000);
    assert.equal(shouldCheck(policy, failed, now), true);
  });
});

describe("sequence high-water mark (SPEC.md section 12.4)", () => {
  test("never moves down", () => {
    const state = new UpdateState();
    state.observeSequence(42);
    state.observeSequence(7);
    assert.equal(state.lastSeenSequence, 42);
  });

  test("index, fallback and directory sequences are independent", () => {
    const state = new UpdateState();
    state.observeSequence(10);
    state.observeFallbackSequence(3);
    state.observeDirectorySequence(5);
    assert.equal(state.lastSeenSequence, 10);
    assert.equal(state.lastSeenFallbackSequence, 3);
    assert.equal(state.lastSeenDirectorySequence, 5);
  });
});

describe("source learning (SPEC.md section 12.7)", () => {
  test("last success wins, then throughput, then original order", () => {
    const state = new UpdateState();
    state.recordSourceSuccess("https://b.example/x", 1000);
    state.recordSourceSuccess("https://c.example/x", 5000);

    const ranked = rankUrlStrings(
      ["https://a.example/x", "https://b.example/x", "https://c.example/x"],
      state,
    );
    // c was the most recent success, so it leads regardless of order.
    assert.equal(ranked[0], "https://c.example/x");
    assert.equal(ranked[1], "https://b.example/x");
    assert.equal(ranked[2], "https://a.example/x");
  });

  test("consecutive failures sink a source", () => {
    const state = new UpdateState();
    state.recordSourceFailure("https://a.example/x");
    state.recordSourceFailure("https://a.example/x");

    const ranked = rankUrlStrings(
      ["https://a.example/x", "https://b.example/x"],
      state,
    );
    assert.equal(ranked[0], "https://b.example/x");
  });

  test("a single candidate is returned untouched", () => {
    const state = new UpdateState();
    assert.deepEqual(rankUrlStrings(["https://only.example/x"], state), [
      "https://only.example/x",
    ]);
  });
});

describe("FileUpdateStateStore", () => {
  test("round-trips state and survives corruption", async () => {
    const dir = await mkdtemp(join(tmpdir(), "rup-node-state-"));
    try {
      const store = new FileUpdateStateStore({
        directory: dir,
        product: "cronkit",
        channel: "stable",
      });

      const state = new UpdateState();
      state.observeSequence(9);
      state.skipped.add(120);
      state.recordSourceSuccess("service:primary", 2048);
      await store.save(state);

      const loaded = await store.load();
      assert.equal(loaded.lastSeenSequence, 9);
      assert.equal(loaded.skipped.has(120), true);
      assert.equal(loaded.lastSuccessfulSourceKey, "service:primary");
      assert.equal(
        loaded.sourceStats.get("service:primary")?.lastBytesPerSecond,
        2048,
      );

      // Corrupt state must not block updating: it degrades to a fresh state
      // rather than throwing, because refusing to run would disable updates
      // permanently until someone deletes a file they do not know about.
      await writeFile(store.path, "{ not json", "utf8");
      const recovered = await store.load();
      assert.equal(recovered.lastSeenSequence, null);
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });

  test("separates files per product and channel", async () => {
    const dir = await mkdtemp(join(tmpdir(), "rup-node-state-"));
    try {
      const stable = new FileUpdateStateStore({
        directory: dir,
        product: "cronkit",
        channel: "stable",
      });
      const beta = new FileUpdateStateStore({
        directory: dir,
        product: "cronkit",
        channel: "beta",
      });
      assert.notEqual(stable.path, beta.path);

      const state = new UpdateState();
      state.observeSequence(5);
      await stable.save(state);
      assert.equal((await beta.load()).lastSeenSequence, null);

      const raw = JSON.parse(await readFile(stable.path, "utf8")) as {
        lastSeenSequence: number;
      };
      assert.equal(raw.lastSeenSequence, 5);
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });
});

describe("trusted key input forms", () => {
  // Regression: a plain object is the obvious way to write this, and passing one
  // used to crash inside verification with "trusted.get is not a function".
  const raw = new Uint8Array(32).fill(3);
  const base64 = Buffer.from(raw).toString("base64");

  test("accepts a plain object of raw bytes", () => {
    const keys = toTrustedKeys({ k1: raw });
    assert.deepEqual(keys.keyIds, ["k1"]);
    assert.equal(keys.isEmpty, false);
  });

  test("accepts a Map and base64 string values", () => {
    const fromMap = toTrustedKeys(new Map([["k1", raw]]));
    assert.deepEqual(fromMap.keyIds, ["k1"]);

    const fromBase64 = toTrustedKeys({ k1: base64 });
    assert.deepEqual(fromBase64.keyIds, ["k1"]);
  });

  test("passes a prebuilt TrustedKeys through unchanged", () => {
    const built = TrustedKeys.fromBase64({ k1: base64 });
    assert.equal(toTrustedKeys(built), built);
  });

  test("rejects a key that is not 32 bytes", () => {
    assert.throws(() => toTrustedKeys({ k1: new Uint8Array(31) }), TypeError);
  });

  test("an empty set is reported as empty rather than accepted", () => {
    assert.equal(toTrustedKeys({}).isEmpty, true);
  });
});

describe("downloadArtifact preconditions", () => {
  const artifact = create(ArtifactSchema, {
    id: "windows-x64",
    filename: "app.zip",
    size: 4n,
    sha256: "a".repeat(64),
    urls: ["https://example.invalid/app.zip"],
  });

  const fetcher: Fetcher = {
    getBytes: async () => {
      throw new Error("not used");
    },
    probe: async () => {
      throw new Error("not used");
    },
    download: async () => {
      throw new Error("not used");
    },
    downloadRange: async () => {
      throw new Error("not used");
    },
    close: async () => {},
  };

  test("a missing destinationDir blames the caller, not node:path", async () => {
    await assert.rejects(
      () =>
        downloadArtifact(artifact, {
          fetcher,
          destinationDir: undefined as unknown as string,
        }),
      (error: unknown) =>
        error instanceof TypeError && /destinationDir/.test(error.message),
    );
  });
});

describe("resumable .part sizing on Windows", () => {
  // Regression: the pre-allocation used to open the .part file in append mode
  // and truncate through that handle, which is EPERM on Windows because an
  // append handle carries only FILE_APPEND_DATA.
  test("truncates a file created in append mode", async () => {
    const dir = await mkdtemp(join(tmpdir(), "rup-node-part-"));
    try {
      const partial = join(dir, "app.zip.part");
      const create = await open(partial, "a");
      await create.close();

      const handle = await open(partial, "r+");
      try {
        await handle.truncate(4096);
      } finally {
        await handle.close();
      }
      assert.equal((await stat(partial)).size, 4096);
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });
});
