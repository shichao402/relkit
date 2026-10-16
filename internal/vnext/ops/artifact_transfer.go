package ops

import (
	"archive/tar"
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
const maxArtifactArchiveSize int64 = 16 << 30

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
	temporary, err := os.MkdirTemp("", "relkit-artifact-push-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	archivePath := filepath.Join(temporary, "artifact-set.tar.gz")
	productDocument, value, digest, size, err := packArtifactSet(options.ProjectPath, options.ArtifactSetPath, archivePath)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(options.URL) == "" {
		return nil, fmt.Errorf("artifact transfer URL is required")
	}
	if options.Username == "" || options.Password == "" {
		return nil, fmt.Errorf("artifact transfer upload credentials are required")
	}
	archive, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, options.URL, archive)
	if err != nil {
		return nil, err
	}
	request.ContentLength = size
	request.Header.Set("Content-Type", "application/gzip")
	request.Header.Set("If-None-Match", "*")
	request.Header.Set("X-BKREPO-EXPIRES", "0")
	request.SetBasicAuth(options.Username, options.Password)
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Hour}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("upload ArtifactSet: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if (response.StatusCode < 200 || response.StatusCode >= 300) &&
		response.StatusCode != http.StatusConflict &&
		response.StatusCode != http.StatusPreconditionFailed {
		return nil, fmt.Errorf("upload ArtifactSet returned HTTP %d", response.StatusCode)
	}
	readbackPath := filepath.Join(temporary, "readback.tar.gz")
	remoteDigest, _, err := downloadArtifactArchive(ctx, client, options.URL, options.Username, options.Password, readbackPath)
	if err != nil {
		return nil, fmt.Errorf("verify uploaded ArtifactSet: %w", err)
	}
	if remoteDigest != digest {
		return nil, fmt.Errorf("uploaded ArtifactSet reads back as %s, expected %s", remoteDigest, digest)
	}
	return &ArtifactTransferResult{
		Product: productDocument.Product, Version: value.Version, Channel: value.Channel,
		ResolutionDigest: value.ResolutionDigest, ArchiveSHA256: digest,
		ArchiveSize: size, URL: options.URL,
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
		client = &http.Client{Timeout: 2 * time.Hour}
	}
	temporary, err := os.MkdirTemp("", "relkit-artifact-pull-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	archivePath := filepath.Join(temporary, "artifact-set.tar.gz")
	digest, size, err := downloadArtifactArchive(
		ctx, client, options.URL, options.Username, options.Password, archivePath,
	)
	if err != nil {
		return nil, err
	}
	if digest != options.ExpectedSHA256 {
		return nil, fmt.Errorf("ArtifactSet archive digest is %s, expected %s", digest, options.ExpectedSHA256)
	}
	value, err := unpackArtifactSet(archivePath, options.OutputDirectory, *productDocument)
	if err != nil {
		return nil, err
	}
	return &ArtifactTransferResult{
		Product: value.Product, Version: value.Version, Channel: value.Channel,
		ResolutionDigest: value.ResolutionDigest, ArchiveSHA256: digest,
		ArchiveSize: size, URL: options.URL, OutputDirectory: options.OutputDirectory,
	}, nil
}

func packArtifactSet(
	projectPath string,
	artifactSetPath string,
	outputPath string,
) (*project.Document, *artifactset.Document, string, int64, error) {
	productDocument, err := project.Load(projectPath)
	if err != nil {
		return nil, nil, "", 0, err
	}
	value, err := artifactset.Load(artifactSetPath)
	if err != nil {
		return nil, nil, "", 0, err
	}
	if value.Product != productDocument.Product {
		return nil, nil, "", 0, fmt.Errorf("ArtifactSet product %q does not match project %q", value.Product, productDocument.Product)
	}
	root := filepath.Dir(artifactSetPath)
	if err := value.VerifyFiles(root, productDocument.SelectorDimensions); err != nil {
		return nil, nil, "", 0, err
	}
	output, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, "", 0, err
	}
	hasher := sha256.New()
	compressed := gzip.NewWriter(io.MultiWriter(output, hasher))
	compressed.Header.ModTime = time.Unix(0, 0)
	compressed.Header.OS = 255
	archive := tar.NewWriter(compressed)
	artifactJSON, err := canonical.Bytes(value)
	if err != nil {
		output.Close()
		return nil, nil, "", 0, err
	}
	entries := map[string]string{}
	expectedDigests := map[string]string{}
	for _, item := range value.Artifacts {
		if item.Path == artifactArchiveName {
			output.Close()
			return nil, nil, "", 0, fmt.Errorf("artifact path %q is reserved", item.Path)
		}
		if _, exists := entries[item.Path]; exists {
			output.Close()
			return nil, nil, "", 0, fmt.Errorf("duplicate archive path %q", item.Path)
		}
		entries[item.Path] = filepath.Join(root, filepath.FromSlash(item.Path))
		expectedDigests[item.Path] = item.SHA256
	}
	names := make([]string, 0, len(entries)+1)
	names = append(names, artifactArchiveName)
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 10_001 {
		output.Close()
		return nil, nil, "", 0, fmt.Errorf("ArtifactSet archive exceeds safety limits")
	}
	var total int64
	for _, name := range names {
		size := int64(len(artifactJSON))
		if name != artifactArchiveName {
			info, statErr := os.Lstat(entries[name])
			if statErr != nil {
				output.Close()
				return nil, nil, "", 0, statErr
			}
			if !info.Mode().IsRegular() {
				output.Close()
				return nil, nil, "", 0, fmt.Errorf("artifact %s is not a regular file", name)
			}
			size = info.Size()
		}
		if size < 0 || size > maxArtifactArchiveSize || total > maxArtifactArchiveSize-size {
			output.Close()
			return nil, nil, "", 0, fmt.Errorf("ArtifactSet archive exceeds safety limits")
		}
		total += size
		header := &tar.Header{
			Name: name, Mode: 0o644, Size: size,
			ModTime: time.Unix(0, 0), AccessTime: time.Unix(0, 0), ChangeTime: time.Unix(0, 0),
			Uid: 0, Gid: 0,
		}
		if err := archive.WriteHeader(header); err != nil {
			output.Close()
			return nil, nil, "", 0, err
		}
		if name == artifactArchiveName {
			if _, err := archive.Write(artifactJSON); err != nil {
				output.Close()
				return nil, nil, "", 0, err
			}
			continue
		}
		source, err := os.Open(entries[name])
		if err != nil {
			output.Close()
			return nil, nil, "", 0, err
		}
		artifactHasher := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(archive, artifactHasher), source)
		closeErr := source.Close()
		if copyErr != nil {
			output.Close()
			return nil, nil, "", 0, copyErr
		}
		if closeErr != nil {
			output.Close()
			return nil, nil, "", 0, closeErr
		}
		if written != size {
			output.Close()
			return nil, nil, "", 0, fmt.Errorf("artifact %s changed while packing", name)
		}
		actualDigest := canonical.DigestPrefix + fmt.Sprintf("%x", artifactHasher.Sum(nil))
		if actualDigest != expectedDigests[name] {
			output.Close()
			return nil, nil, "", 0, fmt.Errorf(
				"artifact %s changed while packing: digest is %s, expected %s",
				name, actualDigest, expectedDigests[name],
			)
		}
	}
	if err := archive.Close(); err != nil {
		output.Close()
		return nil, nil, "", 0, err
	}
	if err := compressed.Close(); err != nil {
		output.Close()
		return nil, nil, "", 0, err
	}
	if err := output.Close(); err != nil {
		return nil, nil, "", 0, err
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return nil, nil, "", 0, err
	}
	if info.Size() > maxArtifactArchiveSize {
		return nil, nil, "", 0, fmt.Errorf("ArtifactSet archive exceeds %d bytes", maxArtifactArchiveSize)
	}
	digest := canonical.DigestPrefix + fmt.Sprintf("%x", hasher.Sum(nil))
	return productDocument, value, digest, info.Size(), nil
}

