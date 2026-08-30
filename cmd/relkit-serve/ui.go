package main

// Operator pages live under /-/ so they never shadow a published file. Protocol
// clients never read any of this: they fetch the signed index.
//
// The public catalog is the static browse dump (GET /). These pages are the
// self-hosted panel: live portal, product cards, file tree. That surface is for
// operators on this box and can grow into an admin UI; it is not the catalog
// people bookmark.

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/webmeta"
)

// productPathPrefix is under /-/ for the same reason the health endpoint is:
// RUP keys start with index/, manifest/ or artifact/, and a leading dash is not
// a legal identifier, so nothing here can ever shadow a real file.
const (
	productPathPrefix = "/-/p/"
	latestPathPrefix  = "/-/latest/"
	adminPath         = "/-/admin"
	adminFilesPath    = "/-/admin/files"
)

const (
	dateLayout  = "2006-01-02"
	stampLayout = "2006-01-02 15:04"

	// A channel index keeps the full upgrade chain when a product needs
	// intermediate hops, and printing hundreds of rows helps nobody.
	maxReleaseRows = 25
)

type crumb struct {
	Label string
	Href  string // empty renders as plain text, i.e. the current page
}

// pageChrome is embedded by every page so the shared head/foot templates can
// reach the same fields regardless of which page is rendering.
type pageChrome struct {
	Title   string
	Crumbs  []crumb
	Version string
	Note    string
}

// channelRow is the portal's one-line summary of a channel. The index object
// behind it is an operator detail, so its link lives on the product page.
//
// Download* is the fixed URL of that channel's build for the visiting platform,
// resolved from the pointer the publish wrote. A channel with no pointer, or no
// build for this platform, simply has no link.
type channelRow struct {
	Name         string
	Version      string
	Code         int64
	Released     string
	Yanked       bool
	Err          string
	DownloadHref string
	DownloadName string
	DownloadSize string
}

type productCard struct {
	Display     string
	Description string
	Homepage    string
	Href        string
	Updated     string
	Downloads   int64
	Channels    []channelRow
}

type portalPage struct {
	pageChrome
	Heading    string
	Sub        string
	StatsSince string
	Products   []productCard
}

type artifactRow struct {
	ID            string
	Filename      string
	Platform      string
	Size          string
	Sha256        string
	Sha256Short   string
	Href          string
	PermanentHref string
	Downloads     int64
	OS            string
	Arch          string
}

type releaseRow struct {
	Version      string
	Code         int64
	Released     string
	Yanked       bool
	ManifestHref string
	NotesURL     string
}

type channelSection struct {
	Name           string
	Sequence       int64
	Generated      string
	IndexHref      string
	LatestLabel    string
	LatestCode     int64
	LatestReleased string
	LatestNotes    string
	Artifacts      []artifactRow
	Releases       []releaseRow
	Truncated      int
	Err            string
}

type productPage struct {
	pageChrome
	Name        string
	Description string
	Homepage    string
	Sub         string
	StatsSince  string
	Channels    []channelSection
}

type listingEntry struct {
	Name  string
	Href  string
	Size  string
	Mtime string
	Dir   bool
}

type listingPage struct {
	pageChrome
	Display string
	Count   int
	Parent  string
	Entries []listingEntry
}

// scanProducts reports the products this box distributes, derived from the
// index tree rather than from configuration: index/<product>/<channel>.pb is
// the only thing that is true by construction after a publish.
//
// A product with no parsable channel is left out entirely. That is what makes
// the plain-static-host case fall back to a file listing instead of showing a
// page full of parse errors.
func (c *config) scanProducts(want platformGuess) []productCard {
	entries, err := fs.ReadDir(c.root.FS(), "index")
	if err != nil {
		return nil
	}
	var cards []productCard
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if card, ok := c.readProductCard(entry.Name(), want); ok {
			cards = append(cards, card)
		}
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].Display < cards[j].Display })
	return cards
}

