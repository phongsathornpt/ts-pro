mod hir;
mod ids;
mod types;

pub use hir::{
    BinaryOp, HirBlock, HirFunction, HirInstruction, HirInstructionKind, HirLiteral, HirModule,
    HirParam, HirTerminator,
};
pub use ids::{BlockId, FunctionId, ModuleId, ShapeId, TypeId, ValueId};
pub use types::{Repr, SemanticType};

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn semantic_type_and_runtime_repr_are_separate() {
        let semantic = SemanticType::Number;
        let repr = Repr::F64;

        assert_eq!(semantic, SemanticType::Number);
        assert_eq!(repr, Repr::F64);
    }
}
