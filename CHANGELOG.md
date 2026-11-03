# Changelog

## v0.12.0 - 2026-11-03

- Added `ChatRunner`, `ChatConfig`, `ChatEvent`, `ChatMessage` and `RunChat` for
  the shared AI terminal. The command-console `Run` API is unchanged.
- Added chat transcript rendering, streaming-delta support, local help/clear/quit
  commands, input history, scrolling and cancellation.
- Kept the chat view within the terminal height and documented the AI terminal
  key map and the runner-owned approval boundary.

## v0.11.1 - 2026-10-31

- Added CI workflow and release metadata.
- Documented that the per-tool interactive session is retired; shared CLI
  banner helpers remain supported.

## v0.11.0

- Shared TUI and CLI helpers.
