package ops

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/fsx"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/project"
)

const artifactArchiveName = "artifact-set.json"

type ArtifactTransferResult struct {
	Product          string `json:"product"`
	Version          string `json:"version"`
	Channel          string `json:"channel"`
	ResolutionDigest string `json:"resolutionDigest"`
	ArchiveSHA256    string `json:"archiveSha256"`
	ArchiveSize      int64  `json:"archiveSize"`
	URL              string `json:"url"`
	OutputDirectory  string `json:"outputDirectory,omitempty"`
}

type PushArtifactSetOptions struct {
	ProjectPath     string
	ArtifactSetPath string
	URL             string
	Username        string
	Password        string
	HTTPClient      *http.Client
}

type PullArtifactSetOptions struct {
	ProjectPath     string
	URL             string
	ExpectedSHA256  string
	OutputDirectory string
	Username        string
	Password        string
	HTTPClient      *http.Client
}

func PushArtifactSet(ctx context.Context, options PushArtifactSetOptions) (*ArtifactTransferResult, error) {
	productDocument, value, archive, digest, err := packArtifactSet(options.ProjectPath, options.ArtifactSetPath)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(options.URL) == "" {
		return nil, fmt.Errorf("artifact transfer URL is required")
	}
	if options.Username == "" || options.Password == "" {
		return nil, fmt.Errorf("artifact transfer upload credentials are required")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, options.URL, bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/gzip")
	request.SetBasicAuth(options.Username, options.Password)
	client := options.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("upload ArtifactSet: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if (response.StatusCode < 200 || response.StatusCode >= 300) && response.StatusCode != http.StatusConflict {
		return nil, fmt.Errorf("upload ArtifactSet returned HTTP %d", response.StatusCode)
	}
	remote, err := downloadArtifactArchive(ctx, client, options.URL, options.Username, options.Password)
	if err != nil {
		return nil, fmt.Errorf("verify uploaded ArtifactSet: %w", err)
	}
	if got := canonical.DigestPrefix + fmt.Sprintf("%x", sha256.Sum256(remote)); got != digest {
		return nil, fmt.Errorf("uploaded ArtifactSet reads back as %s, expected %s", got, digest)
	}
	return &ArtifactTransferResult{
		Product: productDocument.Product, Version: value.Version, Channel: value.Channel,
		ResolutionDigest: value.ResolutionDigest, ArchiveSHA256: digest,
		ArchiveSize: int64(len(archive)), URL: options.URL,
	}, nil
}

func PullArtifactSet(ctx context.Context, options PullArtifactSetOptions) (*ArtifactTransferResult, error) {
	productDocument, err := project.Load(options.ProjectPath)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(options.URL) == "" || strings.TrimSpace(options.ExpectedSHA256) == "" {
		return nil, fmt.Errorf("artifact transfer URL and expected SHA-256 are required")
	}
	if err := canonical.ValidateDigest(options.ExpectedSHA256); err != nil {
		return nil, fmt.Errorf("expected ArtifactSet archive SHA-256: %w", err)
	}
	client := options.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	archive, err := downloadArtifactArchive(ctx, client, options.URL, options.Username, options.Password)
	if err != nil {
		return nil, err
	}
	digest := canonical.DigestPrefix + fmt.Sprintf("%x", sha256.Sum256(archive))
	if digest != options.ExpectedSHA256 {
		return nil, fmt.Errorf("ArtifactSet archive digest is %s, expected %s", digest, options.ExpectedSHA256)
	}
	value, err := unpackArtifactSet(archive, options.OutputDirectory, *productDocument)
	if err != nil {
		return nil, err
	}
	return &ArtifactTransferResult{
		Product: value.Product, Version: value.Version, Channel: value.Channel,
		ResolutionDigest: value.ResolutionDigest, ArchiveSHA256: digest,
		ArchiveSize: int64(len(archive)), URL: options.URL, OutputDirectory: options.OutputDirectory,
	}, nil
}

func packArtifactSet(projectPath, artifactSetPath string) (*project.Document, *artifactset.Document, []byte, string, error) {
	productDocument, err := project.Load(projectPath)
	if err != nil {
		return nil, nil, nil, "", err
	}
	value, err := artifactset.Load(artifactSetPath)
	if err != nil {
		return nil, nil, nil, "", err
	}
	if value.Product != productDocument.Product {
		return nil, nil, nil, "", fmt.Errorf("ArtifactSet product %q does not match project %q", value.Product, productDocument.Product)
	}
	root := filepath.Dir(artifactSetPath)
	if err := value.VerifyFiles(root, productDocument.SelectorDimensions); err != nil {
		return nil, nil, nil, "", err
	}
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	compressed.Header.ModTime = time.Unix(0, 0)
	compressed.Header.OS = 255
	archive := tar.NewWriter(compressed)
	artifactJSON, err := canonical.Bytes(value)
	if err != nil {
		return nil, nil, nil, "", err
	}
	entries := map[string][]byte{artifactArchiveName: artifactJSON}
	for _, item := range value.Artifacts {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(item.Path)))
		if err != nil {
			return nil, nil, nil, "", err
		}
		entries[item.Path] = data
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := entries[name]
		header := &tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(data)),
			ModTime: time.Unix(0, 0), AccessTime: time.Unix(0, 0), ChangeTime: time.Unix(0, 0),
			Uid: 0, Gid: 0,
		}
		if err := archive.WriteHeader(header); err != nil {
			return nil, nil, nil, "", err
		}
		if _, err := archive.Write(data); err != nil {
			return nil, nil, nil, "", err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, nil, nil, "", err
	}
	if err := compressed.Close(); err != nil {
		return nil, nil, nil, "", err
	}
	raw := buffer.Bytes()
	digest := canonical.DigestPrefix + fmt.Sprintf("%x", sha256.Sum256(raw))
	return productDocument, value, raw, digest, nil
}

