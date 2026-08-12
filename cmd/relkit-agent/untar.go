package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func extractTarGz(archivePath, dest string, maxFiles int) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	count := 0
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		count++
		if count > maxFiles {
			return fmt.Errorf("too many files (>%d)", maxFiles)
		}
		// Windows tar often emits "./" as a directory entry; skip the root, keep children.
		name := filepath.Clean(strings.TrimPrefix(filepath.ToSlash(hdr.Name), "./"))
		if name == "." || name == "" {
			if hdr.Typeflag == tar.TypeDir {
				continue
			}
			return fmt.Errorf("invalid path %q", hdr.Name)
		}
		if name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || strings.HasPrefix(name, "../") {
			return fmt.Errorf("invalid path %q", hdr.Name)
		}
		if filepath.IsAbs(name) {
			return fmt.Errorf("absolute path refused: %q", hdr.Name)
		}
		target := filepath.Join(destAbs, filepath.FromSlash(name))
		rel, err := filepath.Rel(destAbs, target)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("path escapes destination: %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode().Perm())
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			// skip other types
		}
	}
	return nil
}
