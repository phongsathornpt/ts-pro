use std::path::{Path, PathBuf};

use anyhow::Result;

mod typescript;

pub use typescript::TypeScriptFrontend;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DiagnosticLevel {
    Error,
    Warning,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Diagnostic {
    pub level: DiagnosticLevel,
    pub code: Option<String>,
    pub file: Option<PathBuf>,
    pub line: Option<u32>,
    pub column: Option<u32>,
    pub message: String,
}

#[derive(Debug)]
pub struct CheckResult {
    pub success: bool,
    pub diagnostics: Vec<Diagnostic>,
    pub raw_stdout: String,
    pub raw_stderr: String,
}
pub trait Frontend {
    fn version(&self) -> Result<String>;
    fn check_project(&self, config: &Path) -> Result<CheckResult>;
}

impl Diagnostic {
    pub fn render(&self) -> String {
        let location = match (&self.file, self.line, self.column) {
            (Some(file), Some(line), Some(column)) => {
                format!("{}({line},{column})", file.display())
            }
            (Some(file), _, _) => file.display().to_string(),
            _ => "<project>".to_owned(),
        };
        let level = match self.level {
            DiagnosticLevel::Error => "error",
            DiagnosticLevel::Warning => "warning",
        };
        let code = self
            .code
            .as_deref()
            .map(|code| format!(" {code}"))
            .unwrap_or_default();

        format!("{location}: {level}{code}: {}", self.message)
    }
}
