package toolchain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

type ObjectCache struct {
	Dir   string
	Clang *Clang
}

func NewObjectCache(root string, clang *Clang) (*ObjectCache, error) {
	dir := filepath.Join(root, ".tsnative", "cache", "objects")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create object cache: %w", err)
	}
	return &ObjectCache{Dir: dir, Clang: clang}, nil
}

func (c *ObjectCache) CompileLLVM(ctx context.Context, input, opt string) (string, bool, error) {
	return c.compile(ctx, "llvm", input, opt, c.Clang.CompileLLVM)
}

func (c *ObjectCache) CompileC(ctx context.Context, input, opt string) (string, bool, error) {
	return c.compile(ctx, "c", input, opt, c.Clang.CompileC)
}

type compileFunc func(context.Context, string, string, string) error

func (c *ObjectCache) compile(ctx context.Context, kind, input, opt string, compile compileFunc) (string, bool, error) {
	data, err := os.ReadFile(input)
	if err != nil {
		return "", false, fmt.Errorf("read cache input %s: %w", input, err)
	}
	identity, err := clangIdentity(c.Clang.Path)
	if err != nil {
		return "", false, err
	}
	h := sha256.New()
	_, _ = h.Write([]byte(kind))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(opt))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(identity))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(data)
	key := hex.EncodeToString(h.Sum(nil))
	output := filepath.Join(c.Dir, key+".o")
	if info, err := os.Stat(output); err == nil && info.Size() > 0 {
		return output, true, nil
	}
	temp, err := os.CreateTemp(c.Dir, key+"-*.o")
	if err != nil {
		return "", false, fmt.Errorf("create cache temp object: %w", err)
	}
	tempPath := temp.Name()
	_ = temp.Close()
	_ = os.Remove(tempPath)
	defer os.Remove(tempPath)
	if err := compile(ctx, input, tempPath, opt); err != nil {
		return "", false, err
	}
	if err := os.Rename(tempPath, output); err != nil {
		if info, statErr := os.Stat(output); statErr != nil || info.Size() == 0 {
			return "", false, fmt.Errorf("publish cached object: %w", err)
		}
	}
	return output, false, nil
}

func clangIdentity(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat clang: %w", err)
	}
	return fmt.Sprintf("%s:%d:%d", path, info.Size(), info.ModTime().UnixNano()), nil
}
