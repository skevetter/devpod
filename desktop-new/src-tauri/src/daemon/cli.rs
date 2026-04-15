use serde::de::DeserializeOwned;
use std::env;
use std::path::PathBuf;
use thiserror::Error;
use tokio::process::Command;
use tokio::sync::mpsc;

#[derive(Error, Debug)]
pub enum CliError {
    #[error("devpod binary not found at {0}")]
    BinaryNotFound(PathBuf),
    #[error("command failed with exit code {code}: {stderr}")]
    CommandFailed { code: i32, stderr: String },
    #[error("failed to parse JSON output: {0}")]
    ParseError(#[from] serde_json::Error),
    #[error("IO error: {0}")]
    IoError(#[from] std::io::Error),
}

#[derive(Debug, Clone)]
pub enum OutputLine {
    Stdout(String),
    Stderr(String),
    Exit(i32),
}

pub struct CliRunner {
    binary_path: PathBuf,
}

impl CliRunner {
    pub fn new(binary_path: PathBuf) -> Result<Self, CliError> {
        if !binary_path.exists() {
            return Err(CliError::BinaryNotFound(binary_path));
        }
        Ok(Self { binary_path })
    }

    pub fn binary_path(&self) -> &PathBuf {
        &self.binary_path
    }

    pub async fn run<T: DeserializeOwned>(&self, args: &[&str]) -> Result<T, CliError> {
        let mut cmd_args: Vec<&str> = args.to_vec();
        cmd_args.push("--output");
        cmd_args.push("json");

        let output = Command::new(&self.binary_path)
            .args(&cmd_args)
            .output()
            .await?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            return Err(CliError::CommandFailed {
                code: output.status.code().unwrap_or(-1),
                stderr,
            });
        }

        let parsed: T = serde_json::from_slice(&output.stdout)?;
        Ok(parsed)
    }

    pub async fn run_raw(&self, args: &[&str]) -> Result<String, CliError> {
        let output = Command::new(&self.binary_path)
            .args(args)
            .output()
            .await?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            return Err(CliError::CommandFailed {
                code: output.status.code().unwrap_or(-1),
                stderr,
            });
        }

        Ok(String::from_utf8_lossy(&output.stdout).to_string())
    }

    pub fn run_streaming(
        &self,
        args: &[&str],
        tx: mpsc::Sender<OutputLine>,
    ) -> tokio::task::JoinHandle<Result<i32, CliError>> {
        use tokio::io::{AsyncBufReadExt, BufReader};

        let binary_path = self.binary_path.clone();
        let args: Vec<String> = args.iter().map(|s| s.to_string()).collect();

        tokio::spawn(async move {
            let mut child = Command::new(&binary_path)
                .args(&args)
                .stdout(std::process::Stdio::piped())
                .stderr(std::process::Stdio::piped())
                .spawn()?;

            let stdout = child.stdout.take().unwrap();
            let stderr = child.stderr.take().unwrap();

            let tx_out = tx.clone();
            let stdout_handle = tokio::spawn(async move {
                let reader = BufReader::new(stdout);
                let mut lines = reader.lines();
                while let Ok(Some(line)) = lines.next_line().await {
                    let _ = tx_out.send(OutputLine::Stdout(line)).await;
                }
            });

            let tx_err = tx.clone();
            let stderr_handle = tokio::spawn(async move {
                let reader = BufReader::new(stderr);
                let mut lines = reader.lines();
                while let Ok(Some(line)) = lines.next_line().await {
                    let _ = tx_err.send(OutputLine::Stderr(line)).await;
                }
            });

            let _ = stdout_handle.await;
            let _ = stderr_handle.await;

            let status = child.wait().await?;
            let code = status.code().unwrap_or(-1);
            let _ = tx.send(OutputLine::Exit(code)).await;

            Ok(code)
        })
    }
}

pub fn resolve_binary_path(explicit: Option<PathBuf>) -> Result<PathBuf, CliError> {
    if let Some(path) = explicit {
        if path.exists() {
            return Ok(path);
        }
        return Err(CliError::BinaryNotFound(path));
    }

    if let Ok(exe_path) = env::current_exe() {
        if let Some(exe_dir) = exe_path.parent() {
            let candidate = exe_dir.join("devpod");
            if candidate.exists() {
                return Ok(candidate);
            }
            let candidate_exe = exe_dir.join("devpod.exe");
            if candidate_exe.exists() {
                return Ok(candidate_exe);
            }
        }
    }

    let name = if cfg!(windows) { "devpod.exe" } else { "devpod" };
    if let Ok(path) = which::which(name) {
        return Ok(path);
    }

    Err(CliError::BinaryNotFound(PathBuf::from("devpod")))
}
