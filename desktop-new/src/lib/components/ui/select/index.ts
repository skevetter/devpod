import { Select as SelectPrimitive } from "bits-ui"
import Content from "./select-content.svelte"
import Item from "./select-item.svelte"
import Trigger from "./select-trigger.svelte"

const Root = SelectPrimitive.Root
const Group = SelectPrimitive.Group
const Value = SelectPrimitive.Value

export {
  Content,
  Content as SelectContent,
  Group,
  Group as SelectGroup,
  Item,
  Item as SelectItem,
  Root,
  Root as Select,
  Trigger,
  Trigger as SelectTrigger,
  Value,
  Value as SelectValue,
}
