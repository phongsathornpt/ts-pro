use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::{Context, Result, bail};

const REQUIRED_TYPESCRIPT_MAJOR: u64 = 7;

#[derive(Debug, Clone)]
pub struct TypeScriptFrontend {
    project_root: PathBuf,
    tsc_path: PathBuf,
}

#[derive(Debug)]
pub struct CheckResult {
    pub success: bool,
    pub stdout: String,
    pub stderr: String,
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

    pub fn version(&self) -> Result<String> {
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
    pub fn check_project(&self, config: impl AsRef<Path>) -> Result<CheckResult> {
        let config = config.as_ref();
        let output = Command::new(&self.tsc_path)
            .args(["--pretty", "false", "--noEmit", "-p"])
            .arg(config)
            .current_dir(&self.project_root)
            .output()
            .with_context(|| format!("failed to check {}", config.display()))?;

        Ok(CheckResult {
            success: output.status.success(),
            stdout: String::from_utf8_lossy(&output.stdout).into_owned(),
            stderr: String::from_utf8_lossy(&output.stderr).into_owned(),
        })
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
