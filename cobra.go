package tui

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CobraAdapter wraps a cobra.Command to implement CommandNode with all optional
// interfaces, allowing CollectCommands to extract full metadata.
//
// Per PROMPT1.md §3: This adapter bridges Cobra's rich command metadata into
// the TUI's extended Command model, so tools using Cobra automatically get
// detailed help with flags, examples, usage strings, etc.
type CobraAdapter struct {
	cmd *cobra.Command
}

// NewCobraAdapter wraps a Cobra command tree for use with CollectCommands.
func NewCobraAdapter(cmd *cobra.Command) *CobraAdapter {
	return &CobraAdapter{cmd: cmd}
}

// CommandNode interface implementation

func (c *CobraAdapter) Name() string {
	return c.cmd.Name()
}

func (c *CobraAdapter) Short() string {
	return c.cmd.Short
}

func (c *CobraAdapter) Children() []CommandNode {
	if !c.cmd.HasSubCommands() {
		return nil
	}
	
	var children []CommandNode
	for _, sub := range c.cmd.Commands() {
		children = append(children, NewCobraAdapter(sub))
	}
	return children
}

func (c *CobraAdapter) Hidden() bool {
	return c.cmd.Hidden
}

// Optional interfaces for extended metadata

func (c *CobraAdapter) Long() string {
	return c.cmd.Long
}

func (c *CobraAdapter) Usage() string {
	return c.cmd.Use
}

func (c *CobraAdapter) Aliases() []string {
	return c.cmd.Aliases
}

func (c *CobraAdapter) Group() string {
	// Cobra uses GroupID (string) for grouping
	if c.cmd.GroupID != "" {
		return c.cmd.GroupID
	}
	
	// Fallback: try to infer from annotations
	if group, ok := c.cmd.Annotations["group"]; ok {
		return group
	}
	
	return ""
}

func (c *CobraAdapter) Flags() []FlagInfo {
	var flags []FlagInfo
	
	// Process all flags (local + inherited)
	c.cmd.Flags().VisitAll(func(f *pflag.Flag) {
		flags = append(flags, flagInfoFromPflag(f))
	})
	
	return flags
}

func (c *CobraAdapter) Examples() []ExampleInfo {
	if c.cmd.Example == "" {
		return nil
	}
	
	// Cobra's Example field is multi-line text with examples.
	// Parse it into structured ExampleInfo entries.
	return parseCobraExamples(c.cmd.Example)
}

func (c *CobraAdapter) Deprecated() string {
	return c.cmd.Deprecated
}

// Helper functions

// flagInfoFromPflag converts a pflag.Flag to FlagInfo.
func flagInfoFromPflag(f *pflag.Flag) FlagInfo {
	info := FlagInfo{
		Name:     "--" + f.Name,
		Desc:     f.Usage,
		Required: false, // pflag doesn't track required directly
	}
	
	// Add short flag if available
	if f.Shorthand != "" {
		info.Short = "-" + f.Shorthand
	}
	
	// Determine type from value
	info.Type = f.Value.Type()
	
	// Get default value
	if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "[]" {
		info.Default = f.DefValue
	}
	
	// Check if required via annotations (common pattern)
	if f.Annotations != nil {
		if _, ok := f.Annotations["required"]; ok {
			info.Required = true
		}
	}
	
	return info
}

// parseCobraExamples parses Cobra's multi-line Example field into structured entries.
// Cobra examples are typically formatted as:
//   command args
//   command args  # with optional comment
func parseCobraExamples(exampleText string) []ExampleInfo {
	if exampleText == "" {
		return nil
	}
	
	var examples []ExampleInfo
	lines := strings.Split(exampleText, "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Split on # for comments
		parts := strings.SplitN(line, "#", 2)
		cmd := strings.TrimSpace(parts[0])
		note := ""
		if len(parts) > 1 {
			note = strings.TrimSpace(parts[1])
		}
		
		if cmd != "" {
			examples = append(examples, ExampleInfo{
				Cmd:  cmd,
				Note: note,
			})
		}
	}
	
	return examples
}

// CobraCommands is a convenience function that wraps a Cobra root command
// and calls CollectCommands, returning the full Command metadata.
//
// This is the typical entry point for tools using Cobra.
func CobraCommands(root *cobra.Command) []Command {
	adapter := NewCobraAdapter(root)
	return CollectCommands(adapter)
}
