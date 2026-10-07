package tui

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCobraAdapterBasicFields(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "scan <target>",
		Short: "Scan a target",
		Long:  "Performs a comprehensive scan of the specified target.",
	}
	
	adapter := NewCobraAdapter(cmd)
	
	if adapter.Name() != "scan" {
		t.Errorf("Name() = %q, want 'scan'", adapter.Name())
	}
	if adapter.Short() != "Scan a target" {
		t.Errorf("Short() = %q, want 'Scan a target'", adapter.Short())
	}
	if adapter.Long() != "Performs a comprehensive scan of the specified target." {
		t.Errorf("Long() = %q, want long description", adapter.Long())
	}
	if adapter.Usage() != "scan <target>" {
		t.Errorf("Usage() = %q, want 'scan <target>'", adapter.Usage())
	}
}

func TestCobraAdapterAliases(t *testing.T) {
	cmd := &cobra.Command{
		Use:     "scan",
		Aliases: []string{"s", "probe"},
	}
	
	adapter := NewCobraAdapter(cmd)
	aliases := adapter.Aliases()
	
	if len(aliases) != 2 {
		t.Fatalf("Aliases() count = %d, want 2", len(aliases))
	}
	if aliases[0] != "s" || aliases[1] != "probe" {
		t.Errorf("Aliases() = %v, want [s probe]", aliases)
	}
}

func TestCobraAdapterHidden(t *testing.T) {
	visible := &cobra.Command{Use: "visible"}
	hidden := &cobra.Command{Use: "hidden", Hidden: true}
	
	if NewCobraAdapter(visible).Hidden() {
		t.Error("visible command reported as hidden")
	}
	if !NewCobraAdapter(hidden).Hidden() {
		t.Error("hidden command reported as visible")
	}
}

func TestCobraAdapterChildren(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	sub1 := &cobra.Command{Use: "sub1"}
	sub2 := &cobra.Command{Use: "sub2"}
	root.AddCommand(sub1, sub2)
	
	adapter := NewCobraAdapter(root)
	children := adapter.Children()
	
	if len(children) != 2 {
		t.Fatalf("Children() count = %d, want 2", len(children))
	}
	if children[0].Name() != "sub1" {
		t.Errorf("First child name = %q, want 'sub1'", children[0].Name())
	}
	if children[1].Name() != "sub2" {
		t.Errorf("Second child name = %q, want 'sub2'", children[1].Name())
	}
}

func TestCobraAdapterFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "scan"}
	cmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
	cmd.Flags().StringP("output", "o", "json", "Output format")
	cmd.Flags().Int("depth", 3, "Scan depth")
	
	adapter := NewCobraAdapter(cmd)
	flags := adapter.Flags()
	
	if len(flags) < 3 {
		t.Fatalf("Flags() count = %d, want at least 3", len(flags))
	}
	
	// Find the verbose flag
	var verboseFlag *FlagInfo
	for i := range flags {
		if flags[i].Name == "--verbose" {
			verboseFlag = &flags[i]
			break
		}
	}
	
	if verboseFlag == nil {
		t.Fatal("verbose flag not found")
	}
	if verboseFlag.Short != "-v" {
		t.Errorf("verbose short = %q, want '-v'", verboseFlag.Short)
	}
	if verboseFlag.Type != "bool" {
		t.Errorf("verbose type = %q, want 'bool'", verboseFlag.Type)
	}
	if !strings.Contains(verboseFlag.Desc, "verbose") {
		t.Errorf("verbose desc = %q, should contain 'verbose'", verboseFlag.Desc)
	}
	
	// Find the output flag
	var outputFlag *FlagInfo
	for i := range flags {
		if flags[i].Name == "--output" {
			outputFlag = &flags[i]
			break
		}
	}
	
	if outputFlag == nil {
		t.Fatal("output flag not found")
	}
	if outputFlag.Default != "json" {
		t.Errorf("output default = %q, want 'json'", outputFlag.Default)
	}
	if outputFlag.Type != "string" {
		t.Errorf("output type = %q, want 'string'", outputFlag.Type)
	}
}

func TestCobraAdapterExamples(t *testing.T) {
	cmd := &cobra.Command{
		Use: "scan",
		Example: `  scan example.com
  scan example.com --deep  # Comprehensive scan
  scan example.com -o json`,
	}
	
	adapter := NewCobraAdapter(cmd)
	examples := adapter.Examples()
	
	if len(examples) < 2 {
		t.Fatalf("Examples() count = %d, want at least 2", len(examples))
	}
	
	// Check first example
	if !strings.Contains(examples[0].Cmd, "example.com") {
		t.Errorf("First example cmd = %q, should contain 'example.com'", examples[0].Cmd)
	}
	
	// Check example with comment
	found := false
	for _, ex := range examples {
		if strings.Contains(ex.Note, "Comprehensive") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Example with 'Comprehensive' note not found")
	}
}

func TestCobraAdapterDeprecated(t *testing.T) {
	deprecated := &cobra.Command{
		Use:        "oldscan",
		Deprecated: "use 'scan' instead",
	}
	
	adapter := NewCobraAdapter(deprecated)
	if adapter.Deprecated() == "" {
		t.Error("Deprecated() should return deprecation message")
	}
	if !strings.Contains(adapter.Deprecated(), "scan") {
		t.Errorf("Deprecated() = %q, should mention 'scan'", adapter.Deprecated())
	}
}

func TestCobraAdapterGroup(t *testing.T) {
	cmd := &cobra.Command{
		Use:     "scan",
		GroupID: "Recon",
	}
	
	adapter := NewCobraAdapter(cmd)
	if adapter.Group() != "Recon" {
		t.Errorf("Group() = %q, want 'Recon'", adapter.Group())
	}
}

