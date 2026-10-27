package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/httpx"
)

// Adapter is the console's single read path into storage facts. Every panel
// page renders from an adapter; the console never touches a store's files or
// a backend client directly. Writes are not part of this surface: publishing,
// token rotation and GC remain agent/store jobs, which is what keeps the
// console from becoming a second publish entry.
//
// The shape deliberately mirrors internal/backends: one small interface, one
// factory that picks an implementation by type, no type-switches at call
// sites. Nothing outside this file names an implementation or reads its
// parameters; the rest of the console, the config file's top level and the
// deploy scripts only ever see an AdapterSpec.
type Adapter interface {
	// Name identifies the adapter in logs and on the panel.
	Name() string
	// ReadKey reads one RUP key (index/, manifest/, latest/, site/...).
	ReadKey(key string) ([]byte, error)
	// ReadDir lists one directory of the key space.
	ReadDir(dir string) ([]Entry, error)
	// ModTime reports when a key last changed, for freshness columns.
	// Implementations without that fact return the zero time.
	ModTime(key string) time.Time
}

// Entry is one directory listing row.
type Entry struct {
	Name  string
	IsDir bool
	Size  int64
	Mtime time.Time
}

// AdapterSpec is the abstract description of where the panel reads the tree:
// a type plus whatever parameters that type defines. It is carried around as
// raw JSON so that callers can store, copy and validate it without knowing
// any implementation's fields. Only the constructor registered for Type ever
// decodes the parameters.
type AdapterSpec json.RawMessage

// MarshalJSON and UnmarshalJSON keep the spec byte-for-byte as written, so a
// config round-trips without the console reshaping an implementation's block.
func (s AdapterSpec) MarshalJSON() ([]byte, error) {
	if len(s) == 0 {
		return []byte("null"), nil
	}
	return []byte(s), nil
}

func (s *AdapterSpec) UnmarshalJSON(raw []byte) error {
	*s = append((*s)[:0], raw...)
	return nil
}

// IsZero reports an absent spec (missing key or JSON null).
func (s AdapterSpec) IsZero() bool {
	trimmed := bytes.TrimSpace(s)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

// The type names are shared with internal/backends on purpose: a console
// adapter reads what the storage backend of the same name writes, so one
// protocol has one name across both seams. An s3-compatible reader would be
// registered here under "s3-compatible".
var adapterConstructors = map[string]func(params json.RawMessage) (Adapter, error){
	"relkit-compatible": newRelkitCompatibleAdapter,
}

// AdapterTypes lists the types newAdapter accepts, sorted for stable errors.
func AdapterTypes() []string {
	types := make([]string, 0, len(adapterConstructors))
	for name := range adapterConstructors {
		types = append(types, name)
	}
	sort.Strings(types)
	return types
}

// newAdapter is the only way the console obtains an Adapter.
func newAdapter(spec AdapterSpec) (Adapter, error) {
	available := strings.Join(AdapterTypes(), ", ")
	if spec.IsZero() {
		return nil, fmt.Errorf("no tree adapter configured: set \"tree\" to {\"type\": ...} (available types: %s)", available)
	}
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(spec, &head); err != nil {
		return nil, fmt.Errorf("tree adapter: %w", err)
	}
	if strings.TrimSpace(head.Type) == "" {
		return nil, fmt.Errorf("tree adapter has no \"type\" (available types: %s)", available)
	}
	construct, ok := adapterConstructors[head.Type]
	if !ok {
		return nil, fmt.Errorf("unknown tree adapter type %q (available types: %s)", head.Type, available)
	}
	adapter, err := construct(json.RawMessage(spec))
	if err != nil {
		return nil, fmt.Errorf("tree adapter %s: %w", head.Type, err)
	}
	return adapter, nil
}

// decodeAdapterParams decodes one implementation's parameters strictly, so a
// typo in a spec is a startup error rather than a silently ignored setting.
// "type" is the factory's key and is accepted by every implementation.
func decodeAdapterParams(params json.RawMessage, into any) error {
	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	return dec.Decode(into)
}

// --- relkit-compatible -------------------------------------------------------

// relkitEntryJSON mirrors relkit-store's /-/list/ row shape. Duplicating four
// fields beats importing the store package: the console must not depend on
// the storage plane's binary, only on its wire format (ADR 0016).
type relkitEntryJSON struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
	Mtime string `json:"mtime"`
}

