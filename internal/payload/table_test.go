package payload

import (
	"strings"
	"testing"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
)

func TestTargetOS(t *testing.T) {
	cases := []struct {
		selectors map[string]string
		want      string
	}{
		{map[string]string{"os": "darwin"}, "darwin"},
		{map[string]string{"os-arch": "darwin-universal"}, "darwin"},
		{map[string]string{"os-arch": "windows-amd64"}, "windows"},
		{map[string]string{"os": "linux", "os-arch": "darwin-arm64"}, "linux"},
		{map[string]string{"os-arch": "darwin"}, ""},
		{map[string]string{}, ""},
	}
	for _, c := range cases {
		if got := TargetOS(c.selectors); got != c.want {
			t.Errorf("TargetOS(%v) = %q, want %q", c.selectors, got, c.want)
		}
	}
}

func TestCheckPublishSelectors(t *testing.T) {
	bundle := &enginestate.FileTable{
		Schema:     SchemaV2,
		Unit:       enginestate.ContentUnit_CONTENT_UNIT_BUNDLE,
		BundleName: "Demo.app",
	}
	symlinked := &enginestate.FileTable{
		Schema: SchemaV2,
		Files: []*enginestate.FileEntry{
			{Relpath: "lib/current", Kind: enginestate.EntryKind_ENTRY_KIND_SYMLINK, LinkTarget: "v1"},
		},
	}
	plain := &enginestate.FileTable{Schema: SchemaV1}
	cases := []struct {
		name      string
		table     *enginestate.FileTable
		selectors map[string]string
		wantErr   string
	}{
		{"bundle by os-arch", bundle, map[string]string{"apply": ApplySelectorV2, "os-arch": "darwin-universal"}, ""},
		{"bundle by os", bundle, map[string]string{"apply": ApplySelectorV2, "os": "darwin"}, ""},
		{"bundle on windows", bundle, map[string]string{"apply": ApplySelectorV2, "os-arch": "windows-amd64"}, "must target darwin"},
		{"bundle without target", bundle, map[string]string{"apply": ApplySelectorV2}, "must target darwin"},
		{"bundle wrong apply", bundle, map[string]string{"apply": ApplySelectorV1, "os": "darwin"}, "must carry selector"},
		{"disagreeing target", bundle, map[string]string{"apply": ApplySelectorV2, "os": "darwin", "os-arch": "windows-amd64"}, "disagree"},
		{"symlinks on windows", symlinked, map[string]string{"apply": ApplySelectorV2, "os-arch": "windows-amd64"}, "cannot target windows"},
		{"symlinks on darwin", symlinked, map[string]string{"apply": ApplySelectorV2, "os-arch": "darwin-arm64"}, ""},
		{"plain on windows", plain, map[string]string{"apply": ApplySelectorV1, "os-arch": "windows-amd64"}, ""},
	}
	for _, c := range cases {
		err := CheckPublishSelectors(c.table, c.selectors)
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: error %v, want containing %q", c.name, err, c.wantErr)
		}
	}
}
