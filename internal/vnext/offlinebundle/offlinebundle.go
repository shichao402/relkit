// Package offlinebundle creates and verifies complete, directory-shaped
// distribution bundles. Every blob is still identified by the digest signed
// in the Distribution Catalog; the directory is only another transport.
package offlinebundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/fsx"
	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/vnext/bootstrap"
	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/resolution"
)

const (
	Schema       = "relkit.distribution-bundle/1"
	ManifestFile = "bundle.json"
	ControlFile  = "resolution-bundle.json"
)

type File struct {
	Digest     string   `json:"digest"`
	Size       int64    `json:"size"`
	Components []string `json:"components,omitempty"`
	Assets     []string `json:"assets,omitempty"`
}

type Manifest struct {
	Schema             string `json:"schema"`
	ResolutionDigest   string `json:"resolutionDigest"`
	DistributionDigest string `json:"distributionDigest"`
	Files              []File `json:"files"`
}

type CreateOptions struct {
	ResolutionBundle resolution.Bundle
	Policy           resolution.BundlePolicy
	Assets           map[string]string
	OutputDirectory  string
}

func Create(options CreateOptions) (*Manifest, error) {
	verified, err := options.ResolutionBundle.Verify(options.Policy)
	if err != nil {
		return nil, err
	}
	distribution, err := selectedDistribution(options.ResolutionBundle.Catalog, verified.SelectedDistribution.Digest)
	if err != nil {
		return nil, err
	}
	required := requiredFiles(distribution)
	for digest := range options.Assets {
		if _, ok := required[digest]; !ok {
			return nil, fmt.Errorf("asset %s is not part of selected distribution %s", digest, distribution.Digest)
		}
	}
	if options.OutputDirectory == "" {
		return nil, fmt.Errorf("output directory is required")
	}
	parent := filepath.Dir(options.OutputDirectory)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(options.OutputDirectory); err == nil {
		return nil, fmt.Errorf("bundle output already exists: %s", options.OutputDirectory)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	temporary, err := os.MkdirTemp(parent, "."+filepath.Base(options.OutputDirectory)+".")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)

	digests := make([]string, 0, len(required))
	for digest := range required {
		digests = append(digests, digest)
	}
	sort.Strings(digests)
	manifest := &Manifest{
		Schema:             Schema,
		ResolutionDigest:   verified.Digest,
		DistributionDigest: distribution.Digest,
	}
	for _, digest := range digests {
		source := options.Assets[digest]
		if source == "" {
			return nil, fmt.Errorf("complete bundle is missing %s used by %s", digest, strings.Join(required[digest].labels(), ", "))
		}
		size, err := copyVerified(source, filepath.Join(temporary, blobPath(digest)), digest)
		if err != nil {
			return nil, err
		}
		entry := required[digest]
		manifest.Files = append(manifest.Files, File{
			Digest: digest, Size: size,
			Components: append([]string(nil), entry.Components...),
			Assets:     append([]string(nil), entry.Assets...),
		})
	}
	if err := document.Write(filepath.Join(temporary, ControlFile), options.ResolutionBundle); err != nil {
		return nil, err
	}
	if err := document.Write(filepath.Join(temporary, ManifestFile), manifest); err != nil {
		return nil, err
	}
	if err := os.Rename(temporary, options.OutputDirectory); err != nil {
		return nil, err
	}
	return manifest, nil
}

func Verify(directory string, policy resolution.BundlePolicy) (*Manifest, *resolution.Bundle, error) {
	var manifest Manifest
	if err := document.Load(filepath.Join(directory, ManifestFile), &manifest); err != nil {
		return nil, nil, err
	}
	if manifest.Schema != Schema || manifest.ResolutionDigest == "" || manifest.DistributionDigest == "" || len(manifest.Files) == 0 {
		return nil, nil, fmt.Errorf("invalid distribution bundle manifest")
	}
	bundle, err := resolution.LoadBundle(filepath.Join(directory, ControlFile))
	if err != nil {
		return nil, nil, err
	}
	verified, err := bundle.Verify(policy)
	if err != nil {
		return nil, nil, err
	}
	if verified.Digest != manifest.ResolutionDigest ||
		verified.SelectedDistribution.Digest != manifest.DistributionDigest {
		return nil, nil, fmt.Errorf("bundle manifest does not match signed resolution")
	}
	distribution, err := selectedDistribution(bundle.Catalog, manifest.DistributionDigest)
	if err != nil {
		return nil, nil, err
	}
	required := requiredFiles(distribution)
	seen := map[string]bool{}
	for _, file := range manifest.Files {
		if seen[file.Digest] {
			return nil, nil, fmt.Errorf("duplicate bundle file %s", file.Digest)
		}
		seen[file.Digest] = true
		if _, ok := required[file.Digest]; !ok {
			return nil, nil, fmt.Errorf("bundle contains unreferenced file %s", file.Digest)
		}
		info, err := os.Stat(filepath.Join(directory, blobPath(file.Digest)))
		if err != nil {
			return nil, nil, err
		}
		if info.Size() != file.Size {
			return nil, nil, fmt.Errorf("bundle file %s size is %d, expected %d", file.Digest, info.Size(), file.Size)
		}
		if got, err := fileDigest(filepath.Join(directory, blobPath(file.Digest))); err != nil || got != file.Digest {
			if err != nil {
				return nil, nil, err
			}
			return nil, nil, fmt.Errorf("bundle file digest is %s, expected %s", got, file.Digest)
		}
	}
	for digest := range required {
		if !seen[digest] {
			return nil, nil, fmt.Errorf("bundle is missing required file %s", digest)
		}
	}
	return &manifest, bundle, nil
}

