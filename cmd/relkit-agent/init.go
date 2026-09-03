package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	relkitconfig "cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/model"
)

const defaultProductRootPrefix = "/srv/relkit"

func runInit(out io.Writer, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "relkit-agent.json", "agent config path")
	product := fs.String("product", "", "add, rotate, or remove this product id")
	root := fs.String("root", "", "with -product: product tree root (default /srv/relkit/<id>)")
	list := fs.Bool("list-products", false, "list products in the config and exit")
	remove := fs.Bool("remove", false, "with -product: drop that product from the map")
	tokenOnly := fs.Bool("token-only", false, "with -product: rotate that product's upload token")
	migrateProfile := fs.Bool("migrate-profile", false, "with -product: extract a machine publish profile from product-root relkit.json, then rename that file aside")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// List and remove must not create the config directory: a typo in -config
	// would otherwise become a silent "no products" listing.
	switch {
	case *list:
		if *product != "" || *remove || *root != "" || *migrateProfile || *tokenOnly {
			return fmt.Errorf("-list-products takes no other arguments")
		}
		return runInitListProducts(out, *configPath)
	case *remove:
		if *product == "" {
			return fmt.Errorf("-remove needs -product <id>")
		}
		if *root != "" {
			return fmt.Errorf("-remove cannot be combined with -root")
		}
		if *migrateProfile {
			return fmt.Errorf("-remove cannot be combined with -migrate-profile")
		}
		if *tokenOnly {
			return fmt.Errorf("-remove cannot be combined with -token-only")
		}
		return runInitRemoveProduct(out, *configPath, *product)
	case *tokenOnly:
		if *product == "" {
			return fmt.Errorf("-token-only needs -product <id>")
		}
		if *root != "" {
			return fmt.Errorf("-token-only cannot be combined with -root")
		}
		if *migrateProfile {
			return fmt.Errorf("-token-only cannot be combined with -migrate-profile")
		}
		return runInitRotateProductToken(out, *configPath, *product)
	case *migrateProfile:
		if *product == "" {
			return fmt.Errorf("-migrate-profile needs -product <id>")
		}
		if *root != "" {
			return fmt.Errorf("-migrate-profile cannot be combined with -root")
		}
		return runInitMigrateProfile(out, *configPath, *product)
	}

	if *product == "" {
		return fmt.Errorf("need -list-products, -product <id>, or -product <id> -remove")
	}
	return runInitAddProduct(out, *configPath, *product, *root)
}

func loadInitConfig(configPath string) (*FileConfig, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%s does not exist; install the agent first", configPath)
	}
	cfg, err := loadFileConfig(configPath)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("config %s is empty", configPath)
	}
	return cfg, nil
}

