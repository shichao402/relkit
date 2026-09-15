import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";
import { fromJson, toJson } from "@bufbuild/protobuf";
import { CheckResultSchema } from "./dist/updater_pb.js";

const fixtures = [
  "check-result-up-to-date.json",
  "check-result-empty-notes.json",
  "check-result-fallback-required.json",
  "check-result-throttled.json",
  "check-result-failed.json",
];

for (const name of fixtures) {
  test(`canonical ProtoJSON round trip: ${name}`, () => {
    const expected = JSON.parse(
      readFileSync(join("../../conformance/updater", name), "utf8"),
    );
    const message = fromJson(CheckResultSchema, expected, {
      ignoreUnknownFields: false,
    });
    assert.deepEqual(
      toJson(CheckResultSchema, message, { alwaysEmitImplicit: true }),
      expected,
    );
  });
}
