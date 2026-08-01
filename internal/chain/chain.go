package chain

import (
	"sort"

	"github.com/shichao402/relkit/internal/model"
)

func SelectNextTarget(index *model.IndexDocument, currentCode int) *model.VersionNode {
	var best *model.VersionNode
	for i := range index.Versions {
		node := &index.Versions[i]
		if node.Yanked {
			continue
		}
		if node.Code <= currentCode {
			continue
		}
		if node.MinFrom > currentCode {
			continue
		}
		if best == nil || node.Code > best.Code {
			best = node
		}
	}
	return best
}

func ResolveUpgradePath(index *model.IndexDocument, currentCode int) []model.VersionNode {
	var path []model.VersionNode
	code := currentCode
	for {
		target := SelectNextTarget(index, code)
		if target == nil {
			return path
		}
		path = append(path, *target)
		code = target.Code
	}
}

func IsMandatory(index *model.IndexDocument, currentCode int) bool {
	return index.MinSupported != nil && currentCode < *index.MinSupported
}

func FindHead(index *model.IndexDocument) *model.VersionNode {
	var head *model.VersionNode
	for i := range index.Versions {
		node := &index.Versions[i]
		if node.Yanked {
			continue
		}
		if head == nil || node.Code > head.Code {
			head = node
		}
	}
	return head
}

func ValidateReachability(index *model.IndexDocument) ([]string, []string) {
	errors := map[string]struct{}{}
	warnings := map[string]struct{}{}

	codeCounts := make(map[int]int, len(index.Versions))
	for _, node := range index.Versions {
		codeCounts[node.Code]++
	}
	for _, count := range codeCounts {
		if count > 1 {
			errors["duplicate-code"] = struct{}{}
			break
		}
	}

	head := FindHead(index)
	if head == nil {
		errors["no-head"] = struct{}{}
		return sortedCodes(errors), sortedCodes(warnings)
	}

	reachesHead := func(code int) bool {
		path := ResolveUpgradePath(index, code)
		return len(path) > 0 && path[len(path)-1].Code == head.Code
	}

	uniqueCodes := make([]int, 0, len(codeCounts))
	for code := range codeCounts {
		uniqueCodes = append(uniqueCodes, code)
	}
	sort.Ints(uniqueCodes)
	for _, code := range uniqueCodes {
		if code >= head.Code {
			continue
		}
		if !reachesHead(code) {
			errors["unreachable"] = struct{}{}
		}
	}

	if index.MinSupported != nil {
		if *index.MinSupported > head.Code {
			errors["min-supported-above-head"] = struct{}{}
		} else if *index.MinSupported < head.Code && !reachesHead(*index.MinSupported) {
			errors["min-supported-unreachable"] = struct{}{}
		}
	}

	if head.Code > 0 && !reachesHead(0) {
		warnings["zero-unreachable"] = struct{}{}
	}

	return sortedCodes(errors), sortedCodes(warnings)
}

func UnreachableStartCodes(index *model.IndexDocument) []int {
	head := FindHead(index)
	if head == nil {
		return nil
	}

	starts := map[int]struct{}{0: {}}
	for _, node := range index.Versions {
		starts[node.Code] = struct{}{}
	}
	if index.MinSupported != nil {
		starts[*index.MinSupported] = struct{}{}
	}

	codes := make([]int, 0, len(starts))
	for code := range starts {
		codes = append(codes, code)
	}
	sort.Ints(codes)

	var stranded []int
	for _, code := range codes {
		if code >= head.Code {
			continue
		}
		path := ResolveUpgradePath(index, code)
		if len(path) == 0 || path[len(path)-1].Code != head.Code {
			stranded = append(stranded, code)
		}
	}
	return stranded
}

func sortedCodes(set map[string]struct{}) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}
