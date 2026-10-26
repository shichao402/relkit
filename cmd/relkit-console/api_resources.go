package main

// Resource endpoints of the panel JSON API: products, tokens, users, roles,
// files, settings.
//
// Authority stays with the agent (requirement 13.6). Each write here forwards
// to /v1/admin/* with the operator's own bearer and relays the answer; the
// console never pre-checks a permission, because a client that mirrors the
// server's rules drifts from them. What this file adds is shape: camelCase JSON,
// the published-tree facts (channels, artifacts, download counts) merged with the
// agent's registrations, and the structured error envelope.
//
// A plaintext token appears in exactly one place: the response to the call that
// issued it. api.go marks every API answer Cache-Control: no-store, and nothing
// below stores, logs or re-reads it.

import (
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/shichao402/relkit/internal/agentadmin"
)

type apiResource func(w http.ResponseWriter, r *http.Request, agent *agentClient, session string)

func (c *console) registerResourceAPI(mux *http.ServeMux) {
	wrap := func(fn apiResource) http.HandlerFunc {
		return c.requireAPIAuth(func(w http.ResponseWriter, r *http.Request, session signedSession) {
			fn(w, r, c.admin.agentClient(), session.Agent)
		})
	}
	mux.HandleFunc(apiPrefix+"settings", wrap(c.apiSettings))
	mux.HandleFunc(apiPrefix+"products", wrap(c.apiProducts))
	mux.HandleFunc(apiPrefix+"products/{id}", wrap(c.apiProduct))
	mux.HandleFunc(apiPrefix+"products/{id}/tokens", wrap(c.apiProductTokens))
	mux.HandleFunc(apiPrefix+"tokens/{id}/{action}", wrap(c.apiTokenAction))
	mux.HandleFunc(apiPrefix+"users", wrap(c.apiUsers))
	mux.HandleFunc(apiPrefix+"users/{name}", wrap(c.apiUser))
	mux.HandleFunc(apiPrefix+"users/{name}/{action}", wrap(c.apiUserAction))
	mux.HandleFunc(apiPrefix+"roles", wrap(c.apiRoles))
	mux.HandleFunc(apiPrefix+"files", wrap(c.apiFiles))
}

// ---- views -----------------------------------------------------------------

type apiChannelSummary struct {
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
	Code     int64  `json:"code,omitempty"`
	Released string `json:"released,omitempty"`
	Yanked   bool   `json:"yanked,omitempty"`
	Error    string `json:"error,omitempty"`
}

// apiProductSummary is one row of the product list: what the agent registered
// and what the tree actually publishes. The two can differ (a registration with
// nothing published yet, or a tree left behind after a registration was
// dropped), and showing both is what lets an operator notice.
type apiProductSummary struct {
	ID            string              `json:"id"`
	Display       string              `json:"display"`
	Description   string              `json:"description,omitempty"`
	Homepage      string              `json:"homepage,omitempty"`
	Registered    bool                `json:"registered"`
	Published     bool                `json:"published"`
	Root          string              `json:"root,omitempty"`
	Profile       string              `json:"profile,omitempty"`
	CreatedAt     string              `json:"createdAt,omitempty"`
	Updated       string              `json:"updated,omitempty"`
	Downloads     int64               `json:"downloads"`
	Channels      []apiChannelSummary `json:"channels"`
	ConnectConfig map[string]any      `json:"connectConfig,omitempty"`
}

type apiArtifact struct {
	ID            string `json:"id"`
	Filename      string `json:"filename"`
	Platform      string `json:"platform"`
	OS            string `json:"os,omitempty"`
	Arch          string `json:"arch,omitempty"`
	Size          string `json:"size"`
	Sha256        string `json:"sha256"`
	Href          string `json:"href,omitempty"`
	PermanentHref string `json:"permanentHref,omitempty"`
	Downloads     int64  `json:"downloads"`
}

