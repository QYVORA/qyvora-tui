import { describe, expect, test } from "bun:test"
import { readAppOptions } from "./options.js"

describe("readAppOptions", () => {
  test("uses defaults", () => {
    expect(readAppOptions({})).toEqual({
      eventsFd: 3,
      commandsFd: 4,
      title: "QYVORA / AFRICA AI",
      version: "0.1.0",
      noColor: false,
    })
  })

  test("reads host-provided configuration", () => {
    expect(
      readAppOptions({
        QYVORA_TUI_EVENTS_FD: "5",
        QYVORA_TUI_COMMANDS_FD: "6",
        QYVORA_TUI_TITLE: "Assessment",
        QYVORA_TUI_VERSION: "0.3.0",
        QYVORA_TUI_NO_COLOR: "1",
      }),
    ).toEqual({
      eventsFd: 5,
      commandsFd: 6,
      title: "Assessment",
      version: "0.3.0",
      noColor: true,
    })
  })

  test("rejects invalid descriptors", () => {
    expect(() => readAppOptions({ QYVORA_TUI_EVENTS_FD: "0" })).toThrow("file descriptor")
    expect(() => readAppOptions({ QYVORA_TUI_COMMANDS_FD: "abc" })).toThrow("file descriptor")
  })
})
