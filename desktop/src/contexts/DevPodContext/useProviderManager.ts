import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useMemo } from "react"
import { client } from "@/client"
import { exists } from "@/lib"
import { QueryKeys } from "@/queryKeys"
import { TProviderManager, TProviders, TWithProviderID } from "@/types"

export function useProviderManager(): TProviderManager {
  const queryClient = useQueryClient()

  const removeMutation = useMutation({
    mutationFn: async ({ providerID }: TWithProviderID) =>
      (await client.providers.remove(providerID)).unwrap(),
    onMutate({ providerID }) {
      queryClient.cancelQueries(QueryKeys.PROVIDERS)
      const oldProviderSnapshot = queryClient.getQueryData<TProviders>(QueryKeys.PROVIDERS)?.[
        providerID
      ]

      queryClient.setQueryData<TProviders>(QueryKeys.PROVIDERS, (current) => {
        const shallowCopy = { ...current }
        delete shallowCopy[providerID]

        return shallowCopy
      })

      return { oldProviderSnapshot }
    },
    onError(_, { providerID }, ctx) {
      const maybeOldProvider = ctx?.oldProviderSnapshot
      if (exists(maybeOldProvider)) {
        queryClient.setQueryData<TProviders>(QueryKeys.PROVIDERS, (current) => ({
          ...current,
          [providerID]: maybeOldProvider,
        }))
      }
    },
    onSuccess(_, { providerID }) {
      queryClient.invalidateQueries(QueryKeys.provider(providerID))
    },
  })

  const renameMutation = useMutation({
    mutationFn: async ({
      oldProviderID,
      newProviderID,
    }: {
      oldProviderID: string
      newProviderID: string
    }) => (await client.providers.rename(oldProviderID, newProviderID)).unwrap(),
    onMutate({ oldProviderID, newProviderID }) {
      queryClient.cancelQueries(QueryKeys.PROVIDERS)
      const oldProviders = queryClient.getQueryData<TProviders>(QueryKeys.PROVIDERS)
      const oldProviderData = oldProviders?.[oldProviderID]

      queryClient.setQueryData<TProviders>(QueryKeys.PROVIDERS, (current) => {
        if (!current) return current
        const shallowCopy = { ...current }
        delete shallowCopy[oldProviderID]
        if (oldProviderData) {
          shallowCopy[newProviderID] = oldProviderData
          if (shallowCopy[newProviderID].config) {
            shallowCopy[newProviderID] = {
              ...shallowCopy[newProviderID],
              config: {
                ...shallowCopy[newProviderID].config!,
                name: newProviderID,
              },
            }
          }
        }

        return shallowCopy
      })

      return { oldProviderData }
    },
    onError(_, { oldProviderID, newProviderID }, ctx) {
      const maybeOldProvider = ctx?.oldProviderData
      if (exists(maybeOldProvider)) {
        queryClient.setQueryData<TProviders>(QueryKeys.PROVIDERS, (current) => {
          if (!current) return current
          const shallowCopy = { ...current }
          delete shallowCopy[newProviderID]
          shallowCopy[oldProviderID] = maybeOldProvider

          return shallowCopy
        })
      }
    },
    onSuccess(_, { oldProviderID, newProviderID }) {
      queryClient.invalidateQueries(QueryKeys.provider(oldProviderID))
      queryClient.invalidateQueries(QueryKeys.provider(newProviderID))
      queryClient.invalidateQueries(QueryKeys.PROVIDERS)
    },
  })

  return useMemo(
    () => ({
      remove: {
        run: removeMutation.mutate,
        status: removeMutation.status,
        error: removeMutation.error,
        target: removeMutation.variables,
      },
      rename: {
        run: ({ oldProviderID, newProviderID }: { oldProviderID: string; newProviderID: string }) =>
          renameMutation.mutateAsync({ oldProviderID, newProviderID }),
        status: renameMutation.status,
        error: renameMutation.error,
        target: renameMutation.variables,
      },
    }),
    [removeMutation, renameMutation]
  )
}
