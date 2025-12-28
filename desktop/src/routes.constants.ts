import { Params, Path } from "react-router-dom"
import { TActionID } from "./contexts"
import { TProInstanceDetail, exists } from "./lib"
import { TProviderID, TSupportedIDE, TWorkspaceID } from "./types"

export const Routes = {
  ROOT: "/",
  SETTINGS: "/settings",
  WORKSPACES: "/workspaces",
  ACTIONS: "/actions",
  get ACTION(): string {
    return `${Routes.ACTIONS}/:action`
  },
  get WORKSPACE_CREATE(): string {
    return `${Routes.WORKSPACES}/new`
  },
  toWorkspaceCreate(
    options: Readonly<{
      workspaceID: TWorkspaceID | null
      providerID: TProviderID | null
      ide: string | null
      rawSource: string | null
    }>
  ): Partial<Path> {
    const searchParams = new URLSearchParams()
    for (const [key, value] of Object.entries(options)) {
      if (exists(value)) {
        searchParams.set(key, value)
      }
    }

    return {
      pathname: Routes.WORKSPACE_CREATE,
      search: searchParams.toString(),
    }
  },
  toAction(actionID: TActionID, onSuccess?: string): string {
    if (onSuccess) {
      return `${Routes.ACTIONS}/${actionID}?onSuccess=${encodeURIComponent(onSuccess)}`
    }

    return `${Routes.ACTIONS}/${actionID}`
  },
  getActionID(params: Params<string>): string | undefined {
    // Needs to match `:action` from detail route exactly!
    return params["action"]
  },
  getWorkspaceCreateParamsFromSearchParams(searchParams: URLSearchParams): Partial<
    Readonly<{
      workspaceID: TWorkspaceID
      providerID: TProviderID
      ide: TSupportedIDE
      rawSource: string
    }>
  > {
    return {
      workspaceID: searchParams.get("workspaceID") ?? undefined,
      providerID: searchParams.get("providerID") ?? undefined,
      ide: (searchParams.get("ide") as TSupportedIDE | null) ?? undefined,
      rawSource: searchParams.get("rawSource") ?? undefined,
    }
  },
  PROVIDERS: "/providers",
  get PROVIDER(): string {
    return `${Routes.PROVIDERS}/:provider`
  },
  toProvider(providerID: string): string {
    return `${Routes.PROVIDERS}/${providerID}`
  },
  getProviderId(params: Params<string>): string | undefined {
    // Needs to match `:provider` from detail route exactly!
    return params["provider"]
  },
  PRO: "/pro",
  PRO_INSTANCE: "/pro/:host",
  PRO_WORKSPACE: "/pro/:host/:workspace",
  PRO_WORKSPACE_SELECT_PRESET: "/pro/:host/select-preset",
  PRO_WORKSPACE_CREATE: "/pro/:host/new",
  PRO_SETTINGS: "/pro/:host/user/settings",
  PRO_CREDENTIALS: "/pro/:host/user/credentials",
  PRO_PROFILE: "/pro/:host/user/profile",
  toProInstance(host: string): string {
    return `/pro/${host}`
  },
  toProWorkspace(host: string, instanceID: string): string {
    const base = this.toProInstance(host)

    return `${base}/${instanceID}`
  },
  toProWorkspaceCreate(host: string, fromPreset?: string): string {
    const base = this.toProInstance(host)

    return `${base}/new${fromPreset ? `?fromPreset=${fromPreset}` : ""}`
  },
  toProSelectPreset(host: string): string {
    const base = this.toProInstance(host)

    return `${base}/select-preset`
  },
  toProWorkspaceDetail(host: string, instanceID: string, detail: TProInstanceDetail): string {
    const base = this.toProInstance(host)

    return `${base}/${instanceID}?tab=${detail}`
  },
  toProSettings(host: string): string {
    const base = this.toProInstance(host)

    return `${base}/user/settings`
  },
  toProCredentials(host: string): string {
    const base = this.toProInstance(host)

    return `${base}/user/credentials`
  },
  toProProfile(host: string): string {
    const base = this.toProInstance(host)

    return `${base}/user/profile`
  },
  getProWorkspaceDetailsParams(
    searchParams: URLSearchParams
  ): Partial<Readonly<{ tab: TProInstanceDetail | null }>> {
    return { tab: searchParams.get("tab") as TProInstanceDetail | null }
  },
} as const
