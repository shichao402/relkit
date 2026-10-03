// Package browse builds the human-facing site: a small SPA shell plus the
// data documents it fetches at runtime. Protocol clients never read these
// files.
//
// The presentation plane is deliberately split from the data plane
// (2026-10-03): webmeta documents (site/, latest/, channel/, release/) are
// written by the publisher and never re-derived here. This package only
// embeds the static SPA assets and aggregates the product catalog snapshot
// (catalog.json), because that one document spans products and no single
// product publish can write it.
package browse

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"sort"

	"github.com/shichao402/relkit/internal/webmeta"
)

//go:embed web/index.html web/assets/app.js web/assets/style.css
var webFS embed.FS

const SchemaCatalog = "relkit.browse-catalog/1"

func IndexKey() string { return "browse/index.html" }

func CatalogKey() string { return "browse/catalog.json" }

func AssetKey(name string) string { return path.Join("browse", "assets", name) }

// Catalog is the product-list snapshot the SPA loads first. It is derived
// data, never merged back, and rebuilt whole on every site rebuild.
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
	Name        string `json:"name"`
	Version     string `json:"version"`
	Code        int64  `json:"code"`
	PublishedAt string `json:"publishedAt,omitempty"`
}

// ProductData is the collected human-facing data for one product: its site
// document plus one channel document per channel. Releases carries the
// release detail documents copied into the dump (backfilled during collect
// or written by the publisher); the SPA fetches them on demand from these
// exact paths.
type ProductData struct {
	Site     *webmeta.Site
	Latests  []webmeta.Latest
	Channels []webmeta.Channel
	Releases []webmeta.Release
}

// Build renders the complete static site: the SPA shell, its assets, the
// catalog snapshot, and a verbatim copy of every collected data document so
// page and data leave from the same origin.
func Build(products []ProductData) (map[string][]byte, error) {
	catalog := &Catalog{Schema: SchemaCatalog}
	for _, input := range products {
		if input.Site == nil || len(input.Channels) == 0 {
			continue
		}
		product := Product{
			ID:          input.Site.Product,
			Title:       input.Site.Product,
			Description: input.Site.Description,
			Homepage:    input.Site.Homepage,
		}
		if input.Site.Title != "" {
			product.Title = input.Site.Title
		}
		for _, channel := range input.Channels {
			if channel.Product != input.Site.Product {
				continue
			}
			product.Channels = append(product.Channels, Channel{
				Name:        channel.Channel,
				Version:     channel.Latest.Version,
				Code:        channel.Latest.Code,
				PublishedAt: channel.Latest.ReleasedAt,
			})
			if channel.UpdatedAt > catalog.UpdatedAt {
				catalog.UpdatedAt = channel.UpdatedAt
			}
		}
		if len(product.Channels) > 0 {
			catalog.Products = append(catalog.Products, product)
		}
	}
	sort.Slice(catalog.Products, func(i, j int) bool {
		return catalog.Products[i].ID < catalog.Products[j].ID
	})
	for i := range catalog.Products {
		sort.Slice(catalog.Products[i].Channels, func(a, b int) bool {
			return channelRank(catalog.Products[i].Channels[a].Name) < channelRank(catalog.Products[i].Channels[b].Name)
		})
	}

	indexHTML, err := webFS.ReadFile("web/index.html")
	if err != nil {
		return nil, fmt.Errorf("spa shell: %w", err)
	}
	appJS, err := webFS.ReadFile("web/assets/app.js")
	if err != nil {
		return nil, fmt.Errorf("spa app: %w", err)
	}
	styleCSS, err := webFS.ReadFile("web/assets/style.css")
	if err != nil {
		return nil, fmt.Errorf("spa style: %w", err)
	}
	catalogJSON, err := MarshalCatalog(catalog)
	if err != nil {
		return nil, err
	}

	dump := map[string][]byte{
		IndexKey():            indexHTML,
		AssetKey("app.js"):    appJS,
		AssetKey("style.css"): styleCSS,
		CatalogKey():          catalogJSON,
	}

	// Data snapshot: every collected document ships with the dump verbatim, so
	// the SPA's fetches resolve on the static host regardless of whether that
	// host is also the data plane (backend sink) or a standalone tree
	// (directory/makers sinks).
	for _, input := range products {
		if input.Site == nil {
			continue
		}
		siteData, err := webmeta.MarshalSite(*input.Site)
		if err != nil {
			return nil, err
		}
		dump[webmeta.SiteKey(input.Site.Product)] = siteData
		for i := range input.Latests {
			data, err := webmeta.MarshalLatest(input.Latests[i])
			if err != nil {
				return nil, err
			}
			dump[webmeta.LatestKey(input.Latests[i].Product, input.Latests[i].Channel)] = data
		}
		for i := range input.Channels {
			data, err := webmeta.MarshalChannel(input.Channels[i])
			if err != nil {
				return nil, err
			}
			dump[webmeta.ChannelKey(input.Channels[i].Product, input.Channels[i].Channel)] = data
		}
		for i := range input.Releases {
			data, err := webmeta.MarshalRelease(input.Releases[i])
			if err != nil {
				return nil, err
			}
			dump[webmeta.ReleaseKey(input.Releases[i].Product, input.Releases[i].Channel, input.Releases[i].Version)] = data
		}
	}

	return dump, nil
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
