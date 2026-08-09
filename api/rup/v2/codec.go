package rupv2

import (
	"fmt"
	"path"
	"sort"

	"google.golang.org/protobuf/proto"
)

const (
	SchemaIndex      = "rup.index/2"
	SchemaManifest   = "rup.manifest/2"
	SchemaStaged     = "rup.staged/2"
	SchemaFallback   = "rup.fallback/2"
	SchemaDirectory  = "rup.directory/2"
	SchemaEnvelope   = "rup.envelope/2"
	SchemaPublicKey  = "rup.publickey/2"
	SchemaPrivateKey = "rup.privatekey/2"
)

func SortSelectors(selectors []*Selector) []*Selector {
	sort.SliceStable(selectors, func(i, j int) bool {
		left := selectors[i]
		right := selectors[j]
		if left == nil || right == nil {
			return right == nil
		}
		if left.Key != right.Key {
			return left.Key < right.Key
		}
		return left.Value < right.Value
	})
	return selectors
}

func SortMetaEntry(entries []*MetaEntry) []*MetaEntry {
	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]
		if left == nil || right == nil {
			return right == nil
		}
		if left.Key != right.Key {
			return left.Key < right.Key
		}
		return left.Value < right.Value
	})
	return entries
}

func NormalizeIndex(doc *Index) *Index {
	if doc == nil {
		return nil
	}
	cloned := proto.Clone(doc).(*Index)
	cloned.Schema = SchemaIndex
	return cloned
}

func NormalizeManifest(doc *Manifest) *Manifest {
	if doc == nil {
		return nil
	}
	cloned := proto.Clone(doc).(*Manifest)
	cloned.Schema = SchemaManifest
	for _, artifact := range cloned.Artifacts {
		normalizeArtifact(artifact)
	}
	return cloned
}

func NormalizeStaged(doc *Staged) *Staged {
	if doc == nil {
		return nil
	}
	cloned := proto.Clone(doc).(*Staged)
	cloned.Schema = SchemaStaged
	for _, artifact := range cloned.Artifacts {
		normalizeStagedArtifact(artifact)
	}
	return cloned
}

func MarshalIndex(doc *Index) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("nil index")
	}
	return proto.Marshal(NormalizeIndex(doc))
}

func MarshalManifest(doc *Manifest) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("nil manifest")
	}
	return proto.Marshal(NormalizeManifest(doc))
}

func MarshalStaged(doc *Staged) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("nil staged")
	}
	return proto.Marshal(NormalizeStaged(doc))
}

func NormalizeFallback(doc *Fallback) *Fallback {
	if doc == nil {
		return nil
	}
	cloned := proto.Clone(doc).(*Fallback)
	cloned.Schema = SchemaFallback
	for _, rule := range cloned.Rules {
		if rule == nil {
			continue
		}
		SortSelectors(rule.Selectors)
	}
	return cloned
}

func MarshalFallback(doc *Fallback) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("nil fallback")
	}
	return proto.Marshal(NormalizeFallback(doc))
}

func NormalizeDirectory(doc *UpdateDirectory) *UpdateDirectory {
	if doc == nil {
		return nil
	}
	cloned := proto.Clone(doc).(*UpdateDirectory)
	cloned.Schema = SchemaDirectory
	return cloned
}

func MarshalDirectory(doc *UpdateDirectory) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("nil directory")
	}
	return proto.Marshal(NormalizeDirectory(doc))
}

func MarshalEnvelope(doc *Envelope) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("nil envelope")
	}
	cloned := proto.Clone(doc).(*Envelope)
	cloned.Schema = SchemaEnvelope
	return proto.Marshal(cloned)
}

func MarshalPublicKey(doc *PublicKeyDocument) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("nil public key document")
	}
	cloned := proto.Clone(doc).(*PublicKeyDocument)
	cloned.Schema = SchemaPublicKey
	return proto.Marshal(cloned)
}

func MarshalPrivateKey(doc *PrivateKeyDocument) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("nil private key document")
	}
	cloned := proto.Clone(doc).(*PrivateKeyDocument)
	cloned.Schema = SchemaPrivateKey
	return proto.Marshal(cloned)
}

func UnmarshalIndex(data []byte) (*Index, error) {
	doc := &Index{}
	if err := proto.Unmarshal(data, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func UnmarshalManifest(data []byte) (*Manifest, error) {
	doc := &Manifest{}
	if err := proto.Unmarshal(data, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func UnmarshalStaged(data []byte) (*Staged, error) {
	doc := &Staged{}
	if err := proto.Unmarshal(data, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func UnmarshalFallback(data []byte) (*Fallback, error) {
	doc := &Fallback{}
	if err := proto.Unmarshal(data, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func UnmarshalDirectory(data []byte) (*UpdateDirectory, error) {
	doc := &UpdateDirectory{}
	if err := proto.Unmarshal(data, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func UnmarshalEnvelope(data []byte) (*Envelope, error) {
	doc := &Envelope{}
	if err := proto.Unmarshal(data, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func UnmarshalPublicKey(data []byte) (*PublicKeyDocument, error) {
	doc := &PublicKeyDocument{}
	if err := proto.Unmarshal(data, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func UnmarshalPrivateKey(data []byte) (*PrivateKeyDocument, error) {
	doc := &PrivateKeyDocument{}
	if err := proto.Unmarshal(data, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func IndexKey(product, channel string) string {
	return path.Join("index", product, channel+".pb")
}

func ManifestKey(product, version string) string {
	return path.Join("manifest", product, version+".pb")
}

func FallbackKey(product string) string {
	return path.Join("fallback", product+".pb")
}

func DirectoryKey(product string) string {
	return path.Join("directory", product+".pb")
}

func normalizeArtifact(artifact *Artifact) {
	if artifact == nil {
		return
	}
	SortSelectors(artifact.Selectors)
	SortMetaEntry(artifact.Meta)
}

func normalizeStagedArtifact(artifact *StagedArtifact) {
	if artifact == nil {
		return
	}
	SortSelectors(artifact.Selectors)
	SortMetaEntry(artifact.Meta)
}
