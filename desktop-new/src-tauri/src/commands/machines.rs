use crate::daemon::cli::CliRunner;
use crate::daemon::state::DaemonState;
use crate::daemon::types::Machine;
use std::sync::Arc;
use tauri::State;
use tokio::sync::RwLock;

type SharedState = Arc<RwLock<DaemonState>>;

#[tauri::command]
pub async fn machine_list(state: State<'_, SharedState>) -> Result<Vec<Machine>, String> {
    let state = state.read().await;
    Ok(state.machine_list().into_iter().cloned().collect())
}

#[tauri::command]
pub async fn machine_create(
    cli: State<'_, Arc<CliRunner>>,
    name: String,
    provider: String,
) -> Result<(), String> {
    cli.run_raw(&["machine", "create", &name, "--provider", &provider])
        .await
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn machine_delete(
    cli: State<'_, Arc<CliRunner>>,
    name: String,
) -> Result<(), String> {
    cli.run_raw(&["machine", "delete", &name])
        .await
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn machine_start(
    cli: State<'_, Arc<CliRunner>>,
    name: String,
) -> Result<(), String> {
    cli.run_raw(&["machine", "start", &name])
        .await
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn machine_stop(
    cli: State<'_, Arc<CliRunner>>,
    name: String,
) -> Result<(), String> {
    cli.run_raw(&["machine", "stop", &name])
        .await
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn machine_status(
    cli: State<'_, Arc<CliRunner>>,
    name: String,
) -> Result<String, String> {
    cli.run_raw(&["machine", "status", &name, "--output", "json"])
        .await
        .map_err(|e| e.to_string())
}
