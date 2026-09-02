package escape

import "github.com/projectthorn/tsv7-bin/internal/mir"

type ScalarObject struct {
	Shape  mir.ShapeID
	Fields []mir.ValueID
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
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			set, ok := inst.Op.(mir.FieldSet)
			if !ok {
				continue
			}
			for origin := range prov[set.Object] {
				mutated[origin] = true
			}
		}
	}
	result := make(map[mir.ValueID]ScalarObject)
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if !stack[inst.Result] || aliased[inst.Result] || mutated[inst.Result] {
				continue
			}
			object, ok := inst.Op.(mir.ObjectNew)
			if !ok {
				continue
			}
			result[inst.Result] = ScalarObject{
				Shape:  object.Shape,
				Fields: append([]mir.ValueID(nil), object.Fields...),
			}
		}
	}
	return result
}