// relkitCompatibleAdapter reads a relkit-compatible tree over HTTP. It is the
// read half of the wire the backends package's relkit-compatible client
// speaks: plain GET on the tree, plus the /-/list/ JSON endpoint for
// directory listings, which carries name/isDir/size/mtime per row.
//
// There is no "same box, read the disk" variant on purpose: a shortcut that
// only exists where the console happens to sit next to the tree makes two
// boxes that look alike behave differently, and lets the panel see files the
// store would never serve.
//
// ReadKey turns httpx.Get's "404 is (nil, nil)" into "missing key is an
// error": scanProducts and friends treat an error as "no data", and a nil body
// would decode as garbage.
type relkitCompatibleAdapter struct {
	name    string
	baseURL string // absolute, no trailing slash
}

func newRelkitCompatibleAdapter(params json.RawMessage) (Adapter, error) {
	var cfg struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	if err := decodeAdapterParams(params, &cfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("\"url\" is required")
	}
	parsed, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("url must be http(s), got %q", cfg.URL)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("url is missing a host: %q", cfg.URL)
	}
	return &relkitCompatibleAdapter{
		name:    "relkit-compatible:" + parsed.Host,
		baseURL: strings.TrimRight(parsed.String(), "/"),
	}, nil
}

func (a *relkitCompatibleAdapter) Name() string { return a.name }

// keyNoCache mirrors the store's no-cache prefixes (and the relkit-compatible
// client's Get): pointers and indexes change under the console between
// renders, while artifacts under a versioned prefix are immutable.
func (a *relkitCompatibleAdapter) keyNoCache(key string) bool {
	for _, prefix := range []string{"index/", "fallback/", "directory/", "latest/", "site/", "browse/"} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (a *relkitCompatibleAdapter) ReadKey(key string) ([]byte, error) {
	body, err := httpx.Get(a.baseURL+"/"+key, 60*time.Second, a.keyNoCache(key))
	if err != nil {
		return nil, err
	}
	if body == nil {
		// 404: a missing key is an error, never a nil body.
		return nil, fmt.Errorf("GET %s: not found", key)
	}
	return body, nil
}

func (a *relkitCompatibleAdapter) ReadDir(dir string) ([]Entry, error) {
	target := a.baseURL + "/-/list/" + strings.TrimPrefix(dir, "./")
	body, err := httpx.Get(target, 60*time.Second, true)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, fmt.Errorf("list %s: not found", dir)
	}
	var rows []relkitEntryJSON
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("list %s: decode: %w", dir, err)
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		entry := Entry{Name: row.Name, IsDir: row.IsDir, Size: row.Size}
		if row.Mtime != "" {
			if parsed, err := time.Parse(time.RFC3339, row.Mtime); err == nil {
				entry.Mtime = parsed
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (a *relkitCompatibleAdapter) ModTime(key string) time.Time {
	// The listing rows carry mtime; a one-row directory query is the cheapest
	// authoritative source. Anything unparsable degrades to the zero time,
	// which the panel renders as "no freshness shown".
	dir, base := path.Split(key)
	entries, err := a.ReadDir(strings.TrimSuffix(dir, "/"))
	if err != nil {
		return time.Time{}
	}
	for _, entry := range entries {
		if entry.Name == base {
			return entry.Mtime
		}
	}
	return time.Time{}
}

var _ Adapter = (*relkitCompatibleAdapter)(nil)
