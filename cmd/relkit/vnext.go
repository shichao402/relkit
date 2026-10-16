package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/jsonio"
	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/payload"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/bootstrap"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/diag"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/materialize"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/producer"
	"github.com/shichao402/relkit/internal/vnext/project"
	vnextrelease "github.com/shichao402/relkit/internal/vnext/release"
	"github.com/shichao402/relkit/internal/vnext/resolution"
	"github.com/shichao402/relkit/internal/vnext/runtimebootstrap"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/trust"
	"github.com/shichao402/relkit/internal/vnext/wrapper"
)

const (
	defaultProjectPath    = "relkit.project.json"
	defaultResolutionPath = ".relkit/resolution.json"
)

func selectedResolutionPath() string {
	if value := strings.TrimSpace(os.Getenv("RELKIT_RESOLUTION")); value != "" {
		return value
	}
	return defaultResolutionPath
}

type resolveOptions struct {
	projectPath              string
	operatorPath             string
	catalogPath              string
	trustKeyPaths            []string
	candidateTrustKeyPaths   []string
	candidateRootFingerprint string
	outputPath               string
	bundlePath               string
	watermarksPath           string
	replace                  bool
	candidateDigest          string
	runtimeProfilePaths      map[string]string
	runtimeRootPaths         map[string][]string
	minProfileSequence       int64
	minCatalogSequence       int64
}

