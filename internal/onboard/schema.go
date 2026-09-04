package onboard

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/jsonio"
)

const (
	SchemaChecklist = "relkit.onboard-checklist/1"
	SchemaAcks      = "relkit.onboard-acks/1"
	SchemaReport    = "relkit.onboard-report/1"
	AcksFileName    = "onboard-acks.json"
	RecoveryJSON    = "recovery.json"
	RecoveryHTML    = "recovery.html"
)

type Status string

const (
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusBlocked Status = "blocked"
	StatusPending Status = "pending"
	StatusNA      Status = "not-applicable"
)

type ItemType string

const (
	TypeDetect ItemType = "detect"
	TypeExec   ItemType = "exec"
	TypeManual ItemType = "manual"
)

type Checklist struct {
	Schema string `json:"schema"`
	ID     string `json:"id"`
	Items  []Item `json:"items"`
}

type Item struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Type      ItemType `json:"type"`
	DependsOn []string `json:"dependsOn,omitempty"`
	Detect    *Detect  `json:"detect,omitempty"`
	Exec      *Exec    `json:"exec,omitempty"`
	Prompt    string   `json:"prompt,omitempty"`
}

type Detect struct {
	Kind string `json:"kind"`
}

type Exec struct {
	Kind string `json:"kind"`
}

