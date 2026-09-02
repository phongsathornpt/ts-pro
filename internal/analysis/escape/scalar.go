package escape

import "github.com/projectthorn/tsv7-bin/internal/mir"

type ScalarFieldValue struct {
	Value mir.ValueID
	Zero  bool
}

type ScalarObject struct {
	Shape           mir.ShapeID
	Block           mir.BlockID
	Fields          []mir.ValueID
	Mutable         bool
	ZeroInitialized bool
	Reads           map[mir.ValueID]ScalarFieldValue
}

type ScalarObjectResult map[mir.FunctionID]map[mir.ValueID]ScalarObject

func (result ScalarObjectResult) Get(fn mir.FunctionID, value mir.ValueID) (ScalarObject, bool) {
	if result[fn] == nil {
		return ScalarObject{}, false
	}
	object, ok := result[fn][value]
	return object, ok
}

func ScalarObjects(module mir.Module, stack StackObjectResult) ScalarObjectResult {
	result := make(ScalarObjectResult, len(module.Functions))
	shapes := make(map[mir.ShapeID]mir.Shape, len(module.Shapes))
	for _, shape := range module.Shapes {
		shapes[shape.ID] = shape
	}
	for _, fn := range module.Functions {
		result[fn.ID] = scalarObjectsForFunction(fn, stack[fn.ID], shapes)
	}
	return result
}

func scalarObjectsForFunction(fn mir.Function, stack map[mir.ValueID]bool, shapes map[mir.ShapeID]mir.Shape) map[mir.ValueID]ScalarObject {
	prov := make(provenance)
	allocationBlocks := make(map[mir.ValueID]mir.BlockID)
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if stack[inst.Result] {
				prov[inst.Result] = valueSet{inst.Result: {}}
				allocationBlocks[inst.Result] = block.ID
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

	mutated := make(map[mir.ValueID]bool)
	fieldUseBlocks := make(map[mir.ValueID]map[mir.BlockID]struct{})
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			var object mir.ValueID
			switch op := inst.Op.(type) {
			case mir.FieldSet:
				object = op.Object
			case mir.FieldGet:
				object = op.Object
			default:
				continue
			}
			for origin := range prov[object] {
				if fieldUseBlocks[origin] == nil {
					fieldUseBlocks[origin] = make(map[mir.BlockID]struct{})
				}
				fieldUseBlocks[origin][block.ID] = struct{}{}
				if _, ok := inst.Op.(mir.FieldSet); ok {
					mutated[origin] = true
				}
			}
		}
	}

	result := make(map[mir.ValueID]ScalarObject)
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if !stack[inst.Result] || aliased[inst.Result] {
				continue
			}
			isMutable := mutated[inst.Result]
			if isMutable && !fieldUsesFollowLinearChain(fn, block.ID, fieldUseBlocks[inst.Result]) {
				continue
			}
			var scalar ScalarObject
			switch object := inst.Op.(type) {
			case mir.ObjectNew:
				scalar = ScalarObject{Shape: object.Shape, Block: block.ID, Fields: append([]mir.ValueID(nil), object.Fields...), Mutable: isMutable}
			case mir.ObjectAlloc:
				scalar = ScalarObject{Shape: object.Shape, Block: block.ID, Mutable: isMutable, ZeroInitialized: true}
			default:
				continue
			}
			if scalar.Mutable {
				fieldCount := len(scalar.Fields)
				if scalar.ZeroInitialized {
					fieldCount = len(shapes[scalar.Shape].Fields)
				}
				scalar.Reads = buildMutableScalarReads(fn, inst.Result, scalar, fieldUseBlocks[inst.Result], fieldCount)
			}
			result[inst.Result] = scalar
		}
	}
	return result
}

func fieldUsesFollowLinearChain(fn mir.Function, allocation mir.BlockID, uses map[mir.BlockID]struct{}) bool {
	remaining := make(map[mir.BlockID]struct{}, len(uses))
	for block := range uses {
		if block != allocation {
			remaining[block] = struct{}{}
		}
	}
	if len(remaining) == 0 {
		return true
	}
	blocks := make(map[mir.BlockID]mir.Block, len(fn.Blocks))
	predecessors := make(map[mir.BlockID][]mir.BlockID)
	for _, block := range fn.Blocks {
		blocks[block.ID] = block
		switch term := block.Terminator.(type) {
		case mir.Jump:
			predecessors[term.Target] = append(predecessors[term.Target], block.ID)
		case mir.Branch:
			predecessors[term.Then] = append(predecessors[term.Then], block.ID)
			predecessors[term.Else] = append(predecessors[term.Else], block.ID)
		}
	}
	current := allocation
	seen := map[mir.BlockID]bool{current: true}
	for len(remaining) != 0 {
		jump, ok := blocks[current].Terminator.(mir.Jump)
		if !ok || seen[jump.Target] {
			return false
		}
		preds := predecessors[jump.Target]
		if len(preds) != 1 || preds[0] != current {
			return false
		}
		current = jump.Target
		seen[current] = true
		delete(remaining, current)
	}
	return true
}

func buildMutableScalarReads(fn mir.Function, origin mir.ValueID, scalar ScalarObject, uses map[mir.BlockID]struct{}, fieldCount int) map[mir.ValueID]ScalarFieldValue {
	reads := make(map[mir.ValueID]ScalarFieldValue)
	fields := make([]ScalarFieldValue, fieldCount)
	if scalar.ZeroInitialized {
		for i := range fields {
			fields[i].Zero = true
		}
	} else {
		for i, value := range scalar.Fields {
			fields[i].Value = value
		}
	}
	blocks := make(map[mir.BlockID]mir.Block, len(fn.Blocks))
	for _, block := range fn.Blocks {
		blocks[block.ID] = block
	}
	current := scalar.Block
	seen := make(map[mir.BlockID]bool)
	for {
		if seen[current] {
			break
		}
		seen[current] = true
		block := blocks[current]
		for _, inst := range block.Instructions {
			switch op := inst.Op.(type) {
			case mir.FieldSet:
				if op.Object == origin && int(op.Field) < len(fields) {
					fields[op.Field] = ScalarFieldValue{Value: op.Value}
				}
			case mir.FieldGet:
				if op.Object == origin && int(op.Field) < len(fields) {
					reads[inst.Result] = fields[op.Field]
				}
			}
		}
		remaining := false
		for useBlock := range uses {
			if !seen[useBlock] {
				remaining = true
				break
			}
		}
		if !remaining {
			break
		}
		jump, ok := block.Terminator.(mir.Jump)
		if !ok {
			break
		}
		current = jump.Target
	}
	return reads
}
