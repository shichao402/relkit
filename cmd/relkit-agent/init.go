package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	relkitconfig "cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/model"
)

const defaultProductRootPrefix = "/srv/relkit"

func runInit(out io.Writer, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "relkit-agent.json", "agent config path")
	product := fs.String("product", "", "add or remove this product id")
	root := fs.String("root", "", "with -product: product tree root (default /srv/relkit/<id>)")
	list := fs.Bool("list-products", false, "list products in the config and exit")
	remove := fs.Bool("remove", false, "with -product: drop that product from the map")
	migrateProfile := fs.Bool("migrate-profile", false, "with -product: extract a machine publish profile from the legacy relkit.json")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// List and remove must not create the config directory: a typo in -config
	// would otherwise become a silent "no products" listing.
	switch {
	case *list:
		if *product != "" || *remove || *root != "" || *migrateProfile {
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
		return runInitRemoveProduct(out, *configPath, *product)
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
	switch {
	case cfg.UploadToken != "":
		fmt.Fprintf(out, "token   inline in config\n")
	case cfg.UploadTokenFile != "":
		fmt.Fprintf(out, "token   %s\n", cfg.UploadTokenFile)
	default:
		fmt.Fprintf(out, "token   none\n")
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
	if existing, ok := cfg.Products[product]; ok {
		if existing.Root == root {
			return fmt.Errorf("%s is already listed (root %s)", product, existing.Root)
		}
		return fmt.Errorf("%s is already listed with root %s; -remove it first to change", product, existing.Root)
	}

	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	cfg.Products[product] = ProductConfig{Root: root}
	if err := writeFileConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(out, "config  %s\n", configPath)
	fmt.Fprintf(out, "product %s\n", product)
	fmt.Fprintf(out, "root    %s\n", root)
	fmt.Fprintf(out, "\nNo new secret. Reuse the existing RELKIT_AGENT_TOKEN.\n")
	fmt.Fprintf(out, "Put signing keys under the root, then create the machine publish profile.\n")
	fmt.Fprintf(out, "A repository release-policy.json will arrive with each staged release.\n")
	fmt.Fprintf(out, "If this directory was created as root, give it to the service user:\n")
	fmt.Fprintf(out, "  chown -R relkit:relkit %s\n", root)
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
	legacyPath := filepath.Join(root, relkitconfig.ConfigName)
	legacy, err := relkitconfig.Load(legacyPath)
	if err != nil {
		return fmt.Errorf("load legacy config: %w", err)
	}
	if legacy.Product != product {
		return fmt.Errorf("legacy config product %q does not match %q", legacy.Product, product)
	}
	profile, err := relkitconfig.ExtractPublishProfile(legacy)
	if err != nil {
		return err
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
	fmt.Fprintf(out, "config  %s\n", configPath)
	fmt.Fprintf(out, "product %s\n", product)
	fmt.Fprintf(out, "profile %s\n", profilePath)
	fmt.Fprintf(out, "source  %s (legacy fallback left in place)\n", legacyPath)
	fmt.Fprintf(out, "\nRestart relkit-agent after deploying a build that supports release-policy.json.\n")
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
	if err := writeFileConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(out, "config  %s\n", configPath)
	fmt.Fprintf(out, "removed %s (product root left on disk)\n", product)
	fmt.Fprintf(out, "\nRestart the service to load it:\n")
	fmt.Fprintf(out, "  systemctl restart relkit-agent\n")
	return nil
}
