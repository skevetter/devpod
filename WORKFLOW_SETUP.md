# GitHub Workflows Setup Guide

This document explains how to configure secrets and variables for the DevPod GitHub workflows.

## Quick Start

For basic testing and CI, only `GITHUB_TOKEN` is needed (automatically provided by GitHub).

For releases, you'll need platform-specific signing credentials.

## Repository Settings

Go to: `Settings` → `Secrets and variables` → `Actions`

### Secrets vs Variables
- **Secrets**: Sensitive data (passwords, keys) - encrypted and hidden in logs
- **Variables**: Non-sensitive configuration (URLs, usernames) - visible in logs

## Required Secrets by Workflow

### E2E Tests (`e2e-tests.yaml`, `e2e-win-full-tests.yaml`)

```
GH_PRIVATE_REPO_USER_TEST     # GitHub username for private repo testing
GH_PRIVATE_REPO_TOKEN_TEST    # GitHub personal access token with repo scope
```

**Setup:**
1. Create a GitHub Personal Access Token at https://github.com/settings/tokens
2. Select scope: `repo` (Full control of private repositories)
3. Add as secret in your repository

---

### Linux Releases (`release-linux.yaml`)

No additional secrets required beyond `GITHUB_TOKEN`.

Optional signing:
```
APP_IMAGE_SIGN                # Set to "1" to enable AppImage signing
APP_IMAGE_SIGN_KEY            # GPG private key for signing
APP_IMAGE_SIGN_PASSPHRASE     # Passphrase for the GPG key
```

**Setup AppImage Signing:**
```bash
# Generate GPG key
gpg --full-generate-key

# Export private key
gpg --export-secret-keys YOUR_KEY_ID | base64

# Add the base64 output as APP_IMAGE_SIGN_KEY secret
```

---

### macOS Releases (`release-macos.yaml`)

```
APPLE_CERTIFICATE              # Base64-encoded .p12 certificate
APPLE_CERTIFICATE_PASSWORD     # Password for the certificate
APPLE_SIGNING_IDENTITY         # Certificate identity (e.g., "Developer ID Application: Your Name")
APPLE_TEAM_ID                  # Apple Developer Team ID (10 characters)
APPLE_ID                       # Apple ID email
APPLE_PASSWORD                 # App-specific password for notarization
```

**Setup macOS Signing:**

1. **Export Certificate from Keychain:**
   ```bash
   # Export from Keychain Access as .p12 file
   # Then convert to base64:
   base64 -i certificate.p12 | pbcopy
   ```
   Add as `APPLE_CERTIFICATE` secret

2. **Get Signing Identity:**
   ```bash
   security find-identity -v -p codesigning
   ```
   Copy the full name (e.g., "Developer ID Application: Company Name (TEAM123)")

3. **Find Team ID:**
   - Go to https://developer.apple.com/account
   - Team ID is shown in the top right (10 characters)

4. **Create App-Specific Password:**
   - Go to https://appleid.apple.com/account/manage
   - Sign in → Security → App-Specific Passwords
   - Generate new password
   - Add as `APPLE_PASSWORD` secret

---

### Windows Releases (`release-windows.yaml`)

```
CODESIGNTOOL_USERNAME          # SSL.com username
CODESIGNTOOL_PASSWORD          # SSL.com password
CODESIGNTOOL_TOTP_SECRET       # TOTP secret for 2FA
CODESIGNTOOL_CREDENTIAL_ID     # SSL.com credential ID
```

**Variables:**
```
CODESIGNTOOL_DOWNLOAD_URL      # URL to download CodeSignTool
```

**Setup Windows Signing:**

1. **Get SSL.com Account:**
   - Sign up at https://www.ssl.com/
   - Purchase a code signing certificate

2. **Get TOTP Secret:**
   - Enable 2FA in SSL.com account
   - When setting up authenticator app, save the secret key
   - Add as `CODESIGNTOOL_TOTP_SECRET`

3. **Get Credential ID:**
   - Log into SSL.com eSigner dashboard
   - Find your credential ID in certificate details

4. **CodeSignTool Download:**
   - Get download URL from SSL.com documentation
   - Add as repository variable: `CODESIGNTOOL_DOWNLOAD_URL`

---

### License Updates (`go-licenses.yaml`)

```
GH_ACCESS_TOKEN               # GitHub PAT with repo write access
```

**Setup:**
1. Create token at https://github.com/settings/tokens
2. Select scopes: `repo`, `workflow`
3. Add as secret

---

## Optional Secrets (Original Release Workflow)

If using the original `release.yaml`:

```
DEVPOD_TELEMETRY_PRIVATE_KEY  # Private key for telemetry (can be empty for forks)
CRANE_PRIVATE_KEY             # Private key for crane signing (can be empty)
TAURI_PRIVATE_KEY             # Tauri updater signing key
TAURI_KEY_PASSWORD            # Password for Tauri key
```

**Generate Tauri Keys:**
```bash
# Install Tauri CLI
npm install -g @tauri-apps/cli

# Generate key pair
tauri signer generate

# Add private key as TAURI_PRIVATE_KEY
# Add password as TAURI_KEY_PASSWORD
```

---

## Testing Without Secrets

You can test workflows without signing:

1. **Remove signing steps** from workflow files
2. **Comment out** environment variables for signing
3. **Use `workflow_dispatch`** to manually trigger builds

Example: Remove these sections from macOS workflow:
```yaml
env:
  ENABLE_CODE_SIGNING: ${{ secrets.APPLE_CERTIFICATE }}
  APPLE_CERTIFICATE: ${{ secrets.APPLE_CERTIFICATE }}
  # ... other signing vars
```

---

## Workflow Triggers

### Automatic Triggers
- **E2E Tests**: On PR to main, when Go files change
- **Releases**: When a prerelease is published

### Manual Triggers
All workflows support `workflow_dispatch` for manual runs:
1. Go to `Actions` tab
2. Select workflow
3. Click `Run workflow`
4. Choose branch and run

---

## Troubleshooting

### "Unable to find prerelease for this workflow"
- Ensure you created a **prerelease** (not a full release)
- Tag must start with `v` (e.g., `v0.1.0-alpha.1`)

### macOS signing fails
- Verify certificate is not expired
- Check Team ID is exactly 10 characters
- Ensure app-specific password is correct

### Windows signing fails
- Verify TOTP secret is correct (test with authenticator app)
- Check cred`ential ID matches SSL.com dashboard
- Ensure CodeSignTool URL is accessible

### Self-hosted runner required
Some workflows need `self-hosted-windows` runner. Either:
- Set up a self-hosted runner
- Change to `windows-latest` (may have limitations)

---

## Minimal Setup for Testing

To get started quickly:

1. **Fork the repository**
2. **No secrets needed** for basic builds
3. **Trigger manually**: Actions → Select workflow → Run workflow
4. **Artifacts won't be signed** but will build successfully

Add signing secrets only when ready to distribute releases.