func (c *config) readProductCard(product string, want platformGuess) (productCard, bool) {
	files, err := fs.ReadDir(c.root.FS(), path.Join("index", product))
	if err != nil {
		return productCard{}, false
	}

	card := productCard{
		Display:   product,
		Href:      productPathPrefix + url.PathEscape(product),
		Downloads: c.stats.totalUnder("artifact/" + product + "/"),
	}
	var newest time.Time
	parsed := 0

	for _, file := range files {
		channel, ok := channelName(file)
		if !ok {
			continue
		}
		key := path.Join("index", product, file.Name())
		row := channelRow{Name: channel}

		if info, err := file.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}

		doc, err := readIndexDoc(c.root, key)
		if err != nil {
			row.Err = "unreadable"
			card.Channels = append(card.Channels, row)
			continue
		}
		parsed++
		if doc.Product != "" {
			card.Display = doc.Product
		}
		if latest := latestVersion(doc.Versions); latest != nil {
			row.Version = latest.Version
			row.Code = latest.Code
			row.Yanked = latest.Yanked
			row.Released = formatTimestamp(latest.ReleasedAt, dateLayout)
			if row.Released == "" {
				row.Released = formatTimestamp(doc.GeneratedAt, dateLayout)
			}
		} else {
			row.Err = "no versions"
		}
		c.attachDownload(&row, product, channel, want)
		card.Channels = append(card.Channels, row)
	}

	if parsed == 0 {
		return productCard{}, false
	}
	sortChannels(card.Channels)
	if !newest.IsZero() {
		card.Updated = newest.Format(dateLayout)
	}

	if meta := c.productSite(product); meta != nil {
		if meta.Title != "" {
			card.Display = meta.Title
		}
		card.Description = meta.Description
		card.Homepage = meta.Homepage
	}
	return card, true
}

// attachDownload points the row at the channel's fixed URL rather than at the
// artifact the index currently names, so the link on the portal is the same one
// a user may have bookmarked.
func (c *config) attachDownload(row *channelRow, product, channel string, want platformGuess) {
	if want.os == "" {
		return
	}
	doc := c.readLatest(product, channel)
	if doc == nil {
		return
	}
	match := bestMatch(c.latestArtifacts(doc), want)
	if match == nil {
		return
	}
	row.DownloadHref = match.Href
	row.DownloadName = match.Filename
	row.DownloadSize = match.Size
}

// productDetail expands one product: every channel, its release chain, and the
// downloadable files of the newest version. Artifacts come from the manifest,
// because that is where filenames, sizes and hashes live.
func (c *config) productDetail(product string) (*productPage, bool) {
	files, err := fs.ReadDir(c.root.FS(), path.Join("index", product))
	if err != nil {
		return nil, false
	}

	page := &productPage{Name: product}
	parsed := 0

	for _, file := range files {
		channel, ok := channelName(file)
		if !ok {
			continue
		}
		key := path.Join("index", product, file.Name())
		section := channelSection{Name: channel, IndexHref: "/" + key}

		doc, err := readIndexDoc(c.root, key)
		if err != nil {
			section.Err = "index unreadable: " + err.Error()
			page.Channels = append(page.Channels, section)
			continue
		}
		parsed++
		if doc.Product != "" {
			page.Name = doc.Product
		}
		section.Sequence = doc.Sequence
		section.Generated = formatTimestamp(doc.GeneratedAt, stampLayout)

		nodes := sortedVersions(doc.Versions)
		for i, node := range nodes {
			if i >= maxReleaseRows {
				section.Truncated = len(nodes) - maxReleaseRows
				break
			}
			row := releaseRow{
				Version:  node.Version,
				Code:     node.Code,
				Yanked:   node.Yanked,
				Released: formatTimestamp(node.ReleasedAt, dateLayout),
				NotesURL: node.NotesUrl,
			}
			if node.Manifest != nil {
				row.ManifestHref = localHref(node.Manifest.Urls)
			}
			section.Releases = append(section.Releases, row)
		}

		if latest := latestVersion(doc.Versions); latest != nil {
			section.Artifacts = c.artifactsFor(latest)
			section.LatestLabel = latest.Version
			section.LatestCode = latest.Code
			section.LatestReleased = formatTimestamp(latest.ReleasedAt, dateLayout)
			section.LatestNotes = latest.NotesUrl
			if latest.Yanked {
				section.LatestLabel += " (yanked)"
			}
		}
		for i := range section.Artifacts {
			section.Artifacts[i].PermanentHref = c.fixedHref(product, channel, section.Artifacts[i].ID)
		}
		page.Channels = append(page.Channels, section)
	}

	if parsed == 0 {
		return nil, false
	}
	sort.Slice(page.Channels, func(i, j int) bool {
		return channelLess(page.Channels[i].Name, page.Channels[j].Name)
	})
	return page, true
}

