/**
 * Upgrade path selection (SPEC.md section 9).
 *
 * Pure functions over a parsed index. This is the part of the protocol most
 * likely to be reimplemented subtly differently in each language, which is why
 * `conformance/version-select/` exists and why this file has no I/O in it.
 */

import {
  indexMinSupported,
  nodeCode,
  nodeMinFrom,
  type Index,
  type VersionNode,
} from "./models.js";

/**
 * The version this client should move to next, or null if there is none.
 *
 * SPEC.md section 9.2. A candidate must not be yanked, must be newer than the
 * client, and must accept the client's current code as a starting point.
 */
export function selectNextTarget(
  index: Index,
  currentCode: number,
): VersionNode | null {
  let best: VersionNode | null = null;
  for (const node of index.versions) {
    const code = nodeCode(node);
    if (node.yanked) continue;
    if (code <= currentCode) continue;
    if (nodeMinFrom(node) > currentCode) continue;
    // Highest code wins, never "the last one in the array". An index should be
    // sorted, but a client that relies on that silently returns the wrong
    // version when it is not, and nothing about the result looks wrong.
    if (best === null || code > nodeCode(best)) best = node;
  }
  return best;
}

/**
 * Every hop from here to the newest reachable version (SPEC.md section 9.5).
 *
 * Terminates because each hop strictly increases the code and the version list
 * is finite. There is deliberately no iteration cap: a cap could only ever turn
 * a correct long chain into a spurious failure.
 */
export function resolveUpgradePath(
  index: Index,
  currentCode: number,
): VersionNode[] {
  const path: VersionNode[] = [];
  let code = currentCode;
  for (;;) {
    const next = selectNextTarget(index, code);
    if (next === null) return path;
    path.push(next);
    code = nodeCode(next);
  }
}

/**
 * Whether the client is below the floor the publisher still supports.
 *
 * Orthogonal to which version comes next: this answers "may I keep running",
 * while `selectNextTarget` answers "where do I go". A client can be required to
 * update and still only be able to reach an intermediate version.
 */
export function isMandatory(index: Index, currentCode: number): boolean {
  if (!index.hasMinSupported) return false;
  return currentCode < indexMinSupported(index);
}