type apiRelease struct {
	Version      string `json:"version"`
	Code         int64  `json:"code"`
	Released     string `json:"released,omitempty"`
	Yanked       bool   `json:"yanked,omitempty"`
	ManifestHref string `json:"manifestHref,omitempty"`
	NotesURL     string `json:"notesUrl,omitempty"`
}

type apiChannelDetail struct {
	Name           string        `json:"name"`
	Sequence       int64         `json:"sequence"`
	Generated      string        `json:"generated,omitempty"`
	IndexHref      string        `json:"indexHref,omitempty"`
	LatestVersion  string        `json:"latestVersion,omitempty"`
	LatestCode     int64         `json:"latestCode,omitempty"`
	LatestReleased string        `json:"latestReleased,omitempty"`
	LatestNotes    string        `json:"latestNotes,omitempty"`
	Artifacts      []apiArtifact `json:"artifacts"`
	Releases       []apiRelease  `json:"releases"`
	Truncated      int           `json:"truncated,omitempty"`
	Error          string        `json:"error,omitempty"`
}

type apiProductDetail struct {
	apiProductSummary
	StatsSince string             `json:"statsSince,omitempty"`
	Detail     []apiChannelDetail `json:"detail"`
}

type apiRoleView struct {
	Name    string `json:"name"`
	Members int    `json:"members"`
}

type apiFileEntry struct {
	Name  string `json:"name"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`
	Mtime string `json:"mtime,omitempty"`
}

func isAdminRole(user agentUser) bool { return user.Role == string(agentadmin.RoleAdmin) }

// ---- settings --------------------------------------------------------------

func (c *console) apiSettings(w http.ResponseWriter, r *http.Request, agent *agentClient, session string) {
	if !apiMethods(w, r, http.MethodGet) {
		return
	}
	settings, err := agent.Settings(session)
	if err != nil {
		writeAgentFailure(w, err)
		return
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"ok": true, "settings": settings})
}

// ---- products --------------------------------------------------------------