func cmdResolve(args []string) error {
	options := resolveOptions{
		projectPath:         defaultProjectPath,
		outputPath:          selectedResolutionPath(),
		runtimeProfilePaths: map[string]string{},
		runtimeRootPaths:    map[string][]string{},
	}
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--project":
			i++
			options.projectPath = mustValue(args, i, "--project")
		case arg == "--operator":
			i++
			options.operatorPath = mustValue(args, i, "--operator")
		case arg == "--catalog":
			i++
			options.catalogPath = mustValue(args, i, "--catalog")
		case arg == "--trust-key":
			i++
			options.trustKeyPaths = append(options.trustKeyPaths, mustValue(args, i, "--trust-key"))
		case arg == "--candidate-trust-key":
			i++
			options.candidateTrustKeyPaths = append(options.candidateTrustKeyPaths, mustValue(args, i, "--candidate-trust-key"))
		case arg == "--candidate-root-fingerprint":
			i++
			options.candidateRootFingerprint = mustValue(args, i, "--candidate-root-fingerprint")
		case arg == "--out":
			i++
			options.outputPath = mustValue(args, i, "--out")
		case arg == "--bundle-out":
			i++
			options.bundlePath = mustValue(args, i, "--bundle-out")
		case arg == "--watermarks":
			i++
			options.watermarksPath = mustValue(args, i, "--watermarks")
		case arg == "--replace":
			options.replace = true
		case arg == "--candidate":
			i++
			options.candidateDigest = mustValue(args, i, "--candidate")
		case arg == "--runtime-profile":
			i++
			product, path, ok := strings.Cut(mustValue(args, i, "--runtime-profile"), "=")
			if !ok || product == "" || path == "" {
				return fmt.Errorf("--runtime-profile expects product=path")
			}
			options.runtimeProfilePaths[product] = path
		case arg == "--runtime-profile-root":
			i++
			product, path, ok := strings.Cut(mustValue(args, i, "--runtime-profile-root"), "=")
			if !ok || product == "" || path == "" {
				return fmt.Errorf("--runtime-profile-root expects product=public-key-path")
			}
			options.runtimeRootPaths[product] = append(options.runtimeRootPaths[product], path)
		case arg == "--min-profile-sequence":
			i++
			value, err := strconv.ParseInt(mustValue(args, i, "--min-profile-sequence"), 10, 64)
			if err != nil {
				return fmt.Errorf("--min-profile-sequence: %w", err)
			}
			options.minProfileSequence = value
		case arg == "--min-catalog-sequence":
			i++
			value, err := strconv.ParseInt(mustValue(args, i, "--min-catalog-sequence"), 10, 64)
			if err != nil {
				return fmt.Errorf("--min-catalog-sequence: %w", err)
			}
			options.minCatalogSequence = value
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	options.applyEnvironment()
	if options.watermarksPath == "" {
		options.watermarksPath = filepath.Join(filepath.Dir(options.outputPath), "control-plane-watermarks.json")
	}
	if options.bundlePath == "" {
		options.bundlePath = filepath.Join(filepath.Dir(options.outputPath), "resolution-bundle.json")
	}
	productDocument, err := project.Load(options.projectPath)
	if err != nil {
		return err
	}
	options.applyOperatorEnvironment(productDocument.OperatorRef)
	if err := options.applyBootstrap(*productDocument); err != nil {
		return err
	}
	options.applyRuntimeProfileEnvironment(productDocument.RuntimeProfileSubscriptions)
	if options.operatorPath == "" {
		return fmt.Errorf("operator %q was not injected; set --operator, RELKIT_OPERATOR_PROFILE, or %s", productDocument.OperatorRef, operatorProfileEnvironment(productDocument.OperatorRef))
	}
	if options.catalogPath == "" {
		return fmt.Errorf("--catalog or RELKIT_DISTRIBUTION_CATALOG is required")
	}
	if len(options.trustKeyPaths) == 0 {
		return fmt.Errorf("--trust-key or RELKIT_DISTRIBUTION_ROOT is required")
	}
	if options.candidateDigest != "" {
		if len(options.candidateTrustKeyPaths) == 0 {
			return fmt.Errorf("--candidate-trust-key or RELKIT_CANDIDATE_ROOT is required for candidate resolution")
		}
		if options.candidateRootFingerprint == "" {
			return fmt.Errorf("--candidate-root-fingerprint or RELKIT_CANDIDATE_ROOT_FINGERPRINT is required for candidate resolution")
		}
	}
	if expected := os.Getenv("RELKIT_EXPECTED_ROOT_FINGERPRINT"); expected != "" &&
		productDocument.TrustFingerprint != expected {
		return fmt.Errorf("project trustFingerprint %s does not match wrapper-pinned %s", productDocument.TrustFingerprint, expected)
	}
	projectDigest, err := productDocument.Digest()
	if err != nil {
		return err
	}
	if !options.replace {
		if existing, loadErr := resolution.Load(options.outputPath); loadErr == nil {
			if existing.ProjectDigest != projectDigest {
				return fmt.Errorf("%s belongs to a different project document; pass --replace to begin a new resolution", options.outputPath)
			}
			if options.candidateDigest == "" && existing.Identity != resolution.IdentityProduction {
				return fmt.Errorf("%s is a candidate resolution; pass --replace to begin a production resolution", options.outputPath)
			}
			if options.candidateDigest != "" &&
				(existing.Identity != resolution.IdentityCandidate || existing.SelectedDistribution.Digest != options.candidateDigest) {
				return fmt.Errorf("%s does not select requested candidate %s; pass --replace to begin a new resolution", options.outputPath, options.candidateDigest)
			}
			bundle, bundleErr := resolution.LoadBundle(options.bundlePath)
			if bundleErr != nil {
				return fmt.Errorf("immutable resolution bundle is unavailable: %w", bundleErr)
			}
			if bundle.Resolution.Digest != existing.Digest {
				return fmt.Errorf("immutable resolution bundle selects %s, resolution selects %s", bundle.Resolution.Digest, existing.Digest)
			}
			fmt.Printf("reusing immutable resolution %s (%s)\n", existing.Digest, existing.SelectedDistribution.Version)
			return nil
		} else if !os.IsNotExist(loadErr) {
			return fmt.Errorf("%s exists but is not a valid immutable resolution: %w", options.outputPath, loadErr)
		}
	}
	operatorDocument, err := operator.Load(options.operatorPath)
	if err != nil {
		return err
	}
	catalogDocument, err := catalog.Load(options.catalogPath)
	if err != nil {
		return err
	}
	trustedKeys, err := loadDistributionKeys(options.trustKeyPaths)
	if err != nil {
		return err
	}
	candidateTrustedKeys := map[string]ed25519.PublicKey{}
	if len(options.candidateTrustKeyPaths) > 0 {
		candidateTrustedKeys, err = loadDistributionKeys(options.candidateTrustKeyPaths)
		if err != nil {
			return err
		}
	}
	runtimeProfiles, trustedRuntimeRoots, err := loadRuntimeProfileInputs(*productDocument, options.runtimeProfilePaths, options.runtimeRootPaths)
	if err != nil {
		return err
	}
	identity := resolution.IdentityProduction
	if options.candidateDigest != "" {
		identity = resolution.IdentityCandidate
	}
	value, err := resolution.Resolve(resolution.Request{
		Project:                  *productDocument,
		Operator:                 *operatorDocument,
		Catalog:                  *catalogDocument,
		TrustedDistributionKeys:  trustedKeys,
		TrustedCandidateKeys:     candidateTrustedKeys,
		CandidateRootFingerprint: options.candidateRootFingerprint,
		RuntimeProfiles:          runtimeProfiles,
		TrustedRuntimeRoots:      trustedRuntimeRoots,
		Identity:                 identity,
		CandidateDigest:          options.candidateDigest,
		MinimumProfileSequence:   options.minProfileSequence,
		MinimumCatalogSequence:   options.minCatalogSequence,
	})
	if err != nil {
		return err
	}
	watermarks, err := resolution.LoadWatermarks(options.watermarksPath)
	if err != nil {
		return err
	}
	if err := watermarks.AdvanceOperator(value.Operator.DeploymentID, value.Operator.Sequence, value.Operator.ProfileDigest); err != nil {
		return err
	}
	if err := watermarks.AdvanceCatalog(productDocument.TrustFingerprint, catalogDocument.Sequence, value.CatalogSnapshotDigest); err != nil {
		return err
	}
	if err := watermarks.Write(options.watermarksPath); err != nil {
		return err
	}
	for product, runtimeProfile := range runtimeProfiles {
		cachePath := runtimeProfileCachePath(options.outputPath, product)
		if err := document.WriteAtomic(cachePath, runtimeProfile); err != nil {
			return err
		}
		if err := cacheRuntimeProfileRoots(options.outputPath, product, trustedRuntimeRoots[product]); err != nil {
			return err
		}
		bootstrapDocument, err := runtimebootstrap.New(
			value.Digest,
			runtimeProfileSubscriptionFingerprint(*productDocument, product),
			runtimeProfile,
			trustedRuntimeRoots[product],
		)
		if err != nil {
			return err
		}
		if err := document.WriteAtomic(runtimeProfileBootstrapPath(options.outputPath, product), bootstrapDocument); err != nil {
			return err
		}
	}
	distributionBundleRoots := map[string]ed25519.PublicKey{}
	for keyID, publicKey := range trustedKeys {
		fingerprint, fingerprintErr := trust.Fingerprint(publicKey)
		if fingerprintErr != nil {
			return fingerprintErr
		}
		if fingerprint == productDocument.TrustFingerprint {
			distributionBundleRoots[keyID] = publicKey
		}
	}
	candidateBundleRoots := map[string]ed25519.PublicKey{}
	for keyID, publicKey := range candidateTrustedKeys {
		fingerprint, fingerprintErr := trust.Fingerprint(publicKey)
		if fingerprintErr != nil {
			return fingerprintErr
		}
		if fingerprint == options.candidateRootFingerprint {
			candidateBundleRoots[keyID] = publicKey
		}
	}
	runtimeBundleRoots := make(map[string][]resolution.PublicKey, len(trustedRuntimeRoots))
	for product, roots := range trustedRuntimeRoots {
		runtimeBundleRoots[product] = resolution.PublicKeys(roots)
	}
	bundle := resolution.Bundle{
		Schema:            resolution.BundleSchema,
		Project:           *productDocument,
		Operator:          *operatorDocument,
		Catalog:           *catalogDocument,
		RuntimeProfiles:   runtimeProfiles,
		DistributionRoots: resolution.PublicKeys(distributionBundleRoots),
		CandidateRoots:    resolution.PublicKeys(candidateBundleRoots),
		RuntimeRoots:      runtimeBundleRoots,
		Resolution:        *value,
	}
	if _, err := bundle.Verify(resolution.BundlePolicy{
		DistributionRootFingerprint: productDocument.TrustFingerprint,
		CandidateRootFingerprint:    options.candidateRootFingerprint,
		OperatorDeploymentID:        operatorDocument.DeploymentID,
		Identity:                    value.Identity,
	}); err != nil {
		return err
	}
	if options.replace {
		err = document.WriteAtomic(options.bundlePath, bundle)
	} else {
		err = document.WriteNew(options.bundlePath, bundle)
	}
	if err != nil {
		return err
	}
	if options.replace {
		err = document.WriteAtomic(options.outputPath, value)
	} else {
		err = document.WriteNew(options.outputPath, value)
	}
	if err != nil {
		return err
	}
	fmt.Printf("resolved %s to %s (%s)\n", productDocument.Product, value.SelectedDistribution.Version, value.Digest)
	fmt.Println("wrote " + options.outputPath)
	return nil
}

