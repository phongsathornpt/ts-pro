package golang

import (
	"fmt"
	"go/format"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/mir"
)

// Emit produces a standalone Go program from the currently supported pure-Go
// MIR subset. The backend deliberately rejects native layouts and operations
// that still depend on the legacy LLVM/C ABI.
func Emit(module mir.Module) (string, error) {
	if err := module.Verify(); err != nil {
		return "", fmt.Errorf("verify MIR before Go emission: %w", err)
	}
	if module.Entry == nil {
		return "", fmt.Errorf("Go emission requires a module entry function")
	}

	canonicalShapes := make(map[mir.ShapeID]mir.ShapeID, len(module.Shapes))
	fingerprints := make(map[string]mir.ShapeID, len(module.Shapes))
	for _, shape := range module.Shapes {
		var b strings.Builder
		for _, f := range shape.Fields {
			fmt.Fprintf(&b, "%s:%d;", f.Name, f.Repr)
		}
		fp := b.String()
		if canon, ok := fingerprints[fp]; ok {
			canonicalShapes[shape.ID] = canon
		} else {
			fingerprints[fp] = shape.ID
			canonicalShapes[shape.ID] = shape.ID
		}
	}

	g := &generator{
		module:          module,
		functions:       make(map[mir.FunctionID]mir.Function, len(module.Functions)),
		shapes:          make(map[mir.ShapeID]mir.Shape, len(module.Shapes)),
		canonicalShapes: canonicalShapes,
	}
	for _, fn := range module.Functions {
		g.functions[fn.ID] = fn
	}
	for _, shape := range module.Shapes {
		g.shapes[shape.ID] = shape
	}

	var body strings.Builder
	if err := g.emitShapeTypes(&body); err != nil {
		return "", err
	}
	functions := append([]mir.Function(nil), module.Functions...)
	sort.Slice(functions, func(i, j int) bool { return functions[i].ID < functions[j].ID })
	for _, fn := range functions {
		if err := g.emitFunction(&body, fn); err != nil {
			return "", err
		}
	}

	entry, ok := g.functions[*module.Entry]
	if !ok {
		return "", fmt.Errorf("module entry function f%d is missing", *module.Entry)
	}
	body.WriteString("func main() {\n")
	call := goFunctionName(entry.ID) + "()"
	if entry.ReturnRepr != mir.ReprVoid {
		fmt.Fprintf(&body, "\t_ = %s\n", call)
	} else {
		fmt.Fprintf(&body, "\t%s\n", call)
	}
	if g.usesTasks {
		g.usesSync = true
		body.WriteString("\ttsAllTasks.Wait()\n")
	}
	body.WriteString("}\n")

	if g.usesJSConvert || g.usesJSAdd || g.usesDynamicField {
		g.usesJSConvert = true
		g.usesFmt = true
		g.usesMath = true
	}
	if g.usesPrintValue {
		g.usesFmt = true
	}

	var source strings.Builder
	source.WriteString("package main\n\n")
	if g.usesFmt || g.usesMath || g.usesRuntime || g.usesSync || g.usesTime || g.usesReflect {
		source.WriteString("import (\n")
		if g.usesFmt {
			source.WriteString("\t\"fmt\"\n")
		}
		if g.usesMath {
			source.WriteString("\t\"math\"\n")
		}
		if g.usesReflect {
			source.WriteString("\t\"reflect\"\n")
		}
		if g.usesRuntime {
			source.WriteString("\t\"runtime\"\n")
		}
		if g.usesSync {
			source.WriteString("\t\"sync\"\n")
		}
		if g.usesTime {
			source.WriteString("\t\"time\"\n")
		}
		source.WriteString(")\n\n")
	}

	if g.usesTasks || g.usesExceptions {
		if g.usesTasks {
			source.WriteString("var tsAllTasks sync.WaitGroup\n\n")
		}
		source.WriteString("type tsTask struct {\n\tval any\n\terr any\n\tdone chan struct{}\n}\n\n")
		source.WriteString("type tsException struct {\n\tval any\n}\n\n")
		source.WriteString("func tsDoneChan() chan struct{} {\n\tc := make(chan struct{})\n\tclose(c)\n\treturn c\n}\n\n")
	}
	if g.usesTaskGroups {
		source.WriteString("type tsTaskGroup struct {\n\twg sync.WaitGroup\n\tmu sync.Mutex\n\tcancelled bool\n}\n\n")
	}
	if g.usesJSConvert {
		source.WriteString(`func tsToF64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func tsToString(v any) string {
	if v == nil {
		return "undefined"
	}
	return fmt.Sprint(v)
}

func tsToBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0 && !math.IsNaN(x)
	case string:
		return x != ""
	default:
		return v != nil
	}
}

`)
	}
	if g.usesJSAdd {
		source.WriteString(`func tsDynamicAdd(left, right any) any {
	if s, ok := left.(string); ok {
		return s + tsToString(right)
	}
	if s, ok := right.(string); ok {
		return tsToString(left) + s
	}
	return tsToF64(left) + tsToF64(right)
}

`)
	}
	if g.usesPrintValue {
		source.WriteString(`func tsPrintValue(v any) {
	if v == nil {
		fmt.Println("undefined")
	} else {
		fmt.Println(v)
	}
}

`)
	}
	if g.usesDynamicCall {
		source.WriteString(`func tsDynamicCall(callee any, args ...any) any {
	fnVal := reflect.ValueOf(callee)
	fnType := fnVal.Type()
	numIn := fnType.NumIn()
	var callArgs []any
	if len(args) == numIn {
		callArgs = args
	} else if len(args) == numIn+1 {
		callArgs = args[1:]
	} else {
		callArgs = args
	}
	rArgs := make([]reflect.Value, len(callArgs))
	for i, arg := range callArgs {
		targetType := fnType.In(i)
		if arg == nil {
			rArgs[i] = reflect.Zero(targetType)
		} else {
			val := reflect.ValueOf(arg)
			if val.Type().AssignableTo(targetType) {
				rArgs[i] = val
			} else if val.Type().ConvertibleTo(targetType) {
				rArgs[i] = val.Convert(targetType)
			} else {
				rArgs[i] = val
			}
		}
	}
	results := fnVal.Call(rArgs)
	if len(results) == 0 {
		return nil
	}
	return results[0].Interface()
}

`)
	}
	if g.usesClassTag {
		g.emitClassTagHelper(&source)
	}
	if g.usesDynamicField {
		g.emitDynamicFieldHelpers(&source)
	}
	if g.usesFieldHelpers {
		g.emitFieldHelpers(&source)
	}
	source.WriteString(body.String())

	formatted, err := format.Source([]byte(source.String()))
	if err != nil {
		return "", fmt.Errorf("format generated Go: %w\n%s", err, source.String())
	}
	return string(formatted), nil
}

type generator struct {
	module         mir.Module
	functions      map[mir.FunctionID]mir.Function
	shapes          map[mir.ShapeID]mir.Shape
	canonicalShapes map[mir.ShapeID]mir.ShapeID
	usesFmt         bool
	usesMath        bool
	usesRuntime     bool
	usesSync        bool
	usesTime        bool
	usesTasks        bool
	usesTaskGroups   bool
	usesJSConvert    bool
	usesJSAdd        bool
	usesReflect      bool
	usesPrintValue   bool
	usesDynamicCall  bool
	usesDynamicField bool
	usesClassTag     bool
	usesFieldHelpers bool
	usesExceptions   bool
}

func (g *generator) canonicalShape(id mir.ShapeID) mir.ShapeID {
	if canon, ok := g.canonicalShapes[id]; ok {
		return canon
	}
	return id
}

