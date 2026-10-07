package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Config configures a TUI session.
type Config struct {
	// Runner executes commands. Required.
	Runner Runner
	// Title is the header label, conventionally "QYVORA / TOOL".
	Title string
	// Version is shown in the header when set.
	Version string
	// Banner is the tool's identity art, drawn at the richest rung of the
	// banner ladder that fits the terminal. A zero value draws nothing and the
	// header carries the name on its own, which is the correct default: most
	// tools are libraries and commands with no wordmark to show.
	//
	// The banner is a TUI-only affordance. It never reaches the one-shot CLI
	// output, because that output is parsed by scripts and compared in tests and
	// a wordmark is not part of any tool's contract.
	Banner Banner
	// In, Out and Err are the streams the TUI owns. Out defaults to stdout.
	In  io.Reader
	Out io.Writer
	// Err receives the tool's human-readable output during execution. Nil keeps
	// it in the transcript, which is the default and normally what is wanted:
	// a capability table, a resolved configuration and the explanation of a
	// failure are all things the operator asked to see. Set it only to divert
	// that text somewhere else entirely.
	Err io.Writer
	// NoColor forces plain output regardless of terminal detection. It is
	// decided once at startup and is not overridable by a Theme, so a tool
	// cannot opt out of a user's NO_COLOR request.
	NoColor bool
	// Capabilities is the tool's capability registry, normalised.
	//
	// It is produced by NormalizeCapabilities from the tool's own
	// `capabilities -o json` output, which is the same command an operator can
	// run by hand. Nothing here is a second hand-maintained list, so a
	// capability added to a tool appears in the interface without a second
	// edit -- the same property CollectCommands gives commands.
	//
	// A nil value is normal and means the tool publishes no registry. The
	// interface stays fully usable without it.
	Capabilities *Capabilities

	// Theme is the tool's optional palette. A nil Theme, or one that sets
	// nothing, resolves to QYVORA_BASE, so branding is never required and never
	// required to be complete: a tool sets the slots it means to change and
	// inherits the rest.
	//
	// A tool with no meaningful brand palette should leave this nil. An
	// invented colour is worse than the shared one, and most tools should
	// inherit.
	Theme *Palette
}

// errNotInteractive means the streams cannot support a full-screen TUI. Tools
// treat it as a cue to fall back to the one-shot CLI rather than as a failure,
// because being piped is a normal way to run a tool.
type errNotInteractive struct{ reason string }

func (e *errNotInteractive) Error() string { return e.reason }

// IsNotInteractive reports whether err means "there is no terminal here", so a
// caller can fall back to ordinary CLI behaviour instead of reporting an error.
func IsNotInteractive(err error) bool {
	var e *errNotInteractive
	return errors.As(err, &e)
}

// runState is what the header status dot reflects.
type runState int

const (
	stateReady runState = iota
	stateRunning
	stateFailed
	stateCancelled
)

func (s runState) String() string {
	switch s {
	case stateRunning:
		return "RUNNING"
	case stateFailed:
		return "FAILED"
	case stateCancelled:
		return "CANCELLED"
	default:
		return "READY"
	}
}

// Messages delivered into the model.
type (
	// batchMsg applies several messages in order. Events arrive faster than a
	// human reads them, so the consumer batches rather than queuing a redraw
	// per line.
	batchMsg []tea.Msg
	// streamMsg delivers one chunk of a run's decoded stream, and the command
	// that continues reading the rest of it. Delivering in chunks while the
	// run is still going is what the live views need: the activity region and
	// a running block's output come from the stream as it happens, not in one
	// lump when the run ends. The last chunk carries a nil next, which ends
	// the chain.
	streamMsg struct {
		items []tea.Msg
		next  tea.Cmd
	}
	// evMsg carries one decoded event envelope.
	evMsg Event
	// runDoneMsg reports an execution's outcome.
	runDoneMsg struct {
		exitCode int
		err      error
		cancel   bool
	}
	// tickMsg drives the elapsed-time and spinner repaints.
	tickMsg time.Time
	// noticeMsg is a transient transcript line the TUI generated.
	noticeMsg struct{ text string }
	// outputMsg is one line of the tool's own printed output. It is kept
	// separate from an event because it has no envelope: the interface shows
	// it as written rather than claiming to have interpreted it.
	outputMsg struct{ line string }
	// capabilitiesMsg carries a tool's capability registry, normally read once
	// at startup from the tool's own `capabilities -o json` output. It is a
	// message so a tool may publish capabilities that change without the
	// interface needing a restart.
	capabilitiesMsg struct{ caps *Capabilities }
)

