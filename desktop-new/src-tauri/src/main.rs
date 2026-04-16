#![cfg_attr(
    all(not(debug_assertions), target_os = "windows"),
    windows_subsystem = "windows"
)]

mod commands;
mod daemon;
mod events;
mod persistence;
mod terminal;
mod tray;

use std::sync::Arc;
use std::time::Duration;

use daemon::cli::{resolve_binary_path, CliRunner};
use daemon::state::DaemonState;
use daemon::watcher::{start_fs_watcher, Watcher};
use log::{error, info};
use persistence::audit::AuditLog;
use persistence::logs::LogStore;
use tauri::{Manager, RunEvent, WindowEvent};
use terminal::pty::PtyManager;
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
            commands::workspaces::workspace_status,
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
            commands::terminal::terminal_create,
            commands::terminal::terminal_create_ssh,
            commands::terminal::terminal_write,
            commands::terminal::terminal_resize,
            commands::terminal::terminal_close,
            commands::terminal::terminal_list,
            commands::audit::audit_recent,
            commands::audit::audit_by_resource,
            commands::logs::workspace_logs_list,
            commands::logs::workspace_log_read,
            commands::contexts::context_list,
            commands::contexts::context_use,
            commands::system::devpod_version,
            commands::ssh_keys::ssh_key_list,
            commands::ssh_keys::ssh_key_generate,
        ])
        .setup(move |app| {
            let window = app.get_webview_window("main").unwrap();
            window.show().unwrap();

            let pty_manager = Arc::new(PtyManager::new(app.handle().clone()));
            app.manage(pty_manager);

            // Initialize persistence
            match LogStore::default_path() {
                Ok(log_store) => {
                    let log_store = Arc::new(log_store);
                    app.manage(log_store.clone());

                    // Prune old logs on startup (> 30 days)
                    tokio::spawn(async move {
                        if let Err(e) = log_store.prune(30).await {
                            error!("Failed to prune old logs: {}", e);
                        }
                    });
                }
                Err(e) => {
                    error!("Failed to initialize log store: {}", e);
                }
            }

            match AuditLog::default_path() {
                Ok(audit_log) => {
                    let audit_log = Arc::new(audit_log);
                    app.manage(audit_log.clone());

                    // Prune old audit entries on startup (> 90 days)
                    let audit_prune = audit_log.clone();
                    tokio::spawn(async move {
                        if let Err(e) = audit_prune.prune(90) {
                            error!("Failed to prune old audit entries: {}", e);
                        }
                    });
                }
                Err(e) => {
                    error!("Failed to initialize audit log: {}", e);
                }
            }

            if let Err(e) = tray::setup_tray(app.handle()) {
                error!("Failed to setup system tray: {e}");
            }

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
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|app, event| {
            if let RunEvent::WindowEvent {
                label,
                event: WindowEvent::CloseRequested { api, .. },
                ..
            } = event
            {
                // Hide the window instead of quitting when user closes it
                if label == "main" {
                    api.prevent_close();
                    if let Some(window) = app.get_webview_window("main") {
                        let _ = window.hide();
                    }
                }
            }
        });
}
