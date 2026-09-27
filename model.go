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
	// In, Out and Err are the streams the TUI owns. Out defaults to stdout.
	In  io.Reader
	Out io.Writer
	// Err receives the tool's human-readable output during execution. Nil
	// discards it: the transcript renders events, and the tool's prose
	// interleaved with redrawn frames is unreadable.
	Err io.Writer
	// NoColor forces plain output regardless of terminal detection.
	NoColor bool
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
	// human reads them, so the consumer batches rather than queuing a
	// redraw per line.
	batchMsg []tea.Msg
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

	m := newModel(cfg, newTheme(!cfg.NoColor && colorEnabled(os.Stdout)))

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
	ti.Prompt = "> "
	ti.Placeholder = "command"
	ti.Focus()

	vp := viewport.New(80, 20)

	m := model{
		cfg:      cfg,
		runner:   cfg.Runner,
		theme:    theme,
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
		m.input.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(accentHex))
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
		m.input.Width = max(8, msg.Width-lipgloss.Width(m.input.Prompt)-2)
		// The viewport owns everything between the header rule and the
		// composer: header, rule, viewport, composer.
		vpHeight := msg.Height - 4
		if vpHeight < 1 {
			vpHeight = 1
		}
		m.viewport = viewport.New(max(1, msg.Width), vpHeight)
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

	case batchMsg:
		for _, inner := range msg {
			if ev, ok := inner.(evMsg); ok {
				if b := m.blockByID(m.activeID); b != nil {
					b.addEvent(Event(ev))
				}
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
		// Re-render after the batch, not per event: a scan emits events far
		// faster than a person reads, and a redraw per line makes the
		// interface the bottleneck.
		m.refreshViewport()
		return m, nil

	case runDoneMsg:
		m.finish(msg)
		return m, nil

	case noticeMsg:
		m.notices = append(m.notices, msg.text)
		return m, nil

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
	m.refreshViewport()
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

	case tea.KeyCtrlO:
		// Reveal the raw event log. The structured stream is always retained;
		// this only decides whether it is on screen.
		m.showEvents = !m.showEvents
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

	switch strings.ToLower(line) {
	case "help", "?":
		m.notices = append(m.notices, m.helpLines()...)
		m.refreshViewport()
		return nil
	case "clear", "cls":
		m.blocks = nil
		m.notices = nil
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
	return func() tea.Msg {
		var msgs []tea.Msg
		stats := readEvents(r, func(ev Event) {
			msgs = append(msgs, evMsg(ev))
		}, func(line string) {
			msgs = append(msgs, outputMsg{line})
		})
		if len(msgs) == 0 {
			if stats.Malformed > 0 {
				return noticeMsg{text: fmt.Sprintf("%d event line(s) could not be parsed and were skipped", stats.Malformed)}
			}
			return nil
		}
		if stats.Malformed > 0 {
			msgs = append(msgs, noticeMsg{text: fmt.Sprintf("%d event line(s) could not be parsed and were skipped", stats.Malformed)})
		}
		return batchMsg(msgs)
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

func (m *model) helpLines() []string {
	return []string{
		m.theme.Group.Render("Keys"),
		"  " + m.theme.Detail.Render("ctrl+c") + "   stop the running command · " + m.theme.Detail.Render("ctrl+d") + "   leave",
		"  " + m.theme.Detail.Render("ctrl+o") + "   show or hide the raw event log",
		"  " + m.theme.Detail.Render("pgup/pgdn") + "   scroll the session · " + m.theme.Detail.Render("ctrl+end") + "   jump to newest",
		"  " + m.theme.Detail.Render("tab") + "         complete · " + m.theme.Detail.Render("↑/↓") + "         history",
		"",
		m.theme.Group.Render("Built-ins") + "  " + m.theme.Detail.Render("help · clear · quit"),
		m.theme.Group.Render("Commands ") + strings.Join(commandNames(m.runner.Commands()), " · "),
	}
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
