package tsls

import (
	"context"
	"testing"
	"time"
)

func TestWorkspaceReusesLiveLanguageServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	workspace := NewWorkspace("../..")
	first, generation, err := workspace.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, secondGeneration, err := workspace.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || generation != secondGeneration {
		t.Fatalf("workspace did not reuse live client")
	}
	if err := workspace.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceRestartsCrashedLanguageServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	workspace := NewWorkspace("../..")
	first, generation, err := workspace.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first.cancel()
	deadline := time.Now().Add(3 * time.Second)
	for first.Alive() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if first.Alive() {
		t.Fatal("language server did not exit after cancellation")
	}
	second, secondGeneration, err := workspace.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || secondGeneration != generation+1 {
		t.Fatalf("workspace did not restart crashed client")
	}
	_ = workspace.Close(ctx)
}
