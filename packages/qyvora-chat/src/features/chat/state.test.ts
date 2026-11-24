import { describe, expect, test } from "bun:test"
import { addSubmittedPrompt, applyHostMessage, eventToLine, initialChatState } from "./state.js"

describe("eventToLine", () => {
  test("maps message roles", () => {
    expect(eventToLine({ type: "message", role: "user", text: "hello" })).toEqual({
      kind: "user",
      text: "hello",
    })
    expect(eventToLine({ type: "message", role: "assistant", text: "answer" })).toEqual({
      kind: "assistant",
      text: "answer",
    })
  })

  test("formats tool, status, and error events", () => {
    expect(eventToLine({ type: "tool", tool: "scan", text: "complete" })).toEqual({
      kind: "tool",
      text: "[scan] complete",
    })
    expect(eventToLine({ type: "status", status: "thinking" })).toEqual({
      kind: "status",
      text: "* thinking",
    })
    expect(eventToLine({ type: "error", error: "boom" })).toEqual({
      kind: "error",
      text: "error: boom",
    })
  })

  test("ignores empty content", () => {
    expect(eventToLine({ type: "status", status: "  " })).toBeUndefined()
    expect(eventToLine({ type: "message", text: "" })).toBeUndefined()
  })
})

describe("applyHostMessage", () => {
  test("records session metadata", () => {
    const state = applyHostMessage(initialChatState(), {
      type: "session",
      title: "QYVORA / AFRICA AI",
      version: "0.3.0",
      sessionId: "ai-1",
      runner: "qyvora-ai",
    })
    expect(state.ready).toBe(true)
    expect(state.session?.sessionId).toBe("ai-1")
  })

  test("marks busy on submit and complete on done", () => {
    const submitted = addSubmittedPrompt(initialChatState(), "hello")
    expect(submitted.busy).toBe(true)
    expect(submitted.lines.at(-1)).toMatchObject({ kind: "user", text: "hello" })

    const done = applyHostMessage(submitted, { type: "done", ok: true })
    expect(done.busy).toBe(false)
    expect(done.done).toBe(true)
  })

  test("adds an error line for a failed turn", () => {
    const done = applyHostMessage(initialChatState(), { type: "done", ok: false, error: "failed" })
    expect(done.lines.at(-1)).toMatchObject({ kind: "error", text: "error: failed" })
  })
})
