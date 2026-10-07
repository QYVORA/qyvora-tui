package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// mockTheme for testing
var testTheme = &Theme{
	Group:  lipgloss.NewStyle(),
	Detail: lipgloss.NewStyle(),
}

func TestRenderHelpIndex(t *testing.T) {
	cmds := []Command{
		{Name: "scan", Short: "Scan a target", Group: "Recon"},
		{Name: "analyze", Short: "Analyze results", Group: "Analysis"},
		{Name: "report", Short: "Generate report", Group: "Reporting"},
		{Name: "info", Short: "Show info"},
	}
	
	lines := RenderHelpIndex(cmds, "testtool", 80, testTheme)
	
	// Should have COMMANDS header
	if len(lines) == 0 || !strings.Contains(lines[0], "COMMANDS") {
		t.Error("Expected COMMANDS header")
	}
	
	// Should have groups
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "Recon") {
		t.Error("Expected Recon group")
	}
	if !strings.Contains(text, "Analysis") {
		t.Error("Expected Analysis group")
	}
	if !strings.Contains(text, "Built-ins") {
		t.Error("Expected Built-ins group")
	}
	
	// Should have built-in commands
	if !strings.Contains(text, "help") {
		t.Error("Expected help command")
	}
	if !strings.Contains(text, "clear") {
		t.Error("Expected clear command")
	}
	if !strings.Contains(text, "quit") {
		t.Error("Expected quit command")
	}
}

func TestGroupCommands(t *testing.T) {
	cmds := []Command{
		{Name: "scan", Group: "Recon"},
		{Name: "probe", Group: "Recon"},
		{Name: "analyze", Group: "Analysis"},
		{Name: "info"}, // No group
	}
	
	grouped := groupCommands(cmds)
	
	if len(grouped["Recon"]) != 2 {
		t.Errorf("Recon group: expected 2 commands, got %d", len(grouped["Recon"]))
	}
	if len(grouped["Analysis"]) != 1 {
		t.Errorf("Analysis group: expected 1 command, got %d", len(grouped["Analysis"]))
	}
	if len(grouped["Commands"]) != 1 {
		t.Errorf("Commands group: expected 1 command, got %d", len(grouped["Commands"]))
	}
}

func TestRenderHelpDetail(t *testing.T) {
	cmd := Command{
		Name:  "scan",
		Short: "Scan a target",
		Long:  "Performs a comprehensive scan of the specified target with various options.",
		Usage: "scan <target> [flags]",
		Flags: []Flag{
			{Name: "--deep", Short: "-d", Type: "bool", Desc: "Perform deep scan"},
		},
		Examples: []Example{
			{Cmd: "scan example.com", Note: "Simple scan"},
		},
		SeeAlso: []string{"analyze", "report"},
	}
	
	lines := RenderHelpDetail(cmd, "testtool", 80, testTheme)
	text := strings.Join(lines, "\n")
	
	// Should have all sections
	if !strings.Contains(text, "NAME") {
		t.Error("Expected NAME section")
	}
	if !strings.Contains(text, "USAGE") {
		t.Error("Expected USAGE section")
	}
	if !strings.Contains(text, "DESCRIPTION") {
		t.Error("Expected DESCRIPTION section")
	}
	if !strings.Contains(text, "FLAGS") {
		t.Error("Expected FLAGS section")
	}
	if !strings.Contains(text, "EXAMPLES") {
		t.Error("Expected EXAMPLES section")
	}
	if !strings.Contains(text, "SEE ALSO") {
		t.Error("Expected SEE ALSO section")
	}
	
	// Should have command details
	if !strings.Contains(text, "scan") {
		t.Error("Expected command name")
	}
	if !strings.Contains(text, "Scan a target") {
		t.Error("Expected short description")
	}
	if !strings.Contains(text, "comprehensive scan") {
		t.Error("Expected long description")
	}
	if !strings.Contains(text, "--deep") {
		t.Error("Expected --deep flag")
	}
	if !strings.Contains(text, "example.com") {
		t.Error("Expected example command")
	}
}

func TestRenderHelpDetailMinimal(t *testing.T) {
	// Command with only required fields
	cmd := Command{
		Name:  "basic",
		Short: "Basic command",
	}
	
	lines := RenderHelpDetail(cmd, "testtool", 80, testTheme)
	text := strings.Join(lines, "\n")
	
	// Should have NAME section
	if !strings.Contains(text, "NAME") {
		t.Error("Expected NAME section")
	}
	if !strings.Contains(text, "basic") {
		t.Error("Expected command name")
	}
	
	// Should NOT have optional sections
	if strings.Contains(text, "USAGE") {
		t.Error("Should not have USAGE section for minimal command")
	}
	if strings.Contains(text, "DESCRIPTION") {
		t.Error("Should not have DESCRIPTION section for minimal command")
	}
	if strings.Contains(text, "FLAGS") {
		t.Error("Should not have FLAGS section for minimal command")
	}
}

