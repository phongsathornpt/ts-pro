package escape

import "github.com/projectthorn/tsv7-bin/internal/mir"

type StackObjectResult map[mir.FunctionID]map[mir.ValueID]bool

func (result StackObjectResult) Contains(fn mir.FunctionID, value mir.ValueID) bool {
	return result[fn] != nil && result[fn][value]
}

func StackObjects(module mir.Module, escapes Result) StackObjectResult {
	shapes := make(map[mir.ShapeID]mir.Shape, len(module.Shapes))
	for _, shape := range module.Shapes {
		shapes[shape.ID] = shape
	}
	result := make(StackObjectResult, len(module.Functions))
	for _, fn := range module.Functions {
		result[fn.ID] = stackObjectsForFunction(fn, shapes, escapes[fn.ID])
	}
	return result
}

func stackObjectsForFunction(fn mir.Function, shapes map[mir.ShapeID]mir.Shape, escapes FunctionResult) map[mir.ValueID]bool {
	result := make(map[mir.ValueID]bool)
	cyclic := cyclicBlocks(fn)
	blocked := stackBlockedValues(fn)
	aliased := stackAliasedValues(fn)
	for _, block := range fn.Blocks {
		if cyclic[block.ID] {
			continue
		}
		for _, inst := range block.Instructions {
			if blocked[inst.Result] || aliased[inst.Result] || !escapes.CanStackAllocate(inst.Result) {
				continue
			}
			if _, ok := inst.Op.(mir.ClosureNew); ok {
				result[inst.Result] = true
				continue
			}
			shapeID, ok := stackObjectShape(inst.Op)
			if !ok {
				continue
			}
			shape, ok := shapes[shapeID]
			if !ok || !stackSupportedShape(shape) {
				continue
			}
			result[inst.Result] = true
		}
	}
	return result
}

func stackObjectShape(op mir.Operation) (mir.ShapeID, bool) {
	switch op := op.(type) {
	case mir.ObjectNew:
		return op.Shape, true
	case mir.ObjectAlloc:
		return op.Shape, true
	default:
		return 0, false
	}
}
func stackSupportedShape(shape mir.Shape) bool {
	return shape.ClassTag != 0 || len(shape.Fields) != 0
}

func stackAliasedValues(fn mir.Function) map[mir.ValueID]bool {
	prov := make(provenance)
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if allocationKind(inst.Op) != AllocationInvalid {
				prov[inst.Result] = valueSet{inst.Result: {}}
			}
		}
	}
	propagateAliases(fn, prov)
	aliased := make(map[mir.ValueID]bool)
	for value, origins := range prov {
		for origin := range origins {
			if value != origin {
				aliased[origin] = true
			}
		}
	}
	return aliased
}

func stackBlockedValues(fn mir.Function) map[mir.ValueID]bool {
	prov := make(provenance)
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if allocationKind(inst.Op) != AllocationInvalid {
				prov[inst.Result] = valueSet{inst.Result: {}}
			}
		}
	}
	propagateAliases(fn, prov)
	blocked := make(map[mir.ValueID]bool)
	blockOrigins := func(value mir.ValueID) {
		for origin := range prov[value] {
			blocked[origin] = true
		}
	}
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			switch op := inst.Op.(type) {
			case mir.ObjectNew:
				for _, field := range op.Fields {
					blockOrigins(field)
				}
			case mir.FieldSet:
				blockOrigins(op.Value)
			case mir.ClosureNew:
				for _, capture := range op.Captures {
					blockOrigins(capture)
				}
			}
		}
	}
	return blocked
}

func cyclicBlocks(fn mir.Function) map[mir.BlockID]bool {
	successors := make(map[mir.BlockID][]mir.BlockID, len(fn.Blocks))
	for _, block := range fn.Blocks {
		successors[block.ID] = blockSuccessors(block.Terminator)
	}
	cyclic := make(map[mir.BlockID]bool)
	for _, block := range fn.Blocks {
		if reachesBlock(successors, block.ID, block.ID) {
			cyclic[block.ID] = true
		}
	}
	return cyclic
}
func blockSuccessors(term mir.Terminator) []mir.BlockID {
	switch term := term.(type) {
	case mir.Jump:
		return []mir.BlockID{term.Target}
	case mir.Branch:
		if term.Then == term.Else {
			return []mir.BlockID{term.Then}
		}
		return []mir.BlockID{term.Then, term.Else}
	default:
		return nil
	}
}

func reachesBlock(graph map[mir.BlockID][]mir.BlockID, start, target mir.BlockID) bool {
	seen := map[mir.BlockID]bool{start: true}
	queue := append([]mir.BlockID(nil), graph[start]...)
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if current == target {
			return true
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		queue = append(queue, graph[current]...)
	}
	return false
}
