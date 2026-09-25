// Package site rebuilds the human-facing static site from authoritative
// data-plane documents. Product publishing never reads or writes rendered
// catalog files.
//
// Destinations are declarative (ADR 0015): relkit-agent.json site.sinks[]
// lists the sinks a dump is distributed to, and the dump always lands in the
// agent state directory first as the local audit/servable copy.
package site

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.firoyang.com/relkit/internal/backends"
	"go.firoyang.com/relkit/internal/browse"
	"go.firoyang.com/relkit/internal/config"
	"go.firoyang.com/relkit/internal/makers"
	"go.firoyang.com/relkit/internal/webmeta"
)

// Sink types (ADR 0015).
const (
	SinkMakers    = "makers"    // EdgeOne Pages project (legacy site.makers spelling)
	SinkBackend   = "backend"   // a backend named in a product publish profile
	SinkDirectory = "directory" // local dir served by an external static host
)

// SinkSpec declares one browse-dump destination in relkit-agent.json
// site.sinks[]. It never appears in a product relkit.json.
type SinkSpec struct {
	Type string `json:"type"`
	// SinkMakers fields.
	ProjectID string `json:"projectId,omitempty"`
	Region    string `json:"region,omitempty"`
	TokenEnv  string `json:"tokenEnv,omitempty"`
	// SinkBackend field: backend name as defined by a product profile.
	Backend string `json:"backend,omitempty"`
	// SinkDirectory field: absolute path the static host serves.
	Path string `json:"path,omitempty"`
}

type Config struct {
	Sinks    []SinkSpec
	StateDir string
}

type Product struct {
	ID      string
	Root    string
	Profile string
}

type Printer func(string)

var createBackend = backends.Create

// NormalizeSinks validates sink specs and applies defaults. It runs at agent
// config load; Rebuild runs it again defensively for direct callers.
func NormalizeSinks(specs []SinkSpec) ([]SinkSpec, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	out := make([]SinkSpec, 0, len(specs))
	seen := map[string]bool{}
	for i, spec := range specs {
		switch spec.Type {
		case SinkMakers:
			if strings.TrimSpace(spec.ProjectID) == "" {
				return nil, fmt.Errorf("site.sinks[%d].projectId is required", i)
			}
			switch spec.Region {
			case "", "china", "global":
			default:
				return nil, fmt.Errorf(`site.sinks[%d].region must be "china" or "global"`, i)
			}
			if spec.TokenEnv == "" {
				spec.TokenEnv = makers.DefaultTokenEnv
			}
			if spec.Region == "" {
				spec.Region = "china"
			}
		case SinkBackend:
			if strings.TrimSpace(spec.Backend) == "" {
				return nil, fmt.Errorf("site.sinks[%d].backend is required", i)
			}
		case SinkDirectory:
			if strings.TrimSpace(spec.Path) == "" {
				return nil, fmt.Errorf("site.sinks[%d].path is required", i)
			}
			if !filepath.IsAbs(spec.Path) {
				return nil, fmt.Errorf("site.sinks[%d].path must be absolute", i)
			}
			spec.Path = filepath.Clean(spec.Path)
		default:
			return nil, fmt.Errorf("site.sinks[%d].type must be one of makers, backend, directory", i)
		}
		name := sinkSpecName(spec)
		if seen[name] {
			return nil, fmt.Errorf("site.sinks[%d] duplicates sink %q", i, name)
		}
		seen[name] = true
		out = append(out, spec)
	}
	return out, nil
}

func sinkSpecName(spec SinkSpec) string {
	switch spec.Type {
	case SinkMakers:
		return "makers:" + spec.ProjectID
	case SinkBackend:
		return "backend:" + spec.Backend
	case SinkDirectory:
		return "directory:" + spec.Path
	}
	return spec.Type
}

// DeployResult reports what one sink deployment produced. The zero value is
// valid: sinks without a remote deployment identity (directory, backend)
// report nothing beyond success.
type DeployResult struct {
	DeploymentID string // remote deployment id when the sink has one (makers)
}

type sink interface {
	Name() string
	Deploy(map[string][]byte) (*DeployResult, error)
}

type backendSink struct {
	name    string
	backend backends.Backend
}

