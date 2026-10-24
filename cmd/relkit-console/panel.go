package main

// The management panel: users, roles, host products. Exactly those three, and
// nothing else, because requirement 13.1 says so.
//
// Every write here is an HTTP call to the agent (requirement 13.6). There is no
// file path in this file, and none of these handlers can change what the agent
// authorizes. What the panel does own is the shape of the forms: it decides which
// fields an operator sees, then lets the agent refuse what it will.
//
// Two rendering rules are load-bearing:
//
//   - A plaintext token is shown in the response to the call that issued it and
//     nowhere else. It is not stored, not put in a cookie, and not in a redirect
//     target, so a refresh cannot recover it (requirement 11.3).
//   - The panel never pre-checks a permission before calling the agent. It calls,
//     then renders the refusal. A client that mirrors the server's rules drifts
//     from them.

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/agentadmin"
)

const (
	panelPath       = "/-/admin/panel"
	panelUsersPath  = "/-/admin/panel/users"
	panelRolesPath  = "/-/admin/panel/roles"
	panelHostsPath  = "/-/admin/panel/products"
	panelTokensPath = "/-/admin/panel/tokens"
)

// panelRow is one user line. Role and products are rendered as chips so the
// page answers "who can do what" without opening the edit form.
type panelRow struct {
	Username  string
	Role      string
	RoleLabel string
	Products  []string
	Created   string
}

type productRow struct {
	ID            string
	Root          string
	Profile       string
	ProfileNote   string
	Created       string
	Server        string
	RootKeyID     string
	RootPublicKey string
	Fingerprint   string
	ConnectJSON   string
	Tokens        []tokenRow
	TokenError    string
}

type tokenRow struct {
	ID      string
	Label   string
	Created string
	Rotated string
	Revoked string
	RevokedFlag bool
}

// issuedPanelToken is the once-only display block. It exists as a struct so the
// template cannot accidentally render it from a listing: nothing stores it.
type issuedPanelToken struct {
	Product   string
	TokenID   string
	Label     string
	Plaintext string
	ConnectJSON string
}

type panelPage struct {
	pageChrome
	Heading     string
	Sub         string
	Tab         string
	Product     string
	AgentBase   string
	Self        agentUser
	IsAdmin     bool
	Error       string
	Notice      string
	Users       []panelRow
	Roles       []roleRow
	Products    []productRow
	Issued      *issuedPanelToken
	ProductList []string
}

type roleRow struct {
	Name        string
	Description string
	Members     int
	Products    string
}

// handlePanel dispatches the four panel screens plus their form posts. Splitting
// it into one handler per action would duplicate the session and role lookup at
// every entry, and that duplication is how a screen ends up unauthenticated.
func (c *console) handlePanel(w http.ResponseWriter, r *http.Request) {
	agentSession := panelAgentSession(r)
	agent := c.admin.agentClient()
	if agent == nil {
		c.serveAgentUnavailable(w, r)
		return
	}
	self, err := agent.Whoami(agentSession)
	if err != nil {
		c.renderAgentFailure(w, r, err)
		return
	}

	if r.Method == http.MethodPost {
		if !sameOriginPOST(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "cannot read form", http.StatusBadRequest)
			return
		}
		c.handlePanelPost(w, r, agent, self)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	switch r.URL.Path {
	case panelUsersPath:
		c.renderUsers(w, r, agent, self, "", "")
	case panelRolesPath:
		c.renderRoles(w, r, agent, self, "", "")
	case panelHostsPath:
		c.renderProducts(w, r, agent, self, "", "", nil)
	case panelTokensPath:
		c.renderTokens(w, r, agent, self, r.URL.Query().Get("product"), "", "", nil)
	default:
		c.renderUsers(w, r, agent, self, "", "")
	}
}

