package domain

// Command describes a single command for completion and help.
// Per PROMPT1.md §3: Extended model with full metadata while keeping
// backwards compatibility with existing fields.
type Command struct {
	Name  string // command word as typed
	Short string // one line, <= 72 chars, no trailing period
	Long  string // optional paragraph(s) with detailed description
	Usage string // e.g. "scan <target> [flags]"

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
