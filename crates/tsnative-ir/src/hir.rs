use serde::{Deserialize, Serialize};

use crate::{BlockId, FunctionId, ModuleId, Repr, SemanticType, TypeId, ValueId};

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct HirModule {
    pub id: ModuleId,
    pub name: String,
    pub types: Vec<SemanticType>,
    pub functions: Vec<HirFunction>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct HirFunction {
    pub id: FunctionId,
    pub name: String,
    pub params: Vec<HirParam>,
    pub return_type: TypeId,
    pub return_repr: Option<Repr>,
    pub entry: BlockId,
    pub blocks: Vec<HirBlock>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct HirParam {
    pub value: ValueId,
    pub name: String,
    pub semantic_type: TypeId,
    pub repr: Option<Repr>,
}
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct HirBlock {
    pub id: BlockId,
    pub instructions: Vec<HirInstruction>,
    pub terminator: HirTerminator,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct HirInstruction {
    pub result: ValueId,
    pub semantic_type: TypeId,
    pub repr: Option<Repr>,
    pub kind: HirInstructionKind,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub enum HirInstructionKind {
    Const(HirLiteral),
    Unary {
        op: UnaryOp,
        operand: ValueId,
    },
    Binary {
        op: BinaryOp,
        left: ValueId,
        right: ValueId,
    },
    Call {
        callee: FunctionId,
        args: Vec<ValueId>,
    },
}