func unpackArtifactSet(archivePath string, outputDirectory string, productDocument project.Document) (*artifactset.Document, error) {
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
	input, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	compressed, err := gzip.NewReader(input)
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
		if header.Size < 0 || header.Size > maxArtifactArchiveSize || total > maxArtifactArchiveSize-header.Size {
			return nil, fmt.Errorf("ArtifactSet archive exceeds safety limits")
		}
		total += header.Size
		if count > 10_001 {
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

func downloadArtifactArchive(
	ctx context.Context,
	client *http.Client,
	rawURL string,
	username string,
	password string,
	destination string,
) (string, int64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, err
	}
	if username != "" || password != "" {
		if username == "" || password == "" {
			return "", 0, fmt.Errorf("artifact transfer read credentials must provide both username and password")
		}
		request.SetBasicAuth(username, password)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return "", 0, fmt.Errorf("download ArtifactSet returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxArtifactArchiveSize {
		return "", 0, fmt.Errorf("ArtifactSet archive exceeds %d bytes", maxArtifactArchiveSize)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", 0, err
	}
	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, hasher), io.LimitReader(response.Body, maxArtifactArchiveSize+1))
	closeErr := output.Close()
	if copyErr != nil {
		return "", 0, copyErr
	}
	if closeErr != nil {
		return "", 0, closeErr
	}
	if size > maxArtifactArchiveSize {
		return "", 0, fmt.Errorf("ArtifactSet archive exceeds %d bytes", maxArtifactArchiveSize)
	}
	return canonical.DigestPrefix + fmt.Sprintf("%x", hasher.Sum(nil)), size, nil
}