func (s backendSink) Name() string { return s.name }
func (s backendSink) Deploy(dump map[string][]byte) (*DeployResult, error) {
	keys := make([]string, 0, len(dump))
	for key := range dump {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := s.backend.PutPointer(dump[key], key); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
	}
	return &DeployResult{}, nil
}

type makersSink struct{ cfg *makers.Config }

func (s makersSink) Name() string { return "makers:" + s.cfg.ProjectID }
func (s makersSink) Deploy(dump map[string][]byte) (*DeployResult, error) {
	result, err := makers.DeployDump(dump, s.cfg)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return &DeployResult{}, nil
	}
	return &DeployResult{DeploymentID: result.DeploymentID}, nil
}

type directorySink struct{ dir string }

func (s directorySink) Name() string { return "directory:" + s.dir }
func (s directorySink) Deploy(dump map[string][]byte) (*DeployResult, error) {
	return &DeployResult{}, writeDumpDir(s.dir, dump)
}

// Rebuild collects every configured product and deploys one complete dump.
// Sink deployment failure is an error but never blocks protocol publishing:
// the caller (agent) runs this outside the publish path, and a failed pass
// does not record dump.sha256, so the next run is a full redeploy.
func Rebuild(siteCfg Config, products []Product, printer Printer) (bool, error) {
	if printer == nil {
		printer = func(string) {}
	}
	sinks, err := NormalizeSinks(siteCfg.Sinks)
	if err != nil {
		return false, err
	}
	inputs, backendsByName, err := collect(products, printer)
	if err != nil {
		return false, err
	}
	if len(inputs) == 0 {
		return false, fmt.Errorf("site rebuild found no published product data")
	}
	if len(sinks) == 0 {
		return false, fmt.Errorf("site rebuild has no destination (configure site.sinks in relkit-agent.json)")
	}
	dests, err := resolveSinks(sinks, backendsByName)
	if err != nil {
		return false, err
	}
	status := Status{At: time.Now().UTC().Format(time.RFC3339)}
	memberPath := filepath.Join(siteCfg.StateDir, "site", "members.json")
	members := inputMembers(inputs)
	if err := guardCompleteSnapshot(memberPath, products, members); err != nil {
		return false, err
	}
	dump, err := browse.Build(inputs)
	if err != nil {
		return false, err
	}
	// The state copy is written first and unconditionally (ADR 0015): it is
	// the audit/rollback reference and a servable document root, so a sink
	// failure never destroys the only local dump.
	if err := writeDumpDir(filepath.Join(siteCfg.StateDir, "site", "dump"), dump); err != nil {
		return false, err
	}
	sum := hashDump(dump, dests)
	statePath := filepath.Join(siteCfg.StateDir, "site", "dump.sha256")
	if previous, err := os.ReadFile(statePath); err == nil && strings.TrimSpace(string(previous)) == sum {
		if err := writeMembers(memberPath, members); err != nil {
			return false, err
		}
		printer("site rebuild: unchanged; deployment skipped")
		return false, nil
	}
	for _, dest := range dests {
		printer("site rebuild: deploying " + dest.Name())
		result, err := dest.Deploy(dump)
		if err != nil {
			return false, fmt.Errorf("%s: %w", dest.Name(), err)
		}
		record := sinkStatus{Name: dest.Name(), OK: true}
		if result != nil {
			record.DeploymentID = result.DeploymentID
		}
		status.Sinks = append(status.Sinks, record)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(statePath, []byte(sum+"\n"), 0o644); err != nil {
		return false, err
	}
	if err := writeStatus(filepath.Join(siteCfg.StateDir, "site", "status.json"), status); err != nil {
		return false, err
	}
	if err := writeMembers(memberPath, members); err != nil {
		return false, err
	}
	return true, nil
}

// resolveSinks turns validated specs into live sinks. A backend sink must
// name a backend that product profiles actually define, and every product
// defining that name must agree on the data plane it points at.
func resolveSinks(specs []SinkSpec, backendsByName map[string][]backends.Backend) ([]sink, error) {
	dests := make([]sink, 0, len(specs))
	for _, spec := range specs {
		switch spec.Type {
		case SinkMakers:
			dests = append(dests, makersSink{cfg: &makers.Config{
				ProjectID: spec.ProjectID, TokenEnv: spec.TokenEnv, Region: spec.Region,
			}})
		case SinkBackend:
			instances := backendsByName[spec.Backend]
			if len(instances) == 0 {
				return nil, fmt.Errorf("site sink backend %q is not defined by any product profile", spec.Backend)
			}
			for _, backend := range instances[1:] {
				if backend.Type() != instances[0].Type() || backend.Describe() != instances[0].Describe() {
					return nil, fmt.Errorf("site sink backend %q is ambiguous: products define it as different data planes", spec.Backend)
				}
			}
			if !instances[0].HostsBrowse() {
				return nil, fmt.Errorf("site sink backend %q (%s) cannot serve the browse dump; only HostsBrowse backends may receive HTML", spec.Backend, instances[0].Type())
			}
			dests = append(dests, backendSink{name: "backend:" + spec.Backend, backend: instances[0]})
		case SinkDirectory:
			dests = append(dests, directorySink{dir: spec.Path})
		default:
			return nil, fmt.Errorf("site sink type %q is not supported", spec.Type)
		}
	}
	return dests, nil
}

// Status is the deploy-event snapshot written after a successful rebuild. It
// records what each sink did; dump.sha256 remains the content fingerprint.
// An unchanged rebuild deploys nothing and rewrites nothing here.
type Status struct {
	At    string       `json:"at"`    // when this rebuild deployed
	Sinks []sinkStatus `json:"sinks"` // one record per sink, in deploy order
}

type sinkStatus struct {
	Name         string `json:"name"`
	OK           bool   `json:"ok"`
	DeploymentID string `json:"deploymentId,omitempty"` // makers deployments carry one
}

func writeStatus(path string, status Status) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ReadStatus loads the deploy-event snapshot for display (agent status
// endpoint, console panels). A missing snapshot is not an error: it means no
// rebuild has deployed since the feature landed.
func ReadStatus(stateDir string) (Status, bool) {
	data, err := os.ReadFile(filepath.Join(stateDir, "site", "status.json"))
	if err != nil {
		return Status{}, false
	}
	var status Status
	if json.Unmarshal(data, &status) != nil {
		return Status{}, false
	}
	return status, true
}

func inputMembers(inputs []browse.ProductData) []string {
	members := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if input.Site != nil {
			members = append(members, input.Site.Product)
		}
	}
	sort.Strings(members)
	return members
}

func guardCompleteSnapshot(path string, products []Product, current []string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var previous []string
	if err := json.Unmarshal(data, &previous); err != nil {
		return fmt.Errorf("site membership state: %w", err)
	}
	configured := map[string]bool{}
	for _, product := range products {
		configured[product.ID] = true
	}
	present := map[string]bool{}
	for _, product := range current {
		present[product] = true
	}
	var missing []string
	for _, product := range previous {
		if configured[product] && !present[product] {
			missing = append(missing, product)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("site rebuild refused incomplete snapshot; previously published products missing data: %s", strings.Join(missing, ", "))
	}
	return nil
}

func writeMembers(path string, members []string) error {
	data, err := json.MarshalIndent(members, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// collect reads every product's site/latest documents from its publish
// backends. Backends are also indexed by profile name so declarative backend
// sinks can be resolved; sinks are no longer derived from HostsBrowse.
func collect(products []Product, printer Printer) ([]browse.ProductData, map[string][]backends.Backend, error) {
	sorted := append([]Product(nil), products...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	var inputs []browse.ProductData
	backendsByName := map[string][]backends.Backend{}
	for _, product := range sorted {
		profile, err := config.LoadPublishProfile(product.Profile)
		if err != nil {
			return nil, nil, fmt.Errorf("%s profile: %w", product.ID, err)
		}
		cfg := &config.Config{
			Root: product.Root, Product: product.ID,
			Backends: profile.Backends, PublishTo: profile.PublishTo,
		}
		var sources []backends.Backend
		for _, name := range profile.PublishTo {
			backend, err := createBackend(name, cfg, product.Root)
			if err != nil {
				return nil, nil, fmt.Errorf("%s backend %s: %w", product.ID, name, err)
			}
			sources = append(sources, backend)
			backendsByName[name] = append(backendsByName[name], backend)
		}
		siteRaw, err := firstGet(sources, webmeta.SiteKey(product.ID))
		if err != nil {
			return nil, nil, fmt.Errorf("%s site: %w", product.ID, err)
		}
		if len(siteRaw) == 0 {
			printer("site rebuild: skip " + product.ID + " (no site document)")
			continue
		}
		siteDoc, err := webmeta.UnmarshalSite(siteRaw)
		if err != nil {
			return nil, nil, fmt.Errorf("%s site: %w", product.ID, err)
		}
		if siteDoc.Product != product.ID {
			return nil, nil, fmt.Errorf("%s site document names product %q", product.ID, siteDoc.Product)
		}
		channels := siteDoc.Channels
		if len(channels) == 0 {
			// Compatibility with relkit.site/1 writers before channels became
			// part of the document. New publishes always provide the exact
			// list; old public products used these conventional channels.
			channels = []string{"stable", "beta", "dev"}
		}
		input := browse.ProductData{Site: siteDoc}
		for _, channel := range channels {
			raw, err := firstGet(sources, webmeta.LatestKey(product.ID, channel))
			if err != nil {
				return nil, nil, fmt.Errorf("%s latest/%s: %w", product.ID, channel, err)
			}
			if len(raw) == 0 {
				printer(fmt.Sprintf("site rebuild: %s has no latest/%s", product.ID, channel))
				continue
			}
			doc, err := webmeta.UnmarshalLatest(raw)
			if err != nil {
				return nil, nil, fmt.Errorf("%s latest/%s: %w", product.ID, channel, err)
			}
			if doc.Product != product.ID || doc.Channel != channel {
				return nil, nil, fmt.Errorf("%s latest/%s document identity mismatch", product.ID, channel)
			}
			input.Latests = append(input.Latests, *doc)
		}
		if len(input.Latests) > 0 {
			inputs = append(inputs, input)
		}
	}
	return inputs, backendsByName, nil
}

func firstGet(sources []backends.Backend, key string) ([]byte, error) {
	var failures []string
	for _, source := range sources {
		data, err := source.Get(key)
		if err == nil && len(data) > 0 {
			return data, nil
		}
		if err != nil {
			failures = append(failures, source.Name()+": "+err.Error())
		}
	}
	if len(failures) == len(sources) && len(failures) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil, nil
}

// writeDumpDir atomically replaces dir with the dump rendered as a servable
// document root: the browse/ prefix is stripped so index.html sits at the
// root an external static host (nginx/Caddy) points at. The swap is
// write-temp-then-rename; a failed write never touches the previous tree.
func writeDumpDir(dir string, dump map[string][]byte) error {
	dir = filepath.Clean(dir)
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(dir)+".tmp-*")
	if err != nil {
		return err
	}
	// MkdirTemp yields 0700; the dump tree is served by an unprivileged
	// static host (nginx/Caddy), so open the tree to world-readable.
	if err := os.Chmod(tmp, 0o755); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	abort := func(err error) error {
		_ = os.RemoveAll(tmp)
		return err
	}
	names := make([]string, 0, len(dump))
	for name := range dump {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rel, err := dumpFileRel(name)
		if err != nil {
			return abort(err)
		}
		target := filepath.Join(tmp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return abort(err)
		}
		if err := os.WriteFile(target, dump[name], 0o644); err != nil {
			return abort(err)
		}
	}
	previous := dir + ".previous"
	_ = os.RemoveAll(previous)
	if err := os.Rename(dir, previous); err != nil && !os.IsNotExist(err) {
		return abort(err)
	}
	if err := os.Rename(tmp, dir); err != nil {
		if _, statErr := os.Stat(previous); statErr == nil {
			_ = os.Rename(previous, dir) // put the last good tree back
		}
		return abort(err)
	}
	_ = os.RemoveAll(previous)
	return nil
}

func dumpFileRel(name string) (string, error) {
	rel := strings.TrimPrefix(path.Clean(name), "browse/")
	if rel == "" || rel == "." || rel == ".." || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("dump key %q is not a servable file name", name)
	}
	return rel, nil
}

func hashDump(dump map[string][]byte, sinks []sink) string {
	keys := make([]string, 0, len(dump))
	for key := range dump {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	names := make([]string, 0, len(sinks))
	for _, dest := range sinks {
		names = append(names, dest.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		hash.Write([]byte("sink:" + name))
		hash.Write([]byte{0})
	}
	for _, key := range keys {
		hash.Write([]byte(key))
		hash.Write([]byte{0})
		hash.Write(dump[key])
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
