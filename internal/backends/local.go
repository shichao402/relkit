package backends

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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

func (b *localBackend) ReceiveCAS(key string, body io.Reader, size int64) error {
	target, err := b.resolveUnder(b.outputDir, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".cas-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(body, size+1))
	closeErr := tmp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != size {
		return fmt.Errorf("cas body size %d does not match declared %d", written, size)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); filepath.Base(key) != got {
		return fmt.Errorf("cas body sha256 %s does not match key", got)
	}
	_ = os.Remove(target)
	return os.Rename(tmpPath, target)
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

func (b *localBackend) Head(key string) (int64, bool, error) {
	target, err := b.resolveUnder(b.outputDir, key)
	if err != nil {
		return 0, false, err
	}
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	if info.IsDir() {
		return 0, false, nil
	}
	return info.Size(), true, nil
}

func (b *localBackend) Promote(srcKey, dstKey string) ([]string, error) {
	src, err := b.resolveUnder(b.outputDir, srcKey)
	if err != nil {
		return nil, err
	}
	dst, err := b.resolveUnder(b.outputDir, dstKey)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(src); err != nil {
		return nil, fmt.Errorf("promote %s -> %s: %w", srcKey, dstKey, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, err
	}
	_ = os.Remove(dst)
	if err := os.Link(src, dst); err != nil {
		if copyErr := b.copyFile(b.outputDir, dstKey, src); copyErr != nil {
			return nil, copyErr
		}
	}
	return []string{*b.URLFor(dstKey)}, nil
}

func (b *localBackend) Delete(key string) error {
	target, err := b.resolveUnder(b.outputDir, key)
	if err != nil {
		return err
	}
	err = os.Remove(target)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

var _ Ingest = (*localBackend)(nil)
var _ Deleter = (*localBackend)(nil)
var _ CASProxyReceiver = (*localBackend)(nil)
