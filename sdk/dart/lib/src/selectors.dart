/// Artifact selection (SPEC.md section 11).
library;

import 'models.dart';

/// The artifact this client should download, or null if none fits.
///
/// An artifact matches when every selector it declares equals the client's
/// value for that key. Keys the artifact does not mention are ignored, and so
/// are extra keys the client offers, which is what lets a publisher add a new
/// dimension without invalidating older clients.
///
/// The consequence worth knowing: a client that does not declare a key an
/// artifact requires does not match it. That is the intended reading of "every
/// declared pair must hold", and it is why a new required selector needs a
/// client release before it can be used.
Artifact? selectArtifact(
  Manifest manifest,
  Map<String, String> clientSelectors,
) {
  Artifact? best;
  for (final artifact in manifest.artifacts) {
    if (!_matches(artifact, clientSelectors)) continue;
    // Lowest id wins. Fixing the tie-break rather than taking the first match
    // is what makes every implementation agree on malformed input; publishing
    // side already refuses two artifacts with identical selectors.
    if (best == null || artifact.id.compareTo(best.id) < 0) best = artifact;
  }
  return best;
}

/// Every artifact matching this client, ordered the same way [selectArtifact]
/// orders them. Useful for diagnosing "why did it pick that one".
List<Artifact> matchingArtifacts(
  Manifest manifest,
  Map<String, String> clientSelectors,
) {
  final matches = [
    for (final artifact in manifest.artifacts)
      if (_matches(artifact, clientSelectors)) artifact,
  ];
  matches.sort((a, b) => a.id.compareTo(b.id));
  return matches;
}

bool _matches(Artifact artifact, Map<String, String> clientSelectors) {
  for (final entry in selectorsToMap(artifact.selectors).entries) {
    if (clientSelectors[entry.key] != entry.value) return false;
  }
  return true;
}
