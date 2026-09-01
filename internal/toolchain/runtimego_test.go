package toolchain

import (
	"context"
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
