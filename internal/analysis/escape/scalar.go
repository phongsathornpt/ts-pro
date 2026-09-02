package escape

import (
	"fmt"
	"sort"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

type ScalarFieldValue struct {
	Value mir.ValueID
	Zero  bool
	Phi   string
}

type ScalarFieldIncoming struct {
	Block  mir.BlockID
	Source ScalarFieldValue
}

type ScalarPhi struct {
	Name     string
	Block    mir.BlockID
	Field    uint32
	Incoming []ScalarFieldIncoming
}

type ScalarObject struct {
	Shape           mir.ShapeID
	Block           mir.BlockID
	Fields          []mir.ValueID
	Mutable         bool
	ZeroInitialized bool
	Reads           map[mir.ValueID]ScalarFieldValue
	Phis            map[string]ScalarPhi
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
	return ScalarObjectsWithEscapeAnalysis(module, stack, Analyze(module))
}

func ScalarObjectsWithEscapeAnalysis(module mir.Module, stack StackObjectResult, escapes Result) ScalarObjectResult {
	result := make(ScalarObjectResult, len(module.Functions))
	shapes := make(map[mir.ShapeID]mir.Shape, len(module.Shapes))
	for _, shape := range module.Shapes {
		shapes[shape.ID] = shape
	}
	for _, fn := range module.Functions {
		result[fn.ID] = scalarObjectsForFunction(fn, stack[fn.ID], escapes[fn.ID], shapes)
	}
	return result
}

func scalarObjectsForFunction(fn mir.Function, stack map[mir.ValueID]bool, escapes FunctionResult, shapes map[mir.ShapeID]mir.Shape) map[mir.ValueID]ScalarObject {
	candidates := scalarObjectCandidates(fn, stack, escapes, shapes)
	prov := make(provenance)
	for value := range candidates {
		prov[value] = valueSet{value: {}}
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
			if !candidates[inst.Result] || aliased[inst.Result] {
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
				reads, phis, ok := planMutableScalarDataflow(fn, inst.Result, scalar, fieldUseBlocks[inst.Result], fieldCount)
				if !ok {
					continue
				}
				scalar.Reads, scalar.Phis = reads, phis
			}
			result[inst.Result] = scalar
		}
	}
	return result
}

func scalarObjectCandidates(fn mir.Function, stack map[mir.ValueID]bool, escapes FunctionResult, shapes map[mir.ShapeID]mir.Shape) map[mir.ValueID]bool {
	result := make(map[mir.ValueID]bool, len(stack))
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if !stack[inst.Result] {
				continue
			}
			if _, ok := stackObjectShape(inst.Op); ok {
				result[inst.Result] = true
			}
		}
	}
	cyclic := cyclicBlocks(fn)
	blocked := stackBlockedValues(fn)
	for _, block := range fn.Blocks {
		if cyclic[block.ID] {
			continue
		}
		for _, inst := range block.Instructions {
			if result[inst.Result] || blocked[inst.Result] || !escapes.CanStackAllocate(inst.Result) {
				continue
			}
			shapeID, ok := stackObjectShape(inst.Op)
			if !ok {
				continue
			}
			if _, ok := shapes[shapeID]; !ok {
				continue
			}
			result[inst.Result] = true
		}
	}
	return result
}

func planMutableScalarDataflow(fn mir.Function, origin mir.ValueID, scalar ScalarObject, uses map[mir.BlockID]struct{}, fieldCount int) (map[mir.ValueID]ScalarFieldValue, map[string]ScalarPhi, bool) {
	blocks, predecessors, successors := scalarCFG(fn)
	order, relevant, ok := scalarDataflowOrder(fn, scalar.Block, uses, predecessors, successors)
	if !ok {
		return nil, nil, false
	}
	reads := make(map[mir.ValueID]ScalarFieldValue)
	phis := make(map[string]ScalarPhi)
	exit := make(map[mir.BlockID][]ScalarFieldValue)
	for _, blockID := range order {
		block := blocks[blockID]
		var fields []ScalarFieldValue
		if blockID == scalar.Block {
			fields = initialScalarFields(scalar, fieldCount)
		} else {
			preds := relevantPredecessors(predecessors[blockID], relevant)
			if len(preds) == 0 {
				return nil, nil, false
			}
			fields = make([]ScalarFieldValue, fieldCount)
			for field := range fields {
				states := make([]ScalarFieldIncoming, 0, len(preds))
				for _, pred := range preds {
					predState, exists := exit[pred]
					if !exists || field >= len(predState) {
						return nil, nil, false
					}
					states = append(states, ScalarFieldIncoming{Block: pred, Source: predState[field]})
				}
				fields[field] = mergeScalarField(origin, blockID, uint32(field), states, phis)
			}
		}
		applyScalarBlock(block, origin, fields, reads)
		exit[blockID] = cloneScalarFields(fields)
	}
	return reads, phis, true
}