func unpackArtifactSet(archive []byte, outputDirectory string, productDocument project.Document) (*artifactset.Document, error) {
	if outputDirectory == "" {
		return nil, fmt.Errorf("ArtifactSet output directory is required")
	}
	if _, err := os.Stat(outputDirectory); err == nil {
		return nil, fmt.Errorf("ArtifactSet output directory already exists: %s", outputDirectory)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	parent := filepath.Dir(outputDirectory)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	temporary, err := os.MkdirTemp(parent, "."+filepath.Base(outputDirectory)+".")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	compressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	seen := map[string]bool{}
	count := 0
	var total int64
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		count++
		total += header.Size
		if count > 10_001 || total > 16<<30 {
			return nil, fmt.Errorf("ArtifactSet archive exceeds safety limits")
		}
		clean := path.Clean(header.Name)
		if clean == "." || clean != header.Name || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, `\`) {
			return nil, fmt.Errorf("ArtifactSet archive contains unsafe path %q", header.Name)
		}
		if header.Typeflag != tar.TypeReg || seen[clean] {
			return nil, fmt.Errorf("ArtifactSet archive contains unsupported or duplicate entry %q", clean)
		}
		seen[clean] = true
		destination := filepath.Join(temporary, filepath.FromSlash(clean))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return nil, err
		}
		file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		_, copyErr := io.CopyN(file, reader, header.Size)
		closeErr := file.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	value, err := artifactset.Load(filepath.Join(temporary, artifactArchiveName))
	if err != nil {
		return nil, err
	}
	if value.Product != productDocument.Product {
		return nil, fmt.Errorf("ArtifactSet product %q does not match project %q", value.Product, productDocument.Product)
	}
	expected := map[string]bool{artifactArchiveName: true}
	for _, item := range value.Artifacts {
		expected[item.Path] = true
	}
	if len(seen) != len(expected) {
		return nil, fmt.Errorf("ArtifactSet archive contains undeclared files")
	}
	for name := range expected {
		if !seen[name] {
			return nil, fmt.Errorf("ArtifactSet archive is missing %q", name)
		}
	}
	if err := value.VerifyFiles(temporary, productDocument.SelectorDimensions); err != nil {
		return nil, err
	}
	if err := fsx.Replace(temporary, outputDirectory); err != nil {
		return nil, err
	}
	return value, nil
}

func downloadArtifactArchive(ctx context.Context, client *http.Client, rawURL, username, password string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if username != "" || password != "" {
		if username == "" || password == "" {
			return nil, fmt.Errorf("artifact transfer read credentials must provide both username and password")
		}
		request.SetBasicAuth(username, password)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil, fmt.Errorf("download ArtifactSet returned HTTP %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, (16<<30)+1))
}
