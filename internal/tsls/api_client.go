package tsls

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type APIClient struct {
	toolchain *Toolchain
	cmd       *exec.Cmd
	conn      *rpcConn
	stdin     io.WriteCloser
	pending   sync.Map
	nextID    atomic.Int64
	waitDone  chan error
}

type APIInitializeResult struct {
	UseCaseSensitiveFileNames bool   `json:"useCaseSensitiveFileNames"`
	CurrentDirectory          string `json:"currentDirectory"`
}

type APIProject struct {
	ID              string         `json:"id"`
	ConfigFileName  string         `json:"configFileName"`
	RootFiles       []string       `json:"rootFiles"`
	CompilerOptions map[string]any `json:"compilerOptions"`
}

type APISnapshot struct {
	Snapshot uint64       `json:"snapshot"`
	Projects []APIProject `json:"projects"`
}

func StartAPI(root string) (*APIClient, error) {
	toolchain, err := Discover(root)
	if err != nil {
		return nil, err
	}
	args := []string{"--api", "--async", "--cwd", toolchain.Root}
	cmd := exec.Command(toolchain.TSCPath, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("TypeScript API stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("TypeScript API stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start TypeScript API: %w", err)
	}

	client := &APIClient{
		toolchain: toolchain,
		cmd:       cmd,
		conn:      newRPCConn(stdout, stdin),
		stdin:     stdin,
		waitDone:  make(chan error, 1),
	}
	go client.readLoop()
	go func() { client.waitDone <- cmd.Wait() }()
	return client, nil
}

func (c *APIClient) readLoop() {
	for {
		message, err := c.conn.read()
		if err != nil {
			c.failPending(err)
			return
		}
		if len(message.ID) == 0 || message.Method != "" {
			continue
		}
		id, err := parseRPCID(message.ID)
		if err != nil {
			continue
		}
		value, ok := c.pending.LoadAndDelete(id)
		if !ok {
			continue
		}
		value.(chan pendingResponse) <- pendingResponse{message: message}
	}
}

func (c *APIClient) failPending(err error) {
	c.pending.Range(func(key, value any) bool {
		if actual, ok := c.pending.LoadAndDelete(key); ok {
			actual.(chan pendingResponse) <- pendingResponse{err: err}
		}
		return true
	})
}

func (c *APIClient) request(ctx context.Context, method string, params, result any) error {
	id := c.nextID.Add(1)
	response := make(chan pendingResponse, 1)
	c.pending.Store(id, response)
	if err := c.conn.write(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		c.pending.Delete(id)
		return err
	}
	select {
	case <-ctx.Done():
		c.pending.Delete(id)
		return ctx.Err()
	case response := <-response:
		if response.err != nil {
			return response.err
		}
		if response.message.Error != nil {
			return fmt.Errorf("TypeScript API %s failed (%d): %s", method, response.message.Error.Code, response.message.Error.Message)
		}
		if result != nil && len(response.message.Result) != 0 {
			if err := json.Unmarshal(response.message.Result, result); err != nil {
				return fmt.Errorf("decode TypeScript API %s result: %w", method, err)
			}
		}
		return nil
	}
}

func (c *APIClient) Initialize(ctx context.Context) (APIInitializeResult, error) {
	var result APIInitializeResult
	err := c.request(ctx, "initialize", nil, &result)
	return result, err
}

func (c *APIClient) UpdateSnapshot(ctx context.Context, config string) (APISnapshot, error) {
	if !filepath.IsAbs(config) {
		config = filepath.Join(c.toolchain.Root, config)
	}
	config, err := filepath.Abs(config)
	if err != nil {
		return APISnapshot{}, fmt.Errorf("resolve config path: %w", err)
	}
	var result APISnapshot
	err = c.request(ctx, "updateSnapshot", map[string]any{
		"openProjects": []string{config},
	}, &result)
	return result, err
}

func (c *APIClient) ReleaseSnapshot(ctx context.Context, snapshot uint64) error {
	return c.request(ctx, "release", map[string]any{"snapshot": snapshot}, nil)
}

func (c *APIClient) Close() error {
	if c.stdin != nil {
		_ = c.stdin.Close()
		c.stdin = nil
	}
	select {
	case err := <-c.waitDone:
		if err != nil {
			return fmt.Errorf("TypeScript API exited: %w", err)
		}
		return nil
	case <-time.After(2 * time.Second):
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		return fmt.Errorf("timed out waiting for TypeScript API to exit")
	}
}