// mergedProducts joins the agent's registrations with the published tree. An
// owner sees only the products the agent lists for them; an admin also sees
// trees with no registration, which is how a leftover is noticed.
func (c *console) mergedProducts(agent *agentClient, session string, self agentUser) ([]apiProductSummary, error) {
	views, err := agent.Products(session)
	if err != nil {
		return nil, err
	}
	byID := map[string]*apiProductSummary{}
	var order []string
	for _, view := range views {
		id := view.Product.ID
		byID[id] = &apiProductSummary{
			ID:            id,
			Display:       id,
			Registered:    true,
			Root:          view.Product.Root,
			Profile:       view.Product.Profile,
			CreatedAt:     view.Product.CreatedAt,
			Channels:      []apiChannelSummary{},
			ConnectConfig: view.ConnectConfig,
		}
		order = append(order, id)
	}
	for _, card := range c.scanProducts() {
		id := path.Base(card.Href)
		if id == "." || id == "/" {
			continue
		}
		id = unescapeSegment(id)
		row, ok := byID[id]
		if !ok {
			if !isAdminRole(self) {
				continue
			}
			row = &apiProductSummary{ID: id, Channels: []apiChannelSummary{}}
			byID[id] = row
			order = append(order, id)
		}
		row.Published = true
		row.Display = card.Display
		row.Description = card.Description
		row.Homepage = card.Homepage
		row.Updated = card.Updated
		row.Downloads = card.Downloads
		for _, ch := range card.Channels {
			row.Channels = append(row.Channels, apiChannelSummary{
				Name: ch.Name, Version: ch.Version, Code: ch.Code,
				Released: ch.Released, Yanked: ch.Yanked, Error: ch.Err,
			})
		}
	}
	sort.Strings(order)
	out := make([]apiProductSummary, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

func unescapeSegment(s string) string {
	if decoded, err := url.PathUnescape(s); err == nil {
		return decoded
	}
	return s
}

func (c *console) apiProducts(w http.ResponseWriter, r *http.Request, agent *agentClient, session string) {
	if !apiMethods(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	if r.Method == http.MethodPost {
		var req struct {
			ID      string `json:"id"`
			Root    string `json:"root"`
			Profile string `json:"profile"`
			Label   string `json:"label"`
		}
		if !decodeAPIBody(w, r, &req) {
			return
		}
		payload, _, err := agent.AddProduct(session,
			strings.TrimSpace(req.ID), strings.TrimSpace(req.Root),
			strings.TrimSpace(req.Profile), strings.TrimSpace(req.Label))
		if err != nil {
			writeAgentFailure(w, err)
			return
		}
		// The agent's answer already carries the token and its one-time
		// plaintext; relay it untouched.
		writeAPIJSON(w, http.StatusOK, payload)
		return
	}
	self, err := agent.Whoami(session)
	if err != nil {
		writeAgentFailure(w, err)
		return
	}
	products, err := c.mergedProducts(agent, session, self)
	if err != nil {
		writeAgentFailure(w, err)
		return
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"ok": true, "products": products})
}

func (c *console) apiProduct(w http.ResponseWriter, r *http.Request, agent *agentClient, session string) {
	if !apiMethods(w, r, http.MethodGet, http.MethodDelete) {
		return
	}
	id := r.PathValue("id")
	if !safeSegment(id) {
		writeAPIError(w, http.StatusNotFound, "not_found", "no such product", "")
		return
	}
	if r.Method == http.MethodDelete {
		// A dry run goes all the way to the agent: only it can look at the
		// published assets and say what retirement would actually remove.
		// Preview answers are flagged so the panel can never mistake one for a
		// completed retirement.
		dryRun := isDryRun(r)
		result, err := agent.DeleteProduct(session, id, dryRun)
		if err != nil {
			writeAgentFailure(w, err)
			return
		}
		if result == nil {
			result = map[string]any{}
		}
		result["ok"] = true
		if dryRun {
			result["dryRun"] = true
			writeAPIJSON(w, http.StatusOK, result)
			return
		}
		writeAPIJSON(w, http.StatusOK, result)
		return
	}
	self, err := agent.Whoami(session)
	if err != nil {
		writeAgentFailure(w, err)
		return
	}
	products, err := c.mergedProducts(agent, session, self)
	if err != nil {
		writeAgentFailure(w, err)
		return
	}
	var summary *apiProductSummary
	for i := range products {
		if products[i].ID == id {
			summary = &products[i]
			break
		}
	}
	if summary == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "no such product", "")
		return
	}
	detail := apiProductDetail{apiProductSummary: *summary, Detail: []apiChannelDetail{}}
	if page, ok := c.productDetail(id); ok {
		if meta := c.productSite(id); meta != nil {
			if meta.Title != "" {
				page.Name = meta.Title
			}
			detail.Description = meta.Description
			detail.Homepage = meta.Homepage
		}
		detail.Display = page.Name
		detail.StatsSince = c.stats.startedAt()
		for _, section := range page.Channels {
			detail.Detail = append(detail.Detail, channelDetailView(section))
		}
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"ok": true, "product": detail})
}

func channelDetailView(section channelSection) apiChannelDetail {
	view := apiChannelDetail{
		Name:           section.Name,
		Sequence:       section.Sequence,
		Generated:      section.Generated,
		IndexHref:      section.IndexHref,
		LatestVersion:  section.LatestLabel,
		LatestCode:     section.LatestCode,
		LatestReleased: section.LatestReleased,
		LatestNotes:    section.LatestNotes,
		Truncated:      section.Truncated,
		Error:          section.Err,
		Artifacts:      []apiArtifact{},
		Releases:       []apiRelease{},
	}
	for _, a := range section.Artifacts {
		view.Artifacts = append(view.Artifacts, apiArtifact{
			ID: a.ID, Filename: a.Filename, Platform: a.Platform, OS: a.OS, Arch: a.Arch,
			Size: a.Size, Sha256: a.Sha256, Href: a.Href, PermanentHref: a.PermanentHref,
			Downloads: a.Downloads,
		})
	}
	for _, rel := range section.Releases {
		view.Releases = append(view.Releases, apiRelease{
			Version: rel.Version, Code: rel.Code, Released: rel.Released, Yanked: rel.Yanked,
			ManifestHref: rel.ManifestHref, NotesURL: rel.NotesURL,
		})
	}
	return view
}

