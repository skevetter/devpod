import { Link, Text } from "@chakra-ui/react"
import { client } from "../client"

export function LoftOSSBadge() {
  return (
    <Link
      display="flex"
      alignItems="center"
      justifyContent="start"
      onClick={() => client.open("https://github.com/skevetter/devpod")}>
      <Text fontSize="sm" variant="muted" marginRight="2">
        Maintained by skevetter
      </Text>
    </Link>
  )
}
