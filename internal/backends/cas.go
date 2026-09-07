package backends

import (
	"fmt"

	"cnb.cool/shichao402/relkit/internal/model"
)

// Ingest is the optional content-addressed write path on a data-plane backend.
// publish.Run must not switch on Type(); it type-asserts this interface.
//
// Head compares size only and does not re-hash. Promote is a same-store copy
// (S3 CopyObject / local hardlink).
type Ingest interface {
	Head(key string) (size int64, exists bool, err error)
	Promote(srcKey, dstKey string) (urls []string, err error)
}

// Deleter is optional. Ingest backends implement it so publish can drop cas
// blobs that no live index/manifest still names.
type Deleter interface {
	Delete(key string) error
}

// PutArtifactCAS writes localPath into cas/{sha256} if missing, then Promotes
// that blob to artifactKey. When cas already has the same size, the local file
// is not uploaded again.
//
// Backends that do not implement Ingest keep PutArtifact (full upload).
func PutArtifactCAS(backend Backend, localPath, artifactKey, sha256 string, size int64) (urls []string, skippedUpload bool, err error) {
	ingest, ok := backend.(Ingest)
	if !ok {
		urls, err = backend.PutArtifact(localPath, artifactKey)
		return urls, false, err
	}
	casKey, err := model.CasKey(sha256)
	if err != nil {
		return nil, false, err
	}
	gotSize, exists, err := ingest.Head(casKey)
	if err != nil {
		return nil, false, err
	}
	if exists && gotSize == size {
		urls, err = ingest.Promote(casKey, artifactKey)
		return urls, true, err
	}
	if _, err := backend.PutArtifact(localPath, casKey); err != nil {
		return nil, false, fmt.Errorf("put cas %s: %w", casKey, err)
	}
	urls, err = ingest.Promote(casKey, artifactKey)
	return urls, false, err
}
