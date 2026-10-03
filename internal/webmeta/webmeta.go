// Package webmeta defines the unsigned, human-facing documents written next
// to a RUP tree. They are conveniences for browsers, never trust inputs for a
// protocol client.
package webmeta

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
)

const (
	SchemaSite    = "relkit.site/1"
	SchemaLatest  = "relkit.latest/1"
	SchemaChannel = "relkit.channel/1"
	SchemaRelease = "relkit.release/1"
)

type Site struct {
	Schema      string   `json:"schema"`
	Product     string   `json:"product"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Channels    []string `json:"channels,omitempty"`
	UpdatedAt   string   `json:"updatedAt"`
}

type Latest struct {
	Schema      string     `json:"schema"`
	Product     string     `json:"product"`
	Channel     string     `json:"channel"`
	Version     string     `json:"version"`
	Code        int64      `json:"code"`
	PublishedAt string     `json:"publishedAt"`
	Artifacts   []Artifact `json:"artifacts"`
}

// Channel is the per-channel directory document: the latest release plus the
// full retained version list projected from the signed index. It is the data
// plane for the site SPA; history depth equals the protocol retainVersions
// window, so there is deliberately no second ledger.
type Channel struct {
	Schema    string       `json:"schema"`
	Product   string       `json:"product"`
	Channel   string       `json:"channel"`
	UpdatedAt string       `json:"updatedAt"`
	Latest    ChannelEntry `json:"latest"`
	Versions  []ChannelEntry `json:"versions"`
}

// ChannelEntry is one row of the channel history. ReleaseDoc points at the
// immutable release/<product>/<channel>/<version>.json so the SPA can fetch
// download details on demand.
type ChannelEntry struct {
	Version    string `json:"version"`
	Code       int64  `json:"code"`
	ReleasedAt string `json:"releasedAt"`
	NotesURL   string `json:"notesUrl,omitempty"`
	Yanked     bool   `json:"yanked,omitempty"`
	ReleaseDoc string `json:"releaseDoc"`
}

// Release is the immutable per-version detail document: everything a human
// needs to download one specific release of one channel.
type Release struct {
	Schema      string     `json:"schema"`
	Product     string     `json:"product"`
	Channel     string     `json:"channel"`
	Version     string     `json:"version"`
	Code        int64      `json:"code"`
	ReleasedAt  string     `json:"releasedAt"`
	NotesURL    string     `json:"notesUrl,omitempty"`
	Artifacts   []Artifact `json:"artifacts"`
}

type Artifact struct {
	ID        string            `json:"id"`
	Filename  string            `json:"filename"`
	Size      int64             `json:"size"`
	Sha256    string            `json:"sha256"`
	Kind      string            `json:"kind"`
	Selectors map[string]string `json:"selectors,omitempty"`
	URLs      []string          `json:"urls"`
}

func SiteKey(product string) string {
	return path.Join("site", product+".json")
}

// LatestKey is channel-scoped: a fixed download URL has to say which channel it
// tracks, or a link pasted into a document silently means "whatever channel the
// publisher considered default that week".
func LatestKey(product, channel string) string {
	return path.Join("latest", product, channel+".json")
}

func ChannelKey(product, channel string) string {
	return path.Join("channel", product, channel+".json")
}

// ReleaseKey is the immutable per-version document. It lives under
// release/<product>/<channel>/ so one channel's history sits in one folder.
func ReleaseKey(product, channel, version string) string {
	return path.Join("release", product, channel, version+".json")
}

func MarshalSite(doc Site) ([]byte, error) {
	doc.Schema = SchemaSite
	return marshal(doc)
}

func MarshalLatest(doc Latest) ([]byte, error) {
	doc.Schema = SchemaLatest
	sort.SliceStable(doc.Artifacts, func(i, j int) bool {
		return doc.Artifacts[i].ID < doc.Artifacts[j].ID
	})
	return marshal(doc)
}

func MarshalChannel(doc Channel) ([]byte, error) {
	doc.Schema = SchemaChannel
	if len(doc.Versions) == 0 {
		return nil, fmt.Errorf("channel %s/%s has no versions", doc.Product, doc.Channel)
	}
	doc.Latest.ReleaseDoc = ReleaseKey(doc.Product, doc.Channel, doc.Latest.Version)
	for i := range doc.Versions {
		doc.Versions[i].ReleaseDoc = ReleaseKey(doc.Product, doc.Channel, doc.Versions[i].Version)
	}
	sort.SliceStable(doc.Versions, func(i, j int) bool {
		if doc.Versions[i].Code != doc.Versions[j].Code {
			return doc.Versions[i].Code > doc.Versions[j].Code
		}
		return doc.Versions[i].Version > doc.Versions[j].Version
	})
	return marshal(doc)
}

func MarshalRelease(doc Release) ([]byte, error) {
	doc.Schema = SchemaRelease
	sort.SliceStable(doc.Artifacts, func(i, j int) bool {
		return doc.Artifacts[i].ID < doc.Artifacts[j].ID
	})
	return marshal(doc)
}

func UnmarshalSite(data []byte) (*Site, error) {
	var doc Site
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Schema != SchemaSite {
		return nil, fmt.Errorf("unexpected site schema %q", doc.Schema)
	}
	if doc.Product == "" {
		return nil, fmt.Errorf("site product is empty")
	}
	return &doc, nil
}

func UnmarshalLatest(data []byte) (*Latest, error) {
	var doc Latest
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Schema != SchemaLatest {
		return nil, fmt.Errorf("unexpected latest schema %q", doc.Schema)
	}
	if doc.Product == "" || doc.Channel == "" || doc.Version == "" {
		return nil, fmt.Errorf("latest product, channel, and version are required")
	}
	if len(doc.Artifacts) == 0 {
		return nil, fmt.Errorf("latest has no artifacts")
	}
	return &doc, nil
}

func UnmarshalChannel(data []byte) (*Channel, error) {
	var doc Channel
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Schema != SchemaChannel {
		return nil, fmt.Errorf("unexpected channel schema %q", doc.Schema)
	}
	if doc.Product == "" || doc.Channel == "" {
		return nil, fmt.Errorf("channel product and channel are required")
	}
	if doc.Latest.Version == "" {
		return nil, fmt.Errorf("channel %s/%s has no latest version", doc.Product, doc.Channel)
	}
	return &doc, nil
}

func UnmarshalRelease(data []byte) (*Release, error) {
	var doc Release
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Schema != SchemaRelease {
		return nil, fmt.Errorf("unexpected release schema %q", doc.Schema)
	}
	if doc.Product == "" || doc.Channel == "" || doc.Version == "" {
		return nil, fmt.Errorf("release product, channel, and version are required")
	}
	return &doc, nil
}

func ArtifactsFromManifest(manifest *rupv2.Manifest) []Artifact {
	if manifest == nil {
		return nil
	}
	out := make([]Artifact, 0, len(manifest.Artifacts))
	for _, item := range manifest.Artifacts {
		if item == nil {
			continue
		}
		if item.Kind == rupv2.ArtifactKind_ARTIFACT_KIND_PAYLOAD {
			continue
		}
		selectors := make(map[string]string, len(item.Selectors))
		for _, selector := range item.Selectors {
			if selector != nil {
				selectors[selector.Key] = selector.Value
			}
		}
		out = append(out, Artifact{
			ID:        item.Id,
			Filename:  item.Filename,
			Size:      item.Size,
			Sha256:    item.Sha256,
			Kind:      kindString(item.Kind),
			Selectors: selectors,
			URLs:      append([]string(nil), item.Urls...),
		})
	}
	return out
}

func kindString(kind rupv2.ArtifactKind) string {
	switch kind {
	case rupv2.ArtifactKind_ARTIFACT_KIND_ARCHIVE:
		return "archive"
	case rupv2.ArtifactKind_ARTIFACT_KIND_INSTALLER:
		return "installer"
	case rupv2.ArtifactKind_ARTIFACT_KIND_BINARY:
		return "binary"
	case rupv2.ArtifactKind_ARTIFACT_KIND_BLOB:
		return "blob"
	default:
		return ""
	}
}

func marshal(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
