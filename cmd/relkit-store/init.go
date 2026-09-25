package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.firoyang.com/relkit/internal/model"
)

// runInit writes a config skeleton plus a freshly generated token, so that a
// working deployment needs no hand-written secrets and no guessing about which
// cache prefixes matter.
//
// The admin-panel half of serve's init (bootstrap minting, -reset-admin) moved
// to relkit-console; a store box has no panel and therefore no bootstrap.
func runInit(out io.Writer, args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	dir := fs.String("dir", "/srv/releases", "directory the server will serve")
	target := fs.String("out", ".", "where to write the config and token")
	force := fs.Bool("force", false, "overwrite existing files")
	tokenOnly := fs.Bool("token-only", false, "generate a new token and leave the config alone")
	product := fs.String("product", "", "add or rotate a product-scoped upload token")
	shareWith := fs.String("share-with", "", "with -product: grant the same token as this existing product")
	list := fs.Bool("list-products", false, "list the product-scoped upload tokens and exit")
	remove := fs.Bool("remove", false, "with -product: revoke that product's upload token")
	fs.Parse(args)

	outDir := *target
	configPath := filepath.Join(outDir, ConfigName)

	// Listing, revoking, and sharing run before MkdirAll: none of them should
	// bring into existence the directory it was pointed at.
	switch {
	case *list:
		if *product != "" || *remove || *shareWith != "" {
			return fmt.Errorf("-list-products takes no other arguments")
		}
		return runInitListProducts(out, configPath)
	case *remove:
		if *product == "" {
			return fmt.Errorf("-remove needs -product <id>")
		}
		if *tokenOnly || *force || *shareWith != "" {
			return fmt.Errorf("-remove cannot be combined with -token-only, -force, or -share-with")
		}
		return runInitRemoveProduct(out, configPath, *product)
	}
	if *shareWith != "" {
		if *product == "" {
			return fmt.Errorf("-share-with needs -product <id>")
		}
		if *tokenOnly || *force {
			return fmt.Errorf("-share-with cannot be combined with -token-only or -force")
		}
		return runInitShareProduct(out, configPath, *product, *shareWith)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	if *product != "" {
		return runInitProduct(out, outDir, *dir, *product, *force, *tokenOnly)
	}

	tokenPath := filepath.Join(outDir, "relkit-store.token")

	// Rotating a token must not touch the config. Anything hand-edited there
	// (listen address, cache prefixes) would be silently reverted to defaults,
	// and a reverted cache prefix shows up only as releases arriving late.
	written := []string{tokenPath}
	if !*tokenOnly {
		written = append(written, configPath)
	}
	for _, path := range written {
		if _, err := os.Stat(path); err == nil && !*force && !*tokenOnly {
			return fmt.Errorf("%s already exists; pass -force to overwrite "+
				"(this invalidates the current token)", path)
		}
	}

	token, err := generateToken()
	if err != nil {
		return err
	}

	// 0600 from the start rather than written then tightened: between those two
	// steps the token would be world-readable, and on a shared box that is long
	// enough.
	if err := os.WriteFile(tokenPath, []byte(token+"\n"), 0o600); err != nil {
		return err
	}
	if *tokenOnly {
		fmt.Fprintf(out, "token  %s (mode 0600, replaced)\n", tokenPath)
		fmt.Fprintf(out, "\nRestart the service to load it. Every publisher needs "+
			"the new value first:\n")
		fmt.Fprintf(out, "  export RELKIT_UPLOAD_TOKEN='%s'\n", token)
		return nil
	}

	if err := os.WriteFile(configPath, skeletonBytes(*dir, ""), 0o644); err != nil {
		return err
	}

	fmt.Fprintf(out, "config %s\n", configPath)
	fmt.Fprintf(out, "token  %s (mode 0600)\n", tokenPath)
	fmt.Fprintf(out, "\nThe publisher needs this token in its environment:\n")
	fmt.Fprintf(out, "  export RELKIT_UPLOAD_TOKEN='%s'\n\n", token)
	fmt.Fprintf(out, "\nServing directory is %s; create it before starting.\n", *dir)
	fmt.Fprintf(out, "Then: relkit-store -config %s\n", configPath)
	fmt.Fprintf(out, "Product token ops: product-repo python scripts/host/relkit_host.py serve\n")
	fmt.Fprintf(out, "Operator panel: relkit-console init (separate binary, ADR 0016)\n")
	return nil
}

func runInitShareProduct(out io.Writer, configPath, product, with string) error {
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return err
	}
	if err := model.CheckIdentifier(with, "product"); err != nil {
		return err
	}
	if product == with {
		return fmt.Errorf("-product and -share-with must be different ids")
	}

	cfg, err := loadInitConfig(configPath)
	if err != nil {
		return err
	}
	relFile, err := cfg.shareProductToken(product, with)
	if err != nil {
		return err
	}
	relFile, err = promoteSharedTokenFile(configPath, cfg, relFile)
	if err != nil {
		return err
	}
	if err := writeFileConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(out, "config %s\n", configPath)
	fmt.Fprintf(out, "token  %s (shared; products now include %s)\n", relFile, product)
	fmt.Fprintf(out, "\nNo new secret. Reuse the RELKIT_UPLOAD_TOKEN already held for %s.\n", with)
	fmt.Fprintf(out, "Restart the service to load it:\n")
	fmt.Fprintf(out, "  systemctl restart relkit-store\n")
	return nil
}