func TestRenderHelpNotFound(t *testing.T) {
	cmds := []Command{
		{Name: "scan", Short: "Scan a target"},
		{Name: "analyze", Short: "Analyze results"},
	}
	
	lines := RenderHelpNotFound("scam", cmds, testTheme)
	text := strings.Join(lines, "\n")
	
	// Should have error message
	if !strings.Contains(text, "Unknown command") {
		t.Error("Expected 'Unknown command' message")
	}
	if !strings.Contains(text, "scam") {
		t.Error("Expected query in error message")
	}
	
	// Should have suggestions
	if !strings.Contains(text, "Did you mean") {
		t.Error("Expected suggestions")
	}
	if !strings.Contains(text, "scan") {
		t.Error("Expected 'scan' as suggestion")
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"a", "a", 0},
		{"scan", "scam", 1},
		{"scan", "can", 1},
		{"analyze", "analyse", 1},
		{"help", "halp", 1},
		{"quit", "quick", 2},
	}
	
	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestFindSuggestions(t *testing.T) {
	cmds := []Command{
		{Name: "scan", Short: "Scan"},
		{Name: "analyze", Short: "Analyze"},
		{Name: "report", Short: "Report"},
	}
	
	// Test close match
	suggestions := findSuggestions("scam", cmds, 3)
	if len(suggestions) == 0 {
		t.Fatal("Expected suggestions for 'scam'")
	}
	if suggestions[0].Name != "scan" {
		t.Errorf("Expected 'scan' as first suggestion, got %q", suggestions[0].Name)
	}
	
	// Test no close matches
	suggestions = findSuggestions("xyzabc", cmds, 3)
	if len(suggestions) != 0 {
		t.Errorf("Expected no suggestions for 'xyzabc', got %d", len(suggestions))
	}
	
	// Test max suggestions limit
	manyCommands := []Command{
		{Name: "a1"},
		{Name: "a2"},
		{Name: "a3"},
		{Name: "a4"},
		{Name: "a5"},
	}
	suggestions = findSuggestions("a", manyCommands, 3)
	if len(suggestions) > 3 {
		t.Errorf("Expected at most 3 suggestions, got %d", len(suggestions))
	}
}

func TestGetHelpLinesIndex(t *testing.T) {
	cmds := []Command{
		{Name: "scan", Short: "Scan"},
	}
	
	lines := GetHelpLines("", cmds, "testtool", 80, testTheme)
	text := strings.Join(lines, "\n")
	
	// Empty query should return index
	if !strings.Contains(text, "COMMANDS") {
		t.Error("Expected index view for empty query")
	}
	if !strings.Contains(text, "Built-ins") {
		t.Error("Expected built-ins in index")
	}
}

func TestGetHelpLinesDetail(t *testing.T) {
	cmds := []Command{
		{Name: "scan", Short: "Scan", Long: "Detailed scan description"},
	}
	
	lines := GetHelpLines("scan", cmds, "testtool", 80, testTheme)
	text := strings.Join(lines, "\n")
	
	// Should return detail view
	if !strings.Contains(text, "NAME") {
		t.Error("Expected detail view for command query")
	}
	if !strings.Contains(text, "Detailed scan description") {
		t.Error("Expected long description in detail view")
	}
}

func TestGetHelpLinesBuiltin(t *testing.T) {
	cmds := []Command{}
	
	lines := GetHelpLines("help", cmds, "testtool", 80, testTheme)
	text := strings.Join(lines, "\n")
	
	// Should return help for built-in command
	if !strings.Contains(text, "NAME") {
		t.Error("Expected detail view for built-in help command")
	}
	if !strings.Contains(text, "help") {
		t.Error("Expected 'help' in detail view")
	}
}

func TestGetHelpLinesAlias(t *testing.T) {
	cmds := []Command{
		{Name: "scan", Short: "Scan", Aliases: []string{"s", "probe"}},
	}
	
	lines := GetHelpLines("s", cmds, "testtool", 80, testTheme)
	text := strings.Join(lines, "\n")
	
	// Should find command by alias
	if !strings.Contains(text, "NAME") {
		t.Error("Expected detail view when querying by alias")
	}
	if !strings.Contains(text, "scan") {
		t.Error("Expected actual command name in detail view")
	}
}

func TestGetHelpLinesNotFound(t *testing.T) {
	cmds := []Command{
		{Name: "scan", Short: "Scan"},
	}
	
	lines := GetHelpLines("unknown", cmds, "testtool", 80, testTheme)
	text := strings.Join(lines, "\n")
	
	// Should return not found view
	if !strings.Contains(text, "Unknown command") {
		t.Error("Expected not found view for unknown command")
	}
}

func TestRenderCommandTable(t *testing.T) {
	cmds := []Command{
		{Name: "scan", Short: "Scan a target"},
		{Name: "analyze", Short: "Analyze scan results in depth"},
	}
	
	lines := renderCommandTable(cmds, 60, testTheme)
	
	if len(lines) < 2 {
		t.Fatal("Expected at least 2 lines from command table")
	}
	
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "scan") {
		t.Error("Expected 'scan' in command table")
	}
	if !strings.Contains(text, "analyze") {
		t.Error("Expected 'analyze' in command table")
	}
}

func TestRenderFlags(t *testing.T) {
	flags := []Flag{
		{
			Name:     "--verbose",
			Short:    "-v",
			Type:     "bool",
			Desc:     "Enable verbose output",
			Required: false,
		},
		{
			Name:     "--config",
			Short:    "-c",
			Type:     "string",
			Default:  "config.yaml",
			Desc:     "Configuration file path",
			Required: true,
		},
	}
	
	lines := renderFlags(flags, 80, testTheme)
	text := strings.Join(lines, "\n")
	
	// Should have flag names
	if !strings.Contains(text, "--verbose") {
		t.Error("Expected --verbose flag")
	}
	if !strings.Contains(text, "--config") {
		t.Error("Expected --config flag")
	}
	
	// Should have short names
	if !strings.Contains(text, "-v") {
		t.Error("Expected -v short flag")
	}
	
	// Should have type and default
	if !strings.Contains(text, "string") {
		t.Error("Expected type in flag")
	}
	if !strings.Contains(text, "config.yaml") {
		t.Error("Expected default value")
	}
	
	// Should indicate required
	if !strings.Contains(text, "required") {
		t.Error("Expected required indicator")
	}
}
