package lowering

import (
	"fmt"
	"sort"

	"github.com/projectthorn/tsv7-bin/internal/frontend"
	"github.com/projectthorn/tsv7-bin/internal/hir"
)

type functionLowerer struct {
	module     *moduleLowerer
	source     frontend.Function
	result     hir.Function
	locals     map[frontend.SymbolID]hir.ValueID
	nextValue  uint32
	nextBlock  uint32
	current    int
	terminated bool
}

type localState struct {
	pred   hir.BlockID
	locals map[frontend.SymbolID]hir.ValueID
}

type loopPhi struct {
	symbol    frontend.SymbolID
	instIndex int
}

func (l *moduleLowerer) lowerFunction(source frontend.Function) (hir.Function, error) {
	fl := &functionLowerer{
		module: l,
		source: source,
		locals: map[frontend.SymbolID]hir.ValueID{},
		result: hir.Function{
			ID: hir.NewFunctionID(uint32(source.ID)), Name: source.Name,
			ReturnType: l.types[source.ReturnType], Entry: hir.NewBlockID(0),
		},
		nextBlock: 1,
	}
	for _, param := range source.Params {
		value := hir.NewValueID(fl.nextValue)
		fl.nextValue++
		fl.locals[param.Symbol] = value
		fl.result.Params = append(fl.result.Params, hir.Param{
			Value: value, Name: param.Name, SemanticType: l.types[param.Type],
		})
	}
	fl.startBlock(fl.result.Entry)
	if err := fl.lowerStatements(source.Body); err != nil {
		return hir.Function{}, fmt.Errorf("lower function %s: %w", source.Name, err)
	}
	if !fl.terminated {
		if int(source.ReturnType) < len(l.source.Types) && l.source.Types[source.ReturnType].Kind == frontend.TypeVoid {
			if err := fl.terminate(hir.ReturnTerm{}); err != nil {
				return hir.Function{}, err
			}
		} else {
			return hir.Function{}, fmt.Errorf("lower function %s: reachable block b%d has no terminator", source.Name, fl.block().ID)
		}
	}
	return fl.result, nil
}

func (f *functionLowerer) block() *hir.Block { return &f.result.Blocks[f.current] }

func (f *functionLowerer) startBlock(id hir.BlockID) {
	f.result.Blocks = append(f.result.Blocks, hir.Block{ID: id})
	f.current = len(f.result.Blocks) - 1
	f.terminated = false
}

func (f *functionLowerer) newBlockID() hir.BlockID {
	id := hir.NewBlockID(f.nextBlock)
	f.nextBlock++
	return id
}

func (f *functionLowerer) emit(typeID frontend.TypeID, op hir.Operation) hir.ValueID {
	value := hir.NewValueID(f.nextValue)
	f.nextValue++
	f.block().Instructions = append(f.block().Instructions, hir.Instruction{
		Result: value, SemanticType: f.module.types[typeID], Op: op,
	})
	return value
}

func (f *functionLowerer) terminate(term hir.Terminator) error {
	if f.terminated {
		return fmt.Errorf("block b%d already terminated", f.block().ID)
	}
	f.block().Terminator = term
	f.terminated = true
	return nil
}