func runInitProduct(out io.Writer, outDir, serveDir, product string, force, tokenOnly bool) error {
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return err
	}

	configPath := filepath.Join(outDir, ConfigName)
	relFile := productTokenRelPath(product)
	tokenPath := filepath.Join(outDir, filepath.FromSlash(relFile))

	if tokenOnly {
		cfg, _, err := LoadFileConfig(configPath)
		if err != nil {
			return err
		}
		rel, ok := cfg.productTokenFile(product)
		if !ok {
			return fmt.Errorf("%s is not listed in uploadTokens; omit -token-only to add it", product)
		}
		path := resolveRelative(rel, configPath)
		token, err := writeTokenFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "token  %s (mode 0600, replaced)\n", path)
		fmt.Fprintf(out, "\nRestart the service to load it. This product's publisher needs "+
			"the new value first:\n")
		fmt.Fprintf(out, "  export RELKIT_UPLOAD_TOKEN='%s'\n", token)
		return nil
	}

	if _, err := os.Stat(tokenPath); err == nil && !force {
		return fmt.Errorf("%s already exists; pass -force to overwrite "+
			"(this invalidates the current token) or -token-only to rotate", tokenPath)
	}

	token, err := writeTokenFile(tokenPath)
	if err != nil {
		return err
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, skeletonBytes(serveDir, ""), 0o644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	cfg, _, err := LoadFileConfig(configPath)
	if err != nil {
		return err
	}
	if cfg == nil {
		return fmt.Errorf("config %s is empty", configPath)
	}
	cfg.upsertProductToken(product, relFile)
	if err := writeFileConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(out, "config %s\n", configPath)
	fmt.Fprintf(out, "token  %s (mode 0600, product %s)\n", tokenPath, product)
	fmt.Fprintf(out, "\nThis product's publisher needs this token in its environment:\n")
	fmt.Fprintf(out, "  export RELKIT_UPLOAD_TOKEN='%s'\n", token)
	return nil
}

// runInitListProducts reports which products this box will accept uploads for.
//
// Ids and file paths only. An inventory that printed the tokens it inventories
// could not be run in front of anyone, and the point of listing is to answer
// "is this product still allowed", not "what is its secret".
func runInitListProducts(out io.Writer, configPath string) error {
	cfg, err := loadInitConfig(configPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "config %s\n", configPath)
	switch {
	case cfg.UploadToken != "":
		fmt.Fprintf(out, "operator  inline in config (full tree)\n")
	case cfg.UploadTokenFile != "":
		fmt.Fprintf(out, "operator  %s (full tree)\n", cfg.UploadTokenFile)
	default:
		fmt.Fprintf(out, "operator  none\n")
	}
	if len(cfg.UploadTokens) == 0 {
		fmt.Fprintf(out, "products  none\n")
		return nil
	}
	fmt.Fprintf(out, "products\n")
	for _, entry := range cfg.UploadTokens {
		fmt.Fprintf(out, "  %-24s %s\n", strings.Join(entry.Products, ","), entry.File)
	}
	return nil
}

// runInitRemoveProduct revokes a product-scoped token: the entry leaves
// uploadTokens and its token file is deleted. Everything else in the config,
// including the operator token and the cache prefixes, is left as written.
func runInitRemoveProduct(out io.Writer, configPath, product string) error {
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return err
	}
	cfg, err := loadInitConfig(configPath)
	if err != nil {
		return err
	}
	relFile, ok := cfg.removeProductToken(product)
	if !ok {
		return fmt.Errorf("%s is not listed in uploadTokens", product)
	}

	// Config first, file second. In the other order a crash in between leaves
	// the config pointing at a token file that no longer exists, and the
	// server refuses to start at all rather than starting without one product.
	if err := writeFileConfig(configPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(out, "config %s (product %s removed)\n", configPath, product)

	if relFile == "" {
		fmt.Fprintf(out, "token  kept: that file is shared with another product\n")
	} else {
		path := resolveRelative(relFile, configPath)
		switch err := os.Remove(path); {
		case err == nil:
			fmt.Fprintf(out, "token  %s deleted\n", path)
		case os.IsNotExist(err):
			fmt.Fprintf(out, "token  %s was already gone\n", path)
		default:
			return err
		}
	}

	fmt.Fprintf(out, "\nThe running process keeps accepting the old token until it reloads:\n")
	fmt.Fprintf(out, "  systemctl restart relkit-store\n")
	return nil
}

func loadInitConfig(configPath string) (*FileConfig, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%s does not exist; run relkit-store init first", configPath)
	}
	cfg, _, err := LoadFileConfig(configPath)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("config %s is empty", configPath)
	}
	return cfg, nil
}

func writeTokenFile(path string) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	return token, nil
}

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// loadTokenFile reads a token from a file.
//
// There is deliberately no flag that takes the token itself: it would be
// visible in `ps` to every user on the box and would land in shell history.
func loadTokenFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return nil, fmt.Errorf("%s is empty", path)
	}
	return hashToken(token), nil
}

// hashToken reduces the token to a fixed-size digest so that comparison is
// constant-time in a way that cannot leak the length either.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func splitPrefixes(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
