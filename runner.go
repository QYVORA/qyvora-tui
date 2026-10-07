package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Runner executes tool commands on behalf of the TUI and produces structured
// events for them.
//
// Implementations must honour ctx: when it is cancelled, the underlying work
// stops and Run returns promptly. The TUI's Ctrl+C cancels the context it
// passed in, so a Runner that ignores ctx makes Ctrl+C a lie.
//
// Run returns the process exit code. A cancelled execution returns 130, the
// conventional status for SIGINT, so the TUI and a shell agree on what
// interrupting a tool meant.
type Runner interface {
	// Name is the tool's name, used in the header and in messages.
	Name() string

	// Commands returns the command metadata available for completion and help.
	// Tools should source this from their command registry rather than
	// maintaining a second list, so the two cannot drift.
	Commands() []Command

	// Run executes args, writing one JSONL event envelope per line to events.
	// It must not close events; the TUI owns the writer.
	Run(ctx context.Context, args []string, events io.Writer) (int, error)
}

// Command describes a single command for completion and help.
// Per PROMPT1.md §3: Extended model with full metadata while keeping
// backwards compatibility with existing fields.
type Command struct {
	Name  string   // command word as typed
	Short string   // one line, <= 72 chars, no trailing period
	Long  string   // optional paragraph(s) with detailed description
	Usage string   // e.g. "scan <target> [flags]"
	
	// Aliases are alternative names for this command
	Aliases []string
	
	// Group categorizes the command (e.g. "Recon", "Analysis", "Reporting", "Session")
	Group string
	
	// Subs lists subcommands. For backwards compatibility, this can be []string
	// (names only), but should be migrated to []Command for full descriptions.
	Subs []string // TODO: migrate to []Command for recursive structure
	
	// Flags documents available flags for this command
	Flags []Flag
	
	// Examples show common usage patterns
	Examples []Example
	
	// SeeAlso suggests related commands
	SeeAlso []string
	
	// Hidden commands are excluded from help index but still resolvable
	Hidden bool
	
	// Deprecated marks a command as deprecated with replacement suggestion
	Deprecated string
}

// Flag describes a command-line flag.
type Flag struct {
	Name     string // e.g. "--depth"
	Short    string // e.g. "-d"
	Type     string // "int", "string", "bool", "duration", etc.
	Default  string // default value as string
	Desc     string // description
	Required bool   // whether this flag is required
}

// Example shows a usage example with optional explanatory note.
type Example struct {
	Cmd  string // the command line, e.g. "scan example.com --deep"
	Note string // optional explanation
}

// Builtins handled by the TUI itself rather than dispatched to the tool.
var builtins = []Command{
	{Name: "help", Short: "List commands, or describe one"},
	{Name: "clear", Short: "Clear the session transcript"},
	{Name: "quit", Short: "Leave the TUI (Ctrl+D also works)"},
}

// ExecRunner runs commands as child processes of the tool's own binary.
//
// This is the fallback for tools whose in-process entry point is not
// re-entrant. It consumes the same documented `--events stdout` contract as
// everything else, so it never parses human-readable output.
//
// Cancellation sends SIGINT to the child rather than SIGKILL, so the child
// runs its own signal-cancellation path, persists partial artifacts and
// exits 130 -- the same path a real Ctrl+C at a terminal takes.
type ExecRunner struct {
	// Path is the executable to run. Empty means the current process.
	Path string
	// Args are prepended to every invocation, before the event routing flag.
	Args []string
	// Meta supplies completion metadata.
	Meta []Command
	// ToolName is the header name.
	ToolName string
	// Stderr receives the child's human-readable output. When nil it is
	// discarded: the transcript shows events, and interleaving the child's
	// progress bars with the TUI's own frames produces unreadable output.
	Stderr io.Writer
}

func (r *ExecRunner) Name() string { return r.ToolName }

func (r *ExecRunner) Commands() []Command { return r.Meta }

func (r *ExecRunner) Run(ctx context.Context, args []string, events io.Writer) (int, error) {
	path := r.Path
	if path == "" {
		var err error
		path, err = osExecutable()
		if err != nil {
			return 1, err
		}
	}

	// --events stdout is the documented way to route JSONL to stdout while
	// pushing report text to stderr, so the TUI's transcript and the tool's
	// report never fight over the same stream.
	full := append([]string{}, r.Args...)
	full = append(full, "--events", "stdout")
	full = append(full, args...)

	ch, err := startChild(ctx, path, full, r.Stderr)
	if err != nil {
		return 1, err
	}

	// Drain the child's event stream concurrently with waiting on it. A tool
	// emitting events faster than the UI renders would otherwise block on a
	// full pipe and appear to hang.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(events, ch.r)
	}()

	code := ch.wait(ctx)

	// Closing the read end lets the copy goroutine finish. The child is
	// already gone by this point, so nothing is lost.
	_ = ch.r.Close()
	wg.Wait()
	return code, nil
}

// commandArgs renders a command plus its arguments for the transcript.
func commandArgs(args []string) string { return strings.Join(args, " ") }

// tokenize splits a command line the way a shell would for the cases that
// matter here: whitespace separation, single and double quotes, and
// backslash escapes inside double quotes.
//
// It is not a full shell parser and does not try to be -- there is no
// expansion, no substitution and no redirection. A command typed into an
// interactive prompt is an argument vector, not a script, and pretending
// otherwise would invite surprises.
func tokenize(line string) ([]string, error) {
	var (
		args    []string
		cur     strings.Builder
		started bool
		quote   rune
	)
	flush := func() {
		if started {
			args = append(args, cur.String())
			cur.Reset()
			started = false
		}
	}
	for i, r := range line {
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
				continue
			}
			cur.WriteRune(r)
			started = true
		case quote == '"':
			if r == '"' {
				quote = 0
				continue
			}
			if r == '\\' && i+1 < len(line) {
				i++
				cur.WriteRune(rune(line[i]))
				started = true
				continue
			}
			cur.WriteRune(r)
			started = true
		case r == '\'' || r == '"':
			quote = r
			started = true
		case r == '\\' && i+1 < len(line):
			i++
			cur.WriteRune(rune(line[i]))
			started = true
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote", quote)
	}
	flush()
	return args, nil
}
