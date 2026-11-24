import type { HostMessage, WireChatEvent } from "../../core/protocol.js"

export type ChatLineKind = "user" | "assistant" | "tool" | "status" | "error"

export interface ChatLine {
  id: number
  kind: ChatLineKind
  text: string
}

export interface ChatSession {
  title: string
  version: string
  sessionId: string
  runner: string
}

export interface ChatState {
  session: ChatSession | undefined
  lines: ChatLine[]
  busy: boolean
  done: boolean
  ready: boolean
  nextId: number
}

export function initialChatState(): ChatState {
  return {
    session: undefined,
    lines: [
      {
        id: 1,
        kind: "status",
        text: "Session ready. Type /assess <target> to plan an assessment.",
      },
    ],
    busy: false,
    done: false,
    ready: false,
    nextId: 2,
  }
}

export function appendLine(state: ChatState, kind: ChatLineKind, text: string): ChatState {
  const trimmed = text.trim()
  if (!trimmed) return state
  return {
    ...state,
    lines: [...state.lines, { id: state.nextId, kind, text: trimmed }],
    nextId: state.nextId + 1,
  }
}

export function addSubmittedPrompt(state: ChatState, text: string): ChatState {
  return appendLine({ ...state, busy: true, done: false }, "user", text)
}

export function eventToLine(event: WireChatEvent): { kind: ChatLineKind; text: string } | undefined {
  switch (event.type) {
    case "message": {
      const role = event.role ?? "assistant"
      const text = event.text ?? ""
      if (!text.trim()) return undefined
      return { kind: role === "user" ? "user" : "assistant", text }
    }
    case "tool": {
      const tool = event.tool ?? "tool"
      const text = event.text ?? "updated"
      return { kind: "tool", text: `[${tool}] ${text}` }
    }
    case "status": {
      const status = event.status ?? event.text ?? ""
      if (!status.trim()) return undefined
      return { kind: "status", text: `* ${status}` }
    }
    case "error": {
      const text = event.error ?? event.text ?? "unknown error"
      return { kind: "error", text: `error: ${text}` }
    }
  }
}

export function applyHostMessage(state: ChatState, message: HostMessage): ChatState {
  switch (message.type) {
    case "session":
      return {
        ...state,
        ready: true,
        session: {
          title: message.title,
          version: message.version,
          sessionId: message.sessionId,
          runner: message.runner,
        },
      }
    case "event": {
      const line = eventToLine(message.event)
      return line ? appendLine(state, line.kind, line.text) : state
    }
    case "done": {
      const next = { ...state, busy: false, done: true }
      return message.ok ? next : appendLine(next, "error", `error: ${message.error ?? "request failed"}`)
    }
  }
}
