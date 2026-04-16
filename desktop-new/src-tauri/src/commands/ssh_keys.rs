use serde::Serialize;
use std::path::PathBuf;
use tokio::fs;
use tokio::process::Command;

#[derive(Debug, Serialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct SshKeyInfo {
    pub name: String,
    pub key_type: String,
    pub fingerprint: String,
    pub comment: String,
    pub public_key: String,
    pub path: String,
    pub has_passphrase: bool,
}

fn ssh_dir() -> Result<PathBuf, String> {
    dirs::home_dir()
        .map(|h| h.join(".ssh"))
        .ok_or_else(|| "Could not determine home directory".to_string())
}

#[tauri::command]
pub async fn ssh_key_list() -> Result<Vec<SshKeyInfo>, String> {
    let ssh_path = ssh_dir()?;
    if !ssh_path.exists() {
        return Ok(vec![]);
    }

    let mut keys = Vec::new();
    let mut entries = fs::read_dir(&ssh_path)
        .await
        .map_err(|e| format!("Failed to read .ssh directory: {}", e))?;

    while let Some(entry) = entries
        .next_entry()
        .await
        .map_err(|e| format!("Failed to read directory entry: {}", e))?
    {
        let path = entry.path();
        let name = path
            .file_name()
            .unwrap_or_default()
            .to_string_lossy()
            .to_string();

        // Only look at .pub files to find key pairs
        if !name.ends_with(".pub") {
            continue;
        }

        let pub_content = match fs::read_to_string(&path).await {
            Ok(c) => c.trim().to_string(),
            Err(_) => continue,
        };

        // Parse the public key line: <type> <key> [comment]
        let parts: Vec<&str> = pub_content.splitn(3, ' ').collect();
        if parts.len() < 2 {
            continue;
        }

        let key_type = parts[0].to_string();
        let comment = parts.get(2).unwrap_or(&"").to_string();
        let base_name = name.trim_end_matches(".pub").to_string();
        let private_path = ssh_path.join(&base_name);

        // Get fingerprint via ssh-keygen
        let fingerprint = match Command::new("ssh-keygen")
            .args(["-l", "-f"])
            .arg(&path)
            .output()
            .await
        {
            Ok(output) if output.status.success() => {
                String::from_utf8_lossy(&output.stdout).trim().to_string()
            }
            _ => String::new(),
        };

        // Check if private key has passphrase by attempting to read it
        let has_passphrase = if private_path.exists() {
            match Command::new("ssh-keygen")
                .args(["-y", "-P", "", "-f"])
                .arg(&private_path)
                .output()
                .await
            {
                Ok(output) => !output.status.success(),
                Err(_) => false,
            }
        } else {
            false
        };

        keys.push(SshKeyInfo {
            name: base_name,
            key_type,
            fingerprint,
            comment,
            public_key: pub_content,
            path: private_path.to_string_lossy().to_string(),
            has_passphrase,
        });
    }

    keys.sort_by(|a, b| a.name.cmp(&b.name));
    Ok(keys)
}

#[tauri::command]
pub async fn ssh_key_generate(
    name: String,
    key_type: Option<String>,
    comment: Option<String>,
) -> Result<SshKeyInfo, String> {
    let ssh_path = ssh_dir()?;

    // Ensure .ssh directory exists
    fs::create_dir_all(&ssh_path)
        .await
        .map_err(|e| format!("Failed to create .ssh directory: {}", e))?;

    let key_path = ssh_path.join(&name);
    if key_path.exists() {
        return Err(format!("Key '{}' already exists", name));
    }

    let algo = key_type.unwrap_or_else(|| "ed25519".to_string());
    let cmt = comment.unwrap_or_else(|| format!("devpod-{}", name));

    let output = Command::new("ssh-keygen")
        .args(["-t", &algo, "-C", &cmt, "-N", "", "-f"])
        .arg(&key_path)
        .output()
        .await
        .map_err(|e| format!("Failed to run ssh-keygen: {}", e))?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(format!("ssh-keygen failed: {}", stderr));
    }

    // Read the generated public key
    let pub_path = ssh_path.join(format!("{}.pub", name));
    let pub_content = fs::read_to_string(&pub_path)
        .await
        .map_err(|e| format!("Failed to read generated public key: {}", e))?
        .trim()
        .to_string();

    let parts: Vec<&str> = pub_content.splitn(3, ' ').collect();
    let key_type_parsed = parts.first().unwrap_or(&"").to_string();

    // Get fingerprint
    let fingerprint = match Command::new("ssh-keygen")
        .args(["-l", "-f"])
        .arg(&pub_path)
        .output()
        .await
    {
        Ok(out) if out.status.success() => {
            String::from_utf8_lossy(&out.stdout).trim().to_string()
        }
        _ => String::new(),
    };

    Ok(SshKeyInfo {
        name,
        key_type: key_type_parsed,
        fingerprint,
        comment: cmt,
        public_key: pub_content,
        path: key_path.to_string_lossy().to_string(),
        has_passphrase: false,
    })
}
