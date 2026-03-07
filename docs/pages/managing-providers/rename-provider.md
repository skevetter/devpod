---
id: rename-provider
title: Rename a Provider
---

You can rename a provider using the `devpod provider rename` command. All workspaces and machines using the provider continue to work under the new name. Provider configuration, options, and state are fully preserved.

## CLI

```bash
devpod provider rename <current-name> <new-name>
```

### Example

```bash
devpod provider rename my-docker local-docker
```

## Constraints

- The new name must be unique — it cannot match an existing provider.
- Provider names can only contain lowercase letters, numbers, and dashes, up to 32 characters.
- Pro providers (proxy/daemon) cannot be renamed. They are managed by the platform.
- Workspaces bound to the provider must be stopped before renaming.

## What happens

- The provider is moved to the new name with all options and settings intact.
- All workspaces and machines associated with the provider are updated to reference the new name.
- If the provider was the default, the default is updated to the new name.
- If any step fails, the entire operation is rolled back to the original state.

## GUI

1. Navigate to the **Providers** section.
2. Select the provider you want to rename.
3. In the provider's configuration page, edit the **Provider Name** field.
4. Click **Update Options** to save.

DevPod will update the provider and all associated workspaces automatically.
