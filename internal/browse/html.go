package browse

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"

	"cnb.cool/shichao402/relkit/internal/webmeta"
)

//go:embed templates/*.html
var templateFS embed.FS

var pages = template.Must(template.New("browse").Funcs(template.FuncMap{
	"firstURL":  firstURL,
	"bytes":     humanBytes,
	"platform":  platformLabel,
	"sliceDate": sliceDate,
	"chanURL":   channelDownload,
}).ParseFS(templateFS, "templates/*.html"))

type crumb struct {
	Label string
	Href  string
}

type pageData struct {
	Title       string
	Heading     string
	Description string
	Sub         string
	Crumbs      []crumb
	Index       *Catalog
	Product     *Product
}

func RenderIndex(cat *Catalog) ([]byte, error) {
	if cat == nil {
		return nil, fmt.Errorf("nil catalog")
	}
	heading := "Releases"
	return render("index", pageData{
		Title:   heading,
		Heading: heading,
		Sub:     "Download the build for your platform. Update clients use signed protocol files, not this page.",
		Index:   cat,
	})
}

func RenderProduct(product *Product) ([]byte, error) {
	if product == nil {
		return nil, fmt.Errorf("nil product")
	}
	title := product.Title
	if title == "" {
		title = product.ID
	}
	return render("product", pageData{
		Title:       title + " · releases",
		Heading:     title,
		Description: product.Description,
		Crumbs:      []crumb{{Label: "Releases", Href: "index.html"}, {Label: title}},
		Product:     product,
	})
}

func render(name string, data pageData) ([]byte, error) {
	var buf bytes.Buffer
	if err := pages.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func channelDownload(ch Channel) string {
	for _, artifact := range ch.Artifacts {
		if href := firstURL(artifact); href != "" {
			return href
		}
	}
	return ""
}

func firstURL(v any) string {
	switch typed := v.(type) {
	case webmeta.Artifact:
		if len(typed.URLs) > 0 {
			return typed.URLs[0]
		}
	case *webmeta.Artifact:
		if typed != nil && len(typed.URLs) > 0 {
			return typed.URLs[0]
		}
	case string:
		return typed
	}
	return ""
}

func platformLabel(selectors map[string]string) string {
	if selectors == nil {
		return ""
	}
	osName := selectors["os"]
	arch := selectors["arch"]
	switch {
	case osName != "" && arch != "":
		return osName + " / " + arch
	case osName != "":
		return osName
	default:
		return arch
	}
}

func humanBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	val := float64(n)
	units := []string{"KiB", "MiB", "GiB"}
	for _, unit := range units {
		val /= 1024
		if val < 1024 {
			return fmt.Sprintf("%.1f %s", val, unit)
		}
	}
	return fmt.Sprintf("%.1f TiB", val/1024)
}

func sliceDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}
