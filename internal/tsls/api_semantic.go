package tsls

import (
	"context"
	"encoding/base64"
	"fmt"
)

type APISymbol struct {
	ID               uint64   `json:"id"`
	Project          string   `json:"project"`
	Name             string   `json:"name"`
	Flags            uint32   `json:"flags"`
	CheckFlags       uint32   `json:"checkFlags"`
	Declarations     []string `json:"declarations"`
	ValueDeclaration string   `json:"valueDeclaration"`
	Parent           uint64   `json:"parent"`
	ExportSymbol     uint64   `json:"exportSymbol"`
}

type APIType struct {
	ID             uint64   `json:"id"`
	Flags          uint32   `json:"flags"`
	ObjectFlags    uint32   `json:"objectFlags"`
	Value          any      `json:"value"`
	Target         uint64   `json:"target"`
	TypeParameters []uint64 `json:"typeParameters"`
	AliasSymbol    uint64   `json:"aliasSymbol"`
	AliasTypeArgs  []uint64 `json:"aliasTypeArguments"`
}

type apiSourceFileResponse struct {
	Data string `json:"data"`
}

func (c *APIClient) GetSourceFileNames(ctx context.Context, snapshot uint64, project string) ([]string, error) {
	var result []string
	err := c.request(ctx, "getSourceFileNames", map[string]any{
		"snapshot": snapshot,
		"project":  project,
	}, &result)
	return result, err
}

func (c *APIClient) GetSourceFile(ctx context.Context, snapshot uint64, project, file string) ([]byte, error) {
	var result apiSourceFileResponse
	if err := c.request(ctx, "getSourceFile", map[string]any{
		"snapshot": snapshot,
		"project":  project,
		"file":     file,
	}, &result); err != nil {
		return nil, err
	}
	if result.Data == "" {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(result.Data)
	if err != nil {
		return nil, fmt.Errorf("decode TypeScript AST: %w", err)
	}
	return data, nil
}

func (c *APIClient) GetSymbolAtPosition(ctx context.Context, snapshot uint64, project, file string, position int) (*APISymbol, error) {
	var result *APISymbol
	err := c.request(ctx, "getSymbolAtPosition", map[string]any{
		"snapshot": snapshot, "project": project, "file": file, "position": position,
	}, &result)
	return result, err
}

func (c *APIClient) GetTypeAtPosition(ctx context.Context, snapshot uint64, project, file string, position int) (*APIType, error) {
	var result *APIType
	err := c.request(ctx, "getTypeAtPosition", map[string]any{
		"snapshot": snapshot, "project": project, "file": file, "position": position,
	}, &result)
	return result, err
}

func (c *APIClient) TypeToString(ctx context.Context, snapshot uint64, project string, typeID uint64) (string, error) {
	var result string
	err := c.request(ctx, "typeToString", map[string]any{
		"snapshot": snapshot,
		"project":  project,
		"type":     typeID,
	}, &result)
	return result, err
}
