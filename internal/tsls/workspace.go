package tsls

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Workspace struct {
	root       string
	mu         sync.Mutex
	client     *Client
	generation uint64
}

func NewWorkspace(root string) *Workspace {
	return &Workspace{root: root}
}

func (w *Workspace) Acquire(ctx context.Context) (*Client, uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.client != nil && w.client.Alive() {
		return w.client, w.generation, nil
	}
	if w.client != nil {
		w.closeStale()
	}
	client, err := Start(w.root)
	if err != nil {
		return nil, 0, err
	}
	if err := client.Initialize(ctx); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = client.Close(closeCtx)
		cancel()
		return nil, 0, fmt.Errorf("initialize TypeScript-LS workspace: %w", err)
	}
	w.client = client
	w.generation++
	return client, w.generation, nil
}

func (w *Workspace) closeStale() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_ = w.client.Close(ctx)
	cancel()
	w.client = nil
}

func (w *Workspace) Close(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.client == nil {
		return nil
	}
	err := w.client.Close(ctx)
	w.client = nil
	return err
}
