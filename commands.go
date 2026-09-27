package tui

// CommandNode is the minimal shape of a command tree that the TUI needs in
// order to offer completion.
//
// It exists so completion can be derived from whatever registry a tool already
// uses, without this module taking a dependency on that tool's command
// framework. Coupling the shared TUI to one particular CLI library would force
// it on every tool and make the two evolve together for no benefit.
//
// A tool adapts its own registry to this interface in a handful of lines, and
// completion is then generated from the commands that actually exist rather
// than from a second, hand-maintained list that can drift.
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

// CollectCommands walks a command tree into the metadata the TUI completes
// against.
//
// The root itself is not included: it is the tool's name, not something a user
// types after it. Hidden commands and the shell-completion generator are
// excluded, since neither is something to complete by hand.
func CollectCommands(root CommandNode) []Command {
	if root == nil {
		return nil
	}
	var out []Command
	for _, child := range root.Children() {
		if child == nil || child.Hidden() {
			continue
		}
		name := child.Name()
		if name == "" || name == "help" || name == "completion" {
			continue
		}
		c := Command{Name: name, Short: child.Short()}
		for _, sub := range child.Children() {
			if sub == nil || sub.Hidden() {
				continue
			}
			if n := sub.Name(); n != "" && n != "help" && n != "completion" {
				c.Subs = append(c.Subs, n)
			}
		}
		out = append(out, c)
	}
	return out
}

// StaticCommands adapts a fixed list into the metadata type, for tools whose
// registry is not a tree.
func StaticCommands(cmds []Command) []Command { return cmds }