func TestCobraAdapterGroupFromAnnotations(t *testing.T) {
	cmd := &cobra.Command{
		Use: "scan",
		Annotations: map[string]string{
			"group": "Analysis",
		},
	}
	
	adapter := NewCobraAdapter(cmd)
	if adapter.Group() != "Analysis" {
		t.Errorf("Group() = %q, want 'Analysis'", adapter.Group())
	}
}

func TestCobraCommands(t *testing.T) {
	root := &cobra.Command{Use: "testtool"}
	scan := &cobra.Command{
		Use:   "scan",
		Short: "Scan target",
		Long:  "Scan target in depth",
	}
	scan.Flags().Bool("deep", false, "Deep scan")
	
	analyze := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze results",
	}
	
	root.AddCommand(scan, analyze)
	
	commands := CobraCommands(root)
	
	if len(commands) < 2 {
		t.Fatalf("CobraCommands() count = %d, want at least 2", len(commands))
	}
	
	// Find scan command
	var scanCmd *Command
	for i := range commands {
		if commands[i].Name == "scan" {
			scanCmd = &commands[i]
			break
		}
	}
	
	if scanCmd == nil {
		t.Fatal("scan command not found in collected commands")
	}
	
	if scanCmd.Short != "Scan target" {
		t.Errorf("scan.Short = %q, want 'Scan target'", scanCmd.Short)
	}
	if scanCmd.Long != "Scan target in depth" {
		t.Errorf("scan.Long = %q, want 'Scan target in depth'", scanCmd.Long)
	}
	if len(scanCmd.Flags) == 0 {
		t.Error("scan command should have flags")
	}
}

func TestCobraAdapterWithRealCobraTree(t *testing.T) {
	// Create a realistic Cobra command tree
	root := &cobra.Command{Use: "security-tool"}
	
	recon := &cobra.Command{
		Use:     "scan <target>",
		Short:   "Scan a target",
		Long:    "Performs comprehensive scanning of the target.",
		GroupID: "Recon",
		Example: `  scan example.com
  scan example.com --deep`,
		Aliases: []string{"s"},
	}
	recon.Flags().BoolP("deep", "d", false, "Deep scan")
	recon.Flags().StringP("output", "o", "json", "Output format")
	
	analyze := &cobra.Command{
		Use:     "analyze",
		Short:   "Analyze results",
		GroupID: "Analysis",
	}
	
	report := &cobra.Command{
		Use:        "oldreport",
		Short:      "Generate report",
		Deprecated: "use 'report-new' instead",
	}
	
	hidden := &cobra.Command{
		Use:    "secret",
		Short:  "Secret command",
		Hidden: true,
	}
	
	root.AddCommand(recon, analyze, report, hidden)
	
	commands := CobraCommands(root)
	
	// Should have 3 visible commands (hidden excluded)
	if len(commands) != 3 {
		t.Errorf("CobraCommands() count = %d, want 3 (excluding hidden)", len(commands))
	}
	
	// Verify scan command has all metadata
	var scanCmd *Command
	for i := range commands {
		if commands[i].Name == "scan" {
			scanCmd = &commands[i]
			break
		}
	}
	
	if scanCmd == nil {
		t.Fatal("scan command not found")
	}
	
	if scanCmd.Group != "Recon" {
		t.Errorf("scan.Group = %q, want 'Recon'", scanCmd.Group)
	}
	if len(scanCmd.Aliases) == 0 {
		t.Error("scan should have aliases")
	}
	if len(scanCmd.Flags) < 2 {
		t.Errorf("scan.Flags count = %d, want at least 2", len(scanCmd.Flags))
	}
	if len(scanCmd.Examples) < 1 {
		t.Error("scan should have examples")
	}
	
	// Verify deprecated command
	var deprecatedCmd *Command
	for i := range commands {
		if commands[i].Name == "oldreport" {
			deprecatedCmd = &commands[i]
			break
		}
	}
	
	if deprecatedCmd == nil {
		t.Fatal("oldreport command not found")
	}
	if deprecatedCmd.Deprecated == "" {
		t.Error("oldreport should be marked as deprecated")
	}
}

func TestParseCobraExamples(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "empty",
			input:    "",
			expected: 0,
		},
		{
			name:     "single line",
			input:    "scan example.com",
			expected: 1,
		},
		{
			name: "multiple lines",
			input: `scan example.com
scan example.com --deep`,
			expected: 2,
		},
		{
			name: "with comments",
			input: `scan example.com  # Simple scan
scan example.com --deep  # Comprehensive scan`,
			expected: 2,
		},
		{
			name: "with blank lines",
			input: `scan example.com

scan example.com --deep`,
			expected: 2,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			examples := parseCobraExamples(tt.input)
			if len(examples) != tt.expected {
				t.Errorf("parseCobraExamples() count = %d, want %d", len(examples), tt.expected)
			}
		})
	}
}

func TestParseCobraExamplesWithNotes(t *testing.T) {
	input := "scan example.com --deep  # Comprehensive scan"
	examples := parseCobraExamples(input)
	
	if len(examples) != 1 {
		t.Fatalf("expected 1 example, got %d", len(examples))
	}
	
	if !strings.Contains(examples[0].Cmd, "scan") {
		t.Errorf("Cmd = %q, should contain 'scan'", examples[0].Cmd)
	}
	if !strings.Contains(examples[0].Note, "Comprehensive") {
		t.Errorf("Note = %q, should contain 'Comprehensive'", examples[0].Note)
	}
}
