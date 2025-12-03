# Log Persistence Issue Analysis

## Problem Statement

When starting a workspace via `devpod up`, users see detailed startup logs. However, when returning to the workspace page in the UI and viewing logs again, the previous log data is lost and only shows:

```
[13:44:51] info Workspace 'github-com-skevetter-features' is 'Running'
```

## Root Cause

### Current Architecture

**Log Storage** (`desktop/src-tauri/src/action_logs.rs`):
- Logs are stored in `{app_data_dir}/action_logs/{action_id}.log`
- Each action gets a unique UUID as its ID
- Logs persist for 30 days before automatic cleanup

**Action System** (`desktop/src/contexts/DevPodContext/action/`):
- Every workspace operation (start, stop, rebuild, checkStatus) creates a NEW action
- Each action has a unique ID generated via `uuidv4()`
- Actions are stored in memory with status: pending, success, error, cancelled

**Log Replay** (`desktop/src/client/workspaces/client.ts`):
- `replayAction()` calls Tauri command `get_action_logs` with the action ID
- Reads logs from `{action_id}.log` file
- Displays logs in terminal

### The Issue

1. User runs `devpod up workspace-name` → Creates Action A with ID `abc-123`
2. Logs are written to `action_logs/abc-123.log`
3. User views workspace in UI → Shows logs from Action A
4. User navigates away and returns to workspace page
5. UI calls `checkStatus()` → Creates NEW Action B with ID `def-456`
6. Action B only outputs "Workspace is 'Running'" to `action_logs/def-456.log`
7. UI shows logs from Action B (the new status check), not Action A (the original startup)

## Code Locations

### Backend (Rust)
```rust
// desktop/src-tauri/src/action_logs.rs

#[tauri::command]
pub fn write_action_log(app_handle: AppHandle, action_id: String, data: String) {
    // Writes to {app_data_dir}/action_logs/{action_id}.log
}

#[tauri::command]
pub fn get_action_logs(app_handle: AppHandle, action_id: String) -> Vec<String> {
    // Reads from {app_data_dir}/action_logs/{action_id}.log
}
```

### Frontend (TypeScript)
```typescript
// desktop/src/contexts/DevPodContext/action/action.ts
export class Action {
  public readonly id = uuidv4()  // NEW ID FOR EACH ACTION
  // ...
}

// desktop/src/contexts/DevPodContext/workspaces/useWorkspace.ts
const checkStatus = useCallback(() => {
  return runAction({
    actionName: "checkStatus",  // Creates NEW action
    // ...
  })
}, [])
```

## Solutions

### Option 1: Show Last Action Logs (Recommended)
**Concept**: When viewing workspace, show logs from the most recent meaningful action (start/rebuild), not status checks.

**Changes**:
1. Track "last significant action ID" per workspace
2. When viewing workspace logs, use that action ID instead of creating new status check
3. Only create new action if explicitly checking status

**Implementation**:
```typescript
// Store last action ID in workspace metadata
interface Workspace {
  // ...
  lastActionID?: string  // ID of last start/rebuild action
}

// When starting workspace
const start = () => {
  const actionID = runAction({ actionName: "start", ... })
  updateWorkspace({ lastActionID: actionID })
}

// When viewing logs
const viewLogs = () => {
  if (workspace.lastActionID) {
    return replayAction(workspace.lastActionID)
  }
  // Fallback to status check
}
```

**Pros**:
- ✅ Preserves full startup logs
- ✅ Minimal code changes
- ✅ No breaking changes

**Cons**:
- ⚠️ Need to persist lastActionID in workspace config

### Option 2: Aggregate Logs Per Workspace
**Concept**: Store all logs for a workspace in a single file, not per-action.

**Changes**:
1. Change log storage from `{action_id}.log` to `{workspace_id}.log`
2. Append all actions to same file with timestamps
3. Read entire workspace log history

**Implementation**:
```rust
// action_logs.rs
pub fn write_workspace_log(workspace_id: String, action_name: String, data: String) {
    let path = format!("{}/workspaces/{}.log", app_data_dir, workspace_id);
    // Append with timestamp and action name
    file.write(format!("[{}][{}] {}\n", timestamp, action_name, data))
}
```