func runtimeProfileCachePath(resolutionPath, product string) string {
	return filepath.Join(filepath.Dir(resolutionPath), "control-plane", "runtime-profiles", product+".json")
}

func runtimeProfileRootCacheDir(resolutionPath, product string) string {
	return filepath.Join(filepath.Dir(resolutionPath), "control-plane", "runtime-profiles", product+".roots")
}

func runtimeProfileBootstrapPath(resolutionPath, product string) string {
	return filepath.Join(filepath.Dir(resolutionPath), "control-plane", "runtime-profiles", product+".bootstrap.json")
}

func runtimeProfileSubscriptionFingerprint(productDocument project.Document, product string) string {
	for _, subscription := range productDocument.RuntimeProfileSubscriptions {
		if subscription.ProductRef == product {
			return subscription.RootFingerprint
		}
	}
	return ""
}

func cacheRuntimeProfileRoots(resolutionPath, product string, roots map[string]ed25519.PublicKey) error {
	directory := runtimeProfileRootCacheDir(resolutionPath, product)
	if err := os.RemoveAll(directory); err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	keyIDs := make([]string, 0, len(roots))
	for keyID := range roots {
		keyIDs = append(keyIDs, keyID)
	}
	sort.Strings(keyIDs)
	for index, keyID := range keyIDs {
		value := model.PublicKeyDocument{
			Schema:    keys.SchemaPublicKey,
			KeyId:     keyID,
			Alg:       "ed25519",
			PublicKey: append([]byte(nil), roots[keyID]...),
		}
		data, err := rupv2.MarshalPublicKey(&value)
		if err != nil {
			return err
		}
		path := filepath.Join(directory, fmt.Sprintf("%03d.public.pb", index))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (o *resolveOptions) applyEnvironment() {
	if o.operatorPath == "" {
		o.operatorPath = os.Getenv("RELKIT_OPERATOR_PROFILE")
	}
	if o.catalogPath == "" {
		o.catalogPath = os.Getenv("RELKIT_DISTRIBUTION_CATALOG")
	}
	if len(o.trustKeyPaths) == 0 {
		if value := os.Getenv("RELKIT_DISTRIBUTION_ROOT"); value != "" {
			o.trustKeyPaths = filepath.SplitList(value)
		}
	}
	if len(o.candidateTrustKeyPaths) == 0 {
		if value := os.Getenv("RELKIT_CANDIDATE_ROOT"); value != "" {
			o.candidateTrustKeyPaths = filepath.SplitList(value)
		}
	}
	if o.candidateRootFingerprint == "" {
		o.candidateRootFingerprint = os.Getenv("RELKIT_CANDIDATE_ROOT_FINGERPRINT")
	}
}

func (o *resolveOptions) applyOperatorEnvironment(operatorRef string) {
	if o.operatorPath == "" {
		o.operatorPath = os.Getenv(operatorProfileEnvironment(operatorRef))
	}
}

func (o *resolveOptions) applyBootstrap(productDocument project.Document) error {
	path := os.Getenv("RELKIT_OPERATOR_BOOTSTRAP")
	if path == "" {
		var err error
		path, err = bootstrap.DefaultPath(productDocument.OperatorRef)
		if err != nil {
			return err
		}
	}
	value, err := bootstrap.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if value.OperatorRef != productDocument.OperatorRef {
		return fmt.Errorf("Operator bootstrap %s is for %q, project requests %q", path, value.OperatorRef, productDocument.OperatorRef)
	}
	if o.operatorPath == "" {
		o.operatorPath = value.OperatorProfile
	}
	if o.catalogPath == "" {
		o.catalogPath = value.Catalog
	}
	if len(o.trustKeyPaths) == 0 {
		o.trustKeyPaths = append([]string(nil), value.DistributionRoots...)
	}
	for product, profile := range value.RuntimeProfiles {
		if o.runtimeProfilePaths[product] == "" {
			o.runtimeProfilePaths[product] = profile.Profile
		}
		if len(o.runtimeRootPaths[product]) == 0 {
			o.runtimeRootPaths[product] = append([]string(nil), profile.Roots...)
		}
	}
	return nil
}

func (o *resolveOptions) applyRuntimeProfileEnvironment(subscriptions []project.Subscription) {
	for _, subscription := range subscriptions {
		suffix := environmentSuffix(subscription.ProductRef)
		if o.runtimeProfilePaths[subscription.ProductRef] == "" {
			o.runtimeProfilePaths[subscription.ProductRef] = os.Getenv("RELKIT_RUNTIME_PROFILE_" + suffix)
		}
		if len(o.runtimeRootPaths[subscription.ProductRef]) == 0 {
			if value := os.Getenv("RELKIT_RUNTIME_PROFILE_ROOT_" + suffix); value != "" {
				o.runtimeRootPaths[subscription.ProductRef] = filepath.SplitList(value)
			}
		}
	}
}

func operatorProfileEnvironment(operatorRef string) string {
	return "RELKIT_OPERATOR_PROFILE_" + environmentSuffix(operatorRef)
}

func environmentSuffix(value string) string {
	var name strings.Builder
	for _, character := range strings.ToUpper(value) {
		if (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') {
			name.WriteRune(character)
		} else {
			name.WriteByte('_')
		}
	}
	return name.String()
}

func loadDistributionKeys(paths []string) (map[string]ed25519.PublicKey, error) {
	result := make(map[string]ed25519.PublicKey, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		keyDocument, err := rupv2.UnmarshalPublicKey(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		publicKey, err := keys.LoadPublicKey(*keyDocument, path)
		if err != nil {
			return nil, err
		}
		if _, exists := result[keyDocument.KeyId]; exists {
			return nil, fmt.Errorf("duplicate trusted key id %q", keyDocument.KeyId)
		}
		result[keyDocument.KeyId] = publicKey
	}
	return result, nil
}

func loadRuntimeProfileInputs(
	productDocument project.Document,
	profilePaths map[string]string,
	rootPaths map[string][]string,
) (map[string]runtimeprofile.Document, map[string]map[string]ed25519.PublicKey, error) {
	profiles := make(map[string]runtimeprofile.Document, len(productDocument.RuntimeProfileSubscriptions))
	trustedRoots := make(map[string]map[string]ed25519.PublicKey, len(productDocument.RuntimeProfileSubscriptions))
	declared := map[string]project.Subscription{}
	for _, subscription := range productDocument.RuntimeProfileSubscriptions {
		declared[subscription.ProductRef] = subscription
		path := profilePaths[subscription.ProductRef]
		if path == "" {
			return nil, nil, fmt.Errorf("runtime profile %q is required; inject --runtime-profile %s=path or RELKIT_RUNTIME_PROFILE_%s", subscription.ProductRef, subscription.ProductRef, environmentSuffix(subscription.ProductRef))
		}
		value, err := runtimeprofile.Load(path)
		if err != nil {
			return nil, nil, err
		}
		roots, err := loadDistributionKeys(rootPaths[subscription.ProductRef])
		if err != nil {
			return nil, nil, fmt.Errorf("runtime profile %q roots: %w", subscription.ProductRef, err)
		}
		matched := map[string]ed25519.PublicKey{}
		for keyID, publicKey := range roots {
			fingerprint, err := trust.Fingerprint(publicKey)
			if err != nil {
				return nil, nil, err
			}
			if fingerprint == subscription.RootFingerprint {
				matched[keyID] = publicKey
			}
		}
		if len(matched) == 0 {
			return nil, nil, fmt.Errorf("runtime profile %q root fingerprint %s did not match any injected key", subscription.ProductRef, subscription.RootFingerprint)
		}
		profiles[subscription.ProductRef] = *value
		trustedRoots[subscription.ProductRef] = matched
	}
	for product := range profilePaths {
		if product != "" && profilePaths[product] != "" {
			if _, ok := declared[product]; !ok {
				return nil, nil, fmt.Errorf("runtime profile %q was injected but is not declared by the project", product)
			}
		}
	}
	return profiles, trustedRoots, nil
}

func cmdDoctor(args []string) error {
	resolutionPath := selectedResolutionPath()
	bundlePath := ""
	projectPath := ""
	operatorPath := ""
	candidateRootFingerprint := ""
	runtimeProfilePaths := map[string]string{}
	runtimeRootPaths := map[string][]string{}
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--resolution":
			i++
			resolutionPath = mustValue(args, i, "--resolution")
		case arg == "--resolution-bundle":
			i++
			bundlePath = mustValue(args, i, "--resolution-bundle")
		case arg == "--project":
			i++
			projectPath = mustValue(args, i, "--project")
		case arg == "--operator":
			i++
			operatorPath = mustValue(args, i, "--operator")
		case arg == "--candidate-root-fingerprint":
			i++
			candidateRootFingerprint = mustValue(args, i, "--candidate-root-fingerprint")
		case arg == "--runtime-profile":
			i++
			product, path, ok := strings.Cut(mustValue(args, i, "--runtime-profile"), "=")
			if !ok || product == "" || path == "" {
				return fmt.Errorf("--runtime-profile expects product=path")
			}
			runtimeProfilePaths[product] = path
		case arg == "--runtime-profile-root":
			i++
			product, path, ok := strings.Cut(mustValue(args, i, "--runtime-profile-root"), "=")
			if !ok || product == "" || path == "" {
				return fmt.Errorf("--runtime-profile-root expects product=public-key-path")
			}
			runtimeRootPaths[product] = append(runtimeRootPaths[product], path)
		case arg == "--json":
			asJSON = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	value, err := resolution.Load(resolutionPath)
	if err != nil {
		return diag.Error{
			Diagnostic: diag.Envelope{
				Code:     "resolution.invalid",
				Severity: diag.SeverityError,
				Stage:    "doctor",
				Message:  "resolution could not be verified",
				Evidence: map[string]any{"path": resolutionPath},
			},
			Cause: err,
		}
	}
	var productDocument *project.Document
	if projectPath != "" {
		productDocument, err = project.Load(projectPath)
		if err != nil {
			return doctorDiagnostic("project.invalid", "project document could not be verified", map[string]any{"path": projectPath}, err)
		}
		projectDigest, digestErr := productDocument.Digest()
		if digestErr != nil {
			return digestErr
		}
		if projectDigest != value.ProjectDigest {
			return doctorDiagnostic(
				"resolution.project_changed",
				"project document changed after resolution",
				map[string]any{"path": projectPath, "resolvedDigest": value.ProjectDigest, "currentDigest": projectDigest},
				nil,
			)
		}
		for _, subscription := range productDocument.RuntimeProfileSubscriptions {
			suffix := environmentSuffix(subscription.ProductRef)
			if runtimeProfilePaths[subscription.ProductRef] == "" {
				runtimeProfilePaths[subscription.ProductRef] = os.Getenv("RELKIT_RUNTIME_PROFILE_" + suffix)
			}
			if len(runtimeRootPaths[subscription.ProductRef]) == 0 {
				if paths := os.Getenv("RELKIT_RUNTIME_PROFILE_ROOT_" + suffix); paths != "" {
					runtimeRootPaths[subscription.ProductRef] = filepath.SplitList(paths)
				}
			}
		}
	}
	if bundlePath == "" {
		bundlePath = filepath.Join(filepath.Dir(resolutionPath), "resolution-bundle.json")
	}
	if candidateRootFingerprint == "" {
		candidateRootFingerprint = os.Getenv("RELKIT_CANDIDATE_ROOT_FINGERPRINT")
	}
	bundle, loadBundleErr := resolution.LoadBundle(bundlePath)
	if loadBundleErr != nil {
		return doctorDiagnostic("resolution.bundle_missing", "signed resolution inputs are unavailable", map[string]any{"path": bundlePath}, loadBundleErr)
	}
	expectedRoot := bundle.Project.TrustFingerprint
	if productDocument != nil {
		expectedRoot = productDocument.TrustFingerprint
	}
	verified, verifyBundleErr := bundle.Verify(resolution.BundlePolicy{
		DistributionRootFingerprint: expectedRoot,
		CandidateRootFingerprint:    candidateRootFingerprint,
		OperatorDeploymentID:        value.Operator.DeploymentID,
		Identity:                    value.Identity,
	})
	if verifyBundleErr != nil {
		return doctorDiagnostic("resolution.bundle_invalid", "signed resolution inputs could not be verified", map[string]any{"path": bundlePath}, verifyBundleErr)
	}
	if verified.Digest != value.Digest {
		return doctorDiagnostic(
			"resolution.bundle_mismatch",
			"resolution does not match its signed input bundle",
			map[string]any{"resolutionDigest": value.Digest, "bundleDigest": verified.Digest},
			nil,
		)
	}
	if operatorPath == "" {
		operatorPath = os.Getenv("RELKIT_OPERATOR_PROFILE")
	}
	if operatorPath == "" && productDocument != nil {
		operatorPath = os.Getenv(operatorProfileEnvironment(productDocument.OperatorRef))
	}
	if operatorPath != "" {
		operatorDocument, loadErr := operator.Load(operatorPath)
		if loadErr != nil {
			return doctorDiagnostic("operator.invalid", "current Operator Profile could not be read", map[string]any{"path": operatorPath}, loadErr)
		}
		operatorDigest, digestErr := operatorDocument.Digest()
		if digestErr != nil {
			return digestErr
		}
		if operatorDigest != value.Operator.ProfileDigest ||
			operatorDocument.DeploymentID != value.Operator.DeploymentID ||
			operatorDocument.Sequence != value.Operator.Sequence {
			return doctorDiagnostic(
				"resolution.operator_changed",
				"Operator Profile changed after resolution; begin a new resolution instead of mixing control-plane snapshots",
				map[string]any{
					"path":             operatorPath,
					"resolvedDigest":   value.Operator.ProfileDigest,
					"currentDigest":    operatorDigest,
					"resolvedSequence": value.Operator.Sequence,
					"currentSequence":  operatorDocument.Sequence,
				},
				nil,
			)
		}
	}
	for product, expectedDigest := range value.RuntimeProfileDigests {
		profilePath := runtimeProfilePaths[product]
		if profilePath == "" {
			profilePath = runtimeProfileCachePath(resolutionPath, product)
		}
		profile, loadErr := runtimeprofile.Load(profilePath)
		if loadErr != nil {
			return doctorDiagnostic(
				"runtime_profile.missing",
				"resolved Runtime Profile is unavailable",
				map[string]any{"product": product, "path": profilePath},
				loadErr,
			)
		}
		if profile.Digest != expectedDigest {
			return doctorDiagnostic(
				"resolution.runtime_profile_changed",
				"Runtime Profile changed after resolution",
				map[string]any{"product": product, "resolvedDigest": expectedDigest, "currentDigest": profile.Digest},
				nil,
			)
		}
		rootPaths := runtimeRootPaths[product]
		if len(rootPaths) == 0 {
			entries, readErr := os.ReadDir(runtimeProfileRootCacheDir(resolutionPath, product))
			if readErr != nil {
				return doctorDiagnostic("runtime_profile.root_missing", "Runtime Profile root is unavailable", map[string]any{"product": product}, readErr)
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					rootPaths = append(rootPaths, filepath.Join(runtimeProfileRootCacheDir(resolutionPath, product), entry.Name()))
				}
			}
		}
		roots, loadErr := loadDistributionKeys(rootPaths)
		if loadErr != nil {
			return doctorDiagnostic("runtime_profile.root_invalid", "Runtime Profile root could not be read", map[string]any{"product": product}, loadErr)
		}
		if verifyErr := profile.Verify(roots); verifyErr != nil {
			return doctorDiagnostic("runtime_profile.signature_invalid", "Runtime Profile signature is invalid", map[string]any{"product": product}, verifyErr)
		}
	}
	report := map[string]any{
		"ok": true,
		"diagnostics": []diag.Envelope{
			{
				Code:     "resolution.valid",
				Severity: diag.SeverityInfo,
				Stage:    "doctor",
				Message:  "immutable resolution is valid",
				Evidence: map[string]any{
					"path":         resolutionPath,
					"digest":       value.Digest,
					"distribution": value.SelectedDistribution.Version,
					"deploymentId": value.Operator.DeploymentID,
					"identity":     value.Identity,
				},
			},
		},
	}
	if asJSON {
		data, err := jsonio.MarshalPretty(report)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(data)
		return err
	}
	fmt.Printf("ok: resolution %s selects %s for %s (%s)\n",
		value.Digest,
		value.SelectedDistribution.Version,
		value.Operator.DeploymentID,
		value.Identity,
	)
	return nil
}

func doctorDiagnostic(code, message string, evidence map[string]any, cause error) error {
	return diag.Error{
		Diagnostic: diag.Envelope{
			Code:     code,
			Severity: diag.SeverityError,
			Stage:    "doctor",
			Message:  message,
			Evidence: evidence,
		},
		Cause: cause,
	}
}

func cmdMaterialize(args []string) error {
	resolutionPath := selectedResolutionPath()
	projectRoot := "."
	target := "host"
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--resolution":
			i++
			resolutionPath = mustValue(args, i, "--resolution")
		case arg == "--project-root":
			i++
			projectRoot = mustValue(args, i, "--project-root")
		case arg == "--target":
			i++
			target = mustValue(args, i, "--target")
		case arg == "--json":
			asJSON = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	value, err := resolution.Load(resolutionPath)
	if err != nil {
		return err
	}
	result, err := materialize.Run(context.Background(), *value, materialize.Options{
		ProjectRoot: projectRoot,
		Target:      target,
	})
	if err != nil {
		return err
	}
	if asJSON {
		data, err := jsonio.MarshalPretty(result)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(data)
		return err
	}
	fmt.Printf("materialized %d components for %s (%s)\n", len(result.Paths), result.Target, result.ResolutionDigest)
	return nil
}

func cmdPayload(args []string) error {
	if len(args) == 0 || args[0] != "build" {
		return fmt.Errorf("usage: relkit payload build --tree <directory> --out <payload.zip>")
	}
	tree := ""
	output := ""
	for i := 1; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--tree":
			i++
			tree = mustValue(args, i, "--tree")
		case "--out":
			i++
			output = mustValue(args, i, "--out")
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	if tree == "" || output == "" {
		return fmt.Errorf("payload build requires --tree and --out")
	}
	table, err := payload.Build(output, payload.BuildOptions{Tree: tree})
	if err != nil {
		return err
	}
	fmt.Printf("built payload %s (%d files, %d scripts)\n", output, len(table.Files), len(table.Scripts))
	return nil
}

func cmdArtifacts(args []string) error {
	if len(args) > 0 && args[0] == "merge" {
		return cmdArtifactsMerge(args[1:])
	}
	if len(args) == 0 || args[0] != "produce" {
		return fmt.Errorf("usage: relkit artifacts <produce|merge> ...")
	}
	projectPath := defaultProjectPath
	resolutionPath := selectedResolutionPath()
	bundlePath := ""
	version := ""
	channel := ""
	target := ""
	outputDir := ""
	outputPath := ""
	for i := 1; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--project":
			i++
			projectPath = mustValue(args, i, "--project")
		case arg == "--resolution":
			i++
			resolutionPath = mustValue(args, i, "--resolution")
		case arg == "--resolution-bundle":
			i++
			bundlePath = mustValue(args, i, "--resolution-bundle")
		case arg == "--version":
			i++
			version = mustValue(args, i, "--version")
		case arg == "--channel":
			i++
			channel = mustValue(args, i, "--channel")
		case arg == "--target":
			i++
			target = mustValue(args, i, "--target")
		case arg == "--out-dir":
			i++
			outputDir = mustValue(args, i, "--out-dir")
		case arg == "--out":
			i++
			outputPath = mustValue(args, i, "--out")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	if version == "" || channel == "" || target == "" {
		return fmt.Errorf("--version, --channel, and --target are required")
	}
	if outputPath == "" {
		outputPath = filepath.Join(filepath.Dir(resolutionPath), "artifact-sets", version, target, "artifact-set.json")
	}
	if outputDir == "" {
		outputDir = filepath.Dir(outputPath)
	}
	if bundlePath == "" {
		bundlePath = filepath.Join(filepath.Dir(resolutionPath), "resolution-bundle.json")
	}
	productDocument, err := project.Load(projectPath)
	if err != nil {
		return err
	}
	resolved, err := resolution.Load(resolutionPath)
	if err != nil {
		return err
	}
	bundle, err := resolution.LoadBundle(bundlePath)
	if err != nil {
		return err
	}
	verified, err := bundle.Verify(resolution.BundlePolicy{
		DistributionRootFingerprint: productDocument.TrustFingerprint,
		CandidateRootFingerprint:    os.Getenv("RELKIT_CANDIDATE_ROOT_FINGERPRINT"),
		OperatorDeploymentID:        resolved.Operator.DeploymentID,
		Identity:                    resolved.Identity,
	})
	if err != nil {
		return fmt.Errorf("resolution bundle: %w", err)
	}
	if verified.Digest != resolved.Digest {
		return fmt.Errorf("resolution bundle digest %s does not match resolution %s", verified.Digest, resolved.Digest)
	}
	runtimeBootstraps, err := runtimeBootstrapsFromBundle(*productDocument, *resolved, *bundle)
	if err != nil {
		return err
	}
	value, err := producer.Run(context.Background(), *productDocument, *resolved, producer.Options{
		ProjectRoot:       filepath.Dir(projectPath),
		OutputDir:         outputDir,
		Version:           version,
		Channel:           channel,
		Target:            target,
		ProtectedPaths:    []string{projectPath, resolutionPath, bundlePath},
		RuntimeBootstraps: runtimeBootstraps,
	})
	if err != nil {
		return err
	}
	if err := document.WriteNew(outputPath, value); err != nil {
		return err
	}
	fmt.Printf("produced %d artifact(s) for %s/%s using resolution %s\n", len(value.Artifacts), version, target, value.ResolutionDigest)
	fmt.Println("wrote " + outputPath)
	return nil
}

func runtimeBootstrapsFromBundle(
	productDocument project.Document,
	resolved resolution.Document,
	bundle resolution.Bundle,
) (map[string]runtimebootstrap.Document, error) {
	result := make(map[string]runtimebootstrap.Document, len(productDocument.RuntimeProfileSubscriptions))
	for _, subscription := range productDocument.RuntimeProfileSubscriptions {
		profile, ok := bundle.RuntimeProfiles[subscription.ProductRef]
		if !ok {
			return nil, fmt.Errorf("resolution bundle has no runtime profile %q", subscription.ProductRef)
		}
		keys := make(map[string]ed25519.PublicKey, len(bundle.RuntimeRoots[subscription.ProductRef]))
		for _, key := range bundle.RuntimeRoots[subscription.ProductRef] {
			if _, exists := keys[key.KeyID]; exists {
				return nil, fmt.Errorf("runtime profile %q has duplicate trusted key %q", subscription.ProductRef, key.KeyID)
			}
			keys[key.KeyID] = ed25519.PublicKey(append([]byte(nil), key.Key...))
		}
		value, err := runtimebootstrap.New(
			resolved.Digest,
			subscription.RootFingerprint,
			profile,
			keys,
		)
		if err != nil {
			return nil, fmt.Errorf("runtime profile %q bootstrap: %w", subscription.ProductRef, err)
		}
		result[subscription.ProductRef] = *value
	}
	return result, nil
}

func cmdArtifactsMerge(args []string) error {
	var inputs []string
	outputPath := ""
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--in":
			i++
			inputs = append(inputs, mustValue(args, i, "--in"))
		case arg == "--out":
			i++
			outputPath = mustValue(args, i, "--out")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	if len(inputs) < 2 || outputPath == "" {
		return fmt.Errorf("artifacts merge requires at least two --in paths and --out")
	}
	documents := make([]artifactset.Document, 0, len(inputs))
	for _, path := range inputs {
		value, err := artifactset.Load(path)
		if err != nil {
			return err
		}
		documents = append(documents, *value)
	}
	merged, err := artifactset.Merge(documents...)
	if err != nil {
		return err
	}
	outputRoot := filepath.Dir(outputPath)
	if outputRoot == "." {
		return fmt.Errorf("--out must be inside a dedicated, new aggregate directory")
	}
	if _, err := os.Stat(outputRoot); err == nil {
		return fmt.Errorf("merge output directory already exists: %s", outputRoot)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputRoot), 0o755); err != nil {
		return err
	}
	temporaryRoot, err := os.MkdirTemp(filepath.Dir(outputRoot), "."+filepath.Base(outputRoot)+".")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporaryRoot)
	seenDestination := map[string]bool{}
	for index, value := range documents {
		sourceRoot := filepath.Dir(inputs[index])
		for _, artifact := range value.Artifacts {
			if seenDestination[artifact.Path] {
				return fmt.Errorf("merged artifacts contain duplicate relative path %q", artifact.Path)
			}
			seenDestination[artifact.Path] = true
			if err := copyArtifactCandidate(
				filepath.Join(sourceRoot, filepath.FromSlash(artifact.Path)),
				filepath.Join(temporaryRoot, filepath.FromSlash(artifact.Path)),
			); err != nil {
				return err
			}
		}
	}
	if err := merged.VerifyFiles(temporaryRoot, selectorDimensions(merged.Artifacts)); err != nil {
		return err
	}
	if err := document.Write(filepath.Join(temporaryRoot, filepath.Base(outputPath)), merged); err != nil {
		return err
	}
	if err := os.Rename(temporaryRoot, outputRoot); err != nil {
		return err
	}
	fmt.Printf("merged %d ArtifactSets into %s (%d artifacts)\n", len(inputs), outputPath, len(merged.Artifacts))
	return nil
}

