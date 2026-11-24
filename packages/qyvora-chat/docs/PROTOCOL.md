# QYVORA AI chat

`qyvora-ai-tui` is the standalone OpenTUI chat client embedded by the QYVORA AI Go binary. It renders the QYVORA theme and exchanges newline-delimited JSON with its host over inherited file descriptors.

## Layout

- `src/app/main.ts` creates the renderer, transport, and chat view.
- `src/core/protocol.ts` owns the JSONL wire contract.
- `src/core/theme.ts` owns QYVORA colors.
- `src/features/chat/state.ts` owns transcript state transitions.
- `src/features/chat/ChatView.ts` owns the OpenTUI view.
- `src/shared/fd-transport.ts` reads host events and writes TUI commands.
- `scripts/build.ts` compiles the standalone executable.

## Wire protocol

The host maps child fd 3 to host-to-TUI events and child fd 4 to TUI-to-host commands. The environment variables `QYVORA_TUI_EVENTS_FD` and `QYVORA_TUI_COMMANDS_FD` can override those defaults.

Host messages:

```json
{"type":"session","title":"QYVORA / AFRICA AI","version":"0.3.0","sessionId":"ai-1","runner":"qyvora-ai"}
{"type":"event","event":{"type":"status","status":"thinking"}}
{"type":"done","ok":true}
```

TUI commands:

```json
{"type":"ready"}
{"type":"submit","text":"hello"}
{"type":"cancel"}
{"type":"quit"}
```

Each JSON object is one UTF-8 line. Lines larger than 1 MiB are rejected.

## Commands

Run these from the repository root:

```bash
bun install
bun run --filter '@qyvora/qyvora-chat' typecheck
bun run --filter '@qyvora/qyvora-chat' test
bun run build:chat
```

The build writes `packages/qyvora-chat/dist/qyvora-ai-tui-bun-linux-x64` for the host target. Use `--target=bun-linux-x64-musl` for musl or `--target=bun-darwin-arm64` for another installed target.
