package lowering

import (
	"fmt"

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

func (l *moduleLowerer) lowerFunction(source frontend.Function) (hir.Function, error) {
	fl := &functionLowerer{
		module: l,
		source: source,
		locals: map[frontend.SymbolID]hir.ValueID{},
		result: hir.Function{
			ID:         hir.NewFunctionID(uint32(source.ID)),
			Name:       source.Name,
			ReturnType: l.types[source.ReturnType],
			Entry:      hir.NewBlockID(0),
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

func (f *functionLowerer) block() *hir.Block {
	return &f.result.Blocks[f.current]
}

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
	thenID := f.newBlockID()
	continueID := f.newBlockID()
	elseID := continueID
	if len(stmt.Else) != 0 {
		elseID = f.newBlockID()
	}
	if err := f.terminate(hir.BranchTerm{Condition: condition, Then: thenID, Else: elseID}); err != nil {
		return err
	}

	f.startBlock(thenID)
	if err := f.lowerStatements(stmt.Then); err != nil {
		return err
	}
	if !f.terminated {
		if err := f.terminate(hir.JumpTerm{Target: continueID}); err != nil {
			return err
		}
	}

	if len(stmt.Else) != 0 {
		f.startBlock(elseID)
		if err := f.lowerStatements(stmt.Else); err != nil {
			return err
		}
		if !f.terminated {
			if err := f.terminate(hir.JumpTerm{Target: continueID}); err != nil {
				return err
			}
		}
	}

	f.startBlock(continueID)
	return nil
}
