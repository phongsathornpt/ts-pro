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

func isChannelRefElementRepr(repr Repr) bool {
	switch repr {
	case ReprStringRef, ReprArrayRef, ReprObjectRef, ReprFunctionRef, ReprJSValue:
		return true
	default:
		return false
	}
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
				if op.Kind == BoxJSObject {
					if _, ok := shapes[op.Shape]; !ok {
						return fmt.Errorf("JSValue object box v%d references unknown shape s%d", inst.Result, op.Shape)
					}
				}
				if op.Kind == BoxJSFunction {
					if !op.HasFunction {
						return fmt.Errorf("JSValue function box v%d has no native target metadata", inst.Result)
					}
					if _, ok := functions[op.Function]; !ok {
						return fmt.Errorf("JSValue function box v%d references unknown function f%d", inst.Result, op.Function)
					}
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
				case UnboxJSObject:
					want = ReprObjectRef
					if _, ok := shapes[op.Shape]; !ok {
						return fmt.Errorf("object JSValue unbox v%d references unknown shape s%d", inst.Result, op.Shape)
					}
				case UnboxJSFunction:
					want = ReprFunctionRef
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
				if op.Operator != DynamicJSNullishCoalesce && op.Operator != DynamicJSLogicalOr && op.Operator != DynamicJSLogicalAnd {
					want := ReprBool
					if op.Operator == DynamicJSSub || op.Operator == DynamicJSMul || op.Operator == DynamicJSDiv {
						want = ReprF64
					}
					if inst.Repr != want {
						return fmt.Errorf("dynamic binary v%d has repr %d; want %d", inst.Result, inst.Repr, want)
					}
				}
				if err := checkValue(op.Left); err != nil {
					return err
				}
				if err := checkValue(op.Right); err != nil {
					return err
				}
			case DynamicCall:
				if inst.Repr != ReprJSValue {
					return fmt.Errorf("dynamic call v%d must produce JSValue", inst.Result)
				}
				if err := checkValue(op.Callee); err != nil {
					return err
				}
				if op.HasReceiver {
					if err := checkValue(op.Receiver); err != nil {
						return err
					}
				}
				for _, arg := range op.Args {
					if err := checkValue(arg); err != nil {
						return err
					}
				}
			case DynamicMethodCall:
				if inst.Repr != ReprJSValue || len(op.Cases) == 0 {
					return fmt.Errorf("dynamic method call v%d must produce JSValue with targets", inst.Result)
				}
				if err := checkValue(op.Receiver); err != nil {
					return err
				}
				for _, arg := range op.Args {
					if err := checkValue(arg); err != nil {
						return err
					}
				}
				for _, target := range op.Cases {
					if _, ok := functions[target.Callee]; !ok {
						return fmt.Errorf("dynamic method call references unknown callee f%d", target.Callee)
					}
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
			case ArrayNewBool:
				if inst.Repr != ReprArrayRef {
					return fmt.Errorf("boolean array v%d must produce arrayref", inst.Result)
				}
				for _, element := range op.Elements {
					if err := checkValue(element); err != nil {
						return err
					}
				}
			case ArrayNewRef:
				if inst.Repr != ReprArrayRef {
					return fmt.Errorf("reference array v%d must produce arrayref", inst.Result)
				}
				for _, element := range op.Elements {
					if err := checkValue(element); err != nil {
						return err
					}
				}
			case ArrayLengthF64:
				if err := checkValue(op.Array); err != nil {
					return err
				}
			case ArrayLengthBool:
				if inst.Repr != ReprF64 {
					return fmt.Errorf("boolean array length v%d must produce f64", inst.Result)
				}
				if err := checkValue(op.Array); err != nil {
					return err
				}
			case ArrayLengthRef:
				if inst.Repr != ReprF64 {
					return fmt.Errorf("reference array length v%d must produce f64", inst.Result)
				}
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
			case ArrayGetBool:
				if inst.Repr != ReprBool {
					return fmt.Errorf("boolean array get v%d must produce bool", inst.Result)
				}
				if err := checkValue(op.Array); err != nil {
					return err
				}
				if err := checkValue(op.Index); err != nil {
					return err
				}
			case ArrayGetRef:
				if inst.Repr != ReprStringRef && inst.Repr != ReprObjectRef && inst.Repr != ReprArrayRef && inst.Repr != ReprFunctionRef && inst.Repr != ReprJSValue && inst.Repr != ReprF64 && inst.Repr != ReprBool {
					return fmt.Errorf("reference array get v%d has non-reference repr %d", inst.Result, inst.Repr)
				}
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
			case ArraySetBool:
				if inst.Repr != ReprBool {
					return fmt.Errorf("boolean array set v%d must return bool", inst.Result)
				}
				if err := checkValue(op.Array); err != nil {
					return err
				}
				if err := checkValue(op.Index); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case ArraySetRef:
				if inst.Repr != ReprStringRef && inst.Repr != ReprObjectRef && inst.Repr != ReprArrayRef && inst.Repr != ReprFunctionRef && inst.Repr != ReprJSValue {
					return fmt.Errorf("reference array set v%d has non-reference repr %d", inst.Result, inst.Repr)
				}
				if err := checkValue(op.Array); err != nil {
					return err
				}
				if err := checkValue(op.Index); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case ArrayPushF64:
				if err := checkValue(op.Array); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case ArrayPushBool:
				if err := checkValue(op.Array); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case ArrayPushRef:
				if err := checkValue(op.Array); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case ArrayPopF64:
				if err := checkValue(op.Array); err != nil {
					return err
				}
			case ArrayPopBool:
				if err := checkValue(op.Array); err != nil {
					return err
				}
			case ArrayPopRef:
				if err := checkValue(op.Array); err != nil {
					return err
				}
			case ArrayConcatF64:
				if inst.Repr != ReprArrayRef {
					return fmt.Errorf("array concat f64 v%d has repr %d; want %d", inst.Result, inst.Repr, ReprArrayRef)
				}
				for _, arr := range op.Arrays {
					if err := checkValue(arr); err != nil {
						return err
					}
				}
			case ArrayConcatBool:
				if inst.Repr != ReprArrayRef {
					return fmt.Errorf("array concat bool v%d has repr %d; want %d", inst.Result, inst.Repr, ReprArrayRef)
				}
				for _, arr := range op.Arrays {
					if err := checkValue(arr); err != nil {
						return err
					}
				}
			case ArrayConcatRef:
				if inst.Repr != ReprArrayRef {
					return fmt.Errorf("array concat ref v%d has repr %d; want %d", inst.Result, inst.Repr, ReprArrayRef)
				}
				for _, arr := range op.Arrays {
					if err := checkValue(arr); err != nil {
						return err
					}
				}
			case StringInterpolate:
				if inst.Repr != ReprStringRef {
					return fmt.Errorf("string interpolate v%d has repr %d; want %d", inst.Result, inst.Repr, ReprStringRef)
				}
				for _, part := range op.Parts {
					if err := checkValue(part); err != nil {
						return err
					}
				}
			case JSONStringify:
				if inst.Repr != ReprStringRef {
					return fmt.Errorf("json stringify v%d has repr %d; want %d", inst.Result, inst.Repr, ReprStringRef)
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case JSONParse:
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case MapOp:
				if op.Kind != MapOpNew {
					if err := checkValue(op.Map); err != nil {
						return err
					}
				}
				if op.Kind == MapOpGet || op.Kind == MapOpSet || op.Kind == MapOpHas || op.Kind == MapOpDelete {
					if err := checkValue(op.Key); err != nil {
						return err
					}
				}
				if op.Kind == MapOpSet {
					if err := checkValue(op.Value); err != nil {
						return err
					}
				}
			case SetOp:
				if op.Kind != SetOpNew {
					if err := checkValue(op.Set); err != nil {
						return err
					}
				}
				if op.Kind == SetOpAdd || op.Kind == SetOpHas || op.Kind == SetOpDelete {
					if err := checkValue(op.Item); err != nil {
						return err
					}
				}
			case DateOp:
				if op.Kind != DateOpNow && op.Kind != DateOpNew {
					if err := checkValue(op.Date); err != nil {
						return err
					}
				}
				if op.Kind == DateOpNew && op.Arg != 0 {
					if err := checkValue(op.Arg); err != nil {
						return err
					}
				}
			case RegExpOp:
				if op.Kind == RegExpOpNew {
					if err := checkValue(op.Pattern); err != nil {
						return err
					}
					if op.Flags != 0 {
						if err := checkValue(op.Flags); err != nil {
							return err
						}
					}
				} else {
					if err := checkValue(op.RegExp); err != nil {
						return err
					}
					if op.Kind == RegExpOpTest {
						if err := checkValue(op.String); err != nil {
							return err
						}
					}
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
			case FieldAddr:
				shape, ok := shapes[op.Shape]
				if !ok {
					return fmt.Errorf("field addr v%d references unknown shape s%d", inst.Result, op.Shape)
				}
				if int(op.Field) >= len(shape.Fields) {
					return fmt.Errorf("field addr v%d references invalid field %d of shape s%d", inst.Result, op.Field, op.Shape)
				}
				if err := checkValue(op.Object); err != nil {
					return err
				}
				if inst.Repr != ReprRawPtr {
					return fmt.Errorf("field addr v%d must produce ReprRawPtr", inst.Result)
				}
			case PtrLoad:
				if err := checkValue(op.Ptr); err != nil {
					return err
				}
				if inst.Repr != op.Repr {
					return fmt.Errorf("ptr load v%d representation mismatch: inst %v != op %v", inst.Result, inst.Repr, op.Repr)
				}
			case PtrStore:
				if err := checkValue(op.Ptr); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case DynamicFieldGet:
				if op.Field == "" {
					return fmt.Errorf("dynamic field access v%d has empty name", inst.Result)
				}
				if inst.Repr != ReprJSValue {
					return fmt.Errorf("dynamic field access v%d must produce JSValue", inst.Result)
				}
				if err := checkValue(op.Object); err != nil {
					return err
				}
			case DynamicFieldSet:
				if op.Field == "" {
					return fmt.Errorf("dynamic field store v%d has empty name", inst.Result)
				}
				if inst.Repr != ReprJSValue {
					return fmt.Errorf("dynamic field store v%d must produce JSValue", inst.Result)
				}
				if err := checkValue(op.Object); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case DynamicIndexGet:
				if err := checkValue(op.Object); err != nil {
					return err
				}
				if err := checkValue(op.Index); err != nil {
					return err
				}
			case DynamicIndexSet:
				if err := checkValue(op.Object); err != nil {
					return err
				}
				if err := checkValue(op.Index); err != nil {
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
			case PromiseResolve:
				if inst.Repr != ReprTaskRef {
					return fmt.Errorf("Promise.resolve v%d must produce TaskRef", inst.Result)
				}
				if op.Result != ReprF64 && op.Result != ReprBool && op.Result != ReprJSValue {
					return fmt.Errorf("Promise.resolve v%d has unsupported result representation %d", inst.Result, op.Result)
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case PromiseAdopt:
				if inst.Repr != ReprTaskRef {
					return fmt.Errorf("Promise adoption v%d must produce TaskRef", inst.Result)
				}
				if err := checkValue(op.Promise); err != nil {
					return err
				}
			case PromiseThenable:
				if inst.Repr != ReprTaskRef || (op.Result != ReprF64 && op.Result != ReprBool && op.Result != ReprJSValue) || (op.Arity != 1 && op.Arity != 2) {
					return fmt.Errorf("Promise thenable v%d has invalid task representation or callback arity", inst.Result)
				}
				if err := checkValue(op.Thenable); err != nil {
					return err
				}
				for _, target := range op.Cases {
					if _, ok := functions[target.Callee]; !ok {
						return fmt.Errorf("Promise thenable references unknown method f%d", target.Callee)
					}
				}
			case PromiseReject:
				if inst.Repr != ReprTaskRef || (op.Result != ReprF64 && op.Result != ReprBool && op.Result != ReprJSValue) {
					return fmt.Errorf("Promise.reject v%d has invalid task representation", inst.Result)
				}
				if err := checkValue(op.Reason); err != nil {
					return err
				}
			case PromiseAllF64:
				if inst.Repr != ReprTaskRef {
					return fmt.Errorf("Promise.all v%d must produce TaskRef", inst.Result)
				}
				for _, promise := range op.Promises {
					if err := checkValue(promise); err != nil {
						return err
					}
				}
			case PromiseRaceF64:
				if inst.Repr != ReprTaskRef {
					return fmt.Errorf("Promise.race v%d must produce TaskRef", inst.Result)
				}
				for _, promise := range op.Promises {
					if err := checkValue(promise); err != nil {
						return err
					}
				}
			case PromiseAllBool:
				if inst.Repr != ReprTaskRef {
					return fmt.Errorf("boolean Promise.all v%d must produce TaskRef", inst.Result)
				}
				for _, promise := range op.Promises {
					if err := checkValue(promise); err != nil {
						return err
					}
				}
			case PromiseRaceBool:
				if inst.Repr != ReprTaskRef {
					return fmt.Errorf("boolean Promise.race v%d must produce TaskRef", inst.Result)
				}
				for _, promise := range op.Promises {
					if err := checkValue(promise); err != nil {
						return err
					}
				}
			case PromiseAllRef:
				if inst.Repr != ReprTaskRef {
					return fmt.Errorf("reference Promise.all v%d must produce TaskRef", inst.Result)
				}
				for _, promise := range op.Promises {
					if err := checkValue(promise); err != nil {
						return err
					}
				}
			case PromiseRaceRef:
				if inst.Repr != ReprTaskRef {
					return fmt.Errorf("reference Promise.race v%d must produce TaskRef", inst.Result)
				}
				for _, promise := range op.Promises {
					if err := checkValue(promise); err != nil {
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
				if op.Group != nil {
					if err := checkValue(*op.Group); err != nil {
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
			case TaskWait:
				if inst.Repr != ReprBool {
					return fmt.Errorf("task wait v%d must produce Bool", inst.Result)
				}
				if err := checkValue(op.Task); err != nil {
					return err
				}
			case TaskFailure:
				if inst.Repr != ReprJSValue {
					return fmt.Errorf("task failure v%d must produce JSValue", inst.Result)
				}
				if err := checkValue(op.Task); err != nil {
					return err
				}
			case TaskRetain:
				if err := checkValue(op.Task); err != nil {
					return err
				}
			case TaskRelease:
				if inst.Repr != ReprVoid {
					return fmt.Errorf("task release v%d must be void", inst.Result)
				}
				if err := checkValue(op.Task); err != nil {
					return err
				}
			case TaskYield:
				if inst.Repr != ReprVoid {
					return fmt.Errorf("task yield v%d must be void", inst.Result)
				}
			case TaskCancel:
				if inst.Repr != ReprVoid {
					return fmt.Errorf("task cancel v%d must be void", inst.Result)
				}
				if err := checkValue(op.Task); err != nil {
					return err
				}
			case TaskCancelled:
				if inst.Repr != ReprBool {
					return fmt.Errorf("task cancelled v%d must produce Bool", inst.Result)
				}
			case TaskGroupNew:
				if inst.Repr != ReprTaskGroupRef {
					return fmt.Errorf("task group new v%d must produce TaskGroupRef", inst.Result)
				}
			case TaskGroupJoin:
				if inst.Repr != ReprVoid {
					return fmt.Errorf("task group join v%d must be void", inst.Result)
				}
				if err := checkValue(op.Group); err != nil {
					return err
				}
			case TaskGroupCancel:
				if inst.Repr != ReprVoid {
					return fmt.Errorf("task group cancel v%d must be void", inst.Result)
				}
				if err := checkValue(op.Group); err != nil {
					return err
				}
			case TaskContextSet:
				if inst.Repr != ReprVoid {
					return fmt.Errorf("task context set v%d must be void", inst.Result)
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case TaskContextGet:
				if inst.Repr != ReprJSValue {
					return fmt.Errorf("task context get v%d must produce JSValue", inst.Result)
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
			case ChannelNewBool:
				if inst.Repr != ReprChannelRef {
					return fmt.Errorf("boolean channel new v%d must produce ChannelRef", inst.Result)
				}
				if err := checkValue(op.Capacity); err != nil {
					return err
				}
			case ChannelTrySendBool:
				if inst.Repr != ReprBool {
					return fmt.Errorf("boolean channel try send v%d must produce Bool", inst.Result)
				}
				if err := checkValue(op.Channel); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case ChannelTryRecvOrBool:
				if inst.Repr != ReprBool {
					return fmt.Errorf("boolean channel try recv v%d must produce Bool", inst.Result)
				}
				if err := checkValue(op.Channel); err != nil {
					return err
				}
				if err := checkValue(op.Fallback); err != nil {
					return err
				}
			case ChannelSendBool:
				if inst.Repr != ReprVoid {
					return fmt.Errorf("boolean channel send v%d must be void", inst.Result)
				}
				if err := checkValue(op.Channel); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case ChannelRecvBool:
				if inst.Repr != ReprBool {
					return fmt.Errorf("boolean channel recv v%d must produce Bool", inst.Result)
				}
				if err := checkValue(op.Channel); err != nil {
					return err
				}
			case ChannelNewRef:
				if inst.Repr != ReprChannelRef {
					return fmt.Errorf("reference channel new v%d must produce ChannelRef", inst.Result)
				}
				if err := checkValue(op.Capacity); err != nil {
					return err
				}
			case ChannelTrySendRef:
				if inst.Repr != ReprBool {
					return fmt.Errorf("reference channel try send v%d must produce Bool", inst.Result)
				}
				if err := checkValue(op.Channel); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case ChannelTryRecvOrRef:
				if !isChannelRefElementRepr(inst.Repr) {
					return fmt.Errorf("reference channel try recv v%d has unsupported result representation %d", inst.Result, inst.Repr)
				}
				if err := checkValue(op.Channel); err != nil {
					return err
				}
				if err := checkValue(op.Fallback); err != nil {
					return err
				}
			case ChannelSendRef:
				if inst.Repr != ReprVoid {
					return fmt.Errorf("reference channel send v%d must be void", inst.Result)
				}
				if err := checkValue(op.Channel); err != nil {
					return err
				}
				if err := checkValue(op.Value); err != nil {
					return err
				}
			case ChannelRecvRef:
				if !isChannelRefElementRepr(inst.Repr) {
					return fmt.Errorf("reference channel recv v%d has unsupported result representation %d", inst.Result, inst.Repr)
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
		case Throw:
			if err := checkValue(term.Value); err != nil {
				return err
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
