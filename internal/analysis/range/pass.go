package rangeanalysis

import (
	"math"
	"math/big"

	"github.com/phongsathornpt/ts-pro/internal/hir"
)

const MaxSafeInteger int64 = 9007199254740991
const MinSafeInteger int64 = -MaxSafeInteger

type Interval struct {
	Min   int64
	Max   int64
	Known bool
}

type FunctionResult map[hir.ValueID]Interval
type Result map[hir.FunctionID]FunctionResult

func Analyze(module hir.Module) Result {
	result := Result{}
	paramRanges := make(map[hir.FunctionID]map[int]Interval)
	fnReturns := make(map[hir.FunctionID]Interval)

	for _, fn := range module.Functions {
		result[fn.ID] = analyzeFunction(fn, paramRanges, fnReturns)
	}

	for round := 0; round < 4; round++ {
		changed := false

		// 1. Collect returns from each function
		for _, fn := range module.Functions {
			fnRes := result[fn.ID]
			var retInterval Interval
			hasReturns := false
			allKnown := true
			for _, b := range fn.Blocks {
				if ret, ok := b.Terminator.(hir.ReturnTerm); ok && ret.Value != nil {
					hasReturns = true
					if val, ok := fnRes[*ret.Value]; ok && val.Known {
						if !retInterval.Known {
							retInterval = val
						} else {
							if val.Min < retInterval.Min {
								retInterval.Min = val.Min
							}
							if val.Max > retInterval.Max {
								retInterval.Max = val.Max
							}
						}
					} else {
						allKnown = false
						break
					}
				}
			}
			if hasReturns && allKnown && retInterval.Known {
				if prev, exists := fnReturns[fn.ID]; !exists || prev != retInterval {
					fnReturns[fn.ID] = retInterval
					changed = true
				}
			}
		}

		// 2. Collect call arguments across all functions
		for _, fn := range module.Functions {
			fnRes := result[fn.ID]
			for _, b := range fn.Blocks {
				for _, inst := range b.Instructions {
					if call, ok := inst.Op.(hir.CallOp); ok {
						if paramRanges[call.Callee] == nil {
							paramRanges[call.Callee] = make(map[int]Interval)
						}
						for i, arg := range call.Args {
							if argVal, ok := fnRes[arg]; ok && argVal.Known {
								prev, exists := paramRanges[call.Callee][i]
								if !exists {
									paramRanges[call.Callee][i] = argVal
									changed = true
								} else {
									merged := prev
									if argVal.Min < merged.Min {
										merged.Min = argVal.Min
									}
									if argVal.Max > merged.Max {
										merged.Max = argVal.Max
									}
									if merged != prev {
										paramRanges[call.Callee][i] = merged
										changed = true
									}
								}
							}
						}
					}
				}
			}
		}

		if !changed {
			break
		}

		for _, fn := range module.Functions {
			result[fn.ID] = analyzeFunction(fn, paramRanges, fnReturns)
		}
	}

	return result
}