// model is the application state.
type model struct {
	cfg    Config
	runner Runner
	theme  Theme

	input   textinput.Model
	history *History
	// viewport is the scrolling session area. It scrolls independently of the
	// composer, so history stays reachable while a command runs.
	viewport viewport.Model
	// following is true when the viewport is pinned to the newest output. It is
	// released when the user scrolls back, so arriving events do not yank the
	// view away from what they were reading.
	following bool
	// showEvents reveals the raw event log beneath each execution.
	showEvents bool
	// regionFocus routes the keyboard to the executions region rather than the
	// composer. execSel is the execution entry the region is focused on, and
	// execOffset is the region's own scroll offset when its history outgrows
	// the column it is drawn in.
	regionFocus bool
	execSel     int
	execOffset  int

	width  int
	height int

	blocks   []*block
	nextID   int
	activeID int
	notices  []string

	state    runState
	spinner  int
	started  time.Time
	running  bool
	quitting bool
	exitCode int

	// cancel stops the in-flight execution. It is non-nil only while one is
	// running, and is cleared when that execution reports back.
	cancel context.CancelFunc

	// caps is the tool's normalised capability registry, or nil when the tool
	// publishes none. It is read from the tool's own `capabilities -o json`
	// output rather than authored here, so it cannot drift from the tool.
	caps *Capabilities

	// activity is the live view of the current run. It is a consumer of
	// events: it counts and describes them and knows nothing about any
	// scanner.
	activity *Activity

	// layout is the resolved geometry for the current terminal size, recomputed
	// on every resize. Components read it rather than asking the terminal
	// themselves, so a region and the transcript cannot disagree about width.
	layout Layout

	// showCapabilities toggles the capability region.
	showCapabilities bool

	// form is the structured input for one capability, or nil when none is
	// open. A form only exists for a capability whose registry describes
	// parameters, so most sessions never have one.
	form *Form
}

// Run starts an interactive session and returns the process exit code.
//
// It returns an error satisfying IsNotInteractive when the streams cannot
// support a TUI, which is the caller's cue to use the one-shot CLI instead.
func Run(cfg Config) (int, error) {
	out := cfg.Out
	if out == nil {
		out = os.Stdout
	}
	if cfg.Runner == nil {
		return 1, fmt.Errorf("tui: Config.Runner is required")
	}
	in := cfg.In
	if in == nil {
		in = os.Stdin
	}

	// Refuse to draw into a pipe. This is the most important safety property
	// of the TUI: `tool scan target | tee log` must keep producing the same
	// output it always did, not a stream of escape codes.
	if f, ok := out.(*os.File); ok && !isTerminal(f) {
		return 1, &errNotInteractive{reason: "tui: output is not a terminal"}
	}

	// The palette is layered before styles are built: QYVORA_BASE, then the
	// tool's palette if it supplied one, then the styles. NO_COLOR is decided
	// last, so it still wins over a tool's branding.
	m := newModel(cfg, newTheme(!cfg.NoColor && colorEnabled(os.Stdout), cfg.Theme))

	// The renderer draws to a private duplicate of stdout. Command execution
	// redirects the process's real stdout into a capture pipe, and without this
	// the renderer would follow it there and overwrite the transcript with its
	// own frames.
	render, releaseRender, err := detachTerminal()
	if err != nil {
		return 1, fmt.Errorf("tui: cannot claim the terminal for rendering: %w", err)
	}
	defer releaseRender()

	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithContext(context.Background()),
		tea.WithOutput(render),
		tea.WithInput(in),
		// Cell motion reporting gives the wheel and click handling their
		// coordinates: a right-hand region has to know which column a click
		// landed in before it can decide whether the transcript or the
		// executions list was addressed.
		tea.WithMouseCellMotion(),
	)
	final, err := p.Run()
	if err != nil {
		return 1, fmt.Errorf("tui: %w", err)
	}
	fm, ok := final.(model)
	if !ok {
		return 0, nil
	}
	return fm.exitCode, nil
}

