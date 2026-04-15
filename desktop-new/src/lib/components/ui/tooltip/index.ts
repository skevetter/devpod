import { Tooltip as TooltipPrimitive } from "bits-ui"
import Content from "./tooltip-content.svelte"

const Root = TooltipPrimitive.Root
const Trigger = TooltipPrimitive.Trigger
const Provider = TooltipPrimitive.Provider

export {
  Content,
  Content as TooltipContent,
  Provider,
  Provider as TooltipProvider,
  Root,
  Root as Tooltip,
  Trigger,
  Trigger as TooltipTrigger,
}
