/// Upgrade path selection (SPEC.md section 9).
///
/// Pure functions over a parsed index. This is the part of the protocol most
/// likely to be reimplemented subtly differently in each language, which is why
/// `conformance/version-select/` exists and why this file has no I/O in it.
library;

import 'models.dart';

/// The version this client should move to next, or null if there is none.
///
/// SPEC.md section 9.2. A candidate must not be yanked, must be newer than the
/// client, and must accept the client's current code as a starting point.
VersionNode? selectNextTarget(Index index, int currentCode) {
  VersionNode? best;
  for (final node in index.versions) {
    final nodeCode = node.code.toInt();
    if (node.yanked) continue;
    if (nodeCode <= currentCode) continue;
    if (node.minFrom.toInt() > currentCode) continue;
    // Highest code wins, never "the last one in the array". An index should be
    // sorted, but a client that relies on that silently returns the wrong
    // version when it is not, and nothing about the result looks wrong.
    if (best == null || nodeCode > best.code.toInt()) best = node;
  }
  return best;
}

/// Every hop from here to the newest reachable version (SPEC.md section 9.5).
///
/// Terminates because each hop strictly increases the code and the version list
/// is finite. There is deliberately no iteration cap: a cap could only ever
/// turn a correct long chain into a spurious failure.
List<VersionNode> resolveUpgradePath(Index index, int currentCode) {
  final path = <VersionNode>[];
  var code = currentCode;
  while (true) {
    final next = selectNextTarget(index, code);
    if (next == null) return path;
    path.add(next);
    code = next.code.toInt();
  }
}

/// Whether the client is below the floor the publisher still supports.
///
/// Orthogonal to which version comes next: this answers "may I keep running",
/// while [selectNextTarget] answers "where do I go". A client can be required
/// to update and still only be able to reach an intermediate version.
bool isMandatory(Index index, int currentCode) {
  if (!index.hasMinSupported_7) return false;
  return currentCode < index.minSupported.toInt();
}
