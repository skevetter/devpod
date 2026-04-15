use portable_pty::{native_pty_system, CommandBuilder, MasterPty, PtySize};
use serde::Serialize;
use std::collections::HashMap;
use std::io::{Read, Write};
use std::sync::{Arc, Mutex};
use std::thread;
use tauri::{AppHandle, Emitter};
use uuid::Uuid;

#[derive(Clone, Serialize)]
pub struct TerminalOutputPayload {
    pub session_id: String,
    pub data: Vec<u8>,
}

#[derive(Clone, Serialize)]
pub struct TerminalExitPayload {
    pub session_id: String,
}

struct PtySession {
    master: Box<dyn MasterPty + Send>,
    writer: Box<dyn Write + Send>,
    _reader_handle: thread::JoinHandle<()>,
}

pub struct PtyManager {
    sessions: Arc<Mutex<HashMap<String, PtySession>>>,
    app_handle: AppHandle,
}

impl PtyManager {
    pub fn new(app_handle: AppHandle) -> Self {
        Self {
            sessions: Arc::new(Mutex::new(HashMap::new())),
            app_handle,
        }
    }

    pub fn create_session(&self, cols: u16, rows: u16) -> Result<String, String> {
        let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/sh".to_string());
        let mut cmd = CommandBuilder::new(&shell);
        cmd.env("TERM", "xterm-256color");

        self.spawn_session(cmd, cols, rows)
    }

    pub fn create_ssh_session(
        &self,
        workspace_id: &str,
        cols: u16,
        rows: u16,
    ) -> Result<String, String> {
        let devpod_path =
            which::which("devpod").map_err(|e| format!("devpod binary not found: {e}"))?;

        let mut cmd = CommandBuilder::new(devpod_path);
        cmd.arg("ssh");
        cmd.arg(workspace_id);
        cmd.env("TERM", "xterm-256color");

        self.spawn_session(cmd, cols, rows)
    }

    fn spawn_session(
        &self,
        cmd: CommandBuilder,
        cols: u16,
        rows: u16,
    ) -> Result<String, String> {
        let session_id = Uuid::new_v4().to_string();
        let pty_system = native_pty_system();

        let size = PtySize {
            rows,
            cols,
            pixel_width: 0,
            pixel_height: 0,
        };

        let pair = pty_system
            .openpty(size)
            .map_err(|e| format!("Failed to open PTY: {e}"))?;

        pair.slave
            .spawn_command(cmd)
            .map_err(|e| format!("Failed to spawn command: {e}"))?;

        // We drop the slave after spawning to avoid keeping it open
        drop(pair.slave);

        let writer = pair
            .master
            .take_writer()
            .map_err(|e| format!("Failed to take PTY writer: {e}"))?;

        let mut reader = pair
            .master
            .try_clone_reader()
            .map_err(|e| format!("Failed to clone PTY reader: {e}"))?;

        let app_handle = self.app_handle.clone();
        let sid = session_id.clone();
        let sessions_ref = self.sessions.clone();

        let reader_handle = thread::spawn(move || {
            let mut buf = [0u8; 4096];
            loop {
                match reader.read(&mut buf) {
                    Ok(0) => break,
                    Ok(n) => {
                        let payload = TerminalOutputPayload {
                            session_id: sid.clone(),
                            data: buf[..n].to_vec(),
                        };
                        let _ = app_handle.emit("terminal:output", payload);
                    }
                    Err(_) => break,
                }
            }

            let exit_payload = TerminalExitPayload {
                session_id: sid.clone(),
            };
            let _ = app_handle.emit("terminal:exit", exit_payload);

            // Clean up session on exit
            let _ = sessions_ref.lock().map(|mut s| s.remove(&sid));
        });

        let session = PtySession {
            master: pair.master,
            writer,
            _reader_handle: reader_handle,
        };

        self.sessions
            .lock()
            .map_err(|e| format!("Lock poisoned: {e}"))?
            .insert(session_id.clone(), session);

        Ok(session_id)
    }

    pub fn write_to_session(&self, session_id: &str, data: &[u8]) -> Result<(), String> {
        let mut sessions = self
            .sessions
            .lock()
            .map_err(|e| format!("Lock poisoned: {e}"))?;
        let session = sessions
            .get_mut(session_id)
            .ok_or_else(|| format!("Session not found: {session_id}"))?;
        session
            .writer
            .write_all(data)
            .map_err(|e| format!("Write failed: {e}"))?;
        session
            .writer
            .flush()
            .map_err(|e| format!("Flush failed: {e}"))?;
        Ok(())
    }

    pub fn resize_session(&self, session_id: &str, cols: u16, rows: u16) -> Result<(), String> {
        let sessions = self
            .sessions
            .lock()
            .map_err(|e| format!("Lock poisoned: {e}"))?;
        let session = sessions
            .get(session_id)
            .ok_or_else(|| format!("Session not found: {session_id}"))?;
        session
            .master
            .resize(PtySize {
                rows,
                cols,
                pixel_width: 0,
                pixel_height: 0,
            })
            .map_err(|e| format!("Resize failed: {e}"))?;
        Ok(())
    }

    pub fn close_session(&self, session_id: &str) -> Result<(), String> {
        let mut sessions = self
            .sessions
            .lock()
            .map_err(|e| format!("Lock poisoned: {e}"))?;
        sessions
            .remove(session_id)
            .ok_or_else(|| format!("Session not found: {session_id}"))?;
        Ok(())
    }

    pub fn list_sessions(&self) -> Result<Vec<String>, String> {
        let sessions = self
            .sessions
            .lock()
            .map_err(|e| format!("Lock poisoned: {e}"))?;
        Ok(sessions.keys().cloned().collect())
    }
}
