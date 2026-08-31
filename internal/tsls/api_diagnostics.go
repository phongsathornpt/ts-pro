package tsls

import "context"

const (
	DiagnosticWarning    = 0
	DiagnosticError      = 1
	DiagnosticSuggestion = 2
	DiagnosticMessage    = 3
)

type APIDiagnostic struct {
	FileName           string          `json:"fileName"`
	Pos                int             `json:"pos"`
	End                int             `json:"end"`
	Code               int             `json:"code"`
	Category           int             `json:"category"`
	Text               string          `json:"text"`
	ReportsUnnecessary bool            `json:"reportsUnnecessary"`
	ReportsDeprecated  bool            `json:"reportsDeprecated"`
	MessageChain       []APIDiagnostic `json:"messageChain"`
	RelatedInformation []APIDiagnostic `json:"relatedInformation"`
}

func (d APIDiagnostic) IsError() bool { return d.Category == DiagnosticError }
func (c *APIClient) GetSyntacticDiagnostics(ctx context.Context, snapshot uint64, project, file string) ([]APIDiagnostic, error) {
	return c.getFileDiagnostics(ctx, "getSyntacticDiagnostics", snapshot, project, file)
}

func (c *APIClient) GetSemanticDiagnostics(ctx context.Context, snapshot uint64, project, file string) ([]APIDiagnostic, error) {
	return c.getFileDiagnostics(ctx, "getSemanticDiagnostics", snapshot, project, file)
}

func (c *APIClient) GetProgramDiagnostics(ctx context.Context, snapshot uint64, project string) ([]APIDiagnostic, error) {
	return c.getProjectDiagnostics(ctx, "getProgramDiagnostics", snapshot, project)
}

func (c *APIClient) GetGlobalDiagnostics(ctx context.Context, snapshot uint64, project string) ([]APIDiagnostic, error) {
	return c.getProjectDiagnostics(ctx, "getGlobalDiagnostics", snapshot, project)
}

func (c *APIClient) GetConfigDiagnostics(ctx context.Context, snapshot uint64, project string) ([]APIDiagnostic, error) {
	return c.getProjectDiagnostics(ctx, "getConfigFileParsingDiagnostics", snapshot, project)
}
func (c *APIClient) getFileDiagnostics(ctx context.Context, method string, snapshot uint64, project, file string) ([]APIDiagnostic, error) {
	var result []APIDiagnostic
	err := c.request(ctx, method, map[string]any{
		"snapshot": snapshot,
		"project":  project,
		"file":     file,
	}, &result)
	return result, err
}

func (c *APIClient) getProjectDiagnostics(ctx context.Context, method string, snapshot uint64, project string) ([]APIDiagnostic, error) {
	var result []APIDiagnostic
	err := c.request(ctx, method, map[string]any{
		"snapshot": snapshot,
		"project":  project,
	}, &result)
	return result, err
}
