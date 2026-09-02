package escape

import "github.com/projectthorn/tsv7-bin/internal/mir"

type ScalarObject struct {
	Shape           mir.ShapeID
	Block           mir.BlockID
	Fields          []mir.ValueID
	Mutable         bool
	ZeroInitialized bool
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
	for _, fn := range module.Functions {
		result[fn.ID] = scalarObjectsForFunction(fn, stack[fn.ID])
	}
	return result
}
func scalarObjectsForFunction(fn mir.Function, stack map[mir.ValueID]bool) map[mir.ValueID]ScalarObject {
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
	fieldUseOutsideBlock := make(map[mir.ValueID]bool)
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			switch op := inst.Op.(type) {
			case mir.FieldSet:
				for origin := range prov[op.Object] {
					mutated[origin] = true
					if allocationBlocks[origin] != block.ID {
						fieldUseOutsideBlock[origin] = true
					}
				}
			case mir.FieldGet:
				for origin := range prov[op.Object] {
					if allocationBlocks[origin] != block.ID {
						fieldUseOutsideBlock[origin] = true
					}
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
			if isMutable && fieldUseOutsideBlock[inst.Result] {
				continue
			}
			switch object := inst.Op.(type) {
			case mir.ObjectNew:
				result[inst.Result] = ScalarObject{
					Shape: object.Shape, Block: block.ID, Fields: append([]mir.ValueID(nil), object.Fields...), Mutable: isMutable,
				}
			case mir.ObjectAlloc:
				result[inst.Result] = ScalarObject{
					Shape: object.Shape, Block: block.ID, Mutable: isMutable, ZeroInitialized: true,
				}
			}
		}
	}
	return result
}
