// Package browse builds the unsigned HTML people open in a browser.
// Protocol clients never read these files.
//
// relkit-agent rebuilds the complete site from data-plane site/latest
// documents. Product publishing never reads rendered catalog files.
package browse

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"

	"go.firoyang.com/relkit/internal/webmeta"
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

// ProductData is the complete human-facing data for one product. It is read
// from the data plane; rendered catalog files are never used as input.
type ProductData struct {
	Site    *webmeta.Site
	Latests []webmeta.Latest
}

// Build deterministically renders the entire static site from authoritative
// site/latest documents.
func Build(products []ProductData) (map[string][]byte, error) {
	catalog := &Catalog{Schema: SchemaCatalog}
	for _, input := range products {
		if len(input.Latests) == 0 {
			continue
		}
		product := productFromData(input)
		if product.ID == "" {
			continue
		}
		catalog.Products = append(catalog.Products, product)
		if input.Site != nil && input.Site.UpdatedAt > catalog.UpdatedAt {
			catalog.UpdatedAt = input.Site.UpdatedAt
		}
		for _, latest := range input.Latests {
			if latest.PublishedAt > catalog.UpdatedAt {
				catalog.UpdatedAt = latest.PublishedAt
			}
		}
	}
	sort.Slice(catalog.Products, func(i, j int) bool {
		return catalog.Products[i].ID < catalog.Products[j].ID
	})

	indexHTML, err := RenderIndex(catalog)
	if err != nil {
		return nil, err
	}
	catalogJSON, err := MarshalCatalog(catalog)
	if err != nil {
		return nil, err
	}
	dump := map[string][]byte{
		IndexKey():   indexHTML,
		CatalogKey(): catalogJSON,
	}
	for i := range catalog.Products {
		productHTML, err := RenderProduct(&catalog.Products[i])
		if err != nil {
			return nil, err
		}
		dump[ProductKey(catalog.Products[i].ID)] = productHTML
	}
	return dump, nil
}

func productFromData(input ProductData) Product {
	id := ""
	if input.Site != nil {
		id = input.Site.Product
	}
	if id == "" && len(input.Latests) > 0 {
		id = input.Latests[0].Product
	}
	page := Product{
		ID:    id,
		Title: id,
	}
	if input.Site != nil {
		if input.Site.Title != "" {
			page.Title = input.Site.Title
		}
		page.Description = input.Site.Description
		page.Homepage = input.Site.Homepage
	}
	for _, latest := range input.Latests {
		if latest.Product != id || latest.Channel == "" {
			continue
		}
		page.Channels = append(page.Channels, Channel{
			Name:        latest.Channel,
			Version:     latest.Version,
			Code:        latest.Code,
			PublishedAt: latest.PublishedAt,
			Artifacts:   humanArtifacts(latest.Artifacts),
		})
	}
	sort.Slice(page.Channels, func(i, j int) bool {
		return channelRank(page.Channels[i].Name) < channelRank(page.Channels[j].Name)
	})
	return page
}

// humanArtifacts keeps runtime-only artifacts in the signed protocol while
// removing them from the unsigned page once a release declares user downloads.
func humanArtifacts(all []webmeta.Artifact) []webmeta.Artifact {
	var userFacing []webmeta.Artifact
	for _, artifact := range all {
		if artifact.Selectors["audience"] == "user" {
			userFacing = append(userFacing, artifact)
		}
	}
	if len(userFacing) > 0 {
		return userFacing
	}
	// Old releases have no audience selector. Preserve their existing page
	// rather than rendering an empty product.
	return append([]webmeta.Artifact(nil), all...)
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
