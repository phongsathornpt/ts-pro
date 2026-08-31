package tsls

import "encoding/json"

func (c *Client) handleServerRequest(message rpcEnvelope) {
	var result any
	switch message.Method {
	case "client/registerCapability", "client/unregisterCapability", "window/workDoneProgress/create":
		result = nil
	case "workspace/workspaceFolders":
		result = []any{}
	case "workspace/configuration":
		result = emptyConfigurationResult(message.Params)
	default:
		_ = c.conn.write(rpcResponse{
			JSONRPC: "2.0",
			ID:      message.ID,
			Result:  nil,
			Error: &rpcError{
				Code:    -32601,
				Message: "method not found",
			},
		})
		return
	}
	_ = c.conn.write(rpcResponse{JSONRPC: "2.0", ID: message.ID, Result: result})
}
func emptyConfigurationResult(params json.RawMessage) []any {
	var payload struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(params, &payload); err != nil {
		return []any{}
	}
	result := make([]any, len(payload.Items))
	for i := range result {
		result[i] = nil
	}
	return result
}
