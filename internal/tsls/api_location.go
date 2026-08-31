package tsls

import "context"

// GetSymbolAtLocation resolves a symbol from an exact TypeScript 7 AST node handle.
func (c *APIClient) GetSymbolAtLocation(ctx context.Context, snapshot uint64, project, location string) (*APISymbol, error) {
	var result *APISymbol
	err := c.request(ctx, "getSymbolAtLocation", map[string]any{
		"snapshot": snapshot,
		"project":  project,
		"location": location,
	}, &result)
	return result, err
}

// GetTypeAtLocation resolves a type from an exact TypeScript 7 AST node handle.
func (c *APIClient) GetTypeAtLocation(ctx context.Context, snapshot uint64, project, location string) (*APIType, error) {
	var result *APIType
	err := c.request(ctx, "getTypeAtLocation", map[string]any{
		"snapshot": snapshot,
		"project":  project,
		"location": location,
	}, &result)
	return result, err
}