**Pros**:
- ✅ Complete workspace history in one place
- ✅ Easy to view all operations

**Cons**:
- ❌ Large refactor required
- ❌ Log files grow indefinitely (need rotation)
- ❌ Breaking change to existing log storage

### Option 3: Don't Create Actions for Status Checks
**Concept**: Status checks shouldn't create actions, just query state.

**Changes**:
1. Make `checkStatus()` NOT create an action
2. Only create actions for operations that modify state (start, stop, rebuild)
3. Status checks just return current state without logging

**Implementation**:
```typescript
const checkStatus = async () => {
  // Don't create action, just query
  const result = await client.workspaces.getStatus(workspaceID)
  return result
}
```

**Pros**:
- ✅ Cleaner separation of concerns
- ✅ Reduces unnecessary action creation

**Cons**:
- ⚠️ Still doesn't solve showing previous logs
- ⚠️ Need to handle UI differently for status vs operations

### Option 4: Link Workspace View to Latest Action
**Concept**: Workspace detail page automatically shows logs from the most recent action.

**Changes**:
1. Query action history for workspace
2. Find most recent action (any type)
3. Display those logs by default

**Implementation**:
```typescript
const WorkspaceDetail = ({ workspaceID }) => {
  const actions = useWorkspaceActions(workspaceID)
  const latestAction = actions[0]  // Most recent

  return <ActionLogs actionID={latestAction.id} />
}
```

**Pros**:
- ✅ Simple implementation
- ✅ Always shows latest activity

**Cons**:
- ⚠️ Might show status check logs instead of startup logs
- ⚠️ Need to filter for "meaningful" actions

## Recommended Solution

**Hybrid Approach: Option 1 + Option 3**

1. **Don't create actions for status checks** - Status is just a query, not an operation
2. **Track last operation action ID** - Store in workspace metadata
3. **Show last operation logs** - When viewing workspace, show logs from last start/rebuild/etc.

### Implementation Steps

#### Step 1: Add lastActionID to Workspace
```typescript
// types/workspace.ts
interface Workspace {
  // ... existing fields
  lastActionID?: string
}
```

#### Step 2: Update Workspace Operations
```typescript
// useWorkspace.ts
const start = useCallback((onStream) => {
  const actionID = runAction({
    actionName: "start",
    // ...
  })

  // Store action ID
  updateWorkspaceMetadata({ lastActionID: actionID })

  return actionID
}, [])
```

#### Step 3: Make Status Check Non-Action
```typescript
const checkStatus = useCallback(async () => {
  // Don't create action, just query
  const result = await client.workspaces.status(workspaceID)
  return result
}, [])
```

#### Step 4: Show Last Action Logs in UI
```typescript
// WorkspaceDetail.tsx
const workspace = useWorkspace(workspaceID)

// Show logs from last operation, not status check
const actionID = workspace.lastActionID || workspace.currentActionID

return <ActionLogs actionID={actionID} />
```

### Benefits
- ✅ Preserves full startup logs
- ✅ Reduces unnecessary action creation
- ✅ Clear separation: operations create actions, queries don't
- ✅ Minimal code changes
- ✅ No breaking changes to log storage

### Effort Estimate
- **Backend**: No changes needed (log storage works as-is)
- **Frontend**:
  - Add `lastActionID` to workspace type: 30 min
  - Update workspace operations to store action ID: 1 hour
  - Make status check non-action: 1 hour
  - Update UI to use lastActionID: 1 hour
- **Testing**: 2 hours
- **Total**: ~5-6 hours

## Alternative Quick Fix

If full solution is too much work, a quick fix:

**Show action history in workspace view**:
```typescript
// WorkspaceDetail.tsx
const actions = useWorkspaceActions(workspaceID)

return (
  <Tabs>
    <Tab>Current Status</Tab>
    <Tab>Action History</Tab>
  </Tabs>

  {selectedTab === "history" && (
    <ActionList actions={actions} />
  )}
)
```

This lets users manually navigate to previous action logs without changing the core architecture.

**Effort**: 2-3 hours
