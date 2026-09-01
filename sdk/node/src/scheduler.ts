/**
 * Background check scheduling (SPEC.md section 12.2 companion).
 *
 * Throttling lives in `shouldCheck` / `RupUpdater.check`. Periodic ticks call
 * `check({ force: false })`. The startup tick may call `check({ force: true })`
 * when `forceOnStart` is true.
 */

import { defaultUpdatePolicy, type UpdatePolicy } from "./state.js";
import type { UpdateCheckResult } from "./updater.js";

/** Scheduler and throttling knobs for a running client. */
export interface UpdateRuntimeConfig {
  /** Whether `start()` runs an immediate tick. */
  checkOnStart: boolean;
  /** When true, the startup tick ignores throttling. */
  forceOnStart: boolean;
  policy: UpdatePolicy;
}

export const defaultRuntimeConfig: UpdateRuntimeConfig = {
  checkOnStart: true,
  forceOnStart: true,
  policy: defaultUpdatePolicy,
};

/** Parses host JSON. Unknown keys are ignored. */
export function runtimeConfigFromJson(raw: unknown): UpdateRuntimeConfig {
  const json =
    typeof raw === "object" && raw !== null
      ? (raw as Record<string, unknown>)
      : {};
  const hours = (value: unknown, fallbackMs: number): number =>
    typeof value === "number" ? value * 60 * 60 * 1000 : fallbackMs;

  return {
    checkOnStart:
      typeof json.checkOnStart === "boolean" ? json.checkOnStart : true,
    forceOnStart:
      typeof json.forceOnStart === "boolean" ? json.forceOnStart : true,
    policy: {
      ...defaultUpdatePolicy,
      afterSuccessMs: hours(
        json.afterSuccessHours,
        defaultUpdatePolicy.afterSuccessMs,
      ),
      afterFailureMs: hours(
        json.afterFailureHours,
        defaultUpdatePolicy.afterFailureMs,
      ),
    },
  };
}

export interface UpdateSchedulerOptions {
  /** Usually `updater.check` or a host wrapper that adds logging / gates. */
  check: (options: { force: boolean }) => Promise<UpdateCheckResult>;
  /**
   * Called for every completed check except `check-throttled` (those only
   * reschedule). Hosts typically open UI on `update-available`.
   */
  onResult: (result: UpdateCheckResult) => void;
  runtime?: Partial<UpdateRuntimeConfig>;
  log?: (message: string) => void;
  now?: () => Date;
}

/** Runs throttled update checks on start and on a wake timer. */
export class UpdateScheduler {
  private readonly runtime: UpdateRuntimeConfig;
  private readonly check: UpdateSchedulerOptions["check"];
  private readonly onResult: UpdateSchedulerOptions["onResult"];
  private readonly log?: (message: string) => void;
  private readonly nowFn: () => Date;

  private running = false;
  private inflight = false;
  private timer: NodeJS.Timeout | null = null;
  private generation = 0;

  constructor(options: UpdateSchedulerOptions) {
    this.check = options.check;
    this.onResult = options.onResult;
    if (options.log) this.log = options.log;
    this.nowFn = options.now ?? (() => new Date());
    this.runtime = {
      ...defaultRuntimeConfig,
      ...options.runtime,
      policy: { ...defaultUpdatePolicy, ...options.runtime?.policy },
    };
  }

  get policy(): UpdatePolicy {
    return this.runtime.policy;
  }

  get isRunning(): boolean {
    return this.running;
  }

  start(overrides: { checkOnStart?: boolean; forceOnStart?: boolean } = {}): void {
    if (this.running) return;
    this.running = true;
    this.generation++;

    const onStart = overrides.checkOnStart ?? this.runtime.checkOnStart;
    const force = overrides.forceOnStart ?? this.runtime.forceOnStart;
    this.log?.(
      `update scheduler started (checkOnStart=${onStart} forceOnStart=${force})`,
    );

    if (onStart) {
      void this.tick(force);
    } else {
      this.arm(this.policy.afterFailureMs);
    }
  }

  /**
   * Cancels the pending wake. An in-flight check may still finish; its result is
   * ignored for rescheduling once stopped.
   */
  stop(): void {
    if (!this.running) return;
    this.running = false;
    this.generation++;
    if (this.timer !== null) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    this.log?.("update scheduler stopped");
  }

  private arm(delayMs: number): void {
    if (this.timer !== null) clearTimeout(this.timer);
    this.timer = null;
    if (!this.running) return;
    const safe = Math.max(0, delayMs);
    const generation = this.generation;
    this.timer = setTimeout(() => {
      if (!this.running || generation !== this.generation) return;
      void this.tick(false);
    }, safe);
    this.timer.unref?.();
  }

  private armUntil(when: Date): void {
    this.arm(when.getTime() - this.nowFn().getTime());
  }

  private async tick(force: boolean): Promise<void> {
    if (!this.running || this.inflight) return;
    this.inflight = true;
    const generation = this.generation;
    try {
      const result = await this.check({ force });
      if (!this.running || generation !== this.generation) return;

      switch (result.kind) {
        case "check-throttled":
          this.log?.(`scheduler throttled until ${result.nextAllowedAt.toISOString()}`);
          this.armUntil(result.nextAllowedAt);
          break;
        case "up-to-date":
        case "update-available":
        case "fallback-required":
          // Surface to the user, but do not poll the network again until the
          // success interval elapses (install / skip happens on the host).
          this.onResult(result);
          this.arm(this.policy.afterSuccessMs);
          break;
        case "check-failed":
          this.onResult(result);
          this.arm(this.policy.afterFailureMs);
          break;
      }
    } catch (error) {
      this.log?.(`scheduler tick failed: ${String(error)}`);
      if (this.running && generation === this.generation) {
        this.arm(this.policy.afterFailureMs);
      }
    } finally {
      this.inflight = false;
    }
  }
}
