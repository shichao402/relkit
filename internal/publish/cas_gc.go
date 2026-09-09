package publish

import (
	"fmt"

	rupv2 "firoyang.com/relkit/api/rup/v2"
	"firoyang.com/relkit/internal/backends"
	"firoyang.com/relkit/internal/config"
	"firoyang.com/relkit/internal/model"
)

// sweepOrphanCAS deletes cas/{sha256} blobs that this product just dropped
// from its index and that no remaining channel still names. It does not list
// the whole cas/ prefix, so a shared ingest bucket used by another product is
// left alone. Best-effort: index is already committed.
func sweepOrphanCAS(
	cfg *config.Config,
	published *model.IndexDocument,
	pruned []*model.VersionNode,
	staged *model.StagedDocument,
	targets []backends.Backend,
	printer Printer,
) {
	if printer == nil {
		printer = func(string) {}
	}
	if len(pruned) == 0 {
		return
	}
	for _, backend := range targets {
		deleter, ok := backend.(backends.Deleter)
		if !ok {
			continue
		}
		live := liveArtifactDigests(cfg, published, staged, backend)
		dropped := 0
		for _, digest := range digestsFromNodes(cfg.Product, pruned, backend) {
			if _, keep := live[digest]; keep {
				continue
			}
			key, err := model.CasKey(digest)
			if err != nil {
				continue
			}
			if err := deleter.Delete(key); err != nil {
				printer(fmt.Sprintf("  %-12s cas gc %s: %v", backend.Name(), key, err))
				continue
			}
			dropped++
		}
		if dropped > 0 {
			printer(fmt.Sprintf("  %-12s cas: dropped %d unreferenced blob(s)", backend.Name(), dropped))
		}
	}
}

func liveArtifactDigests(
	cfg *config.Config,
	published *model.IndexDocument,
	staged *model.StagedDocument,
	backend backends.Backend,
) map[string]struct{} {
	live := map[string]struct{}{}
	if staged != nil {
		for _, artifact := range staged.Artifacts {
			if artifact == nil || artifact.Sha256 == "" {
				continue
			}
			live[artifact.Sha256] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	for _, channel := range cfg.Channels {
		var idx *model.IndexDocument
		if published != nil && channel == published.Channel {
			idx = published
		} else {
			idx = loadIndexDocument(backend, cfg.Product, channel)
		}
		if idx == nil {
			continue
		}
		for _, node := range idx.Versions {
			if node == nil || node.Version == "" {
				continue
			}
			if _, ok := seen[node.Version]; ok {
				continue
			}
			seen[node.Version] = struct{}{}
			for _, digest := range digestsFromManifest(backend, cfg.Product, node.Version) {
				live[digest] = struct{}{}
			}
		}
	}
	return live
}

func digestsFromNodes(product string, nodes []*model.VersionNode, backend backends.Backend) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, node := range nodes {
		if node == nil || node.Version == "" {
			continue
		}
		for _, digest := range digestsFromManifest(backend, product, node.Version) {
			if _, ok := seen[digest]; ok {
				continue
			}
			seen[digest] = struct{}{}
			out = append(out, digest)
		}
	}
	return out
}

func digestsFromManifest(backend backends.Backend, product, version string) []string {
	raw, err := backend.Get(model.ManifestKey(product, version))
	if err != nil || len(raw) == 0 {
		return nil
	}
	doc, err := rupv2.UnmarshalManifest(raw)
	if err != nil {
		return nil
	}
	var out []string
	for _, artifact := range doc.Artifacts {
		if artifact == nil || artifact.Sha256 == "" {
			continue
		}
		out = append(out, artifact.Sha256)
	}
	return out
}

func loadIndexDocument(backend backends.Backend, product, channel string) *model.IndexDocument {
	raw, err := backend.Get(model.IndexKey(product, channel))
	if err != nil || len(raw) == 0 {
		return nil
	}
	env, err := rupv2.UnmarshalEnvelope(raw)
	if err != nil {
		return nil
	}
	idx, err := rupv2.UnmarshalIndex(env.Payload)
	if err != nil {
		return nil
	}
	return idx
}