func (g *generator) emitFunction(out *strings.Builder, fn mir.Function) error {
	valueTypes := make(map[mir.ValueID]mir.Repr)
	valueShapes := make(map[mir.ValueID]mir.ShapeID)
	valueHasShapes := make(map[mir.ValueID]bool)
	paramValues := make(map[mir.ValueID]struct{}, len(fn.Params))
	for _, param := range fn.Params {
		if err := addValueType(valueTypes, param.Value, param.Repr); err != nil {
			return fmt.Errorf("function %s: %w", fn.Name, err)
		}
		if param.HasObjectShape && param.Repr != mir.ReprObjectRef {
			valueShapes[param.Value] = param.ObjectShape
			valueHasShapes[param.Value] = true
		}
		paramValues[param.Value] = struct{}{}
	}
	taskSpawns := make(map[mir.ValueID]mir.TaskSpawn)
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if spawn, ok := inst.Op.(mir.TaskSpawn); ok {
				taskSpawns[inst.Result] = spawn
			}
			if inst.Repr == mir.ReprVoid {
				continue
			}
			if err := addValueType(valueTypes, inst.Result, inst.Repr); err != nil {
				return fmt.Errorf("function %s: %w", fn.Name, err)
			}
			switch op := inst.Op.(type) {
			case mir.ObjectNew:
				valueShapes[inst.Result] = g.canonicalShape(op.Shape)
				valueHasShapes[inst.Result] = true
			case mir.ObjectAlloc:
				valueShapes[inst.Result] = g.canonicalShape(op.Shape)
				valueHasShapes[inst.Result] = true
			case mir.FieldGet:
				field, ok := g.shapeField(g.canonicalShape(op.Shape), op.Field)
				if ok && field.HasObjectShape {
					valueShapes[inst.Result] = g.canonicalShape(field.ObjectShape)
					valueHasShapes[inst.Result] = true
				}
			case mir.Call:
				if callee, ok := g.functions[op.Callee]; ok && callee.HasReturnObjectShape {
					valueShapes[inst.Result] = g.canonicalShape(callee.ReturnObjectShape)
					valueHasShapes[inst.Result] = true
				}
			case mir.TaskJoin:
				if spawn, ok := taskSpawns[op.Task]; ok {
					if callee, ok := g.functions[spawn.Callee]; ok && callee.HasReturnObjectShape {
						valueShapes[inst.Result] = g.canonicalShape(callee.ReturnObjectShape)
						valueHasShapes[inst.Result] = true
					}
				}
			}
		}
	}

	changed := true
	for changed {
		changed = false
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				phi, ok := inst.Op.(mir.Phi)
				if !ok {
					continue
				}
				if valueHasShapes[inst.Result] {
					continue
				}
				for _, in := range phi.Incoming {
					if shape, ok := valueShapes[in.Value]; ok && valueHasShapes[in.Value] {
						valueShapes[inst.Result] = shape
						valueHasShapes[inst.Result] = true
						changed = true
						break
					}
				}
			}
		}
	}

	returnType, err := g.goTypeForValue(fn.ReturnRepr, g.canonicalShape(fn.ReturnObjectShape), fn.HasReturnObjectShape)
	if err != nil {
		return fmt.Errorf("function %s return: %w", fn.Name, err)
	}
	fmt.Fprintf(out, "func %s(", goFunctionName(fn.ID))
	for i, param := range fn.Params {
		if i != 0 {
			out.WriteString(", ")
		}
		paramType, err := g.goTypeForValue(param.Repr, param.ObjectShape, param.HasObjectShape)
		if err != nil {
			return fmt.Errorf("function %s parameter %s: %w", fn.Name, param.Name, err)
		}
		if param.Repr == mir.ReprObjectRef {
			paramType = "any"
		}
		fmt.Fprintf(out, "%s %s", goValueName(param.Value), paramType)
	}
	out.WriteString(")")
	if returnType != "" {
		fmt.Fprintf(out, " %s", returnType)
	}
	out.WriteString(" {\n")

	valueIDs := make([]mir.ValueID, 0, len(valueTypes))
	for value := range valueTypes {
		valueIDs = append(valueIDs, value)
	}
	sort.Slice(valueIDs, func(i, j int) bool { return valueIDs[i] < valueIDs[j] })
	for _, value := range valueIDs {
		if _, isParam := paramValues[value]; isParam {
			continue
		}
		typ, err := g.goTypeForValue(valueTypes[value], valueShapes[value], valueHasShapes[value])
		if err != nil {
			return fmt.Errorf("function %s value v%d: %w", fn.Name, value, err)
		}
		fmt.Fprintf(out, "\tvar %s %s\n", goValueName(value), typ)
	}
	for _, value := range valueIDs {
		fmt.Fprintf(out, "\t_ = %s\n", goValueName(value))
	}

	blocks := append([]mir.Block(nil), fn.Blocks...)
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].ID < blocks[j].ID })
	phis := functionPhis(blocks)
	targets := functionTargets(blocks)
	for _, block := range blocks {
		if targets[block.ID] {
			fmt.Fprintf(out, "block%d:\n", block.ID)
		}
		for _, inst := range block.Instructions {
			if _, ok := inst.Op.(mir.Phi); ok {
				continue
			}
			if err := g.emitInstruction(out, fn, block.ID, inst, valueTypes, valueShapes, valueHasShapes); err != nil {
				return err
			}
		}
		if err := g.emitTerminator(out, fn, block.ID, block.Terminator, phis, valueTypes, valueShapes, valueHasShapes); err != nil {
			return err
		}
	}

	if returnType == "" {
		out.WriteString("\treturn\n")
	} else {
		fmt.Fprintf(out, "\treturn %s\n", zeroValue(fn.ReturnRepr))
	}
	out.WriteString("}\n\n")
	return nil
}

func (g *generator) emitShapeTypes(out *strings.Builder) error {
	emitted := make(map[mir.ShapeID]bool)
	shapes := append([]mir.Shape(nil), g.module.Shapes...)
	sort.Slice(shapes, func(i, j int) bool { return shapes[i].ID < shapes[j].ID })
	for _, shape := range shapes {
		canon := g.canonicalShape(shape.ID)
		if emitted[canon] {
			continue
		}
		emitted[canon] = true
		fmt.Fprintf(out, "type %s struct {\n", goShapeName(canon))
		fmt.Fprintf(out, "\tclassTag uint32\n")
		for index, field := range shape.Fields {
			typ, err := g.goTypeForValue(field.Repr, field.ObjectShape, field.HasObjectShape)
			if err != nil {
				return fmt.Errorf("shape s%d field %d: %w", canon, index, err)
			}
			fmt.Fprintf(out, "\t%s %s\n", goFieldName(uint32(index)), typ)
		}
		out.WriteString("}\n\n")
	}
	return nil
}

