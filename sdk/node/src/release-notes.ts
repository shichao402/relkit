/**
 * Release-notes helpers for hosts (SPEC.md §5.2 `notes` / `notesUrl`).
 *
 * Contract:
 * - The update target carries full Markdown in `VersionNode.notes` (fallback:
 *   `Manifest.notes`).
 * - Earlier releases are presented as repository links via `VersionNode.notesUrl`
 *   rather than inlined bodies (publish-side compaction when
 *   `changelog.urlTemplate` is configured).
 */

import { nodeCode, type Index, type Manifest, type VersionNode } from "./models.js";

/** A historical release whose notes are not inlined in the current hop. */
export interface PriorReleaseNotes {
  version: string;
  code: number;
  /** Absolute http(s) URL to the changelog section in the source repository. */
  notesUrl: string;
  /** Usually empty after publish-side compaction; kept for older indexes. */
  notes: string;
}

/**
 * Collects prior release-note links from an adopted index.
 *
 * Includes every version with `code < targetCode` that still has a `notesUrl` or
 * a non-empty `notes` body, newest first.
 */
export function collectPriorReleaseNotes(
  index: Index,
  targetCode: number,
): PriorReleaseNotes[] {
  const items: PriorReleaseNotes[] = [];
  for (const node of index.versions) {
    const code = nodeCode(node);
    if (code >= targetCode) continue;
    const url = node.notesUrl.trim();
    const body = node.notes.trim();
    if (url.length === 0 && body.length === 0) continue;
    items.push({ version: node.version, code, notesUrl: url, notes: body });
  }
  items.sort((a, b) => b.code - a.code);
  return items;
}

/** Markdown body for the update being offered. */
export function resolveReleaseNotesMarkdown(
  target: VersionNode,
  manifest: Manifest,
): string {
  const fromTarget = target.notes.trim();
  if (fromTarget.length > 0) return fromTarget;
  return manifest.notes.trim();
}
