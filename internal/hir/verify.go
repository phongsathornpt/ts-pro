package hir

import (
	"fmt"
	"strings"
)

type VerificationError struct {
	Function *FunctionID
	Message  string
}

type VerificationErrors []VerificationError

func (e VerificationErrors) Error() string {
	parts := make([]string, 0, len(e))
	for _, item := range e {
		if item.Function == nil {
			parts = append(parts, item.Message)
			continue
		}
		parts = append(parts, fmt.Sprintf("function f%d: %s", *item.Function, item.Message))
	}
	return strings.Join(parts, "; ")
}

func (m Module) Verify() error {
	var errors VerificationErrors
	errors = append(errors, m.verifyTypes()...)
	errors = append(errors, m.verifyShapes()...)
	errors = append(errors, m.verifyFunctions()...)
	if len(errors) == 0 {
		return nil
	}
	return errors
}
func (m Module) verifyTypes() VerificationErrors {
	var errors VerificationErrors
	for index, semantic := range m.Types {
		check := func(target TypeID) {
			if int(target) >= len(m.Types) {
				errors = append(errors, VerificationError{Message: fmt.Sprintf("type t%d references invalid type t%d", index, target)})
			}
		}
		switch semantic.Kind {
		case TypeArray:
			check(semantic.Element)
		case TypeUnion:
			for _, member := range semantic.Members {
				check(member)
			}
		case TypeFunction:
			for _, param := range semantic.Params {
				check(param)
			}
			check(semantic.ReturnType)
		}
	}
	return errors
}

func (m Module) verifyShapes() VerificationErrors {
	var errors VerificationErrors
	seen := map[ShapeID]struct{}{}
	for _, shape := range m.Shapes {
		if _, ok := seen[shape.ID]; ok {
			errors = append(errors, VerificationError{Message: fmt.Sprintf("duplicate shape s%d", shape.ID)})
		}
		seen[shape.ID] = struct{}{}
		for _, field := range shape.Fields {
			if int(field.SemanticType) >= len(m.Types) {
				errors = append(errors, VerificationError{Message: fmt.Sprintf("shape s%d field %s references invalid type t%d", shape.ID, field.Name, field.SemanticType)})
			}
		}
	}
	for i, typ := range m.Types {
		if typ.Kind == TypeObject {
			if _, ok := seen[typ.Shape]; !ok {
				errors = append(errors, VerificationError{Message: fmt.Sprintf("object type t%d references unknown shape s%d", i, typ.Shape)})
			}
		}
	}
	return errors
}

