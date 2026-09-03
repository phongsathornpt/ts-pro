package escape

import "github.com/phongsathornpt/ts-pro/internal/mir"

type AllocationKind uint8

const (
	AllocationInvalid AllocationKind = iota
	AllocationObject
	AllocationClosure
)

type Reason uint32

const (
	ReasonReturn Reason = 1 << iota
	ReasonThrow
	ReasonCall
	ReasonHeapStore
	ReasonTask
	ReasonChannel
	ReasonBox
	ReasonSuspension
	ReasonContained
)

type Info struct {
	Kind    AllocationKind
	Escapes bool
	Reasons Reason
}

type FunctionResult map[mir.ValueID]Info
type Result map[mir.FunctionID]FunctionResult

func (result FunctionResult) CanStackAllocate(value mir.ValueID) bool {
	info, ok := result[value]
	return ok && !info.Escapes
}

type valueSet map[mir.ValueID]struct{}
type provenance map[mir.ValueID]valueSet

func Analyze(module mir.Module) Result {
	result := make(Result, len(module.Functions))
	for _, fn := range module.Functions {
		result[fn.ID] = analyzeFunction(fn)
	}
	return result
}
func analyzeFunction(fn mir.Function) FunctionResult {
	infos := make(FunctionResult)
	prov := make(provenance)
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			kind := allocationKind(inst.Op)
			if kind == AllocationInvalid {
				continue
			}
			infos[inst.Result] = Info{Kind: kind}
			prov[inst.Result] = valueSet{inst.Result: {}}
		}
	}
	propagateAliases(fn, prov)
	contained := make(map[mir.ValueID]valueSet)
	if functionHasSuspension(fn) {
		for value := range infos {
			markEscaping(infos, value, ReasonSuspension)
		}
	}
	collectEscapeUses(fn, prov, infos, contained)
	propagateContainedEscapes(infos, contained)
	return infos
}

func allocationKind(op mir.Operation) AllocationKind {
	switch op.(type) {
	case mir.ObjectNew, mir.ObjectAlloc:
		return AllocationObject
	case mir.ClosureNew:
		return AllocationClosure
	default:
		return AllocationInvalid
	}
}

type fieldProvenanceKey struct {
	container mir.ValueID
	field     uint32
}

func propagateAliases(fn mir.Function, prov provenance) {
	fields := make(map[fieldProvenanceKey]valueSet)
	mergeField := func(key fieldProvenanceKey, origins valueSet) bool {
		if len(origins) == 0 {
			return false
		}
		if fields[key] == nil {
			fields[key] = make(valueSet)
		}
		changed := false
		for origin := range origins {
			if _, ok := fields[key][origin]; !ok {
				fields[key][origin] = struct{}{}
				changed = true
			}
		}
		return changed
	}

	changed := true
	for changed {
		changed = false
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				switch op := inst.Op.(type) {
				case mir.Phi:
					for _, incoming := range op.Incoming {
						changed = mergeOrigins(prov, inst.Result, prov[incoming.Value]) || changed
					}
				case mir.ObjectNew:
					for container := range prov[inst.Result] {
						for field, value := range op.Fields {
							changed = mergeField(fieldProvenanceKey{container: container, field: uint32(field)}, prov[value]) || changed
						}
					}
				case mir.FieldSet:
					changed = mergeOrigins(prov, inst.Result, prov[op.Value]) || changed
					for container := range prov[op.Object] {
						changed = mergeField(fieldProvenanceKey{container: container, field: op.Field}, prov[op.Value]) || changed
					}
				case mir.FieldGet:
					for container := range prov[op.Object] {
						changed = mergeOrigins(prov, inst.Result, fields[fieldProvenanceKey{container: container, field: op.Field}]) || changed
					}
				case mir.FieldAddr:
					changed = mergeOrigins(prov, inst.Result, prov[op.Object]) || changed
				case mir.PtrLoad:
					changed = mergeOrigins(prov, inst.Result, prov[op.Ptr]) || changed
				case mir.PtrStore:
					changed = mergeOrigins(prov, op.Ptr, prov[op.Value]) || changed
				}
			}
		}
	}
}

func mergeOrigins(prov provenance, value mir.ValueID, origins valueSet) bool {
	if len(origins) == 0 {
		return false
	}
	if prov[value] == nil {
		prov[value] = make(valueSet)
	}
	changed := false
	for origin := range origins {
		if _, ok := prov[value][origin]; !ok {
			prov[value][origin] = struct{}{}
			changed = true
		}
	}
	return changed
}
func functionHasSuspension(fn mir.Function) bool {
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			switch inst.Op.(type) {
			case mir.TaskJoin, mir.TaskWait, mir.TaskYield, mir.TaskGroupJoin,
				mir.ChannelSendF64, mir.ChannelRecvF64,
				mir.ChannelSendBool, mir.ChannelRecvBool,
				mir.ChannelSendRef, mir.ChannelRecvRef,
				mir.Sleep:
				return true
			}
		}
	}
	return false
}