func newModel(cfg Config, theme Theme) model {
	ti := textinput.New()
	ti.Prompt = promptGlyph
	ti.Placeholder = "enter a command"
	ti.Focus()

	vp := viewport.New(80, 20)

	m := model{
		cfg:      cfg,
		runner:   cfg.Runner,
		theme:    theme,
		caps:     cfg.Capabilities,
		input:    ti,
		history:  NewHistory(),
		viewport: vp,
		// The session starts pinned to the newest output, so the first command
		// is visible without the user having to scroll.
		following: true,
		width:     80,
		height:    24,
		state:     stateReady,
	}
	m.input.Width = 60
	m.input.PlaceholderStyle = m.theme.Detail
	m.input.PromptStyle = m.theme.Prompt
	m.input.TextStyle = m.theme.Value
	if theme.Color {
		m.input.Cursor.Style = lipgloss.NewStyle().Foreground(paletteColor(theme.Palette.Accent))
	} else {
		m.input.Cursor.Style = lipgloss.NewStyle()
	}
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.tick(), textinput.Blink)
}

func (m model) tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Update handles input, execution events and repaint ticks.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.applyLayout(msg.Width, msg.Height)
		m.refreshViewport()
		return m, nil

	case tickMsg:
		var cmds []tea.Cmd
		cmds = append(cmds, m.tick())
		// Only repaint while work is in flight; an idle session that
		// redraws four times a second is needlessly noisy.
		if m.running {
			m.spinner = (m.spinner + 1) % len(spinnerFrames)
			// The spinner and elapsed clock live in the transcript, so the
			// running line has to be re-rendered for the animation to show.
			m.refreshViewport()
		}
		return m, tea.Batch(cmds...)

	case streamMsg:
		// A chunk of a live stream. The activity region and the running
		// block's output rows are fed from here, so a long scan reads as
		// activity rather than as a frozen screen that fills in at the end.
		// The chunk carries the command to read on (or nil, at the end of
		// the stream), which turns version of the old blocking reader into
		// a chain that stays one chunk ahead of a running tool.
		m.applyStream(msg.items)
		if len(msg.items) > 0 {
			m.refreshViewport()
		}
		return m, msg.next

	case batchMsg:
		m.applyStream(msg)
		// Re-render after the batch, not per event: a scan emits events far
		// faster than a person reads, and a redraw per line makes the
		// interface the bottleneck.
		m.refreshViewport()
		return m, nil

	case runDoneMsg:
		m.finish(msg)
		return m, nil

	case capabilitiesMsg:
		// Capabilities arriving after startup: the interface adopts them
		// without a restart.
		m.caps = msg.caps
		m.applyLayout(m.width, m.height)
		m.refreshViewport()
		return m, nil

	case noticeMsg:
		m.notices = append(m.notices, msg.text)
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// finish records an execution's outcome.
func (m *model) finish(msg runDoneMsg) {
	m.running = false
	m.cancel = nil

	if b := m.blockByID(m.activeID); b != nil {
		b.finished = time.Now()
		b.exitCode = msg.exitCode
		b.cancelled = msg.cancel
		switch {
		case msg.cancel:
			b.status = StatusCancelled
			m.state = stateCancelled
			m.notices = append(m.notices, "Cancelled -- partial results were kept")
		case msg.err != nil:
			b.status = StatusFailed
			b.err = msg.err.Error()
			m.state = stateFailed
			m.notices = append(m.notices, "Error: "+msg.err.Error())
		case msg.exitCode != 0:
			b.status = StatusFailed
			b.err = fmt.Sprintf("exit status %d", msg.exitCode)
			m.state = stateFailed
		default:
			b.status = StatusDone
			m.state = stateReady
		}
	}
	// With the run over the possibly-wider executions region takes over from
	// the narrower live view, and the transcript is rebuilt at its new width.
	m.applyLayout(m.width, m.height)
	m.refreshViewport()
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// An open form takes the keyboard. While it is up, every key belongs to it,
	// including the ones that would otherwise scroll the session: a form the
	// operator cannot type into because a key did something else is a form that
	// cannot be used.
	if m.form != nil {
		return m.handleFormKey(msg)
	}

	// The executions region takes the keyboard while it is focused. A few keys
	// leave it rather than being eaten: the session must stay stoppable and
	// quittable from anywhere, and Tab/Esc/F2 are the way back to the composer.
	// Every other key -- a letter, a scroll chord -- releases the region and
	// falls through to the composer, so typing while the region is focused
	// returns to the prompt and lands in it.
	if m.regionFocus {
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlD, tea.KeyEsc, tea.KeyTab, tea.KeyF2:
			m.regionFocus = false
			m.refreshViewport()
		case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown,
			tea.KeyHome, tea.KeyEnd, tea.KeyCtrlA, tea.KeyCtrlE,
			tea.KeyEnter, tea.KeySpace:
			return m.handleRegionKey(msg)
		default:
			m.regionFocus = false
			m.refreshViewport()
		}
	}

	switch msg.Type {

	case tea.KeyCtrlC:
		// The documented behaviour: Ctrl+C stops the work in flight and keeps
		// the session, rather than tearing the interface down. Tearing down on
		// the first press would discard the very partial results cancellation
		// preserves.
		switch {
		case m.running:
			m.cancelExecution()
		case m.input.Value() != "":
			m.input.SetValue("")
		default:
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case tea.KeyCtrlD:
		if m.input.Value() == "" {
			m.quitting = true
			return m, tea.Quit
		}

	case tea.KeyF1:
		// F1 reveals the capability registry. A dedicated key rather than a
		// built-in word, because the registry is a reference to consult, not a
		// command to run, and it must stay discoverable.
		m.showCapabilities = !m.showCapabilities
		m.applyLayout(m.width, m.height)
		m.refreshViewport()
		return m, nil

	case tea.KeyF2:
		// F2 moves focus between the composer and the executions region. The
		// region is the navigator for the session's history, which is what a
		// second focus target needs to have: it has rows worth selecting.
		if m.canFocusRegion() {
			m.regionFocus = !m.regionFocus
		}
		m.refreshViewport()
		return m, nil

	case tea.KeyCtrlO:
		// Reveal the raw event log. The structured stream is always retained;
		// this only decides whether it is on screen.
		m.showEvents = !m.showEvents
		m.refreshViewport()
		return m, nil

	case tea.KeyCtrlE:
		path, err := m.exportSession()
		if err != nil {
			m.notices = append(m.notices, "Export failed: "+err.Error())
		} else {
			m.notices = append(m.notices, "Exported raw session output to "+path)
		}
		m.refreshViewport()
		return m, nil

	case tea.KeyCtrlEnd, tea.KeyF15:
		// Return to the newest output, the way a chat log's "jump to latest"
		// does.
		m.following = true
		m.gotoBottom()
		return m, nil

	case tea.KeyPgUp:
		m.scrollUp(m.viewport.Height / 2)
		return m, nil

	case tea.KeyPgDown:
		m.scrollDown(m.viewport.Height / 2)
		return m, nil

	case tea.KeyHome:
		if m.input.Value() == "" {
			m.following = false
			m.viewport.GotoTop()
			return m, nil
		}

	case tea.KeyEnd:
		if m.input.Value() == "" {
			m.following = true
			m.gotoBottom()
			return m, nil
		}

	case tea.KeyShiftUp:
		m.scrollUp(1)
		return m, nil

	case tea.KeyShiftDown:
		m.scrollDown(1)
		return m, nil

	case tea.KeyUp:
		if v, ok := m.history.Prev(m.input.Value()); ok {
			m.input.SetValue(v)
			m.input.CursorEnd()
		}
		return m, nil

	case tea.KeyDown:
		if v, ok := m.history.Next(m.input.Value()); ok {
			m.input.SetValue(v)
			m.input.CursorEnd()
		}
		return m, nil

	case tea.KeyTab:
		return m, m.complete()

	case tea.KeyEnter:
		return m, m.submit()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// scrollUp moves the session viewport back, releasing follow mode so arriving
// output does not immediately pull the view forward again.
func (m *model) scrollUp(n int) {
	if m.viewport.YOffset <= 0 {
		return
	}
	m.viewport.LineUp(n)
	m.syncFollow()
}

// scrollDown moves the session viewport forward, re-pinning to the newest
// output once it reaches the end.
func (m *model) scrollDown(n int) {
	m.viewport.LineDown(n)
	m.syncFollow()
}

func (m *model) syncFollow() {
	atBottom := m.viewport.AtBottom()
	m.following = atBottom
	if atBottom {
		m.gotoBottom()
	}
}

func (m *model) gotoBottom() {
	m.viewport.GotoBottom()
	m.following = true
}

// refreshViewport re-renders the transcript into the viewport.
//
// It is called after any change to the session, so the scroll position and
// content never drift apart.
func (m *model) refreshViewport() {
	content := strings.Join(m.transcriptLines(), "\n")
	m.viewport.SetContent(content)
	// While the user is following the newest output, keep them there as it
	// grows. While they are reading history, leave them where they are.
	if m.following {
		m.viewport.GotoBottom()
	}
}

// submit runs the entered command line.
func (m *model) submit() tea.Cmd {
	line := strings.TrimSpace(m.input.Value())
	if line == "" {
		return nil
	}
	m.history.Add(line)
	m.input.SetValue("")

	// Handle help command specially to support "help <command>" syntax
	if strings.HasPrefix(strings.ToLower(line), "help") || line == "?" {
		query := ""
		parts := strings.Fields(line)
		if len(parts) > 1 {
			query = parts[1]
		}
		helpLines := GetHelpLines(query, m.runner.Commands(), m.cfg.Title, m.width-4, &m.theme)
		m.notices = append(m.notices, helpLines...)
		m.refreshViewport()
		return nil
	}

	switch strings.ToLower(line) {
	case "clear", "cls":
		m.blocks = nil
		m.notices = nil
		m.regionFocus = false
		m.execSel = 0
		m.execOffset = 0
		m.applyLayout(m.width, m.height)
		m.refreshViewport()
		return nil
	case "quit", "exit", ":q":
		m.quitting = true
		return tea.Quit
	}

	args, err := tokenize(line)
	if err != nil {
		m.notices = append(m.notices, "Error: "+err.Error())
		m.refreshViewport()
		return nil
	}
	if len(args) == 0 {
		return nil
	}
	if strings.EqualFold(args[0], "form") {
		// `form` with no name opens the only capability that has one, or says
		// which do. It is the discovery path: an operator who does not know
		// which capabilities take parameters should not have to go looking.
		if len(args) > 1 {
			m.openFormFor(args[1])
			return nil
		}
		m.openForm()
		return nil
	}
	return m.start(args)
}

// start launches an execution.
//
// Everything slow happens inside the returned command. Running the tool from
// Update itself would block the event loop, freezing the interface for the
// entire duration of the scan.
func (m *model) start(args []string) tea.Cmd {
	// One execution at a time. Two overlapping runs would interleave two
	// event streams into one transcript, and the in-process runner could not
	// do it safely at all, since both would write to the captured stdout.
	if m.running {
		m.notices = append(m.notices, "An execution is already running; Ctrl+C stops it")
		m.refreshViewport()
		return nil
	}

	m.nextID++
	// Notices belong to the command that raised them; carrying them into the
	// next command would make a resolved error look permanent.
	m.notices = nil
	b := newBlock(m.nextID, args)
	m.blocks = append(m.blocks, b)
	m.activeID = b.id
	m.running = true
	m.state = stateRunning
	m.started = time.Now()
	// Each run gets a fresh activity view. Carrying the previous run's counts
	// into a new one would report a scan as having inherited the last scan's
	// findings.
	m.activity = newActivity()
	// A run in flight changes what the layout can afford, so the viewport is
	// rebuilt at its new width now. Deferring this to the next resize would leave
	// the transcript clipped to the old width, with every line truncated and an
	// ellipsis where the region now is.
	//
	// The newest execution becomes the region's selected entry; running work is
	// what an operator focuses on, and the region leads with it.
	m.execSel = 0
	m.applyLayout(m.width, m.height)

	// A per-execution child context, never a shared or root one. Ctrl+C
	// cancels this execution and nothing else, and the next execution starts
	// from a clean context rather than inheriting a previous cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	// The new command is on screen before it starts, so the session reads in
	// the order it happened.
	m.refreshViewport()

	reader, writer := io.Pipe()
	runner := m.runner

	// Closes the write end when the run returns, which is what lets the
	// consumer goroutine see EOF and finish.
	runCmd := func() tea.Msg {
		code, err := runner.Run(ctx, args, writer)
		_ = writer.Close()
		return runDoneMsg{exitCode: code, err: err, cancel: ctx.Err() != nil}
	}

	return tea.Batch(runCmd, m.consume(reader))
}

// consume decodes an execution's event stream and forwards the envelopes.
//
// The pipe must be fully drained even after the run has finished, otherwise a
// writer blocked on a full pipe would never return and the execution would
// appear to hang.
func (m model) consume(r io.Reader) tea.Cmd {
	ch := make(chan tea.Msg, streamBuffer)
	go func() {
		stats := readEvents(r, func(ev Event) {
			ch <- evMsg(ev)
		}, func(line string) {
			ch <- outputMsg{line: line}
		})
		if stats.Malformed > 0 {
			ch <- noticeMsg{text: fmt.Sprintf("%d event line(s) could not be parsed and were skipped", stats.Malformed)}
		}
		close(ch)
	}()
	return m.drain(ch)
}

// streamBuffer is how many decoded messages the reader goroutine can hand
// over while the interface is busy rendering, before it has to wait.
const streamBuffer = 1024

// streamChunk is how many messages one delivery carries at most, and streamWait
// how long a quiet stream is left alone before the reader re-checks. Together
// they bound redraws: the elapsed clock and spinner already repaint while a
// command runs, so a busy stream is delivered often enough to feel live and a
// silent one never spins the interface flat.
const (
	streamChunk = 256
	streamWait  = 90 * time.Millisecond
)

// drain delivers a run's decoded stream in chunks.
//
// The returned command takes whatever has arrived since the last delivery --
// up to a chunk cap, or after a short quiet wait -- and hands it over as one
// message, re-arming itself when the stream is still open. The re-arm is what
// makes the transcript and activity region live: without it the reader would
// block until EOF and a long scan would paint exactly nothing until it had
// finished.
func (m model) drain(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		open := true
		var items []tea.Msg
		timer := time.NewTimer(streamWait)
		defer timer.Stop()
	drain:
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					open = false
					break drain
				}
				items = append(items, msg)
				if len(items) >= streamChunk {
					break drain
				}
			case <-timer.C:
				break drain
			}
		}
		var next tea.Cmd
		if open {
			next = m.drain(ch)
		}
		return streamMsg{items: items, next: next}
	}
}

