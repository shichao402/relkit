/** Artifact selection (SPEC.md section 11). */

import { selectorsToMap, type Artifact, type Manifest } from "./models.js";

/**
 * Whether every key in `required` equals the client's value for that key. An
 * empty `required` map matches every client.
 */
export function matchesSelectors(
  required: Record<string, string>,
  clientSelectors: Record<string, string>,
): boolean {
  for (const [key, value] of Object.entries(required)) {
    if (clientSelectors[key] !== value) return false;
  }
  return true;
}

function matches(
  artifact: Artifact,
  clientSelectors: Record<string, string>,
): boolean {
  return matchesSelectors(selectorsToMap(artifact.selectors), clientSelectors);
}

/**
 * The artifact this client should download, or null if none fits.
 *
 * An artifact matches when every selector it declares equals the client's value
 * for that key. Keys the artifact does not mention are ignored, and so are
 * extra keys the client offers, which is what lets a publisher add a new
 * dimension without invalidating older clients.
 *
 * The consequence worth knowing: a client that does not declare a key an
 * artifact requires does not match it. That is the intended reading of "every
 * declared pair must hold", and it is why a new required selector needs a
 * client release before it can be used.
 */
export function selectArtifact(
  manifest: Manifest,
  clientSelectors: Record<string, string>,
): Artifact | null {
  let best: Artifact | null = null;
  for (const artifact of manifest.artifacts) {
    if (!matches(artifact, clientSelectors)) continue;
    // Lowest id wins. Fixing the tie-break rather than taking the first match
    // is what makes every implementation agree on malformed input; publishing
    // side already refuses two artifacts with identical selectors.
    if (best === null || artifact.id < best.id) best = artifact;
  }
  return best;
}

/**
 * Every artifact matching this client, ordered the same way `selectArtifact`
 * orders them. Useful for diagnosing "why did it pick that one".
 */
export function matchingArtifacts(
  manifest: Manifest,
  clientSelectors: Record<string, string>,
): Artifact[] {
  return manifest.artifacts
    .filter((artifact) => matches(artifact, clientSelectors))
    .sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
}
