package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/shichao402/relkit/internal/model"
)

// PublishProfile is the machine-owned publishing part of relkit
// configuration. It names backends and the directory layout only; signing is
// no longer a per-product fact, because a server signs every product with its
// one root (requirement 10.5).
type PublishProfile struct {
	Product   string                    `json:"product"`
	Backends  map[string]map[string]any `json:"backends"`
	PublishTo []string                  `json:"publishTo,omitempty"`
	Directory *PublishDirectoryProfile  `json:"directory,omitempty"`
}

type PublishDirectoryProfile struct {
	PublishTo []string                 `json:"publishTo,omitempty"`
	EntryURLs []string                 `json:"entryUrls,omitempty"`
	Services  []DirectoryServiceConfig `json:"services,omitempty"`
}

func LoadPublishProfile(path string) (*PublishProfile, error) {
	var profile PublishProfile
	if err := loadStrict(path, &profile); err != nil {
		return nil, Error{Message: fmt.Sprintf("%s is not a valid publish profile: %v", path, err)}
	}
	if err := validatePublishProfile(&profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

func validatePublishProfile(profile *PublishProfile) error {
	if profile == nil {
		return Error{Message: "publish profile is nil"}
	}
	if err := model.CheckIdentifier(profile.Product, "product"); err != nil {
		return err
	}
	if len(profile.Backends) == 0 {
		return Error{Message: "at least one backend must be configured"}
	}
	for name, backend := range profile.Backends {
		if backend == nil {
			return Error{Message: fmt.Sprintf("backend %q must be an object", name)}
		}
	}
	checkTargets := func(field string, targets []string) error {
		for _, name := range targets {
			if _, ok := profile.Backends[name]; !ok {
				return Error{Message: fmt.Sprintf("%s names unknown backend %q (configured: %s)", field, name, strings.Join(sortedKeys(profile.Backends), ", "))}
			}
		}
		return nil
	}
	if err := checkTargets("publishTo", profile.PublishTo); err != nil {
		return err
	}
	if profile.Directory != nil {
		if err := checkTargets("directory.publishTo", profile.Directory.PublishTo); err != nil {
			return err
		}
	}
	if _, err := json.Marshal(profile); err != nil {
		return Error{Message: fmt.Sprintf("publish profile is not serializable: %v", err)}
	}
	return nil
}

func loadStrict(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return fmt.Errorf("unexpected trailing JSON content")
	} else if err != io.EOF {
		return err
	}
	return nil
}
