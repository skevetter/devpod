#![cfg_attr(
    all(not(debug_assertions), target_os = "windows"),
    windows_subsystem = "windows"
)]

mod commands;
mod daemon;
mod events;

use std::sync::Arc;
use std::time::Duration;

use daemon::cli::{resolve_binary_path, CliRunner};
use daemon::state::DaemonState;
use daemon::watcher::{start_fs_watcher, Watcher};
use log::{error, info};
use tauri::Manager;
use tokio::sync::RwLock;

pub type SharedState = Arc<RwLock<DaemonState>>;

fn main() {
    let state: SharedState = Arc::new(RwLock::new(DaemonState::new()));

    tauri::Builder::default()
        .plugin(tauri_plugin_log::Builder::new().build())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_os::init())
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_fs::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_clipboard_manager::init())
        .plugin(tauri_plugin_opener::init())
        .manage(state.clone())
        .invoke_handler(tauri::generate_handler![
            commands::workspaces::workspace_list,
            commands::workspaces::workspace_up,
            commands::workspaces::workspace_stop,
            commands::workspaces::workspace_delete,
            commands::workspaces::workspace_rebuild,
            commands::providers::provider_list,
            commands::providers::provider_add,
            commands::providers::provider_delete,
            commands::providers::provider_use,
            commands::providers::provider_update,
            commands::providers::provider_options,
            commands::providers::provider_set_options,
            commands::machines::machine_list,
            commands::machines::machine_create,
            commands::machines::machine_delete,
            commands::machines::machine_start,
            commands::machines::machine_stop,
            commands::machines::machine_status,
        ])
        .setup(move |app| {
            let window = app.get_webview_window("main").unwrap();
            window.show().unwrap();

            let binary_path = match resolve_binary_path(None) {
                Ok(path) => {
                    info!("Resolved devpod binary: {}", path.display());
                    path
                }
                Err(e) => {
                    error!("Failed to resolve devpod binary: {}. Polling disabled.", e);
                    return Ok(());
                }
            };

            let cli = match CliRunner::new(binary_path) {
                Ok(runner) => Arc::new(runner),
                Err(e) => {
                    error!("Failed to create CLI runner: {}. Polling disabled.", e);
                    return Ok(());
                }
            };

            app.manage(cli.clone());

            let watcher = Arc::new(Watcher::new(
                cli,
                state.clone(),
                Duration::from_secs(3),
                app.handle().clone(),
            ));

            watcher.clone().start_polling();
            start_fs_watcher(watcher);

            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
