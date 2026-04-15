// Types matching the Rust daemon types (camelCase)

export interface WorkspaceSource {
  gitRepository?: string
  gitBranch?: string
  gitCommit?: string
  localFolder?: string
  image?: string
}

export interface WorkspaceIDEConfig {
  name?: string
  options?: Record<string, OptionValue>
}

export interface WorkspaceMachineConfig {
  id?: string
  autoDelete?: boolean
}

export interface WorkspaceProviderConfig {
  name?: string
  options?: Record<string, OptionValue>
}

export interface Workspace {
  id: string
  uid?: string
  picture?: string
  provider?: WorkspaceProviderConfig
  machine?: WorkspaceMachineConfig
  ide?: WorkspaceIDEConfig
  source?: WorkspaceSource
  creationTimestamp?: string
  lastUsedTimestamp?: string
  context?: string
  status?: string
}

export type OptionValue = string | number | boolean

export interface ProviderSource {
  github?: string
  file?: string
  url?: string
  raw?: string
}

export interface ProviderOptionGroup {
  name?: string
  options?: string[]
  defaultVisible?: boolean
}

export interface ProviderOption {
  name: string
  displayName?: string
  description?: string
  default?: OptionValue
  enum?: string[]
  required?: boolean
  hidden?: boolean
  password?: boolean
  command?: string
  type?: string
  group?: string
}

export interface Provider {
  name: string
  version?: string
  source?: ProviderSource
  description?: string
  options?: Record<string, ProviderOption>
  optionGroups?: ProviderOptionGroup[]
  state?: {
    initialized?: boolean
    singleMachine?: boolean
  }
}

export interface MachineProviderConfig {
  name?: string
}

export interface Machine {
  id: string
  provider?: MachineProviderConfig
  status?: string
  creationTimestamp?: string
  lastUsedTimestamp?: string
}

export interface Context {
  name: string
  options?: Record<string, OptionValue>
}

export interface CommandProgress {
  commandId: string
  message: string
  done: boolean
}

export interface AuditEntry {
  id: number
  timestamp: string
  action: string
  resourceType: string
  resourceId: string
  details: string
  success: boolean
}

export interface LogEntry {
  workspaceId: string
  filename: string
  createdAt: string
  sizeBytes: number
}
