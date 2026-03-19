import { client } from "@/client/client"
import { TActionObj } from "@/contexts/DevPodContext/action"
import { TWorkspace } from "@/types"
import { useToast } from "@chakra-ui/react"
import { useMutation } from "@tanstack/react-query"
import { ProWorkspaceInstance } from "@/contexts"
import JSZip from "jszip"

export function useStoreTroubleshoot() {
  const toast = useToast()
  const { mutate, isPending: isStoring } = useMutation({
    mutationFn: async ({
      workspace,
      workspaceActions,
    }: {
      workspace: TWorkspace | ProWorkspaceInstance
      workspaceActions: TActionObj[]
    }) => {
      const logFiles = await Promise.all(
        workspaceActions.map((action) => client.workspaces.getActionLogFile(action.id))
      )

      const targetFolder = await client.selectFromDir("Save Troubleshooting Data")

      // user cancelled "save file" dialog
      if (targetFolder === null) {
        return false
      }

      const unwrappedLogFiles: [src: [string], targetFolder: string][] = logFiles
        .filter((f) => f.ok)
        .map((f) => f.unwrap())
        .map((f) => [[f], f.split(client.pathSeparator()).pop() ?? ""])

      const zip = new JSZip()

      const logFilesData = (
        await Promise.all(
          unwrappedLogFiles.map(async ([src, target]) => {
            try {
              const data = await client.readFile(src)

              return { fileName: target, data }
            } catch {
              // ignore missing log files and continue
              return null
            }
          })
        )
      ).filter((d): d is Exclude<typeof d, null> => d != null)

      logFilesData.forEach((logFile) => {
        zip.file(logFile.fileName, logFile.data)
      })

      zip.file("workspace_actions.json", JSON.stringify(workspaceActions, null, 2))
      zip.file("workspace.json", JSON.stringify(workspace, null, 2))

      const troubleshootOutput = await client.workspaces.troubleshoot({
        id: workspace.id,
        actionID: "",
        streamID: "",
      })

      if (troubleshootOutput.ok) {
        zip.file("cli_troubleshoot.json", troubleshootOutput.unwrap().stdout)
      }

      const out = await zip.generateAsync({ type: "uint8array" })

      await client.writeFile([targetFolder, "devpod_troubleshoot.zip"], out)

      return true
    },
    onError(error) {
      toast({
        title: `Failed to save zip: ${error}`,
        status: "error",
        isClosable: true,
        duration: 30_000, // 30 sec
      })
    },
    onSuccess(fileWasSaved) {
      if (fileWasSaved) {
        toast({
          title: "Troubleshooting data saved successfully",
          status: "success",
          isClosable: true,
          duration: 5_000,
        })
      }
    },
  })

  return { store: mutate, isStoring }
}
