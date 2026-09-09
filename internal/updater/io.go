package updater

import (
	"os"
	"strings"
)

func readNames(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range ents {
		name := e.Name()
		if strings.HasSuffix(name, ".pb") {
			names = append(names, strings.TrimSuffix(name, ".pb"))
		}
	}
	return names, nil
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
