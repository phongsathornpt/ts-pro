package tsls

import "context"

type APISignature struct {
	ID         uint64   `json:"id"`
	Flags      uint32   `json:"flags"`
	Parameters []uint64 `json:"parameters"`
}

func (c *APIClient) GetSignaturesOfType(ctx context.Context, snapshot uint64, project string, typeID uint64, kind uint32) ([]APISignature, error) {
	var result []APISignature
	err := c.request(ctx, "getSignaturesOfType", map[string]any{
		"snapshot": snapshot,
		"project":  project,
		"type":     typeID,
		"kind":     kind,
	}, &result)
	return result, err
}

func (c *APIClient) GetParametersOfSignature(ctx context.Context, snapshot uint64, project string, signatureID uint64) ([]APISymbol, error) {
	var result []APISymbol
	err := c.request(ctx, "getParametersOfSignature", map[string]any{
		"snapshot": snapshot,
		"project":  project,
		"objectId": signatureID,
	}, &result)
	return result, err
}

func (c *APIClient) GetReturnTypeOfSignature(ctx context.Context, snapshot uint64, project string, signatureID uint64) (*APIType, error) {
	var result *APIType
	err := c.request(ctx, "getReturnTypeOfSignature", map[string]any{
		"snapshot":  snapshot,
		"project":   project,
		"signature": signatureID,
	}, &result)
	return result, err
}