func (c *console) handlePanelPost(w http.ResponseWriter, r *http.Request, agent *agentClient, self agentUser) {
	session := panelAgentSession(r)
	action := r.FormValue("action")
	username := strings.TrimSpace(r.FormValue("username"))
	product := strings.TrimSpace(r.FormValue("product"))

	switch action {
	case "add-user":
		role := strings.TrimSpace(r.FormValue("role"))
		_, err := agent.AddUser(session, username, r.FormValue("password"), role, formProducts(r))
		if err != nil {
			c.renderUsers(w, r, agent, self, "The agent refused the new account: "+err.Error(), "")
			return
		}
		c.renderUsers(w, r, agent, self, "", "Account "+username+" created.")
	case "delete-user":
		if err := agent.DeleteUser(session, username); err != nil {
			c.renderUsers(w, r, agent, self, "The agent refused to delete "+username+": "+err.Error(), "")
			return
		}
		c.renderUsers(w, r, agent, self, "", "Account "+username+" deleted.")
	case "set-password":
		if err := agent.SetPassword(session, username, r.FormValue("password")); err != nil {
			c.renderUsers(w, r, agent, self, "The agent refused the password change: "+err.Error(), "")
			return
		}
		c.renderUsers(w, r, agent, self, "", "Password for "+username+" changed.")
	case "set-role":
		role := strings.TrimSpace(r.FormValue("role"))
		if _, err := agent.SetRole(session, username, role, formProducts(r)); err != nil {
			c.renderRoles(w, r, agent, self, "The agent refused the role change: "+err.Error(), "")
			return
		}
		c.renderRoles(w, r, agent, self, "", username+" is now "+role+".")
	case "add-product":
		label := strings.TrimSpace(r.FormValue("label"))
		payload, issued, err := agent.AddProduct(session, product, strings.TrimSpace(r.FormValue("root")), strings.TrimSpace(r.FormValue("profile")), label)
		if err != nil {
			c.renderProducts(w, r, agent, self, "The agent refused the product: "+err.Error(), "", nil)
			return
		}
		notice := "Product " + product + " registered."
		if missing, _ := payload["profileMissing"].(bool); missing {
			if expected, _ := payload["profileExpectedAt"].(string); expected != "" {
				notice += " NOTE: no publish profile at " + expected + "; publishing needs one."
			}
		}
		c.renderProducts(w, r, agent, self, "", notice, &issuedPanelToken{
			Product:     product,
			TokenID:     issued.Token.ID,
			Label:       issued.Token.Label,
			Plaintext:   issued.Plaintext,
			ConnectJSON: connectJSON(payload["connectConfig"]),
		})
	case "delete-product":
		result, err := agent.DeleteProduct(session, product)
		if err != nil {
			c.renderProducts(w, r, agent, self, "The agent refused to retire "+product+": "+err.Error(), "", nil)
			return
		}
		c.renderProducts(w, r, agent, self, "", retirementNotice(product, result), nil)
	case "issue-token":
		issued, err := agent.IssueToken(session, product, strings.TrimSpace(r.FormValue("label")))
		if err != nil {
			c.renderTokens(w, r, agent, self, product, "The agent refused to issue a token: "+err.Error(), "", nil)
			return
		}
		c.renderTokens(w, r, agent, self, product, "", "", &issuedPanelToken{
			Product:   product,
			TokenID:   issued.Token.ID,
			Label:     issued.Token.Label,
			Plaintext: issued.Plaintext,
		})
	case "rotate-token", "revoke-token":
		id := strings.TrimSpace(r.FormValue("token"))
		var (
			issued issuedToken
			err    error
		)
		if action == "rotate-token" {
			issued, err = agent.RotateToken(session, id)
		} else {
			issued, err = agent.RevokeToken(session, id)
		}
		if err != nil {
			c.renderTokens(w, r, agent, self, product, "", "The agent refused: "+err.Error(), nil)
			return
		}
		block := &issuedPanelToken{Product: product, TokenID: issued.Token.ID, Label: issued.Token.Label}
		notice := "Token rotated. The previous value stopped working."
		if action == "revoke-token" {
			// A revocation issues nothing, so there is no plaintext to show and
			// nothing to warn about copying.
			notice = "Token revoked. Nothing that used it will authenticate again."
		} else {
			block.Plaintext = issued.Plaintext
		}
		c.renderTokens(w, r, agent, self, product, notice, "", block)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}

func formProducts(r *http.Request) []string {
	raw := r.Form["products"]
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, value := range raw {
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

func retirementNotice(product string, result map[string]any) string {
	removed, _ := result["removedTokens"].(float64)
	notice := fmt.Sprintf("Product %s retired; every asset and %d token went with it.", product, int(removed))
	if dry, ok := result["dryRun"].(bool); ok && dry {
		notice = "Dry run only: nothing was deleted."
	}
	return notice
}

func connectJSON(raw any) string {
	config, ok := raw.(map[string]any)
	if !ok || len(config) == 0 {
		return ""
	}
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	builder.WriteString("{\n")
	for index, key := range keys {
		value, _ := config[key].(string)
		fmt.Fprintf(&builder, "  %q: %q", key, value)
		if index < len(keys)-1 {
			builder.WriteString(",")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("}")
	return builder.String()
}

func (c *console) renderAgentFailure(w http.ResponseWriter, r *http.Request, err error) {
	var refusal *agentError
	if errors.As(err, &refusal) && refusal.Unauthorized() {
		// The agent forgot the session (it restarts signed out, by design). Drop
		// the cookie and send the operator back to sign in rather than showing an
		// error they cannot act on.
		http.SetCookie(w, expireCookie())
		http.Redirect(w, r, adminLoginPath, http.StatusFound)
		return
	}
	c.renderUsers(w, r, c.admin.agentClient(), agentUser{Username: panelUsername(r)}, "The agent did not answer: "+err.Error(), "")
}

func (c *console) renderUsers(w http.ResponseWriter, r *http.Request, agent *agentClient, self agentUser, errMsg, notice string) {
	page := c.newPanelPage("users", self, errMsg, notice)
	users, err := agent.Users(panelAgentSession(r))
	if err != nil {
		page.Error = joinError(page.Error, err.Error())
	}
	for _, user := range users {
		page.Users = append(page.Users, panelRow{
			Username:  user.Username,
			Role:      user.Role,
			RoleLabel: roleLabel(user.Role),
			Products:  append([]string(nil), user.Products...),
			Created:   formatStamp(user.CreatedAt),
		})
	}
	products, _ := agent.Products(panelAgentSession(r))
	for _, view := range products {
		page.ProductList = append(page.ProductList, view.Product.ID)
	}
	sort.Strings(page.ProductList)
	page.Heading = "Users"
	page.Sub = "Accounts that can sign in here. The agent owns them; this page is a form."
	c.renderPage(w, r, "panel_users", page)
}

func (c *console) renderRoles(w http.ResponseWriter, r *http.Request, agent *agentClient, self agentUser, errMsg, notice string) {
	page := c.newPanelPage("roles", self, errMsg, notice)
	users, err := agent.Users(panelAgentSession(r))
	if err != nil {
		page.Error = joinError(page.Error, err.Error())
	}
	admins, owners := 0, 0
	for _, user := range users {
		row := panelRow{
			Username:  user.Username,
			Role:      user.Role,
			RoleLabel: roleLabel(user.Role),
			Products:  append([]string(nil), user.Products...),
			Created:   formatStamp(user.CreatedAt),
		}
		page.Users = append(page.Users, row)
		if user.Role == string(agentadmin.RoleAdmin) {
			admins++
			continue
		}
		owners++
	}
	page.Roles = []roleRow{
		{
			Name:        string(agentadmin.RoleAdmin),
			Description: "Manages every account and every product, including tokens for products it was never assigned.",
			Members:     admins,
		},
		{
			Name:        string(agentadmin.RoleOwner),
			Description: "Manages the tokens of the products assigned to it. Cannot create accounts or products.",
			Members:     owners,
		},
	}
	products, _ := agent.Products(panelAgentSession(r))
	for _, view := range products {
		page.ProductList = append(page.ProductList, view.Product.ID)
	}
	sort.Strings(page.ProductList)
	page.Heading = "Roles"
	page.Sub = "Two roles, because the panel has three subjects: users, roles, products."
	c.renderPage(w, r, "panel_roles", page)
}

func (c *console) renderProducts(w http.ResponseWriter, r *http.Request, agent *agentClient, self agentUser, errMsg, notice string, issued *issuedPanelToken) {
	page := c.newPanelPage("products", self, errMsg, notice)
	page.Issued = issued
	views, err := agent.Products(panelAgentSession(r))
	if err != nil {
		page.Error = joinError(page.Error, err.Error())
	}
	for _, view := range views {
		row := productRow{
			ID:            view.Product.ID,
			Root:          view.Product.Root,
			Profile:       view.Product.Profile,
			Created:       formatStamp(view.Product.CreatedAt),
			Server:        view.server(),
			RootKeyID:     view.rootKeyID(),
			RootPublicKey: view.rootKey(),
			Fingerprint:   view.fingerprint(),
			ConnectJSON:   connectJSON(view.ConnectConfig),
		}
		page.Products = append(page.Products, row)
	}
	page.Heading = "Host products"
	page.Sub = "Each row is one product this server publishes. Deleting a row is the whole retirement, not just a label."
	c.renderPage(w, r, "panel_products", page)
}

func (c *console) renderTokens(w http.ResponseWriter, r *http.Request, agent *agentClient, self agentUser, product, notice, errMsg string, issued *issuedPanelToken) {
	session := panelAgentSession(r)
	page := c.newPanelPage("tokens", self, errMsg, notice)
	page.Issued = issued
	views, err := agent.Products(session)
	if err != nil {
		page.Error = joinError(page.Error, err.Error())
	}
	for _, view := range views {
		page.ProductList = append(page.ProductList, view.Product.ID)
	}
	sort.Strings(page.ProductList)
	if product == "" && len(page.ProductList) > 0 {
		product = page.ProductList[0]
	}
	page.Sub = "Tokens authorize one product each. The plaintext exists only in the response that issued it."
	if product != "" {
		row := productRow{ID: product}
		for _, view := range views {
			if view.Product.ID != product {
				continue
			}
			row.Server = view.server()
			row.RootPublicKey = view.rootKey()
			row.ConnectJSON = connectJSON(view.ConnectConfig)
		}
		tokens, err := agent.Tokens(session, product)
		if err != nil {
			row.TokenError = err.Error()
		}
		for _, token := range tokens {
			row.Tokens = append(row.Tokens, tokenRow{
				ID:          token.ID,
				Label:       token.Label,
				Created:     formatStamp(token.CreatedAt),
				Rotated:     formatStamp(token.RotatedAt),
				Revoked:     formatStamp(token.RevokedAt),
				RevokedFlag: token.Revoked(),
			})
		}
		page.Products = append(page.Products, row)
	}
	page.Heading = "Tokens"
	page.Product = product
	c.renderPage(w, r, "panel_tokens", page)
}

func (c *console) newPanelPage(tab string, self agentUser, errMsg, notice string) *panelPage {
	page := &panelPage{
		pageChrome: pageChrome{
			Title:   "Panel",
			Version: version,
			Crumbs: []crumb{
				{Label: "Releases", Href: adminPath},
				{Label: "panel", Href: panelUsersPath},
			},
		},
		Tab:     tab,
		Self:    self,
		IsAdmin: self.Role == string(agentadmin.RoleAdmin),
		Error:   errMsg,
		Notice:  notice,
	}
	if agent := c.admin.agentClient(); agent != nil {
		page.AgentBase = agent.base
	}
	return page
}

func roleLabel(role string) string {
	switch role {
	case string(agentadmin.RoleAdmin):
		return "Administrator"
	case string(agentadmin.RoleOwner):
		return "Product owner"
	}
	return role
}

func joinError(existing, next string) string {
	if existing == "" {
		return next
	}
	return existing + "; " + next
}

func formatStamp(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	return parsed.Local().Format(stampLayout)
}

// panelTabHref is used by the templates so the tab strip stays in one place.
func panelTabHref(tab string) string {
	switch tab {
	case "users":
		return panelUsersPath
	case "roles":
		return panelRolesPath
	case "products":
		return panelHostsPath
	case "tokens":
		return panelTokensPath
	}
	return panelUsersPath
}

func productTokensHref(product string) string {
	return panelTokensPath + "?product=" + url.PathEscape(product)
}

