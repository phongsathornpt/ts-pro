package golang

import (
	"fmt"
	"go/format"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
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

	var source strings.Builder
	source.WriteString("package main\n\n")
	if g.usesFmt || g.usesMath {
		source.WriteString("import (\n")
		if g.usesFmt {
			source.WriteString("\t\"fmt\"\n")
		}
		if g.usesMath {
			source.WriteString("\t\"math\"\n")
		}
		source.WriteString(")\n\n")
	}
	source.WriteString(body.String())

	formatted, err := format.Source([]byte(source.String()))
	if err != nil {
		return "", fmt.Errorf("format generated Go: %w\n%s", err, source.String())
	}
	return string(formatted), nil
}

type generator struct {
	module    mir.Module
	functions map[mir.FunctionID]mir.Function
	shapes    map[mir.ShapeID]mir.Shape
	usesFmt   bool
	usesMath  bool
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
		assign("float64(len(" + operand(op.Array) + "))")
	case mir.ArrayGetF64:
		assign(fmt.Sprintf("%s[int(%s)]", operand(op.Array), operand(op.Index)))
	case mir.ArraySetF64:
		fmt.Fprintf(out, "\t%s[int(%s)] = %s\n", operand(op.Array), operand(op.Index), operand(op.Value))
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
		case mir.IntrinsicConsoleLogF64, mir.IntrinsicConsoleLogString:
			g.usesFmt = true
			fmt.Fprintf(out, "\tfmt.Println(%s)\n", operand(op.Args[0]))
		default:
			return unsupportedInstruction(fn, block, inst, "intrinsic")
		}
	case mir.ArrayNewBool, mir.ArrayLengthBool, mir.ArrayGetBool, mir.ArraySetBool,
		mir.ArrayNewRef, mir.ArrayLengthRef, mir.ArrayGetRef, mir.ArraySetRef:
		return unsupportedInstruction(fn, block, inst, "only numeric arrays are supported")
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

func goType(repr mir.Repr) (string, error) {
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
