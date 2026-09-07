package backends

import (
	"fmt"
	"io"
	"os"
	"time"

	"cnb.cool/shichao402/relkit/internal/model"
)

// CASUpload describes one temporary direct-upload destination. Callers only
// execute the returned HTTP PUT; backend-specific signing stays here.
type CASUpload struct {
	PutURL    string
	Headers   map[string]string
	ExpiresAt time.Time
}

// CASUploadAuthorizer is implemented by ingest backends that can let CI upload
// a blob directly without proxying its bytes through relkit-agent.
type CASUploadAuthorizer interface {
	AuthorizeCASUpload(key string, size int64, ttl time.Duration) (*CASUpload, error)
}

// CASProxyReceiver is implemented by an ingest that receives CI bytes through
// relkit-agent rather than through a directly presigned storage URL.
type CASProxyReceiver interface {
	ReceiveCAS(key string, body io.Reader, size int64) error
}

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
	if _, statErr := os.Stat(localPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, false, fmt.Errorf("CAS miss for %s and staged artifact is unavailable: %s", casKey, localPath)
		}
		return nil, false, fmt.Errorf("inspect staged artifact %s: %w", localPath, statErr)
	}
	if _, err := backend.PutArtifact(localPath, casKey); err != nil {
		return nil, false, fmt.Errorf("put cas %s: %w", casKey, err)
	}
	urls, err = ingest.Promote(casKey, artifactKey)
	return urls, false, err
}
