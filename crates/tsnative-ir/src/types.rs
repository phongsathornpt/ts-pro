use serde::{Deserialize, Serialize};

use crate::{ShapeId, TypeId};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub enum SemanticType {
    Any,
    Unknown,
    Never,
    Void,
    Undefined,
    Null,
    Boolean,
    Number,
    String,
    Object(ShapeId),
    Array(TypeId),
    Union(Vec<TypeId>),
    Function {
        params: Vec<TypeId>,
        return_type: TypeId,
    },
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub enum Repr {
    Void,
    Bool,
    I32,
    I64,
    F64,
    StringRef,
    ArrayRef,
    ObjectRef(ShapeId),
    FunctionRef,
    TaggedUnion,
    JsValue,
}
