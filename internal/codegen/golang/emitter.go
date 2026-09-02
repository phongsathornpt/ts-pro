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

	g := &generator{
		module:    module,
		functions: make(map[mir.FunctionID]mir.Function, len(module.Functions)),
		shapes:    make(map[mir.ShapeID]mir.Shape, len(module.Shapes)),
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
	body.WriteString("}\n")

	if g.usesJSConvert || g.usesJSAdd {
		g.usesFmt = true
		g.usesMath = true
	}

	var source strings.Builder
	source.WriteString("package main\n\n")
	if g.usesFmt || g.usesMath || g.usesRuntime || g.usesSync {
		source.WriteString("import (\n")
		if g.usesFmt {
			source.WriteString("\t\"fmt\"\n")
		}
		if g.usesMath {
			source.WriteString("\t\"math\"\n")
		}
		if g.usesRuntime {
			source.WriteString("\t\"runtime\"\n")
		}
		if g.usesSync {
			source.WriteString("\t\"sync\"\n")
		}
		source.WriteString(")\n\n")
	}

	if g.usesTasks {
		source.WriteString("type tsnativeTask struct {\n\tval any\n\terr any\n\tdone chan struct{}\n}\n\n")
		source.WriteString("func tsnativeDoneChan() chan struct{} {\n\tc := make(chan struct{})\n\tclose(c)\n\treturn c\n}\n\n")
	}
	if g.usesTaskGroups {
		source.WriteString("type tsnativeTaskGroup struct {\n\twg sync.WaitGroup\n\tmu sync.Mutex\n\tcancelled bool\n}\n\n")
	}
	if g.usesJSConvert {
		source.WriteString(`func tsnativeToF64(v any) float64 {
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

func tsnativeToString(v any) string {
	if v == nil {
		return "undefined"
	}
	return fmt.Sprint(v)
}

func tsnativeToBool(v any) bool {
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
		source.WriteString(`func tsnativeDynamicAdd(left, right any) any {
	if s, ok := left.(string); ok {
		return s + tsnativeToString(right)
	}
	if s, ok := right.(string); ok {
		return tsnativeToString(left) + s
	}
	return tsnativeToF64(left) + tsnativeToF64(right)
}

`)
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
	shapes         map[mir.ShapeID]mir.Shape
	usesFmt        bool
	usesMath       bool
	usesRuntime    bool
	usesSync       bool
	usesTasks      bool
	usesTaskGroups bool
	usesJSConvert  bool
	usesJSAdd      bool
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
		if param.HasObjectShape {
			valueShapes[param.Value] = param.ObjectShape
			valueHasShapes[param.Value] = true
		}
		paramValues[param.Value] = struct{}{}
	}
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if inst.Repr == mir.ReprVoid {
				continue
			}
			if err := addValueType(valueTypes, inst.Result, inst.Repr); err != nil {
				return fmt.Errorf("function %s: %w", fn.Name, err)
			}
			switch op := inst.Op.(type) {
			case mir.ObjectNew:
				valueShapes[inst.Result] = op.Shape
				valueHasShapes[inst.Result] = true
			case mir.ObjectAlloc:
				valueShapes[inst.Result] = op.Shape
				valueHasShapes[inst.Result] = true
			case mir.FieldGet:
				field, ok := g.shapeField(op.Shape, op.Field)
				if ok && field.HasObjectShape {
					valueShapes[inst.Result] = field.ObjectShape
					valueHasShapes[inst.Result] = true
				}
			case mir.Call:
				if callee, ok := g.functions[op.Callee]; ok && callee.HasReturnObjectShape {
					valueShapes[inst.Result] = callee.ReturnObjectShape
					valueHasShapes[inst.Result] = true
				}
			}
		}
	}

	returnType, err := g.goTypeForValue(fn.ReturnRepr, fn.ReturnObjectShape, fn.HasReturnObjectShape)
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
			if err := g.emitInstruction(out, fn, block.ID, inst, valueTypes); err != nil {
				return err
			}
		}
		if err := g.emitTerminator(out, fn, block.ID, block.Terminator, phis); err != nil {
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
	shapes := append([]mir.Shape(nil), g.module.Shapes...)
	sort.Slice(shapes, func(i, j int) bool { return shapes[i].ID < shapes[j].ID })
	for _, shape := range shapes {
		fmt.Fprintf(out, "type %s struct {\n", goShapeName(shape.ID))
		for index, field := range shape.Fields {
			typ, err := g.goTypeForValue(field.Repr, field.ObjectShape, field.HasObjectShape)
			if err != nil {
				return fmt.Errorf("shape s%d field %d: %w", shape.ID, index, err)
			}
			fmt.Fprintf(out, "\t%s %s\n", goFieldName(uint32(index)), typ)
		}
		out.WriteString("}\n\n")
	}
	return nil
}

func (g *generator) emitInstruction(out *strings.Builder, fn mir.Function, block mir.BlockID, inst mir.Instruction, valueTypes map[mir.ValueID]mir.Repr) error {
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
		shape, ok := g.shapes[op.Shape]
		if !ok {
			return unsupportedInstruction(fn, block, inst, fmt.Sprintf("missing shape s%d", op.Shape))
		}
		if len(shape.Fields) != len(op.Fields) {
			return unsupportedInstruction(fn, block, inst, "object field count mismatch")
		}
		fields := make([]string, len(op.Fields))
		for i, field := range op.Fields {
			fields[i] = fmt.Sprintf("%s: %s", goFieldName(uint32(i)), operand(field))
		}
		assign(fmt.Sprintf("&%s{%s}", goShapeName(op.Shape), strings.Join(fields, ", ")))
	case mir.ObjectAlloc:
		if _, ok := g.shapes[op.Shape]; !ok {
			return unsupportedInstruction(fn, block, inst, fmt.Sprintf("missing shape s%d", op.Shape))
		}
		assign("&" + goShapeName(op.Shape) + "{}")
	case mir.FieldSet:
		if _, ok := g.shapeField(op.Shape, op.Field); !ok {
			return unsupportedInstruction(fn, block, inst, "invalid object field")
		}
		fmt.Fprintf(out, "\t%s.%s = %s\n", operand(op.Object), goFieldName(op.Field), operand(op.Value))
		if inst.Repr != mir.ReprVoid {
			assign(operand(op.Value))
		}
	case mir.FieldGet:
		if _, ok := g.shapeField(op.Shape, op.Field); !ok {
			return unsupportedInstruction(fn, block, inst, "invalid object field")
		}
		assign(fmt.Sprintf("%s.%s", operand(op.Object), goFieldName(op.Field)))
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
		case mir.IntrinsicConsoleLogF64, mir.IntrinsicConsoleLogString, mir.IntrinsicConsoleLogJSValue:
			g.usesFmt = true
			fmt.Fprintf(out, "\tfmt.Println(%s)\n", operand(op.Args[0]))
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
		assign(fmt.Sprintf("%s.([]any)[int(%s)]", operand(op.Array), operand(op.Index)))
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
			assign(fmt.Sprintf("tsnativeToF64(%s)", operand(op.Value)))
		case mir.UnboxJSString:
			g.usesJSConvert = true
			assign(fmt.Sprintf("tsnativeToString(%s)", operand(op.Value)))
		case mir.UnboxJSBoolean:
			g.usesJSConvert = true
			assign(fmt.Sprintf("tsnativeToBool(%s)", operand(op.Value)))
		default:
			assign(operand(op.Value))
		}
	case mir.DynamicAddJSValue:
		g.usesJSConvert = true
		g.usesJSAdd = true
		assign(fmt.Sprintf("tsnativeDynamicAdd(%s, %s)", operand(op.Left), operand(op.Right)))
	case mir.DynamicBinaryJSValue:
		g.usesJSConvert = true
		switch op.Operator {
		case mir.DynamicJSSub:
			assign(fmt.Sprintf("tsnativeToF64(%s) - tsnativeToF64(%s)", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSMul:
			assign(fmt.Sprintf("tsnativeToF64(%s) * tsnativeToF64(%s)", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSDiv:
			assign(fmt.Sprintf("tsnativeToF64(%s) / tsnativeToF64(%s)", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSLessThan:
			assign(fmt.Sprintf("tsnativeToF64(%s) < tsnativeToF64(%s)", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSLessEqual:
			assign(fmt.Sprintf("tsnativeToF64(%s) <= tsnativeToF64(%s)", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSGreaterThan:
			assign(fmt.Sprintf("tsnativeToF64(%s) > tsnativeToF64(%s)", operand(op.Left), operand(op.Right)))
		case mir.DynamicJSGreaterEqual:
			assign(fmt.Sprintf("tsnativeToF64(%s) >= tsnativeToF64(%s)", operand(op.Left), operand(op.Right)))
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
		assign("&tsnativeTask{done: make(chan struct{})}")
		fmt.Fprintf(out, "\tgo func(t *tsnativeTask) {\n\t\tdefer close(t.done)\n")
		if callee.ReturnRepr != mir.ReprVoid {
			fmt.Fprintf(out, "\t\tt.val = %s(%s)\n\t}(%s)\n", goFunctionName(op.Callee), strings.Join(args, ", "), result)
		} else {
			fmt.Fprintf(out, "\t\t%s(%s)\n\t}(%s)\n", goFunctionName(op.Callee), strings.Join(args, ", "), result)
		}
	case mir.TaskJoin:
		fmt.Fprintf(out, "\t<-%s.done\n", operand(op.Task))
		if inst.Repr != mir.ReprVoid {
			targetType, _ := g.goType(inst.Repr)
			assign(fmt.Sprintf("%s.val.(%s)", operand(op.Task), targetType))
		}
	case mir.TaskWait:
		fmt.Fprintf(out, "\t<-%s.done\n", operand(op.Task))
	case mir.TaskYield:
		g.usesRuntime = true
		fmt.Fprintf(out, "\truntime.Gosched()\n")
	case mir.TaskCancel:
		fmt.Fprintf(out, "\t_ = %s\n", operand(op.Task))
	case mir.TaskCancelled:
		assign("false")
	case mir.PromiseResolve:
		g.usesTasks = true
		assign(fmt.Sprintf("&tsnativeTask{val: %s, done: tsnativeDoneChan()}", operand(op.Value)))
	case mir.PromiseReject:
		g.usesTasks = true
		assign(fmt.Sprintf("&tsnativeTask{err: %s, done: tsnativeDoneChan()}", operand(op.Reason)))
	case mir.ChannelNewF64:
		assign(fmt.Sprintf("make(chan float64, int(%s))", operand(op.Capacity)))
	case mir.ChannelSendF64:
		fmt.Fprintf(out, "\t%s <- %s\n", operand(op.Channel), operand(op.Value))
	case mir.ChannelTrySendF64:
		fmt.Fprintf(out, "\tselect {\n\tcase %s <- %s:\n\t\t%s = true\n\tdefault:\n\t\t%s = false\n\t}\n",
			operand(op.Channel), operand(op.Value), result, result)
	case mir.ChannelTryRecvOrF64:
		fmt.Fprintf(out, "\tselect {\n\tcase %s = <-%s:\n\tdefault:\n\t\t%s = %s\n\t}\n",
			result, operand(op.Channel), result, operand(op.Fallback))
	case mir.ChannelNewBool:
		assign(fmt.Sprintf("make(chan bool, int(%s))", operand(op.Capacity)))
	case mir.ChannelSendBool:
		fmt.Fprintf(out, "\t%s <- %s\n", operand(op.Channel), operand(op.Value))
	case mir.ChannelNewRef:
		assign(fmt.Sprintf("make(chan any, int(%s))", operand(op.Capacity)))
	case mir.ChannelSendRef:
		fmt.Fprintf(out, "\t%s <- %s\n", operand(op.Channel), operand(op.Value))
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

func (g *generator) emitTerminator(out *strings.Builder, fn mir.Function, block mir.BlockID, term mir.Terminator, phis map[mir.BlockID][]phiValue) error {
	emitEdge := func(target mir.BlockID) error {
		for _, phi := range phis[target] {
			value, ok := phi.incoming[block]
			if !ok {
				return fmt.Errorf("function %s block b%d: phi v%d has no incoming value from b%d", fn.Name, block, phi.result, block)
			}
			fmt.Fprintf(out, "\t%s = %s\n", goValueName(phi.result), goValueName(value))
		}
		fmt.Fprintf(out, "\tgoto block%d\n", target)
		return nil
	}

	switch term := term.(type) {
	case mir.Return:
		if term.Value == nil {
			out.WriteString("\treturn\n")
		} else {
			fmt.Fprintf(out, "\treturn %s\n", goValueName(*term.Value))
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
		return unsupportedTerminator(fn, block, term, "throw")
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

func goFunctionName(id mir.FunctionID) string { return fmt.Sprintf("tsnativeGoFunction%d", id) }

func goValueName(id mir.ValueID) string { return fmt.Sprintf("v%d", id) }

func goShapeName(id mir.ShapeID) string { return fmt.Sprintf("tsnativeGoShape%d", id) }

func goFieldName(id uint32) string { return fmt.Sprintf("Field%d", id) }

func (g *generator) goTypeForValue(repr mir.Repr, shapeID mir.ShapeID, hasShape bool) (string, error) {
	if repr == mir.ReprObjectRef && hasShape {
		return "*" + goShapeName(shapeID), nil
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
		return "*tsnativeTask", nil
	case mir.ReprChannelRef:
		return "chan any", nil
	case mir.ReprTaskGroupRef:
		g.usesSync = true
		g.usesTaskGroups = true
		return "*tsnativeTaskGroup", nil
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
