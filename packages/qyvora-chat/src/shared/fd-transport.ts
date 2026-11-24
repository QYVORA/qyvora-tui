import { createReadStream, writeSync } from "node:fs"
import {
  decodeHostMessage,
  encodeTuiCommand,
  LineDecoder,
  type HostMessage,
  type TuiCommand,
} from "../core/protocol.js"

export interface TransportOptions {
  eventsFd: number
  commandsFd: number
  onMessage: (message: HostMessage) => void
  onError: (error: Error) => void
  onEnd: () => void
}

export class FdTransport {
  private readonly options: TransportOptions
  private readonly decoder = new LineDecoder()
  private readonly stream
  private ended = false

  constructor(options: TransportOptions) {
    this.options = options
    this.stream = createReadStream("", { fd: options.eventsFd, autoClose: false })
    this.stream.on("data", (chunk: Buffer | string) => this.handleData(chunk))
    this.stream.on("error", (error: Error) => this.handleError(error))
    this.stream.on("end", () => this.handleEnd())
  }

  send(command: TuiCommand): void {
    if (this.ended) return
    const data = Buffer.from(encodeTuiCommand(command), "utf8")
    let offset = 0
    while (offset < data.length) {
      offset += writeSync(this.options.commandsFd, data, offset, data.length - offset)
    }
  }

  close(): void {
    this.ended = true
    this.stream.destroy()
  }

  private handleData(chunk: Buffer | string): void {
    if (this.ended) return
    try {
      for (const line of this.decoder.push(chunk)) {
        const message = decodeHostMessage(line)
        if (message) this.options.onMessage(message)
      }
    } catch (error) {
      this.handleError(error instanceof Error ? error : new Error(String(error)))
    }
  }

  private handleError(error: Error): void {
    if (this.ended) return
    this.close()
    this.options.onError(error)
  }

  private handleEnd(): void {
    if (this.ended) return
    this.close()
    try {
      for (const line of this.decoder.flush()) {
        const message = decodeHostMessage(line)
        if (message) this.options.onMessage(message)
      }
    } catch (error) {
      this.options.onError(error instanceof Error ? error : new Error(String(error)))
      return
    }
    this.options.onEnd()
  }
}
