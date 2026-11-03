package tui

import (
	"context"
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

// ChatRole identifies the speaker of a chat message.
type ChatRole string

const (
	// ChatRoleUser is the operator.
	ChatRoleUser ChatRole = "user"
	// ChatRoleAssistant is the model.
	ChatRoleAssistant ChatRole = "assistant"
	// ChatRoleTool is a tool or workflow update.
	ChatRoleTool ChatRole = "tool"
	// ChatRoleSystem is local terminal feedback.
	ChatRoleSystem ChatRole = "system"
)

// ChatEventType is one kind of update a ChatRunner can emit.
type ChatEventType string

const (
	// ChatEventMessage is a complete message.
	ChatEventMessage ChatEventType = "message"
	// ChatEventDelta appends text to the current assistant message.
	ChatEventDelta ChatEventType = "delta"
	// ChatEventStatus updates the transient status line.
	ChatEventStatus ChatEventType = "status"
	// ChatEventTool adds a tool/workflow transcript block.
	ChatEventTool ChatEventType = "tool"
	// ChatEventError reports a failed turn.
	ChatEventError ChatEventType = "error"
)

// ChatEvent is one update emitted by a ChatRunner while handling a turn.
type ChatEvent struct {
	Type   ChatEventType
	Role   ChatRole
	Text   string
	Tool   string
	Status string
	Err    error
}

// ChatMessage is one message retained by the terminal transcript.
type ChatMessage struct {
	Role ChatRole
	Text string
	Tool string
	At   time.Time
}

// ChatRunner handles one operator turn. It may emit any number of events and
// must return when ctx is cancelled.
type ChatRunner interface {
	Name() string
	Send(ctx context.Context, input string, emit func(ChatEvent) error) error
}

// ChatConfig configures an interactive AI terminal session.
type ChatConfig struct {
	Runner    ChatRunner
	Title     string
	Version   string
	In        io.Reader
	Out       io.Writer
	NoColor   bool
	Theme     *Palette
	SessionID string
}

// RunChat starts an interactive AI terminal and returns the process exit code.
//
// It returns an error satisfying IsNotInteractive when the output stream cannot
// support a full-screen TUI.
func RunChat(cfg ChatConfig) (int, error) {
	out := cfg.Out
	if out == nil {
		out = os.Stdout
	}
	if cfg.Runner == nil {
		return 1, fmt.Errorf("tui: ChatConfig.Runner is required")
	}
	in := cfg.In
	if in == nil {
		in = os.Stdin
	}
	f, ok := out.(*os.File)
	if !ok || !isTerminal(f) {
		return 1, &errNotInteractive{reason: "tui: output is not a terminal"}
	}

	m := newChatModel(cfg, newTheme(!cfg.NoColor && colorEnabled(os.Stdout), cfg.Theme))
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
	fm, ok := final.(chatModel)
	if !ok {
		return 0, nil
	}
	return fm.exitCode, nil
}

type chatModel struct {
	cfg      ChatConfig
	runner   ChatRunner
	theme    Theme
	input    textinput.Model
	history  *History
	viewport viewport.Model

	messages []ChatMessage
	running  bool
	stream   bool
	status   string
	err      string
	state    runState
	spinner  int

	width     int
	height    int
	following bool
	cancel    context.CancelFunc
	quitting  bool
	exitCode  int
}

type chatStreamMsg struct{ ch <-chan ChatEvent }
type chatEventMsg struct {
	ev ChatEvent
	ch <-chan ChatEvent
}
type chatDoneMsg struct{}

func newChatModel(cfg ChatConfig, theme Theme) chatModel {
	ti := textinput.New()
	ti.Prompt = promptGlyph
	ti.Placeholder = "ask qyvora or type /assess <target>"
	ti.Focus()

	vp := viewport.New(80, 20)
	m := chatModel{
		cfg:       cfg,
		runner:    cfg.Runner,
		theme:     theme,
		input:     ti,
		history:   NewHistory(),
		viewport:  vp,
		state:     stateReady,
		width:     80,
		height:    24,
		following: true,
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
	m.applyChatLayout()
	m.refreshChatViewport()
	return m
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(m.tick(), textinput.Blink)
}

func (m chatModel) tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.applyChatLayout()
		m.refreshChatViewport()
		return m, nil

	case tickMsg:
		if m.running {
			m.spinner = (m.spinner + 1) % len(spinnerFrames)
			m.refreshChatViewport()
		}
		return m, m.tick()

	case chatStreamMsg:
		return m, waitChat(msg.ch)

	case chatEventMsg:
		m.applyChatEvent(msg.ev)
		m.refreshChatViewport()
		return m, waitChat(msg.ch)

	case chatDoneMsg:
		m.running = false
		m.stream = false
		m.cancel = nil
		m.status = ""
		if m.state == stateRunning {
			m.state = stateReady
		}
		m.refreshChatViewport()
		return m, nil

	case tea.KeyMsg:
		return m.handleChatKey(msg)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m chatModel) handleChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.running {
			if m.cancel != nil {
				m.cancel()
			}
			m.state = stateCancelled
			m.status = "cancelling"
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	case "ctrl+d":
		if strings.TrimSpace(m.input.Value()) == "" {
			m.quitting = true
			return m, tea.Quit
		}
	case "ctrl+l":
		m.messages = nil
		m.state = stateReady
		m.err = ""
		m.following = true
		m.refreshChatViewport()
		return m, nil
	case "enter":
		if m.running {
			m.status = "wait for the current response"
			return m, nil
		}
		return m.submitChat()
	case "up":
		if line, ok := m.history.Prev(m.input.Value()); ok {
			m.input.SetValue(line)
			m.input.CursorEnd()
		}
		return m, nil
	case "down":
		if line, ok := m.history.Next(m.input.Value()); ok {
			m.input.SetValue(line)
			m.input.CursorEnd()
		}
		return m, nil
	case "pgup":
		m.following = false
		m.viewport.PageUp()
		return m, nil
	case "pgdown":
		m.viewport.PageDown()
		if m.viewport.AtBottom() {
			m.following = true
		}
		return m, nil
	case "ctrl+end":
		m.following = true
		m.viewport.GotoBottom()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m chatModel) submitChat() (chatModel, tea.Cmd) {
	line := strings.TrimSpace(m.input.Value())
	if line == "" {
		return m, nil
	}
	m.input.Reset()
	m.history.Add(line)

	switch strings.ToLower(line) {
	case "quit", "exit", "/quit", "/exit":
		m.quitting = true
		return m, tea.Quit
	case "clear", "/clear":
		m.messages = nil
		m.state = stateReady
		m.err = ""
		m.following = true
		m.refreshChatViewport()
		return m, nil
	case "help", "/help":
		m.messages = append(m.messages, ChatMessage{
			Role: ChatRoleSystem,
			Text: "Commands:\n" +
				"  /assess <target>  prepare an assessment plan\n" +
				"  /clear            clear the transcript\n" +
				"  /help             show this help\n" +
				"  /quit             leave the terminal\n\n" +
				"Press enter to send. Use pgup/pgdown to scroll, ctrl+c to stop a turn, ctrl+d to quit.",
			At: time.Now(),
		})
		m.following = true
		m.refreshChatViewport()
		return m, nil
	}

	m.messages = append(m.messages, ChatMessage{Role: ChatRoleUser, Text: line, At: time.Now()})
	m.running = true
	m.stream = false
	m.state = stateRunning
	m.status = "thinking"
	m.err = ""
	m.following = true

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.refreshChatViewport()
	return m, startChat(ctx, m.runner, line)
}

func startChat(ctx context.Context, runner ChatRunner, input string) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan ChatEvent, 32)
		go func() {
			defer close(ch)
			err := runner.Send(ctx, input, func(ev ChatEvent) error {
				select {
				case ch <- ev:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
			if err != nil && ctx.Err() == nil {
				select {
				case ch <- ChatEvent{Type: ChatEventError, Err: err}:
				case <-ctx.Done():
				}
			}
		}()
		return chatStreamMsg{ch: ch}
	}
}

func waitChat(ch <-chan ChatEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return chatDoneMsg{}
		}
		return chatEventMsg{ev: ev, ch: ch}
	}
}

