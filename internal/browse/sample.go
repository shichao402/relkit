package browse

import (
	"fmt"
	"os"
	"path/filepath"

	"cnb.cool/shichao402/relkit/internal/webmeta"
)

// WriteSampleDump renders the browse templates with fake catalog data so a
// human (or an agent) can open the HTML in a browser without publishing.
func WriteSampleDump(dir string) error {
	if dir == "" {
		return fmt.Errorf("output directory is required")
	}
	cat := sampleCatalog()
	indexHTML, err := RenderIndex(cat)
	if err != nil {
		return err
	}
	product := ProductPage(cat, "svn-auto-merge")
	if product == nil {
		return fmt.Errorf("sample catalog missing svn-auto-merge")
	}
	productHTML, err := RenderProduct(product)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	catalogJSON, err := MarshalCatalog(cat)
	if err != nil {
		return err
	}
	files := map[string][]byte{
		"index.html":          indexHTML,
		"svn-auto-merge.html": productHTML,
		"catalog.json":        catalogJSON,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func sampleCatalog() *Catalog {
	stable := ApplyPublish(nil, &webmeta.Site{
		Product:     "svn-auto-merge",
		Title:       "SVN Auto Merge",
		Description: "OSGame 客户端团队的 SVN 合并工具。解压到任意目录即可运行。",
		Homepage:    "https://git.woa.com/osgame-client/SvnMergeTool",
	}, webmeta.Latest{
		Product:     "svn-auto-merge",
		Channel:     "stable",
		Version:     "0.2.0+100",
		Code:        100,
		PublishedAt: "2026-08-20T00:00:00Z",
		Artifacts: []webmeta.Artifact{
			{ID: "win", Filename: "SvnAutoMerge_windows_0.2.0build100.zip", Size: 12 << 20, Selectors: map[string]string{"os": "windows", "arch": "x64"}, URLs: []string{"https://update.example/artifact/win.zip"}},
			{ID: "mac", Filename: "SvnAutoMerge_macos_0.2.0build100.zip", Size: 18 << 20, Selectors: map[string]string{"os": "macos"}, URLs: []string{"https://update.example/artifact/mac.zip"}},
		},
	}, "2026-08-20T00:00:00Z")
	return ApplyPublish(stable, &webmeta.Site{
		Product:     "svn-auto-merge",
		Title:       "SVN Auto Merge",
		Description: "OSGame 客户端团队的 SVN 合并工具。解压到任意目录即可运行。",
		Homepage:    "https://git.woa.com/osgame-client/SvnMergeTool",
	}, webmeta.Latest{
		Product:     "svn-auto-merge",
		Channel:     "dev",
		Version:     "0.2.0+106",
		Code:        106,
		PublishedAt: "2026-08-30T00:00:00Z",
		Artifacts: []webmeta.Artifact{
			{ID: "win", Filename: "SvnAutoMerge_windows_0.2.0build106.zip", Size: 12 << 20, Selectors: map[string]string{"os": "windows", "arch": "x64"}, URLs: []string{"https://update.example/artifact/win-dev.zip"}},
			{ID: "mac", Filename: "SvnAutoMerge_macos_0.2.0build106.zip", Size: 18 << 20, Selectors: map[string]string{"os": "macos"}, URLs: []string{"https://update.example/artifact/mac-dev.zip"}},
		},
	}, "2026-08-30T00:00:00Z")
}
