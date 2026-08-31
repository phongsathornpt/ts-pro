use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::{Context, Result, bail};

use super::{CheckResult, Diagnostic, DiagnosticLevel, Frontend};

const REQUIRED_TYPESCRIPT_MAJOR: u64 = 7;

#[derive(Debug, Clone)]
pub struct TypeScriptFrontend {
    project_root: PathBuf,
    tsc_path: PathBuf,
}

impl TypeScriptFrontend {
    pub fn discover(project_root: impl Into<PathBuf>) -> Result<Self> {
        let project_root = project_root.into();
        let tsc_path = project_root.join("node_modules/.bin/tsc");

        if !tsc_path.is_file() {
            bail!("TypeScript 7 CLI not found at {}", tsc_path.display());
        }

        let frontend = Self {
            project_root,
            tsc_path,
        };
        frontend.ensure_typescript_7()?;
        Ok(frontend)
    }
    pub fn project_root(&self) -> &Path {
        &self.project_root
    }

    fn ensure_typescript_7(&self) -> Result<()> {
        let version = self.version()?;
        let major = version
            .split('.')
            .next()
            .and_then(|part| part.parse::<u64>().ok())
            .context("unable to parse TypeScript version")?;

        if major != REQUIRED_TYPESCRIPT_MAJOR {
            bail!("TypeScript 7 is required, found {version}");
        }

        Ok(())
    }
}

impl Frontend for TypeScriptFrontend {
    fn version(&self) -> Result<String> {
        let output = Command::new(&self.tsc_path)
            .arg("--version")
            .current_dir(&self.project_root)
            .output()
            .context("failed to execute TypeScript CLI")?;
        if !output.status.success() {
            bail!("TypeScript CLI --version failed");
        }

        let raw = String::from_utf8_lossy(&output.stdout);
        Ok(raw.trim().trim_start_matches("Version ").to_owned())
    }

    fn check_project(&self, config: &Path) -> Result<CheckResult> {
        let output = Command::new(&self.tsc_path)
            .args(["--pretty", "false", "--noEmit", "-p"])
            .arg(config)
            .current_dir(&self.project_root)
            .output()
            .with_context(|| format!("failed to check {}", config.display()))?;

        let raw_stdout = String::from_utf8_lossy(&output.stdout).into_owned();
        let raw_stderr = String::from_utf8_lossy(&output.stderr).into_owned();
        let diagnostics = raw_stdout
            .lines()
            .chain(raw_stderr.lines())
            .filter_map(parse_diagnostic)
            .collect();

        Ok(CheckResult {
            success: output.status.success(),
            diagnostics,
            raw_stdout,
            raw_stderr,
        })
    }
}
fn parse_diagnostic(line: &str) -> Option<Diagnostic> {
    if line.starts_with("error TS") || line.starts_with("warning TS") {
        let (head, message) = line.split_once(": ")?;
        let (level, code) = parse_level_and_code(head)?;
        return Some(Diagnostic {
            level,
            code,
            file: None,
            line: None,
            column: None,
            message: message.to_owned(),
        });
    }

    let (location, rest) = line.split_once(": ")?;
    let (head, message) = rest.split_once(": ")?;
    let (level, code) = parse_level_and_code(head)?;
    let (file, line, column) = parse_location(location);

    Some(Diagnostic {
        level,
        code,
        file,
        line,
        column,
        message: message.to_owned(),
    })
}
fn parse_level_and_code(head: &str) -> Option<(DiagnosticLevel, Option<String>)> {
    let mut parts = head.split_whitespace();
    let level = match parts.next()? {
        "error" => DiagnosticLevel::Error,
        "warning" => DiagnosticLevel::Warning,
        _ => return None,
    };
    let code = parts.next().map(ToOwned::to_owned);
    Some((level, code))
}

fn parse_location(location: &str) -> (Option<PathBuf>, Option<u32>, Option<u32>) {
    let Some(open) = location.rfind('(') else {
        return (Some(PathBuf::from(location)), None, None);
    };
    let Some(coords) = location[open + 1..].strip_suffix(')') else {
        return (Some(PathBuf::from(location)), None, None);
    };
    let Some((line, column)) = coords.split_once(',') else {
        return (Some(PathBuf::from(location)), None, None);
    };

    (
        Some(PathBuf::from(&location[..open])),
        line.parse().ok(),
        column.parse().ok(),
    )
}
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_file_diagnostic() {
        let diagnostic = parse_diagnostic(
            "examples/bad.ts(3,7): error TS2322: Type 'string' is not assignable to type 'number'.",
        )
        .expect("diagnostic");

        assert_eq!(diagnostic.level, DiagnosticLevel::Error);
        assert_eq!(diagnostic.code.as_deref(), Some("TS2322"));
        assert_eq!(diagnostic.file, Some(PathBuf::from("examples/bad.ts")));
        assert_eq!(diagnostic.line, Some(3));
        assert_eq!(diagnostic.column, Some(7));
    }

    #[test]
    fn parses_project_diagnostic() {
        let diagnostic =
            parse_diagnostic("error TS18003: No inputs were found.").expect("diagnostic");

        assert_eq!(diagnostic.code.as_deref(), Some("TS18003"));
        assert_eq!(diagnostic.file, None);
    }
}
