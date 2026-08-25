// Package webmeta defines the unsigned, human-facing documents written next
// to a RUP tree. They are conveniences for browsers, never trust inputs for a
// protocol client.
package webmeta

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
)

const (
	SchemaSite   = "relkit.site/1"
	SchemaLatest = "relkit.latest/1"
)

type Site struct {
	Schema      string `json:"schema"`
	Product     string `json:"product"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
	UpdatedAt   string `json:"updatedAt"`
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

type Artifact struct {
	ID        string            `json:"id"`
	Filename  string            `json:"filename"`
	Size      int64             `json:"size"`
	Sha256    string            `json:"sha256"`
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

func ArtifactsFromManifest(manifest *rupv2.Manifest) []Artifact {
	if manifest == nil {
		return nil
	}
	out := make([]Artifact, 0, len(manifest.Artifacts))
	for _, item := range manifest.Artifacts {
		if item == nil {
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
			Selectors: selectors,
			URLs:      append([]string(nil), item.Urls...),
		})
	}
	return out
}

func marshal(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
