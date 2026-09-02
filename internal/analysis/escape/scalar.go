package escape

import "github.com/projectthorn/tsv7-bin/internal/mir"

type ScalarFieldIncoming struct {
	Block mir.BlockID
	Value mir.ValueID
	Zero  bool
}

type ScalarFieldValue struct {
	Value    mir.ValueID
	Zero     bool
	Incoming []ScalarFieldIncoming
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
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if stack[inst.Result] {
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
				reads, ok := planMutableScalarReads(fn, inst.Result, scalar, fieldUseBlocks[inst.Result], fieldCount)
				if !ok {
					continue
				}
				scalar.Reads = reads
			}
			result[inst.Result] = scalar
		}
	}
	return result
}

func planMutableScalarReads(fn mir.Function, origin mir.ValueID, scalar ScalarObject, uses map[mir.BlockID]struct{}, fieldCount int) (map[mir.ValueID]ScalarFieldValue, bool) {
	if fieldUsesFollowLinearChain(fn, scalar.Block, uses) {
		return buildLinearMutableScalarReads(fn, origin, scalar, uses, fieldCount), true
	}
	return buildDiamondMutableScalarReads(fn, origin, scalar, uses, fieldCount)
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
	blocks, predecessors := scalarCFG(fn)
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

func buildLinearMutableScalarReads(fn mir.Function, origin mir.ValueID, scalar ScalarObject, uses map[mir.BlockID]struct{}, fieldCount int) map[mir.ValueID]ScalarFieldValue {
	reads := make(map[mir.ValueID]ScalarFieldValue)
	fields := initialScalarFields(scalar, fieldCount)
	blocks, _ := scalarCFG(fn)
	current := scalar.Block
	seen := make(map[mir.BlockID]bool)
	for {
		if seen[current] {
			break
		}
		seen[current] = true
		block := blocks[current]
		applyScalarBlock(block, origin, fields, reads)
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

func buildDiamondMutableScalarReads(fn mir.Function, origin mir.ValueID, scalar ScalarObject, uses map[mir.BlockID]struct{}, fieldCount int) (map[mir.ValueID]ScalarFieldValue, bool) {
	blocks, predecessors := scalarCFG(fn)
	allocation, ok := blocks[scalar.Block]
	if !ok {
		return nil, false
	}
	branch, ok := allocation.Terminator.(mir.Branch)
	if !ok || branch.Then == branch.Else {
		return nil, false
	}
	thenBlock, thenOK := blocks[branch.Then]
	elseBlock, elseOK := blocks[branch.Else]
	if !thenOK || !elseOK {
		return nil, false
	}
	thenJump, thenOK := thenBlock.Terminator.(mir.Jump)
	elseJump, elseOK := elseBlock.Terminator.(mir.Jump)
	if !thenOK || !elseOK || thenJump.Target != elseJump.Target {
		return nil, false
	}
	mergeID := thenJump.Target
	mergeBlock, mergeOK := blocks[mergeID]
	if !mergeOK {
		return nil, false
	}
	preds := predecessors[mergeID]
	if len(preds) != 2 || !containsBlock(preds, branch.Then) || !containsBlock(preds, branch.Else) {
		return nil, false
	}
	allowed := map[mir.BlockID]bool{scalar.Block: true, branch.Then: true, branch.Else: true, mergeID: true}
	for useBlock := range uses {
		if !allowed[useBlock] {
			return nil, false
		}
	}

	reads := make(map[mir.ValueID]ScalarFieldValue)
	base := initialScalarFields(scalar, fieldCount)
	applyScalarBlock(allocation, origin, base, reads)
	thenState := cloneScalarFields(base)
	elseState := cloneScalarFields(base)
	applyScalarBlock(thenBlock, origin, thenState, reads)
	applyScalarBlock(elseBlock, origin, elseState, reads)
	merged := make([]ScalarFieldValue, fieldCount)
	for i := range merged {
		if sameScalarFieldValue(thenState[i], elseState[i]) {
			merged[i] = thenState[i]
			continue
		}
		merged[i] = ScalarFieldValue{Incoming: []ScalarFieldIncoming{
			{Block: branch.Then, Value: thenState[i].Value, Zero: thenState[i].Zero},
			{Block: branch.Else, Value: elseState[i].Value, Zero: elseState[i].Zero},
		}}
	}
	applyScalarBlock(mergeBlock, origin, merged, reads)
	return reads, true
}

func scalarCFG(fn mir.Function) (map[mir.BlockID]mir.Block, map[mir.BlockID][]mir.BlockID) {
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
	return blocks, predecessors
}

func initialScalarFields(scalar ScalarObject, fieldCount int) []ScalarFieldValue {
	fields := make([]ScalarFieldValue, fieldCount)
	if scalar.ZeroInitialized {
		for i := range fields {
			fields[i].Zero = true
		}
		return fields
	}
	for i, value := range scalar.Fields {
		fields[i].Value = value
	}
	return fields
}

func cloneScalarFields(source []ScalarFieldValue) []ScalarFieldValue {
	result := make([]ScalarFieldValue, len(source))
	copy(result, source)
	return result
}

func applyScalarBlock(block mir.Block, origin mir.ValueID, fields []ScalarFieldValue, reads map[mir.ValueID]ScalarFieldValue) {
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
}

func sameScalarFieldValue(left, right ScalarFieldValue) bool {
	return left.Zero == right.Zero && left.Value == right.Value && len(left.Incoming) == 0 && len(right.Incoming) == 0
}

func containsBlock(blocks []mir.BlockID, target mir.BlockID) bool {
	for _, block := range blocks {
		if block == target {
			return true
		}
	}
	return false
}
