package tsls

import "context"

func (c *APIClient) GetTypeArguments(ctx context.Context, snapshot uint64, project string, typeID uint64) ([]APIType, error) {
	var result []APIType
	err := c.request(ctx, "getTypeArguments", map[string]any{
		"snapshot": snapshot,
		"project":  project,
		"type":     typeID,
	}, &result)
	return result, err
}
