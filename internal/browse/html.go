package browse

import (
	"bytes"
	"fmt"
	"html/template"

	"cnb.cool/shichao402/relkit/internal/webmeta"
)

var pages = template.Must(template.New("browse").Funcs(template.FuncMap{
	"firstURL":  firstURL,
	"bytes":     humanBytes,
	"platform":  platformLabel,
	"sliceDate": sliceDate,
	"chanURL":   channelDownload,
}).Parse(pageHTML))

const pageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root{color-scheme:light dark;--bg:#f6f5f1;--fg:#1c1917;--muted:#57534e;--line:#d6d3d1;--card:#fff;--accent:#1d4ed8}
@media (prefers-color-scheme:dark){:root{--bg:#1c1917;--fg:#f5f5f4;--muted:#a8a29e;--line:#44403c;--card:#292524;--accent:#93c5fd}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.5 system-ui,Segoe UI,sans-serif}
.wrap{max-width:52rem;margin:0 auto;padding:1.5rem 1rem 3rem}
h1{font-size:1.6rem;margin:.2rem 0 .4rem}h2{font-size:1.1rem;margin:0}
.sub,.desc{color:var(--muted);margin:.2rem 0 1rem}
.card,.release{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:1rem 1.1rem;margin:0 0 1rem}
.cardhead{display:flex;gap:.8rem;align-items:baseline;flex-wrap:wrap}
.cardhead a{color:inherit}table{width:100%;border-collapse:collapse;font-size:.92rem}
th,td{text-align:left;padding:.35rem .2rem;border-bottom:1px solid var(--line)}th{color:var(--muted);font-weight:600}
.num{text-align:right;font-variant-numeric:tabular-nums}.mono{font-family:ui-monospace,Consolas,monospace;font-size:.88em}
a{color:var(--accent)}nav a{color:var(--muted);text-decoration:none}nav a:hover{text-decoration:underline}
.download{display:grid;grid-template-columns:1fr auto;gap:.35rem 1rem;padding:.45rem 0;border-bottom:1px solid var(--line)}
.download:last-child{border-bottom:0}.platform{color:var(--muted);font-size:.82rem}
.btn{display:inline-block;padding:.28rem .7rem;border-radius:7px;background:color-mix(in srgb,var(--accent) 14%,transparent);color:var(--accent);font-weight:600;text-decoration:none}
p.foot{margin:2rem 0 0;padding-top:.8rem;border-top:1px solid var(--line);color:var(--muted);font-size:.8rem}
</style>
</head>
<body><div class="wrap">
{{if .Crumbs}}<nav>{{range $i, $c := .Crumbs}}{{if $i}} / {{end}}{{if $c.Href}}<a href="{{$c.Href}}">{{$c.Label}}</a>{{else}}{{$c.Label}}{{end}}{{end}}</nav>{{end}}
<header><h1>{{.Heading}}</h1>{{if .Description}}<p class="desc">{{.Description}}</p>{{end}}{{if .Sub}}<p class="sub">{{.Sub}}</p>{{end}}</header>
{{if .Index}}
{{range .Index.Products}}
<section class="card">
<div class="cardhead"><h2><a href="{{.ID}}.html">{{if .Title}}{{.Title}}{{else}}{{.ID}}{{end}}</a></h2>{{if .Homepage}}<a href="{{.Homepage}}">homepage</a>{{end}}</div>
{{if .Description}}<p class="desc">{{.Description}}</p>{{end}}
<table>
<thead><tr><th>Channel</th><th>Version</th><th class="num">Code</th><th class="num">Released</th><th class="num">Download</th></tr></thead>
<tbody>
{{range .Channels}}<tr>
<td>{{.Name}}</td>
<td class="mono">{{.Version}}</td>
<td class="num">{{if .Code}}{{.Code}}{{end}}</td>
<td class="num">{{sliceDate .PublishedAt}}</td>
<td class="num">{{with chanURL .}}<a href="{{.}}">download</a>{{else}}-{{end}}</td>
</tr>{{end}}
</tbody>
</table>
</section>
{{end}}
{{else}}
{{range .Product.Channels}}
<section class="release">
<h2>{{.Name}}</h2>
<p class="sub">{{.Version}}{{if .Code}} · build {{.Code}}{{end}}{{if .PublishedAt}} · {{sliceDate .PublishedAt}}{{end}}</p>
{{range .Artifacts}}
<div class="download"><div><div class="platform">{{platform .Selectors}}</div><div class="mono">{{.Filename}}</div><div class="sub">{{bytes .Size}}</div></div>{{with firstURL .}}<a class="btn" href="{{.}}">Download</a>{{end}}</div>
{{end}}
</section>
{{end}}
{{end}}
<p class="foot">Signed releases. Protocol clients do not read this page.</p>
</div></body></html>
`

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
	return render(pageData{
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
	return render(pageData{
		Title:       title + " · releases",
		Heading:     title,
		Description: product.Description,
		Crumbs:      []crumb{{Label: "Releases", Href: "index.html"}, {Label: title}},
		Product:     product,
	})
}

func render(data pageData) ([]byte, error) {
	var buf bytes.Buffer
	if err := pages.Execute(&buf, data); err != nil {
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
