use crate::terminal::pty::PtyManager;
use std::sync::Arc;
use tauri::State;

#[tauri::command]
pub fn terminal_create(
    pty: State<'_, Arc<PtyManager>>,
    cols: u16,
    rows: u16,
) -> Result<String, String> {
    pty.create_session(cols, rows)
}

#[tauri::command]
pub fn terminal_create_ssh(
    pty: State<'_, Arc<PtyManager>>,
    workspace_id: String,
    cols: u16,
    rows: u16,
) -> Result<String, String> {
    pty.create_ssh_session(&workspace_id, cols, rows)
}

#[tauri::command]
pub fn terminal_write(
    pty: State<'_, Arc<PtyManager>>,
    session_id: String,
    data: Vec<u8>,
) -> Result<(), String> {
    pty.write_to_session(&session_id, &data)
}

#[tauri::command]
pub fn terminal_resize(
    pty: State<'_, Arc<PtyManager>>,
    session_id: String,
    cols: u16,
    rows: u16,
) -> Result<(), String> {
    pty.resize_session(&session_id, cols, rows)
}

#[tauri::command]
pub fn terminal_close(
    pty: State<'_, Arc<PtyManager>>,
    session_id: String,
) -> Result<(), String> {
    pty.close_session(&session_id)
}

#[tauri::command]
pub fn terminal_list(pty: State<'_, Arc<PtyManager>>) -> Result<Vec<String>, String> {
    pty.list_sessions()
}
