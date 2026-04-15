use crate::persistence::logs::{LogEntry, LogStore};
use std::sync::Arc;
use tauri::State;

#[tauri::command]
pub async fn workspace_logs_list(
    log_store: State<'_, Arc<LogStore>>,
    workspace_id: String,
) -> Result<Vec<LogEntry>, String> {
    log_store
        .list_logs(&workspace_id)
        .await
        .map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn workspace_log_read(
    log_store: State<'_, Arc<LogStore>>,
    workspace_id: String,
    filename: String,
) -> Result<String, String> {
    log_store
        .read_log(&workspace_id, &filename)
        .await
        .map_err(|e| e.to_string())
}
