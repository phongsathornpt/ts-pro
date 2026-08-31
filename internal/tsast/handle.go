package tsast

import "fmt"

// Handle returns the TypeScript 7 API node handle for this AST node.
// The path must be the canonical source-file path used by the API project.
func (n Node) Handle(path string) string {
	return fmt.Sprintf("%d.%d.%s", n.Index(), n.Kind(), path)
}
