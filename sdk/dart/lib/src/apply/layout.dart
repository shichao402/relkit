/// How a verified package is turned into a running installation.
///
/// Applying a file is not RUP protocol (see `rup_client.dart`). These layouts
/// are the strategies the SDK offers so hosts do not each invent a slightly
/// wrong Windows file-lock dance. A host still *declares* which one its
/// artifact uses; this file is the capability table that rejects illegal
/// combinations.
library;

import 'dart:io';

/// On-disk shape of an installation the SDK knows how to apply.
enum InstallLayout {
  /// Replace the entire install root (today's portable / `.app` swap).
  wholeRoot,

  /// Write `versions/<id>/` beside a stable launcher and atomically switch
  /// `active.json`.
  versionedDir,
}

/// Default layout for a host OS. Products may declare the same value; they
/// may not declare an unsupported one.
InstallLayout defaultInstallLayoutFor(String operatingSystem) {
  switch (operatingSystem) {
    case 'windows':
      return InstallLayout.versionedDir;
    case 'macos':
      return InstallLayout.wholeRoot;
    default:
      return InstallLayout.wholeRoot;
  }
}

/// Whether [layout] is allowed on [operatingSystem].
bool isInstallLayoutSupported({
  required String operatingSystem,
  required InstallLayout layout,
}) {
  if (operatingSystem == 'macos' && layout == InstallLayout.versionedDir) {
    return false;
  }
  return true;
}

/// Throws if the product declared an unsupported pair.
void ensureInstallLayoutSupported({
  required String operatingSystem,
  required InstallLayout layout,
}) {
  if (isInstallLayoutSupported(
    operatingSystem: operatingSystem,
    layout: layout,
  )) {
    return;
  }
  throw ArgumentError(
    'install layout ${layout.name} is not supported on $operatingSystem',
  );
}

/// Where the apply session file lives so a non-Flutter launcher can find it.
///
/// [versionedDir]: the install root is stable, so the file sits next to the
/// launcher. [wholeRoot]: the install root is what is being replaced, so the
/// file must live in application-support.
File resolveApplySessionFile({
  required InstallLayout layout,
  required Directory installDir,
  required Directory appSupportDir,
  String fileName = 'update_apply.json',
}) {
  final dir = layout == InstallLayout.versionedDir
      ? installDir
      : appSupportDir;
  return File('${dir.path}${Platform.pathSeparator}$fileName');
}

/// Parses `--layout` / a stored name. Unknown values are null, not a guess.
InstallLayout? installLayoutByName(String? name) {
  if (name == null || name.isEmpty) return null;
  for (final candidate in InstallLayout.values) {
    if (candidate.name == name) return candidate;
  }
  return null;
}