func runInitListProducts(out io.Writer, configPath string) error {
	cfg, err := loadInitConfig(configPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "config %s\n", configPath)
	if cfg.UploadToken != "" || cfg.UploadTokenFile != "" {
		fmt.Fprintf(out, "FORBIDDEN instance token still in json; agent will refuse to start until it is deleted\n")
	}
	if len(cfg.UploadTokens) == 0 {
		fmt.Fprintf(out, "uploadTokens  none\n")
	} else {
		fmt.Fprintf(out, "uploadTokens\n")
		for _, entry := range cfg.UploadTokens {
			fmt.Fprintf(out, "  %-24s %s\n", strings.Join(entry.Products, ","), entry.File)
		}
	}
	if len(cfg.Products) == 0 {
		fmt.Fprintf(out, "products  none\n")
		return nil
	}
	names := make([]string, 0, len(cfg.Products))
	for name := range cfg.Products {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintf(out, "products\n")
	for _, name := range names {
		product := cfg.Products[name]
		profile := product.Profile
		if profile == "" {
			profile = defaultProfilePath(configPath, name)
		}
		fmt.Fprintf(out, "  %-24s %s  profile=%s\n", name, product.Root, profile)
	}
	return nil
}

func productTokenRelPath(product string) string {
	return "tokens/" + product + ".token"
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

func (c *FileConfig) productTokenFile(product string) (string, bool) {
	if c == nil {
		return "", false
	}
	for _, entry := range c.UploadTokens {
		for _, id := range entry.Products {
			if id == product {
				return entry.File, true
			}
		}
	}
	return "", false
}

func (c *FileConfig) upsertProductToken(product, relFile string) {
	for i, entry := range c.UploadTokens {
		if len(entry.Products) == 1 && entry.Products[0] == product {
			c.UploadTokens[i].File = relFile
			return
		}
	}
	c.UploadTokens = append(c.UploadTokens, UploadTokenEntry{
		File:     relFile,
		Products: []string{product},
	})
}

func (c *FileConfig) removeProductToken(product string) (string, bool) {
	if c == nil {
		return "", false
	}
	var kept []UploadTokenEntry
	orphan := ""
	found := false
	for _, entry := range c.UploadTokens {
		hit := false
		var products []string
		for _, id := range entry.Products {
			if id == product {
				hit = true
				continue
			}
			products = append(products, id)
		}
		if !hit {
			kept = append(kept, entry)
			continue
		}
		found = true
		if len(products) == 0 {
			orphan = entry.File
			continue
		}
		entry.Products = products
		kept = append(kept, entry)
	}
	if !found {
		return "", false
	}
	c.UploadTokens = kept
	return orphan, true
}

func (c *FileConfig) stripInstanceToken() {
	c.UploadToken = ""
	c.UploadTokenFile = ""
}

func runInitRotateProductToken(out io.Writer, configPath, product string) error {
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return err
	}
	cfg, err := loadInitConfig(configPath)
	if err != nil {
		return err
	}
	rel, ok := cfg.productTokenFile(product)
	if !ok {
		return fmt.Errorf("%s has no product token; run -product %s first", product, product)
	}
	path := rel
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(configPath), filepath.FromSlash(rel))
	}
	token, err := writeTokenFile(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "token  %s (mode 0600, replaced)\n", path)
	fmt.Fprintf(out, "\nRestart the service to load it. This product's publisher needs the new value first:\n")
	fmt.Fprintf(out, "  export RELKIT_UPLOAD_TOKEN='%s'\n", token)
	return nil
}

func runInitAddProduct(out io.Writer, configPath, product, root string) error {
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return err
	}
	root = strings.TrimSpace(root)
	if root == "" {
		root = filepath.Join(defaultProductRootPrefix, product)
	}

	cfg, err := loadInitConfig(configPath)
	if err != nil {
		return err
	}
	if cfg.Products == nil {
		cfg.Products = map[string]ProductConfig{}
	}

	issuingOnly := false
	if existing, ok := cfg.Products[product]; ok {
		if _, hasToken := cfg.productTokenFile(product); hasToken {
			if existing.Root == root {
				return fmt.Errorf("%s is already listed (root %s)", product, existing.Root)
			}
			return fmt.Errorf("%s is already listed with root %s; -remove it first to change", product, existing.Root)
		}
		issuingOnly = true
		root = existing.Root
	} else {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return err
		}
		cfg.Products[product] = ProductConfig{Root: root}
	}

	relFile := productTokenRelPath(product)
	tokenPath := filepath.Join(filepath.Dir(configPath), filepath.FromSlash(relFile))
	token, err := writeTokenFile(tokenPath)
	if err != nil {
		return err
	}
	cfg.upsertProductToken(product, relFile)
	cfg.stripInstanceToken()
	if err := writeFileConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(out, "config  %s\n", configPath)
	fmt.Fprintf(out, "product %s\n", product)
	fmt.Fprintf(out, "root    %s\n", root)
	fmt.Fprintf(out, "token   %s (mode 0600)\n", tokenPath)
	if issuingOnly {
		fmt.Fprintf(out, "stripped instance-wide uploadToken / uploadTokenFile if they were present\n")
	}
	fmt.Fprintf(out, "\nThis product's publisher needs this token:\n")
	fmt.Fprintf(out, "  export RELKIT_UPLOAD_TOKEN='%s'\n", token)
	fmt.Fprintf(out, "Put signing keys under the root, then create the machine publish profile.\n")
	fmt.Fprintf(out, "A repository release-policy.json will arrive with each staged release.\n")
	fmt.Fprintf(out, "If this directory was created as root, give it to the service user:\n")
	fmt.Fprintf(out, "  chown -R relkit:relkit %s\n", root)
	fmt.Fprintf(out, "  chown relkit:relkit %s\n", tokenPath)
	fmt.Fprintf(out, "Restart the service to load it:\n")
	fmt.Fprintf(out, "  systemctl restart relkit-agent\n")
	return nil
}