func markEscaping(infos FunctionResult, value mir.ValueID, reason Reason) {
	info, ok := infos[value]
	if !ok {
		return
	}
	info.Escapes = true
	info.Reasons |= reason
	infos[value] = info
}

func markOrigins(infos FunctionResult, origins valueSet, reason Reason) {
	for origin := range origins {
		markEscaping(infos, origin, reason)
	}
}
func addContainment(contained map[mir.ValueID]valueSet, containers, children valueSet) {
	for container := range containers {
		if contained[container] == nil {
			contained[container] = make(valueSet)
		}
		for child := range children {
			contained[container][child] = struct{}{}
		}
	}
}

func markArguments(infos FunctionResult, prov provenance, args []mir.ValueID, reason Reason) {
	for _, arg := range args {
		markOrigins(infos, prov[arg], reason)
	}
}

func collectEscapeUses(fn mir.Function, prov provenance, infos FunctionResult, contained map[mir.ValueID]valueSet) {
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			switch op := inst.Op.(type) {
			case mir.ObjectNew:
				for _, field := range op.Fields {
					addContainment(contained, prov[inst.Result], prov[field])
				}
			case mir.ArrayNewRef:
				for _, element := range op.Elements {
					markOrigins(infos, prov[element], ReasonHeapStore)
				}
			case mir.ArraySetRef:
				markOrigins(infos, prov[op.Value], ReasonHeapStore)
			case mir.ClosureNew:
				for _, capture := range op.Captures {
					addContainment(contained, prov[inst.Result], prov[capture])
				}
			case mir.FieldSet:
				children := prov[op.Value]
				if len(children) == 0 {
					continue
				}
				containers := prov[op.Object]
				if len(containers) == 0 {
					markOrigins(infos, children, ReasonHeapStore)
				} else {
					addContainment(contained, containers, children)
				}
			case mir.PtrStore:
				children := prov[op.Value]
				if len(children) == 0 {
					continue
				}
				containers := prov[op.Ptr]
				if len(containers) == 0 {
					markOrigins(infos, children, ReasonHeapStore)
				} else {
					addContainment(contained, containers, children)
				}
			case mir.Call:
				markArguments(infos, prov, op.Args, ReasonCall)
			case mir.DispatchCall:
				markArguments(infos, prov, op.Args, ReasonCall)
			case mir.IntrinsicCall:
				markArguments(infos, prov, op.Args, ReasonCall)
			case mir.ClosureCall:
				markArguments(infos, prov, op.Args, ReasonCall)
			case mir.PromiseResolve:
				markOrigins(infos, prov[op.Value], ReasonTask)
			case mir.PromiseReject:
				markOrigins(infos, prov[op.Reason], ReasonTask)
			case mir.TaskSpawn:
				markArguments(infos, prov, op.Captures, ReasonTask)
			case mir.TaskContextSet:
				markOrigins(infos, prov[op.Value], ReasonTask)
			case mir.ChannelTrySendRef:
				markOrigins(infos, prov[op.Value], ReasonChannel)
			case mir.ChannelSendRef:
				markOrigins(infos, prov[op.Value], ReasonChannel)
			case mir.BoxJSValue:
				markOrigins(infos, prov[op.Value], ReasonBox)
			case mir.DynamicAddJSValue:
				markOrigins(infos, prov[op.Left], ReasonCall)
				markOrigins(infos, prov[op.Right], ReasonCall)
			case mir.DynamicBinaryJSValue:
				markOrigins(infos, prov[op.Left], ReasonCall)
				markOrigins(infos, prov[op.Right], ReasonCall)
			}
		}
		switch term := block.Terminator.(type) {
		case mir.Return:
			if term.Value != nil {
				markOrigins(infos, prov[*term.Value], ReasonReturn)
			}
		case mir.Throw:
			markOrigins(infos, prov[term.Value], ReasonThrow)
		}
	}
}
func propagateContainedEscapes(infos FunctionResult, contained map[mir.ValueID]valueSet) {
	changed := true
	for changed {
		changed = false
		for container, children := range contained {
			parent, ok := infos[container]
			if !ok || !parent.Escapes {
				continue
			}
			for child := range children {
				before, ok := infos[child]
				if !ok {
					continue
				}
				markEscaping(infos, child, ReasonContained)
				if infos[child] != before {
					changed = true
				}
			}
		}
	}
}