func copyArtifactCandidate(source, destination string) error {
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("merge destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func selectorDimensions(artifacts []artifactset.Artifact) []string {
	seen := map[string]bool{}
	var result []string
	for _, artifact := range artifacts {
		for dimension := range artifact.Selectors {
			if !seen[dimension] {
				seen[dimension] = true
				result = append(result, dimension)
			}
		}
	}
	sort.Strings(result)
	return result
}

func cmdStageVNext(args []string) error {
	projectPath := defaultProjectPath
	resolutionPath := selectedResolutionPath()
	bundlePath := ""
	artifactSetPath := ""
	candidateRootFingerprint := ""
	code := 0
	minFrom := 0
	notesURL := ""
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--project":
			i++
			projectPath = mustValue(args, i, "--project")
		case arg == "--resolution":
			i++
			resolutionPath = mustValue(args, i, "--resolution")
		case arg == "--resolution-bundle":
			i++
			bundlePath = mustValue(args, i, "--resolution-bundle")
		case arg == "--artifact-set":
			i++
			artifactSetPath = mustValue(args, i, "--artifact-set")
		case arg == "--candidate-root-fingerprint":
			i++
			candidateRootFingerprint = mustValue(args, i, "--candidate-root-fingerprint")
		case arg == "--code":
			i++
			value, err := strconv.Atoi(mustValue(args, i, "--code"))
			if err != nil {
				return fmt.Errorf("--code: %w", err)
			}
			if value <= 0 {
				return fmt.Errorf("--code must be positive")
			}
			code = value
		case arg == "--min-from":
			i++
			value, err := strconv.Atoi(mustValue(args, i, "--min-from"))
			if err != nil {
				return fmt.Errorf("--min-from: %w", err)
			}
			minFrom = value
		case arg == "--notes-url":
			i++
			notesURL = mustValue(args, i, "--notes-url")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	if artifactSetPath == "" {
		return fmt.Errorf("vNext stage requires --artifact-set")
	}
	if bundlePath == "" {
		bundlePath = filepath.Join(filepath.Dir(resolutionPath), "resolution-bundle.json")
	}
	if candidateRootFingerprint == "" {
		candidateRootFingerprint = os.Getenv("RELKIT_CANDIDATE_ROOT_FINGERPRINT")
	}
	productDocument, err := project.Load(projectPath)
	if err != nil {
		return err
	}
	resolved, err := resolution.Load(resolutionPath)
	if err != nil {
		return err
	}
	artifacts, err := artifactset.Load(artifactSetPath)
	if err != nil {
		return err
	}
	staged, err := vnextrelease.Stage(*productDocument, *resolved, *artifacts, vnextrelease.StageOptions{
		ProjectRoot:              filepath.Dir(projectPath),
		ArtifactSetPath:          artifactSetPath,
		ResolutionBundlePath:     bundlePath,
		CandidateRootFingerprint: candidateRootFingerprint,
		Code:                     code,
		MinFrom:                  minFrom,
		NotesURL:                 notesURL,
	})
	if err != nil {
		return err
	}
	fmt.Printf("staged %s (code %d, channel %s) with resolution %s\n", staged.Version, staged.Code, staged.Channel, staged.ResolutionDigest)
	return nil
}

func cmdWrapper(args []string) error {
	if len(args) == 0 || args[0] != "generate" {
		return fmt.Errorf("usage: relkit wrapper generate [--project relkit.project.json] [--out-dir .]")
	}
	projectPath := defaultProjectPath
	outputDir := "."
	for i := 1; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--project":
			i++
			projectPath = mustValue(args, i, "--project")
		case arg == "--out-dir":
			i++
			outputDir = mustValue(args, i, "--out-dir")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	paths, err := wrapper.Generate(wrapper.Options{
		ProjectPath: projectPath,
		OutputDir:   outputDir,
	})
	if err != nil {
		return err
	}
	for _, path := range paths {
		fmt.Println("wrote " + path)
	}
	return nil
}

func inspectVNextResolution(path string) error {
	value, err := resolution.Load(path)
	if err != nil {
		return err
	}
	output := map[string]any{
		"schema":             value.Schema,
		"identity":           value.Identity,
		"deploymentId":       value.Operator.DeploymentID,
		"selectionPolicy":    value.Operator.SelectionPolicy,
		"distribution":       value.SelectedDistribution.Version,
		"distributionCommit": value.SelectedDistribution.Commit,
		"resolutionDigest":   value.Digest,
		"catalogDigest":      value.CatalogSnapshotDigest,
		"profileDigest":      value.Operator.ProfileDigest,
		"components":         value.Components,
	}
	data, err := jsonio.MarshalPretty(output)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}
