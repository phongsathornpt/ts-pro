package tsls

import (
	"context"
	"testing"
	"time"
)

func TestDiscoverPinnedTypeScript(t *testing.T) {
	toolchain, err := Discover("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	version, err := toolchain.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != SupportedVersion {
		t.Fatalf("version = %q, want %q", version, SupportedVersion)
	}
}
