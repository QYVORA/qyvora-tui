#!/usr/bin/env bun

import { createCliRenderer } from "@opentui/core"
import { ChatView } from "../features/chat/ChatView.js"
import { FdTransport } from "../shared/fd-transport.js"
import { readAppOptions } from "../shared/options.js"

async function main(): Promise<void> {
  const options = readAppOptions()
  const renderer = await createCliRenderer({
    stdin: process.stdin,
    stdout: process.stdout,
    exitOnCtrlC: false,
    useMouse: false,
  })

  let view: ChatView | undefined
  let stopping = false
  let exitCode = 0

  const stop = (code: number): void => {
    if (stopping) return
    stopping = true
    exitCode = code
    transport.close()
    view?.destroy()
    renderer.destroy()
  }

  const transport = new FdTransport({
    eventsFd: options.eventsFd,
    commandsFd: options.commandsFd,
    onMessage: (message) => view?.applyMessage(message),
    onError: (error) => {
      process.stderr.write(`qyvora-ai-tui: ${error.message}\n`)
      stop(1)
    },
    onEnd: () => stop(0),
  })

  view = new ChatView(renderer, {
    title: options.title,
    version: options.version,
    noColor: options.noColor,
    onCommand: (command) => transport.send(command),
  })

  renderer.start()
  transport.send({ type: "ready" })

  await renderer.closed.catch((error: unknown) => {
    process.stderr.write(`qyvora-ai-tui: ${error instanceof Error ? error.message : String(error)}\n`)
    exitCode = 1
  })
  transport.close()
  process.exit(exitCode)
}

main().catch((error: unknown) => {
  process.stderr.write(`qyvora-ai-tui: ${error instanceof Error ? error.message : String(error)}\n`)
  process.exit(1)
})