func (g *generator) emitInstruction(out *strings.Builder, fn mir.Function, block mir.BlockID, inst mir.Instruction, valueTypes map[mir.ValueID]mir.Repr, valueShapes map[mir.ValueID]mir.ShapeID, valueHasShapes map[mir.ValueID]bool) error {
	result := goValueName(inst.Result)
	assign := func(expression string) {
		fmt.Fprintf(out, "\t%s = %s\n", result, expression)
	}
	operand := func(value mir.ValueID) string { return goValueName(value) }

	switch op := inst.Op.(type) {
	case mir.ConstBool:
		assign(strconv.FormatBool(op.Value))
	case mir.ConstF64:
		literal := g.floatLiteral(op.Value)
		assign(literal)
	case mir.ConstString:
		assign(strconv.Quote(op.Value))
	case mir.StringConcat:
		assign(operand(op.Left) + " + " + operand(op.Right))
	case mir.FloatBinary:
		operator, ok := floatOperator(op.Operator)
		if !ok {
			return unsupportedInstruction(fn, block, inst, "float operator")
		}
		assign(operand(op.Left) + " " + operator + " " + operand(op.Right))
	case mir.ProvenIntBinary:
		operator, ok := intOperator(op.Operator)
		if !ok {
			return unsupportedInstruction(fn, block, inst, "integer operator")
		}
		var typ string
		switch op.Width {
		case mir.IntWidth32:
			typ = "int32"
		case mir.IntWidth64:
			typ = "int64"
		default:
			return unsupportedInstruction(fn, block, inst, "integer width")
		}
		// MIR keeps the proven integer operation's public representation as
		// F64, so preserve the same checked conversion boundary as LLVM.
		assign(fmt.Sprintf("float64(%s(%s) %s %s(%s))", typ, operand(op.Left), operator, typ, operand(op.Right)))
	case mir.FloatCompare:
		operator, ok := compareOperator(op.Operator)
		if !ok {
			return unsupportedInstruction(fn, block, inst, "comparison operator")
		}
		assign(operand(op.Left) + " " + operator + " " + operand(op.Right))
	case mir.ArrayNewF64:
		elements := make([]string, len(op.Elements))
		for i, element := range op.Elements {
			elements[i] = operand(element)
		}
		assign("[]float64{" + strings.Join(elements, ", ") + "}")
	case mir.ArrayLengthF64:
		assign("float64(len(" + operand(op.Array) + ".([]float64)))")
	case mir.ArrayGetF64:
		assign(fmt.Sprintf("%s.([]float64)[int(%s)]", operand(op.Array), operand(op.Index)))
	case mir.ArraySetF64:
		fmt.Fprintf(out, "\t%s.([]float64)[int(%s)] = %s\n", operand(op.Array), operand(op.Index), operand(op.Value))
		assign(operand(op.Value))
	case mir.ObjectNew:
		canon := g.canonicalShape(op.Shape)
		shape, ok := g.shapes[canon]
		if !ok {
			return unsupportedInstruction(fn, block, inst, fmt.Sprintf("missing shape s%d", op.Shape))
		}
		if len(shape.Fields) != len(op.Fields) {
			return unsupportedInstruction(fn, block, inst, "object field count mismatch")
		}
		fields := make([]string, 0, len(op.Fields)+1)
		origShape := g.shapes[op.Shape]
		if origShape.ClassTag != 0 {
			fields = append(fields, fmt.Sprintf("classTag: %d", origShape.ClassTag))
		}
		for i, field := range op.Fields {
			fields = append(fields, fmt.Sprintf("%s: %s", goFieldName(uint32(i)), operand(field)))
		}
		assign(fmt.Sprintf("&%s{%s}", goShapeName(canon), strings.Join(fields, ", ")))
	case mir.ObjectAlloc:
		canon := g.canonicalShape(op.Shape)
		if _, ok := g.shapes[canon]; !ok {
			return unsupportedInstruction(fn, block, inst, fmt.Sprintf("missing shape s%d", op.Shape))
		}
		origShape := g.shapes[op.Shape]
		if origShape.ClassTag != 0 {
			assign(fmt.Sprintf("&%s{classTag: %d}", goShapeName(canon), origShape.ClassTag))
		} else {
			assign("&" + goShapeName(canon) + "{}")
		}
	case mir.FieldSet:
		canon := g.canonicalShape(op.Shape)
		field, ok := g.shapeField(canon, op.Field)
		if !ok {
			return unsupportedInstruction(fn, block, inst, "invalid object field")
		}
		fieldName := goFieldName(op.Field)
		if valueHasShapes[op.Object] && valueShapes[op.Object] == canon {
			fmt.Fprintf(out, "\t%s.%s = %s\n", operand(op.Object), fieldName, operand(op.Value))
		} else {
			g.usesFieldHelpers = true
			switch field.Repr {
			case mir.ReprF64, mir.ReprI32, mir.ReprI64:
				fmt.Fprintf(out, "\ttsSetFieldF64(%s, %d, %s)\n", operand(op.Object), op.Field, operand(op.Value))
			case mir.ReprBool:
				fmt.Fprintf(out, "\ttsSetFieldBool(%s, %d, %s)\n", operand(op.Object), op.Field, operand(op.Value))
			case mir.ReprStringRef:
				fmt.Fprintf(out, "\ttsSetFieldString(%s, %d, %s)\n", operand(op.Object), op.Field, operand(op.Value))
			default:
				fmt.Fprintf(out, "\ttsSetFieldAny(%s, %d, %s)\n", operand(op.Object), op.Field, operand(op.Value))
			}
		}
		if inst.Repr != mir.ReprVoid {
			assign(operand(op.Value))
		}
	case mir.FieldGet:
		canon := g.canonicalShape(op.Shape)
		field, ok := g.shapeField(canon, op.Field)
		if !ok {
			return unsupportedInstruction(fn, block, inst, "invalid object field")
		}
		fieldName := goFieldName(op.Field)
		if valueHasShapes[op.Object] && valueShapes[op.Object] == canon {
			assign(fmt.Sprintf("%s.%s", operand(op.Object), fieldName))
		} else {
			g.usesFieldHelpers = true
			switch field.Repr {
			case mir.ReprF64, mir.ReprI32, mir.ReprI64:
				assign(fmt.Sprintf("tsGetFieldF64(%s, %d)", operand(op.Object), op.Field))
			case mir.ReprBool:
				assign(fmt.Sprintf("tsGetFieldBool(%s, %d)", operand(op.Object), op.Field))
			case mir.ReprStringRef:
				assign(fmt.Sprintf("tsGetFieldString(%s, %d)", operand(op.Object), op.Field))
			default:
				targetType, _ := g.goTypeForValue(field.Repr, field.ObjectShape, field.HasObjectShape)
				if targetType != "" && targetType != "any" {
					assign(fmt.Sprintf("tsGetFieldAny(%s, %d).(%s)", operand(op.Object), op.Field, targetType))
				} else {
					assign(fmt.Sprintf("tsGetFieldAny(%s, %d)", operand(op.Object), op.Field))
				}
			}
		}
	case mir.Call:
		callee, ok := g.functions[op.Callee]
		if !ok {
			return unsupportedInstruction(fn, block, inst, fmt.Sprintf("missing callee f%d", op.Callee))
		}
		if len(callee.Params) != len(op.Args) {
			return unsupportedInstruction(fn, block, inst, "call arity mismatch")
		}
		args := make([]string, len(op.Args))
		for i, arg := range op.Args {
			args[i] = operand(arg)
		}
		call := fmt.Sprintf("%s(%s)", goFunctionName(op.Callee), strings.Join(args, ", "))
		if inst.Repr == mir.ReprVoid {
			fmt.Fprintf(out, "\t%s\n", call)
		} else {
			assign(call)
		}
	case mir.IntrinsicCall:
		if len(op.Args) != 1 {
			return unsupportedInstruction(fn, block, inst, "intrinsic arity")
		}
		switch op.Intrinsic {
		case mir.IntrinsicConsoleLogF64, mir.IntrinsicConsoleLogString:
			g.usesFmt = true
			fmt.Fprintf(out, "\tfmt.Println(%s)\n", operand(op.Args[0]))
		case mir.IntrinsicConsoleLogJSValue:
			g.usesFmt = true
			g.usesPrintValue = true
			fmt.Fprintf(out, "\ttsPrintValue(%s)\n", operand(op.Args[0]))
		default:
			return unsupportedInstruction(fn, block, inst, "intrinsic")
		}
	case mir.ArrayNewBool:
		elements := make([]string, len(op.Elements))
		for i, element := range op.Elements {
			elements[i] = operand(element)
		}
		assign("[]bool{" + strings.Join(elements, ", ") + "}")
	case mir.ArrayLengthBool:
		assign("float64(len(" + operand(op.Array) + ".([]bool)))")
	case mir.ArrayGetBool:
		assign(fmt.Sprintf("%s.([]bool)[int(%s)]", operand(op.Array), operand(op.Index)))
	case mir.ArraySetBool:
		fmt.Fprintf(out, "\t%s.([]bool)[int(%s)] = %s\n", operand(op.Array), operand(op.Index), operand(op.Value))
		assign(operand(op.Value))
	case mir.ArrayNewRef:
		elements := make([]string, len(op.Elements))
		for i, element := range op.Elements {
			elements[i] = operand(element)
		}
		assign("[]any{" + strings.Join(elements, ", ") + "}")
	case mir.ArrayLengthRef:
		assign("float64(len(" + operand(op.Array) + ".([]any)))")
	case mir.ArrayGetRef:
		targetType, _ := g.goTypeForValue(inst.Repr, valueShapes[inst.Result], valueHasShapes[inst.Result])
		if targetType != "" && targetType != "any" {
			assign(fmt.Sprintf("%s.([]any)[int(%s)].(%s)", operand(op.Array), operand(op.Index), targetType))
		} else {
			assign(fmt.Sprintf("%s.([]any)[int(%s)]", operand(op.Array), operand(op.Index)))
		}
	case mir.ArraySetRef:
		fmt.Fprintf(out, "\t%s.([]any)[int(%s)] = %s\n", operand(op.Array), operand(op.Index), operand(op.Value))
		assign(operand(op.Value))
	case mir.ConstJSValue:
		assign("nil")
	case mir.BoxJSValue:
		assign(operand(op.Value))
	case mir.UnboxJSValue:
		switch op.Kind {
		case mir.UnboxJSNumber:
			g.usesJSConvert = true
			assign(fmt.Sprintf("tsToF64(%s)", operand(op.Value)))
		case mir.UnboxJSString:
			g.usesJSConvert = true
			assign(fmt.Sprintf("tsToString(%s)", operand(op.Value)))
		case mir.UnboxJSBoolean:
			g.usesJSConvert = true
			assign(fmt.Sprintf("tsToBool(%s)", operand(op.Value)))
		default:
			assign(operand(op.Value))
		}
	case mir.DynamicAddJSValue:
		g.usesJSConvert = true
		g.usesJSAdd = true
		assign(fmt.Sprintf("tsDynamicAdd(%s, %s)", operand(op.Left), operand(op.Right)))
	case mir.DynamicBinaryJSValue:
		g.usesJSConvert = true
		switch op.Operator {
		case mir.DynamicJSSub:
			assign(fmt.Sprintf("tsToF64(%s) - tsToF64(%s)", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSMul:
			assign(fmt.Sprintf("tsToF64(%s) * tsToF64(%s)", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSDiv:
			assign(fmt.Sprintf("tsToF64(%s) / tsToF64(%s)", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSLessThan:
			assign(fmt.Sprintf("tsToF64(%s) < tsToF64(%s)", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSLessEqual:
			assign(fmt.Sprintf("tsToF64(%s) <= tsToF64(%s)", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSGreaterThan:
			assign(fmt.Sprintf("tsToF64(%s) > tsToF64(%s)", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSGreaterEqual:
			assign(fmt.Sprintf("tsToF64(%s) >= tsToF64(%s)", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSEqual, mir.DynamicJSStrictEqual:
			assign(fmt.Sprintf("%s == %s", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSNotEqual, mir.DynamicJSStrictNotEqual:
			assign(fmt.Sprintf("%s != %s", operand(op.Left), operand(op.Right)))
		default:
			return unsupportedInstruction(fn, block, inst, "dynamic binary operator")
		}
	case mir.ClosureNew:
		callee, ok := g.functions[op.Callee]
		if !ok {
			return unsupportedInstruction(fn, block, inst, fmt.Sprintf("missing callee f%d", op.Callee))
		}
		captureCount := len(op.Captures)
		callParams := make([]string, 0, len(callee.Params))
		for i := 0; i < captureCount; i++ {
			callParams = append(callParams, operand(op.Captures[i]))
		}
		argNames := make([]string, 0, len(callee.Params)-captureCount)
		for i := captureCount; i < len(callee.Params); i++ {
			argName := fmt.Sprintf("a%d", i-captureCount)
			argType, _ := g.goTypeForValue(callee.Params[i].Repr, callee.Params[i].ObjectShape, callee.Params[i].HasObjectShape)
			argNames = append(argNames, fmt.Sprintf("%s %s", argName, argType))
			callParams = append(callParams, argName)
		}
		retType, _ := g.goTypeForValue(callee.ReturnRepr, callee.ReturnObjectShape, callee.HasReturnObjectShape)
		closureSig := fmt.Sprintf("func(%s)", strings.Join(argNames, ", "))
		if retType != "" {
			closureSig += " " + retType
		}
		retPrefix := ""
		if retType != "" {
			retPrefix = "return "
		}
		assign(fmt.Sprintf("%s {\n\t\t%s%s(%s)\n\t}", closureSig, retPrefix, goFunctionName(op.Callee), strings.Join(callParams, ", ")))
	case mir.ClosureCall:
		args := make([]string, len(op.Args))
		for i, arg := range op.Args {
			args[i] = operand(arg)
		}
		argTypes := make([]string, len(op.Args))
		for i, arg := range op.Args {
			argRepr := valueTypes[arg]
			t, _ := g.goType(argRepr)
			argTypes[i] = t
		}
		retType, _ := g.goType(inst.Repr)
		sig := fmt.Sprintf("func(%s)", strings.Join(argTypes, ", "))
		if retType != "" {
			sig += " " + retType
		}
		callStr := fmt.Sprintf("%s.(%s)(%s)", operand(op.Closure), sig, strings.Join(args, ", "))
		if inst.Repr == mir.ReprVoid {
			fmt.Fprintf(out, "\t%s\n", callStr)
		} else {
			assign(callStr)
		}
	case mir.TaskSpawn:
		g.usesTasks = true
		callee, ok := g.functions[op.Callee]
		if !ok {
			return unsupportedInstruction(fn, block, inst, fmt.Sprintf("missing callee f%d", op.Callee))
		}
		args := make([]string, len(op.Captures))
		for i, cap := range op.Captures {
			args[i] = operand(cap)
		}
		if op.Group != nil {
			fmt.Fprintf(out, "\t%s.wg.Add(1)\n", operand(*op.Group))
		}
		fmt.Fprintf(out, "\ttsAllTasks.Add(1)\n")
		assign("&tsTask{done: make(chan struct{})}")
		if op.Group != nil {
			fmt.Fprintf(out, "\tgo func(t *tsTask, grp *tsTaskGroup) {\n\t\tdefer tsAllTasks.Done()\n\t\tdefer grp.wg.Done()\n\t\tdefer close(t.done)\n")
		} else {
			fmt.Fprintf(out, "\tgo func(t *tsTask) {\n\t\tdefer tsAllTasks.Done()\n\t\tdefer close(t.done)\n")
		}
		fmt.Fprintf(out, "\t\tdefer func() {\n\t\t\tif r := recover(); r != nil {\n\t\t\t\tif ex, ok := r.(tsException); ok {\n\t\t\t\t\tt.err = ex.val\n\t\t\t\t} else {\n\t\t\t\t\tt.err = r\n\t\t\t\t}\n\t\t\t}\n\t\t}()\n")
		if callee.ReturnRepr != mir.ReprVoid {
			if op.Group != nil {
				fmt.Fprintf(out, "\t\tt.val = %s(%s)\n\t}(%s, %s)\n", goFunctionName(op.Callee), strings.Join(args, ", "), result, operand(*op.Group))
			} else {
				fmt.Fprintf(out, "\t\tt.val = %s(%s)\n\t}(%s)\n", goFunctionName(op.Callee), strings.Join(args, ", "), result)
			}
		} else {
			if op.Group != nil {
				fmt.Fprintf(out, "\t\t%s(%s)\n\t}(%s, %s)\n", goFunctionName(op.Callee), strings.Join(args, ", "), result, operand(*op.Group))
			} else {
				fmt.Fprintf(out, "\t\t%s(%s)\n\t}(%s)\n", goFunctionName(op.Callee), strings.Join(args, ", "), result)
			}
		}
	case mir.TaskJoin:
		fmt.Fprintf(out, "\t<-%s.done\n", operand(op.Task))
		if inst.Repr != mir.ReprVoid {
			targetType, _ := g.goTypeForValue(inst.Repr, valueShapes[inst.Result], valueHasShapes[inst.Result])
			if targetType != "" && targetType != "any" {
				fmt.Fprintf(out, "\tif %s.val != nil { %s = %s.val.(%s) }\n", operand(op.Task), result, operand(op.Task), targetType)
			} else {
				assign(fmt.Sprintf("%s.val", operand(op.Task)))
			}
		}
	case mir.TaskWait:
		fmt.Fprintf(out, "\t<-%s.done\n", operand(op.Task))
		if inst.Repr == mir.ReprBool {
			assign(fmt.Sprintf("%s.err == nil", operand(op.Task)))
		}
	case mir.TaskFailure:
		assign(fmt.Sprintf("%s.err", operand(op.Task)))
	case mir.TaskRetain:
		fmt.Fprintf(out, "\t_ = %s\n", operand(op.Task))
	case mir.TaskRelease:
		fmt.Fprintf(out, "\t_ = %s\n", operand(op.Task))
	case mir.TaskYield:
		g.usesRuntime = true
		fmt.Fprintf(out, "\truntime.Gosched()\n")
	case mir.TaskCancel:
		fmt.Fprintf(out, "\t_ = %s\n", operand(op.Task))
	case mir.TaskCancelled:
		assign("false")
	case mir.TaskGroupNew:
		g.usesSync = true
		g.usesTaskGroups = true
		assign("&tsTaskGroup{}")
	case mir.TaskGroupJoin:
		fmt.Fprintf(out, "\t%s.wg.Wait()\n", operand(op.Group))
	case mir.TaskGroupCancel:
		fmt.Fprintf(out, "\t%s.mu.Lock()\n\t%s.cancelled = true\n\t%s.mu.Unlock()\n",
			operand(op.Group), operand(op.Group), operand(op.Group))
	case mir.TaskContextSet:
		fmt.Fprintf(out, "\t_ = %s\n", operand(op.Value))
	case mir.TaskContextGet:
		assign("nil")
	case mir.PromiseResolve:
		g.usesTasks = true
		assign(fmt.Sprintf("&tsTask{val: %s, done: tsDoneChan()}", operand(op.Value)))
	case mir.PromiseReject:
		g.usesTasks = true
		assign(fmt.Sprintf("&tsTask{err: %s, done: tsDoneChan()}", operand(op.Reason)))
	case mir.PromiseAdopt:
		assign(operand(op.Promise))
	case mir.PromiseThenable:
		g.usesTasks = true
		g.usesSync = true
		taskVar := result
		assign("&tsTask{done: make(chan struct{})}")
		onceVar := fmt.Sprintf("once_%s", taskVar)
		fmt.Fprintf(out, "\tvar %s sync.Once\n", onceVar)

		resolveName := fmt.Sprintf("resolve_%s", taskVar)
		rejectName := fmt.Sprintf("reject_%s", taskVar)

		var resParamType string
		switch op.Result {
		case mir.ReprF64:
			resParamType = "float64"
		case mir.ReprBool:
			resParamType = "bool"
		default:
			resParamType = "any"
		}

		if op.ResolveReturnsJS {
			fmt.Fprintf(out, "\t%s := func(v %s) any { %s.Do(func() { %s.val = v; close(%s.done) }); return nil }\n",
				resolveName, resParamType, onceVar, taskVar, taskVar)
		} else {
			fmt.Fprintf(out, "\t%s := func(v %s) { %s.Do(func() { %s.val = v; close(%s.done) }) }\n",
				resolveName, resParamType, onceVar, taskVar, taskVar)
		}

		if op.RejectReturnsJS {
			fmt.Fprintf(out, "\t%s := func(e any) any { %s.Do(func() { %s.err = e; close(%s.done) }); return nil }\n",
				rejectName, onceVar, taskVar, taskVar)
		} else {
			fmt.Fprintf(out, "\t%s := func(e any) { %s.Do(func() { %s.err = e; close(%s.done) }) }\n",
				rejectName, onceVar, taskVar, taskVar)
		}

		cbArgs := []string{resolveName}
		if op.Arity == 2 {
			cbArgs = append(cbArgs, rejectName)
		}

		if len(op.Cases) != 0 {
			g.usesClassTag = true
			fmt.Fprintf(out, "\tswitch tsGetClassTag(%s) {\n", operand(op.Thenable))
			for _, c := range op.Cases {
				callArgs := append([]string{operand(op.Thenable)}, cbArgs...)
				fmt.Fprintf(out, "\tcase %d:\n\t\t%s(%s)\n", c.ClassTag, goFunctionName(c.Callee), strings.Join(callArgs, ", "))
			}
			if len(op.Cases) > 0 {
				callArgs := append([]string{operand(op.Thenable)}, cbArgs...)
				fmt.Fprintf(out, "\tdefault:\n\t\t%s(%s)\n", goFunctionName(op.Cases[0].Callee), strings.Join(callArgs, ", "))
			}
			fmt.Fprintf(out, "\t}\n")
		} else {
			g.usesDynamicField = true
			g.usesDynamicCall = true
			g.usesReflect = true
			thenFn := fmt.Sprintf("thenFn_%s", taskVar)
			fmt.Fprintf(out, "\t%s := tsDynamicFieldGet(%s, \"then\")\n", thenFn, operand(op.Thenable))
			fmt.Fprintf(out, "\ttsDynamicCall(%s, %s, %s)\n", thenFn, operand(op.Thenable), strings.Join(cbArgs, ", "))
		}
	case mir.PromiseAllF64:
		g.usesTasks = true
		assign("&tsTask{done: make(chan struct{})}")
		var list []string
		for _, v := range op.Promises {
			list = append(list, operand(v))
		}
		fmt.Fprintf(out, "\tgo func(dst *tsTask, promises []*tsTask) {\n\t\tdefer close(dst.done)\n")
		fmt.Fprintf(out, "\t\tvals := make([]float64, len(promises))\n")
		fmt.Fprintf(out, "\t\tfor i, p := range promises {\n\t\t\t<-p.done\n\t\t\tif p.err != nil { dst.err = p.err; return }\n\t\t\tif p.val != nil { vals[i] = p.val.(float64) }\n\t\t}\n")
		fmt.Fprintf(out, "\t\tdst.val = vals\n")
		fmt.Fprintf(out, "\t}(%s, []*tsTask{%s})\n", result, strings.Join(list, ", "))
	case mir.PromiseAllBool:
		g.usesTasks = true
		assign("&tsTask{done: make(chan struct{})}")
		var list []string
		for _, v := range op.Promises {
			list = append(list, operand(v))
		}
		fmt.Fprintf(out, "\tgo func(dst *tsTask, promises []*tsTask) {\n\t\tdefer close(dst.done)\n")
		fmt.Fprintf(out, "\t\tvals := make([]bool, len(promises))\n")
		fmt.Fprintf(out, "\t\tfor i, p := range promises {\n\t\t\t<-p.done\n\t\t\tif p.err != nil { dst.err = p.err; return }\n\t\t\tif p.val != nil { vals[i] = p.val.(bool) }\n\t\t}\n")
		fmt.Fprintf(out, "\t\tdst.val = vals\n")
		fmt.Fprintf(out, "\t}(%s, []*tsTask{%s})\n", result, strings.Join(list, ", "))
	case mir.PromiseAllRef:
		g.usesTasks = true
		assign("&tsTask{done: make(chan struct{})}")
		var list []string
		for _, v := range op.Promises {
			list = append(list, operand(v))
		}
		fmt.Fprintf(out, "\tgo func(dst *tsTask, promises []*tsTask) {\n\t\tdefer close(dst.done)\n")
		fmt.Fprintf(out, "\t\tvals := make([]any, len(promises))\n")
		fmt.Fprintf(out, "\t\tfor i, p := range promises {\n\t\t\t<-p.done\n\t\t\tif p.err != nil { dst.err = p.err; return }\n\t\t\tvals[i] = p.val\n\t\t}\n")
		fmt.Fprintf(out, "\t\tdst.val = vals\n")
		fmt.Fprintf(out, "\t}(%s, []*tsTask{%s})\n", result, strings.Join(list, ", "))
	case mir.PromiseRaceF64, mir.PromiseRaceBool, mir.PromiseRaceRef:
		g.usesTasks = true
		assign("&tsTask{done: make(chan struct{})}")
		var list []string
		switch p := op.(type) {
		case mir.PromiseRaceF64:
			for _, v := range p.Promises {
				list = append(list, operand(v))
			}
		case mir.PromiseRaceBool:
			for _, v := range p.Promises {
				list = append(list, operand(v))
			}
		case mir.PromiseRaceRef:
			for _, v := range p.Promises {
				list = append(list, operand(v))
			}
		}
		fmt.Fprintf(out, "\tgo func(dst *tsTask, promises []*tsTask) {\n\t\tdefer close(dst.done)\n")
		fmt.Fprintf(out, "\t\tif len(promises) == 0 { return }\n")
		fmt.Fprintf(out, "\t\twon := make(chan *tsTask, len(promises))\n")
		fmt.Fprintf(out, "\t\tfor _, p := range promises {\n\t\t\tgo func(t *tsTask) { <-t.done; won <- t }(p)\n\t\t}\n")
		fmt.Fprintf(out, "\t\tfirst := <-won\n\t\tdst.val = first.val\n\t\tdst.err = first.err\n")
		fmt.Fprintf(out, "\t}(%s, []*tsTask{%s})\n", result, strings.Join(list, ", "))
	case mir.Sleep:
		g.usesTime = true
		fmt.Fprintf(out, "\ttime.Sleep(time.Duration(%s * float64(time.Millisecond)))\n", operand(op.Duration))
	case mir.ChannelNewF64:
		assign(fmt.Sprintf("make(chan float64, int(%s))", operand(op.Capacity)))
	case mir.ChannelSendF64:
		fmt.Fprintf(out, "\t%s.(chan float64) <- %s\n", operand(op.Channel), operand(op.Value))
	case mir.ChannelRecvF64:
		assign(fmt.Sprintf("<-%s.(chan float64)", operand(op.Channel)))
	case mir.ChannelTrySendF64:
		fmt.Fprintf(out, "\tselect {\n\tcase %s.(chan float64) <- %s:\n\t\t%s = true\n\tdefault:\n\t\t%s = false\n\t}\n",
			operand(op.Channel), operand(op.Value), result, result)
	case mir.ChannelTryRecvOrF64:
		fmt.Fprintf(out, "\tselect {\n\tcase %s = <-%s.(chan float64):\n\tdefault:\n\t\t%s = %s\n\t}\n",
			result, operand(op.Channel), result, operand(op.Fallback))
	case mir.ChannelNewBool:
		assign(fmt.Sprintf("make(chan bool, int(%s))", operand(op.Capacity)))
	case mir.ChannelSendBool:
		fmt.Fprintf(out, "\t%s.(chan bool) <- %s\n", operand(op.Channel), operand(op.Value))
	case mir.ChannelRecvBool:
		assign(fmt.Sprintf("<-%s.(chan bool)", operand(op.Channel)))
	case mir.ChannelTrySendBool:
		fmt.Fprintf(out, "\tselect {\n\tcase %s.(chan bool) <- %s:\n\t\t%s = true\n\tdefault:\n\t\t%s = false\n\t}\n",
			operand(op.Channel), operand(op.Value), result, result)
	case mir.ChannelTryRecvOrBool:
		fmt.Fprintf(out, "\tselect {\n\tcase %s = <-%s.(chan bool):\n\tdefault:\n\t\t%s = %s\n\t}\n",
			result, operand(op.Channel), result, operand(op.Fallback))
	case mir.ChannelNewRef:
		assign(fmt.Sprintf("make(chan any, int(%s))", operand(op.Capacity)))
	case mir.ChannelSendRef:
		fmt.Fprintf(out, "\t%s.(chan any) <- %s\n", operand(op.Channel), operand(op.Value))
	case mir.ChannelRecvRef:
		targetType, _ := g.goType(inst.Repr)
		if targetType != "" && targetType != "any" {
			assign(fmt.Sprintf("(<-%s.(chan any)).(%s)", operand(op.Channel), targetType))
		} else {
			assign(fmt.Sprintf("<-%s.(chan any)", operand(op.Channel)))
		}
	case mir.ChannelTrySendRef:
		fmt.Fprintf(out, "\tselect {\n\tcase %s.(chan any) <- %s:\n\t\t%s = true\n\tdefault:\n\t\t%s = false\n\t}\n",
			operand(op.Channel), operand(op.Value), result, result)
	case mir.ChannelTryRecvOrRef:
		targetType, _ := g.goType(inst.Repr)
		if targetType != "" && targetType != "any" {
			fmt.Fprintf(out, "\tselect {\n\tcase tmp := <-%s.(chan any):\n\t\t%s = tmp.(%s)\n\tdefault:\n\t\t%s = %s\n\t}\n",
				operand(op.Channel), result, targetType, result, operand(op.Fallback))
		} else {
			fmt.Fprintf(out, "\tselect {\n\tcase %s = <-%s.(chan any):\n\tdefault:\n\t\t%s = %s\n\t}\n",
				result, operand(op.Channel), result, operand(op.Fallback))
		}
	case mir.DynamicFieldGet:
		g.usesDynamicField = true
		assign(fmt.Sprintf("tsDynamicFieldGet(%s, %q)", operand(op.Object), op.Field))
	case mir.DynamicFieldSet:
		g.usesDynamicField = true
		fmt.Fprintf(out, "\ttsDynamicFieldSet(%s, %q, %s)\n", operand(op.Object), op.Field, operand(op.Value))
		if inst.Repr != mir.ReprVoid {
			assign(operand(op.Value))
		}
	case mir.DynamicCall:
		g.usesReflect = true
		g.usesDynamicCall = true
		args := make([]string, 0, len(op.Args)+1)
		if op.HasReceiver {
			args = append(args, operand(op.Receiver))
		}
		for _, arg := range op.Args {
			args = append(args, operand(arg))
		}
		call := fmt.Sprintf("tsDynamicCall(%s, %s)", operand(op.Callee), strings.Join(args, ", "))
		if inst.Repr == mir.ReprVoid {
			fmt.Fprintf(out, "\t%s\n", call)
		} else {
			targetType, _ := g.goType(inst.Repr)
			if targetType != "" && targetType != "any" {
				assign(fmt.Sprintf("%s.(%s)", call, targetType))
			} else {
				assign(call)
			}
		}
	case mir.DynamicMethodCall:
		g.usesClassTag = true
		resName := ""
		if inst.Repr != mir.ReprVoid {
			resName = result + " = "
		}
		receiver := operand(op.Receiver)
		emitMethodCall := func(calleeID mir.FunctionID) string {
			callee := g.functions[calleeID]
			cArgs := []string{receiver}
			for i, a := range op.Args {
				argExpr := operand(a)
				paramIdx := i + 1
				if paramIdx < len(callee.Params) {
					p := callee.Params[paramIdx]
					if (p.Repr == mir.ReprF64 || p.Repr == mir.ReprI32 || p.Repr == mir.ReprI64) && valueTypes[a] != p.Repr {
						g.usesJSConvert = true
						argExpr = fmt.Sprintf("tsToF64(%s)", argExpr)
					} else if p.Repr == mir.ReprBool && valueTypes[a] != mir.ReprBool {
						argExpr = fmt.Sprintf("%s.(bool)", argExpr)
					} else if p.Repr == mir.ReprStringRef && valueTypes[a] != mir.ReprStringRef {
						argExpr = fmt.Sprintf("%s.(string)", argExpr)
					}
				}
				cArgs = append(cArgs, argExpr)
			}
			return fmt.Sprintf("%s(%s)", goFunctionName(calleeID), strings.Join(cArgs, ", "))
		}
		fmt.Fprintf(out, "\tswitch tsGetClassTag(%s) {\n", receiver)
		for _, c := range op.Cases {
			callStr := emitMethodCall(c.Callee)
			fmt.Fprintf(out, "\tcase %d:\n\t\t%s%s\n", c.ClassTag, resName, callStr)
		}
		if len(op.Cases) > 0 {
			callStr := emitMethodCall(op.Cases[0].Callee)
			fmt.Fprintf(out, "\tdefault:\n\t\t%s%s\n", resName, callStr)
		}
		fmt.Fprintf(out, "\t}\n")
	case mir.DispatchCall:
		g.usesClassTag = true
		resName := ""
		if inst.Repr != mir.ReprVoid {
			resName = result + " = "
		}
		receiver := operand(op.Args[0])
		emitDispatchCall := func(calleeID mir.FunctionID) string {
			callee := g.functions[calleeID]
			cArgs := make([]string, len(op.Args))
			for i, a := range op.Args {
				argExpr := operand(a)
				if i < len(callee.Params) {
					p := callee.Params[i]
					if (p.Repr == mir.ReprF64 || p.Repr == mir.ReprI32 || p.Repr == mir.ReprI64) && valueTypes[a] != p.Repr {
						g.usesJSConvert = true
						argExpr = fmt.Sprintf("tsToF64(%s)", argExpr)
					} else if p.Repr == mir.ReprBool && valueTypes[a] != mir.ReprBool {
						argExpr = fmt.Sprintf("%s.(bool)", argExpr)
					} else if p.Repr == mir.ReprStringRef && valueTypes[a] != mir.ReprStringRef {
						argExpr = fmt.Sprintf("%s.(string)", argExpr)
					}
				}
				cArgs[i] = argExpr
			}
			return fmt.Sprintf("%s(%s)", goFunctionName(calleeID), strings.Join(cArgs, ", "))
		}
		fmt.Fprintf(out, "\tswitch tsGetClassTag(%s) {\n", receiver)
		for _, c := range op.Cases {
			callStr := emitDispatchCall(c.Callee)
			fmt.Fprintf(out, "\tcase %d:\n\t\t%s%s\n", c.ClassTag, resName, callStr)
		}
		if len(op.Cases) > 0 {
			callStr := emitDispatchCall(op.Cases[0].Callee)
			fmt.Fprintf(out, "\tdefault:\n\t\t%s%s\n", resName, callStr)
		}
		fmt.Fprintf(out, "\t}\n")
	default:
		return unsupportedInstruction(fn, block, inst, "operation")
	}

	_ = valueTypes
	return nil
}

func (g *generator) shapeField(shapeID mir.ShapeID, field uint32) (mir.ShapeField, bool) {
	shape, ok := g.shapes[shapeID]
	if !ok || field >= uint32(len(shape.Fields)) {
		return mir.ShapeField{}, false
	}
	return shape.Fields[field], true
}

func (g *generator) emitTerminator(out *strings.Builder, fn mir.Function, block mir.BlockID, term mir.Terminator, phis map[mir.BlockID][]phiValue, valueTypes map[mir.ValueID]mir.Repr, valueShapes map[mir.ValueID]mir.ShapeID, valueHasShapes map[mir.ValueID]bool) error {
	emitEdge := func(target mir.BlockID) error {
		for _, phi := range phis[target] {
			value, ok := phi.incoming[block]
			if !ok {
				return fmt.Errorf("function %s block b%d: phi v%d has no incoming value from b%d", fn.Name, block, phi.result, block)
			}
			phiType, _ := g.goTypeForValue(valueTypes[phi.result], valueShapes[phi.result], valueHasShapes[phi.result])
			valType, _ := g.goTypeForValue(valueTypes[value], valueShapes[value], valueHasShapes[value])
			if phiType != "" && phiType != "any" && valType == "any" {
				fmt.Fprintf(out, "\t%s = %s.(%s)\n", goValueName(phi.result), goValueName(value), phiType)
			} else {
				fmt.Fprintf(out, "\t%s = %s\n", goValueName(phi.result), goValueName(value))
			}
		}
		fmt.Fprintf(out, "\tgoto block%d\n", target)
		return nil
	}

	switch term := term.(type) {
	case mir.Return:
		if term.Value == nil {
			out.WriteString("\treturn\n")
		} else {
			retVal := goValueName(*term.Value)
			retType, _ := g.goTypeForValue(fn.ReturnRepr, g.canonicalShape(fn.ReturnObjectShape), fn.HasReturnObjectShape)
			valType, _ := g.goTypeForValue(valueTypes[*term.Value], valueShapes[*term.Value], valueHasShapes[*term.Value])
			if retType != "" && retType != "any" && valType == "any" {
				fmt.Fprintf(out, "\treturn %s.(%s)\n", retVal, retType)
			} else {
				fmt.Fprintf(out, "\treturn %s\n", retVal)
			}
		}
	case mir.Jump:
		if err := emitEdge(term.Target); err != nil {
			return err
		}
	case mir.Branch:
		fmt.Fprintf(out, "\tif %s {\n", goValueName(term.Condition))
		if err := emitEdge(term.Then); err != nil {
			return err
		}
		out.WriteString("\t} else {\n")
		if err := emitEdge(term.Else); err != nil {
			return err
		}
		out.WriteString("\t}\n")
	case mir.Throw:
		g.usesExceptions = true
		fmt.Fprintf(out, "\tpanic(tsException{val: %s})\n", goValueName(term.Value))
	default:
		return unsupportedTerminator(fn, block, term, "terminator")
	}
	return nil
}

type phiValue struct {
	result   mir.ValueID
	incoming map[mir.BlockID]mir.ValueID
}

func functionPhis(blocks []mir.Block) map[mir.BlockID][]phiValue {
	result := make(map[mir.BlockID][]phiValue)
	for _, block := range blocks {
		for _, inst := range block.Instructions {
			phi, ok := inst.Op.(mir.Phi)
			if !ok {
				continue
			}
			incoming := make(map[mir.BlockID]mir.ValueID, len(phi.Incoming))
			for _, item := range phi.Incoming {
				incoming[item.Block] = item.Value
			}
			result[block.ID] = append(result[block.ID], phiValue{result: inst.Result, incoming: incoming})
		}
	}
	return result
}

func functionTargets(blocks []mir.Block) map[mir.BlockID]bool {
	targets := make(map[mir.BlockID]bool)
	for _, block := range blocks {
		switch term := block.Terminator.(type) {
		case mir.Jump:
			targets[term.Target] = true
		case mir.Branch:
			targets[term.Then] = true
			targets[term.Else] = true
		}
	}
	return targets
}

func addValueType(values map[mir.ValueID]mir.Repr, value mir.ValueID, repr mir.Repr) error {
	if previous, ok := values[value]; ok && previous != repr {
		return fmt.Errorf("value v%d has conflicting representations %d and %d", value, previous, repr)
	}
	values[value] = repr
	return nil
}

func goFunctionName(id mir.FunctionID) string { return fmt.Sprintf("tsFunction%d", id) }

func goValueName(id mir.ValueID) string { return fmt.Sprintf("v%d", id) }

func goShapeName(id mir.ShapeID) string { return fmt.Sprintf("tsShape%d", id) }

func goFieldName(id uint32) string { return fmt.Sprintf("Field%d", id) }

func (g *generator) goTypeForValue(repr mir.Repr, shapeID mir.ShapeID, hasShape bool) (string, error) {
	if repr == mir.ReprObjectRef && hasShape {
		return "*" + goShapeName(g.canonicalShape(shapeID)), nil
	}
	return g.goType(repr)
}

func (g *generator) goType(repr mir.Repr) (string, error) {
	switch repr {
	case mir.ReprVoid:
		return "", nil
	case mir.ReprBool:
		return "bool", nil
	case mir.ReprI32:
		return "int32", nil
	case mir.ReprI64:
		return "int64", nil
	case mir.ReprF64:
		return "float64", nil
	case mir.ReprStringRef:
		return "string", nil
	case mir.ReprArrayRef:
		return "any", nil
	case mir.ReprObjectRef:
		return "any", nil
	case mir.ReprFunctionRef:
		return "any", nil
	case mir.ReprTaskRef:
		g.usesTasks = true
		return "*tsTask", nil
	case mir.ReprChannelRef:
		return "any", nil
	case mir.ReprTaskGroupRef:
		g.usesSync = true
		g.usesTaskGroups = true
		return "*tsTaskGroup", nil
	case mir.ReprTagged, mir.ReprJSValue:
		return "any", nil
	default:
		return "", fmt.Errorf("representation %d is not supported by the pure-Go backend", repr)
	}
}

func zeroValue(repr mir.Repr) string {
	switch repr {
	case mir.ReprBool:
		return "false"
	case mir.ReprI32:
		return "0"
	case mir.ReprI64:
		return "0"
	case mir.ReprF64:
		return "0"
	case mir.ReprStringRef:
		return `""`
	case mir.ReprArrayRef, mir.ReprObjectRef, mir.ReprFunctionRef, mir.ReprTaskRef,
		mir.ReprChannelRef, mir.ReprTaskGroupRef, mir.ReprTagged, mir.ReprJSValue:
		return "nil"
	default:
		return "0"
	}
}

func (g *generator) floatLiteral(value float64) string {
	switch {
	case math.IsNaN(value):
		g.usesMath = true
		return "math.NaN()"
	case math.IsInf(value, 1):
		g.usesMath = true
		return "math.Inf(1)"
	case math.IsInf(value, -1):
		g.usesMath = true
		return "math.Inf(-1)"
	default:
		return strconv.FormatFloat(value, 'g', -1, 64)
	}
}

func floatOperator(operator mir.FloatBinaryOp) (string, bool) {
	switch operator {
	case mir.FloatAdd:
		return "+", true
	case mir.FloatSub:
		return "-", true
	case mir.FloatMul:
		return "*", true
	case mir.FloatDiv:
		return "/", true
	default:
		return "", false
	}
}

func intOperator(operator mir.FloatBinaryOp) (string, bool) {
	switch operator {
	case mir.FloatAdd:
		return "+", true
	case mir.FloatSub:
		return "-", true
	case mir.FloatMul:
		return "*", true
	case mir.FloatDiv:
		return "/", true
	default:
		return "", false
	}
}

func compareOperator(operator mir.FloatCompareOp) (string, bool) {
	switch operator {
	case mir.FloatLessThan:
		return "<", true
	case mir.FloatLessEqual:
		return "<=", true
	case mir.FloatGreaterThan:
		return ">", true
	case mir.FloatGreaterEqual:
		return ">=", true
	case mir.FloatEqual:
		return "==", true
	case mir.FloatNotEqual:
		return "!=", true
	default:
		return "", false
	}
}

func unsupportedInstruction(fn mir.Function, block mir.BlockID, inst mir.Instruction, reason string) error {
	return fmt.Errorf("function %s block b%d instruction v%d (%T) is unsupported by pure-Go backend: %s", fn.Name, block, inst.Result, inst.Op, reason)
}

func unsupportedTerminator(fn mir.Function, block mir.BlockID, term mir.Terminator, reason string) error {
	return fmt.Errorf("function %s block b%d terminator (%T) is unsupported by pure-Go backend: %s", fn.Name, block, term, reason)
}

func (g *generator) emitClassTagHelper(source *strings.Builder) {
	emitted := make(map[mir.ShapeID]bool)
	shapes := append([]mir.Shape(nil), g.module.Shapes...)
	sort.Slice(shapes, func(i, j int) bool { return shapes[i].ID < shapes[j].ID })

	source.WriteString("func tsGetClassTag(obj any) uint32 {\n\tif obj == nil {\n\t\treturn 0\n\t}\n\tswitch o := obj.(type) {\n")
	for _, shape := range shapes {
		canon := g.canonicalShape(shape.ID)
		if emitted[canon] {
			continue
		}
		emitted[canon] = true
		origShape := g.shapes[shape.ID]
		if origShape.ClassTag != 0 {
			fmt.Fprintf(source, "\tcase *%s:\n\t\treturn o.classTag\n", goShapeName(canon))
		}
	}
	source.WriteString("\t}\n\treturn 0\n}\n\n")
}

func (g *generator) emitDynamicFieldHelpers(source *strings.Builder) {
	emitted := make(map[mir.ShapeID]bool)
	shapes := append([]mir.Shape(nil), g.module.Shapes...)
	sort.Slice(shapes, func(i, j int) bool { return shapes[i].ID < shapes[j].ID })

	source.WriteString("func tsDynamicFieldGet(obj any, field string) any {\n\tif obj == nil {\n\t\treturn nil\n\t}\n\tswitch o := obj.(type) {\n")
	for _, shape := range shapes {
		canon := g.canonicalShape(shape.ID)
		if emitted[canon] {
			continue
		}
		emitted[canon] = true
		if len(shape.Fields) == 0 {
			continue
		}
		fmt.Fprintf(source, "\tcase *%s:\n\t\tswitch field {\n", goShapeName(canon))
		for index, f := range shape.Fields {
			fmt.Fprintf(source, "\t\tcase %q:\n\t\t\treturn o.%s\n", f.Name, goFieldName(uint32(index)))
		}
		source.WriteString("\t\t}\n")
	}
	source.WriteString("\t}\n\treturn nil\n}\n\n")

	emitted = make(map[mir.ShapeID]bool)
	source.WriteString("func tsDynamicFieldSet(obj any, field string, val any) {\n\tif obj == nil {\n\t\treturn\n\t}\n\tswitch o := obj.(type) {\n")
	for _, shape := range shapes {
		canon := g.canonicalShape(shape.ID)
		if emitted[canon] {
			continue
		}
		emitted[canon] = true
		if len(shape.Fields) == 0 {
			continue
		}
		fmt.Fprintf(source, "\tcase *%s:\n\t\tswitch field {\n", goShapeName(canon))
		for index, f := range shape.Fields {
			fname := goFieldName(uint32(index))
			switch f.Repr {
			case mir.ReprF64, mir.ReprI32, mir.ReprI64:
				fmt.Fprintf(source, "\t\tcase %q:\n\t\t\to.%s = tsToF64(val)\n", f.Name, fname)
			case mir.ReprBool:
				fmt.Fprintf(source, "\t\tcase %q:\n\t\t\to.%s = val.(bool)\n", f.Name, fname)
			case mir.ReprStringRef:
				fmt.Fprintf(source, "\t\tcase %q:\n\t\t\to.%s = val.(string)\n", f.Name, fname)
			case mir.ReprObjectRef:
				fieldCanon := g.canonicalShape(f.ObjectShape)
				fmt.Fprintf(source, "\t\tcase %q:\n\t\t\tif v, ok := val.(*%s); ok { o.%s = v } else if v, ok := val.(any); ok && v != nil { o.%s = v.(*%s) }\n", f.Name, goShapeName(fieldCanon), fname, fname, goShapeName(fieldCanon))
			default:
				fmt.Fprintf(source, "\t\tcase %q:\n\t\t\to.%s = val\n", f.Name, fname)
			}
		}
		source.WriteString("\t\t}\n")
	}
	source.WriteString("\t}\n}\n\n")
}

func (g *generator) emitFieldHelpers(source *strings.Builder) {
	emitted := make(map[mir.ShapeID]bool)
	shapes := append([]mir.Shape(nil), g.module.Shapes...)
	sort.Slice(shapes, func(i, j int) bool { return shapes[i].ID < shapes[j].ID })

	hasF64 := false
	for _, shape := range shapes {
		for _, f := range shape.Fields {
			if f.Repr == mir.ReprF64 || f.Repr == mir.ReprI32 || f.Repr == mir.ReprI64 {
				hasF64 = true
				break
			}
		}
	}
	if !hasF64 {
		source.WriteString("func tsGetFieldF64(obj any, fieldIndex int) float64 { return 0 }\nfunc tsSetFieldF64(obj any, fieldIndex int, val float64) {}\n\n")
	} else {
		source.WriteString("func tsGetFieldF64(obj any, fieldIndex int) float64 {\n\tif obj == nil { return 0 }\n\tswitch o := obj.(type) {\n")
		for _, shape := range shapes {
			canon := g.canonicalShape(shape.ID)
			if emitted[canon] { continue }
			emitted[canon] = true
			hasFields := false
			for _, f := range shape.Fields {
				if f.Repr == mir.ReprF64 || f.Repr == mir.ReprI32 || f.Repr == mir.ReprI64 {
					hasFields = true
					break
				}
			}
			if !hasFields { continue }
			fmt.Fprintf(source, "\tcase *%s:\n\t\tswitch fieldIndex {\n", goShapeName(canon))
			for index, f := range shape.Fields {
				if f.Repr == mir.ReprF64 || f.Repr == mir.ReprI32 || f.Repr == mir.ReprI64 {
					fmt.Fprintf(source, "\t\tcase %d: return float64(o.%s)\n", index, goFieldName(uint32(index)))
				}
			}
			source.WriteString("\t\t}\n")
		}
		source.WriteString("\t}\n\treturn 0\n}\n\n")

		emitted = make(map[mir.ShapeID]bool)
		source.WriteString("func tsSetFieldF64(obj any, fieldIndex int, val float64) {\n\tif obj == nil { return }\n\tswitch o := obj.(type) {\n")
		for _, shape := range shapes {
			canon := g.canonicalShape(shape.ID)
			if emitted[canon] { continue }
			emitted[canon] = true
			hasFields := false
			for _, f := range shape.Fields {
				if f.Repr == mir.ReprF64 || f.Repr == mir.ReprI32 || f.Repr == mir.ReprI64 {
					hasFields = true
					break
				}
			}
			if !hasFields { continue }
			fmt.Fprintf(source, "\tcase *%s:\n\t\tswitch fieldIndex {\n", goShapeName(canon))
			for index, f := range shape.Fields {
				if f.Repr == mir.ReprF64 || f.Repr == mir.ReprI32 || f.Repr == mir.ReprI64 {
					fmt.Fprintf(source, "\t\tcase %d: o.%s = val\n", index, goFieldName(uint32(index)))
				}
			}
			source.WriteString("\t\t}\n")
		}
		source.WriteString("\t}\n}\n\n")
	}

	emitted = make(map[mir.ShapeID]bool)
	hasBool := false
	for _, shape := range shapes {
		for _, f := range shape.Fields {
			if f.Repr == mir.ReprBool {
				hasBool = true
				break
			}
		}
	}
	if !hasBool {
		source.WriteString("func tsGetFieldBool(obj any, fieldIndex int) bool { return false }\nfunc tsSetFieldBool(obj any, fieldIndex int, val bool) {}\n\n")
	} else {
		source.WriteString("func tsGetFieldBool(obj any, fieldIndex int) bool {\n\tif obj == nil { return false }\n\tswitch o := obj.(type) {\n")
		for _, shape := range shapes {
			canon := g.canonicalShape(shape.ID)
			if emitted[canon] { continue }
			emitted[canon] = true
			hasFields := false
			for _, f := range shape.Fields {
				if f.Repr == mir.ReprBool {
					hasFields = true
					break
				}
			}
			if !hasFields { continue }
			fmt.Fprintf(source, "\tcase *%s:\n\t\tswitch fieldIndex {\n", goShapeName(canon))
			for index, f := range shape.Fields {
				if f.Repr == mir.ReprBool {
					fmt.Fprintf(source, "\t\tcase %d: return o.%s\n", index, goFieldName(uint32(index)))
				}
			}
			source.WriteString("\t\t}\n")
		}
		source.WriteString("\t}\n\treturn false\n}\n\n")

		emitted = make(map[mir.ShapeID]bool)
		source.WriteString("func tsSetFieldBool(obj any, fieldIndex int, val bool) {\n\tif obj == nil { return }\n\tswitch o := obj.(type) {\n")
		for _, shape := range shapes {
			canon := g.canonicalShape(shape.ID)
			if emitted[canon] { continue }
			emitted[canon] = true
			hasFields := false
			for _, f := range shape.Fields {
				if f.Repr == mir.ReprBool {
					hasFields = true
					break
				}
			}
			if !hasFields { continue }
			fmt.Fprintf(source, "\tcase *%s:\n\t\tswitch fieldIndex {\n", goShapeName(canon))
			for index, f := range shape.Fields {
				if f.Repr == mir.ReprBool {
					fmt.Fprintf(source, "\t\tcase %d: o.%s = val\n", index, goFieldName(uint32(index)))
				}
			}
			source.WriteString("\t\t}\n")
		}
		source.WriteString("\t}\n}\n\n")
	}

	emitted = make(map[mir.ShapeID]bool)
	hasString := false
	for _, shape := range shapes {
		for _, f := range shape.Fields {
			if f.Repr == mir.ReprStringRef {
				hasString = true
				break
			}
		}
	}
	if !hasString {
		source.WriteString("func tsGetFieldString(obj any, fieldIndex int) string { return \"\" }\nfunc tsSetFieldString(obj any, fieldIndex int, val string) {}\n\n")
	} else {
		source.WriteString("func tsGetFieldString(obj any, fieldIndex int) string {\n\tif obj == nil { return \"\" }\n\tswitch o := obj.(type) {\n")
		for _, shape := range shapes {
			canon := g.canonicalShape(shape.ID)
			if emitted[canon] { continue }
			emitted[canon] = true
			hasFields := false
			for _, f := range shape.Fields {
				if f.Repr == mir.ReprStringRef {
					hasFields = true
					break
				}
			}
			if !hasFields { continue }
			fmt.Fprintf(source, "\tcase *%s:\n\t\tswitch fieldIndex {\n", goShapeName(canon))
			for index, f := range shape.Fields {
				if f.Repr == mir.ReprStringRef {
					fmt.Fprintf(source, "\t\tcase %d: return o.%s\n", index, goFieldName(uint32(index)))
				}
			}
			source.WriteString("\t\t}\n")
		}
		source.WriteString("\t}\n\treturn \"\"\n}\n\n")

		emitted = make(map[mir.ShapeID]bool)
		source.WriteString("func tsSetFieldString(obj any, fieldIndex int, val string) {\n\tif obj == nil { return }\n\tswitch o := obj.(type) {\n")
		for _, shape := range shapes {
			canon := g.canonicalShape(shape.ID)
			if emitted[canon] { continue }
			emitted[canon] = true
			hasFields := false
			for _, f := range shape.Fields {
				if f.Repr == mir.ReprStringRef {
					hasFields = true
					break
				}
			}
			if !hasFields { continue }
			fmt.Fprintf(source, "\tcase *%s:\n\t\tswitch fieldIndex {\n", goShapeName(canon))
			for index, f := range shape.Fields {
				if f.Repr == mir.ReprStringRef {
					fmt.Fprintf(source, "\t\tcase %d: o.%s = val\n", index, goFieldName(uint32(index)))
				}
			}
			source.WriteString("\t\t}\n")
		}
		source.WriteString("\t}\n}\n\n")
	}

	emitted = make(map[mir.ShapeID]bool)
	hasAny := false
	for _, shape := range shapes {
		for _, f := range shape.Fields {
			if f.Repr != mir.ReprF64 && f.Repr != mir.ReprI32 && f.Repr != mir.ReprI64 && f.Repr != mir.ReprBool && f.Repr != mir.ReprStringRef {
				hasAny = true
				break
			}
		}
	}
	if !hasAny {
		source.WriteString("func tsGetFieldAny(obj any, fieldIndex int) any { return nil }\nfunc tsSetFieldAny(obj any, fieldIndex int, val any) {}\n\n")
	} else {
		source.WriteString("func tsGetFieldAny(obj any, fieldIndex int) any {\n\tif obj == nil { return nil }\n\tswitch o := obj.(type) {\n")
		for _, shape := range shapes {
			canon := g.canonicalShape(shape.ID)
			if emitted[canon] { continue }
			emitted[canon] = true
			hasFields := false
			for _, f := range shape.Fields {
				if f.Repr != mir.ReprF64 && f.Repr != mir.ReprI32 && f.Repr != mir.ReprI64 && f.Repr != mir.ReprBool && f.Repr != mir.ReprStringRef {
					hasFields = true
					break
				}
			}
			if !hasFields { continue }
			fmt.Fprintf(source, "\tcase *%s:\n\t\tswitch fieldIndex {\n", goShapeName(canon))
			for index, f := range shape.Fields {
				if f.Repr != mir.ReprF64 && f.Repr != mir.ReprI32 && f.Repr != mir.ReprI64 && f.Repr != mir.ReprBool && f.Repr != mir.ReprStringRef {
					fmt.Fprintf(source, "\t\tcase %d: return o.%s\n", index, goFieldName(uint32(index)))
				}
			}
			source.WriteString("\t\t}\n")
		}
		source.WriteString("\t}\n\treturn nil\n}\n\n")

		emitted = make(map[mir.ShapeID]bool)
		source.WriteString("func tsSetFieldAny(obj any, fieldIndex int, val any) {\n\tif obj == nil { return }\n\tswitch o := obj.(type) {\n")
		for _, shape := range shapes {
			canon := g.canonicalShape(shape.ID)
			if emitted[canon] { continue }
			emitted[canon] = true
			hasFields := false
			for _, f := range shape.Fields {
				if f.Repr != mir.ReprF64 && f.Repr != mir.ReprI32 && f.Repr != mir.ReprI64 && f.Repr != mir.ReprBool && f.Repr != mir.ReprStringRef {
					hasFields = true
					break
				}
			}
			if !hasFields { continue }
			fmt.Fprintf(source, "\tcase *%s:\n\t\tswitch fieldIndex {\n", goShapeName(canon))
			for index, f := range shape.Fields {
				if f.Repr != mir.ReprF64 && f.Repr != mir.ReprI32 && f.Repr != mir.ReprI64 && f.Repr != mir.ReprBool && f.Repr != mir.ReprStringRef {
					if f.Repr == mir.ReprObjectRef && f.HasObjectShape {
						fieldCanon := g.canonicalShape(f.ObjectShape)
						fmt.Fprintf(source, "\t\tcase %d: if v, ok := val.(*%s); ok { o.%s = v } else if v, ok := val.(any); ok && v != nil { o.%s = v.(*%s) }\n", index, goShapeName(fieldCanon), goFieldName(uint32(index)), goFieldName(uint32(index)), goShapeName(fieldCanon))
					} else {
						fmt.Fprintf(source, "\t\tcase %d: o.%s = val\n", index, goFieldName(uint32(index)))
					}
				}
			}
			source.WriteString("\t\t}\n")
		}
		source.WriteString("\t}\n}\n\n")
	}
}
