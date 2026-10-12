package tui

import "github.com/QYVORA/qyvora-tui/internal/commands"

// Command completion and help metadata live in internal/commands. These
// aliases and forwards keep the root package's API unchanged.
type (
	// CommandNode is the minimal shape of a command tree the TUI completes against.
	CommandNode = commands.CommandNode
	// FlagInfo describes one flag from a CommandNode's Flagger interface.
	FlagInfo = commands.FlagInfo
	// ExampleInfo describes one example from a CommandNode's Exampler interface.
	ExampleInfo = commands.ExampleInfo
	// CobraAdapter adapts a cobra command tree to CommandNode.
	CobraAdapter = commands.CobraAdapter
	// History is the input history ring.
	History = commands.History

	// Longer provides a detailed description for the help detail view.
	Longer = commands.Longer
	// Usager provides a usage string.
	Usager = commands.Usager
	// Aliaser provides alternative command names.
	Aliaser = commands.Aliaser
	// Grouper categorizes a command.
	Grouper = commands.Grouper
	// Flagger exposes available flags.
	Flagger = commands.Flagger
	// Exampler provides usage examples.
	Exampler = commands.Exampler
	// Deprecator marks a command as deprecated.
	Deprecator = commands.Deprecator
)

// CollectCommands walks a command tree into the metadata the TUI completes
// against.
var CollectCommands = commands.CollectCommands

// StaticCommands adapts a fixed list into the metadata type.
var StaticCommands = commands.StaticCommands

// NewHistory starts an empty input history.
var NewHistory = commands.NewHistory

// NewCobraAdapter adapts a cobra command tree.
var NewCobraAdapter = commands.NewCobraAdapter

// CobraCommands collects completion metadata from a cobra command tree.
var CobraCommands = commands.CobraCommands

// parseCobraExamples is retained for the package's own tests.
var parseCobraExamples = commands.ParseCobraExamples
