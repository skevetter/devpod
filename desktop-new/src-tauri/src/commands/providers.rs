use crate::daemon::cli::CliRunner;
use crate::daemon::state::DaemonState;
use crate::daemon::types::Provider;
use std::sync::Arc;
use tauri::State;
use tokio::sync::RwLock;

type SharedState = Arc<RwLock<DaemonState>>;

#[tauri::command]
pub async fn provider_list(state: State<'_, SharedState>) -> Result<Vec<Provider>, String> {
    let state = state.read().await;
    Ok(state.provider_list().into_iter().cloned().collect())
}

#[tauri::command]
pub async fn provider_add(
    cli: State<'_, Arc<CliRunner>>,
    name: String,
) -> Result<(), String> {
    cli.run_raw(&["provider", "add", &name])
        .await
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn provider_delete(
    cli: State<'_, Arc<CliRunner>>,
    name: String,
) -> Result<(), String> {
    cli.run_raw(&["provider", "delete", &name])
        .await
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn provider_use(
    cli: State<'_, Arc<CliRunner>>,
    name: String,
) -> Result<(), String> {
    cli.run_raw(&["provider", "use", &name])
        .await
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn provider_update(
    cli: State<'_, Arc<CliRunner>>,
    name: String,
) -> Result<(), String> {
    cli.run_raw(&["provider", "update", &name])
        .await
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn provider_options(
    cli: State<'_, Arc<CliRunner>>,
    name: String,
) -> Result<serde_json::Value, String> {
    cli.run::<serde_json::Value>(&["provider", "options", &name])
        .await
        .map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn provider_set_options(
    cli: State<'_, Arc<CliRunner>>,
    name: String,
    options: Vec<String>,
) -> Result<(), String> {
    let mut args: Vec<&str> = vec!["provider", "set-options", &name];
    let option_refs: Vec<&str> = options.iter().map(|s| s.as_str()).collect();
    args.extend(option_refs);
    cli.run_raw(&args).await.map_err(|e| e.to_string())?;
    Ok(())
}
