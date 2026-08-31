package tsls

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestRPCConnReadsContentLengthFrame(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`
	input := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	message, err := newRPCConn(strings.NewReader(input), &bytes.Buffer{}).read()
	if err != nil {
		t.Fatal(err)
	}
	id, err := parseRPCID(message.ID)
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("id = %d, want 1", id)
	}
}

func TestRPCConnWritesContentLengthFrame(t *testing.T) {
	var output bytes.Buffer
	conn := newRPCConn(strings.NewReader(""), &output)
	if err := conn.write(rpcNotification{JSONRPC: "2.0", Method: "initialized"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output.String(), "Content-Length: ") {
		t.Fatalf("missing Content-Length header: %q", output.String())
	}
}
