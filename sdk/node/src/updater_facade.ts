/** Generated host facade (relkit.updater.v1). Do not hand-edit the method set. */

import { spawn } from "node:child_process";
import { Buffer } from "node:buffer";
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { fromBinary, toBinary } from "@bufbuild/protobuf";
import {
  UpdaterEventSchema,
  UpdaterRequestSchema,
  type Capabilities,
  type CheckPolicy,
  type CheckResult,
  type ClientProfile,
  type DownloadResult,
  type ApplyResult,
  type Error,
  type Runtime,
  type StatusSnapshot,
  type UpdaterEvent,
  type UpdaterRequest,
  type Result,
} from "./gen/updater/v1/updater_pb.js";

export const ipcMin = 1;
export const ipcMax = 1;
export const ipcCurrent = 1;

export type Glue = {
  locate(runtime: Runtime): Promise<string>;
  run(bin: string, args: string[], stdin: Uint8Array): AsyncIterable<UpdaterEvent>;
};

function frame(bytes: Uint8Array): Uint8Array {
  const out = Buffer.alloc(4 + bytes.length);
  out.writeUInt32BE(bytes.length, 0);
  Buffer.from(bytes).copy(out, 4);
  return out;
}

async function* decodeFrames(stream: AsyncIterable<Buffer>): AsyncGenerator<UpdaterEvent> {
  let buf = Buffer.alloc(0);
  for await (const chunk of stream) {
    buf = Buffer.concat([buf, chunk]);
    while (buf.length >= 4) {
      const n = buf.readUInt32BE(0);
      if (buf.length < 4 + n) break;
      const payload = buf.subarray(4, 4 + n);
      buf = buf.subarray(4 + n);
      yield fromBinary(UpdaterEventSchema, payload);
    }
  }
}

export const defaultGlue: Glue = {
  async locate(runtime) {
    const name = process.platform === "win32" ? "relkit-updater.exe" : "relkit-updater";
    const candidates: string[] = [];
    if (runtime.sidecarPath) candidates.push(runtime.sidecarPath);
    if (process.env.RELKIT_UPDATER) candidates.push(process.env.RELKIT_UPDATER);
    if (runtime.install?.installRoot) {
      candidates.push(path.join(runtime.install.installRoot, name));
    }
    candidates.push(path.join(path.dirname(process.execPath), name));
    for (const c of candidates) {
      if (c && existsSync(c)) return c;
    }
    throw new Error("sidecar not found");
  },
  async *run(bin, args, stdin) {
    const child = spawn(bin, args, { stdio: ["pipe", "pipe", "inherit"] });
    child.stdin.end(Buffer.from(stdin));
    yield* decodeFrames(child.stdout);
    await new Promise<void>((resolve, reject) => {
      child.on("error", reject);
      child.on("close", () => resolve());
    });
  },
};

export class Updater {
  constructor(
    private readonly profile: ClientProfile,
    private readonly runtime: Runtime,
    private readonly glue: Glue,
    private readonly bin: string,
    readonly capabilities: Capabilities,
  ) {}

  static async open(
    profile: ClientProfile,
    runtime: Runtime,
    glue: Glue = defaultGlue,
  ): Promise<{ kind: "opened"; updater: Updater; capabilities: Capabilities } | { kind: "failed"; error: Error }> {
    try {
      const bin = await glue.locate(runtime);
      let caps: Capabilities | undefined;
      let ferr: Error | undefined;
      for await (const ev of glue.run(bin, ["-capabilities"], new Uint8Array())) {
        if (ev.kind.case === "capabilities") caps = ev.kind.value;
        if (ev.kind.case === "failed") ferr = ev.kind.value.error;
      }
      if (ferr) return { kind: "failed", error: ferr };
      if (!caps) {
        return {
          kind: "failed",
          error: { code: 8, retryable: false, message: "no capabilities", attempts: [] },
        };
      }
      if (caps.ipc < ipcMin) {
        return { kind: "failed", error: { code: 9, retryable: false, message: "sidecar IPC too old", attempts: [] } };
      }
      if (caps.ipc > ipcMax) {
        return { kind: "failed", error: { code: 10, retryable: false, message: "sidecar IPC too new", attempts: [] } };
      }
      return { kind: "opened", updater: new Updater(profile, runtime, glue, bin, caps), capabilities: caps };
    } catch (e) {
      return {
        kind: "failed",
        error: { code: 19, retryable: false, message: String(e), attempts: [] },
      };
    }
  }

  private async *call(partial: Partial<UpdaterRequest> & { op?: UpdaterRequest["op"] }): AsyncGenerator<UpdaterEvent> {
    const req = {
      hello: { ipcMin, ipcMax },
      profile: this.profile,
      runtime: this.runtime,
      ...partial,
    } as UpdaterRequest;
    const bytes = frame(toBinary(UpdaterRequestSchema, req));
    yield* this.glue.run(this.bin, [], bytes);
  }

  async check(opts: { force?: boolean; exactCode?: bigint; policy?: CheckPolicy } = {}): Promise<CheckResult> {
    for await (const ev of this.call({
      op: { case: "check", value: { force: !!opts.force, exactCode: opts.exactCode ?? 0n, policy: opts.policy } },
    })) {
      if (ev.kind.case === "check") return ev.kind.value;
      if (ev.kind.case === "failed") return { kind: { case: "failed", value: ev.kind.value } } as CheckResult;
    }
    return { kind: { case: "failed", value: { error: { code: 1, message: "no check result", retryable: true, attempts: [] } } } } as CheckResult;
  }

  async skip(opts: { code: bigint }): Promise<Result> {
    for await (const ev of this.call({ op: { case: "skip", value: { code: opts.code } } })) {
      if (ev.kind.case === "result") return ev.kind.value;
    }
    return { kind: { case: "ok", value: {} } } as Result;
  }

  async download(opts: { planId: string }, onEvent?: (e: UpdaterEvent) => void): Promise<DownloadResult> {
    for await (const ev of this.call({ op: { case: "download", value: { planId: opts.planId } } })) {
      onEvent?.(ev);
      if (ev.kind.case === "download") return ev.kind.value;
    }
    return { kind: { case: "failed", value: { error: { code: 1, message: "no download result", retryable: true, attempts: [] } } } } as DownloadResult;
  }

  async apply(opts: { planId: string }, onEvent?: (e: UpdaterEvent) => void): Promise<ApplyResult> {
    for await (const ev of this.call({ op: { case: "apply", value: { planId: opts.planId } } })) {
      onEvent?.(ev);
      if (ev.kind.case === "apply") return ev.kind.value;
    }
    return { kind: { case: "failed", value: { error: { code: 1, message: "no apply result", retryable: true, attempts: [] } } } } as ApplyResult;
  }

  async status(): Promise<StatusSnapshot> {
    for await (const ev of this.call({ op: { case: "status", value: {} } })) {
      if (ev.kind.case === "status") return ev.kind.value;
    }
    return {} as StatusSnapshot;
  }

  async cleanup(): Promise<Result> {
    for await (const ev of this.call({ op: { case: "cleanup", value: {} } })) {
      if (ev.kind.case === "result") return ev.kind.value;
    }
    return { kind: { case: "ok", value: {} } } as Result;
  }

  async cancel(): Promise<Result> {
    return { kind: { case: "ok", value: {} } } as Result;
  }
}

void fileURLToPath;
