export enum ESortWorkspaceMode {
  RECENTLY_USED = "Recently Used",
  LEAST_RECENTLY_USED = "Least Recently Used",
  RECENTLY_CREATED = "Recently Created",
  LEAST_RECENTLY_CREATED = "Least Recently Created",
}

export const DEFAULT_SORT_WORKSPACE_MODE = ESortWorkspaceMode.RECENTLY_USED

type TSortable<TOriginal> = {
  original: TOriginal
  created: number
  used: number
}

export function sortWorkspaces<T>(
  sortables: TSortable<T>[],
  sortMode: ESortWorkspaceMode | undefined
): T[] {
  const copy = [...sortables]

  copy.sort((a, b) => {
    if (sortMode === ESortWorkspaceMode.RECENTLY_USED) {
      return a.used > b.used ? -1 : 1
    }

    if (sortMode === ESortWorkspaceMode.LEAST_RECENTLY_USED) {
      return b.used > a.used ? -1 : 1
    }

    if (sortMode === ESortWorkspaceMode.RECENTLY_CREATED) {
      return a.created > b.created ? -1 : 1
    }

    if (sortMode === ESortWorkspaceMode.LEAST_RECENTLY_CREATED) {
      return b.created > a.created ? -1 : 1
    }

    return 0
  })

  return copy.map((copy) => copy.original)
}
