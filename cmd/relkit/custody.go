package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/shichao402/relkit/internal/custody"
)

// cmdCustody is the script-facing entry point into the credential manifest.
//
// Programs in Go call internal/custody directly. Python and shell scripts must
// not parse the manifest themselves, so they ask this command for a value by
// keyId and the same reading module answers. That keeps one implementation of
// "where does this key live" for every language in the repo.
func cmdCustody(args []string) error {
	usage := `usage:
  relkit custody check  --manifest <path> [--key <keyId> ...] [--json]
  relkit custody get    --manifest <path> --key <keyId> [--field <field>]
  relkit custody public --manifest <path> --key <keyId> [--json]`

	if len(args) == 0 {
		return fmt.Errorf("%s", usage)
	}
	switch args[0] {
	case "check":
		return custodyCheck(args[1:])
	case "get":
		return custodyGet(args[1:])
	case "public":
		return custodyPublic(args[1:])
	default:
		return fmt.Errorf("unknown custody action %q\n%s", args[0], usage)
	}
}

func custodyCheck(args []string) error {
	manifestPath, keyIDs, asJSON, err := custodyFlags(args, true)
	if err != nil {
		return err
	}
	manifest, err := custody.Load(manifestPath)
	if err != nil {
		return err
	}
	if len(keyIDs) == 0 {
		for _, entry := range manifest.Keys {
			keyIDs = append(keyIDs, entry.KeyID)
		}
	}
	report, checkErr := manifest.SelfCheck(keyIDs...)
	if asJSON {
		if report != nil {
			data, encodeErr := report.Encode()
			if encodeErr != nil {
				return encodeErr
			}
			if _, writeErr := os.Stdout.Write(data); writeErr != nil {
				return writeErr
			}
		}
		if checkErr != nil {
			return checkErr
		}
		return nil
	}
	if report != nil {
		fmt.Printf("manifest : %s\n", manifestPath)
		fmt.Printf("scope    : %s\n", report.Scope)
		fmt.Println()
		for _, keyReport := range report.Keys {
			state := "OK  "
			if keyReport.Verified == "" {
				state = "FAIL"
			}
			fmt.Printf("[%s] %s  (%s via %s)\n", state, keyReport.KeyID, keyReport.Kind, keyReport.Carrier)
			if keyReport.Fingerprint != "" {
				fmt.Printf("          fingerprint %s\n", keyReport.Fingerprint)
			}
			if keyReport.Verified != "" {
				fmt.Printf("          verified    %s\n", keyReport.Verified)
			}
		}
		fmt.Println()
	}
	if checkErr != nil {
		fmt.Println("RESULT = FAIL")
		return checkErr
	}
	fmt.Println("RESULT = PASS")
	return nil
}

func custodyGet(args []string) error {
	manifestPath := ""
	keyID := ""
	field := ""
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--manifest":
			index++
			manifestPath = mustValue(args, index, "--manifest")
		case "--key":
			index++
			keyID = mustValue(args, index, "--key")
		case "--field":
			index++
			field = mustValue(args, index, "--field")
		default:
			return fmt.Errorf("unknown flag %q", args[index])
		}
	}
	if manifestPath == "" || keyID == "" {
		return fmt.Errorf("custody get requires --manifest and --key")
	}
	manifest, err := custody.Load(manifestPath)
	if err != nil {
		return err
	}
	// Scripts ask for a value, not a key type: the entry's kind decides what
	// the carrier is read as.
	switch field {
	case "", "value":
		entry, entryErr := manifest.Entry(keyID)
		if entryErr != nil {
			return entryErr
		}
		switch entry.Kind {
		case custody.KindRoot:
			seed, seedErr := manifest.PrivateSeed(keyID)
			if seedErr != nil {
				return seedErr
			}
			fmt.Println(custody.EncodeSeed(seed))
			return nil
		default:
			value, valueErr := manifest.ReadString(keyID)
			if valueErr != nil {
				return valueErr
			}
			fmt.Println(value)
			return nil
		}
	case "fingerprint":
		fingerprint, fingerprintErr := manifest.Fingerprint(keyID)
		if fingerprintErr != nil {
			return fingerprintErr
		}
		fmt.Println(fingerprint)
		return nil
	case "path":
		entry, entryErr := manifest.Entry(keyID)
		if entryErr != nil {
			return entryErr
		}
		fmt.Println(entry.Path)
		return nil
	default:
		return fmt.Errorf("unknown field %q (expected value, fingerprint or path)", field)
	}
}

func custodyPublic(args []string) error {
	manifestPath := ""
	keyID := ""
	asJSON := false
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--manifest":
			index++
			manifestPath = mustValue(args, index, "--manifest")
		case "--key":
			index++
			keyID = mustValue(args, index, "--key")
		case "--json":
			asJSON = true
		default:
			return fmt.Errorf("unknown flag %q", args[index])
		}
	}
	if manifestPath == "" || keyID == "" {
		return fmt.Errorf("custody public requires --manifest and --key")
	}
	manifest, err := custody.Load(manifestPath)
	if err != nil {
		return err
	}
	publicKey, err := manifest.PublicKey(keyID)
	if err != nil {
		return err
	}
	fingerprint, err := manifest.Fingerprint(keyID)
	if err != nil {
		return err
	}
	encoded := custody.EncodePublicKey(publicKey)
	if asJSON {
		payload := map[string]any{
			"keyId":       keyID,
			"fingerprint": fingerprint,
			"publicKey":   encoded,
		}
		data, marshalErr := json.MarshalIndent(payload, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		_, writeErr := os.Stdout.Write(append(data, '\n'))
		return writeErr
	}
	fmt.Println(encoded)
	return nil
}

func custodyFlags(args []string, allowKeys bool) (string, []string, bool, error) {
	manifestPath := ""
	var keyIDs []string
	asJSON := false
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--manifest":
			index++
			manifestPath = mustValue(args, index, "--manifest")
		case "--key":
			if !allowKeys {
				return "", nil, false, fmt.Errorf("unknown flag %q", args[index])
			}
			index++
			keyIDs = append(keyIDs, mustValue(args, index, "--key"))
		case "--json":
			asJSON = true
		default:
			return "", nil, false, fmt.Errorf("unknown flag %q", args[index])
		}
	}
	if manifestPath == "" {
		return "", nil, false, fmt.Errorf("--manifest is required")
	}
	return manifestPath, keyIDs, asJSON, nil
}
