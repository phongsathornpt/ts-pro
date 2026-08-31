package tsls

import (
	"context"
	"testing"
	"time"
)

func TestTypeScriptLSPInitializeAndShutdown(t *testing.T) {
	client, err := Start("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		client.cancel()
		t.Fatal(err)
	}
	if err := client.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
