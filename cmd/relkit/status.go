package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/shichao402/relkit/internal/consume"
)

// cmdConsumeStatus implements `relkit status`: report the detected stack,
// the consume component selection, and per-component install health against
// the lock. Mirrors the product CI surface of relkit_host.py cmd_status
// without the remote-ops sections (serve/agent stay in Python until phase 2).
func cmdConsumeStatus(args []string) error {
	lockPath := ""
	root := ""
	target := "host"
	asJSON := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--lock":
			i++
			lockPath = mustValue(args, i, "--lock")
		case arg == "--project-root":
			i++
			root = mustValue(args, i, "--project-root")
		case arg == "--target":
			i++
			target = mustValue(args, i, "--target")
		case arg == "--json":
			asJSON = true
		default:
			return fmt.Errorf("unknown flag %q", arg)
		}
	}

	if root == "" {
		root = "."
	}
	if lockPath == "" {
		lockPath = filepath.Join(root, "scripts", "relkit.lock.json")
	}

	lock, err := consume.LoadLock(lockPath)
	if err != nil {
		return err
	}
	lock, err = resolveFollowLatest(root, lock, lockPath)
	if err != nil {
		return err
	}
	if target == "host" {
		target, err = consume.HostTarget()
		if err != nil {
			return err
		}
	}

	stack, err := consume.DetectStack(root)
	if err != nil {
		return err
	}
	components, err := consume.ConsumeComponents(root)
	if err != nil {
		return err
	}

	if asJSON {
		report := map[string]any{
			"schema":     lock.Schema,
			"release":    lock.Release,
			"commit":     lock.Commit,
			"target":     target,
			"languages":  stack.Languages,
			"updater":    stack.Updater,
			"components": components,
		}
		out, err := marshalStatusJSON(report)
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}

	fmt.Printf("schema %s\n", lock.Schema)
	fmt.Printf("release %s (%s)\n", lock.Release, lock.Commit)
	fmt.Printf("target %s\n", target)
	fmt.Printf("languages %s\n", strings.Join(stack.Languages, ","))
	if stack.Updater != "" {
		fmt.Printf("updater %s\n", stack.Updater)
	}
	fmt.Println("components")
	for _, name := range components {
		result := consume.CheckInstalled(root, lock, name, target)
		state := "ok"
		if !result.OK {
			state = result.Detail
		}
		fmt.Printf("  %-14s %s\n", name, state)
	}
	return nil
}

func marshalStatusJSON(report map[string]any) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

// consumeComponentsFor resolves the component list for install: explicit
// --component flags win; otherwise the detected consume selection plus
// host-scripts first when the lock pins it (mirroring relkit_consume.main).
func consumeComponentsFor(root string, explicit []string, lock *consume.Lock) ([]string, error) {
	if len(explicit) > 0 {
		return explicit, nil
	}
	names, err := consume.ConsumeComponents(root)
	if err != nil {
		return nil, err
	}
	if lock == nil {
		return names, nil
	}
	pinned := lock.PinnedComponents()
	if pinned == nil || !pinned["host-scripts"] {
		return names, nil
	}
	for _, name := range names {
		if name == "host-scripts" {
			return names, nil
		}
	}
	return append([]string{"host-scripts"}, names...), nil
}
