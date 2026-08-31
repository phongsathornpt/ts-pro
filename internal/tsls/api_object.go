package tsls

import "context"

func (c *APIClient) GetPropertiesOfType(ctx context.Context, snapshot uint64, project string, typeID uint64) ([]APISymbol, error) {
	var result []APISymbol
	err := c.request(ctx, "getPropertiesOfType", map[string]any{
		"snapshot": snapshot,
		"project":  project,
		"type":     typeID,
	}, &result)
	return result, err
}

func (c *APIClient) GetTypeOfSymbol(ctx context.Context, snapshot uint64, project string, symbolID uint64) (*APIType, error) {
	var result *APIType
	err := c.request(ctx, "getTypeOfSymbol", map[string]any{
		"snapshot": snapshot,
		"project":  project,
		"symbol":   symbolID,
	}, &result)
	return result, err
}

func (c *APIClient) GetDeclaredTypeOfSymbol(ctx context.Context, snapshot uint64, project string, symbolID uint64) (*APIType, error) {
	var result *APIType
	err := c.request(ctx, "getDeclaredTypeOfSymbol", map[string]any{
		"snapshot": snapshot,
		"project":  project,
		"symbol":   symbolID,
	}, &result)
	return result, err
}
