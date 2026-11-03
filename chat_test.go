package tui

import (
	"context"
	"os"
	"strings"
	"testing"
)

type fakeChatRunner struct {
	name string
	got  []string
}

func (f *fakeChatRunner) Name() string { return f.name }

func (f *fakeChatRunner) Send(_ context.Context, input string, emit func(ChatEvent) error) error {
	f.got = append(f.got, input)
	return emit(ChatEvent{Type: ChatEventMessage, Role: ChatRoleAssistant, Text: "answer"})
}

func TestRunChatRejectsNonInteractiveOutput(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	_, err = RunChat(ChatConfig{Runner: &fakeChatRunner{name: "test"}, Out: w})
	if !IsNotInteractive(err) {
		t.Fatalf("RunChat error = %v, want IsNotInteractive", err)
	}
}

func TestChatStreamingDeltaAccumulates(t *testing.T) {
	m := newChatModel(ChatConfig{Runner: &fakeChatRunner{name: "test"}}, newThemeAt(false, nil, DepthNone))
	m.applyChatEvent(ChatEvent{Type: ChatEventMessage, Role: ChatRoleUser, Text: "hi"})
	m.applyChatEvent(ChatEvent{Type: ChatEventDelta, Text: "Hello"})
	m.applyChatEvent(ChatEvent{Type: ChatEventDelta, Text: " world"})

	if len(m.messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(m.messages))
	}
	if got := m.messages[1].Text; got != "Hello world" {
		t.Fatalf("streamed text = %q, want %q", got, "Hello world")
	}
	if !m.stream {
		t.Fatal("stream flag should be set after a delta")
	}

	m.applyChatEvent(ChatEvent{Type: ChatEventMessage, Role: ChatRoleAssistant, Text: "done"})
	if m.stream {
		t.Fatal("stream flag should clear after a complete message")
	}
}

func TestChatViewRendersTranscriptAndComposer(t *testing.T) {
	m := newChatModel(ChatConfig{
		Runner:    &fakeChatRunner{name: "test"},
		Title:     "QYVORA / AFRICA AI",
		Version:   "test",
		SessionID: "demo",
	}, newThemeAt(false, nil, DepthNone))
	m.applyChatEvent(ChatEvent{Type: ChatEventMessage, Role: ChatRoleUser, Text: "hello"})
	m.applyChatEvent(ChatEvent{Type: ChatEventMessage, Role: ChatRoleAssistant, Text: "hello operator"})
	m.applyChatEvent(ChatEvent{Type: ChatEventTool, Tool: "assess", Text: "step recon: success"})
	m.applyChatLayout()
	m.refreshChatViewport()

	view := stripANSI(m.View())
	for _, want := range []string{"QYVORA / AFRICA AI", "YOU", "QYVORA", "TOOL ASSESS", "hello operator", "AI ASSISTANT", "enter send"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestChatViewFitsTerminalHeight(t *testing.T) {
	m := newChatModel(ChatConfig{Runner: &fakeChatRunner{name: "test"}}, newThemeAt(false, nil, DepthNone))
	m.width, m.height = 80, 24
	m.applyChatLayout()
	m.refreshChatViewport()
	lines := strings.Split(stripANSI(m.View()), "\n")
	if len(lines) > 24 {
		t.Fatalf("view has %d lines, want <= 24:\n%s", len(lines), strings.Join(lines, "\n"))
	}
}

func TestChatLocalHelpAndClear(t *testing.T) {
	m := newChatModel(ChatConfig{Runner: &fakeChatRunner{name: "test"}}, newThemeAt(false, nil, DepthNone))
	m.input.SetValue("/help")
	m, _ = m.submitChat()
	if len(m.messages) != 1 || !strings.Contains(m.messages[0].Text, "/assess") {
		t.Fatalf("help messages = %#v", m.messages)
	}

	m.input.SetValue("/clear")
	m, _ = m.submitChat()
	if len(m.messages) != 0 {
		t.Fatalf("clear left %d messages", len(m.messages))
	}
}

func TestChatSubmitStartsRunner(t *testing.T) {
	runner := &fakeChatRunner{name: "test"}
	m := newChatModel(ChatConfig{Runner: runner}, newThemeAt(false, nil, DepthNone))
	m.input.SetValue("what is next?")
	m, cmd := m.submitChat()
	if cmd == nil {
		t.Fatal("submit did not start a turn")
	}
	if len(m.messages) == 0 || m.messages[0].Role != ChatRoleUser || m.messages[0].Text != "what is next?" {
		t.Fatalf("messages = %#v", m.messages)
	}
	if !m.running || m.state != stateRunning {
		t.Fatalf("running=%v state=%v", m.running, m.state)
	}
}
