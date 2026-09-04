package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"cnb.cool/shichao402/relkit/internal/humansize"
	"cnb.cool/shichao402/relkit/internal/stagedput"
)

func cmdStagedPut(args []string) error {
	opts := stagedput.Options{
		URL:         strings.TrimSpace(os.Getenv("RELKIT_AGENT_URL")),
		Token:       strings.TrimSpace(os.Getenv("RELKIT_UPLOAD_TOKEN")),
		PartSize:    stagedput.DefaultPartSizeFromEnv(),
		Concurrency: stagedput.DefaultConcurrencyFromEnv(),
		Log:         func(line string) { fmt.Fprintln(os.Stderr, line) },
	}
	var file string
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
		case arg == "--part-size":
			i++
			n, err := humansize.Parse(mustValue(args, i, "--part-size"))
			if err != nil || n < 1 {
				return fmt.Errorf("--part-size: %v", err)
			}
			opts.PartSize = n
		case arg == "--concurrency":
			i++
			n, err := strconv.Atoi(mustValue(args, i, "--concurrency"))
			if err != nil || n < 1 {
				return fmt.Errorf("--concurrency: invalid value")
			}
			opts.Concurrency = n
		case arg == "--single":
			opts.Single = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		case file == "":
			file = arg
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	opts.File = file
	if opts.File == "" || opts.Product == "" || opts.Version == "" {
		return fmt.Errorf("usage: relkit staged-put FILE --product ID --version VER --url URL [--part-size 8MiB] [--concurrency 8] [--single]")
	}
	result, err := stagedput.Put(context.Background(), opts)
	if err != nil {
		return err
	}
	fmt.Printf("staged %s/%s sha256=%s bytes=%d\n", opts.Product, opts.Version, result.SHA256, result.Bytes)
	return nil
}