// ---- tokens ----------------------------------------------------------------

func (c *console) apiProductTokens(w http.ResponseWriter, r *http.Request, agent *agentClient, session string) {
	if !apiMethods(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	product := r.PathValue("id")
	if !safeSegment(product) {
		writeAPIError(w, http.StatusNotFound, "not_found", "no such product", "")
		return
	}
	if r.Method == http.MethodPost {
		var req struct {
			Label string `json:"label"`
		}
		if !decodeAPIBody(w, r, &req) {
			return
		}
		issued, err := agent.IssueToken(session, product, strings.TrimSpace(req.Label))
		if err != nil {
			writeAgentFailure(w, err)
			return
		}
		writeAPIJSON(w, http.StatusOK, map[string]any{
			"ok": true, "token": issued.Token, "plaintext": issued.Plaintext,
		})
		return
	}
	tokens, err := agent.Tokens(session, product)
	if err != nil {
		writeAgentFailure(w, err)
		return
	}
	if tokens == nil {
		tokens = []agentToken{}
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"ok": true, "tokens": tokens})
}

func (c *console) apiTokenAction(w http.ResponseWriter, r *http.Request, agent *agentClient, session string) {
	if !apiMethods(w, r, http.MethodPost) {
		return
	}
	id, action := r.PathValue("id"), r.PathValue("action")
	var (
		issued issuedToken
		err    error
	)
	switch action {
	case "rotate":
		issued, err = agent.RotateToken(session, id)
	case "revoke":
		issued, err = agent.RevokeToken(session, id)
	default:
		writeAPIError(w, http.StatusNotFound, "not_found", "no such token action", "")
		return
	}
	if err != nil {
		writeAgentFailure(w, err)
		return
	}
	body := map[string]any{"ok": true, "token": issued.Token}
	if action == "rotate" {
		body["plaintext"] = issued.Plaintext
	}
	writeAPIJSON(w, http.StatusOK, body)
}

// ---- users and roles -------------------------------------------------------

