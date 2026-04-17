use std::sync::Arc;
use std::time::Duration;

use log::{error, info, warn};
use notify::{EventKind, RecommendedWatcher, RecursiveMode, Watcher as NotifyWatcher};
use tauri::{async_runtime, AppHandle, Emitter, Runtime};
use tokio::sync::RwLock;
use tokio::time::sleep;

use super::cli::CliRunner;
use super::state::DaemonState;
use super::types::{Context, Machine, Provider, Workspace};
use crate::events::{
    event_names, ContextsPayload, MachinesPayload, ProvidersPayload, WorkspacesPayload,
};

#[derive(Debug, serde::Deserialize)]
struct ContextListOutput {
    #[serde(default)]
    contexts: Vec<Context>,
    #[serde(default)]
    active: String,
}

/// Intermediate type matching `devpod provider list --output json` format.
/// The CLI returns `{ "name": { "config": {...}, "state": {...}, "default": bool } }`.
#[derive(Debug, serde::Deserialize)]
struct ProviderEntry {
    #[serde(default)]
    config: ProviderConfig,
    #[serde(default)]
    state: ProviderState,
    #[serde(default, rename = "default")]
    is_default: bool,
}

#[derive(Debug, Default, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
struct ProviderConfig {
    #[serde(default)]
    name: String,
    #[serde(default)]
    version: String,
    #[serde(default)]
    icon: String,
    #[serde(default)]
    description: String,
    #[serde(default)]
    source: super::types::ProviderSource,
    #[serde(default)]
    options: std::collections::HashMap<String, super::types::ProviderOption>,
    #[serde(default)]
    option_groups: Vec<super::types::ProviderOptionGroup>,
}

#[derive(Debug, Default, serde::Deserialize)]
struct ProviderState {
    #[serde(default)]
    initialized: bool,
}

impl ProviderEntry {
    fn into_provider(self) -> Provider {
        Provider {
            name: self.config.name,
            version: self.config.version,
            icon: self.config.icon,
            description: self.config.description,
            source: self.config.source,
            options: self.config.options,
            option_groups: self.config.option_groups,
            is_default: self.is_default,
            state: super::types::ProviderStateInfo {
                initialized: self.state.initialized,
                single_machine: false,
            },
        }
    }
}

pub struct Watcher<R: Runtime> {
    cli: Arc<CliRunner>,
    state: Arc<RwLock<DaemonState>>,
    interval: Duration,
    app_handle: AppHandle<R>,
}

impl<R: Runtime> Watcher<R> {
    pub fn new(
        cli: Arc<CliRunner>,
        state: Arc<RwLock<DaemonState>>,
        interval: Duration,
        app_handle: AppHandle<R>,
    ) -> Self {
        Self {
            cli,
            state,
            interval,
            app_handle,
        }
    }

    /// Start the polling loop in a background tokio task.
    pub fn start_polling(self: Arc<Self>) {
        let watcher = self.clone();
        async_runtime::spawn(async move {
            loop {
                watcher.poll_once().await;
                sleep(watcher.interval).await;
            }
        });
    }

    /// Run a single poll cycle for all resource types.
    pub async fn poll_once(&self) {
        self.poll_workspaces().await;
        self.poll_providers().await;
        self.poll_machines().await;
        self.poll_contexts().await;
    }

    async fn poll_workspaces(&self) {
        match self.cli.run::<Vec<Workspace>>(&["list", "--skip-pro"]).await {
            Ok(workspaces) => {
                let changed = {
                    let mut state = self.state.write().await;
                    state.update_workspaces(workspaces)
                };
                if changed {
                    let state = self.state.read().await;
                    let list: Vec<Workspace> = state.workspace_list().into_iter().cloned().collect();
                    let _ = self.app_handle.emit(
                        event_names::WORKSPACES_CHANGED,
                        WorkspacesPayload { workspaces: list },
                    );
                }
            }
            Err(e) => {
                warn!("Failed to poll workspaces: {}", e);
            }
        }
    }

