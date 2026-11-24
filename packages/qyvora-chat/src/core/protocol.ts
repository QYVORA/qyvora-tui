export type ChatRole = "user" | "assistant" | "tool"

export type ChatEventType = "message" | "tool" | "status" | "error"

export interface WireChatEvent {
  type: ChatEventType
  role?: ChatRole
  text?: string
  tool?: string
  status?: string
  error?: string
}

export interface SessionMessage {
  type: "session"
  title: string
  version: string
  sessionId: string
  runner: string
}

export interface EventMessage {
  type: "event"
  event: WireChatEvent
}

export interface DoneMessage {
  type: "done"
  ok: boolean
  error?: string
}

export type HostMessage = SessionMessage | EventMessage | DoneMessage

export interface ReadyCommand {
  type: "ready"
}

export interface SubmitCommand {
  type: "submit"
  text: string
}

export interface CancelCommand {
  type: "cancel"
}

export interface QuitCommand {
  type: "quit"
}

export type TuiCommand = ReadyCommand | SubmitCommand | CancelCommand | QuitCommand

export const MAX_LINE_BYTES = 1024 * 1024

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function optionalString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined
}

export function decodeHostMessage(line: string): HostMessage | undefined {
  let raw: unknown
  try {
    raw = JSON.parse(line)
  } catch {
    return undefined
  }
  if (!isRecord(raw)) return undefined

  if (raw.type === "session") {
    if (
      typeof raw.title !== "string" ||
      typeof raw.version !== "string" ||
      typeof raw.sessionId !== "string" ||
      typeof raw.runner !== "string"
    ) {
      return undefined
    }
    return {
      type: "session",
      title: raw.title,
      version: raw.version,
      sessionId: raw.sessionId,
      runner: raw.runner,
    }
  }

  if (raw.type === "event") {
    const event = decodeChatEvent(raw.event)
    return event ? { type: "event", event } : undefined
  }

  if (raw.type === "done") {
    if (typeof raw.ok !== "boolean") return undefined
    return { type: "done", ok: raw.ok, error: optionalString(raw.error) }
  }

  return undefined
}

export function decodeChatEvent(value: unknown): WireChatEvent | undefined {
  if (!isRecord(value)) return undefined
  const role = value.role
  if (value.type !== "message" && value.type !== "tool" && value.type !== "status" && value.type !== "error") {
    return undefined
  }
  if (role !== undefined && role !== "user" && role !== "assistant" && role !== "tool") {
    return undefined
  }
  return {
    type: value.type,
    role,
    text: optionalString(value.text),
    tool: optionalString(value.tool),
    status: optionalString(value.status),
    error: optionalString(value.error),
  }
}

export function encodeTuiCommand(command: TuiCommand): string {
  return `${JSON.stringify(command)}\n`
}

export class LineDecoder {
  private readonly decoder = new TextDecoder()
  private buffer = ""

  push(chunk: string | Uint8Array): string[] {
    this.buffer += typeof chunk === "string" ? chunk : this.decoder.decode(chunk, { stream: true })
    if (Buffer.byteLength(this.buffer, "utf8") > MAX_LINE_BYTES * 2) {
      throw new Error("protocol line too large")
    }
    const lines: string[] = []
    let index = this.buffer.indexOf("\n")
    while (index >= 0) {
      lines.push(this.takeLine(this.buffer.slice(0, index)))
      this.buffer = this.buffer.slice(index + 1)
      index = this.buffer.indexOf("\n")
    }
    if (Buffer.byteLength(this.buffer, "utf8") > MAX_LINE_BYTES) {
      throw new Error("protocol line too large")
    }
    return lines
  }

  flush(): string[] {
    const tail = this.decoder.decode()
    this.buffer += tail
    if (this.buffer.length === 0) return []
    const line = this.takeLine(this.buffer)
    this.buffer = ""
    return [line]
  }

  private takeLine(value: string): string {
    const line = value.endsWith("\r") ? value.slice(0, -1) : value
    if (Buffer.byteLength(line, "utf8") > MAX_LINE_BYTES) {
      throw new Error("protocol line too large")
    }
    return line
  }
}