func (c *config) artifactsFor(node *rupv2.VersionNode) []artifactRow {
	if node == nil || node.Manifest == nil {
		return nil
	}
	for _, raw := range node.Manifest.Urls {
		key, ok := localKeyFromURL(raw)
		if !ok || !strings.HasPrefix(key, "manifest/") {
			continue
		}
		doc, err := readManifestDoc(c.root, key)
		if err != nil {
			continue
		}
		rows := make([]artifactRow, 0, len(doc.Artifacts))
		for _, artifact := range doc.Artifacts {
			if artifact == nil {
				continue
			}
			row := artifactRow{
				ID:          artifact.Id,
				Filename:    artifact.Filename,
				Platform:    platformLabel(artifact.Selectors),
				Size:        humanBytes(artifact.Size),
				Sha256:      artifact.Sha256,
				Sha256Short: shortHash(artifact.Sha256),
				Href:        downloadHref(artifact.Urls),
				OS:          selectorValue(artifact.Selectors, "os"),
				Arch:        selectorValue(artifact.Selectors, "arch"),
			}
			if row.Filename == "" {
				row.Filename = artifact.Id
			}
			if key, ok := localKeyFromURL(row.Href); ok {
				row.Downloads = c.stats.count(key)
			}
			rows = append(rows, row)
		}
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].Platform != rows[j].Platform {
				return rows[i].Platform < rows[j].Platform
			}
			return rows[i].Filename < rows[j].Filename
		})
		return rows
	}
	return nil
}

func (c *config) productSite(product string) *webmeta.Site {
	if raw, err := c.root.ReadFile(webmeta.SiteKey(product)); err == nil {
		if doc, err := webmeta.UnmarshalSite(raw); err == nil && doc.Product == product {
			return doc
		}
	}
	// Server config remains a migration fallback. New product teams publish
	// their own copy from relkit.json.
	if meta := c.site.product(product); meta != nil {
		return &webmeta.Site{
			Product:     product,
			Title:       meta.Title,
			Description: meta.Description,
			Homepage:    meta.Homepage,
		}
	}
	return nil
}

func (c *config) readLatest(product, channel string) *webmeta.Latest {
	raw, err := c.root.ReadFile(webmeta.LatestKey(product, channel))
	if err != nil {
		return nil
	}
	doc, err := webmeta.UnmarshalLatest(raw)
	if err != nil || doc.Product != product || doc.Channel != channel {
		return nil
	}
	return doc
}

