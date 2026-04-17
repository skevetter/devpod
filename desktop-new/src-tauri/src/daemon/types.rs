use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct Workspace {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub uid: String,
    #[serde(default)]
    pub picture: String,
    #[serde(default)]
    pub provider: WorkspaceProviderConfig,
    #[serde(default)]
    pub machine: WorkspaceMachineConfig,
    #[serde(default)]
    pub ide: WorkspaceIDEConfig,
    #[serde(default)]
    pub source: WorkspaceSource,
    #[serde(default)]
    pub dev_container_image: String,
    #[serde(default)]
    pub dev_container_path: String,
    #[serde(default)]
    pub creation_timestamp: String,
    #[serde(default, rename = "lastUsed")]
    pub last_used_timestamp: String,
    #[serde(default)]
    pub context: String,
    #[serde(default)]
    pub imported: bool,
    #[serde(default)]
    pub ssh_config_path: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceProviderConfig {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub options: HashMap<String, OptionValue>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceMachineConfig {
    #[serde(default, rename = "machineId")]
    pub id: String,
    #[serde(default)]
    pub auto_delete: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceIDEConfig {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub options: HashMap<String, OptionValue>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceSource {
    #[serde(default)]
    pub git_repository: String,
    #[serde(default)]
    pub git_branch: String,
    #[serde(default)]
    pub git_commit: String,
    #[serde(default, rename = "gitSubDir")]
    pub git_sub_path: String,
    #[serde(default)]
    pub local_folder: String,
    #[serde(default)]
    pub image: String,
    #[serde(default)]
    pub container: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct OptionValue {
    #[serde(default)]
    pub value: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct Provider {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub version: String,
    #[serde(default)]
    pub icon: String,
    #[serde(default)]
    pub description: String,
    #[serde(default)]
    pub source: ProviderSource,
    #[serde(default)]
    pub options: HashMap<String, ProviderOption>,
    #[serde(default)]
    pub option_groups: Vec<ProviderOptionGroup>,
    #[serde(default, rename = "isDefault")]
    pub is_default: bool,
    #[serde(default)]
    pub state: ProviderStateInfo,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct ProviderStateInfo {
    #[serde(default)]
    pub initialized: bool,
    #[serde(default)]
    pub single_machine: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct ProviderSource {
    #[serde(default)]
    pub github: String,
    #[serde(default)]
    pub file: String,
    #[serde(default)]
    pub url: String,
    #[serde(default)]
    pub raw: String,
    #[serde(default)]
    pub internal: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct ProviderOption {
    #[serde(default)]
    pub value: String,
    #[serde(default)]
    pub description: String,
    #[serde(default)]
    pub required: bool,
    #[serde(default)]
    pub default: String,
    #[serde(default, rename = "type")]
    pub option_type: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct ProviderOptionGroup {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub options: Vec<String>,
    #[serde(default)]
    pub default_visible: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct Machine {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub provider: MachineProviderConfig,
    #[serde(default)]
    pub creation_timestamp: String,
    #[serde(default)]
    pub context: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct MachineProviderConfig {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub options: HashMap<String, OptionValue>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "camelCase")]
pub struct Context {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub options: HashMap<String, String>,
}