func (m *chatModel) applyChatEvent(ev ChatEvent) {
	switch ev.Type {
	case ChatEventMessage:
		text := strings.TrimRight(ev.Text, "\n")
		if strings.TrimSpace(text) == "" {
			return
		}
		role := ev.Role
		if role == "" {
			role = ChatRoleAssistant
		}
		m.messages = append(m.messages, ChatMessage{Role: role, Text: text, Tool: ev.Tool, At: time.Now()})
		m.stream = false
	case ChatEventDelta:
		if ev.Text == "" {
			return
		}
		if !m.stream || len(m.messages) == 0 || m.messages[len(m.messages)-1].Role != ChatRoleAssistant {
			m.messages = append(m.messages, ChatMessage{Role: ChatRoleAssistant, At: time.Now()})
			m.stream = true
		}
		m.messages[len(m.messages)-1].Text += ev.Text
	case ChatEventStatus:
		m.status = strings.TrimSpace(ev.Status)
	case ChatEventTool:
		text := strings.TrimRight(ev.Text, "\n")
		if strings.TrimSpace(text) == "" && strings.TrimSpace(ev.Status) != "" {
			text = ev.Status
		}
		if strings.TrimSpace(text) == "" {
			return
		}
		m.messages = append(m.messages, ChatMessage{Role: ChatRoleTool, Text: text, Tool: ev.Tool, At: time.Now()})
		m.stream = false
	case ChatEventError:
		text := strings.TrimSpace(ev.Text)
		if ev.Err != nil {
			if text != "" {
				text += ": "
			}
			text += ev.Err.Error()
		}
		if text == "" {
			text = "the turn failed"
		}
		m.err = text
		m.state = stateFailed
		m.messages = append(m.messages, ChatMessage{Role: ChatRoleSystem, Text: "Error: " + text, At: time.Now()})
		m.stream = false
	}
}

