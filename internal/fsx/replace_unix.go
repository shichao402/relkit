//go:build !windows

package fsx

import "os"

func Replace(source, destination string) error {
	return os.Rename(source, destination)
}

func CommitNew(source, destination string) error {
	return os.Link(source, destination)
}
