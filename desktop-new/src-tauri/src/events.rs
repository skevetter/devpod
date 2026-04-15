use serde::{Deserialize, Serialize};

use crate::daemon::types::{Context, Machine, Provider, Workspace};

pub mod event_names {
    pub const WORKSPACES_CHANGED: &str = "workspaces-changed";
    pub const PROVIDERS_CHANGED: &str = "providers-changed";
    pub const MACHINES_CHANGED: &str = "machines-changed";
    pub const CONTEXTS_CHANGED: &str = "contexts-changed";
    pub const COMMAND_PROGRESS: &str = "command-progress";
    pub const DAEMON_STATUS: &str = "daemon-status";
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkspacesPayload {
    pub workspaces: Vec<Workspace>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProvidersPayload {
    pub providers: Vec<Provider>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MachinesPayload {
    pub machines: Vec<Machine>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContextsPayload {
    pub contexts: Vec<Context>,
    pub active_context: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CommandProgressPayload {
    pub command_id: String,
    pub message: String,
    pub done: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum DaemonStatusPayload {
    Starting,
    Running,
    Error(String),
}
