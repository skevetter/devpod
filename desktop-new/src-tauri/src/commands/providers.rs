use crate::daemon::cli::CliRunner;
use crate::daemon::state::DaemonState;
use crate::daemon::types::Provider;
use crate::persistence::audit::AuditLog;
use log::error;
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
    audit: State<'_, Arc<AuditLog>>,
    name: String,
    source: Option<String>,
) -> Result<(), String> {
    let src = source.as_deref().unwrap_or(&name);
    let mut args = vec!["provider", "add", src, "--use=false"];
    if source.is_some() {
        args.push("--name");
        args.push(&name);
    }
    let result = cli.run_raw(&args).await;
    let success = result.is_ok();
    if let Err(e) = audit.record("add", "provider", &name, "", success) {
        error!("Failed to record audit entry: {}", e);
    }
    result.map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn provider_delete(
    cli: State<'_, Arc<CliRunner>>,
    audit: State<'_, Arc<AuditLog>>,
    name: String,
) -> Result<(), String> {
    let result = cli.run_raw(&["provider", "delete", &name]).await;
    let success = result.is_ok();
    if let Err(e) = audit.record("delete", "provider", &name, "", success) {
        error!("Failed to record audit entry: {}", e);
    }
    result.map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn provider_use(
    cli: State<'_, Arc<CliRunner>>,
    audit: State<'_, Arc<AuditLog>>,
    name: String,
) -> Result<(), String> {
    let result = cli.run_raw(&["provider", "use", &name]).await;
    let success = result.is_ok();
    if let Err(e) = audit.record("use", "provider", &name, "", success) {
        error!("Failed to record audit entry: {}", e);
    }
    result.map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn provider_update(
    cli: State<'_, Arc<CliRunner>>,
    audit: State<'_, Arc<AuditLog>>,
    name: String,
) -> Result<(), String> {
    let result = cli.run_raw(&["provider", "update", &name]).await;
    let success = result.is_ok();
    if let Err(e) = audit.record("update", "provider", &name, "", success) {
        error!("Failed to record audit entry: {}", e);
    }
    result.map_err(|e| e.to_string())?;
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
    audit: State<'_, Arc<AuditLog>>,
    name: String,
    options: Vec<String>,
) -> Result<(), String> {
    let mut args: Vec<&str> = vec!["provider", "set-options", &name];
    for opt in &options {
        args.push("-o");
        args.push(opt.as_str());
    }
    let result = cli.run_raw(&args).await;
    let success = result.is_ok();
    if let Err(e) = audit.record("set-options", "provider", &name, "", success) {
        error!("Failed to record audit entry: {}", e);
    }
    result.map_err(|e| e.to_string())?;
    Ok(())
}
