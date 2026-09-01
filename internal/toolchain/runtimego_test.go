package toolchain

import (
	"context"
	"os"
	"strings"
	"testing"
)

func buildGoRuntimeArchiveForTest(t *testing.T, ctx context.Context, root string, clang *Clang) string {
	t.Helper()
	cache, err := NewObjectCache(root, clang)
	if err != nil {
		t.Fatal(err)
	}
	archive, _, err := cache.BuildGoArchive(ctx, root, "./runtimego")
	if err != nil {
		t.Fatal(err)
	}
	return archive
}

func goRuntimeHeaderForTest(t *testing.T, archive string) string {
	t.Helper()
	header := strings.TrimSuffix(archive, ".a") + ".h"
	if info, err := os.Stat(header); err != nil || info.Size() == 0 {
		t.Fatalf("Go runtime header missing for %s: %v", archive, err)
	}
	return header
}
