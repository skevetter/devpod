use std::collections::HashMap;

use super::types::{Context, Machine, Provider, Workspace};

#[derive(Debug, Default)]
pub struct DaemonState {
    pub workspaces: HashMap<String, Workspace>,
    pub providers: HashMap<String, Provider>,
    pub machines: HashMap<String, Machine>,
    pub contexts: Vec<Context>,
    pub active_context: String,
}

impl DaemonState {
    pub fn new() -> Self {
        Self::default()
    }

    /// Update workspaces from a list. Returns true if anything changed.
    pub fn update_workspaces(&mut self, list: Vec<Workspace>) -> bool {
        let new_map: HashMap<String, Workspace> = list
            .into_iter()
            .map(|w| (w.id.clone(), w))
            .collect();
        if new_map != self.workspaces {
            self.workspaces = new_map;
            true
        } else {
            false
        }
    }

    /// Update providers from a list. Returns true if anything changed.
    pub fn update_providers(&mut self, list: Vec<Provider>) -> bool {
        let new_map: HashMap<String, Provider> = list
            .into_iter()
            .map(|p| (p.name.clone(), p))
            .collect();
        if new_map != self.providers {
            self.providers = new_map;
            true
        } else {
            false
        }
    }

    /// Update machines from a list. Returns true if anything changed.
    pub fn update_machines(&mut self, list: Vec<Machine>) -> bool {
        let new_map: HashMap<String, Machine> = list
            .into_iter()
            .map(|m| (m.id.clone(), m))
            .collect();
        if new_map != self.machines {
            self.machines = new_map;
            true
        } else {
            false
        }
    }

    /// Update contexts and active context. Returns true if anything changed.
    pub fn update_contexts(&mut self, contexts: Vec<Context>, active: String) -> bool {
        if contexts != self.contexts || active != self.active_context {
            self.contexts = contexts;
            self.active_context = active;
            true
        } else {
            false
        }
    }

    /// Return workspaces sorted by last_used_timestamp descending.
    pub fn workspace_list(&self) -> Vec<&Workspace> {
        let mut list: Vec<&Workspace> = self.workspaces.values().collect();
        list.sort_by(|a, b| b.last_used_timestamp.cmp(&a.last_used_timestamp));
        list
    }

    /// Return providers sorted by name ascending.
    pub fn provider_list(&self) -> Vec<&Provider> {
        let mut list: Vec<&Provider> = self.providers.values().collect();
        list.sort_by(|a, b| a.name.cmp(&b.name));
        list
    }

    /// Return machines sorted by id ascending.
    pub fn machine_list(&self) -> Vec<&Machine> {
        let mut list: Vec<&Machine> = self.machines.values().collect();
        list.sort_by(|a, b| a.id.cmp(&b.id));
        list
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::daemon::types::*;

    fn make_workspace(id: &str, last_used: &str) -> Workspace {
        Workspace {
            id: id.to_string(),
            last_used_timestamp: last_used.to_string(),
            ..Default::default()
        }
    }

    fn make_provider(name: &str) -> Provider {
        Provider {
            name: name.to_string(),
            ..Default::default()
        }
    }

    #[test]
    fn detects_workspace_change() {
        let mut state = DaemonState::new();
        let ws = vec![make_workspace("ws1", "2024-01-01")];
        assert!(state.update_workspaces(ws.clone()));
        // Same data should return false
        assert!(!state.update_workspaces(ws));
    }

    #[test]
    fn detects_workspace_removal() {
        let mut state = DaemonState::new();
        let ws = vec![
            make_workspace("ws1", "2024-01-01"),
            make_workspace("ws2", "2024-01-02"),
        ];
        assert!(state.update_workspaces(ws));
        // Remove one workspace
        let ws = vec![make_workspace("ws1", "2024-01-01")];
        assert!(state.update_workspaces(ws));
        assert_eq!(state.workspaces.len(), 1);
    }

    #[test]
    fn detects_provider_change() {
        let mut state = DaemonState::new();
        let providers = vec![make_provider("docker")];
        assert!(state.update_providers(providers.clone()));
        assert!(!state.update_providers(providers));
        // Change the provider list
        let providers = vec![make_provider("docker"), make_provider("kubernetes")];
        assert!(state.update_providers(providers));
    }

    #[test]
    fn sort_order() {
        let mut state = DaemonState::new();
        state.update_workspaces(vec![
            make_workspace("old", "2024-01-01"),
            make_workspace("new", "2024-06-01"),
            make_workspace("mid", "2024-03-01"),
        ]);
        let sorted = state.workspace_list();
        assert_eq!(sorted[0].id, "new");
        assert_eq!(sorted[1].id, "mid");
        assert_eq!(sorted[2].id, "old");

        state.update_providers(vec![
            make_provider("zebra"),
            make_provider("alpha"),
            make_provider("middle"),
        ]);
        let sorted = state.provider_list();
        assert_eq!(sorted[0].name, "alpha");
        assert_eq!(sorted[1].name, "middle");
        assert_eq!(sorted[2].name, "zebra");
    }
}
