# qyvora-tui

**Status:** `CURRENT` · **Version:** `v0.12.0`

The shared interactive terminal application for the QYVORA security toolchain.

All fourteen QYVORA frameworks expose the same interactive experience, built once
here rather than fourteen times. A framework supplies a `tui.Runner`; this module
supplies everything the user actually sees.

## What it is

A presentation and interaction layer. It is not the machine layer, it never
scrapes stdout, and it never parses human-readable output. It consumes the
structured JSONL event envelope every QYVORA tool already emits:

```json
{"schema_version":"1.0","timestamp":"...","execution_id":"...","framework":"...","level":"info","event":"finding.discovered","data":{}}
```

Because the TUI reads that contract and nothing else, a tool is free to
reword, recolour or drop any text it prints without affecting the interface.

## Use

```go
runner := &tui.InProcessRunner{
	ToolName: "anansi",
	Execute:  cli.ExecuteArgs,        // func(ctx, []string) int
	Meta:     commandsFromCobra(rootCmd),
}

code, err := tui.Run(tui.Config{Title: "QYVORA / ANANSI", Runner: runner})
if tui.IsNotInteractive(err) {
	// Not a terminal: fall through to the one-shot CLI. Piping a tool's
	// output has to keep working exactly as before.
}
```

`ExecRunner` is the alternative for tools whose entry point is not re-entrant:
it runs the tool's own binary with `--events stdout` and reads the same JSONL
off the pipe, so it needs no changes to the tool at all.

## AI terminal mode

The same module now also provides a conversation surface for the QYVORA AI
layer. `Run` remains the command console and is unchanged; `RunChat` is additive:

```go
code, err := tui.RunChat(tui.ChatConfig{
	Runner:    runner,                    // implements tui.ChatRunner
	Title:     "QYVORA / AFRICA AI",
	Version:   "0.3.0",
	SessionID: sessionID,                 // shown in the composer
})
if tui.IsNotInteractive(err) {
	// No terminal: the caller should fall back to a one-shot command.
}
```

A `ChatRunner` owns the turn. It emits `ChatEvent` values for complete messages,
streaming deltas, transient status, tool/workflow blocks, and errors:

```go
type ChatRunner interface {
	Name() string
	Send(ctx context.Context, input string, emit func(ChatEvent) error) error
}
```

The UI owns the transcript, input history, scrolling, cancellation and rendering.
The runner owns model calls, approval policy, tool execution and session
persistence. Approval is deliberately not a UI feature: a runner that needs it
prints the pending action and interprets the next turn itself, so the terminal
cannot execute anything the runner did not authorize.

`ChatEventDelta` is supported for providers that stream tokens; a runner may also
send one complete `ChatEventMessage` per response.

### AI terminal keys

| Key | Effect |
| --- | --- |
| `Enter` | Send the current line |
| `PgUp` / `PgDn` | Scroll the transcript |
| `Ctrl+C` | Stop the current turn; when idle, quit |
| `Ctrl+D` | Quit when the input is empty |
| `Ctrl+L` | Clear the transcript |
| `↑` / `↓` | Message history |
| `/help` `/clear` `/quit` | Built-ins, handled locally |

`/assess <target>` is not a TUI built-in; it is passed to the runner, which may
choose to treat it as a shortcut. The UI never assumes what the runner can do.

### Consuming this module

Add it the ordinary way, then make sure the package is actually **imported** so
the linker keeps it:

```bash
go get github.com/QYVORA/qyvora-tui@v0.8.0
```

```go
import tui "github.com/QYVORA/qyvora-tui"
```

A `require` on its own is not enough. Go drops an unimported dependency from the
build, so a tool can satisfy `go.mod` and still ship a binary with no interface
in it. Verify the *binary*, not the manifest:

```bash
go version -m ./mytool | grep qyvora-tui
```

The line must carry an `h1:` content hash, for example
`dep github.com/QYVORA/qyvora-tui v0.8.0 h1:...`. A version with no hash means
the module was required but never linked.

Both QYVORA checks enforce this rather than trusting review: `qyvora-conformance`
probes every built framework, and each framework's `scripts/verify-artifact.sh`
repeats the check against the released binary.

## Guarantees

- **Non-TTY fallback.** `Run` refuses to start when stdout is not a terminal, so
  `tool scan target | tee log` keeps producing the same output it always did.
- **`NO_COLOR`.** Honoured at startup; every style becomes a no-op, so there is
  one rendering path rather than a plain-text variant to keep in step.
- **Real cancellation.** `Ctrl+C` cancels the execution's context, not the
  interface. The tool shuts down, persists its partial artifacts, and the run is
  reported with exit status 130.
- **Unknown events render.** Event types this build has never seen still appear
  in the transcript, so a tool can add event types without a TUI release.
- **Resize-safe.** The layout is recomputed from the live terminal size; no
  width is hard-coded and narrow terminals stack rather than overflow.

## Interface

| Key | Effect |
| --- | --- |
| `Ctrl+C` | Stop the running execution, keeping partial results |
| `Ctrl+C` (idle) | Quit |
| `Ctrl+D` | Quit, when the input is empty |
| `Ctrl+E` | Export every run's tool output and structured events to a timestamped `.log` file in the current directory |
| `Tab` | Complete from the tool's own command registry |
| `↑` / `↓` | Command history |
| `help` `clear` `quit` | Built-ins, handled locally |

The AI terminal uses a different key map; see the section above.

Export writes a new file named `<tool>-session-<timestamp>.log`. Printed output
is exported in full even when the on-screen transcript has folded older lines;
event data is included as JSONL for later inspection.

## Licence

MIT — see [LICENSE](LICENSE).
