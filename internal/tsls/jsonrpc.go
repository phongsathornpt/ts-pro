package tsls

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

type rpcConn struct {
	reader *bufio.Reader
	writer io.Writer
	mu     sync.Mutex
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func newRPCConn(reader io.Reader, writer io.Writer) *rpcConn {
	return &rpcConn{reader: bufio.NewReader(reader), writer: writer}
}

func (c *rpcConn) read() (rpcEnvelope, error) {
	length := -1
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return rpcEnvelope{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return rpcEnvelope{}, fmt.Errorf("malformed JSON-RPC header %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || parsed < 0 {
				return rpcEnvelope{}, fmt.Errorf("invalid Content-Length %q", value)
			}
			length = parsed
		}
	}
	if length < 0 {
		return rpcEnvelope{}, fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(c.reader, body); err != nil {
		return rpcEnvelope{}, err
	}
	var message rpcEnvelope
	if err := json.Unmarshal(body, &message); err != nil {
		return rpcEnvelope{}, fmt.Errorf("decode JSON-RPC body: %w", err)
	}
	return message, nil
}

func (c *rpcConn) write(message any) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode JSON-RPC body: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := fmt.Fprintf(c.writer, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.writer.Write(body)
	return err
}

func parseRPCID(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("missing response id")
	}
	return strconv.ParseInt(string(raw), 10, 64)
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
}
