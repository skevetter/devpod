import { LegacyRef, useEffect, useRef, useState } from "react"

export function useHover<T extends HTMLButtonElement>(): [boolean, LegacyRef<T>] {
  const [isHovering, setIsHovering] = useState<boolean>(false)

  const ref = useRef<T>(null)

  useEffect(
    () => {
      let timeoutId: NodeJS.Timeout

      const handleMouseEnter = () => {
        clearTimeout(timeoutId)
        setIsHovering(true)
      }

      const handleMouseLeave = () => {
        // Add small delay to prevent flickering when moving between elements
        timeoutId = setTimeout(() => setIsHovering(false), 100)
      }

      setTimeout(() => {
        const node = ref.current
        if (node) {
          node.addEventListener("mouseenter", handleMouseEnter)
          node.addEventListener("mouseleave", handleMouseLeave)

          return () => {
            clearTimeout(timeoutId)
            node.removeEventListener("mouseenter", handleMouseEnter)
            node.removeEventListener("mouseleave", handleMouseLeave)
          }
        }
      })
    },
    // rerun if ref changes!
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [ref.current]
  )

  return [isHovering, ref]
}