// applyStream folds delivered stream messages into the running execution.
func (m *model) applyStream(items []tea.Msg) {
	for _, inner := range items {
		if ev, ok := inner.(evMsg); ok {
			if b := m.blockByID(m.activeID); b != nil {
				b.addEvent(Event(ev))
			}
			// The activity view counts every event, whatever its topic.
			// An event type the TUI has never seen is recorded like any
			// other: discarding it would throw away exactly the
			// information the tool took the trouble to emit.
			m.activity.record(Event(ev))
			continue
		}
		if out, ok := inner.(outputMsg); ok {
			if b := m.blockByID(m.activeID); b != nil {
				b.addOutput(out.line)
			}
			continue
		}
		if nt, ok := inner.(noticeMsg); ok {
			m.notices = append(m.notices, nt.text)
		}
	}
}

// cancelExecution interrupts the in-flight execution.
func (m *model) cancelExecution() {
	if m.cancel == nil {
		return
	}
	m.state = stateCancelled
	m.notices = append(m.notices, "Cancelling...")
	m.refreshViewport()
	// The context is cancelled, not the interface. The runner shuts the work
	// down, the tool persists what it already produced, and the transcript
	// records the outcome.
	m.cancel()
}

// complete offers completions for the word under the cursor.
func (m *model) complete() tea.Cmd {
	line := m.input.Value()
	word, start := wordAt(line, m.input.Position())

	// The second word of a command completes against that command's
	// subcommands rather than against the tool's top-level names. The two
	// cases are told apart by whether a command word precedes the cursor.
	cmd, _, hasCmd := commandWordBefore(line, start)
	if hasCmd {
		subs := m.matchingSubcommands(cmd, word)
		switch len(subs) {
		case 0:
			return nil
		case 1:
			m.input.SetValue(line[:start] + subs[0] + " ")
			m.input.CursorEnd()
			return nil
		default:
			m.notices = append(m.notices, strings.Join(subs, "   "))
			m.refreshViewport()
			return nil
		}
	}

	cands := m.candidates(word)
	switch len(cands) {
	case 0:
		return nil
	case 1:
		// An unambiguous prefix completes itself.
		completed := replaceWord(line, start, cands[0])
		if !strings.HasSuffix(completed, " ") {
			completed += " "
		}
		m.input.SetValue(completed)
		m.input.CursorEnd()
		return nil
	}

	// Ambiguous: list the options. Silently taking the first would run a
	// command the user did not mean to type.
	m.notices = append(m.notices, strings.Join(cands, "   "))
	m.refreshViewport()
	return nil
}

