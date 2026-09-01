package llvm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

func (e *emitter) taskTargets() []mir.FunctionID {
	set := map[mir.FunctionID]struct{}{}
	for _, fn := range e.module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				if spawn, ok := inst.Op.(mir.TaskSpawn); ok {
					set[spawn.Callee] = struct{}{}
				}
			}
		}
	}
	result := make([]mir.FunctionID, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
func taskWrapperName(id mir.FunctionID) string {
	return fmt.Sprintf("tsnative_task_entry_f%d", id)
}

func (e *emitter) emitTaskWrappers(b *strings.Builder) error {
	for _, id := range e.taskTargets() {
		fn, ok := e.functions[id]
		if !ok {
			return fmt.Errorf("task wrapper references unknown function f%d", id)
		}
		if fn.ReturnRepr != mir.ReprVoid || len(fn.Params) != 0 {
			return fmt.Errorf("task target f%d must have native signature () -> void", id)
		}
		fmt.Fprintf(b, "define void @%s(ptr %%state) {\nentry:\n", taskWrapperName(id))
		fmt.Fprintf(b, "  call void @%s()\n", functionName(id))
		b.WriteString("  ret void\n}\n")
	}
	return nil
}
