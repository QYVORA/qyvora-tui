package tui

import "regexp"

// ansiPattern matches the CSI escape sequences a terminal style produces.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// stripANSI removes escape sequences from a string.
//
// Layout decisions have to be made on visible width, but the strings being
// measured are already styled, so the styling has to come off first.
func stripANSI(s string) string { return ansiPattern.ReplaceAllString(s, "") }
