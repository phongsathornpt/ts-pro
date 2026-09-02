package tsast

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/phongsathornpt/ts-pro/internal/tsls"
)

func TestDecodeTypeScriptBinaryAST(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := tsls.StartAPI("../..")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	if _, err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, err := client.UpdateSnapshot(ctx, "tsconfig.json")
	if err != nil {
		t.Fatal(err)
	}
	project := snapshot.Projects[0]
	fileName := filepath.Join(projectRoot(t), "examples", "fib.ts")
	payload, err := client.GetSourceFile(ctx, snapshot.Snapshot, project.ID, fileName)
	if err != nil {
		t.Fatal(err)
	}
	file, err := Decode(payload)
	if err != nil {
		t.Fatal(err)
	}

	root := file.Root()
	if root.Kind() != KindSourceFile {
		t.Fatalf("root kind = %s", KindName(root.Kind()))
	}
	text, ok := root.Text()
	if !ok || len(text) != 135 {
		t.Fatalf("source text len = %d, ok=%v", len(text), ok)
	}

	children := root.Children()
	if len(children) != 3 {
		t.Fatalf("source children = %d", len(children))
	}
	if children[0].Kind() != KindFunctionDeclaration || children[1].Kind() != KindExpressionStatement || children[2].Kind() != KindEndOfFile {
		t.Fatalf("source kinds = %s, %s, %s", KindName(children[0].Kind()), KindName(children[1].Kind()), KindName(children[2].Kind()))
	}

	fn := children[0]
	name, ok := fn.NamedChild("name")
	if !ok {
		t.Fatal("function name child missing")
	}
	if got, ok := name.Text(); !ok || got != "fib" {
		t.Fatalf("function name = %q, ok=%v", got, ok)
	}
	params, ok := fn.NamedChild("parameters")
	if !ok || !params.IsList() {
		t.Fatalf("parameters = %+v, ok=%v", params, ok)
	}
	paramItems := params.ListElements()
	if len(paramItems) != 1 || paramItems[0].Kind() != KindParameter {
		t.Fatalf("parameter list = %+v", paramItems)
	}
	paramName, ok := paramItems[0].NamedChild("name")
	if !ok {
		t.Fatal("parameter name missing")
	}
	if got, _ := paramName.Text(); got != "n" {
		t.Fatalf("parameter name = %q", got)
	}
	paramType, ok := paramItems[0].NamedChild("type")
	if !ok || paramType.Kind() != KindNumberKeyword {
		t.Fatalf("parameter type kind = %s", KindName(paramType.Kind()))
	}

	body, ok := fn.NamedChild("body")
	if !ok || body.Kind() != KindBlock {
		t.Fatalf("body kind = %s", KindName(body.Kind()))
	}
	statements, ok := body.NamedChild("statements")
	if !ok || !statements.IsList() {
		t.Fatal("body statements missing")
	}
	bodyItems := statements.ListElements()
	if len(bodyItems) != 2 || bodyItems[0].Kind() != KindIfStatement || bodyItems[1].Kind() != KindReturnStatement {
		t.Fatalf("body statement kinds unexpected")
	}
	condition, ok := bodyItems[0].NamedChild("expression")
	if !ok || condition.Kind() != KindBinaryExpression {
		t.Fatal("if condition is not binary")
	}
	operator, ok := condition.NamedChild("operatorToken")
	if !ok || operator.Kind() != KindLessThanEqualsToken {
		t.Fatalf("operator kind = %s", KindName(operator.Kind()))
	}
	right, ok := condition.NamedChild("right")
	if !ok || right.Kind() != KindNumericLiteral {
		t.Fatalf("right kind = %s", KindName(right.Kind()))
	}
	if got, ok := right.Text(); !ok || got != "1" {
		t.Fatalf("literal text = %q, ok=%v", got, ok)
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}