func runInitMigrateProfile(out io.Writer, configPath, product string) error {
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return err
	}
	cfg, err := loadInitConfig(configPath)
	if err != nil {
		return err
	}
	pc, ok := cfg.Products[product]
	if !ok {
		return fmt.Errorf("%s is not listed in products", product)
	}
	root := pc.Root
	if !filepath.IsAbs(root) {
		root = filepath.Join(filepath.Dir(configPath), root)
	}
	profilePath := pc.Profile
	if profilePath == "" {
		profilePath = defaultProfilePath(configPath, product)
		pc.Profile = filepath.ToSlash(filepath.Join("products", product+".json"))
	} else if !filepath.IsAbs(profilePath) {
		profilePath = filepath.Join(filepath.Dir(configPath), profilePath)
	}
	if _, err := os.Stat(profilePath); err == nil {
		return fmt.Errorf("publish profile already exists: %s", profilePath)
	} else if !os.IsNotExist(err) {
		return err
	}

	legacyPath := filepath.Join(root, relkitconfig.ConfigName)
	legacy, err := relkitconfig.Load(legacyPath)
	if err != nil {
		return fmt.Errorf("load product-root config: %w", err)
	}
	if legacy.Product != product {
		return fmt.Errorf("product-root config product %q does not match %q", legacy.Product, product)
	}
	profile, err := relkitconfig.ExtractPublishProfile(legacy)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	// Publish profiles contain no secret values and must be traversable by the
	// unprivileged service user even when init runs as root.
	if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
		return err
	}
	// Profile contains endpoint and environment-variable names, never secret
	// values. Keep it world-readable so a root-run init remains readable by the
	// unprivileged relkit-agent service user.
	if err := os.WriteFile(profilePath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	cfg.Products[product] = pc
	if err := writeFileConfig(configPath, cfg); err != nil {
		_ = os.Remove(profilePath)
		return err
	}
	// Agent never reads product-root relkit.json. Rename it aside so it cannot
	// be mistaken for a second publish source.
	asidePath := legacyPath + ".migrated"
	if _, err := os.Stat(asidePath); err == nil {
		asidePath = fmt.Sprintf("%s.migrated.%d", legacyPath, time.Now().Unix())
	} else if err != nil && !os.IsNotExist(err) {
		_ = os.Remove(profilePath)
		return err
	}
	if err := os.Rename(legacyPath, asidePath); err != nil {
		_ = os.Remove(profilePath)
		return fmt.Errorf("wrote profile but failed to move %s aside: %w", legacyPath, err)
	}
	fmt.Fprintf(out, "config  %s\n", configPath)
	fmt.Fprintf(out, "product %s\n", product)
	fmt.Fprintf(out, "profile %s\n", profilePath)
	fmt.Fprintf(out, "source  %s -> %s\n", legacyPath, asidePath)
	fmt.Fprintf(out, "\nPublish now requires staged release-policy.json + the profile above.\n")
	fmt.Fprintf(out, "Restart relkit-agent after deploying a build that supports release-policy.json.\n")
	return nil
}

func defaultProfilePath(configPath, product string) string {
	return filepath.Join(filepath.Dir(configPath), "products", product+".json")
}

func runInitRemoveProduct(out io.Writer, configPath, product string) error {
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return err
	}
	cfg, err := loadInitConfig(configPath)
	if err != nil {
		return err
	}
	if _, ok := cfg.Products[product]; !ok {
		return fmt.Errorf("%s is not listed in products", product)
	}
	delete(cfg.Products, product)
	if cfg.Products == nil {
		cfg.Products = map[string]ProductConfig{}
	}
	relFile, _ := cfg.removeProductToken(product)
	if err := writeFileConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(out, "config  %s\n", configPath)
	fmt.Fprintf(out, "removed %s (product root left on disk)\n", product)
	if relFile != "" {
		path := relFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(configPath), filepath.FromSlash(relFile))
		}
		switch err := os.Remove(path); {
		case err == nil:
			fmt.Fprintf(out, "token   %s deleted\n", path)
		case os.IsNotExist(err):
			fmt.Fprintf(out, "token   %s was already gone\n", path)
		default:
			return err
		}
	}
	fmt.Fprintf(out, "\nRestart the service to load it:\n")
	fmt.Fprintf(out, "  systemctl restart relkit-agent\n")
	return nil
}
