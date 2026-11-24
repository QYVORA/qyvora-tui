import { describe, expect, test } from "bun:test"
import { decodeChatEvent, decodeHostMessage, encodeTuiCommand, LineDecoder, MAX_LINE_BYTES } from "./protocol.js"

describe("decodeHostMessage", () => {
  test("decodes a session message", () => {
    expect(
      decodeHostMessage(
        JSON.stringify({
          type: "session",
          title: "QYVORA / AFRICA AI",
          version: "0.3.0",
          sessionId: "ai-1",
          runner: "qyvora-ai",
        }),
      ),
    ).toEqual({
      type: "session",
      title: "QYVORA / AFRICA AI",
      version: "0.3.0",
      sessionId: "ai-1",
      runner: "qyvora-ai",
    })
  })

  test("decodes event and done messages", () => {
    expect(decodeHostMessage(JSON.stringify({ type: "event", event: { type: "status", status: "thinking" } }))).toEqual(
      { type: "event", event: { type: "status", status: "thinking" } },
    )
    expect(decodeHostMessage(JSON.stringify({ type: "done", ok: true }))).toEqual({
      type: "done",
      ok: true,
      error: undefined,
    })
  })

  test("rejects invalid messages", () => {
    expect(decodeHostMessage("not-json")).toBeUndefined()
    expect(decodeHostMessage("[]")).toBeUndefined()
    expect(decodeHostMessage(JSON.stringify({ type: "done", ok: "yes" }))).toBeUndefined()
    expect(decodeHostMessage(JSON.stringify({ type: "event", event: { type: "unknown" } }))).toBeUndefined()
    expect(decodeChatEvent({ type: "message", role: "system" })).toBeUndefined()
  })
})

describe("encodeTuiCommand", () => {
  test("writes one newline-delimited JSON command", () => {
    expect(encodeTuiCommand({ type: "submit", text: "hello" })).toBe('{"type":"submit","text":"hello"}\n')
    expect(encodeTuiCommand({ type: "ready" })).toBe('{"type":"ready"}\n')
  })
})

describe("LineDecoder", () => {
  test("handles chunk boundaries and CRLF", () => {
    const decoder = new LineDecoder()
    expect(decoder.push('{"a":1}\r\n{"')).toEqual(['{"a":1}'])
    expect(decoder.push('b":2}\n')).toEqual(['{"b":2}'])
    expect(decoder.flush()).toEqual([])
  })

  test("decodes UTF-8 split across chunks", () => {
    const decoder = new LineDecoder()
    const bytes = Buffer.from("héllo\n", "utf8")
    expect(decoder.push(bytes.subarray(0, 2))).toEqual([])
    expect(decoder.push(bytes.subarray(2))).toEqual(["héllo"])
  })

  test("flushes a final unterminated line", () => {
    const decoder = new LineDecoder()
    expect(decoder.push("tail")).toEqual([])
    expect(decoder.flush()).toEqual(["tail"])
  })

  test("rejects oversized lines", () => {
    const decoder = new LineDecoder()
    const oversized = "x".repeat(MAX_LINE_BYTES + 1)
    expect(() => decoder.push(oversized)).toThrow("protocol line too large")
  })
})
