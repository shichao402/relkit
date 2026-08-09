package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
)

const (
	SchemaStaged     = rupv2.SchemaStaged
	SchemaManifest   = rupv2.SchemaManifest
	SchemaIndex      = rupv2.SchemaIndex
	SchemaFallback   = rupv2.SchemaFallback
	SchemaDirectory  = rupv2.SchemaDirectory
	SchemaEnvelope   = rupv2.SchemaEnvelope
	SchemaPublicKey  = rupv2.SchemaPublicKey
	SchemaPrivateKey = rupv2.SchemaPrivateKey
)

type Envelope = rupv2.Envelope
type Signature = rupv2.Signature
type PublicKeyDocument = rupv2.PublicKeyDocument
type PrivateKeyDocument = rupv2.PrivateKeyDocument
type StagedDocument = rupv2.Staged
type StagedArtifact = rupv2.StagedArtifact
type ManifestDocument = rupv2.Manifest
type ManifestArtifact = rupv2.Artifact
type IndexDocument = rupv2.Index
type FallbackDocument = rupv2.Fallback
type FallbackRule = rupv2.FallbackRule
type UpdateDirectory = rupv2.UpdateDirectory
type DirectoryService = rupv2.DirectoryService
type VersionNode = rupv2.VersionNode
type ManifestRef = rupv2.DigestRef
type Selector = rupv2.Selector
type MetaEntry = rupv2.MetaEntry

var Kinds = []string{"archive", "installer", "binary", "blob"}

var (
	identifierPattern  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	selectorKeyPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)
	windowsReserved    = map[string]struct{}{
		"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
		"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {},
		"COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
		"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {},
		"LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
	}
	kindBySuffix = []struct {
		Suffix string
		Kind   string
	}{
		{".tar.gz", "archive"},
		{".tgz", "archive"},
		{".zip", "archive"},
		{".exe", "installer"},
		{".msi", "installer"},
		{".dmg", "installer"},
		{".pkg", "installer"},
		{".apk", "installer"},
		{".deb", "installer"},
		{".rpm", "installer"},
	}
	standardSelectorOrder = []string{"os", "arch", "target", "abi", "variant"}
)

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

func UTCNow() string {
	return time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
}

func CheckIdentifier(value, what string) error {
	if value == "" {
		return ValidationError{Message: fmt.Sprintf("%s must be a non-empty string", what)}
	}
	if len(value) > 64 {
		return ValidationError{Message: fmt.Sprintf("%s exceeds 64 characters: %q", what, value)}
	}
	if !identifierPattern.MatchString(value) {
		return ValidationError{Message: fmt.Sprintf("%s must match [a-zA-Z0-9][a-zA-Z0-9._-]*, got %q", what, value)}
	}
	return nil
}

