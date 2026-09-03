package main

import "testing"

func TestParseSizeGiBNotBytes(t *testing.T) {
	// The old map-range parser returned 8 for "8GiB" whenever suffix "b"
	// won the iteration. Repeat so a regression cannot hide behind luck.
	for i := 0; i < 100; i++ {
		got, err := parseSize("8GiB")
		if err != nil {
			t.Fatalf("parseSize(8GiB): %v", err)
		}
		if got != 8<<30 {
			t.Fatalf("parseSize(8GiB) = %d, want %d", got, 8<<30)
		}
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1024", 1024},
		{"8b", 8},
		{"8B", 8},
		{"512MiB", 512 << 20},
		{"4GiB", 4 << 30},
		{"8GiB", 8 << 30},
		{"100MB", 100 * 1000 * 1000},
		{"2g", 2 << 30},
		{" 8MiB ", 8 << 20},
	}
	for _, c := range cases {
		got, err := parseSize(c.in)
		if err != nil {
			t.Errorf("parseSize(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
	for _, bad := range []string{"", "abc", "-1", "12XiB", "GiB"} {
		if _, err := parseSize(bad); err == nil {
			t.Errorf("parseSize(%q) should fail", bad)
		}
	}
}
