package escape

import "github.com/phongsathornpt/ts-pro/internal/mir"

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
	aliased := stackUnsafeAliasOrigins(fn)
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

func safeSingleOriginAliases(fn mir.Function) map[mir.ValueID]mir.ValueID {
	aliases := make(map[mir.ValueID]mir.ValueID)
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if allocationKind(inst.Op) != AllocationInvalid {
				aliases[inst.Result] = inst.Result
			}
		}
	}
	changed := true
	for changed {
		changed = false
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				phi, ok := inst.Op.(mir.Phi)
				if !ok || len(phi.Incoming) == 0 {
					continue
				}
				var origin mir.ValueID
				valid := true
				for i, incoming := range phi.Incoming {
					candidate, ok := aliases[incoming.Value]
					if !ok {
						valid = false
						break
					}
					if i == 0 {
						origin = candidate
					} else if candidate != origin {
						valid = false
						break
					}
				}
				if valid {
					current, ok := aliases[inst.Result]
					if !ok || current != origin {
						aliases[inst.Result] = origin
						changed = true
					}
				}
			}
		}
	}
	return aliases
}

func stackUnsafeAliasOrigins(fn mir.Function) map[mir.ValueID]bool {
	prov := make(provenance)
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if allocationKind(inst.Op) != AllocationInvalid {
				prov[inst.Result] = valueSet{inst.Result: {}}
			}
		}
	}
	propagateAliases(fn, prov)
	safe := safeSingleOriginAliases(fn)
	blocked := make(map[mir.ValueID]bool)
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if _, ok := inst.Op.(mir.Phi); !ok {
				continue
			}
			for origin := range prov[inst.Result] {
				safeOrigin, ok := safe[inst.Result]
				if !ok || safeOrigin != origin {
					blocked[origin] = true
				}
			}
		}
	}
	return blocked
}

func StackObjectAliases(module mir.Module, stack StackObjectResult) map[mir.FunctionID]map[mir.ValueID]mir.ValueID {
	result := make(map[mir.FunctionID]map[mir.ValueID]mir.ValueID, len(module.Functions))
	for _, fn := range module.Functions {
		aliases := safeSingleOriginAliases(fn)
		selected := make(map[mir.ValueID]mir.ValueID)
		for value, origin := range aliases {
			if value != origin && stack.Contains(fn.ID, origin) {
				selected[value] = origin
			}
		}
		result[fn.ID] = selected
	}
	return result
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
