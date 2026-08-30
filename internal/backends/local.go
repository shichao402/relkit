package backends

import (
	"fmt"
	"os"
	"path/filepath"
)

type localBackend struct {
	*pathStyleBackend
	outputDir string
}

func newLocalBackend(name string, cfg map[string]any, root string) (Backend, error) {
	base, err := newPathStyleBackend(name, "local", cfg)
	if err != nil {
		return nil, err
	}
	outputDir, err := requiredString(cfg, "outputDir", name)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(root, outputDir)
	}
	outputDir, err = filepath.Abs(outputDir)
	if err != nil {
		return nil, err
	}
	return &localBackend{pathStyleBackend: base, outputDir: outputDir}, nil
}

func (b *localBackend) Describe() string {
	return fmt.Sprintf("%s (local -> %s)", b.Name(), b.outputDir)
}

func (b *localBackend) URLsAreLive() bool {
	return false
}

func (b *localBackend) Writable() bool {
	return true
}

func (b *localBackend) HostsBrowse() bool {
	return true
}

func (b *localBackend) PutArtifact(localPath string, key string) ([]string, error) {
	if err := b.copyFile(b.outputDir, key, localPath); err != nil {
		return nil, err
	}
	return []string{*b.URLFor(key)}, nil
}

func (b *localBackend) PutImmutable(data []byte, key string) ([]string, error) {
	if err := b.writeFile(b.outputDir, key, data); err != nil {
		return nil, err
	}
	return []string{*b.URLFor(key)}, nil
}

func (b *localBackend) PutPointer(data []byte, key string) ([]string, error) {
	if err := b.writeFile(b.outputDir, key, data); err != nil {
		return nil, err
	}
	return []string{*b.URLFor(key)}, nil
}

func (b *localBackend) Get(key string) ([]byte, error) {
	target, err := b.resolveUnder(b.outputDir, key)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		return nil, nil
	}
	return os.ReadFile(target)
}