type InstallOptions struct {
	Directory                string
	ProjectPath              string
	ProjectRoot              string
	RelkitHome               string
	Target                   string
	CandidateRootFingerprint string
}

func Install(options InstallOptions) (*resolution.Document, error) {
	product, err := project.Load(options.ProjectPath)
	if err != nil {
		return nil, err
	}
	manifest, bundle, err := Verify(options.Directory, resolution.BundlePolicy{
		DistributionRootFingerprint: product.TrustFingerprint,
		CandidateRootFingerprint:    options.CandidateRootFingerprint,
	})
	if err != nil {
		return nil, err
	}
	projectDigest, err := product.Digest()
	if err != nil {
		return nil, err
	}
	if bundle.Resolution.ProjectDigest != projectDigest || bundle.Project.OperatorRef != product.OperatorRef {
		return nil, fmt.Errorf("offline bundle belongs to a different project declaration")
	}
	root := options.ProjectRoot
	if root == "" {
		root = filepath.Dir(options.ProjectPath)
	}
	if root, err = filepath.Abs(root); err != nil {
		return nil, err
	}
	home := options.RelkitHome
	if home == "" {
		home, err = bootstrap.Home()
		if err != nil {
			return nil, err
		}
	}
	bootstrapPath := bootstrap.Path(home, product.OperatorRef)
	controlDirectory := filepath.Join(filepath.Dir(bootstrapPath), "control-plane")
	if err := os.MkdirAll(controlDirectory, 0o755); err != nil {
		return nil, err
	}
	operatorPath := filepath.Join(controlDirectory, "operator.json")
	catalogPath := filepath.Join(controlDirectory, "catalog.json")
	if err := document.WriteAtomic(operatorPath, bundle.Operator); err != nil {
		return nil, err
	}
	if err := document.WriteAtomic(catalogPath, bundle.Catalog); err != nil {
		return nil, err
	}
	rootPaths, err := writePublicKeys(controlDirectory, "distribution-root", bundle.DistributionRoots)
	if err != nil {
		return nil, err
	}
	runtimeProfiles := map[string]bootstrap.RuntimeProfile{}
	for productRef, profile := range bundle.RuntimeProfiles {
		profilePath := filepath.Join(controlDirectory, "runtime", productRef+".json")
		if err := document.WriteAtomic(profilePath, profile); err != nil {
			return nil, err
		}
		roots, err := writePublicKeys(filepath.Join(controlDirectory, "runtime"), productRef+"-root", bundle.RuntimeRoots[productRef])
		if err != nil {
			return nil, err
		}
		runtimeProfiles[productRef] = bootstrap.RuntimeProfile{Profile: profilePath, Roots: roots}
	}
	if _, err := bootstrap.Write(home, bootstrap.Document{
		Schema:            bootstrap.Schema,
		OperatorRef:       product.OperatorRef,
		OperatorProfile:   operatorPath,
		Catalog:           catalogPath,
		DistributionRoots: rootPaths,
		RuntimeProfiles:   runtimeProfiles,
		AssetRoots:        []string{options.Directory},
	}); err != nil {
		return nil, err
	}

	cache := filepath.Join(root, ".relkit", "cache", "vnext")
	for _, file := range manifest.Files {
		destination := filepath.Join(cache, strings.TrimPrefix(file.Digest, canonical.DigestPrefix))
		if err := copyBlobAtomic(BlobPath(options.Directory, file.Digest), destination, file.Digest, 0o644); err != nil {
			return nil, err
		}
	}
	distribution, err := selectedDistribution(bundle.Catalog, manifest.DistributionDigest)
	if err != nil {
		return nil, err
	}
	target := options.Target
	if target == "" {
		return nil, fmt.Errorf("offline install requires an explicit target")
	}
	var cliDigest string
	for _, component := range distribution.Components {
		if component.Name == "cli" && component.Platform == target {
			cliDigest = component.SHA256
			break
		}
	}
	if cliDigest == "" {
		return nil, fmt.Errorf("offline distribution has no CLI for target %s", target)
	}
	name := "relkit"
	if strings.HasPrefix(target, "windows-") {
		name += ".exe"
	}
	if err := copyBlobAtomic(
		BlobPath(options.Directory, cliDigest),
		filepath.Join(root, ".relkit", "bootstrap", name),
		cliDigest,
		0o755,
	); err != nil {
		return nil, err
	}
	if err := writeImmutableEqual(filepath.Join(root, ".relkit", "resolution-bundle.json"), *bundle); err != nil {
		return nil, err
	}
	if err := writeImmutableEqual(filepath.Join(root, ".relkit", "resolution.json"), bundle.Resolution); err != nil {
		return nil, err
	}
	return &bundle.Resolution, nil
}

