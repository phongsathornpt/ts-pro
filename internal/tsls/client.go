package tsls

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type Client struct {
	toolchain *Toolchain
	conn      *rpcConn
	stdin     io.WriteCloser
	cancel    context.CancelFunc
	waitDone  chan error
	stderr    *bytes.Buffer

	nextID  atomic.Int64
	pending sync.Map
}

type pendingResponse struct {
	message rpcEnvelope
	err     error
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}
type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

func Start(root string) (*Client, error) {
	toolchain, err := Discover(root)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := toolchain.LSPCommand(ctx)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open TypeScript-LS stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open TypeScript-LS stdout: %w", err)
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start TypeScript-LS: %w", err)
	}
	client := &Client{
		toolchain: toolchain,
		conn:      newRPCConn(stdout, stdin),
		stdin:     stdin,
		cancel:    cancel,
		waitDone:  make(chan error, 1),
		stderr:    stderr,
	}
	go client.readLoop()
	go func() { client.waitDone <- cmd.Wait() }()
	return client, nil
}
func (c *Client) readLoop() {
	for {
		message, err := c.conn.read()
		if err != nil {
			c.failPending(err)
			return
		}
		if message.Method != "" {
			if len(message.ID) != 0 {
				c.handleServerRequest(message)
			}
			continue
		}
		if len(message.ID) == 0 {
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

func (c *Client) failPending(err error) {
	c.pending.Range(func(key, value any) bool {
		if actual, ok := c.pending.LoadAndDelete(key); ok {
			actual.(chan pendingResponse) <- pendingResponse{err: err}
		}
		return true
	})
}

func (c *Client) request(ctx context.Context, method string, params, result any) error {
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
		_ = c.notify("$/cancelRequest", map[string]any{"id": id})
		return ctx.Err()
	case response := <-response:
		if response.err != nil {
			return response.err
		}
		if response.message.Error != nil {
			return fmt.Errorf("TypeScript-LS %s failed (%d): %s", method, response.message.Error.Code, response.message.Error.Message)
		}
		if result != nil && len(response.message.Result) != 0 {
			if err := json.Unmarshal(response.message.Result, result); err != nil {
				return fmt.Errorf("decode %s result: %w", method, err)
			}
		}
		return nil
	}
}

func (c *Client) notify(method string, params any) error {
	return c.conn.write(rpcNotification{JSONRPC: "2.0", Method: method, Params: params})
}

func (c *Client) Initialize(ctx context.Context) error {
	rootURI := (&url.URL{Scheme: "file", Path: filepath.ToSlash(c.toolchain.Root)}).String()
	params := map[string]any{
		"processId":    os.Getpid(),
		"rootUri":      rootURI,
		"capabilities": map[string]any{},
		"clientInfo":   map[string]any{"name": "tsnative", "version": "dev"},
	}
	var result json.RawMessage
	if err := c.request(ctx, "initialize", params, &result); err != nil {
		return err
	}
	return c.notify("initialized", map[string]any{})
}
func (c *Client) Close(ctx context.Context) error {
	// TypeScript 7.0.2 accepts shutdown but may not send a response. Keep it
	// best-effort, then use the LSP exit notification as the reliable terminator.
	shutdownCtx, cancelShutdown := context.WithTimeout(ctx, 500*time.Millisecond)
	_ = c.request(shutdownCtx, "shutdown", nil, nil)
	cancelShutdown()

	_ = c.notify("exit", nil)
	_ = c.stdin.Close()

	select {
	case err := <-c.waitDone:
		c.cancel()
		if err != nil {
			return fmt.Errorf("TypeScript-LS exited: %w", err)
		}
		return nil
	case <-ctx.Done():
		c.cancel()
		select {
		case <-c.waitDone:
		case <-time.After(time.Second):
		}
		return ctx.Err()
	}
}
