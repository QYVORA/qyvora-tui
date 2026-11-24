import {
  BoxRenderable,
  InputRenderable,
  InputRenderableEvents,
  ScrollBoxRenderable,
  TextRenderable,
  type CliRenderer,
  type KeyEvent,
} from "@opentui/core"
import { type HostMessage, type TuiCommand } from "../../core/protocol.js"
import { lineColor, theme } from "../../core/theme.js"
import { addSubmittedPrompt, applyHostMessage, initialChatState, type ChatState } from "./state.js"

export interface ChatViewOptions {
  title: string
  version: string
  noColor?: boolean
  onCommand: (command: TuiCommand) => void
}

export class ChatView {
  private readonly renderer: CliRenderer
  private readonly noColor: boolean
  private readonly onCommand: (command: TuiCommand) => void
  private readonly root: BoxRenderable
  private readonly transcript: ScrollBoxRenderable
  private readonly status: TextRenderable
  private readonly meta: TextRenderable
  private readonly input: InputRenderable
  private readonly lineNodes: TextRenderable[] = []
  private state: ChatState = initialChatState()
  private destroyed = false

  constructor(renderer: CliRenderer, options: ChatViewOptions) {
    this.renderer = renderer
    this.noColor = options.noColor === true
    this.onCommand = options.onCommand

    this.root = new BoxRenderable(renderer, {
      id: "qyvora-chat-root",
      width: "100%",
      height: "100%",
      backgroundColor: this.noColor ? undefined : theme.canvas,
    })

    const header = new BoxRenderable(renderer, {
      id: "qyvora-chat-header",
      width: "100%",
      height: 1,
      backgroundColor: this.noColor ? undefined : theme.surface,
    })
    header.add(
      new TextRenderable(renderer, {
        id: "qyvora-chat-title",
        content: `${options.title}  ${options.version}`,
        fg: this.noColor ? undefined : theme.accent,
      }),
    )

    this.meta = new TextRenderable(renderer, {
      id: "qyvora-chat-meta",
      content: "session=pending",
      fg: this.noColor ? undefined : theme.textMuted,
    })

    this.transcript = new ScrollBoxRenderable(renderer, {
      id: "qyvora-chat-transcript",
      width: "100%",
      flexGrow: 1,
      minHeight: 0,
      stickyScroll: true,
      stickyStart: "bottom",
      scrollY: true,
      border: true,
      borderColor: this.noColor ? undefined : theme.border,
      backgroundColor: this.noColor ? undefined : theme.canvas,
      viewportOptions: {
        paddingLeft: 1,
        paddingRight: 1,
        paddingTop: 1,
        paddingBottom: 1,
      },
    })

    this.status = new TextRenderable(renderer, {
      id: "qyvora-chat-status",
      height: 1,
      fg: this.noColor ? undefined : theme.textMuted,
    })

    const prompt = new BoxRenderable(renderer, {
      id: "qyvora-chat-prompt",
      width: "100%",
      height: 3,
      border: true,
      borderColor: this.noColor ? undefined : theme.accent,
      backgroundColor: this.noColor ? undefined : theme.surface,
      title: " message ",
      titleColor: this.noColor ? undefined : theme.accent,
      paddingLeft: 1,
      paddingRight: 1,
    })

    this.input = new InputRenderable(renderer, {
      id: "qyvora-chat-input",
      width: "100%",
      maxLength: 4096,
      placeholder: "Ask a question or /assess <target> [goal]",
      placeholderColor: this.noColor ? undefined : theme.textMuted,
      textColor: this.noColor ? undefined : theme.textPrimary,
      backgroundColor: this.noColor ? undefined : theme.surface,
      focusedBackgroundColor: this.noColor ? undefined : theme.surface,
      cursorColor: this.noColor ? undefined : theme.accent,
    })
    this.input.focus()
    this.input.on(InputRenderableEvents.ENTER, (value: string) => this.submit(value))
    prompt.add(this.input)

    this.root.add(header)
    this.root.add(this.meta)
    this.root.add(this.transcript)
    this.root.add(this.status)
    this.root.add(prompt)
    renderer.root.add(this.root)

    renderer.keyInput.on("keypress", this.handleKey)
    this.renderState()
  }

  private renderState(): void {
    const session = this.state.session
    this.meta.content = session
      ? `session=${session.sessionId}  runner=${session.runner}  version=${session.version}`
      : "session=pending"
    this.status.content = this.state.busy
      ? "working — ctrl+c: cancel  esc: quit"
      : this.state.done
        ? "turn complete — esc: quit"
        : "ready — enter: send  esc: quit"

    while (this.lineNodes.length < this.state.lines.length) {
      const line = this.state.lines[this.lineNodes.length]!
      const node = new TextRenderable(this.renderer, {
        id: `qyvora-chat-line-${line.id}`,
        content: line.text,
        width: "100%",
        wrapMode: "word",
        fg: this.noColor ? undefined : lineColor(line.kind),
      })
      this.lineNodes.push(node)
      this.transcript.add(node)
    }
    this.transcript.scrollTo(this.transcript.scrollHeight)
  }

  private submit(value: string): void {
    const text = value.trim()
    if (!text || this.state.busy || this.destroyed) return
    this.state = addSubmittedPrompt(this.state, text)
    this.input.value = ""
    this.onCommand({ type: "submit", text })
    this.renderState()
    this.input.focus()
  }

  applyMessage(message: HostMessage): void {
    if (this.destroyed) return
    this.state = applyHostMessage(this.state, message)
    this.renderState()
  }

  getState(): ChatState {
    return this.state
  }

  private readonly handleKey = (key: KeyEvent): void => {
    if (key.defaultPrevented) return
    if (key.ctrl && key.name === "c") {
      key.preventDefault()
      if (this.state.busy) this.onCommand({ type: "cancel" })
      else this.quit()
      return
    }
    if (key.name === "escape") {
      key.preventDefault()
      if (this.state.busy) this.onCommand({ type: "cancel" })
      this.quit()
    }
  }

  private quit(): void {
    if (this.destroyed) return
    this.onCommand({ type: "quit" })
    this.destroy()
  }

  destroy(): void {
    if (this.destroyed) return
    this.destroyed = true
    this.renderer.keyInput.off("keypress", this.handleKey)
    this.root.destroyRecursively()
  }
}
