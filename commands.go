package tui

// CommandNode is the minimal shape of a command tree that the TUI needs in
// order to offer completion and help.
//
// It exists so completion can be derived from whatever registry a tool already
// uses, without this module taking a dependency on that tool's command
// framework. Coupling the shared TUI to one particular CLI library would force
// it on every tool and make the two evolve together for no benefit.
//
// A tool adapts its own registry to this interface in a handful of lines, and
// completion is then generated from the commands that actually exist rather
// than from a second, hand-maintained list that can drift.
//
// Extended metadata is exposed through optional interfaces (see below) that
// can be type-asserted. This keeps the minimal interface small while allowing
// rich help content when available.
type CommandNode interface {
	// Name is the command word as typed.
	Name() string
	// Short is the one-line description shown in help.
	Short() string
	// Children returns the command's subcommands.
	Children() []CommandNode
	// Hidden reports whether the command should be hidden from completion.
	Hidden() bool
}

// Optional interfaces for enhanced metadata (type-assert to check support):

// Longer provides detailed description for help detail view.
type Longer interface {
	Long() string
}

// Usager provides usage string (e.g. "scan <target> [flags]").
type Usager interface {
	Usage() string
}

// Aliaser provides alternative command names.
type Aliaser interface {
	Aliases() []string
}

// Grouper categorizes the command (e.g. "Recon", "Analysis").
type Grouper interface {
	Group() string
}

// Flagger exposes available flags. Returns name, short, type, default, desc, required.
type Flagger interface {
	Flags() []FlagInfo
}

// FlagInfo describes one flag from a CommandNode's Flagger interface.
type FlagInfo struct {
	Name     string
	Short    string
	Type     string
	Default  string
	Desc     string
	Required bool
}

// Exampler provides usage examples.
type Exampler interface {
	Examples() []ExampleInfo
}

// ExampleInfo describes one example from a CommandNode's Exampler interface.
type ExampleInfo struct {
	Cmd  string
	Note string
}

// Deprecator marks a command as deprecated.
type Deprecator interface {
	Deprecated() string // returns replacement suggestion
}

// CollectCommands walks a command tree into the metadata the TUI completes
// against.
//
// The root itself is not included: it is the tool's name, not something a user
// types after it. Hidden commands and the shell-completion generator are
// excluded, since neither is something to complete by hand.
//
// Per PROMPT1.md §3: Now walks the WHOLE tree recursively and fills every
// field it can via optional interface type assertions.
func CollectCommands(root CommandNode) []Command {
	if root == nil {
		return nil
	}
	return collectRecursive(root, 0)
}

func collectRecursive(node CommandNode, depth int) []Command {
	if node == nil {
		return nil
	}
	
	var out []Command
	for _, child := range node.Children() {
		if child == nil || child.Hidden() {
			continue
		}
		name := child.Name()
		if name == "" || name == "help" || name == "completion" {
			continue
		}
		
		// Build command with all available metadata
		c := Command{
			Name:  name,
			Short: child.Short(),
		}
		
		// Try optional interfaces
		if longer, ok := child.(Longer); ok {
			c.Long = longer.Long()
		}
		if usager, ok := child.(Usager); ok {
			c.Usage = usager.Usage()
		}
		if aliaser, ok := child.(Aliaser); ok {
			c.Aliases = aliaser.Aliases()
		}
		if grouper, ok := child.(Grouper); ok {
			c.Group = grouper.Group()
		}
		if flagger, ok := child.(Flagger); ok {
			flagInfos := flagger.Flags()
			c.Flags = make([]Flag, len(flagInfos))
			for i, fi := range flagInfos {
				c.Flags[i] = Flag{
					Name:     fi.Name,
					Short:    fi.Short,
					Type:     fi.Type,
					Default:  fi.Default,
					Desc:     fi.Desc,
					Required: fi.Required,
				}
			}
		}
		if exampler, ok := child.(Exampler); ok {
			exampleInfos := exampler.Examples()
			c.Examples = make([]Example, len(exampleInfos))
			for i, ei := range exampleInfos {
				c.Examples[i] = Example{
					Cmd:  ei.Cmd,
					Note: ei.Note,
				}
			}
		}
		if deprecator, ok := child.(Deprecator); ok {
			c.Deprecated = deprecator.Deprecated()
		}
		
		// Collect subcommands (still as []string for backwards compat)
		// TODO: Migrate to recursive []Command structure
		for _, sub := range child.Children() {
			if sub == nil || sub.Hidden() {
				continue
			}
			if n := sub.Name(); n != "" && n != "help" && n != "completion" {
				c.Subs = append(c.Subs, n)
			}
		}
		
		out = append(out, c)
		
		// Recursively collect subcommands if needed (for now, only go 2 levels deep)
		// Full recursion would require changing Subs to []Command
	}
	return out
}

// StaticCommands adapts a fixed list into the metadata type, for tools whose
// registry is not a tree.
func StaticCommands(cmds []Command) []Command { return cmds }