func scalarDataflowOrder(fn mir.Function, start mir.BlockID, uses map[mir.BlockID]struct{}, predecessors, successors map[mir.BlockID][]mir.BlockID) ([]mir.BlockID, map[mir.BlockID]bool, bool) {
	forward := map[mir.BlockID]bool{start: true}
	queue := []mir.BlockID{start}
	for len(queue) != 0 {
		block := queue[0]
		queue = queue[1:]
		for _, next := range successors[block] {
			if !forward[next] {
				forward[next] = true
				queue = append(queue, next)
			}
		}
	}
	for use := range uses {
		if !forward[use] {
			return nil, nil, false
		}
	}
	backward := make(map[mir.BlockID]bool)
	queue = queue[:0]
	for use := range uses {
		if !backward[use] {
			backward[use] = true
			queue = append(queue, use)
		}
	}
	if len(queue) == 0 {
		backward[start] = true
		queue = append(queue, start)
	}
	for len(queue) != 0 {
		block := queue[0]
		queue = queue[1:]
		for _, pred := range predecessors[block] {
			if !backward[pred] {
				backward[pred] = true
				queue = append(queue, pred)
			}
		}
	}
	relevant := make(map[mir.BlockID]bool)
	for block := range forward {
		if backward[block] {
			relevant[block] = true
		}
	}
	relevant[start] = true
	cyclic := cyclicBlocks(fn)
	for block := range relevant {
		if cyclic[block] {
			return nil, nil, false
		}
	}
	indegree := make(map[mir.BlockID]int)
	for block := range relevant {
		for _, pred := range predecessors[block] {
			if relevant[pred] {
				indegree[block]++
			}
		}
	}
	ready := make([]mir.BlockID, 0)
	for block := range relevant {
		if indegree[block] == 0 {
			ready = append(ready, block)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
	order := make([]mir.BlockID, 0, len(relevant))
	for len(ready) != 0 {
		block := ready[0]
		ready = ready[1:]
		order = append(order, block)
		for _, next := range successors[block] {
			if !relevant[next] {
				continue
			}
			indegree[next]--
			if indegree[next] == 0 {
				ready = append(ready, next)
				sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
			}
		}
	}
	if len(order) != len(relevant) || len(order) == 0 || order[0] != start {
		return nil, nil, false
	}
	return order, relevant, true
}

func mergeScalarField(origin mir.ValueID, block mir.BlockID, field uint32, incoming []ScalarFieldIncoming, phis map[string]ScalarPhi) ScalarFieldValue {
	first := incoming[0].Source
	same := true
	for _, item := range incoming[1:] {
		if !sameScalarFieldValue(first, item.Source) {
			same = false
			break
		}
	}
	if same {
		return first
	}
	name := fmt.Sprintf("scalar.phi.v%d.f%d.b%d", origin, field, block)
	phis[name] = ScalarPhi{Name: name, Block: block, Field: field, Incoming: append([]ScalarFieldIncoming(nil), incoming...)}
	return ScalarFieldValue{Phi: name}
}

func scalarCFG(fn mir.Function) (map[mir.BlockID]mir.Block, map[mir.BlockID][]mir.BlockID, map[mir.BlockID][]mir.BlockID) {
	blocks := make(map[mir.BlockID]mir.Block, len(fn.Blocks))
	predecessors := make(map[mir.BlockID][]mir.BlockID)
	successors := make(map[mir.BlockID][]mir.BlockID)
	for _, block := range fn.Blocks {
		blocks[block.ID] = block
		switch term := block.Terminator.(type) {
		case mir.Jump:
			predecessors[term.Target] = append(predecessors[term.Target], block.ID)
			successors[block.ID] = append(successors[block.ID], term.Target)
		case mir.Branch:
			predecessors[term.Then] = append(predecessors[term.Then], block.ID)
			predecessors[term.Else] = append(predecessors[term.Else], block.ID)
			successors[block.ID] = append(successors[block.ID], term.Then, term.Else)
		}
	}
	return blocks, predecessors, successors
}

func relevantPredecessors(predecessors []mir.BlockID, relevant map[mir.BlockID]bool) []mir.BlockID {
	result := make([]mir.BlockID, 0, len(predecessors))
	for _, pred := range predecessors {
		if relevant[pred] {
			result = append(result, pred)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
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
				source := ScalarFieldValue{Value: op.Value}
				if read, ok := reads[op.Value]; ok {
					source = read
				}
				fields[op.Field] = source
			}
		case mir.FieldGet:
			if op.Object == origin && int(op.Field) < len(fields) {
				reads[inst.Result] = fields[op.Field]
			}
		}
	}
}

func sameScalarFieldValue(left, right ScalarFieldValue) bool {
	return left.Zero == right.Zero && left.Value == right.Value && left.Phi == right.Phi
}