// matchingSubcommands returns the subcommands of a top-level command that
// begin with prefix, in declaration order.
func (m *model) matchingSubcommands(name, prefix string) []string {
	for _, c := range m.runner.Commands() {
		if c.Name != name {
			continue
		}
		seen := map[string]bool{}
		var out []string
		for _, s := range c.Subs {
			if s == "" || seen[s] || !strings.HasPrefix(s, prefix) {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
		return out
	}
	return nil
}

// commandWordBefore returns the command word preceding the cursor, when the
// cursor sits at the start of a new word.
func commandWordBefore(line string, pos int) (string, int, bool) {
	pos = clamp(pos, 0, len(line))
	// Walk back over the space that precedes the cursor, then over the word
	// before it.
	i := pos
	for i > 0 && isSpaceByte(line[i-1]) {
		i--
	}
	if i == 0 {
		return "", 0, false
	}
	end := i
	for i > 0 && !isSpaceByte(line[i-1]) {
		i--
	}
	if i == end {
		return "", 0, false
	}
	return line[i:end], i, true
}

// candidates returns completions for a partially typed word, drawn from the
// tool's own command registry so the list cannot drift from reality.
func (m *model) candidates(word string) []string {
	if word == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, c := range m.runner.Commands() {
		if strings.HasPrefix(c.Name, word) {
			add(c.Name)
		}
		for _, s := range c.Subs {
			if strings.HasPrefix(s, word) {
				add(c.Name + " " + s)
			}
		}
	}
	for _, c := range builtins {
		if strings.HasPrefix(c.Name, word) {
			add(c.Name)
		}
	}
	return out
}

func commandNames(cmds []Command) []string {
	out := make([]string, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, c.Name)
	}
	return out
}

func (m *model) blockByID(id int) *block {
	for _, b := range m.blocks {
		if b.id == id {
			return b
		}
	}
	return nil
}

// wordAt returns the word under the cursor and the offset where it starts.
func wordAt(line string, pos int) (string, int) {
	pos = clamp(pos, 0, len(line))
	start := pos
	for start > 0 && !isSpaceByte(line[start-1]) {
		start--
	}
	end := pos
	for end < len(line) && !isSpaceByte(line[end]) {
		end++
	}
	return line[start:end], start
}

// replaceWord substitutes a completed word back into the line.
func replaceWord(line string, start int, word string) string {
	start = clamp(start, 0, len(line))
	end := start
	for end < len(line) && !isSpaceByte(line[end]) {
		end++
	}
	return line[:start] + word + line[end:]
}

func isSpaceByte(b byte) bool { return b == ' ' || b == '\t' }

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// openFormFor opens a structured input for a named capability.
//
// The capability is resolved through the normalised registry, so the short form
// works: `form campaign.fuzz` finds `sekhmet.campaign.fuzz`. A capability whose
// registry describes no parameters yields no form, and the notice says so
// rather than opening an empty one.
func (m *model) openFormFor(id string) {
	// One execution at a time, as everywhere else: a form is a way of starting
	// a run, and opening one mid-run would leave the operator with an input they
	// cannot submit.
	if m.running {
		m.notices = append(m.notices, "An execution is already running; Ctrl+C stops it")
		m.refreshViewport()
		return
	}
	if m.caps == nil || len(m.caps.Items) == 0 {
		m.notices = append(m.notices, "No capability registry is available for "+m.cfg.Title)
		m.refreshViewport()
		return
	}
	c, ok := m.caps.Find(id)
	if !ok {
		m.notices = append(m.notices, "No capability named "+id)
		m.refreshViewport()
		return
	}
	f := OpenForm(c)
	if f == nil {
		m.notices = append(m.notices, c.Name+" takes no parameters; type the command instead")
		m.refreshViewport()
		return
	}
	m.form = f
	m.showCapabilities = true
	m.applyLayout(m.width, m.height)
	m.refreshViewport()
}

// openForm opens a form without being told which capability.
//
// The registry decides: the capabilities that describe parameters are the only
// ones with a form, and there is usually one. When there are several, they are
// named rather than guessed at, because picking the first would be a choice the
// tool never expressed.
func (m *model) openForm() {
	if m.running {
		m.notices = append(m.notices, "An execution is already running; Ctrl+C stops it")
		m.refreshViewport()
		return
	}
	if m.caps == nil {
		m.notices = append(m.notices, "No capability registry is available for "+m.cfg.Title)
		m.refreshViewport()
		return
	}
	formable := m.caps.Formable()
	switch len(formable) {
	case 0:
		m.notices = append(m.notices, m.caps.Tool+" publishes no parameters; type the command instead")
	case 1:
		m.form = OpenForm(formable[0])
		m.showCapabilities = true
		m.applyLayout(m.width, m.height)
	default:
		names := make([]string, 0, len(formable))
		for _, c := range formable {
			names = append(names, c.ID)
		}
		m.notices = append(m.notices,
			"Capabilities with parameters: "+strings.Join(names, ", ")+" -- form <name>")
	}
	m.refreshViewport()
}

// closeForm discards the open form and returns the session to the composer.
func (m *model) closeForm() {
	if m.form == nil {
		return
	}
	m.form = nil
	m.refreshViewport()
}

// submitForm runs the capability the open form belongs to.
//
// The capability's ID is a registry identifier, not a command word: mansa
// publishes "mansa.analyze" and its command tree has "analyze". Submitting the
// ID verbatim would ask the tool for a command that does not exist, so the words
// are resolved against the live tree first. When no command matches, the form
// stays open and says so rather than running something the operator did not ask
// for.
func (m *model) submitForm() tea.Cmd {
	f := m.form
	if f == nil {
		return nil
	}
	if err := f.Validate(); err != nil {
		m.refreshViewport()
		return nil
	}
	words := m.commandWordsFor(f.Capability)
	if len(words) == 0 {
		f.fail("No command runs " + f.Capability.ID + "; type the command instead")
		m.refreshViewport()
		return nil
	}
	m.form = nil
	return m.start(append(words, f.Args()...))
}

// commandWordsFor resolves a capability to the command words that run it.
//
// The search is over the tool's own command tree, longest match first, and the
// candidate words come from the capability's own ID. Nothing here knows any
// tool's vocabulary: a registry whose IDs are not derived from its commands
// simply yields no words, and the operator types the command by hand.
func (m *model) commandWordsFor(c Capability) []string {
	parts := strings.Split(c.ID, ".")
	// "mansa.analyze" is a two-word candidate; the leading tool name is not
	// something anyone types after the program name, so every suffix is tried
	// rather than only the full ID.
	var best []string
	for i := range parts {
		cand := parts[i:]
		if w := m.resolveCommandPath(cand); len(w) > 0 && len(w) > len(best) {
			best = w
		}
	}
	return best
}

// resolveCommandPath resolves words against the live command tree, following at
// most one level of subcommand, which is all the tree records.
func (m *model) resolveCommandPath(words []string) []string {
	if len(words) == 0 {
		return nil
	}
	for _, c := range m.runner.Commands() {
		if c.Name != words[0] {
			continue
		}
		if len(words) == 1 {
			return []string{c.Name}
		}
		for _, s := range c.Subs {
			if s == words[1] {
				return []string{c.Name, s}
			}
		}
		return nil
	}
	return nil
}

// handleFormKey routes a keystroke to the open form.
func (m model) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.closeForm()
		return m, nil
	case tea.KeyEnter:
		return m, m.submitForm()
	case tea.KeyCtrlC:
		// The same rule as everywhere else: the first press abandons the form
		// rather than the session.
		m.closeForm()
		return m, nil
	case tea.KeyUp, tea.KeyShiftTab:
		if !m.form.FocusPrev() {
			m.closeForm()
		}
		m.refreshViewport()
		return m, nil
	case tea.KeyDown, tea.KeyTab:
		if !m.form.FocusNext() {
			m.closeForm()
		}
		m.refreshViewport()
		return m, nil
	case tea.KeyLeft:
		m.form.MoveCursor(-1)
		m.refreshViewport()
		return m, nil
	case tea.KeyRight:
		m.form.MoveCursor(1)
		m.refreshViewport()
		return m, nil
	case tea.KeyHome, tea.KeyCtrlA:
		m.form.MoveToStart()
		m.refreshViewport()
		return m, nil
	case tea.KeyEnd, tea.KeyCtrlE:
		m.form.MoveToEnd()
		m.refreshViewport()
		return m, nil
	case tea.KeyCtrlU:
		// Clear the field the way a shell does, which is what an operator who
		// has mistyped a whole value reaches for.
		m.form.ClearField()
		m.refreshViewport()
		return m, nil
	case tea.KeyBackspace:
		m.form.Backspace()
		m.refreshViewport()
		return m, nil
	case tea.KeyDelete:
		m.form.ClearField()
		m.refreshViewport()
		return m, nil
	case tea.KeySpace:
		m.form.Insert(' ')
		m.refreshViewport()
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		for _, r := range msg.Runes {
			m.form.Insert(r)
		}
		m.refreshViewport()
	}
	return m, nil
}