func CheckFilename(name string) error {
	if name == "" {
		return ValidationError{Message: "filename must be a non-empty string"}
	}
	if len(name) > 255 {
		return ValidationError{Message: fmt.Sprintf("filename exceeds 255 characters: %q", name)}
	}
	if strings.ContainsAny(name, `/\`) {
		return ValidationError{Message: fmt.Sprintf("filename must not contain a path separator: %q", name)}
	}
	if name == "." || name == ".." {
		return ValidationError{Message: fmt.Sprintf("filename must not be %q", name)}
	}
	if strings.Contains(name, "..") {
		return ValidationError{Message: fmt.Sprintf("filename must not contain '..': %q", name)}
	}
	for _, ch := range name {
		if ch < 0x20 || ch == 0x7F {
			return ValidationError{Message: fmt.Sprintf("filename must not contain control characters: %q", name)}
		}
	}
	stem := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if _, ok := windowsReserved[stem]; ok {
		return ValidationError{Message: fmt.Sprintf("filename is a reserved device name on Windows: %q", name)}
	}
	return nil
}

func CheckSelectors(selectors map[string]string, what string) error {
	if selectors == nil {
		return ValidationError{Message: fmt.Sprintf("%s must be an object", what)}
	}
	for key, value := range selectors {
		if !selectorKeyPattern.MatchString(key) {
			return ValidationError{Message: fmt.Sprintf("%s key must match [a-zA-Z][a-zA-Z0-9_-]*, got %q", what, key)}
		}
		if value == "" {
			return ValidationError{Message: fmt.Sprintf("%s[%q] must be a non-empty string", what, key)}
		}
	}
	return nil
}

func CheckVersion(version string) error {
	if version == "" {
		return ValidationError{Message: "version must be a non-empty string"}
	}
	if len(version) > 64 {
		return ValidationError{Message: fmt.Sprintf("version exceeds 64 characters: %q", version)}
	}
	if strings.ContainsAny(version, `/\`) {
		return ValidationError{Message: fmt.Sprintf("version must not contain a path separator: %q", version)}
	}
	if version == "." || version == ".." || strings.Contains(version, "..") {
		return ValidationError{Message: fmt.Sprintf("version must not contain '..': %q", version)}
	}
	for _, ch := range version {
		if ch < 0x20 || ch == 0x7F {
			return ValidationError{Message: "version must not contain control characters"}
		}
	}
	return nil
}

func IndexKey(product, channel string) string {
	return rupv2.IndexKey(product, channel)
}

func ManifestKey(product, version string) string {
	return rupv2.ManifestKey(product, version)
}

func FallbackKey(product string) string {
	return rupv2.FallbackKey(product)
}

func DirectoryKey(product string) string {
	return rupv2.DirectoryKey(product)
}

func ArtifactKey(product, version, filename string) string {
	return path.Join("artifact", product, version, filename)
}

func InferKind(filename string) string {
	lowered := strings.ToLower(filename)
	for _, entry := range kindBySuffix {
		if strings.HasSuffix(lowered, entry.Suffix) {
			return entry.Kind
		}
	}
	if !strings.Contains(filename, ".") {
		return "binary"
	}
	return "blob"
}

func DefaultArtifactID(selectors map[string]string) (string, error) {
	if len(selectors) == 0 {
		return "default", nil
	}

	keys := make([]string, 0, len(selectors))
	for key := range selectors {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		ri := selectorRank(keys[i])
		rj := selectorRank(keys[j])
		if ri[0] != rj[0] {
			return ri[0] < rj[0]
		}
		if ri[1] != rj[1] {
			return ri[1] < rj[1]
		}
		return ri[2] < rj[2]
	})

	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, selectors[key])
	}
	joined := strings.Join(values, "-")
	if err := CheckIdentifier(joined, "derived artifact id"); err != nil {
		return "", err
	}
	return joined, nil
}

func SelectorsFromMap(selectors map[string]string) []*Selector {
	if len(selectors) == 0 {
		return nil
	}
	out := make([]*Selector, 0, len(selectors))
	for key, value := range selectors {
		out = append(out, &Selector{Key: key, Value: value})
	}
	return rupv2.SortSelectors(out)
}

func SelectorsToMap(selectors []*Selector) map[string]string {
	if len(selectors) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(selectors))
	for _, selector := range selectors {
		if selector == nil || selector.Key == "" {
			continue
		}
		out[selector.Key] = selector.Value
	}
	return out
}

func MetaEntriesFromAnyMap(meta map[string]any) ([]*MetaEntry, error) {
	if len(meta) == 0 {
		return nil, nil
	}
	out := make([]*MetaEntry, 0, len(meta))
	for key, raw := range meta {
		value, ok := raw.(string)
		if !ok {
			return nil, ValidationError{Message: fmt.Sprintf("meta[%q] must be a string, got %T", key, raw)}
		}
		out = append(out, &MetaEntry{Key: key, Value: value})
	}
	return rupv2.SortMetaEntry(out), nil
}

func MetaToMap(entries []*MetaEntry) map[string]string {
	if len(entries) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.Key == "" {
			continue
		}
		out[entry.Key] = entry.Value
	}
	return out
}

func HasMinSupported(index *IndexDocument) bool {
	return index != nil && index.HasMinSupported
}

func MinSupportedPtr(index *IndexDocument) *int {
	if !HasMinSupported(index) {
		return nil
	}
	value := int(index.MinSupported)
	return &value
}

func Sha256File(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	digest := sha256.New()
	size, err := io.Copy(digest, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(digest.Sum(nil)), size, nil
}

func Sha256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func NewStagedDocument(product, version string, code, minFrom int, channel string, artifacts []*StagedArtifact, notes, notesURL, createdAt string) (*StagedDocument, error) {
	if err := CheckIdentifier(product, "product"); err != nil {
		return nil, err
	}
	if err := CheckIdentifier(channel, "channel"); err != nil {
		return nil, err
	}
	if err := CheckVersion(version); err != nil {
		return nil, err
	}
	if code < 0 {
		return nil, ValidationError{Message: "code must be a non-negative integer"}
	}
	if minFrom < 0 {
		return nil, ValidationError{Message: "minFrom must be a non-negative integer"}
	}
	if len(artifacts) == 0 {
		return nil, ValidationError{Message: "a release needs at least one artifact"}
	}

	doc := &StagedDocument{
		Schema:    SchemaStaged,
		Product:   product,
		Version:   version,
		Code:      int64(code),
		MinFrom:   int64(minFrom),
		Channel:   channel,
		CreatedAt: nonEmptyOr(createdAt, UTCNow()),
		Artifacts: cloneStagedArtifacts(artifacts),
	}
	if notes != "" {
		doc.Notes = notes
	}
	if notesURL != "" {
		doc.NotesUrl = notesURL
	}
	return doc, nil
}

func NewStagedArtifact(artifactID, filename string, size int64, digest, kind string, selectors map[string]string, meta map[string]any, sourcePath string) (*StagedArtifact, error) {
	if err := CheckIdentifier(artifactID, "artifact id"); err != nil {
		return nil, err
	}
	if err := CheckFilename(filename); err != nil {
		return nil, err
	}
	if err := CheckSelectors(selectors, "selectors"); err != nil {
		return nil, err
	}
	if !containsKind(kind) {
		return nil, ValidationError{Message: fmt.Sprintf("kind must be one of %v, got %q", Kinds, kind)}
	}

	metaEntries, err := MetaEntriesFromAnyMap(meta)
	if err != nil {
		return nil, err
	}

	artifact := &StagedArtifact{
		Id:        artifactID,
		Filename:  filename,
		Size:      size,
		Sha256:    digest,
		Kind:      kind,
		Selectors: SelectorsFromMap(selectors),
	}
	if len(metaEntries) > 0 {
		artifact.Meta = metaEntries
	}
	if sourcePath != "" {
		artifact.SourcePath = sourcePath
	}
	return artifact, nil
}

func NewManifestFromStaged(staged *StagedDocument, urlsByArtifactID map[string][]string, releasedAt string) (*ManifestDocument, error) {
	artifacts := make([]*ManifestArtifact, 0, len(staged.Artifacts))
	for _, item := range staged.Artifacts {
		if item == nil {
			continue
		}
		urls := append([]string(nil), urlsByArtifactID[item.Id]...)
		if len(urls) == 0 {
			return nil, ValidationError{Message: fmt.Sprintf("no URLs resolved for artifact %q", item.Id)}
		}
		artifact := &ManifestArtifact{
			Id:        item.Id,
			Filename:  item.Filename,
			Size:      item.Size,
			Sha256:    item.Sha256,
			Kind:      item.Kind,
			Selectors: cloneSelectors(item.Selectors),
			Urls:      urls,
		}
		if len(item.Meta) > 0 {
			artifact.Meta = cloneMetaEntries(item.Meta)
		}
		artifacts = append(artifacts, artifact)
	}

	manifest := &ManifestDocument{
		Schema:     SchemaManifest,
		Product:    staged.Product,
		Version:    staged.Version,
		Code:       staged.Code,
		ReleasedAt: nonEmptyOr(releasedAt, UTCNow()),
		Artifacts:  artifacts,
	}
	if staged.Notes != "" {
		manifest.Notes = staged.Notes
	}
	return manifest, nil
}

func NewIndexNode(staged *StagedDocument, manifestDigest string, manifestSize int64, manifestURLs []string, releasedAt string) *VersionNode {
	node := &VersionNode{
		Version:    staged.Version,
		Code:       staged.Code,
		MinFrom:    staged.MinFrom,
		ReleasedAt: nonEmptyOr(releasedAt, UTCNow()),
		Manifest: &ManifestRef{
			Sha256: manifestDigest,
			Size:   manifestSize,
			Urls:   append([]string(nil), manifestURLs...),
		},
	}
	if staged.Notes != "" {
		node.Notes = staged.Notes
	}
	if staged.NotesUrl != "" {
		node.NotesUrl = staged.NotesUrl
	}
	return node
}

func NewIndex(product, channel string, sequence int, versions []*VersionNode, minSupported *int, generatedAt string) (*IndexDocument, error) {
	if err := CheckIdentifier(product, "product"); err != nil {
		return nil, err
	}
	if err := CheckIdentifier(channel, "channel"); err != nil {
		return nil, err
	}
	if sequence < 1 {
		return nil, ValidationError{Message: "sequence must be an integer >= 1"}
	}
	if len(versions) == 0 {
		return nil, ValidationError{Message: "index must contain at least one version"}
	}

	cloned := cloneVersionNodes(versions)
	sort.Slice(cloned, func(i, j int) bool {
		return cloned[i].Code < cloned[j].Code
	})

	doc := &IndexDocument{
		Schema:      SchemaIndex,
		Product:     product,
		Channel:     channel,
		Sequence:    int64(sequence),
		GeneratedAt: nonEmptyOr(generatedAt, UTCNow()),
		Versions:    cloned,
	}
	if minSupported != nil {
		doc.MinSupported = int64(*minSupported)
		doc.HasMinSupported = true
	}
	return doc, nil
}

func EmptyIndex(product, channel string) *IndexDocument {
	return &IndexDocument{
		Schema:      SchemaIndex,
		Product:     product,
		Channel:     channel,
		Sequence:    0,
		GeneratedAt: UTCNow(),
		Versions:    []*VersionNode{},
	}
}

func selectorRank(key string) [3]string {
	for idx, candidate := range standardSelectorOrder {
		if key == candidate {
			return [3]string{"0", fmt.Sprintf("%03d", idx), key}
		}
	}
	return [3]string{"1", "000", key}
}

func cloneVersionNodes(values []*VersionNode) []*VersionNode {
	if len(values) == 0 {
		return nil
	}
	out := make([]*VersionNode, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		cloned := *value
		if value.Manifest != nil {
			manifest := *value.Manifest
			manifest.Urls = append([]string(nil), value.Manifest.Urls...)
			cloned.Manifest = &manifest
		}
		out = append(out, &cloned)
	}
	return out
}

func cloneStagedArtifacts(values []*StagedArtifact) []*StagedArtifact {
	if len(values) == 0 {
		return nil
	}
	out := make([]*StagedArtifact, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		cloned := *value
		cloned.Selectors = cloneSelectors(value.Selectors)
		cloned.Meta = cloneMetaEntries(value.Meta)
		out = append(out, &cloned)
	}
	return out
}

func cloneSelectors(values []*Selector) []*Selector {
	if len(values) == 0 {
		return nil
	}
	out := make([]*Selector, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		cloned := *value
		out = append(out, &cloned)
	}
	return out
}

func cloneMetaEntries(values []*MetaEntry) []*MetaEntry {
	if len(values) == 0 {
		return nil
	}
	out := make([]*MetaEntry, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		cloned := *value
		out = append(out, &cloned)
	}
	return out
}

func containsKind(kind string) bool {
	for _, candidate := range Kinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

func nonEmptyOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
