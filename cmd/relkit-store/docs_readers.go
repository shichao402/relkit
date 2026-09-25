package main

import (
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	rupv2 "go.firoyang.com/relkit/api/rup/v2"
)

// readIndexDoc and readManifestDoc are the storage-plane readers GC depends
// on. They are deliberately the same code console uses, so both binaries
// parse the tree identically.

func readIndexDoc(root *os.Root, name string) (*rupv2.Index, error) {
	raw, err := root.ReadFile(name)
	if err != nil {
		return nil, err
	}
	env, err := rupv2.UnmarshalEnvelope(raw)
	if err != nil {
		return nil, err
	}
	if env.Schema != rupv2.SchemaEnvelope {
		return nil, errUnexpectedSchema(env.Schema)
	}
	return rupv2.UnmarshalIndex(env.Payload)
}

func readManifestDoc(root *os.Root, name string) (*rupv2.Manifest, error) {
	raw, err := root.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return rupv2.UnmarshalManifest(raw)
}

type schemaError string

func (e schemaError) Error() string { return "unexpected envelope schema " + strconv.Quote(string(e)) }

func errUnexpectedSchema(schema string) error { return schemaError(schema) }

// channelName reports the channel a latest/ entry stands for. Only called by
// test fixtures in this package; kept beside the readers it shares helpers
// with.
func channelName(entry fs.DirEntry) (string, bool) {
	if entry.IsDir() {
		return "", false
	}
	name := entry.Name()
	if !strings.HasSuffix(name, ".json") {
		return "", false
	}
	return strings.TrimSuffix(name, ".json"), true
}

// humanBytes renders a byte count for operator-facing logs and listings.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", value)
}