func (c *config) latestArtifacts(doc *webmeta.Latest) []artifactRow {
	if doc == nil {
		return nil
	}
	rows := make([]artifactRow, 0, len(doc.Artifacts))
	for _, artifact := range doc.Artifacts {
		href := latestHref(doc.Product, doc.Channel, artifact.ID)
		row := artifactRow{
			ID:          artifact.ID,
			Filename:    artifact.Filename,
			Platform:    platformMapLabel(artifact.Selectors),
			Size:        humanBytes(artifact.Size),
			Sha256:      artifact.Sha256,
			Sha256Short: shortHash(artifact.Sha256),
			Href:        href,
			OS:          artifact.Selectors["os"],
			Arch:        artifact.Selectors["arch"],
		}
		if target := downloadHref(artifact.URLs); target != "" {
			if key, ok := localKeyFromURL(target); ok {
				row.Downloads = c.stats.count(key)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// fixedHref returns a copy-pasteable URL that keeps following one channel.
func (c *config) fixedHref(product, channel, artifactID string) string {
	doc := c.readLatest(product, channel)
	if doc == nil {
		return ""
	}
	for _, artifact := range doc.Artifacts {
		if artifact.ID == artifactID {
			return latestHref(product, channel, artifact.ID)
		}
	}
	return ""
}

func latestHref(product, channel, artifactID string) string {
	return latestPathPrefix + url.PathEscape(product) + "/" + url.PathEscape(channel) + "/" + url.PathEscape(artifactID)
}

func (c *config) servePortal(w http.ResponseWriter, r *http.Request, products []productCard) {
	heading := "Releases"
	if c.site != nil && c.site.Title != "" {
		heading = c.site.Title
	}
	page := &portalPage{
		pageChrome: pageChrome{
			Title:   heading + " · " + r.Host,
			Version: version,
			Note:    plural(len(products), "product", "products"),
		},
		Heading:  heading,
		Sub:      r.Host,
		Products: products,
	}
	for _, product := range products {
		if product.Downloads > 0 {
			page.StatsSince = c.stats.startedAt()
			break
		}
	}
	c.renderPage(w, r, "portal", page)
}

func (c *config) serveAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	products := c.scanProducts(guessPlatform(r.UserAgent()))
	if len(products) > 0 {
		c.servePortal(w, r, products)
		return
	}
	c.serveRootListing(w, r)
}

func (c *config) serveAdminFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c.serveRootListing(w, r)
}

func (c *config) serveRootListing(w http.ResponseWriter, r *http.Request) {
	file, err := c.root.Open(".")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer file.Close()
	c.listDir(w, r, file, ".")
}

func (c *config) serveProduct(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, productPathPrefix), "/")
	if rest == "" || strings.Contains(rest, "/") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	product, err := url.PathUnescape(rest)
	if err != nil || product != path.Clean(product) || strings.HasPrefix(product, ".") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	page, ok := c.productDetail(product)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if meta := c.productSite(product); meta != nil {
		if meta.Title != "" {
			page.Name = meta.Title
		}
		page.Description = meta.Description
		page.Homepage = meta.Homepage
	}
	page.pageChrome = pageChrome{
		Title:   page.Name + " · releases",
		Version: version,
		Note:    plural(len(page.Channels), "channel", "channels"),
		Crumbs:  []crumb{{Label: "Releases", Href: adminPath}, {Label: page.Name}},
	}
	page.Sub = r.Host
	page.StatsSince = c.stats.startedAt()
	c.renderPage(w, r, "product", page)
}

// serveLatest resolves /-/latest/<product>/<channel>/<artifact-id> from the
// pointer written during publish. It performs no index/manifest scan.
func (c *config) serveLatest(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, latestPathPrefix), "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 3 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	product, errProduct := url.PathUnescape(parts[0])
	channel, errChannel := url.PathUnescape(parts[1])
	artifactID, errArtifact := url.PathUnescape(parts[2])
	if errProduct != nil || errChannel != nil || errArtifact != nil ||
		!safeSegment(product) || !safeSegment(channel) || !safeSegment(artifactID) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	doc := c.readLatest(product, channel)
	if doc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	for _, artifact := range doc.Artifacts {
		if artifact.ID != artifactID {
			continue
		}
		target := downloadHref(artifact.URLs)
		if target == "" {
			break
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.Redirect(w, r, target, http.StatusFound)
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func safeSegment(s string) bool {
	return s != "" && s == path.Clean(s) && !strings.HasPrefix(s, ".") && !strings.Contains(s, "/")
}

// renderPage buffers first so a template error cannot leave a half-written page
// behind an already-sent 200.
func (c *config) renderPage(w http.ResponseWriter, r *http.Request, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	var buf bytes.Buffer
	if err := pageTemplates.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "cannot render page", http.StatusInternalServerError)
		return
	}
	buf.WriteTo(w)
}

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

