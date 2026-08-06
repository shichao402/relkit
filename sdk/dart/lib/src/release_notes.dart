/// Release-notes helpers for hosts (SPEC.md §5.2 `notes` / `notesUrl`).
///
/// Contract:
/// - The update target carries full Markdown in [VersionNode.notes] (fallback:
///   [Manifest.notes]).
/// - Earlier releases are presented as repository links via [VersionNode.notesUrl]
///   rather than inlined bodies (publish-side compaction when
///   `changelog.urlTemplate` is configured).
library;

import 'models.dart';

/// A historical release whose notes are not inlined in the current hop.
class PriorReleaseNotes {
  const PriorReleaseNotes({
    required this.version,
    required this.code,
    required this.notesUrl,
    this.notes = '',
  });

  final String version;
  final int code;

  /// Absolute http(s) URL to the changelog section in the source repository.
  final String notesUrl;

  /// Usually empty after publish-side compaction; kept for older indexes.
  final String notes;

  bool get hasLink => notesUrl.isNotEmpty;
  bool get hasBody => notes.trim().isNotEmpty;
}

/// Collects prior release-note links from an adopted index.
///
/// Includes every version with `code < targetCode` that still has a [notesUrl]
/// or a non-empty [notes] body, newest first.
List<PriorReleaseNotes> collectPriorReleaseNotes(
  Index index, {
  required int targetCode,
}) {
  final items = <PriorReleaseNotes>[];
  for (final node in index.versions) {
    final code = node.code.toInt();
    if (code >= targetCode) continue;
    final url = node.notesUrl.trim();
    final body = node.notes.trim();
    if (url.isEmpty && body.isEmpty) continue;
    items.add(PriorReleaseNotes(
      version: node.version,
      code: code,
      notesUrl: url,
      notes: body,
    ));
  }
  items.sort((a, b) => b.code.compareTo(a.code));
  return List.unmodifiable(items);
}

/// Markdown body for the update being offered.
String resolveReleaseNotesMarkdown({
  required VersionNode target,
  required Manifest manifest,
}) {
  final fromTarget = target.notes.trim();
  if (fromTarget.isNotEmpty) return fromTarget;
  return manifest.notes.trim();
}
