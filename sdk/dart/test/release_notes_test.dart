import 'package:fixnum/fixnum.dart';
import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

void main() {
  test('collectPriorReleaseNotes skips target and empties, newest first', () {
    final index = Index(
      schema: indexSchemaId,
      product: 'demo',
      channel: 'stable',
      sequence: Int64(3),
      generatedAt: '2026-01-01T00:00:00Z',
      versions: [
        VersionNode(
          version: '1.0.0',
          code: Int64(100),
          notesUrl: 'https://example.com/#v1.0.0',
          manifest: DigestRef(
            sha256: 'a' * 64,
            size: Int64(1),
            urls: ['https://example.com/m1'],
          ),
        ),
        VersionNode(
          version: '1.1.0',
          code: Int64(110),
          notes: 'should not appear as prior of itself',
          manifest: DigestRef(
            sha256: 'b' * 64,
            size: Int64(1),
            urls: ['https://example.com/m2'],
          ),
        ),
        VersionNode(
          version: '1.0.5',
          code: Int64(105),
          notes: 'legacy body only',
          manifest: DigestRef(
            sha256: 'c' * 64,
            size: Int64(1),
            urls: ['https://example.com/m3'],
          ),
        ),
      ],
    );

    final prior = collectPriorReleaseNotes(index, targetCode: 110);
    expect(prior.map((e) => e.version).toList(), ['1.0.5', '1.0.0']);
    expect(prior.first.hasBody, isTrue);
    expect(prior.last.hasLink, isTrue);
  });

  test('resolveReleaseNotesMarkdown prefers target.notes', () {
    final target = VersionNode(version: '1.1.0', code: Int64(110), notes: '# New');
    final manifest = Manifest(
      schema: manifestSchemaId,
      product: 'demo',
      version: '1.1.0',
      code: Int64(110),
      notes: 'from manifest',
      artifacts: [
        Artifact(
          id: 'default',
          filename: 'a.zip',
          size: Int64(1),
          sha256: 'd' * 64,
          urls: ['https://example.com/a.zip'],
        ),
      ],
    );
    expect(
      resolveReleaseNotesMarkdown(target: target, manifest: manifest),
      '# New',
    );
  });
}
