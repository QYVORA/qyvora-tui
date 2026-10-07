package tui

import (
	"fmt"
	"sort"
	"strings"
)

// RenderHelpIndex produces the help index view per PROMPT1.md §4.
// Groups commands by Group field, presents as two-column table with hanging indent.
func RenderHelpIndex(cmds []Command, toolName string, width int, theme *Theme) []string {
	if width < 40 {
		width = 40
	}
	
	var lines []string
	
	// Header
	lines = append(lines, theme.Group.Render("COMMANDS"))
	lines = append(lines, "")
	
	// Group commands
	grouped := groupCommands(cmds)
	groups := sortedGroupNames(grouped)
	
	// Add built-ins as their own group
	builtins := []Command{
		{Name: "help", Short: "List commands, or describe one"},
		{Name: "clear", Short: "Clear the session transcript"},
		{Name: "quit", Short: "Leave the TUI (Ctrl+D also works)"},
	}
	
	// Render each group
	for i, groupName := range groups {
		if i > 0 {
			lines = append(lines, "")
		}
		
		groupCmds := grouped[groupName]
		lines = append(lines, theme.Group.Render(groupName))
		
		// Two-column table with hanging indent
		tableLines := renderCommandTable(groupCmds, width-2, theme)
		for _, line := range tableLines {
			lines = append(lines, "  "+line)
		}
	}
	
	// Built-ins at the end
	if len(groups) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, theme.Group.Render("Built-ins"))
	tableLines := renderCommandTable(builtins, width-2, theme)
	for _, line := range tableLines {
		lines = append(lines, "  "+line)
	}
	
	lines = append(lines, "")
	lines = append(lines, theme.Detail.Render("Type 'help <command>' for details."))
	
	return lines
}

// groupCommands organizes commands by their Group field.
// Commands without a group go into "Commands".
func groupCommands(cmds []Command) map[string][]Command {
	grouped := make(map[string][]Command)
	for _, cmd := range cmds {
		group := cmd.Group
		if group == "" {
			group = "Commands"
		}
		grouped[group] = append(grouped[group], cmd)
	}
	
	// Sort within each group
	for _, cmds := range grouped {
		sort.Slice(cmds, func(i, j int) bool {
			return cmds[i].Name < cmds[j].Name
		})
	}
	
	return grouped
}

// sortedGroupNames returns group names in a sensible order:
// "Recon", "Analysis", "Reporting", "Session", then alphabetical.
func sortedGroupNames(grouped map[string][]Command) []string {
	priority := []string{"Recon", "Analysis", "Reporting", "Session"}
	var names []string
	
	// Add priority groups if they exist
	for _, p := range priority {
		if _, ok := grouped[p]; ok {
			names = append(names, p)
		}
	}
	
	// Add remaining groups alphabetically
	var others []string
	for name := range grouped {
		isPriority := false
		for _, p := range priority {
			if name == p {
				isPriority = true
				break
			}
		}
		if !isPriority {
			others = append(others, name)
		}
	}
	sort.Strings(others)
	names = append(names, others...)
	
	return names
}

// renderCommandTable produces a two-column table: name on left, description on right.
// Description wraps with hanging indent if needed.
func renderCommandTable(cmds []Command, width int, theme *Theme) []string {
	if len(cmds) == 0 {
		return nil
	}
	
	// Find longest name for column sizing
	maxNameLen := 0
	for _, cmd := range cmds {
		if len(cmd.Name) > maxNameLen {
			maxNameLen = len(cmd.Name)
		}
	}
	
	// Column 1 width: name + padding
	col1Width := maxNameLen + 2
	if col1Width < 12 {
		col1Width = 12
	}
	if col1Width > 20 {
		col1Width = 20
	}
	
	// Column 2 width: remaining space
	col2Width := width - col1Width
	if col2Width < 30 {
		col2Width = 30
	}
	
	var lines []string
	for _, cmd := range cmds {
		name := theme.Detail.Render(cmd.Name)
		desc := cmd.Short
		
		// Wrap description if needed
		wrappedDesc := Wrap(desc, WrapOpts{Width: col2Width})
		
		for i, descLine := range wrappedDesc {
			if i == 0 {
				// First line: name + description
				namePart := name + strings.Repeat(" ", col1Width-displayWidth(cmd.Name))
				lines = append(lines, namePart+descLine)
			} else {
				// Continuation lines: indent to align with description
				indent := strings.Repeat(" ", col1Width)
				lines = append(lines, indent+descLine)
			}
		}
	}
	
	return lines
}