func writePublicKeys(directory, prefix string, values []resolution.PublicKey) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("%s public keys are empty", prefix)
	}
	result := make([]string, 0, len(values))
	for index, value := range values {
		keyDocument := model.PublicKeyDocument{
			Schema:    keys.SchemaPublicKey,
			KeyId:     value.KeyID,
			Alg:       "ed25519",
			PublicKey: append([]byte(nil), value.Key...),
		}
		data, err := rupv2.MarshalPublicKey(&keyDocument)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(directory, fmt.Sprintf("%s-%03d.public.pb", prefix, index))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return nil, err
		}
		result = append(result, path)
	}
	return result, nil
}

func copyBlobAtomic(source, destination, expected string, mode os.FileMode) error {
	if got, err := fileDigest(destination); err == nil && got == expected {
		return nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	got := canonical.DigestPrefix + fmt.Sprintf("%x", sum[:])
	if got != expected {
		return fmt.Errorf("%s digest is %s, expected %s", source, got, expected)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), "."+filepath.Base(destination)+".")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return fsx.Replace(temporaryPath, destination)
}

func writeImmutableEqual(path string, value any) error {
	expected, err := canonical.Bytes(value)
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil {
		var decoded any
		if err := json.Unmarshal(existing, &decoded); err != nil {
			return fmt.Errorf("%s exists but is not valid JSON", path)
		}
		actual, err := canonical.Bytes(decoded)
		if err != nil {
			return err
		}
		if bytes.Equal(expected, actual) {
			return nil
		}
		return fmt.Errorf("%s already exists with different immutable content", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	return document.WriteNew(path, value)
}

type references struct {
	Components []string
	Assets     []string
}

func (r *references) labels() []string {
	result := append([]string(nil), r.Components...)
	result = append(result, r.Assets...)
	sort.Strings(result)
	return result
}

func requiredFiles(distribution catalog.Distribution) map[string]*references {
	result := map[string]*references{}
	for _, component := range distribution.Components {
		entry := result[component.SHA256]
		if entry == nil {
			entry = &references{}
			result[component.SHA256] = entry
		}
		entry.Components = append(entry.Components, component.Name+"@"+component.Platform)
	}
	for name, digest := range distribution.ReleaseAssets {
		entry := result[digest]
		if entry == nil {
			entry = &references{}
			result[digest] = entry
		}
		entry.Assets = append(entry.Assets, name)
	}
	for _, entry := range result {
		sort.Strings(entry.Components)
		sort.Strings(entry.Assets)
	}
	return result
}

func selectedDistribution(value catalog.Document, digest string) (catalog.Distribution, error) {
	for _, distribution := range value.Distributions {
		if distribution.Digest == digest {
			return distribution, nil
		}
	}
	return catalog.Distribution{}, fmt.Errorf("selected distribution %s is absent from bundled catalog", digest)
}

func BlobPath(directory, digest string) string {
	return filepath.Join(directory, blobPath(digest))
}

func blobPath(digest string) string {
	return filepath.Join("blobs", strings.TrimPrefix(digest, canonical.DigestPrefix))
}

func copyVerified(source, destination, expected string) (int64, error) {
	input, err := os.Open(source)
	if err != nil {
		return 0, err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return 0, err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, hash), input)
	closeErr := output.Close()
	if copyErr != nil {
		return 0, copyErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	got := canonical.DigestPrefix + fmt.Sprintf("%x", hash.Sum(nil))
	if got != expected {
		return 0, fmt.Errorf("%s digest is %s, expected %s", source, got, expected)
	}
	return size, nil
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return canonical.DigestPrefix + fmt.Sprintf("%x", hash.Sum(nil)), nil
}