func (m *chatModel) applyChatLayout() {
	width := max(20, m.width)
	height := max(8, m.height)
	m.viewport.Width = width
	m.viewport.Height = max(3, height-7)
	m.input.Width = max(20, width-2)
}

func (m *chatModel) refreshChatViewport() {
	m.viewport.SetContent(strings.Join(m.chatTranscriptLines(), "\n"))
	if m.following {
		m.viewport.GotoBottom()
	}
}

func (m chatModel) chatTranscriptLines() []string {
	width := max(20, m.width-2)
	if len(m.messages) == 0 {
		return []string{
			m.theme.Title.Render("QYVORA Africa AI"),
			m.theme.Detail.Render("Authorized security assessment terminal"),
			"",
			m.theme.Detail.Render("Ask a question, or describe a target to assess."),
			m.theme.Detail.Render("Type /assess <target> to prepare a plan, /help for commands."),
		}
	}

	var out []string
	for i, msg := range m.messages {
		label, style := m.chatLabel(msg)
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, style.Render("▌ "+label))
		text := strings.TrimRight(msg.Text, "\n")
		if text == "" {
			text = " "
		}
		for _, line := range wrapText(text, max(8, width-4)) {
			out = append(out, "  "+line)
		}
		if m.stream && i == len(m.messages)-1 && msg.Role == ChatRoleAssistant {
			out = append(out, "  "+m.theme.Running.Render("▍"))
		}
	}
	return out
}

func (m chatModel) chatLabel(msg ChatMessage) (string, lipglossStyle) {
	switch msg.Role {
	case ChatRoleUser:
		return "YOU", m.theme.Prompt
	case ChatRoleTool:
		label := "TOOL"
		if strings.TrimSpace(msg.Tool) != "" {
			label += " " + strings.ToUpper(strings.TrimSpace(msg.Tool))
		}
		return label, m.theme.Cyan
	case ChatRoleSystem:
		return "SYSTEM", m.theme.Detail
	default:
		return "QYVORA", m.theme.Success
	}
}

func (m chatModel) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.chatHeader())
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")
	b.WriteString(m.chatComposer())
	return b.String()
}

func (m chatModel) chatHeader() string {
	title := strings.TrimSpace(m.cfg.Title)
	if title == "" {
		title = "QYVORA / AFRICA AI"
	}
	left := m.theme.Title.Render(title)
	if m.cfg.Version != "" {
		left += " " + m.theme.Version.Render(m.cfg.Version)
	}
	right := m.theme.Pill.Render(m.chatStateLabel())
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	return clampLine(left+strings.Repeat(" ", gap)+right, m.width)
}

func (m chatModel) chatStateLabel() string {
	switch m.state {
	case stateRunning:
		return spinnerFrames[m.spinner] + " THINKING"
	case stateFailed:
		return "! FAILED"
	case stateCancelled:
		return "■ CANCELLED"
	default:
		return "● READY"
	}
}

func (m chatModel) chatComposer() string {
	mode := "AI ASSISTANT"
	if m.running && m.status != "" {
		mode += " · " + strings.ToUpper(m.status)
	} else if m.err != "" {
		mode += " · LAST TURN FAILED"
	}
	if m.cfg.SessionID != "" {
		mode += " · " + m.cfg.SessionID
	}
	bar := m.theme.ModeBar.Width(max(1, m.width)).Render(" " + m.theme.ModeFg.Render(clampLine(mode, max(1, m.width-2))) + " ")
	hint := m.theme.Hint.Render("enter send · pgup/pgdown scroll · ctrl+c stop · ctrl+d quit")
	return strings.Join([]string{
		m.theme.Rule.Render(strings.Repeat("─", max(1, m.width))),
		clampLine(bar, m.width),
		clampLine(m.input.View(), m.width),
		clampLine(hint, m.width),
	}, "\n")
}
