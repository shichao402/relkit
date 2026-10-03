package browse

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/relkit/internal/webmeta"
)

// WriteSampleDump materializes the SPA shell, assets, and sample data
// documents so a human (or an agent) can open the site in a browser without
// publishing. Sample data mimics one product with two channels of history.
func WriteSampleDump(dir string) error {
	if dir == "" {
		return fmt.Errorf("output directory is required")
	}
	dump, err := Build(sampleProducts())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, body := range dump {
		rel := strings.TrimPrefix(name, "browse/")
		if rel == "" || rel == name {
			// Data documents (site/, latest/, channel/) keep their full key so
			// the sample tree mirrors a servable data plane exactly.
			rel = name
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func sampleProducts() []ProductData {
	artifacts := func(version string, code int64) []webmeta.Artifact {
		return []webmeta.Artifact{
			{ID: "win", Filename: "SvnAutoMerge_windows_" + version + "build" + fmtIt(code) + ".zip", Size: 12 << 20, Selectors: map[string]string{"os": "windows", "arch": "x64"}, URLs: []string{"https://update.example/artifact/win.zip"}},
			{ID: "mac", Filename: "SvnAutoMerge_macos_" + version + "build" + fmtIt(code) + ".zip", Size: 18 << 20, Selectors: map[string]string{"os": "macos"}, URLs: []string{"https://update.example/artifact/mac.zip"}},
		}
	}
	entry := func(version string, code int64, at string) webmeta.ChannelEntry {
		return webmeta.ChannelEntry{Version: version, Code: code, ReleasedAt: at}
	}
	release := func(channel, version string, code int64, at string) webmeta.Release {
		return webmeta.Release{
			Product: "svn-auto-merge", Channel: channel, Version: version,
			Code: code, ReleasedAt: at, Artifacts: artifacts(version, code),
		}
	}
	stableVersions := []webmeta.ChannelEntry{entry("0.1.0+98", 98, "2026-08-10T00:00:00Z"), entry("0.2.0+100", 100, "2026-08-20T00:00:00Z")}
	devVersions := append([]webmeta.ChannelEntry{entry("0.2.0+104", 104, "2026-08-27T00:00:00Z"), entry("0.2.0+106", 106, "2026-08-30T00:00:00Z")},
		stableVersions...)
	return []ProductData{{
		Site: &webmeta.Site{
			Product:     "svn-auto-merge",
			Title:       "SVN Auto Merge",
			Description: "OSGame 客户端团队的 SVN 合并工具。解压到任意目录即可运行。",
			Homepage:    "https://git.woa.com/osgame-client/SvnMergeTool",
			UpdatedAt:   "2026-08-30T00:00:00Z",
		},
		Latests: []webmeta.Latest{
			{Product: "svn-auto-merge", Channel: "stable", Version: "0.2.0+100", Code: 100, PublishedAt: "2026-08-20T00:00:00Z", Artifacts: artifacts("0.2.0", 100)},
			{Product: "svn-auto-merge", Channel: "dev", Version: "0.2.0+106", Code: 106, PublishedAt: "2026-08-30T00:00:00Z", Artifacts: artifacts("0.2.0", 106)},
		},
		Channels: []webmeta.Channel{
			{Product: "svn-auto-merge", Channel: "stable", UpdatedAt: "2026-08-20T00:00:00Z", Latest: entry("0.2.0+100", 100, "2026-08-20T00:00:00Z"), Versions: stableVersions},
			{Product: "svn-auto-merge", Channel: "dev", UpdatedAt: "2026-08-30T00:00:00Z", Latest: entry("0.2.0+106", 106, "2026-08-30T00:00:00Z"), Versions: devVersions},
		},
		Releases: []webmeta.Release{
			release("stable", "0.1.0+98", 98, "2026-08-10T00:00:00Z"),
			release("stable", "0.2.0+100", 100, "2026-08-20T00:00:00Z"),
			release("dev", "0.2.0+104", 104, "2026-08-27T00:00:00Z"),
			release("dev", "0.2.0+106", 106, "2026-08-30T00:00:00Z"),
		},
	}}
}

func fmtIt(n int64) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
