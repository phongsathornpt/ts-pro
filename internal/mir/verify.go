package mir

import "fmt"

func (m Module) Verify() error {
	functions := map[FunctionID]struct{}{}
	for _, fn := range m.Functions {
		if _, ok := functions[fn.ID]; ok {
			return fmt.Errorf("duplicate MIR function f%d", fn.ID)
		}
		functions[fn.ID] = struct{}{}
	}
	if m.Entry != nil {
		if _, ok := functions[*m.Entry]; !ok {
			return fmt.Errorf("entry references unknown function f%d", *m.Entry)
		}
		for _, fn := range m.Functions {
			if fn.ID == *m.Entry {
				if len(fn.Params) != 0 || fn.ReturnRepr != ReprVoid {
					return fmt.Errorf("entry function f%d must be () -> void", fn.ID)
				}
				break
			}
		}
	}
	for _, fn := range m.Functions {
		if err := verifyFunction(fn, functions); err != nil {
			return fmt.Errorf("function f%d: %w", fn.ID, err)
		}
	}
	return nil
}

func verifyFunction(fn Function, functions map[FunctionID]struct{}) error {
	blocks := map[BlockID]struct{}{}
	values := map[ValueID]struct{}{}
	for _, param := range fn.Params {
		if param.Repr == ReprInvalid {
			return fmt.Errorf("parameter v%d has invalid representation", param.Value)
		}
		if _, ok := values[param.Value]; ok {
			return fmt.Errorf("duplicate value v%d", param.Value)
		}
		values[param.Value] = struct{}{}
	}
	for _, block := range fn.Blocks {
		if _, ok := blocks[block.ID]; ok {
			return fmt.Errorf("duplicate block b%d", block.ID)
		}
		blocks[block.ID] = struct{}{}
		for _, inst := range block.Instructions {
			if inst.Repr == ReprInvalid || inst.Op == nil {
				return fmt.Errorf("invalid instruction v%d", inst.Result)
			}
			if _, ok := values[inst.Result]; ok {
				return fmt.Errorf("duplicate value v%d", inst.Result)
			}
			values[inst.Result] = struct{}{}
		}
	}
	if _, ok := blocks[fn.Entry]; !ok {
		return fmt.Errorf("missing entry block b%d", fn.Entry)
	}
	return verifyUses(fn, functions, blocks, values)
}

func verifyUses(fn Function, functions map[FunctionID]struct{}, blocks map[BlockID]struct{}, values map[ValueID]struct{}) error {
	checkValue := func(v ValueID) error {
		if _, ok := values[v]; !ok {
			return fmt.Errorf("unknown value v%d", v)
		}
		return nil
	}
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			switch op := inst.Op.(type) {
			case ConstF64:
			case FloatBinary:
				if err := checkValue(op.Left); err != nil {
					return err
				}
				if err := checkValue(op.Right); err != nil {
					return err
				}
			case FloatCompare:
				if err := checkValue(op.Left); err != nil {
					return err
				}
				if err := checkValue(op.Right); err != nil {
					return err
				}
			case Call:
				if _, ok := functions[op.Callee]; !ok {
					return fmt.Errorf("unknown callee f%d", op.Callee)
				}
				for _, arg := range op.Args {
					if err := checkValue(arg); err != nil {
						return err
					}
				}
			case IntrinsicCall:
				if op.Intrinsic == IntrinsicInvalid {
					return fmt.Errorf("invalid intrinsic")
				}
				for _, arg := range op.Args {
					if err := checkValue(arg); err != nil {
						return err
					}
				}
			case Phi:
				if len(op.Incoming) == 0 {
					return fmt.Errorf("phi v%d has no incoming values", inst.Result)
				}
				for _, incoming := range op.Incoming {
					if _, ok := blocks[incoming.Block]; !ok {
						return fmt.Errorf("phi v%d references unknown block b%d", inst.Result, incoming.Block)
					}
					if err := checkValue(incoming.Value); err != nil {
						return err
					}
				}
			case ArrayNewF64:
				for _, element := range op.Elements {
					if err := checkValue(element); err != nil {
						return err
					}
				}
			case ArrayLengthF64:
				if err := checkValue(op.Array); err != nil {
					return err
				}
			case ArrayGetF64:
				if err := checkValue(op.Array); err != nil {
					return err
				}
				if err := checkValue(op.Index); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported operation %T", inst.Op)
			}
		}
		if block.Terminator == nil {
			return fmt.Errorf("block b%d has no terminator", block.ID)
		}
		switch term := block.Terminator.(type) {
		case Return:
			if term.Value != nil {
				if err := checkValue(*term.Value); err != nil {
					return err
				}
			}
		case Jump:
			if _, ok := blocks[term.Target]; !ok {
				return fmt.Errorf("unknown block b%d", term.Target)
			}
		case Branch:
			if err := checkValue(term.Condition); err != nil {
				return err
			}
			if _, ok := blocks[term.Then]; !ok {
				return fmt.Errorf("unknown block b%d", term.Then)
			}
			if _, ok := blocks[term.Else]; !ok {
				return fmt.Errorf("unknown block b%d", term.Else)
			}
		default:
			return fmt.Errorf("unsupported terminator %T", block.Terminator)
		}
	}
	return nil
}