type ItemResult struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        ItemType `json:"type"`
	Status      Status   `json:"status"`
	Evidence    string   `json:"evidence,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
	BlockedBy   []string `json:"blockedBy,omitempty"`
}

type Report struct {
	Schema     string       `json:"schema"`
	Checklist  string       `json:"checklist"`
	HostRoot   string       `json:"hostRoot"`
	ComputedAt string       `json:"computedAt"`
	Items      []ItemResult `json:"items"`
}

type AcksFile struct {
	Schema     string         `json:"schema"`
	InputsHash string         `json:"inputsHash"`
	Acks       map[string]Ack `json:"acks"`
}

type Ack struct {
	At   string `json:"at"`
	Note string `json:"note,omitempty"`
}

func DecodeChecklist(data []byte) (Checklist, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var c Checklist
	if err := dec.Decode(&c); err != nil {
		return Checklist{}, fmt.Errorf("checklist json: %w", err)
	}
	if err := ValidateChecklist(c); err != nil {
		return Checklist{}, err
	}
	return c, nil
}

func ValidateChecklist(c Checklist) error {
	if c.Schema != SchemaChecklist {
		return fmt.Errorf("checklist schema must be %s", SchemaChecklist)
	}
	if c.ID == "" {
		return fmt.Errorf("checklist id is required")
	}
	seen := map[string]bool{}
	for _, item := range c.Items {
		if item.ID == "" {
			return fmt.Errorf("item id is required")
		}
		if seen[item.ID] {
			return fmt.Errorf("duplicate item id %q", item.ID)
		}
		seen[item.ID] = true
		switch item.Type {
		case TypeDetect, TypeExec, TypeManual:
		default:
			return fmt.Errorf("item %q has unknown type %q", item.ID, item.Type)
		}
	}
	for _, item := range c.Items {
		for _, dep := range item.DependsOn {
			if !seen[dep] {
				return fmt.Errorf("item %q depends on unknown %q", item.ID, dep)
			}
		}
	}
	return detectCycle(c)
}

func detectCycle(c Checklist) error {
	byID := map[string]Item{}
	for _, item := range c.Items {
		byID[item.ID] = item
	}
	const visiting, visited = 1, 2
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == visited {
			return nil
		}
		if state[id] == visiting {
			return fmt.Errorf("checklist has a dependency cycle involving %q", id)
		}
		state[id] = visiting
		for _, dep := range byID[id].DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[id] = visited
		return nil
	}
	ids := make([]string, 0, len(c.Items))
	for _, item := range c.Items {
		ids = append(ids, item.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func topoOrder(c Checklist) []Item {
	byID := map[string]Item{}
	indeg := map[string]int{}
	for _, item := range c.Items {
		byID[item.ID] = item
		indeg[item.ID] = 0
	}
	for _, item := range c.Items {
		indeg[item.ID] = len(item.DependsOn)
	}
	var ready []string
	for id, n := range indeg {
		if n == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	var out []Item
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		out = append(out, byID[id])
		for _, item := range c.Items {
			for _, dep := range item.DependsOn {
				if dep == id {
					indeg[item.ID]--
					if indeg[item.ID] == 0 {
						ready = append(ready, item.ID)
						sort.Strings(ready)
					}
				}
			}
		}
	}
	return out
}

func RelkitDir(hostRoot string) string {
	return filepath.Join(hostRoot, ".relkit")
}

func AcksPath(hostRoot string) string {
	return filepath.Join(RelkitDir(hostRoot), AcksFileName)
}

func RecoveryJSONPath(hostRoot string) string {
	return filepath.Join(RelkitDir(hostRoot), RecoveryJSON)
}

func RecoveryHTMLPath(hostRoot string) string {
	return filepath.Join(RelkitDir(hostRoot), RecoveryHTML)
}

func InputsHash(cfg *config.Config) string {
	payload := struct {
		Product    string
		Recovery   *config.RecoveryConfig
		PublicKeys any
		EntryURLs  []string
	}{}
	if cfg != nil {
		payload.Product = cfg.Product
		payload.Recovery = cfg.Recovery
		payload.PublicKeys = cfg.Signing["publicKeys"]
		if cfg.Directory != nil {
			payload.EntryURLs = cfg.Directory.EntryURLs
		}
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func LoadAcks(hostRoot string) (*AcksFile, error) {
	path := AcksPath(hostRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AcksFile{Schema: SchemaAcks, Acks: map[string]Ack{}}, nil
		}
		return nil, err
	}
	var doc AcksFile
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Acks == nil {
		doc.Acks = map[string]Ack{}
	}
	return &doc, nil
}

func IsManualItem(c Checklist, id string) bool {
	for _, item := range c.Items {
		if item.ID == id {
			return item.Type == TypeManual
		}
	}
	return false
}

func SaveAck(hostRoot, itemID, note, inputsHash string) error {
	if err := os.MkdirAll(RelkitDir(hostRoot), 0o755); err != nil {
		return err
	}
	doc, err := LoadAcks(hostRoot)
	if err != nil {
		return err
	}
	if doc.InputsHash != "" && doc.InputsHash != inputsHash {
		doc.Acks = map[string]Ack{}
	}
	doc.Schema = SchemaAcks
	doc.InputsHash = inputsHash
	doc.Acks[itemID] = Ack{At: time.Now().UTC().Format(time.RFC3339), Note: note}
	raw, err := jsonio.MarshalPretty(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(AcksPath(hostRoot), raw, 0o644)
}

func WriteRecoveryEmbed(hostRoot string, rec *config.RecoveryConfig) error {
	if rec == nil {
		return fmt.Errorf("recovery is not configured")
	}
	if err := os.MkdirAll(RelkitDir(hostRoot), 0o755); err != nil {
		return err
	}
	payload := map[string]any{
		"schema":  "relkit.recovery/1",
		"message": rec.Message,
		"links":   rec.Links,
	}
	raw, err := jsonio.MarshalPretty(payload)
	if err != nil {
		return err
	}
	if err := os.WriteFile(RecoveryJSONPath(hostRoot), raw, 0o644); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><title>Update unavailable</title></head><body>\n")
	b.WriteString("<h1>Update unavailable</h1>\n<p>")
	b.WriteString(htmlEscape(rec.Message))
	b.WriteString("</p>\n<ul>\n")
	for _, link := range rec.Links {
		fmt.Fprintf(&b, "<li><a href=\"%s\">%s</a></li>\n", htmlEscape(link.URL), htmlEscape(link.Label))
	}
	b.WriteString("</ul>\n</body></html>\n")
	return os.WriteFile(RecoveryHTMLPath(hostRoot), []byte(b.String()), 0o644)
}

func RecoveryEmbedMatches(hostRoot string, rec *config.RecoveryConfig) (bool, string) {
	if rec == nil {
		return false, "recovery is not configured"
	}
	data, err := os.ReadFile(RecoveryJSONPath(hostRoot))
	if err != nil {
		return false, err.Error()
	}
	var got struct {
		Message string                `json:"message"`
		Links   []config.RecoveryLink `json:"links"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		return false, err.Error()
	}
	if got.Message != rec.Message || len(got.Links) != len(rec.Links) {
		return false, "embedded recovery.json does not match relkit.json"
	}
	for i := range rec.Links {
		if got.Links[i] != rec.Links[i] {
			return false, "embedded recovery.json does not match relkit.json"
		}
	}
	if _, err := os.Stat(RecoveryHTMLPath(hostRoot)); err != nil {
		return false, "recovery.html is missing"
	}
	return true, "embedded recovery matches relkit.json"
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
