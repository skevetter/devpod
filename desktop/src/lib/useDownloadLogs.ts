import { client } from "@/client/client"
import { TActionID } from "@/contexts"
import { useToast } from "@chakra-ui/react"
import { useMutation } from "@tanstack/react-query"
import * as dialog from "@tauri-apps/plugin-dialog"

export function useDownloadLogs() {
  const toast = useToast()
  const { mutate, isLoading: isDownloading } = useMutation({
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
        return
      }

      await client.copyFile(actionLogFile, targetFile)

      // Due to permissions restrictions with tauri-apps/plugin-opener, the
      // saved log file cannot be opened since it can be saved to any
      // user-specified location.
      // This can be re-enabled if the code is re-implemented to save logs in a
      // fixed location, like $APPDATA

      // client.open(targetFile)
    },
    onError(error) {
      toast({
        title: `Failed to save logs: ${error}`,
        status: "error",
        isClosable: true,
        duration: 30_000, // 30 sec
      })
    },
  })

  return { download: mutate, isDownloading }
}
