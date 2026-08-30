package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

func writeOperatorPreview(dir string) error {
	if dir == "" {
		return fmt.Errorf("output directory is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	pages := map[string]any{
		"portal.html":  samplePortalPage(),
		"product.html": sampleProductPage(),
		"listing.html": sampleListingPage(),
	}
	for name, data := range pages {
		tmpl := name[:len(name)-len(filepath.Ext(name))]
		var buf bytes.Buffer
		if err := pageTemplates.ExecuteTemplate(&buf, tmpl, data); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func samplePortalPage() portalPage {
	return portalPage{
		pageChrome: pageChrome{Title: "Releases", Version: "preview"},
		Heading:    "Releases",
		Sub:        "Operator panel preview. Protocol clients do not read this page.",
		StatsSince: "2026-08-01",
		Products: []productCard{{
			Display:     "SVN Auto Merge",
			Description: "Intranet desktop merge tool.",
			Homepage:    "https://git.woa.com/osgame-client/SvnMergeTool",
			Href:        "/-/p/svn-auto-merge",
			Updated:     "2026-08-30",
			Downloads:   12,
			Channels: []channelRow{{
				Name:         "dev",
				Version:      "0.2.0+106",
				Code:         106,
				Released:     "2026-08-30",
				DownloadHref: "/-/latest/svn-auto-merge/dev",
				DownloadName: "SvnAutoMerge_windows_0.2.0build106.zip",
				DownloadSize: "12 MiB",
			}},
		}},
	}
}

func sampleProductPage() productPage {
	return productPage{
		pageChrome: pageChrome{
			Title:   "svn-auto-merge",
			Version: "preview",
			Crumbs:  []crumb{{Label: "Releases", Href: "/-/admin"}, {Label: "svn-auto-merge"}},
		},
		Name:        "SVN Auto Merge",
		Description: "Intranet desktop merge tool.",
		Homepage:    "https://git.woa.com/osgame-client/SvnMergeTool",
		Channels: []channelSection{{
			Name:           "dev",
			Sequence:       12,
			Generated:      "2026-08-30 17:01",
			IndexHref:      "/index/svn-auto-merge/dev.pb",
			LatestLabel:    "0.2.0+106",
			LatestCode:     106,
			LatestReleased: "2026-08-30",
			Artifacts: []artifactRow{{
				ID:            "win",
				Filename:      "SvnAutoMerge_windows_0.2.0build106.zip",
				Platform:      "windows / x64",
				Size:          "12 MiB",
				Sha256:        "abcd",
				Sha256Short:   "abcd…",
				Href:          "/artifact/svn-auto-merge/0.2.0+106/win.zip",
				PermanentHref: "/-/latest/svn-auto-merge/dev",
				Downloads:     8,
			}},
			Releases: []releaseRow{{
				Version:      "0.2.0+106",
				Code:         106,
				Released:     "2026-08-30",
				ManifestHref: "/manifest/svn-auto-merge/0.2.0+106.pb",
			}},
		}},
	}
}

func sampleListingPage() listingPage {
	return listingPage{
		pageChrome: pageChrome{Title: "files", Version: "preview"},
		Display:    "/browse/",
		Count:      3,
		Parent:     "/-/admin/files",
		Entries: []listingEntry{
			{Name: "index.html", Href: "/browse/index.html", Size: "3 KiB", Mtime: "2026-08-30"},
			{Name: "svn-auto-merge.html", Href: "/browse/svn-auto-merge.html", Size: "3 KiB", Mtime: "2026-08-30"},
			{Name: "catalog.json", Href: "/browse/catalog.json", Size: "1 KiB", Mtime: "2026-08-30"},
		},
	}
}
