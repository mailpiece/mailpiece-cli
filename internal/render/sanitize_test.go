package render

import "testing"

func TestSanitizeTerminal(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello", "hello"},
		{"keeps newline and tab", "a\nb\tc", "a\nb\tc"},
		{"strips CSI colour", "\x1b[31mred\x1b[0m", "red"},
		{"strips OSC title", "\x1b]0;owned\x07text", "text"},
		{"strips OSC 8 hyperlink", "\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\", "link"},
		{"drops bell and backspace", "a\x07b\x08c", "abc"},
		{"drops carriage return", "a\rb", "ab"},
		{"drops DEL", "a\x7fb", "ab"},
		{"drops C1 controls", "a\u0085b", "ab"},
		{"keeps other unicode", "café ✓", "café ✓"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeTerminal(tt.input); got != tt.want {
				t.Errorf("SanitizeTerminal(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeSingleLine(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"one\ntwo", "one two"},
		{"one\r\ntwo\rthree", "one two three"},
		{"  spaced \t out  ", "spaced out"},
		{"\x1b[1mbold\x1b[0m\nnext", "bold next"},
	}
	for _, tt := range tests {
		if got := SanitizeSingleLine(tt.input); got != tt.want {
			t.Errorf("SanitizeSingleLine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
