import { client } from "@/client/client"
import { TActionID } from "@/contexts"
import { useToast } from "@chakra-ui/react"
import { useMutation } from "@tanstack/react-query"
import * as dialog from "@tauri-apps/plugin-dialog"

export function useDownloadLogs() {
  const toast = useToast()
  const { mutate, isPending: isDownloading } = useMutation({
    mutationFn: async ({ actionID }: { actionID: TActionID }) => {
      const actionLogFile = (await client.workspaces.getActionLogFile(actionID)).unwrap()

      if (actionLogFile === undefined) {
        throw new Error(`Unable to retrieve file for action ${actionID}`)
      }

      const targetFile = await dialog.save({
        title: "Save Logs",
        filters: [{ name: "format", extensions: ["log", "txt"] }],
      })

      // user cancelled "save file" dialog
      if (targetFile === null) {
        return false
      }

      await client.copyFile(actionLogFile, targetFile)

      return true
    },
    onError(error) {
      toast({
        title: `Failed to save logs: ${error}`,
        status: "error",
        isClosable: true,
        duration: 30_000, // 30 sec
      })
    },
    onSuccess(fileWasSaved) {
      if (fileWasSaved) {
        toast({
          title: "Logs saved successfully",
          status: "success",
          isClosable: true,
          duration: 5_000,
        })
      }
    },
  })

  return { download: mutate, isDownloading }
}
