/**
 * Rank mirrors from real download history only (SPEC.md section 12.7).
 *
 * No probe / speed-test traffic: callers may only call
 * `UpdateState.recordSourceSuccess` / `recordSourceFailure` after authentic
 * directory, index, manifest, or artifact attempts.
 */

import type { UpdateState } from "./state.js";

/**
 * Stable reorder: last success → higher historical bytes/sec → fewer
 * consecutive failures → original order.
 *
 * `items` is the default sequence (already priority-sorted for directory
 * services).
 */
export function rankByLearning<T>(options: {
  items: readonly T[];
  keyOf: (item: T) => string;
  state: UpdateState;
}): T[] {
  const { items, keyOf, state } = options;
  if (items.length <= 1) return [...items];

  const indexed = items.map((item, index) => ({
    item,
    index,
    key: keyOf(item),
  }));

  indexed.sort((a, b) => {
    const aLast = state.lastSuccessfulSourceKey === a.key;
    const bLast = state.lastSuccessfulSourceKey === b.key;
    if (aLast !== bLast) return aLast ? -1 : 1;

    const aBps = state.sourceStats.get(a.key)?.lastBytesPerSecond ?? -1;
    const bBps = state.sourceStats.get(b.key)?.lastBytesPerSecond ?? -1;
    if (aBps !== bBps) return bBps - aBps;

    const aFail = state.sourceStats.get(a.key)?.consecutiveFailures ?? 0;
    const bFail = state.sourceStats.get(b.key)?.consecutiveFailures ?? 0;
    if (aFail !== bFail) return aFail - bFail;

    return a.index - b.index;
  });

  return indexed.map((row) => row.item);
}

export function rankUrlStrings(
  urls: readonly string[],
  state: UpdateState,
): string[] {
  return rankByLearning({ items: urls, keyOf: (url) => url, state });
}
