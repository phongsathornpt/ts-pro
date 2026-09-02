package frontend

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/phongsathornpt/ts-pro/internal/tsls"
)

func TestExtractTypedFibFromTypeScript7(t *testing.T) {
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
	file := filepath.Join(projectRootFrontend(t), "examples", "fib.ts")
	semantic, err := ExtractFile(ctx, client, snapshot.Snapshot, project.ID, file)
	if err != nil {
		t.Fatal(err)
	}
	if len(semantic.Functions) != 1 {
		t.Fatalf("functions = %d", len(semantic.Functions))
	}
	fn := semantic.Functions[0]
	if fn.Name != "fib" || !fn.Exported || len(fn.Params) != 1 {
		t.Fatalf("function = %+v", fn)
	}
	if typ, ok := semantic.Type(fn.Params[0].Type); !ok || typ.Kind != TypeNumber {
		t.Fatalf("parameter type = %+v, ok=%v", typ, ok)
	}
	if typ, ok := semantic.Type(fn.ReturnType); !ok || typ.Kind != TypeNumber {
		t.Fatalf("return type = %+v, ok=%v", typ, ok)
	}
	if len(fn.Body) != 2 || fn.Body[0].Kind != StmtIf || fn.Body[1].Kind != StmtReturn {
		t.Fatalf("body = %+v", fn.Body)
	}
	ret := fn.Body[1].Return
	if ret == nil || ret.Kind != ExprBinary || ret.Operator != BinaryAdd {
		t.Fatalf("final return = %+v", ret)
	}
	for _, call := range []*Expr{ret.Left, ret.Right} {
		if call == nil || call.Kind != ExprCall || call.CallTarget == nil || *call.CallTarget != fn.ID {
			t.Fatalf("recursive call = %+v", call)
		}
		if typ, ok := semantic.Type(call.Type); !ok || typ.Kind != TypeNumber {
			t.Fatalf("call type = %+v, ok=%v", typ, ok)
		}
	}
	if len(semantic.Entry) != 1 || semantic.Entry[0].Kind != StmtExpr {
		t.Fatalf("entry = %+v", semantic.Entry)
	}
	entryCall := semantic.Entry[0].Expr
	if entryCall == nil || entryCall.Intrinsic != IntrinsicConsoleLogF64 || len(entryCall.Args) != 1 {
		t.Fatalf("entry call = %+v", entryCall)
	}
	if entryCall.Args[0].CallTarget == nil || *entryCall.Args[0].CallTarget != fn.ID {
		t.Fatalf("entry fib call = %+v", entryCall.Args[0])
	}
}

func projectRootFrontend(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}
