// Command preview writes sample browse dump HTML for local inspection.
//
//	go run ./internal/browse/preview -out /tmp/browse-preview
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"cnb.cool/shichao402/relkit/internal/browse"
)

func main() {
	out := flag.String("out", "preview-out", "directory to write sample index.html / product HTML")
	flag.Parse()
	if err := browse.WriteSampleDump(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	abs, err := filepath.Abs(*out)
	if err != nil {
		abs = *out
	}
	fmt.Println(abs)
}