func channelName(entry fs.DirEntry) (string, bool) {
	if entry.IsDir() {
		return "", false
	}
	name := entry.Name()
	if strings.HasSuffix(name, ".tmp~") || !strings.HasSuffix(name, ".pb") {
		return "", false
	}
	return strings.TrimSuffix(name, ".pb"), true
}

// latestVersion prefers the highest live code. A channel whose head is yanked
// still has to show something, or the page would claim the product has no
// releases while the index plainly has nodes.
func latestVersion(nodes []*rupv2.VersionNode) *rupv2.VersionNode {
	var live, yanked *rupv2.VersionNode
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Yanked {
			if yanked == nil || node.Code > yanked.Code {
				yanked = node
			}
			continue
		}
		if live == nil || node.Code > live.Code {
			live = node
		}
	}
	if live != nil {
		return live
	}
	return yanked
}

func sortedVersions(nodes []*rupv2.VersionNode) []*rupv2.VersionNode {
	out := make([]*rupv2.VersionNode, 0, len(nodes))
	for _, node := range nodes {
		if node != nil {
			out = append(out, node)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Code > out[j].Code })
	return out
}

func sortChannels(rows []channelRow) {
	sort.SliceStable(rows, func(i, j int) bool { return channelLess(rows[i].Name, rows[j].Name) })
}

// channelLess puts the channel most people came for first.
func channelLess(a, b string) bool {
	ra, rb := channelRank(a), channelRank(b)
	if ra != rb {
		return ra < rb
	}
	return a < b
}

func channelRank(name string) int {
	switch name {
	case "stable":
		return 0
	case "beta":
		return 1
	case "dev":
		return 2
	}
	return 3
}

// platformGuess is what the visitor's browser says it is running on, expressed
// in Go's GOOS/GOARCH vocabulary so that it can be compared with normalized
// selector values.
type platformGuess struct {
	os   string
	arch string
}

// guessPlatform reads the User-Agent. It is a hint for one link on the page and
// nothing else: no content depends on it, and being wrong costs the visitor one
// glance at the table below.
func guessPlatform(userAgent string) platformGuess {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "android"):
		// Android UAs carry "linux" too, so this has to come first.
		return platformGuess{os: "android"}
	case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipad"):
		return platformGuess{os: "ios"}
	case strings.Contains(ua, "windows"):
		return platformGuess{os: "windows", arch: uaArch(ua, "amd64")}
	case strings.Contains(ua, "mac os x"), strings.Contains(ua, "macintosh"):
		// Safari and Chrome report Intel on Apple Silicon as well, so the
		// architecture here is deliberately left unknown.
		return platformGuess{os: "darwin"}
	case strings.Contains(ua, "linux"), strings.Contains(ua, "x11"):
		return platformGuess{os: "linux", arch: uaArch(ua, "amd64")}
	}
	return platformGuess{}
}

func uaArch(ua, fallback string) string {
	switch {
	case strings.Contains(ua, "arm64"), strings.Contains(ua, "aarch64"):
		return "arm64"
	case strings.Contains(ua, "wow64"), strings.Contains(ua, "win64"),
		strings.Contains(ua, "x86_64"), strings.Contains(ua, "x64"):
		return "amd64"
	}
	return fallback
}

// bestMatch prefers an exact os+arch hit, then the right os with an
// unspecified or unknown arch. Nothing else is offered: handing a visitor the
// wrong binary is worse than handing them a table.
func bestMatch(rows []artifactRow, want platformGuess) *artifactRow {
	if want.os == "" {
		return nil
	}
	var loose *artifactRow
	for i := range rows {
		row := &rows[i]
		if row.Href == "" || normalizeOS(row.OS) != want.os {
			continue
		}
		arch := normalizeArch(row.Arch)
		if want.arch != "" && arch == want.arch {
			return row
		}
		if arch == "" || want.arch == "" {
			if loose == nil {
				loose = row
			}
		}
	}
	return loose
}

