use serde::{Deserialize, Serialize};

macro_rules! id_type {
    ($name:ident) => {
        #[derive(
            Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Serialize, Deserialize,
        )]
        pub struct $name(pub u32);

        impl $name {
            pub const fn new(value: u32) -> Self {
                Self(value)
            }
        }
    };
}

id_type!(ModuleId);
id_type!(FunctionId);
id_type!(BlockId);
id_type!(ValueId);
id_type!(TypeId);
id_type!(ShapeId);
