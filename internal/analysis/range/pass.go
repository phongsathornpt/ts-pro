package rangeanalysis

import (
	"math"
	"math/big"

	"github.com/projectthorn/tsv7-bin/internal/hir"
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
	for _, fn := range module.Functions {
		result[fn.ID] = analyzeFunction(fn)
	}
	return result
}

func analyzeFunction(fn hir.Function) FunctionResult {
	values := FunctionResult{}
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if interval, ok := constInterval(inst); ok {
				values[inst.Result] = interval
			}
		}
	}
	for iteration := 0; iteration < 8; iteration++ {
		changed := false
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				interval, ok := inferInstruction(inst, values)
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

func inferInstruction(inst hir.Instruction, values FunctionResult) (Interval, bool) {
	switch op := inst.Op.(type) {
	case hir.BinaryExpr:
		left, lok := values[op.Left]
		right, rok := values[op.Right]
		if !lok || !rok || !left.Known || !right.Known {
			return Interval{}, false
		}
		return inferBinary(op.Operator, left, right)
	case hir.PhiOp:
		return inferPhi(op, values)
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

func inferPhi(op hir.PhiOp, values FunctionResult) (Interval, bool) {
	if len(op.Incoming) == 0 {
		return Interval{}, false
	}
	var result Interval
	for _, incoming := range op.Incoming {
		value, ok := values[incoming.Value]
		if !ok || !value.Known {
			return Interval{}, false
		}
		if !result.Known {
			result = value
			continue
		}
		if value.Min < result.Min {
			result.Min = value.Min
		}
		if value.Max > result.Max {
			result.Max = value.Max
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
