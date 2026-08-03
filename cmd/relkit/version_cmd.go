package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/jsonio"
	projver "github.com/shichao402/relkit/version"
)

func cmdVersion(args []string, configPath string) error {
	if len(args) == 0 {
		return fmt.Errorf("version requires a subcommand: get, set, bump, code, path")
	}
	switch args[0] {
	case "get":
		return cmdVersionGet(args[1:], configPath)
	case "set":
		return cmdVersionSet(args[1:], configPath)
	case "bump":
		return cmdVersionBump(args[1:], configPath)
	case "code":
		return cmdVersionCode(args[1:], configPath)
	case "path":
		return cmdVersionPath(args[1:], configPath)
	default:
		return fmt.Errorf("unknown version subcommand %q", args[0])
	}
}

func cmdVersionGet(args []string, configPath string) error {
	field := "version"
	asJSON := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			asJSON = true
		case arg == "--field":
			i++
			field = mustValue(args, i, "--field")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}

	doc, err := loadProjectVersion(configPath)
	if err != nil {
		return err
	}
	parts, err := projver.Parse(doc.Version)
	if err != nil {
		return err
	}

	if asJSON {
		payload := map[string]any{
			"schema":        projver.SchemaID,
			"version":       parts.String(),
			"versionNumber": parts.Number(),
			"major":         parts.Major,
			"minor":         parts.Minor,
			"patch":         parts.Patch,
			"build":         parts.Build,
			"path":          doc.Path,
		}
		if field == "code" || field == "all" {
			code, err := resolveProjectCode(configPath, parts.String(), nil)
			if err != nil {
				return err
			}
			payload["code"] = code
		}
		data, err := jsonio.MarshalPretty(payload)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(data)
		return err
	}

	switch field {
	case "version":
		fmt.Println(parts.String())
	case "number", "versionNumber":
		fmt.Println(parts.Number())
	case "build":
		fmt.Println(parts.Build)
	case "major":
		fmt.Println(parts.Major)
	case "minor":
		fmt.Println(parts.Minor)
	case "patch":
		fmt.Println(parts.Patch)
	case "code":
		code, err := resolveProjectCode(configPath, parts.String(), nil)
		if err != nil {
			return err
		}
		fmt.Println(code)
	default:
		return fmt.Errorf("unknown --field %q; expected version, number, build, major, minor, patch, or code", field)
	}
	return nil
}

func cmdVersionSet(args []string, configPath string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: relkit version set <x.y.z+build>")
	}
	root, err := projectRoot(configPath)
	if err != nil {
		return err
	}
	path := filepath.Join(root, projver.FileName)
	var doc *projver.Document
	if _, err := os.Stat(path); os.IsNotExist(err) {
		doc, err = projver.Skeleton(args[0])
		if err != nil {
			return err
		}
		doc.Path = path
	} else {
		doc, err = projver.LoadPath(path)
		if err != nil {
			return err
		}
		if err := doc.SetVersion(args[0]); err != nil {
			return err
		}
	}
	return writeVersion(doc)
}

func cmdVersionBump(args []string, configPath string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: relkit version bump major|minor|patch|build")
	}
	doc, err := loadProjectVersion(configPath)
	if err != nil {
		return err
	}
	parts, err := doc.Bump(args[0])
	if err != nil {
		return err
	}
	if err := writeVersion(doc); err != nil {
		return err
	}
	fmt.Println(parts.String())
	return nil
}

func cmdVersionCode(args []string, configPath string) error {
	if len(args) > 0 {
		return fmt.Errorf("version code takes no arguments; use 'version get --field code'")
	}
	return cmdVersionGet([]string{"--field", "code"}, configPath)
}

func cmdVersionPath(args []string, configPath string) error {
	if len(args) > 0 {
		return fmt.Errorf("version path takes no arguments")
	}
	doc, err := loadProjectVersion(configPath)
	if err != nil {
		return err
	}
	fmt.Println(doc.Path)
	return nil
}

func loadProjectVersion(configPath string) (*projver.Document, error) {
	root, err := projectRoot(configPath)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, projver.FileName)
	if _, err := os.Stat(path); err == nil {
		return projver.LoadPath(path)
	}
	// Fall back to walk from cwd when config is absent / elsewhere.
	return projver.Load(root)
}

func projectRoot(configPath string) (string, error) {
	if configPath != "" {
		resolved, err := filepath.Abs(configPath)
		if err != nil {
			return "", err
		}
		return filepath.Dir(resolved), nil
	}
	if found, err := config.FindConfig(""); err == nil && found != "" {
		return filepath.Dir(found), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return cwd, nil
}

func resolveProjectCode(configPath, versionString string, explicit *int) (int, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		parts, perr := projver.Parse(versionString)
		if perr != nil {
			return 0, err
		}
		// Without relkit.json, version-build semantics: code == +build.
		if explicit != nil {
			return *explicit, nil
		}
		return parts.Build, nil
	}
	return cfg.ResolveCode(versionString, explicit)
}

func writeVersion(doc *projver.Document) error {
	if err := doc.Write(); err != nil {
		return err
	}
	fmt.Println("wrote " + doc.Path)
	fmt.Println(doc.Version)
	return nil
}

func resolveStageVersion(opts *stageArgs, configPath string) error {
	if opts.version != "" {
		return nil
	}
	doc, err := loadProjectVersion(configPath)
	if err != nil {
		return fmt.Errorf("stage requires a version argument or a %s: %w", projver.FileName, err)
	}
	opts.version = doc.Version
	return nil
}

func resolvePublishVersion(versionArg, configPath string) (string, error) {
	if versionArg != "" {
		return versionArg, nil
	}
	doc, err := loadProjectVersion(configPath)
	if err != nil {
		return "", fmt.Errorf("publish requires a version argument or a %s: %w", projver.FileName, err)
	}
	return doc.Version, nil
}
