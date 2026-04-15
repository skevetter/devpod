use anyhow::{Context, Result};
use chrono::Utc;
use log::info;
use rusqlite::{params, Connection};
use std::path::PathBuf;
use std::sync::Mutex;

/// SQLite-backed audit log for tracking user actions.
/// Database stored at `~/.devpod/audit.db`.
pub struct AuditLog {
    conn: Mutex<Connection>,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AuditEntry {
    pub id: i64,
    pub timestamp: String,
    pub action: String,
    pub resource_type: String,
    pub resource_id: String,
    pub details: String,
    pub success: bool,
}

impl AuditLog {
    pub fn open(path: PathBuf) -> Result<Self> {
        let conn = Connection::open(&path)
            .with_context(|| format!("Failed to open audit database: {}", path.display()))?;

        conn.execute_batch(
            "PRAGMA journal_mode = WAL;
             PRAGMA busy_timeout = 5000;",
        )?;

        conn.execute_batch(
            "CREATE TABLE IF NOT EXISTS audit_log (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                timestamp TEXT NOT NULL,
                action TEXT NOT NULL,
                resource_type TEXT NOT NULL,
                resource_id TEXT NOT NULL DEFAULT '',
                details TEXT NOT NULL DEFAULT '',
                success INTEGER NOT NULL DEFAULT 1
            );
            CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp);
            CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit_log(resource_type, resource_id);",
        )?;

        info!("Audit log opened at {}", path.display());
        Ok(Self {
            conn: Mutex::new(conn),
        })
    }

    pub fn default_path() -> Result<Self> {
        let home = dirs::home_dir().context("Could not determine home directory")?;
        let path = home.join(".devpod").join("audit.db");
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        Self::open(path)
    }

    /// Record an audit event.
    pub fn record(
        &self,
        action: &str,
        resource_type: &str,
        resource_id: &str,
        details: &str,
        success: bool,
    ) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        let timestamp = Utc::now().to_rfc3339();

        conn.execute(
            "INSERT INTO audit_log (timestamp, action, resource_type, resource_id, details, success)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
            params![timestamp, action, resource_type, resource_id, details, success as i32],
        )?;
        Ok(())
    }

    /// Get the most recent audit entries, up to `limit`.
    pub fn recent(&self, limit: u32) -> Result<Vec<AuditEntry>> {
        let conn = self.conn.lock().unwrap();
        let mut stmt = conn.prepare(
            "SELECT id, timestamp, action, resource_type, resource_id, details, success
             FROM audit_log
             ORDER BY id DESC
             LIMIT ?1",
        )?;

        let entries = stmt
            .query_map(params![limit], |row| {
                Ok(AuditEntry {
                    id: row.get(0)?,
                    timestamp: row.get(1)?,
                    action: row.get(2)?,
                    resource_type: row.get(3)?,
                    resource_id: row.get(4)?,
                    details: row.get(5)?,
                    success: row.get::<_, i32>(6)? != 0,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;

        Ok(entries)
    }

    /// Get audit entries for a specific resource.
    pub fn by_resource(
        &self,
        resource_type: &str,
        resource_id: &str,
        limit: u32,
    ) -> Result<Vec<AuditEntry>> {
        let conn = self.conn.lock().unwrap();
        let mut stmt = conn.prepare(
            "SELECT id, timestamp, action, resource_type, resource_id, details, success
             FROM audit_log
             WHERE resource_type = ?1 AND resource_id = ?2
             ORDER BY id DESC
             LIMIT ?3",
        )?;

        let entries = stmt
            .query_map(params![resource_type, resource_id, limit], |row| {
                Ok(AuditEntry {
                    id: row.get(0)?,
                    timestamp: row.get(1)?,
                    action: row.get(2)?,
                    resource_type: row.get(3)?,
                    resource_id: row.get(4)?,
                    details: row.get(5)?,
                    success: row.get::<_, i32>(6)? != 0,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;

        Ok(entries)
    }

    /// Prune entries older than `max_age_days`.
    pub fn prune(&self, max_age_days: i64) -> Result<u64> {
        let conn = self.conn.lock().unwrap();
        let cutoff = Utc::now() - chrono::Duration::days(max_age_days);
        let cutoff_str = cutoff.to_rfc3339();

        let removed = conn.execute(
            "DELETE FROM audit_log WHERE timestamp < ?1",
            params![cutoff_str],
        )? as u64;

        if removed > 0 {
            info!("Pruned {} old audit entries", removed);
        }
        Ok(removed)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn test_audit() -> (TempDir, AuditLog) {
        let tmp = TempDir::new().unwrap();
        let db_path = tmp.path().join("test_audit.db");
        let audit = AuditLog::open(db_path).unwrap();
        (tmp, audit)
    }

    #[test]
    fn test_record_and_recent() {
        let (_tmp, audit) = test_audit();

        audit
            .record("create", "workspace", "ws-1", "Created workspace", true)
            .unwrap();
        audit
            .record("delete", "workspace", "ws-2", "Deleted workspace", true)
            .unwrap();

        let entries = audit.recent(10).unwrap();
        assert_eq!(entries.len(), 2);
        assert_eq!(entries[0].action, "delete"); // newest first
        assert_eq!(entries[1].action, "create");
    }

    #[test]
    fn test_by_resource() {
        let (_tmp, audit) = test_audit();

        audit
            .record("start", "workspace", "ws-1", "", true)
            .unwrap();
        audit
            .record("stop", "workspace", "ws-1", "", true)
            .unwrap();
        audit
            .record("start", "workspace", "ws-2", "", true)
            .unwrap();

        let entries = audit.by_resource("workspace", "ws-1", 10).unwrap();
        assert_eq!(entries.len(), 2);
    }

    #[test]
    fn test_recent_limit() {
        let (_tmp, audit) = test_audit();

        for i in 0..5 {
            audit
                .record("action", "test", &format!("r-{}", i), "", true)
                .unwrap();
        }

        let entries = audit.recent(3).unwrap();
        assert_eq!(entries.len(), 3);
    }

    #[test]
    fn test_prune_keeps_recent() {
        let (_tmp, audit) = test_audit();

        audit.record("action", "test", "r-1", "", true).unwrap();

        let removed = audit.prune(30).unwrap();
        assert_eq!(removed, 0);

        let entries = audit.recent(10).unwrap();
        assert_eq!(entries.len(), 1);
    }
}
