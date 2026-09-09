// Package publishproto is the HTTP capability contract between a relkit
// publisher and the write endpoints it talks to (relkit-agent and
// relkit-serve). It is separate from RUP: update clients never use it.
package publishproto

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
)

const (
	Current = 2
	Min     = 2
	Max     = 2

	ProtocolHeader = "X-Relkit-Publish-Protocol"
	MinHeader      = "X-Relkit-Publish-Protocol-Min"
	MaxHeader      = "X-Relkit-Publish-Protocol-Max"
	VersionHeader  = "X-Relkit-Version"

	// PreflightPath is the serve (data-plane) preflight. Agent traffic goes
	// through nginx /v1/, so the agent listens on AgentPreflightPath instead.
	PreflightPath      = "/-/publish/preflight"
	AgentPreflightPath = "/v1/publish/preflight"

	ErrUpgradeRequired = "publisher_upgrade_required"
	ErrTooNew          = "publisher_too_new"
)

// PublisherVersion is set by cmd/relkit from its build-time version.
var PublisherVersion = "dev"

// ServerCommit is optional; when empty, Identity falls back to vcs.revision.
var ServerCommit = ""

type Window struct {
	Min int
	Max int
}

func DefaultWindow() Window {
	return Window{Min: Min, Max: Max}
}

func (w Window) Disabled() bool {
	return w.Min <= 0
}

func (w Window) Valid() bool {
	if w.Disabled() {
		return true
	}
	return w.Max >= w.Min && w.Min > 0
}

type Offer struct {
	Min              int
	Max              int
	Protocol         int
	PublisherVersion string
}

func Apply(h http.Header) {
	h.Set(ProtocolHeader, strconv.Itoa(Current))
	h.Set(MinHeader, strconv.Itoa(Min))
	h.Set(MaxHeader, strconv.Itoa(Max))
	h.Set(VersionHeader, PublisherVersion)
}

func Declared(h http.Header) int {
	return atoi(h.Get(ProtocolHeader))
}

func ParseOffer(h http.Header) Offer {
	protocol := Declared(h)
	min := atoi(h.Get(MinHeader))
	max := atoi(h.Get(MaxHeader))
	if min <= 0 && max <= 0 {
		min, max = protocol, protocol
	}
	if min <= 0 {
		min = protocol
	}
	if max <= 0 {
		max = protocol
	}
	return Offer{
		Min:              min,
		Max:              max,
		Protocol:         protocol,
		PublisherVersion: strings.TrimSpace(h.Get(VersionHeader)),
	}
}

type Decision struct {
	OK               bool   `json:"ok"`
	Selected         int    `json:"selected,omitempty"`
	Protocol         int    `json:"protocol,omitempty"`
	MinProtocol      int    `json:"minProtocol"`
	MaxProtocol      int    `json:"maxProtocol"`
	ServerVersion    string `json:"serverVersion,omitempty"`
	ServerCommit     string `json:"serverCommit,omitempty"`
	Error            string `json:"error,omitempty"`
	Message          string `json:"message,omitempty"`
	PublisherVersion string `json:"publisherVersion,omitempty"`
}

func Negotiate(server Window, offer Offer) Decision {
	decision := Decision{
		MinProtocol:      server.Min,
		MaxProtocol:      server.Max,
		PublisherVersion: offer.PublisherVersion,
		Protocol:         offer.Protocol,
	}
	if server.Disabled() {
		decision.OK = true
		decision.Selected = offer.Protocol
		decision.Protocol = offer.Protocol
		return decision
	}
	if !server.Valid() {
		decision.Error = ErrUpgradeRequired
		decision.Message = "server publish protocol window is invalid"
		return decision
	}
	low := max(server.Min, offer.Min)
	high := min(server.Max, offer.Max)
	if low > high {
		if offer.Max < server.Min {
			decision.Error = ErrUpgradeRequired
			decision.Message = "this relkit publisher is too old for the write contract; " +
				"rebuild it from the relkit revision this server was deployed from"
			return decision
		}
		decision.Error = ErrTooNew
		decision.Message = "this relkit publisher speaks a newer write contract than the server; " +
			"upgrade the agent/serve to the same release as the publisher"
		return decision
	}
	selected := Current
	if selected < low || selected > high {
		selected = high
	}
	decision.OK = true
	decision.Selected = selected
	decision.Protocol = selected
	return decision
}

func Check(w http.ResponseWriter, r *http.Request, server Window, serverVersion string) bool {
	if server.Disabled() {
		return true
	}
	decision := Negotiate(server, ParseOffer(r.Header))
	if decision.OK {
		return true
	}
	decision.ServerVersion = serverVersion
	decision.ServerCommit = IdentityCommit()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Upgrade", "relkit-publish/"+strconv.Itoa(server.Min))
	w.WriteHeader(http.StatusUpgradeRequired)
	_ = json.NewEncoder(w).Encode(decision)
	return false
}

func Success(server Window, selected int, serverVersion string) Decision {
	if selected <= 0 {
		selected = Current
	}
	return Decision{
		OK:            true,
		Selected:      selected,
		Protocol:      selected,
		MinProtocol:   server.Min,
		MaxProtocol:   server.Max,
		ServerVersion: serverVersion,
		ServerCommit:  IdentityCommit(),
	}
}

func Explain(status int, body []byte) error {
	var decision Decision
	_ = json.Unmarshal(body, &decision)
	if status != http.StatusUpgradeRequired && decision.Error == "" {
		return nil
	}
	switch decision.Error {
	case ErrTooNew:
		return fmt.Errorf(
			"publisher_too_new: server supports %d-%d, this publisher speaks %d-%d; upgrade the server",
			decision.MinProtocol, decision.MaxProtocol, decision.Protocol, decision.Protocol,
		)
	case ErrUpgradeRequired, "":
		need := decision.MinProtocol
		if need <= 0 {
			need = Current
		}
		return fmt.Errorf(
			"publisher_upgrade_required: server requires protocol %d (window %d-%d); rebuild this publisher from the server's release",
			need, decision.MinProtocol, decision.MaxProtocol,
		)
	default:
		return fmt.Errorf("%s: %s", decision.Error, strings.TrimSpace(decision.Message))
	}
}

func ReadDecision(resp *http.Response) (Decision, error) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var decision Decision
	_ = json.Unmarshal(body, &decision)
	if resp.StatusCode == http.StatusUpgradeRequired || decision.Error != "" {
		return decision, Explain(resp.StatusCode, body)
	}
	return decision, nil
}

func IdentityCommit() string {
	if strings.TrimSpace(ServerCommit) != "" {
		return strings.TrimSpace(ServerCommit)
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return ""
}

func Identity(version string) map[string]any {
	return map[string]any{
		"ok":          true,
		"version":     version,
		"commit":      IdentityCommit(),
		"minProtocol": Min,
		"maxProtocol": Max,
		"protocol":    Current,
	}
}

func PreflightAgent(ctx context.Context, client *http.Client, agentURL, token, product string) error {
	if client == nil {
		client = http.DefaultClient
	}
	body, err := json.Marshal(map[string]string{"product": product})
	if err != nil {
		return err
	}
	base := strings.TrimRight(strings.TrimSpace(agentURL), "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/publish/preflight", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	Apply(req.Header)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return nil
	}
	if resp.StatusCode == http.StatusUpgradeRequired {
		return Explain(resp.StatusCode, data)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := strings.TrimSpace(string(data))
		if detail == "" {
			detail = resp.Status
		}
		return fmt.Errorf("publish preflight HTTP %d: %s", resp.StatusCode, detail)
	}
	return nil
}

func atoi(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	return n
}
