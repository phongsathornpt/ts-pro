package mir

import "fmt"

func (m Module) Verify() error {
	shapes := map[ShapeID]Shape{}
	for _, shape := range m.Shapes {
		if _, ok := shapes[shape.ID]; ok {
			return fmt.Errorf("duplicate MIR shape s%d", shape.ID)
		}
		for i, field := range shape.Fields {
			if field.Repr == ReprInvalid || field.Repr == ReprVoid {
				return fmt.Errorf("shape s%d field %d has invalid representation %d", shape.ID, i, field.Repr)
			}
		}
		shapes[shape.ID] = shape
	}
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
		if err := verifyFunction(fn, functions, shapes); err != nil {
			return fmt.Errorf("function f%d: %w", fn.ID, err)
		}
	}
	return nil
}

func verifyFunction(fn Function, functions map[FunctionID]struct{}, shapes map[ShapeID]Shape) error {
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
	return verifyUses(fn, functions, shapes, blocks, values)
}

func verifyUses(fn Function, functions map[FunctionID]struct{}, shapes map[ShapeID]Shape, blocks map[BlockID]struct{}, values map[ValueID]struct{}) error {
	checkValue := func(v ValueID) error {
		if _, ok := values[v]; !ok {
			return fmt.Errorf("unknown value v%d", v)
		}
		return nil
	}
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			switch op := inst.Op.(type) {
			case ConstJSValue:
				if inst.Repr != ReprJSValue || (op.Kind != ConstJSNull && op.Kind != ConstJSUndefined) {
					return fmt.Errorf("JSValue const v%d has invalid kind/repr", inst.Result)
				}
			case ConstBool, ConstF64, ConstString:
			case StringConcat:
				if err := checkValue(op.Left); err != nil {
					return err
				}
				if err := checkValue(op.Right); err != nil {
					return err
				}
			case FloatBinary:
				if err := checkValue(op.Left); err != nil {
					return err
				}
				if err := checkValue(op.Right); err != nil {
					return err
				}
			case ProvenIntBinary:
				if op.Width != IntWidth32 && op.Width != IntWidth64 {
					return fmt.Errorf("proven integer v%d has invalid width %d", inst.Result, op.Width)
				}
				if inst.Repr != ReprF64 {
					return fmt.Errorf("proven integer v%d must preserve f64 boundary representation", inst.Result)
				}
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
			case BoxJSValue:
				if op.Kind != BoxJSNumber && op.Kind != BoxJSString && op.Kind != BoxJSBoolean && op.Kind != BoxJSObject && op.Kind != BoxJSArray && op.Kind != BoxJSFunction {
					return fmt.Errorf("JSValue box v%d has invalid kind %d", inst.Result, op.Kind)
				}
				if inst.Repr != ReprJSValue {
					return fmt.Errorf("JSValue box v%d must produce JSValue representation", inst.Result)
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case UnboxJSValue:
				want := ReprInvalid
				switch op.Kind {
				case UnboxJSNumber:
					want = ReprF64
				case UnboxJSString:
					want = ReprStringRef
				case UnboxJSBoolean:
					want = ReprBool
				case UnboxJSArray:
					want = ReprArrayRef
				}
				if want == ReprInvalid || inst.Repr != want {
					return fmt.Errorf("JSValue unbox v%d has invalid kind/repr %d/%d", inst.Result, op.Kind, inst.Repr)
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case DynamicAddJSValue:
				if inst.Repr != ReprJSValue {
					return fmt.Errorf("dynamic add v%d must produce JSValue representation", inst.Result)
				}
				if err := checkValue(op.Left); err != nil {
					return err
				}
				if err := checkValue(op.Right); err != nil {
					return err
				}
			case DynamicBinaryJSValue:
				if op.Operator == DynamicJSInvalid {
					return fmt.Errorf("dynamic binary v%d has invalid operator", inst.Result)
				}
				want := ReprBool
				if op.Operator == DynamicJSSub || op.Operator == DynamicJSMul || op.Operator == DynamicJSDiv {
					want = ReprF64
				}
				if inst.Repr != want {
					return fmt.Errorf("dynamic binary v%d has repr %d; want %d", inst.Result, inst.Repr, want)
				}
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
			case DispatchCall:
				if len(op.Cases) == 0 || len(op.Args) == 0 {
					return fmt.Errorf("dispatch v%d requires targets and receiver", inst.Result)
				}
				for _, target := range op.Cases {
					if _, ok := functions[target.Callee]; !ok {
						return fmt.Errorf("dispatch v%d references unknown callee f%d", inst.Result, target.Callee)
					}
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
			case ArraySetF64:
				if err := checkValue(op.Array); err != nil {
					return err
				}
				if err := checkValue(op.Index); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
				if inst.Repr != ReprF64 {
					return fmt.Errorf("array set v%d must return f64 assigned value", inst.Result)
				}
			case ObjectNew:
				shape, ok := shapes[op.Shape]
				if !ok {
					return fmt.Errorf("object v%d references unknown shape s%d", inst.Result, op.Shape)
				}
				if len(op.Fields) != len(shape.Fields) {
					return fmt.Errorf("object v%d has %d fields; shape s%d requires %d", inst.Result, len(op.Fields), op.Shape, len(shape.Fields))
				}
				for _, field := range op.Fields {
					if err := checkValue(field); err != nil {
						return err
					}
				}
			case ObjectAlloc:
				if _, ok := shapes[op.Shape]; !ok {
					return fmt.Errorf("object alloc v%d references unknown shape s%d", inst.Result, op.Shape)
				}
			case FieldSet:
				shape, ok := shapes[op.Shape]
				if !ok {
					return fmt.Errorf("field set v%d references unknown shape s%d", inst.Result, op.Shape)
				}
				if int(op.Field) >= len(shape.Fields) {
					return fmt.Errorf("field set v%d references invalid field %d of shape s%d", inst.Result, op.Field, op.Shape)
				}
				if err := checkValue(op.Object); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case FieldGet:
				shape, ok := shapes[op.Shape]
				if !ok {
					return fmt.Errorf("field get v%d references unknown shape s%d", inst.Result, op.Shape)
				}
				if int(op.Field) >= len(shape.Fields) {
					return fmt.Errorf("field get v%d references invalid field %d of shape s%d", inst.Result, op.Field, op.Shape)
				}
				if err := checkValue(op.Object); err != nil {
					return err
				}
			case ClosureNew:
				if _, ok := functions[op.Callee]; !ok {
					return fmt.Errorf("closure v%d references unknown callee f%d", inst.Result, op.Callee)
				}
				for _, capture := range op.Captures {
					if err := checkValue(capture); err != nil {
						return err
					}
				}
			case ClosureCall:
				if err := checkValue(op.Closure); err != nil {
					return err
				}
				for _, arg := range op.Args {
					if err := checkValue(arg); err != nil {
						return err
					}
				}
			case TaskSpawn:
				if _, ok := functions[op.Callee]; !ok {
					return fmt.Errorf("task spawn v%d references unknown callee f%d", inst.Result, op.Callee)
				}
				if inst.Repr != ReprTaskRef {
					return fmt.Errorf("task spawn v%d must produce TaskRef", inst.Result)
				}
				for _, capture := range op.Captures {
					if err := checkValue(capture); err != nil {
						return err
					}
				}
			case TaskJoin:
				if inst.Repr != ReprVoid && inst.Repr != ReprBool && inst.Repr != ReprF64 && inst.Repr != ReprStringRef && inst.Repr != ReprArrayRef && inst.Repr != ReprObjectRef && inst.Repr != ReprFunctionRef && inst.Repr != ReprJSValue {
					return fmt.Errorf("task join v%d has unsupported result representation %d", inst.Result, inst.Repr)
				}
				if err := checkValue(op.Task); err != nil {
					return err
				}
			case TaskYield:
				if inst.Repr != ReprVoid {
					return fmt.Errorf("task yield v%d must be void", inst.Result)
				}
			case ChannelNewF64:
				if inst.Repr != ReprChannelRef {
					return fmt.Errorf("channel new v%d must produce ChannelRef", inst.Result)
				}
				if err := checkValue(op.Capacity); err != nil {
					return err
				}
			case ChannelTrySendF64:
				if inst.Repr != ReprBool {
					return fmt.Errorf("channel try send v%d must produce Bool", inst.Result)
				}
				if err := checkValue(op.Channel); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case ChannelTryRecvOrF64:
				if inst.Repr != ReprF64 {
					return fmt.Errorf("channel try recv v%d must produce F64", inst.Result)
				}
				if err := checkValue(op.Channel); err != nil {
					return err
				}
				if err := checkValue(op.Fallback); err != nil {
					return err
				}
			case ChannelSendF64:
				if inst.Repr != ReprVoid {
					return fmt.Errorf("channel send v%d must be void", inst.Result)
				}
				if err := checkValue(op.Channel); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case ChannelRecvF64:
				if inst.Repr != ReprF64 {
					return fmt.Errorf("channel recv v%d must produce F64", inst.Result)
				}
				if err := checkValue(op.Channel); err != nil {
					return err
				}
			case Sleep:
				if inst.Repr != ReprVoid {
					return fmt.Errorf("sleep v%d must be void", inst.Result)
				}
				if err := checkValue(op.Duration); err != nil {
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
