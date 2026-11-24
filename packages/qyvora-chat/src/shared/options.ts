export interface AppOptions {
  eventsFd: number
  commandsFd: number
  title: string
  version: string
  noColor: boolean
}

function parseFd(value: string | undefined, fallback: number, name: string): number {
  if (value === undefined || value.trim() === "") return fallback
  const fd = Number(value)
  if (!Number.isInteger(fd) || fd < 3) {
    throw new Error(`${name} must be an integer file descriptor of at least 3`)
  }
  return fd
}

export function readAppOptions(env: NodeJS.ProcessEnv = process.env): AppOptions {
  return {
    eventsFd: parseFd(env.QYVORA_TUI_EVENTS_FD, 3, "QYVORA_TUI_EVENTS_FD"),
    commandsFd: parseFd(env.QYVORA_TUI_COMMANDS_FD, 4, "QYVORA_TUI_COMMANDS_FD"),
    title: env.QYVORA_TUI_TITLE ?? "QYVORA / AFRICA AI",
    version: env.QYVORA_TUI_VERSION ?? "0.1.0",
    noColor: env.NO_COLOR === "1" || env.QYVORA_TUI_NO_COLOR === "1",
  }
}
