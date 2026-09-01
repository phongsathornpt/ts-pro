package toolchain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func (c *ObjectCache) BuildGoArchive(ctx context.Context, root, packagePath string) (string, bool, error) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return "", false, fmt.Errorf("go toolchain not found: %w", err)
	}
	identity, err := goIdentity(ctx, goPath)
	if err != nil {
		return "", false, err
	}
	h := sha256.New()
	_, _ = h.Write([]byte("go-c-archive\x00" + identity + "\x00" + packagePath))
	for _, name := range []string{"go.mod", "go.sum"} {
		path := filepath.Join(root, name)
		if data, readErr := os.ReadFile(path); readErr == nil {
			_, _ = h.Write([]byte("\x00" + name + "\x00"))
			_, _ = h.Write(data)
		}
	}
	packageDir := filepath.Join(root, strings.TrimPrefix(packagePath, "./"))
	var files []string
	if err := filepath.WalkDir(packageDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			ext := filepath.Ext(entry.Name())
			if ext == ".go" || ext == ".s" || ext == ".h" {
				files = append(files, path)
			}
		}
		return nil
	}); err != nil {
		return "", false, fmt.Errorf("scan Go runtime package: %w", err)
	}
	sort.Strings(files)
	for _, path := range files {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", false, fmt.Errorf("read Go runtime source %s: %w", path, readErr)
		}
		rel, _ := filepath.Rel(root, path)
		_, _ = h.Write([]byte("\x00" + rel + "\x00"))
		_, _ = h.Write(data)
	}
	key := hex.EncodeToString(h.Sum(nil))
	output := filepath.Join(c.Dir, key+".a")
	outputHeader := strings.TrimSuffix(output, ".a") + ".h"
	archiveInfo, archiveErr := os.Stat(output)
	headerInfo, headerErr := os.Stat(outputHeader)
	if archiveErr == nil && archiveInfo.Size() > 0 && headerErr == nil && headerInfo.Size() > 0 {
		return output, true, nil
	}
	tempFile, err := os.CreateTemp(c.Dir, key+"-*.a")
	if err != nil {
		return "", false, fmt.Errorf("create Go archive cache temp: %w", err)
	}
	temp := tempFile.Name()
	_ = tempFile.Close()
	header := strings.TrimSuffix(temp, ".a") + ".h"
	defer os.Remove(temp)
	defer os.Remove(header)
	cmd := exec.CommandContext(ctx, goPath, "build", "-trimpath", "-buildvcs=false", "-buildmode=c-archive", "-o", temp, packagePath)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	if data, runErr := cmd.CombinedOutput(); runErr != nil {
		return "", false, fmt.Errorf("build Go runtime archive: %w: %s", runErr, data)
	}
	if err := os.Rename(temp, output); err != nil {
		if info, statErr := os.Stat(output); statErr != nil || info.Size() == 0 {
			return "", false, fmt.Errorf("publish Go runtime archive: %w", err)
		}
	}
	if err := os.Rename(header, outputHeader); err != nil {
		if info, statErr := os.Stat(outputHeader); statErr != nil || info.Size() == 0 {
			return "", false, fmt.Errorf("publish Go runtime header: %w", err)
		}
	}
	return output, false, nil
}

func goIdentity(ctx context.Context, goPath string) (string, error) {
	version, err := exec.CommandContext(ctx, goPath, "version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("query go version: %w: %s", err, version)
	}
	cmd := exec.CommandContext(ctx, goPath, "env", "GOOS", "GOARCH", "CGO_ENABLED")
	env, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("query go environment: %w: %s", err, env)
	}
	return strings.TrimSpace(string(version)) + "\n" + strings.TrimSpace(string(env)), nil
}