// RenderHelpDetail produces detailed help for a single command per PROMPT1.md §4.
// Sections: NAME, USAGE, DESCRIPTION, SUBCOMMANDS, FLAGS, EXAMPLES, SEE ALSO.
func RenderHelpDetail(cmd Command, toolName string, width int, theme *Theme) []string {
	if width < 40 {
		width = 40
	}
	if width > 100 {
		width = 100
	}
	
	var lines []string
	
	// NAME section
	lines = append(lines, theme.Group.Render("NAME"))
	nameText := cmd.Name + " - " + cmd.Short
	wrapped := Wrap(nameText, WrapOpts{Width: width - 2})
	for _, line := range wrapped {
		lines = append(lines, "  "+line)
	}
	
	// USAGE section (if available)
	if cmd.Usage != "" {
		lines = append(lines, "")
		lines = append(lines, theme.Group.Render("USAGE"))
		usage := cmd.Usage
		if !strings.Contains(usage, cmd.Name) {
			// Prefix with command name if not present
			usage = cmd.Name + " " + usage
		}
		wrapped := Wrap(usage, WrapOpts{Width: width - 2})
		for _, line := range wrapped {
			lines = append(lines, "  "+line)
		}
	}
	
	// DESCRIPTION section (Long field)
	if cmd.Long != "" {
		lines = append(lines, "")
		lines = append(lines, theme.Group.Render("DESCRIPTION"))
		wrapped := Wrap(cmd.Long, WrapOpts{Width: width - 2})
		for _, line := range wrapped {
			lines = append(lines, "  "+line)
		}
	}
	
	// ALIASES section (if available)
	if len(cmd.Aliases) > 0 {
		lines = append(lines, "")
		lines = append(lines, theme.Group.Render("ALIASES"))
		aliasText := strings.Join(cmd.Aliases, ", ")
		wrapped := Wrap(aliasText, WrapOpts{Width: width - 2})
		for _, line := range wrapped {
			lines = append(lines, "  "+line)
		}
	}
	
	// SUBCOMMANDS section (if available)
	if len(cmd.Subs) > 0 {
		lines = append(lines, "")
		lines = append(lines, theme.Group.Render("SUBCOMMANDS"))
		subText := strings.Join(cmd.Subs, ", ")
		wrapped := Wrap(subText, WrapOpts{Width: width - 2})
		for _, line := range wrapped {
			lines = append(lines, "  "+line)
		}
	}
	
	// FLAGS section (if available)
	if len(cmd.Flags) > 0 {
		lines = append(lines, "")
		lines = append(lines, theme.Group.Render("FLAGS"))
		flagLines := renderFlags(cmd.Flags, width-2, theme)
		for _, line := range flagLines {
			lines = append(lines, "  "+line)
		}
	}
	
	// EXAMPLES section (if available)
	if len(cmd.Examples) > 0 {
		lines = append(lines, "")
		lines = append(lines, theme.Group.Render("EXAMPLES"))
		for i, ex := range cmd.Examples {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "  "+theme.Detail.Render(ex.Cmd))
			if ex.Note != "" {
				noteWrapped := Wrap(ex.Note, WrapOpts{Width: width - 4})
				for _, line := range noteWrapped {
					lines = append(lines, "    "+line)
				}
			}
		}
	}
	
	// SEE ALSO section (if available)
	if len(cmd.SeeAlso) > 0 {
		lines = append(lines, "")
		lines = append(lines, theme.Group.Render("SEE ALSO"))
		seeAlsoText := strings.Join(cmd.SeeAlso, ", ")
		wrapped := Wrap(seeAlsoText, WrapOpts{Width: width - 2})
		for _, line := range wrapped {
			lines = append(lines, "  "+line)
		}
	}
	
	// DEPRECATED notice (if applicable)
	if cmd.Deprecated != "" {
		lines = append(lines, "")
		lines = append(lines, theme.Group.Render("⚠ WARNING"))
		deprecatedText := "DEPRECATED: " + cmd.Deprecated
		wrapped := Wrap(deprecatedText, WrapOpts{Width: width - 2})
		for _, line := range wrapped {
			lines = append(lines, "  "+line)
		}
	}
	
	return lines
}

