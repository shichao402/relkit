// Package browse builds the unsigned HTML people open in a browser.
// Protocol clients never read these files.
package browse

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"

	"cnb.cool/shichao402/relkit/internal/webmeta"
)

const SchemaCatalog = "relkit.browse-catalog/1"

func IndexKey() string { return "browse/index.html" }

func CatalogKey() string { return "browse/catalog.json" }

func ProductKey(product string) string {
	return path.Join("browse", product+".html")
}

type Catalog struct {
	Schema    string    `json:"schema"`
	UpdatedAt string    `json:"updatedAt"`
	Products  []Product `json:"products"`
}

type Product struct {
	ID          string    `json:"id"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	Homepage    string    `json:"homepage,omitempty"`
	Channels    []Channel `json:"channels"`
}

type Channel struct {
	Name        string             `json:"name"`
	Version     string             `json:"version"`
	Code        int64              `json:"code"`
	PublishedAt string             `json:"publishedAt,omitempty"`
	Artifacts   []webmeta.Artifact `json:"artifacts"`
}

func ApplyPublish(existing *Catalog, site *webmeta.Site, latest webmeta.Latest, updatedAt string) *Catalog {
	out := &Catalog{Schema: SchemaCatalog, UpdatedAt: updatedAt}
	if existing != nil {
		out.Products = append([]Product(nil), existing.Products...)
	}

	page := productFromPublish(site, latest)
	replaced := false
	for i, prev := range out.Products {
		if prev.ID != page.ID {
			continue
		}
		out.Products[i] = mergeProduct(prev, page)
		replaced = true
		break
	}
	if !replaced {
		out.Products = append(out.Products, page)
	}
	sort.Slice(out.Products, func(i, j int) bool {
		return out.Products[i].ID < out.Products[j].ID
	})
	return out
}

func productFromPublish(site *webmeta.Site, latest webmeta.Latest) Product {
	page := Product{
		ID:    latest.Product,
		Title: latest.Product,
		Channels: []Channel{{
			Name:        latest.Channel,
			Version:     latest.Version,
			Code:        latest.Code,
			PublishedAt: latest.PublishedAt,
			Artifacts:   append([]webmeta.Artifact(nil), latest.Artifacts...),
		}},
	}
	if site != nil {
		if site.Title != "" {
			page.Title = site.Title
		}
		page.Description = site.Description
		page.Homepage = site.Homepage
	}
	return page
}

func mergeProduct(prev, next Product) Product {
	out := prev
	out.Title = next.Title
	out.Description = next.Description
	out.Homepage = next.Homepage
	replaced := false
	for i, ch := range out.Channels {
		if ch.Name != next.Channels[0].Name {
			continue
		}
		out.Channels[i] = next.Channels[0]
		replaced = true
		break
	}
	if !replaced {
		out.Channels = append(out.Channels, next.Channels[0])
	}
	sort.Slice(out.Channels, func(i, j int) bool {
		return channelRank(out.Channels[i].Name) < channelRank(out.Channels[j].Name)
	})
	return out
}

func channelRank(name string) string {
	switch name {
	case "stable":
		return "0" + name
	case "beta":
		return "1" + name
	case "dev":
		return "2" + name
	default:
		return "3" + name
	}
}

func MarshalCatalog(doc *Catalog) ([]byte, error) {
	doc.Schema = SchemaCatalog
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func UnmarshalCatalog(data []byte) (*Catalog, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty catalog")
	}
	var doc Catalog
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Schema != SchemaCatalog {
		return nil, fmt.Errorf("unexpected catalog schema %q", doc.Schema)
	}
	return &doc, nil
}

func ProductPage(cat *Catalog, id string) *Product {
	if cat == nil {
		return nil
	}
	for i := range cat.Products {
		if cat.Products[i].ID == id {
			return &cat.Products[i]
		}
	}
	return nil
}

func DumpDir(root string) string {
	return filepath.Join(root, ".relkit", "browse")
}

func WriteDump(root string, files map[string][]byte) error {
	dir := DumpDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for key, data := range files {
		name := filepath.Base(key)
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func ReadDumpCatalog(root string) *Catalog {
	raw, err := os.ReadFile(filepath.Join(DumpDir(root), "catalog.json"))
	if err != nil {
		return nil
	}
	doc, err := UnmarshalCatalog(raw)
	if err != nil {
		return nil
	}
	return doc
}