func analyzeFunction(fn hir.Function, paramRanges map[hir.FunctionID]map[int]Interval, fnReturns map[hir.FunctionID]Interval) FunctionResult {
	values := FunctionResult{}
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if interval, ok := constInterval(inst); ok {
				values[inst.Result] = interval
			}
		}
	}
	if paramRanges != nil && paramRanges[fn.ID] != nil {
		for i, param := range fn.Params {
			if interval, ok := paramRanges[fn.ID][i]; ok && interval.Known {
				values[param.Value] = interval
			}
		}
	}

	for iteration := 0; iteration < 12; iteration++ {
		bounds := extractBranchBounds(fn, values)
		changed := false
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				interval, ok := inferInstruction(inst, values, bounds, fnReturns)
				if !ok {
					continue
				}
				if previous, exists := values[inst.Result]; !exists || previous != interval {
					values[inst.Result] = interval
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	return values
}

func extractBranchBounds(fn hir.Function, values FunctionResult) map[hir.ValueID]Interval {
	bounds := make(map[hir.ValueID]Interval)
	instMap := make(map[hir.ValueID]hir.Instruction)
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			instMap[inst.Result] = inst
		}
	}
	for _, block := range fn.Blocks {
		branch, ok := block.Terminator.(hir.BranchTerm)
		if !ok {
			continue
		}
		condInst, ok := instMap[branch.Condition]
		if !ok {
			continue
		}
		cmp, ok := condInst.Op.(hir.BinaryExpr)
		if !ok {
			continue
		}
		right, rok := values[cmp.Right]
		left, lok := values[cmp.Left]
		if rok && right.Known {
			switch cmp.Operator {
			case hir.BinaryLessThan:
				mergeBound(bounds, cmp.Left, Interval{Min: MinSafeInteger, Max: right.Max - 1, Known: true})
			case hir.BinaryLessEqual:
				mergeBound(bounds, cmp.Left, Interval{Min: MinSafeInteger, Max: right.Max, Known: true})
			case hir.BinaryGreaterThan:
				mergeBound(bounds, cmp.Left, Interval{Min: right.Min + 1, Max: MaxSafeInteger, Known: true})
			case hir.BinaryGreaterEqual:
				mergeBound(bounds, cmp.Left, Interval{Min: right.Min, Max: MaxSafeInteger, Known: true})
			}
		}
		if lok && left.Known {
			switch cmp.Operator {
			case hir.BinaryLessThan:
				mergeBound(bounds, cmp.Right, Interval{Min: left.Min + 1, Max: MaxSafeInteger, Known: true})
			case hir.BinaryLessEqual:
				mergeBound(bounds, cmp.Right, Interval{Min: left.Min, Max: MaxSafeInteger, Known: true})
			case hir.BinaryGreaterThan:
				mergeBound(bounds, cmp.Right, Interval{Min: MinSafeInteger, Max: left.Max - 1, Known: true})
			case hir.BinaryGreaterEqual:
				mergeBound(bounds, cmp.Right, Interval{Min: MinSafeInteger, Max: left.Max, Known: true})
			}
		}
	}
	return bounds
}

func mergeBound(bounds map[hir.ValueID]Interval, val hir.ValueID, b Interval) {
	if prev, ok := bounds[val]; ok && prev.Known {
		if b.Min > prev.Min {
			prev.Min = b.Min
		}
		if b.Max < prev.Max {
			prev.Max = b.Max
		}
		bounds[val] = prev
	} else {
		bounds[val] = b
	}
}

func constInterval(inst hir.Instruction) (Interval, bool) {
	op, ok := inst.Op.(hir.ConstOp)
	if !ok || op.Literal.Kind != hir.LiteralNumber {
		return Interval{}, false
	}
	value := op.Literal.Number
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return Interval{}, false
	}
	if value < float64(MinSafeInteger) || value > float64(MaxSafeInteger) {
		return Interval{}, false
	}
	integer := int64(value)
	return Interval{Min: integer, Max: integer, Known: true}, true
}

func inferInstruction(inst hir.Instruction, values FunctionResult, bounds map[hir.ValueID]Interval, fnReturns map[hir.FunctionID]Interval) (Interval, bool) {
	switch op := inst.Op.(type) {
	case hir.BinaryExpr:
		left, lok := values[op.Left]
		right, rok := values[op.Right]
		if !lok || !rok || !left.Known || !right.Known {
			return Interval{}, false
		}
		return inferBinary(op.Operator, left, right)
	case hir.PhiOp:
		return inferPhi(op, values, bounds, inst.Result)
	case hir.CallOp:
		if fnReturns != nil {
			if ret, ok := fnReturns[op.Callee]; ok && ret.Known {
				return ret, true
			}
		}
		return Interval{}, false
	default:
		return Interval{}, false
	}
}

