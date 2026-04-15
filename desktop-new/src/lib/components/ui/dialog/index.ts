import { Dialog as DialogPrimitive } from "bits-ui"
import Content from "./dialog-content.svelte"
import Description from "./dialog-description.svelte"
import Footer from "./dialog-footer.svelte"
import Header from "./dialog-header.svelte"
import Title from "./dialog-title.svelte"

const Root = DialogPrimitive.Root
const Trigger = DialogPrimitive.Trigger
const Close = DialogPrimitive.Close

export {
  Close,
  Close as DialogClose,
  Content,
  Content as DialogContent,
  Description,
  Description as DialogDescription,
  Footer,
  Footer as DialogFooter,
  Header,
  Header as DialogHeader,
  Root,
  Root as Dialog,
  Title,
  Title as DialogTitle,
  Trigger,
  Trigger as DialogTrigger,
}
