mod hir;
mod ids;
mod types;
mod verify;

pub use hir::{
    BinaryOp, HirBlock, HirFunction, HirInstruction, HirInstructionKind, HirLiteral, HirModule,
    HirParam, HirTerminator, UnaryOp,
};
pub use ids::{BlockId, FunctionId, ModuleId, ShapeId, TypeId, ValueId};
pub use types::{Repr, SemanticType};
pub use verify::{VerifyError, verify_module};

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

    #[test]
    fn module_keeps_semantic_types_separate_from_function_repr() {
        let module = identity_module();
        assert_eq!(module.types[0], SemanticType::Number);
        assert_eq!(module.functions[0].return_repr, Some(Repr::F64));
        assert_eq!(module.verify(), Ok(()));
    }

    #[test]
    fn verifier_rejects_missing_entry_block() {
        let mut module = identity_module();
        module.functions[0].entry = BlockId::new(99);

        assert_eq!(
            module.verify(),
            Err(vec![VerifyError::MissingEntryBlock {
                function: FunctionId::new(0),
                block: BlockId::new(99),
            }])
        );
    }

    fn identity_module() -> HirModule {
        HirModule {
            id: ModuleId::new(0),
            name: "math".to_owned(),
            types: vec![SemanticType::Number],
            functions: vec![HirFunction {
                id: FunctionId::new(0),
                name: "identity".to_owned(),
                params: vec![HirParam {
                    value: ValueId::new(0),
                    name: "x".to_owned(),
                    semantic_type: TypeId::new(0),
                    repr: Some(Repr::F64),
                }],
                return_type: TypeId::new(0),
                return_repr: Some(Repr::F64),
                entry: BlockId::new(0),
                blocks: vec![HirBlock {
                    id: BlockId::new(0),
                    instructions: vec![],
                    terminator: HirTerminator::Return(Some(ValueId::new(0))),
                }],
            }],
        }
    }
}
