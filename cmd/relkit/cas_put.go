package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"cnb.cool/shichao402/relkit/internal/casput"
	"cnb.cool/shichao402/relkit/internal/config"
)

func cmdCASPut(args []string, configPath string) error {
	opts := casput.Options{
		URL:         strings.TrimSpace(os.Getenv("RELKIT_AGENT_URL")),
		Token:       strings.TrimSpace(os.Getenv("RELKIT_UPLOAD_TOKEN")),
		Concurrency: 4,
		Log:         func(line string) { fmt.Fprintln(os.Stderr, line) },
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--product":
			i++
			opts.Product = mustValue(args, i, "--product")
		case arg == "--version":
			i++
			opts.Version = mustValue(args, i, "--version")
		case arg == "--url":
			i++
			opts.URL = mustValue(args, i, "--url")
		case arg == "--token":
			i++
			opts.Token = mustValue(args, i, "--token")
		case arg == "--concurrency":
			i++
			value, err := strconv.Atoi(mustValue(args, i, "--concurrency"))
			if err != nil || value < 1 {
				return fmt.Errorf("--concurrency must be a positive integer")
			}
			opts.Concurrency = value
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	opts.Root = cfg.Root
	if opts.Product == "" {
		opts.Product = cfg.Product
	}
	if opts.Version == "" {
		return fmt.Errorf("usage: relkit cas-put --version VER [--product ID] [--url URL] [--concurrency 4]")
	}
	result, err := casput.Put(context.Background(), opts)
	if err != nil {
		return err
	}
	fmt.Printf(
		"cas uploaded=%d skipped=%d; staged %s/%s sha256=%s bytes=%d\n",
		result.Uploaded, result.Skipped, opts.Product, opts.Version, result.StagedSHA256, result.StagedBytes,
	)
	return nil
}
