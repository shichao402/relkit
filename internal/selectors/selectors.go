package selectors

import (
	"sort"

	"github.com/shichao402/relkit/internal/model"
)

type DuplicateSelectors struct {
	FirstID   string
	SecondID  string
	Selectors map[string]string
}

func SelectArtifact(manifest *model.ManifestDocument, clientSelectors map[string]string) *model.ManifestArtifact {
	var matches []model.ManifestArtifact
	for _, artifact := range manifest.Artifacts {
		if matchesClient(artifact.Selectors, clientSelectors) {
			matches = append(matches, artifact)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].ID < matches[j].ID
	})
	chosen := matches[0]
	return &chosen
}

func FindDuplicateSelectors(artifacts []model.StagedArtifact) []DuplicateSelectors {
	seen := make(map[string]string, len(artifacts))
	var duplicates []DuplicateSelectors
	for _, artifact := range artifacts {
		key := selectorKey(artifact.Selectors)
		if firstID, ok := seen[key]; ok {
			duplicates = append(duplicates, DuplicateSelectors{
				FirstID:   firstID,
				SecondID:  artifact.ID,
				Selectors: cloneSelectors(artifact.Selectors),
			})
			continue
		}
		seen[key] = artifact.ID
	}
	return duplicates
}

func matchesClient(artifactSelectors, clientSelectors map[string]string) bool {
	for key, value := range artifactSelectors {
		if clientSelectors[key] != value {
			return false
		}
	}
	return true
}

func selectorKey(selectors map[string]string) string {
	keys := make([]string, 0, len(selectors))
	for key := range selectors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+selectors[key])
	}
	return stringsJoin(parts, "\x1f")
}

func cloneSelectors(selectors map[string]string) map[string]string {
	if len(selectors) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(selectors))
	for key, value := range selectors {
		cloned[key] = value
	}
	return cloned
}

func stringsJoin(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, part := range parts[1:] {
		result += sep + part
	}
	return result
}