    async fn poll_providers(&self) {
        match self
            .cli
            .run::<std::collections::HashMap<String, ProviderEntry>>(&["provider", "list"])
            .await
        {
            Ok(provider_map) => {
                let providers: Vec<Provider> = provider_map
                    .into_values()
                    .map(|entry| entry.into_provider())
                    .collect();
                let changed = {
                    let mut state = self.state.write().await;
                    state.update_providers(providers)
                };
                if changed {
                    let state = self.state.read().await;
                    let list: Vec<Provider> =
                        state.provider_list().into_iter().cloned().collect();
                    let _ = self.app_handle.emit(
                        event_names::PROVIDERS_CHANGED,
                        ProvidersPayload { providers: list },
                    );
                }
            }
            Err(e) => {
                warn!("Failed to poll providers: {}", e);
            }
        }
    }

    async fn poll_machines(&self) {
        match self
            .cli
            .run::<Vec<Machine>>(&["machine", "list"])
            .await
        {
            Ok(machines) => {
                let changed = {
                    let mut state = self.state.write().await;
                    state.update_machines(machines)
                };
                if changed {
                    let state = self.state.read().await;
                    let list: Vec<Machine> =
                        state.machine_list().into_iter().cloned().collect();
                    let _ = self.app_handle.emit(
                        event_names::MACHINES_CHANGED,
                        MachinesPayload { machines: list },
                    );
                }
            }
            Err(e) => {
                warn!("Failed to poll machines: {}", e);
            }
        }
    }

    async fn poll_contexts(&self) {
        match self
            .cli
            .run::<ContextListOutput>(&["context", "list"])
            .await
        {
            Ok(output) => {
                let changed = {
                    let mut state = self.state.write().await;
                    state.update_contexts(output.contexts, output.active)
                };
                if changed {
                    let state = self.state.read().await;
                    let _ = self.app_handle.emit(
                        event_names::CONTEXTS_CHANGED,
                        ContextsPayload {
                            contexts: state.contexts.clone(),
                            active_context: state.active_context.clone(),
                        },
                    );
                }
            }
            Err(e) => {
                warn!("Failed to poll contexts: {}", e);
            }
        }
    }
}

/// Start a filesystem watcher on `~/.devpod/` that triggers a poll on changes.
/// Debounces events by 500ms.
pub fn start_fs_watcher<R: Runtime>(watcher: Arc<Watcher<R>>) {
    let devpod_dir = match dirs::home_dir() {
        Some(home) => home.join(".devpod"),
        None => {
            error!("Could not determine home directory for fs watcher");
            return;
        }
    };

    if !devpod_dir.exists() {
        info!(
            "DevPod config directory does not exist yet: {}",
            devpod_dir.display()
        );
        return;
    }

    let watch_path = devpod_dir.clone();

    std::thread::spawn(move || {
        let (tx, rx) = std::sync::mpsc::channel();

        let mut fs_watcher: RecommendedWatcher =
            match notify::Watcher::new(tx, notify::Config::default()) {
                Ok(w) => w,
                Err(e) => {
                    error!("Failed to create fs watcher: {}", e);
                    return;
                }
            };

        if let Err(e) = fs_watcher.watch(&watch_path, RecursiveMode::Recursive) {
            error!("Failed to watch {}: {}", watch_path.display(), e);
            return;
        }

        info!("Filesystem watcher started on {}", watch_path.display());

        let debounce = Duration::from_millis(500);
        let mut last_event = std::time::Instant::now() - debounce;

        loop {
            match rx.recv() {
                Ok(Ok(event)) => {
                    match event.kind {
                        EventKind::Create(_)
                        | EventKind::Modify(_)
                        | EventKind::Remove(_) => {
                            let now = std::time::Instant::now();
                            if now.duration_since(last_event) >= debounce {
                                last_event = now;
                                let w = watcher.clone();
                                async_runtime::spawn(async move {
                                    w.poll_once().await;
                                });
                            }
                        }
                        _ => {}
                    }
                }
                Ok(Err(e)) => {
                    warn!("Filesystem watch error: {}", e);
                }
                Err(e) => {
                    error!("Filesystem watcher channel closed: {}", e);
                    break;
                }
            }
        }
    });
}