func (f *functionLowerer) lowerStatements(statements []frontend.Statement) error {
	for _, stmt := range statements {
		if f.terminated {
			break
		}
		if err := f.lowerStatement(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (f *functionLowerer) lowerStatement(stmt frontend.Statement) error {
	switch stmt.Kind {
	case frontend.StmtReturn:
		if stmt.Return == nil {
			return f.terminate(hir.ReturnTerm{})
		}
		value, err := f.lowerExpr(stmt.Return)
		if err != nil {
			return err
		}
		return f.terminate(hir.ReturnTerm{Value: &value})
	case frontend.StmtIf:
		return f.lowerIf(stmt)
	case frontend.StmtBlock:
		return f.lowerStatements(stmt.Then)
	case frontend.StmtExpr:
		if stmt.Expr == nil {
			return fmt.Errorf("expression statement has no expression")
		}
		_, err := f.lowerExpr(stmt.Expr)
		return err
	case frontend.StmtVar, frontend.StmtAssign:
		if stmt.Value == nil {
			return fmt.Errorf("variable %q has no value", stmt.Name)
		}
		value, err := f.lowerExpr(stmt.Value)
		if err != nil {
			return err
		}
		f.locals[stmt.Symbol] = value
		return nil
	case frontend.StmtWhile:
		return f.lowerLoop(stmt.Expr, stmt.Then, nil)
	case frontend.StmtFor:
		if err := f.lowerStatements(stmt.Init); err != nil {
			return err
		}
		return f.lowerLoop(stmt.Expr, stmt.Then, stmt.Update)
	case frontend.StmtClosureBind:
		return nil
	case frontend.StmtArrayAssign:
		if stmt.Object == nil || stmt.Index == nil || stmt.Value == nil {
			return fmt.Errorf("array assignment is incomplete")
		}
		array, err := f.lowerExpr(stmt.Object)
		if err != nil {
			return err
		}
		index, err := f.lowerExpr(stmt.Index)
		if err != nil {
			return err
		}
		value, err := f.lowerExpr(stmt.Value)
		if err != nil {
			return err
		}
		f.emit(stmt.Type, hir.ArraySetOp{Array: array, Index: index, Value: value})
		return nil
	case frontend.StmtFieldAssign:
		if stmt.Object == nil || stmt.Value == nil {
			return fmt.Errorf("field assignment %q is incomplete", stmt.Field)
		}
		object, err := f.lowerExpr(stmt.Object)
		if err != nil {
			return err
		}
		value, err := f.lowerExpr(stmt.Value)
		if err != nil {
			return err
		}
		if int(stmt.Object.Type) >= len(f.module.source.Types) {
			return fmt.Errorf("field assignment has invalid receiver type")
		}
		shape := f.module.source.Types[stmt.Object.Type].Shape
		f.emit(stmt.Type, hir.FieldSetOp{Object: object, Shape: hir.NewShapeID(uint32(shape)), Field: stmt.FieldIndex, Value: value})
		return nil
	default:
		return fmt.Errorf("unsupported semantic statement kind %d", stmt.Kind)
	}
}

func (f *functionLowerer) lowerIf(stmt frontend.Statement) error {
	if stmt.Expr == nil {
		return fmt.Errorf("if statement has no condition")
	}
	condition, err := f.lowerExpr(stmt.Expr)
	if err != nil {
		return err
	}
	origin := f.block().ID
	before := cloneLocals(f.locals)

	thenID := f.newBlockID()
	continueID := f.newBlockID()
	elseID := continueID
	if len(stmt.Else) != 0 {
		elseID = f.newBlockID()
	}
	if err := f.terminate(hir.BranchTerm{Condition: condition, Then: thenID, Else: elseID}); err != nil {
		return err
	}

	states := make([]localState, 0, 2)
	f.locals = cloneLocals(before)
	f.startBlock(thenID)
	if err := f.lowerStatements(stmt.Then); err != nil {
		return err
	}
	if !f.terminated {
		pred := f.block().ID
		state := cloneLocals(f.locals)
		if err := f.terminate(hir.JumpTerm{Target: continueID}); err != nil {
			return err
		}
		states = append(states, localState{pred: pred, locals: state})
	}

	if len(stmt.Else) != 0 {
		f.locals = cloneLocals(before)
		f.startBlock(elseID)
		if err := f.lowerStatements(stmt.Else); err != nil {
			return err
		}
		if !f.terminated {
			pred := f.block().ID
			state := cloneLocals(f.locals)
			if err := f.terminate(hir.JumpTerm{Target: continueID}); err != nil {
				return err
			}
			states = append(states, localState{pred: pred, locals: state})
		}
	} else {
		states = append(states, localState{pred: origin, locals: cloneLocals(before)})
	}

	if len(states) == 0 {
		f.locals = cloneLocals(before)
		f.terminated = true
		return nil
	}
	f.startBlock(continueID)
	return f.mergeLocals(states)
}

func (f *functionLowerer) mergeLocals(states []localState) error {
	if len(states) == 0 {
		return nil
	}
	keys := make([]frontend.SymbolID, 0, len(states[0].locals))
	for symbol := range states[0].locals {
		keys = append(keys, symbol)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	merged := make(map[frontend.SymbolID]hir.ValueID)
	for _, symbol := range keys {
		first := states[0].locals[symbol]
		incoming := make([]hir.PhiIncoming, 0, len(states))
		allSame, present := true, true
		for _, state := range states {
			value, ok := state.locals[symbol]
			if !ok {
				present = false
				break
			}
			if value != first {
				allSame = false
			}
			incoming = append(incoming, hir.PhiIncoming{Block: state.pred, Value: value})
		}
		if !present {
			continue
		}
		if allSame {
			merged[symbol] = first
			continue
		}
		typeID, err := f.symbolType(symbol)
		if err != nil {
			return err
		}
		merged[symbol] = f.emit(typeID, hir.PhiOp{Incoming: incoming})
	}
	f.locals = merged
	return nil
}

func (f *functionLowerer) lowerLoop(condition *frontend.Expr, body, update []frontend.Statement) error {
	if condition == nil {
		return fmt.Errorf("native loop requires a condition")
	}
	preheader := f.block().ID
	before := cloneLocals(f.locals)
	mutated := map[frontend.SymbolID]struct{}{}
	collectAssigned(body, mutated)
	collectAssigned(update, mutated)

	headerID := f.newBlockID()
	bodyID := f.newBlockID()
	exitID := f.newBlockID()
	if err := f.terminate(hir.JumpTerm{Target: headerID}); err != nil {
		return err
	}

	f.locals = cloneLocals(before)
	f.startBlock(headerID)
	headerIndex := f.current
	keys := make([]frontend.SymbolID, 0, len(mutated))
	for symbol := range mutated {
		if _, ok := before[symbol]; ok {
			keys = append(keys, symbol)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	phis := make([]loopPhi, 0, len(keys))
	for _, symbol := range keys {
		typeID, err := f.symbolType(symbol)
		if err != nil {
			return err
		}
		initial := before[symbol]
		value := f.emit(typeID, hir.PhiOp{Incoming: []hir.PhiIncoming{{Block: preheader, Value: initial}}})
		f.locals[symbol] = value
		phis = append(phis, loopPhi{symbol: symbol, instIndex: len(f.block().Instructions) - 1})
	}
	headerLocals := cloneLocals(f.locals)
	conditionValue, err := f.lowerExpr(condition)
	if err != nil {
		return err
	}
	if err := f.terminate(hir.BranchTerm{Condition: conditionValue, Then: bodyID, Else: exitID}); err != nil {
		return err
	}

	f.locals = cloneLocals(headerLocals)
	f.startBlock(bodyID)
	if err := f.lowerStatements(body); err != nil {
		return err
	}
	if !f.terminated {
		if err := f.lowerStatements(update); err != nil {
			return err
		}
		backPred := f.block().ID
		backLocals := cloneLocals(f.locals)
		if err := f.terminate(hir.JumpTerm{Target: headerID}); err != nil {
			return err
		}
		for _, ref := range phis {
			backValue, ok := backLocals[ref.symbol]
			if !ok {
				return fmt.Errorf("loop lost value for symbol s%d", ref.symbol)
			}
			inst := &f.result.Blocks[headerIndex].Instructions[ref.instIndex]
			phi := inst.Op.(hir.PhiOp)
			phi.Incoming = append(phi.Incoming, hir.PhiIncoming{Block: backPred, Value: backValue})
			inst.Op = phi
		}
	}

	f.locals = cloneLocals(headerLocals)
	f.startBlock(exitID)
	return nil
}

func (f *functionLowerer) symbolType(symbol frontend.SymbolID) (frontend.TypeID, error) {
	if int(symbol) >= len(f.module.source.Symbols) {
		return 0, fmt.Errorf("symbol s%d is out of range", symbol)
	}
	return f.module.source.Symbols[symbol].Type, nil
}

func cloneLocals(source map[frontend.SymbolID]hir.ValueID) map[frontend.SymbolID]hir.ValueID {
	result := make(map[frontend.SymbolID]hir.ValueID, len(source))
	for symbol, value := range source {
		result[symbol] = value
	}
	return result
}

func collectAssigned(statements []frontend.Statement, result map[frontend.SymbolID]struct{}) {
	for _, stmt := range statements {
		switch stmt.Kind {
		case frontend.StmtAssign:
			result[stmt.Symbol] = struct{}{}
		case frontend.StmtBlock:
			collectAssigned(stmt.Then, result)
		case frontend.StmtIf:
			collectAssigned(stmt.Then, result)
			collectAssigned(stmt.Else, result)
		case frontend.StmtWhile:
			collectAssigned(stmt.Then, result)
		case frontend.StmtFor:
			collectAssigned(stmt.Init, result)
			collectAssigned(stmt.Then, result)
			collectAssigned(stmt.Update, result)
		}
	}
}
