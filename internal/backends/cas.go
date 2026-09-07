package backends

import (
	"fmt"
	"io"
	"os"
	"time"

	"cnb.cool/shichao402/relkit/internal/model"
)

// CASSign is optional SigV4 material so CI can PUT an unsigned object URL
// with header authentication (STS). Query-presigned URLs leave this nil.
type CASSign struct {
	Algorithm    string `json:"algorithm"`
	Region       string `json:"region"`
	AccessKey    string `json:"accessKey"`
	SecretKey    string `json:"secretKey"`
	SessionToken string `json:"sessionToken,omitempty"`
	PayloadHash  string `json:"payloadHash"`
}

// CASUpload describes one temporary direct-upload destination. Callers only
// execute the returned HTTP PUT; backend-specific signing stays here.
type CASUpload struct {
	PutURL    string
	Headers   map[string]string
	Sign      *CASSign
	ExpiresAt time.Time
}

// CASUploadRequest contains the transport-neutral inputs an ingest backend
// needs to describe how CI should upload one blob. Product and Authorization
// are used by backends whose upload endpoint is hosted by relkit-agent.
type CASUploadRequest struct {
	Product       string
	Key           string
	Size          int64
	TTL           time.Duration
	Authorization string
}

// CASUploadAuthorizer is implemented by ingest backends that can describe how
// CI should upload a blob. The backend type owns the resulting URL and headers;
// callers always execute the returned HTTP PUT without switching on Type().
type CASUploadAuthorizer interface {
	AuthorizeCASUpload(req CASUploadRequest) (*CASUpload, error)
}

// CASUploadReceiver is implemented by a backend whose upload URL is served by
// relkit-agent instead of by an external object store.
type CASUploadReceiver interface {
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

// PrimaryIngest returns the first backend that implements Ingest. CI credentials
// and cas/ live only there; remaining backends receive copies via Materialize.
func PrimaryIngest(all []Backend) (Backend, Ingest, bool) {
	for _, backend := range all {
		if ingest, ok := backend.(Ingest); ok {
			return backend, ingest, true
		}
	}
	return nil, nil, false
}

// Materialize copies one artifact onto dest. Bytes come from a local staged
// file when present, otherwise from source.Get(cas/{sha256}).
func Materialize(source Backend, dest Backend, artifactKey, sha256 string, size int64, localPath string) ([]string, error) {
	path, cleanup, err := materializeSourceFile(source, sha256, size, localPath)
	if err != nil {
		return nil, fmt.Errorf("materialize %s onto %s: %w", artifactKey, dest.Name(), err)
	}
	defer cleanup()
	return dest.PutArtifact(path, artifactKey)
}

func materializeSourceFile(source Backend, sha256 string, size int64, localPath string) (string, func(), error) {
	if localPath != "" {
		info, err := os.Stat(localPath)
		if err == nil && !info.IsDir() {
			if info.Size() != size {
				return "", nil, fmt.Errorf("local %s is %d bytes, want %d", localPath, info.Size(), size)
			}
			return localPath, func() {}, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", nil, err
		}
	}
	casKey, err := model.CasKey(sha256)
	if err != nil {
		return "", nil, err
	}
	data, err := source.Get(casKey)
	if err != nil {
		return "", nil, err
	}
	if len(data) == 0 {
		return "", nil, fmt.Errorf("cas %s missing on %s", casKey, source.Name())
	}
	if int64(len(data)) != size {
		return "", nil, fmt.Errorf("cas %s is %d bytes, want %d", casKey, len(data), size)
	}
	tmp, err := os.CreateTemp("", "relkit-materialize-*")
	if err != nil {
		return "", nil, err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return tmpPath, cleanup, nil
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
