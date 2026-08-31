use std::path::PathBuf;

use anyhow::{Result, bail};
use clap::{Parser, Subcommand};
use tsnative_compiler::TypeScriptFrontend;

#[derive(Debug, Parser)]
#[command(
    name = "tsnative",
    version,
    about = "TypeScript 7 to native binary compiler"
)]
struct Cli {
    #[arg(long, default_value = ".", global = true)]
    project_root: PathBuf,

    #[command(subcommand)]
    command: Command,
}

#[derive(Debug, Subcommand)]
enum Command {
    /// Verify the TypeScript 7 frontend toolchain.
    Doctor,
    /// Run TypeScript 7 semantic/type checking.
    Check {
        #[arg(short = 'p', long = "project", default_value = "tsconfig.native.json")]
        config: PathBuf,
    },
}
fn main() -> Result<()> {
    let cli = Cli::parse();
    let frontend = TypeScriptFrontend::discover(&cli.project_root)?;

    match cli.command {
        Command::Doctor => {
            println!("TypeScript {}", frontend.version()?);
            println!("project root: {}", frontend.project_root().display());
            println!("frontend: ready");
        }
        Command::Check { config } => {
            let result = frontend.check_project(&config)?;
            print!("{}", result.stdout);
            eprint!("{}", result.stderr);

            if !result.success {
                bail!("TypeScript 7 check failed");
            }

            println!("TypeScript 7 check passed");
        }
    }

    Ok(())
}
