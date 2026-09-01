/**
 * Filename safety for artifacts (SPEC.md section 14.4).
 *
 * A manifest is signed, but "signed" only means the publisher wrote it, not that
 * every field is sane. `filename` is the one field that becomes a path on the
 * client's disk, so it is validated before use rather than trusted.
 */

const CONTROL_CHARS = /[\u0000-\u001f\u007f]/;

const WINDOWS_RESERVED = new Set([
  "con",
  "prn",
  "aux",
  "nul",
  ...Array.from({ length: 9 }, (_, i) => `com${i + 1}`),
  ...Array.from({ length: 9 }, (_, i) => `lpt${i + 1}`),
]);

/**
 * Returns null when the filename is safe to join onto a directory, or a reason
 * why it is not.
 */
export function checkArtifactFilename(filename: string): string | null {
  if (filename.length === 0) return "filename is empty";
  if (filename.includes("/") || filename.includes("\\")) {
    return `filename "${filename}" contains a path separator`;
  }
  if (filename === "." || filename === "..") {
    return `filename "${filename}" is a directory reference`;
  }
  if (filename.split(/[.]/).includes("..")) {
    return `filename "${filename}" contains a parent-directory segment`;
  }
  if (CONTROL_CHARS.test(filename)) {
    return `filename "${filename}" contains control characters`;
  }
  const stem = filename.split(".")[0]?.toLowerCase() ?? "";
  if (WINDOWS_RESERVED.has(stem)) {
    return `filename "${filename}" is a reserved Windows device name`;
  }
  return null;
}
