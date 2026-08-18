package simulate

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/backends"
	"cnb.cool/shichao402/relkit/internal/chain"
	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/envelope"
	"cnb.cool/shichao402/relkit/internal/httpx"
	"cnb.cool/shichao402/relkit/internal/model"
	"cnb.cool/shichao402/relkit/internal/stage"
)

type Error struct {
	Message string
}

func (e Error) Error() string {
	return e.Message
}

type Printer func(string)

func LoadIndex(cfg *config.Config, indexPath string, channel string, withStaged string, backendName string) (*model.IndexDocument, error) {
	resolvedChannel, err := cfg.ChannelOrDefault(channel)
	if err != nil {
		return nil, err
	}

	var index *model.IndexDocument
	if indexPath != "" {
		raw, err := os.ReadFile(indexPath)
		if err != nil {
			return nil, err
		}
		index, err = loadDocument(raw, cfg)
		if err != nil {
			return nil, err
		}
	} else {
		names := cfg.PublishTo
		if backendName != "" {
			names = []string{backendName}
		}
		for _, name := range names {
			backend, err := backends.Create(name, cfg, cfg.Root)
			if err != nil {
				return nil, err
			}
			raw, err := backend.Get(model.IndexKey(cfg.Product, resolvedChannel))
			if err != nil {
				var httpErr *httpx.Error
				if AsHTTPError(err, &httpErr) {
					continue
				}
				return nil, err
			}
			if raw == nil {
				continue
			}
			index, err = loadDocument(raw, cfg)
			if err != nil {
				return nil, err
			}
			break
		}
		if index == nil {
			index = model.EmptyIndex(cfg.Product, resolvedChannel)
		}
	}

	if withStaged != "" {
		staged, err := stage.LoadStaged(cfg.Root, withStaged)
		if err != nil {
			return nil, err
		}
		node := model.NewIndexNode(staged, strings.Repeat("0", 64), 0, []string{"https://placeholder.invalid/"}, "")
		versions := mergeNode(index.Versions, node)
		index, err = model.NewIndex(
			chooseNonEmpty(index.Product, cfg.Product),
			chooseNonEmpty(index.Channel, resolvedChannel),
			max(int(index.Sequence), 1),
			versions,
			model.MinSupportedPtr(index),
			"",
		)
		if err != nil {
			return nil, err
		}
	}

	return index, nil
}

func StartCodes(index *model.IndexDocument, spec string) ([]int, error) {
	if spec == "" || spec == "all" {
		codes := map[int]struct{}{0: {}}
		for _, node := range index.Versions {
			if node == nil {
				continue
			}
			codes[int(node.Code)] = struct{}{}
		}
		if model.HasMinSupported(index) {
			codes[int(index.MinSupported)] = struct{}{}
		}
		values := make([]int, 0, len(codes))
		for code := range codes {
			values = append(values, code)
		}
		sort.Ints(values)
		return values, nil
	}
	value, err := strconv.Atoi(spec)
	if err != nil {
		return nil, Error{Message: fmt.Sprintf("--from expects an integer code or 'all', got %q", spec)}
	}
	return []int{value}, nil
}

func Run(cfg *config.Config, fromSpec string, indexPath string, channel string, withStaged string, backendName string, printer Printer) (*model.IndexDocument, error) {
	if printer == nil {
		printer = func(string) {}
	}

	index, err := LoadIndex(cfg, indexPath, channel, withStaged, backendName)
	if err != nil {
		return nil, err
	}
	if len(index.Versions) == 0 {
		printer("index has no versions; nothing to simulate")
		return index, nil
	}

	head := chain.FindHead(index)
	printer(fmt.Sprintf("product %s, channel %s, sequence %d", index.Product, index.Channel, index.Sequence))
	if head == nil {
		printer("every version is yanked; no client can update")
	} else {
		printer(fmt.Sprintf("newest reachable version: %s (code %d)", head.Version, head.Code))
	}
	if model.HasMinSupported(index) {
		printer(fmt.Sprintf("minSupported: %d", index.MinSupported))
	}
	printer("")

	startCodes, err := StartCodes(index, fromSpec)
	if err != nil {
		return nil, err
	}

	var stranded []int
	for _, code := range startCodes {
		path := chain.ResolveUpgradePath(index, code)
		mandatory := chain.IsMandatory(index, code)
		header := fmt.Sprintf("current code %d", code)
		if mandatory {
			header += "  [MANDATORY UPDATE]"
		}
		printer(header)

		if len(path) == 0 {
			if head != nil && int64(code) < head.Code {
				printer("    no upgrade available -- STRANDED")
				stranded = append(stranded, code)
			} else {
				printer("    already at or above the newest version")
			}
			continue
		}

		for i, node := range path {
			line := fmt.Sprintf("    hop %d  ->  %-12s (code %d)   minFrom=%d", i+1, node.Version, node.Code, node.MinFrom)
			if node.Notes != "" {
				firstLine, _, _ := strings.Cut(node.Notes, "\n")
				line += "   " + firstLine
			}
			printer(line)
		}
		if head != nil && path[len(path)-1].Code != head.Code {
			printer(fmt.Sprintf("    ends at %s, not the newest version -- STRANDED", path[len(path)-1].Version))
			stranded = append(stranded, code)
		}
	}

	printer("")
	if len(stranded) > 0 {
		printer("STRANDED start codes: " + intsToString(stranded))
		printer("Lower the new version's minFrom, or add an intermediate release.")
	} else if head != nil {
		printer("every start point reaches " + head.Version)
	}
	return index, nil
}

func loadDocument(raw []byte, cfg *config.Config) (*model.IndexDocument, error) {
	if env, err := rupv2.UnmarshalEnvelope(raw); err == nil && env.Schema == envelope.EnvelopeSchema {
		trusted, err := cfg.TrustedPublicKeys()
		if err != nil {
			return nil, err
		}
		return envelope.OpenEnvelope(env, trusted)
	}

	if index, err := rupv2.UnmarshalIndex(raw); err == nil && index.Schema == model.SchemaIndex {
		return index, nil
	}

	return nil, Error{Message: "input is neither a rup.envelope/2 nor rup.index/2 protobuf document"}
}

func mergeNode(versions []*model.VersionNode, node *model.VersionNode) []*model.VersionNode {
	merged := make([]*model.VersionNode, 0, len(versions)+1)
	for _, version := range versions {
		if version.Code != node.Code {
			merged = append(merged, version)
		}
	}
	merged = append(merged, node)
	return merged
}

func chooseNonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func intsToString(values []int) string {
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	parts := make([]string, 0, len(sorted))
	for _, value := range sorted {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ", ")
}

func AsHTTPError(err error, target **httpx.Error) bool {
	httpErr, ok := err.(*httpx.Error)
	if ok {
		*target = httpErr
	}
	return ok
}