func normalizeOS(value string) string {
	switch strings.ToLower(value) {
	case "windows", "win", "win32", "win64":
		return "windows"
	case "darwin", "macos", "mac", "osx", "mac-os":
		return "darwin"
	case "linux":
		return "linux"
	case "android":
		return "android"
	case "ios", "iphoneos":
		return "ios"
	}
	return strings.ToLower(value)
}

func normalizeArch(value string) string {
	switch strings.ToLower(value) {
	case "amd64", "x64", "x86_64", "x86-64":
		return "amd64"
	case "arm64", "aarch64":
		return "arm64"
	case "386", "x86", "i386", "ia32":
		return "386"
	}
	return strings.ToLower(value)
}

func selectorValue(selectors []*rupv2.Selector, key string) string {
	for _, selector := range selectors {
		if selector != nil && selector.Key == key {
			return selector.Value
		}
	}
	return ""
}

func platformLabel(selectors []*rupv2.Selector) string {
	values := map[string]string{}
	var extra []string
	for _, selector := range selectors {
		if selector == nil {
			continue
		}
		switch selector.Key {
		case "os", "arch":
			values[selector.Key] = selector.Value
		default:
			extra = append(extra, selector.Key+"="+selector.Value)
		}
	}
	return joinPlatformLabel(values, extra)
}

func platformMapLabel(selectors map[string]string) string {
	values := map[string]string{
		"os":   selectors["os"],
		"arch": selectors["arch"],
	}
	var extra []string
	for key, value := range selectors {
		if key != "os" && key != "arch" {
			extra = append(extra, key+"="+value)
		}
	}
	sort.Strings(extra)
	return joinPlatformLabel(values, extra)
}

func joinPlatformLabel(values map[string]string, extra []string) string {
	var parts []string
	if os, arch := values["os"], values["arch"]; os != "" || arch != "" {
		switch {
		case os != "" && arch != "":
			parts = append(parts, os+"/"+arch)
		case os != "":
			parts = append(parts, os)
		default:
			parts = append(parts, arch)
		}
	}
	parts = append(parts, extra...)
	if len(parts) == 0 {
		return "any"
	}
	return strings.Join(parts, " ")
}

// downloadHref prefers a URL that resolves to a file on this box, so the link
// keeps working when the published baseUrl points at a CDN this server is only
// the origin for.
func downloadHref(urls []string) string {
	if href := localHref(urls); href != "" {
		return href
	}
	if len(urls) > 0 {
		return urls[0]
	}
	return ""
}

func localHref(urls []string) string {
	for _, raw := range urls {
		if key, ok := localKeyFromURL(raw); ok {
			return "/" + key
		}
	}
	return ""
}

func shortHash(sum string) string {
	if len(sum) <= 12 {
		return sum
	}
	return sum[:12]
}

func formatTimestamp(raw, layout string) string {
	if raw == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	return parsed.Format(layout)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func breadcrumbs(name string) []crumb {
	crumbs := []crumb{{Label: "Releases", Href: adminPath}}
	if name == "." {
		crumbs = append(crumbs, crumb{Label: "files"})
		return crumbs
	}
	crumbs = append(crumbs, crumb{Label: "files", Href: adminFilesPath})
	segments := strings.Split(name, "/")
	href := ""
	for i, segment := range segments {
		href += url.PathEscape(segment) + "/"
		if i == len(segments)-1 {
			crumbs = append(crumbs, crumb{Label: segment})
			continue
		}
		crumbs = append(crumbs, crumb{Label: segment, Href: "/" + href})
	}
	return crumbs
}

//go:embed templates/*.html
var templateFS embed.FS

// Operator HTML lives in templates/. Edit those files and re-run tests, or
// `go run ./internal/browse/preview` for the public dump pages.
var pageTemplates = template.Must(template.New("pages").ParseFS(templateFS, "templates/*.html"))
