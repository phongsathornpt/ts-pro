package lower

import (
	"encoding/binary"
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
)

func finalizeAMD64(e *amd64.Emitter, fnOffsets map[string]int, strFixups []stringFixupAMD64, closureCodeFixups []closureCodeFixupAMD64, callFixups []callFixup, branchFixups []branchFixupAMD64, bbOffsets map[*ir.BasicBlock]int) ([]byte, error) {
	// Emit String Constants Table
	strOffsets := make(map[string]int)
	for _, sf := range strFixups {
		if _, exists := strOffsets[sf.str]; !exists {
			for len(e.Code)%8 != 0 {
				e.Code = append(e.Code, 0)
			}
			strOffsets[sf.str] = len(e.Code)

			var lenBuf [8]byte
			binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(sf.str)))
			e.Code = append(e.Code, lenBuf[:]...)
			e.Code = append(e.Code, []byte(sf.str)...)
			e.Code = append(e.Code, 0)
		}
	}

	// Fix up string LEA instructions
	for _, sf := range strFixups {
		targetAddr := strOffsets[sf.str]
		disp := int32(targetAddr - (sf.offset + 4))
		binary.LittleEndian.PutUint32(e.Code[sf.offset:], uint32(disp))
	}

	// Fix up closure code pointers encoded as RIP-relative LEA instructions.
	for _, cf := range closureCodeFixups {
		targetAddr, exists := fnOffsets[cf.function]
		if !exists {
			return nil, fmt.Errorf("unresolved closure function %q", cf.function)
		}
		disp := int32(targetAddr - (cf.offset + 4))
		binary.LittleEndian.PutUint32(e.Code[cf.offset:], uint32(disp))
	}

	// Fix up function calls
	for _, cf := range callFixups {
		targetAddr, exists := fnOffsets[cf.callee]
		if !exists {
			return nil, fmt.Errorf("unresolved call target %q", cf.callee)
		}
		rel32 := int32(targetAddr - (cf.offset + 5))
		binary.LittleEndian.PutUint32(e.Code[cf.offset+1:], uint32(rel32))
	}

	// Fix up local branches
	for _, bf := range branchFixups {
		targetAddr, exists := bbOffsets[bf.targetBB]
		if !exists {
			return nil, fmt.Errorf("unresolved basic block target %p", bf.targetBB)
		}
		if bf.isCond {
			rel32 := int32(targetAddr - (bf.offset + 6))
			binary.LittleEndian.PutUint32(e.Code[bf.offset+2:], uint32(rel32))
		} else {
			rel32 := int32(targetAddr - (bf.offset + 5))
			binary.LittleEndian.PutUint32(e.Code[bf.offset+1:], uint32(rel32))
		}
	}
	return e.Code, nil
}