func inferBinary(op hir.BinaryOperator, left, right Interval) (Interval, bool) {
	switch op {
	case hir.BinaryAdd:
		return boundedBinary(left, right, func(a, b *big.Int) *big.Int { return new(big.Int).Add(a, b) })
	case hir.BinarySub:
		lo := new(big.Int).Sub(big.NewInt(left.Min), big.NewInt(right.Max))
		hi := new(big.Int).Sub(big.NewInt(left.Max), big.NewInt(right.Min))
		return boundedBig(lo, hi)
	case hir.BinaryMul:
		candidates := []*big.Int{
			new(big.Int).Mul(big.NewInt(left.Min), big.NewInt(right.Min)),
			new(big.Int).Mul(big.NewInt(left.Min), big.NewInt(right.Max)),
			new(big.Int).Mul(big.NewInt(left.Max), big.NewInt(right.Min)),
			new(big.Int).Mul(big.NewInt(left.Max), big.NewInt(right.Max)),
		}
		lo, hi := candidates[0], candidates[0]
		for _, value := range candidates[1:] {
			if value.Cmp(lo) < 0 {
				lo = value
			}
			if value.Cmp(hi) > 0 {
				hi = value
			}
		}
		return boundedBig(lo, hi)
	case hir.BinaryDiv:
		if left.Min != left.Max || right.Min != right.Max || right.Min == 0 || left.Min%right.Min != 0 {
			return Interval{}, false
		}
		value := left.Min / right.Min
		return safeInterval(value, value)
	default:
		return Interval{}, false
	}
}

func boundedBinary(left, right Interval, fn func(*big.Int, *big.Int) *big.Int) (Interval, bool) {
	lo := fn(big.NewInt(left.Min), big.NewInt(right.Min))
	hi := fn(big.NewInt(left.Max), big.NewInt(right.Max))
	return boundedBig(lo, hi)
}

func boundedBig(lo, hi *big.Int) (Interval, bool) {
	minSafe, maxSafe := big.NewInt(MinSafeInteger), big.NewInt(MaxSafeInteger)
	if lo.Cmp(minSafe) < 0 || hi.Cmp(maxSafe) > 0 || !lo.IsInt64() || !hi.IsInt64() {
		return Interval{}, false
	}
	return Interval{Min: lo.Int64(), Max: hi.Int64(), Known: true}, true
}

func safeInterval(minimum, maximum int64) (Interval, bool) {
	if minimum < MinSafeInteger || maximum > MaxSafeInteger || minimum > maximum {
		return Interval{}, false
	}
	return Interval{Min: minimum, Max: maximum, Known: true}, true
}

func inferPhi(op hir.PhiOp, values FunctionResult, bounds map[hir.ValueID]Interval, resultID hir.ValueID) (Interval, bool) {
	if len(op.Incoming) == 0 {
		return Interval{}, false
	}
	var result Interval
	hasAnyKnown := false
	allKnown := true
	for _, incoming := range op.Incoming {
		value, ok := values[incoming.Value]
		if !ok || !value.Known {
			allKnown = false
			continue
		}
		if !hasAnyKnown {
			result = value
			hasAnyKnown = true
		} else {
			if value.Min < result.Min {
				result.Min = value.Min
			}
			if value.Max > result.Max {
				result.Max = value.Max
			}
		}
	}
	if !hasAnyKnown {
		return Interval{}, false
	}
	if !allKnown {
		if bound, ok := bounds[resultID]; ok && bound.Known {
			if bound.Max < result.Max {
				result.Max = bound.Max
			}
			if bound.Min > result.Min {
				result.Min = bound.Min
			}
			result.Known = true
			return result, true
		}
		return Interval{}, false
	}
	if bound, ok := bounds[resultID]; ok && bound.Known {
		if bound.Max < result.Max {
			result.Max = bound.Max
		}
		if bound.Min > result.Min {
			result.Min = bound.Min
		}
	}
	return result, true
}

func (i Interval) FitsI32() bool {
	return i.Known && i.Min >= math.MinInt32 && i.Max <= math.MaxInt32
}

func (i Interval) FitsI64() bool {
	return i.Known && i.Min >= MinSafeInteger && i.Max <= MaxSafeInteger
}
