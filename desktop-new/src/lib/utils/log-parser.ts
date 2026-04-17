// biome-ignore lint/suspicious/noControlCharactersInRegex: ANSI escape stripping requires matching ESC (0x1b)
const ANSI_RE = /\x1b\[[0-9;]*m/g
const BRACKET_RE = /\[0[;0-9]*m/g

/** Strip ANSI escape codes from a string */
export function stripAnsi(str: string): string {
  return str.replace(ANSI_RE, "").replace(BRACKET_RE, "")
}

export interface ParsedLogLine {
  time: string
  level: "info" | "warn" | "fatal" | "debug" | "error" | ""
  message: string
  source: string
}

/**
 * Parse a DevPod CLI log line into structured fields.
 * Format: `<time> <level> <message> <source.go:line>`
 * ANSI codes are stripped before parsing.
 */
export function parseLogLine(raw: string): ParsedLogLine {
  const clean = stripAnsi(raw)

  // Match: `HH:MM:SS level message source.go:NNN`
  const match = clean.match(
    /^(\d{1,2}:\d{2}:\d{2})\s+(info|warn|fatal|debug|error)\s+(.*?)\s+(\S+\.\w+:\d+)\s*$/,
  )

  if (match) {
    return {
      time: match[1],
      level: match[2] as ParsedLogLine["level"],
      message: match[3],
      source: match[4],
    }
  }

  // Continuation or unstructured line
  return { time: "", level: "", message: clean.trim(), source: "" }
}
