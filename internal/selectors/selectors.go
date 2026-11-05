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
	var prefs []string
	if want := clientSelectors[applyKey]; want != "" {
		prefs = []string{want}
	}
	return SelectArtifactPreferring(manifest, clientSelectors, prefs)
}

const applyKey = "apply"

// SelectArtifactPreferring is SelectArtifact for a client that accepts
// several apply formats (ADR 0027 §3). An artifact's apply selector matches
// when it is any of applyPrefs; the client map's own apply value is ignored.
// Among matches, the earliest apply in applyPrefs wins, artifacts without an
// apply selector (full installs) come last, and ties break by id.
//
// SelectArtifact(m, c) is exactly SelectArtifactPreferring(m, c, [c["apply"]]).
func SelectArtifactPreferring(manifest *model.ManifestDocument, clientSelectors map[string]string, applyPrefs []string) *model.ManifestArtifact {
	rank := func(artifact *model.ManifestArtifact) int {
		value, ok := model.SelectorsToMap(artifact.Selectors)[applyKey]
		if !ok {
			return len(applyPrefs)
		}
		for i, pref := range applyPrefs {
			if value == pref {
				return i
			}
		}
		return -1
	}
	var matches []*model.ManifestArtifact
	for _, artifact := range manifest.Artifacts {
		if artifact != nil && MatchesClientApply(model.SelectorsToMap(artifact.Selectors), clientSelectors, applyPrefs) {
			matches = append(matches, artifact)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool {
		left, right := rank(matches[i]), rank(matches[j])
		if left != right {
			return left < right
		}
		return matches[i].Id < matches[j].Id
	})
	return matches[0]
}

// MatchesClientApply is MatchesClient with the apply key matched against a
// list of accepted values instead of a single client value.
func MatchesClientApply(requiredSelectors, clientSelectors map[string]string, applyPrefs []string) bool {
	for key, value := range requiredSelectors {
		if key == applyKey {
			accepted := false
			for _, pref := range applyPrefs {
				if value == pref {
					accepted = true
					break
				}
			}
			if !accepted {
				return false
			}
			continue
		}
		if clientSelectors[key] != value {
			return false
		}
	}
	return true
}

func FindDuplicateSelectors(artifacts []*model.StagedArtifact) []DuplicateSelectors {
	seen := make(map[string]string, len(artifacts))
	var duplicates []DuplicateSelectors
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		selectors := model.SelectorsToMap(artifact.Selectors)
		key := selectorKey(selectors)
		if firstID, ok := seen[key]; ok {
			duplicates = append(duplicates, DuplicateSelectors{
				FirstID:   firstID,
				SecondID:  artifact.Id,
				Selectors: cloneSelectors(selectors),
			})
			continue
		}
		seen[key] = artifact.Id
	}
	return duplicates
}

// MatchesClient reports whether every key in requiredSelectors is present in
// clientSelectors with the same value. An empty required map matches everyone.
func MatchesClient(requiredSelectors, clientSelectors map[string]string) bool {
	for key, value := range requiredSelectors {
		if clientSelectors[key] != value {
			return false
		}
	}
	return true
}

func matchesClient(artifactSelectors, clientSelectors map[string]string) bool {
	return MatchesClient(artifactSelectors, clientSelectors)
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