func (m Module) verifyFunctions() VerificationErrors {
	var errors VerificationErrors
	functionIDs := make(map[FunctionID]struct{}, len(m.Functions))
	for _, function := range m.Functions {
		if _, exists := functionIDs[function.ID]; exists {
			errors = append(errors, VerificationError{Message: fmt.Sprintf("duplicate function id f%d", function.ID)})
		}
		functionIDs[function.ID] = struct{}{}
	}
	if m.Entry != nil {
		if _, exists := functionIDs[*m.Entry]; !exists {
			errors = append(errors, VerificationError{Message: fmt.Sprintf("entry references unknown function f%d", *m.Entry)})
		}
	}
	for i := range m.Functions {
		errors = append(errors, m.verifyFunction(&m.Functions[i], functionIDs)...)
	}
	return errors
}
func (m Module) verifyFunction(function *Function, functionIDs map[FunctionID]struct{}) VerificationErrors {
	var errors VerificationErrors
	add := func(message string) {
		id := function.ID
		errors = append(errors, VerificationError{Function: &id, Message: message})
	}
	checkType := func(typeID TypeID) {
		if int(typeID) >= len(m.Types) {
			add(fmt.Sprintf("references invalid type t%d", typeID))
		}
	}

	checkType(function.ReturnType)
	blockIDs := make(map[BlockID]struct{}, len(function.Blocks))
	for _, block := range function.Blocks {
		if _, exists := blockIDs[block.ID]; exists {
			add(fmt.Sprintf("duplicate block id b%d", block.ID))
		}
		blockIDs[block.ID] = struct{}{}
	}
	if _, exists := blockIDs[function.Entry]; !exists {
		add(fmt.Sprintf("missing entry block b%d", function.Entry))
	}

	values := make(map[ValueID]struct{})
	for _, param := range function.Params {
		checkType(param.SemanticType)
		if _, exists := values[param.Value]; exists {
			add(fmt.Sprintf("duplicate value id v%d", param.Value))
		}
		values[param.Value] = struct{}{}
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instructions {
			checkType(instruction.SemanticType)
			if _, exists := values[instruction.Result]; exists {
				add(fmt.Sprintf("duplicate value id v%d", instruction.Result))
			}
			values[instruction.Result] = struct{}{}
		}
	}

	checkValue := func(value ValueID) {
		if _, exists := values[value]; !exists {
			add(fmt.Sprintf("references unknown value v%d", value))
		}
	}
	checkBlock := func(block BlockID) {
		if _, exists := blockIDs[block]; !exists {
			add(fmt.Sprintf("references unknown block b%d", block))
		}
	}

	for _, block := range function.Blocks {
		for _, instruction := range block.Instructions {
			switch op := instruction.Op.(type) {
			case ConstOp:
			case UnaryExpr:
				checkValue(op.Operand)
			case BinaryExpr:
				checkValue(op.Left)
				checkValue(op.Right)
			case CallOp:
				if _, exists := functionIDs[op.Callee]; !exists {
					add(fmt.Sprintf("calls unknown function f%d", op.Callee))
				}
				for _, arg := range op.Args {
					checkValue(arg)
				}
			case IntrinsicCallOp:
				if op.Intrinsic == IntrinsicInvalid {
					add(fmt.Sprintf("instruction v%d has invalid intrinsic", instruction.Result))
				}
				for _, arg := range op.Args {
					checkValue(arg)
				}
			case PhiOp:
				if len(op.Incoming) == 0 {
					add(fmt.Sprintf("phi v%d has no incoming values", instruction.Result))
				}
				for _, incoming := range op.Incoming {
					checkBlock(incoming.Block)
					checkValue(incoming.Value)
				}
			case ArrayNewOp:
				for _, element := range op.Elements {
					checkValue(element)
				}
			case ArrayLengthOp:
				checkValue(op.Array)
			case ArrayGetOp:
				checkValue(op.Array)
				checkValue(op.Index)
			case ObjectNewOp:
				if int(op.Shape) >= len(m.Shapes) {
					add(fmt.Sprintf("object allocation references unknown shape s%d", op.Shape))
				}
				for _, field := range op.Fields {
					checkValue(field)
				}
			case FieldGetOp:
				checkValue(op.Object)
				if int(op.Shape) >= len(m.Shapes) {
					add(fmt.Sprintf("field access references unknown shape s%d", op.Shape))
				} else if int(op.Field) >= len(m.Shapes[op.Shape].Fields) {
					add(fmt.Sprintf("field access s%d.%d is out of range", op.Shape, op.Field))
				}
			case ClosureNewOp:
				if _, exists := functionIDs[op.Callee]; !exists {
					add(fmt.Sprintf("closure references unknown function f%d", op.Callee))
				}
				for _, capture := range op.Captures {
					checkValue(capture)
				}
			case ClosureCallOp:
				checkValue(op.Closure)
				for _, arg := range op.Args {
					checkValue(arg)
				}
			case nil:
				add(fmt.Sprintf("instruction v%d has nil operation", instruction.Result))
			}
		}
		switch term := block.Terminator.(type) {
		case ReturnTerm:
			if term.Value != nil {
				checkValue(*term.Value)
			}
		case JumpTerm:
			checkBlock(term.Target)
		case BranchTerm:
			checkValue(term.Condition)
			checkBlock(term.Then)
			checkBlock(term.Else)
		case nil:
			add(fmt.Sprintf("block b%d has nil terminator", block.ID))
		}
	}
	return errors
}
