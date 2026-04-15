use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use log::{error, info};
use std::path::{Path, PathBuf};
use tokio::fs;
use tokio::io::AsyncWriteExt;

/// File-based log storage for workspace command output.
/// Stores logs at `~/.devpod/logs/{workspace_id}/{timestamp}.log`
pub struct LogStore {
    base_dir: PathBuf,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct LogEntry {
    pub workspace_id: String,
    pub filename: String,
    pub created_at: String,
    pub size_bytes: u64,
}

impl LogStore {
    pub fn new(base_dir: PathBuf) -> Self {
        Self { base_dir }
    }

    pub fn default_path() -> Result<Self> {
        let home = dirs::home_dir().context("Could not determine home directory")?;
        Ok(Self::new(home.join(".devpod").join("logs")))
    }

    /// Create a new log file for a workspace command and return its path.
    pub async fn create_log_file(&self, workspace_id: &str) -> Result<PathBuf> {
        let dir = self.base_dir.join(workspace_id);
        fs::create_dir_all(&dir).await.with_context(|| {
            format!("Failed to create log directory: {}", dir.display())
        })?;

        let timestamp = Utc::now().format("%Y%m%dT%H%M%SZ");
        let filename = format!("{}.log", timestamp);
        let path = dir.join(&filename);

        fs::File::create(&path).await.with_context(|| {
            format!("Failed to create log file: {}", path.display())
        })?;

        Ok(path)
    }

    /// Append a line to an existing log file.
    pub async fn append_log(path: &Path, line: &str) -> Result<()> {
        let mut file = fs::OpenOptions::new()
            .append(true)
            .open(path)
            .await
            .with_context(|| format!("Failed to open log file: {}", path.display()))?;

        file.write_all(line.as_bytes()).await?;
        file.write_all(b"\n").await?;
        Ok(())
    }

    /// List all log files for a workspace, newest first.
    pub async fn list_logs(&self, workspace_id: &str) -> Result<Vec<LogEntry>> {
        let dir = self.base_dir.join(workspace_id);
        if !dir.exists() {
            return Ok(Vec::new());
        }

        let mut entries = Vec::new();
        let mut read_dir = fs::read_dir(&dir).await?;

        while let Some(entry) = read_dir.next_entry().await? {
            let path = entry.path();
            if path.extension().is_some_and(|ext| ext == "log") {
                let metadata = entry.metadata().await?;
                let filename = entry.file_name().to_string_lossy().to_string();

                let created_at = metadata
                    .created()
                    .ok()
                    .and_then(|t| {
                        let dt: DateTime<Utc> = t.into();
                        Some(dt.to_rfc3339())
                    })
                    .unwrap_or_default();

                entries.push(LogEntry {
                    workspace_id: workspace_id.to_string(),
                    filename,
                    created_at,
                    size_bytes: metadata.len(),
                });
            }
        }

        // Sort newest first
        entries.sort_by(|a, b| b.created_at.cmp(&a.created_at));
        Ok(entries)
    }

    /// Read the full contents of a log file.
    pub async fn read_log(&self, workspace_id: &str, filename: &str) -> Result<String> {
        let path = self.base_dir.join(workspace_id).join(filename);
        fs::read_to_string(&path)
            .await
            .with_context(|| format!("Failed to read log: {}", path.display()))
    }

    /// Remove log files older than `max_age` days.
    pub async fn prune(&self, max_age_days: i64) -> Result<u64> {
        let cutoff = Utc::now() - chrono::Duration::days(max_age_days);
        let mut removed = 0u64;

        if !self.base_dir.exists() {
            return Ok(0);
        }

        let mut workspace_dirs = fs::read_dir(&self.base_dir).await?;
        while let Some(ws_entry) = workspace_dirs.next_entry().await? {
            if !ws_entry.file_type().await?.is_dir() {
                continue;
            }

            let mut log_files = fs::read_dir(ws_entry.path()).await?;
            while let Some(log_entry) = log_files.next_entry().await? {
                let path = log_entry.path();
                if !path.extension().is_some_and(|ext| ext == "log") {
                    continue;
                }

                // Parse timestamp from filename: 20260415T143000Z.log
                let stem = path
                    .file_stem()
                    .and_then(|s| s.to_str())
                    .unwrap_or_default();

                if let Ok(file_time) =
                    chrono::NaiveDateTime::parse_from_str(stem, "%Y%m%dT%H%M%SZ")
                {
                    let file_utc = file_time.and_utc();
                    if file_utc < cutoff {
                        if let Err(e) = fs::remove_file(&path).await {
                            error!("Failed to prune log file {}: {}", path.display(), e);
                        } else {
                            removed += 1;
                        }
                    }
                }
            }
        }

        if removed > 0 {
            info!("Pruned {} old log files", removed);
        }
        Ok(removed)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[tokio::test]
    async fn test_create_and_read_log() {
        let tmp = TempDir::new().unwrap();
        let store = LogStore::new(tmp.path().to_path_buf());

        let path = store.create_log_file("test-ws").await.unwrap();
        LogStore::append_log(&path, "line 1").await.unwrap();
        LogStore::append_log(&path, "line 2").await.unwrap();

        let entries = store.list_logs("test-ws").await.unwrap();
        assert_eq!(entries.len(), 1);

        let content = store
            .read_log("test-ws", &entries[0].filename)
            .await
            .unwrap();
        assert!(content.contains("line 1"));
        assert!(content.contains("line 2"));
    }

    #[tokio::test]
    async fn test_list_logs_empty() {
        let tmp = TempDir::new().unwrap();
        let store = LogStore::new(tmp.path().to_path_buf());
        let entries = store.list_logs("nonexistent").await.unwrap();
        assert!(entries.is_empty());
    }

    #[tokio::test]
    async fn test_prune_keeps_recent() {
        let tmp = TempDir::new().unwrap();
        let store = LogStore::new(tmp.path().to_path_buf());

        let _path = store.create_log_file("test-ws").await.unwrap();

        // Recent file should not be pruned
        let removed = store.prune(30).await.unwrap();
        assert_eq!(removed, 0);

        let entries = store.list_logs("test-ws").await.unwrap();
        assert_eq!(entries.len(), 1);
    }
}
