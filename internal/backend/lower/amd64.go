package lower

import (
	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
)

func lowerAMD64(prog *ir.Program) ([]byte, error) {
	e := amd64.NewEmitter()

	fnOffsets := make(map[string]int)
	var callFixups []callFixup
	var branchFixups []branchFixupAMD64
	var strFixups []stringFixupAMD64
	var closureCodeFixups []closureCodeFixupAMD64
	bbOffsets := make(map[*ir.BasicBlock]int)

	// Linux process entry is not a normal function call. Emit an explicit
	// startup stub so generated functions can use ordinary SysV call/return.
	hasMain := false
	for _, fn := range prog.Functions {
		if fn.Name == "@main" {
			hasMain = true
			break
		}
	}
	fnOffsets["_start"] = len(e.Code)
	// Reserve a small runtime context on the process stack. R15 is callee-saved
	// by SysV and deliberately excluded from the program register allocator.
	e.SubRegImm32(amd64.RSP, 256)
	e.MovRegReg(amd64.R15, amd64.RSP)
	initOffset := len(e.Code)
	e.CallRel32(0)
	callFixups = append(callFixups, callFixup{offset: initOffset, callee: "ts_runtime_init"})
	if hasMain {
		callOffset := len(e.Code)
		e.CallRel32(0)
		callFixups = append(callFixups, callFixup{offset: callOffset, callee: "@main"})
	}
	drainOffset := len(e.Code)
	e.CallRel32(0)
	callFixups = append(callFixups, callFixup{offset: drainOffset, callee: "ts_task_drain"})
	exitOffset := len(e.Code)
	e.CallRel32(0)
	callFixups = append(callFixups, callFixup{offset: exitOffset, callee: "ts_sys_exit"})

	if err := lowerAMD64Functions(e, prog, fnOffsets, &callFixups, &branchFixups, &strFixups, &closureCodeFixups, bbOffsets); err != nil {
		return nil, err
	}

	fnOffsets["ts_print_val"] = len(e.Code)
	emitAMD64PrintValV2(e)

	fnOffsets["ts_print_true"] = len(e.Code)
	emitAMD64PrintLiteral(e, "true\n")
	fnOffsets["ts_print_false"] = len(e.Code)
	emitAMD64PrintLiteral(e, "false\n")
	fnOffsets["ts_print_bool"] = len(e.Code)
	emitAMD64PrintBool(e, fnOffsets["ts_print_true"], fnOffsets["ts_print_false"])
	fnOffsets["ts_print_undefined"] = len(e.Code)
	emitAMD64PrintLiteral(e, "undefined\n")
	fnOffsets["ts_print_null"] = len(e.Code)
	emitAMD64PrintLiteral(e, "null\n")

	fnOffsets["ts_print_str"] = len(e.Code)
	emitAMD64PrintStr(e)
	fnOffsets["ts_print_object"] = len(e.Code)
	emitAMD64PrintLiteral(e, "[object Object]\n")

	fnOffsets["ts_js_box_number"] = len(e.Code)
	emitAMD64JSBoxNumber(e)
	fnOffsets["ts_js_box_bool"] = len(e.Code)
	emitAMD64JSBoxBool(e)
	fnOffsets["ts_js_box_string"] = len(e.Code)
	emitAMD64JSBoxString(e)
	fnOffsets["ts_js_box_ref"] = len(e.Code)
	emitAMD64JSBoxRef(e)
	fnOffsets["ts_js_unbox_number"] = len(e.Code)
	emitAMD64JSUnboxNumber(e)
	fnOffsets["ts_js_unbox_bool"] = len(e.Code)
	emitAMD64JSUnboxBool(e)
	fnOffsets["ts_js_unbox_string"] = len(e.Code)
	emitAMD64JSUnboxString(e)
	fnOffsets["ts_js_unbox_ref"] = len(e.Code)
	emitAMD64JSUnboxRef(e)
	fnOffsets["ts_js_to_bool"] = len(e.Code)
	emitAMD64JSToBool(e)
	fnOffsets["ts_js_print"] = len(e.Code)
	emitAMD64JSPrint(e, fnOffsets["ts_print_val"], fnOffsets["ts_print_str"], fnOffsets["ts_print_undefined"], fnOffsets["ts_print_null"], fnOffsets["ts_print_object"], fnOffsets["ts_print_true"], fnOffsets["ts_print_false"])
	fnOffsets["ts_json_parse_scalar"] = len(e.Code)
	emitAMD64JSONParseScalar(e)
	fnOffsets["ts_js_string_to_number"] = len(e.Code)
	emitAMD64JSStringToNumber(e, fnOffsets["ts_json_parse_scalar"])

	emitAMD64RuntimeSymbols(e, fnOffsets)
	emitAMD64IntervalRuntimeSymbols(e, fnOffsets)
	emitAMD64SHA2RuntimeSymbols(e, fnOffsets)
	emitAMD64SHA1RuntimeSymbols(e, fnOffsets)

	return finalizeAMD64(e, fnOffsets, strFixups, closureCodeFixups, callFixups, branchFixups, bbOffsets)
}
