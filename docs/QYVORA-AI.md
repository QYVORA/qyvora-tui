# QYVORA TUI integration

The canonical OpenTUI fork in this repository supplies the interactive chat client for QYVORA AI. `@qyvora/qyvora-chat` is the source of truth for its theme, protocol, and standalone executable.

## Integration contract

The Go binary embeds the compiled `qyvora-ai-tui` executable and launches it with inherited terminal stdin/stdout. Two extra pipes carry structured JSONL:

- child fd 3: host messages (`session`, `event`, `done`)
- child fd 4: TUI commands (`ready`, `submit`, `cancel`, `quit`)

The Go process keeps assessment policy, approvals, persistence, and tool execution. The TUI only renders messages and reports user input. `--no-color` is forwarded as `NO_COLOR`/`QYVORA_TUI_NO_COLOR` and suppresses the app's custom QYVORA colours.

The Go build stages the compiled executable at `platform/qyvora-ai/internal/tuiasset/assets/<goos>_<goarch>/qyvora-ai-tui`; `scripts/build-embedded.sh` builds it from `QYVORA_TUI_ROOT`. The published corrected beta is `https://github.com/QYVORA/qyvora-ai-releases/releases/tag/v0.3.0-beta.2`.

## Build and verification

```bash
bun install
bun run typecheck
bun run --filter '@qyvora/qyvora-chat' test
bun run build:chat
bun run fmt:check
bun run lint
```

See [`packages/qyvora-chat/docs/PROTOCOL.md`](../packages/qyvora-chat/docs/PROTOCOL.md) for the wire contract and package layout.
