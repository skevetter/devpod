use crate::persistence::audit::{AuditEntry, AuditLog};
use std::sync::Arc;
use tauri::State;

#[tauri::command]
pub async fn audit_recent(
    audit: State<'_, Arc<AuditLog>>,
    limit: Option<u32>,
) -> Result<Vec<AuditEntry>, String> {
    audit
        .recent(limit.unwrap_or(50))
        .map_err(|e| e.to_string())
}

#[tauri::command]
pub async fn audit_by_resource(
    audit: State<'_, Arc<AuditLog>>,
    resource_type: String,
    resource_id: String,
    limit: Option<u32>,
) -> Result<Vec<AuditEntry>, String> {
    audit
        .by_resource(&resource_type, &resource_id, limit.unwrap_or(50))
        .map_err(|e| e.to_string())
}
