package tui

import (
	"testing"
)

// TestCommandModelBackwardsCompat tests that the extended Command model
// is backwards compatible with existing code.
func TestCommandModelBackwardsCompat(t *testing.T) {
	// Old-style command (only Name and Short)
	cmd := Command{
		Name:  "scan",
		Short: "Scan a target",
		Subs:  []string{"deep", "quick"},
	}
	
	if cmd.Name != "scan" {
		t.Errorf("Name = %q, want scan", cmd.Name)
	}
	if cmd.Short != "Scan a target" {
		t.Errorf("Short = %q, want 'Scan a target'", cmd.Short)
	}
	if len(cmd.Subs) != 2 {
		t.Errorf("Subs count = %d, want 2", len(cmd.Subs))
	}
}

// TestCommandModelExtended tests the new fields.
func TestCommandModelExtended(t *testing.T) {
	cmd := Command{
		Name:  "scan",
		Short: "Scan a target",
		Long:  "Performs a comprehensive scan of the specified target.",
		Usage: "scan <target> [flags]",
		Aliases: []string{"s", "probe"},
		Group: "Recon",
		Flags: []Flag{
			{
				Name:    "--depth",
				Short:   "-d",
				Type:    "int",
				Default: "3",
				Desc:    "Maximum scan depth",
			},
		},
		Examples: []Example{
			{
				Cmd:  "scan example.com --deep",
				Note: "Deep scan of example.com",
			},
		},
		SeeAlso: []string{"analyze", "report"},
	}
	
	if cmd.Long == "" {
		t.Error("Long description should be set")
	}
	if cmd.Usage == "" {
		t.Error("Usage should be set")
	}
	if len(cmd.Aliases) != 2 {
		t.Errorf("Aliases count = %d, want 2", len(cmd.Aliases))
	}
	if cmd.Group != "Recon" {
		t.Errorf("Group = %q, want Recon", cmd.Group)
	}
	if len(cmd.Flags) != 1 {
		t.Errorf("Flags count = %d, want 1", len(cmd.Flags))
	}
	if len(cmd.Examples) != 1 {
		t.Errorf("Examples count = %d, want 1", len(cmd.Examples))
	}
	if len(cmd.SeeAlso) != 2 {
		t.Errorf("SeeAlso count = %d, want 2", len(cmd.SeeAlso))
	}
}

// mockCommandNode implements CommandNode with all optional interfaces.
type mockCommandNode struct {
	name       string
	short      string
	long       string
	usage      string
	aliases    []string
	group      string
	flags      []FlagInfo
	examples   []ExampleInfo
	deprecated string
	children   []CommandNode
	hidden     bool
}

func (m *mockCommandNode) Name() string              { return m.name }
func (m *mockCommandNode) Short() string             { return m.short }
func (m *mockCommandNode) Long() string              { return m.long }
func (m *mockCommandNode) Usage() string             { return m.usage }
func (m *mockCommandNode) Aliases() []string         { return m.aliases }
func (m *mockCommandNode) Group() string             { return m.group }
func (m *mockCommandNode) Flags() []FlagInfo         { return m.flags }
func (m *mockCommandNode) Examples() []ExampleInfo   { return m.examples }
func (m *mockCommandNode) Deprecated() string        { return m.deprecated }
func (m *mockCommandNode) Children() []CommandNode   { return m.children }
func (m *mockCommandNode) Hidden() bool              { return m.hidden }

// TestCollectCommandsWithExtendedMetadata tests that CollectCommands
// extracts all available metadata via optional interfaces.
func TestCollectCommandsWithExtendedMetadata(t *testing.T) {
	root := &mockCommandNode{
		name:  "root",
		short: "Root command",
		children: []CommandNode{
			&mockCommandNode{
				name:    "scan",
				short:   "Scan targets",
				long:    "Performs comprehensive scanning",
				usage:   "scan <target> [flags]",
				aliases: []string{"s"},
				group:   "Recon",
				flags: []FlagInfo{
					{Name: "--deep", Short: "-d", Type: "bool", Desc: "Deep scan"},
				},
				examples: []ExampleInfo{
					{Cmd: "scan example.com", Note: "Simple scan"},
				},
			},
			&mockCommandNode{
				name:   "help",
				short:  "Help command",
				hidden: false, // Should be filtered out by name
			},
		},
	}
	
	commands := CollectCommands(root)
	
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	
	scan := commands[0]
	if scan.Name != "scan" {
		t.Errorf("Name = %q, want scan", scan.Name)
	}
	if scan.Long == "" {
		t.Error("Long should be populated")
	}
	if scan.Usage == "" {
		t.Error("Usage should be populated")
	}
	if len(scan.Aliases) != 1 {
		t.Errorf("Aliases count = %d, want 1", len(scan.Aliases))
	}
	if scan.Group != "Recon" {
		t.Errorf("Group = %q, want Recon", scan.Group)
	}
	if len(scan.Flags) != 1 {
		t.Errorf("Flags count = %d, want 1", len(scan.Flags))
	}
	if len(scan.Examples) != 1 {
		t.Errorf("Examples count = %d, want 1", len(scan.Examples))
	}
}

// TestCollectCommandsFiltersHidden tests that hidden commands are excluded.
func TestCollectCommandsFiltersHidden(t *testing.T) {
	root := &mockCommandNode{
		name:  "root",
		short: "Root",
		children: []CommandNode{
			&mockCommandNode{name: "visible", short: "Visible command"},
			&mockCommandNode{name: "hidden", short: "Hidden command", hidden: true},
		},
	}
	
	commands := CollectCommands(root)
	
	if len(commands) != 1 {
		t.Fatalf("expected 1 command (hidden filtered), got %d", len(commands))
	}
	if commands[0].Name != "visible" {
		t.Errorf("expected visible command, got %q", commands[0].Name)
	}
}

// minimalNode for testing backwards compatibility
type minimalNode struct {
	name     string
	short    string
	children []CommandNode
}

func (m *minimalNode) Name() string            { return m.name }
func (m *minimalNode) Short() string           { return m.short }
func (m *minimalNode) Children() []CommandNode { return m.children }
func (m *minimalNode) Hidden() bool            { return false }

// TestCollectCommandsHandlesMinimalNode tests backwards compatibility
// with nodes that don't implement optional interfaces.
func TestCollectCommandsHandlesMinimalNode(t *testing.T) {
	root := &minimalNode{
		name:  "root",
		short: "Root",
		children: []CommandNode{
			&minimalNode{name: "basic", short: "Basic command"},
		},
	}
	
	commands := CollectCommands(root)
	
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	
	cmd := commands[0]
	if cmd.Name != "basic" {
		t.Errorf("Name = %q, want basic", cmd.Name)
	}
	if cmd.Short != "Basic command" {
		t.Errorf("Short = %q, want 'Basic command'", cmd.Short)
	}
	// Extended fields should be zero values
	if cmd.Long != "" {
		t.Error("Long should be empty for minimal node")
	}
	if cmd.Usage != "" {
		t.Error("Usage should be empty for minimal node")
	}
}
