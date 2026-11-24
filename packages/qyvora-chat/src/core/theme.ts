import type { ChatLineKind } from "../features/chat/state.js"
import type { ChatEventType, ChatRole } from "./protocol.js"

export const theme = {
  canvas: "#000000",
  accent: "#06B66F",
  textPrimary: "#EEF0EE",
  textSecondary: "#A7AAA7",
  textMuted: "#5F625F",
  danger: "#F87171",
  border: "#1B2A22",
  surface: "#07110B",
} as const

export function roleColor(role: ChatRole | undefined): string {
  switch (role) {
    case "user":
      return theme.textSecondary
    case "assistant":
      return theme.accent
    case "tool":
      return theme.textSecondary
    default:
      return theme.textPrimary
  }
}

export function eventColor(type: ChatEventType): string {
  switch (type) {
    case "error":
      return theme.danger
    case "status":
    case "tool":
      return theme.accent
    default:
      return theme.textPrimary
  }
}

export function lineColor(kind: ChatLineKind): string {
  switch (kind) {
    case "user":
    case "tool":
      return theme.textSecondary
    case "assistant":
    case "status":
      return theme.accent
    case "error":
      return theme.danger
  }
}