func normalizeProducts(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (c *console) apiUsers(w http.ResponseWriter, r *http.Request, agent *agentClient, session string) {
	if !apiMethods(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	if r.Method == http.MethodPost {
		var req struct {
			Username string   `json:"username"`
			Password string   `json:"password"`
			Role     string   `json:"role"`
			Products []string `json:"products"`
		}
		if !decodeAPIBody(w, r, &req) {
			return
		}
		user, err := agent.AddUser(session, strings.TrimSpace(req.Username), req.Password,
			strings.TrimSpace(req.Role), normalizeProducts(req.Products))
		if err != nil {
			writeAgentFailure(w, err)
			return
		}
		writeAPIJSON(w, http.StatusOK, map[string]any{"ok": true, "user": user})
		return
	}
	users, err := agent.Users(session)
	if err != nil {
		writeAgentFailure(w, err)
		return
	}
	if users == nil {
		users = []agentUser{}
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"ok": true, "users": users})
}

func (c *console) apiUser(w http.ResponseWriter, r *http.Request, agent *agentClient, session string) {
	if !apiMethods(w, r, http.MethodDelete) {
		return
	}
	name := r.PathValue("name")
	if err := agent.DeleteUser(session, name); err != nil {
		writeAgentFailure(w, err)
		return
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": name})
}

func (c *console) apiUserAction(w http.ResponseWriter, r *http.Request, agent *agentClient, session string) {
	name, action := r.PathValue("name"), r.PathValue("action")
	switch action {
	case "password":
		if !apiMethods(w, r, http.MethodPost) {
			return
		}
		var req struct {
			Password string `json:"password"`
		}
		if !decodeAPIBody(w, r, &req) {
			return
		}
		if err := agent.SetPassword(session, name, req.Password); err != nil {
			writeAgentFailure(w, err)
			return
		}
		writeAPIJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "role":
		if !apiMethods(w, r, http.MethodPost) {
			return
		}
		var req struct {
			Role     string   `json:"role"`
			Products []string `json:"products"`
		}
		if !decodeAPIBody(w, r, &req) {
			return
		}
		user, err := agent.SetRole(session, name, strings.TrimSpace(req.Role), normalizeProducts(req.Products))
		if err != nil {
			writeAgentFailure(w, err)
			return
		}
		writeAPIJSON(w, http.StatusOK, map[string]any{"ok": true, "user": user})
	case "products":
		if !apiMethods(w, r, http.MethodPut) {
			return
		}
		var req struct {
			Products []string `json:"products"`
		}
		if !decodeAPIBody(w, r, &req) {
			return
		}
		user, err := agent.SetUserProducts(session, name, normalizeProducts(req.Products))
		if err != nil {
			writeAgentFailure(w, err)
			return
		}
		writeAPIJSON(w, http.StatusOK, map[string]any{"ok": true, "user": user})
	default:
		writeAPIError(w, http.StatusNotFound, "not_found", "no such user action", "")
	}
}

// apiRoles reports the two roles and how many accounts hold each. The role
// names are the agent's; the front end owns the human-readable wording.
func (c *console) apiRoles(w http.ResponseWriter, r *http.Request, agent *agentClient, session string) {
	if !apiMethods(w, r, http.MethodGet) {
		return
	}
	users, err := agent.Users(session)
	if err != nil {
		writeAgentFailure(w, err)
		return
	}
	admins, owners := 0, 0
	for _, user := range users {
		if isAdminRole(user) {
			admins++
		} else {
			owners++
		}
	}
	writeAPIJSON(w, http.StatusOK, map[string]any{"ok": true, "roles": []apiRoleView{
		{Name: string(agentadmin.RoleAdmin), Members: admins},
		{Name: string(agentadmin.RoleOwner), Members: owners},
	}})
}

// ---- files -----------------------------------------------------------------

// apiFiles lists one directory of the raw key space. It is admin-only: the tree
// holds every product, so an owner restricted to their own products must not be
// able to browse the rest through it.
func (c *console) apiFiles(w http.ResponseWriter, r *http.Request, agent *agentClient, session string) {
	if !apiMethods(w, r, http.MethodGet) {
		return
	}
	self, err := agent.Whoami(session)
	if err != nil {
		writeAgentFailure(w, err)
		return
	}
	if !isAdminRole(self) {
		writeAPIError(w, http.StatusForbidden, "forbidden", "only an administrator can browse the file tree", "")
		return
	}
	dir := strings.Trim(r.URL.Query().Get("dir"), "/")
	if dir == "" {
		dir = "."
	}
	if dir != "." && (dir != path.Clean(dir) || strings.HasPrefix(dir, "..") || strings.Contains(dir, "\x00")) {
		writeAPIError(w, http.StatusBadRequest, "bad_path", "invalid directory", "dir")
		return
	}
	entries, err := c.adapter.ReadDir(dir)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "no such directory", "dir")
		return
	}
	out := make([]apiFileEntry, 0, len(entries))
	for _, entry := range entries {
		if c.hiddenKey(path.Join(dir, entry.Name)) {
			continue
		}
		row := apiFileEntry{Name: entry.Name, Dir: entry.IsDir}
		if !entry.Mtime.IsZero() {
			row.Mtime = entry.Mtime.Format(stampLayout)
			if !entry.IsDir {
				row.Size = entry.Size
			}
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return out[i].Name < out[j].Name
	})
	writeAPIJSON(w, http.StatusOK, map[string]any{"ok": true, "dir": dir, "entries": out})
}