// renderFlags produces formatted flag descriptions.
func renderFlags(flags []Flag, width int, theme *Theme) []string {
	var lines []string
	
	for _, f := range flags {
		// Flag line: --name, -s
		flagParts := []string{}
		if f.Name != "" {
			flagParts = append(flagParts, f.Name)
		}
		if f.Short != "" {
			flagParts = append(flagParts, f.Short)
		}
		flagLine := strings.Join(flagParts, ", ")
		
		// Add type and default if available
		if f.Type != "" && f.Type != "bool" {
			flagLine += " <" + f.Type + ">"
		}
		if f.Required {
			flagLine += " (required)"
		}
		if f.Default != "" {
			flagLine += " [default: " + f.Default + "]"
		}
		
		lines = append(lines, theme.Detail.Render(flagLine))
		
		// Description wrapped with indent
		if f.Desc != "" {
			descWrapped := Wrap(f.Desc, WrapOpts{Width: width - 2})
			for _, line := range descWrapped {
				lines = append(lines, "  "+line)
			}
		}
		
		lines = append(lines, "")
	}
	
	// Remove trailing blank line
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	
	return lines
}

// RenderHelpNotFound produces help for an unknown command with suggestions.
func RenderHelpNotFound(query string, cmds []Command, theme *Theme) []string {
	var lines []string
	
	lines = append(lines, theme.Group.Render(fmt.Sprintf("Unknown command: %s", query)))
	lines = append(lines, "")
	
	// Find suggestions using edit distance
	suggestions := findSuggestions(query, cmds, 3)
	
	if len(suggestions) > 0 {
		lines = append(lines, "Did you mean:")
		for _, s := range suggestions {
			lines = append(lines, "  "+theme.Detail.Render(s.Name)+" - "+s.Short)
		}
		lines = append(lines, "")
	}
	
	lines = append(lines, "Type 'help' to see all commands.")
	
	return lines
}

// findSuggestions finds commands similar to the query using edit distance.
func findSuggestions(query string, cmds []Command, maxSuggestions int) []Command {
	type candidate struct {
		cmd      Command
		distance int
	}
	
	var candidates []candidate
	
	// Add built-ins
	builtins := []Command{
		{Name: "help", Short: "List commands, or describe one"},
		{Name: "clear", Short: "Clear the session transcript"},
		{Name: "quit", Short: "Leave the TUI (Ctrl+D also works)"},
	}
	allCmds := append([]Command{}, cmds...)
	allCmds = append(allCmds, builtins...)
	
	for _, cmd := range allCmds {
		dist := levenshtein(query, cmd.Name)
		if dist <= 3 { // Only suggest if distance is reasonable
			candidates = append(candidates, candidate{cmd, dist})
		}
	}
	
	// Sort by distance, then alphabetically
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distance != candidates[j].distance {
			return candidates[i].distance < candidates[j].distance
		}
		return candidates[i].cmd.Name < candidates[j].cmd.Name
	})
	
	// Return top N
	result := []Command{}
	for i := 0; i < len(candidates) && i < maxSuggestions; i++ {
		result = append(result, candidates[i].cmd)
	}
	
	return result
}

// levenshtein computes the Levenshtein distance between two strings.
func levenshtein(a, b string) int {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	
	// Create matrix
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
	}
	
	// Initialize first row and column
	for i := 0; i <= len(a); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(b); j++ {
		matrix[0][j] = j
	}
	
	// Fill matrix
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			matrix[i][j] = min3(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}
	
	return matrix[len(a)][len(b)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// GetHelpLines is the main entry point for rendering help.
// If query is empty, renders index. Otherwise renders detail for that command.
func GetHelpLines(query string, cmds []Command, toolName string, width int, theme *Theme) []string {
	if query == "" {
		return RenderHelpIndex(cmds, toolName, width, theme)
	}
	
	// Search for the command (including built-ins)
	builtins := []Command{
		{
			Name:  "help",
			Short: "List commands, or describe one",
			Long:  "Display help information. Run 'help' to see all commands, or 'help <command>' for details about a specific command.",
			Usage: "[command]",
		},
		{
			Name:  "clear",
			Short: "Clear the session transcript",
			Long:  "Clears the session transcript, removing all output from the screen. The command history is preserved.",
		},
		{
			Name:  "quit",
			Short: "Leave the TUI (Ctrl+D also works)",
			Long:  "Exit the interactive session. All unsaved state will be lost. You can also press Ctrl+D to quit.",
		},
	}
	
	allCmds := append([]Command{}, cmds...)
	allCmds = append(allCmds, builtins...)
	
	for _, cmd := range allCmds {
		if cmd.Name == query {
			return RenderHelpDetail(cmd, toolName, width, theme)
		}
		// Check aliases
		for _, alias := range cmd.Aliases {
			if alias == query {
				return RenderHelpDetail(cmd, toolName, width, theme)
			}
		}
	}
	
	// Not found
	return RenderHelpNotFound(query, cmds, theme)
}
