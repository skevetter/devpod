---
id: rename-provider
title: Rename a Provider
---

You can rename a provider using the `devpod provider rename` command. This is useful for organizing your providers and giving them more descriptive names. The rename operation works by cloning the existing provider with the new name, automatically rebinding all associated workspaces to use the new provider name, and then cleaning up the old provider.

## CLI

To rename a provider via the command-line interface (CLI), use the following command:

```bash
devpod provider rename [CURRENT_NAME] [NEW_NAME]
```

### Arguments

-   `CURRENT_NAME`: The current name of the provider you want to rename.
-   `NEW_NAME`: The new name for the provider.

### Example

If you have a provider named `my-docker` and you want to rename it to `local-docker`, you would run:

```bash
devpod provider rename my-docker local-docker
```

## Behavior

The rename operation performs the following steps:

1.  Clones the existing provider with the new name
2.  Automatically rebinds all workspaces associated with the old provider to use the new provider name
3.  If all workspace rebinding succeeds and the provider being renamed is the default provider, updates the default provider setting to the new name
4.  Cleans up the old provider after successful rebinding and default provider update (if applicable)
5.  If any workspace rebinding or default provider update fails, the operation rolls back by reverting the workspace configurations, restoring the default provider setting (if it was changed), and deleting the cloned provider

## GUI

You can also rename a provider from the DevPod desktop application:

1.  Navigate to the **Providers** section.
2.  Select the provider you want to rename.
3.  In the provider's configuration page, you will find an editable text field for the provider name.
4.  Change the name to your desired new name.
5.  Click **Update Options** to save the changes.

After renaming, DevPod will automatically update the provider's configuration and rebind associated workspaces.
