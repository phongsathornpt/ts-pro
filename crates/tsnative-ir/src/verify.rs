use std::collections::HashSet;

use thiserror::Error;

use crate::{
    BlockId, FunctionId, HirFunction, HirInstructionKind, HirModule, HirTerminator, TypeId, ValueId,
};

#[derive(Debug, Clone, PartialEq, Eq, Error)]
pub enum VerifyError {
    #[error("duplicate function id {0:?}")]
    DuplicateFunction(FunctionId),
    #[error("function {function:?} has duplicate block id {block:?}")]
    DuplicateBlock {
        function: FunctionId,
        block: BlockId,
    },
    #[error("function {function:?} has duplicate value id {value:?}")]
    DuplicateValue {
        function: FunctionId,
        value: ValueId,
    },
    #[error("function {function:?} references invalid type id {type_id:?}")]
    InvalidType {
        function: FunctionId,
        type_id: TypeId,
    },
    #[error("function {function:?} has missing entry block {block:?}")]
    MissingEntryBlock {
        function: FunctionId,
        block: BlockId,
    },
    #[error("function {function:?} references unknown block {block:?}")]
    UnknownBlock {
        function: FunctionId,
        block: BlockId,
    },
    #[error("function {function:?} references unknown value {value:?}")]
    UnknownValue {
        function: FunctionId,
        value: ValueId,
    },
    #[error("function {function:?} calls unknown function {callee:?}")]
    UnknownFunction {
        function: FunctionId,
        callee: FunctionId,
    },
}

impl HirModule {
    pub fn verify(&self) -> Result<(), Vec<VerifyError>> {
        verify_module(self)
    }
}

pub fn verify_module(module: &HirModule) -> Result<(), Vec<VerifyError>> {
    let mut errors = Vec::new();
    let mut function_ids = HashSet::new();

    for function in &module.functions {
        if !function_ids.insert(function.id) {
            errors.push(VerifyError::DuplicateFunction(function.id));
        }
    }

    for function in &module.functions {
        verify_function(module, function, &function_ids, &mut errors);
    }

    if errors.is_empty() {
        Ok(())
    } else {
        Err(errors)
    }
}

fn verify_function(
    module: &HirModule,
    function: &HirFunction,
    function_ids: &HashSet<FunctionId>,
    errors: &mut Vec<VerifyError>,
) {
    verify_type(module, function.id, function.return_type, errors);

    let mut block_ids = HashSet::new();
    for block in &function.blocks {
        if !block_ids.insert(block.id) {
            errors.push(VerifyError::DuplicateBlock {
                function: function.id,
                block: block.id,
            });
        }
    }

    if !block_ids.contains(&function.entry) {
        errors.push(VerifyError::MissingEntryBlock {
            function: function.id,
            block: function.entry,
        });
    }

    let mut values = HashSet::new();
    for param in &function.params {
        verify_type(module, function.id, param.semantic_type, errors);
        insert_value(function.id, param.value, &mut values, errors);
    }

    for block in &function.blocks {
        for instruction in &block.instructions {
            verify_type(module, function.id, instruction.semantic_type, errors);
            insert_value(function.id, instruction.result, &mut values, errors);
        }
    }

    for block in &function.blocks {
        for instruction in &block.instructions {
            verify_instruction(
                function.id,
                &instruction.kind,
                &values,
                function_ids,
                errors,
            );
        }
        verify_terminator(function.id, &block.terminator, &values, &block_ids, errors);
    }
}

fn verify_type(
    module: &HirModule,
    function: FunctionId,
    type_id: TypeId,
    errors: &mut Vec<VerifyError>,
) {
    if type_id.0 as usize >= module.types.len() {
        errors.push(VerifyError::InvalidType { function, type_id });
    }
}

fn insert_value(
    function: FunctionId,
    value: ValueId,
    values: &mut HashSet<ValueId>,
    errors: &mut Vec<VerifyError>,
) {
    if !values.insert(value) {
        errors.push(VerifyError::DuplicateValue { function, value });
    }
}

fn verify_value(
    function: FunctionId,
    value: ValueId,
    values: &HashSet<ValueId>,
    errors: &mut Vec<VerifyError>,
) {
    if !values.contains(&value) {
        errors.push(VerifyError::UnknownValue { function, value });
    }
}

fn verify_instruction(
    function: FunctionId,
    kind: &HirInstructionKind,
    values: &HashSet<ValueId>,
    functions: &HashSet<FunctionId>,
    errors: &mut Vec<VerifyError>,
) {
    match kind {
        HirInstructionKind::Const(_) => {}
        HirInstructionKind::Unary { operand, .. } => {
            verify_value(function, *operand, values, errors);
        }
        HirInstructionKind::Binary { left, right, .. } => {
            verify_value(function, *left, values, errors);
            verify_value(function, *right, values, errors);
        }
        HirInstructionKind::Call { callee, args } => {
            if !functions.contains(callee) {
                errors.push(VerifyError::UnknownFunction {
                    function,
                    callee: *callee,
                });
            }
            for arg in args {
                verify_value(function, *arg, values, errors);
            }
        }
    }
}

fn verify_terminator(
    function: FunctionId,
    terminator: &HirTerminator,
    values: &HashSet<ValueId>,
    blocks: &HashSet<BlockId>,
    errors: &mut Vec<VerifyError>,
) {
    match terminator {
        HirTerminator::Return(value) => {
            if let Some(value) = value {
                verify_value(function, *value, values, errors);
            }
        }
        HirTerminator::Jump(target) => {
            verify_block(function, *target, blocks, errors);
        }
        HirTerminator::Branch {
            condition,
            then_block,
            else_block,
        } => {
            verify_value(function, *condition, values, errors);
            verify_block(function, *then_block, blocks, errors);
            verify_block(function, *else_block, blocks, errors);
        }
    }
}

fn verify_block(
    function: FunctionId,
    block: BlockId,
    blocks: &HashSet<BlockId>,
    errors: &mut Vec<VerifyError>,
) {
    if !blocks.contains(&block) {
        errors.push(VerifyError::UnknownBlock { function, block });
    }
}
