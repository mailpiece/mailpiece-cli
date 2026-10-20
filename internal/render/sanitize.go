package render

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// SanitizeTerminal removes terminal escape sequences and control characters from
// text the API returns, which people write, before it reaches a
// terminal. After ansi.Strip it
// drops every remaining C0 control but newline and tab, DEL and the C1 range,
// which some terminals execute.
func SanitizeTerminal(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			return -1
		default:
			return r
		}
	}, s)
}

// SanitizeSingleLine is SanitizeTerminal for a one-line sink, such as a table
// cell: line breaks and runs of whitespace become single spaces.
func SanitizeSingleLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = SanitizeTerminal(s)
	return strings.Join(strings.Fields(s), " ")
}
