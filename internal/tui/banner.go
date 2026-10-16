package tui

import (
	"io"
	"math/bits"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

// Banner is the braille-art Mailpiece mark + MAILPIECE text banner.
const Banner = "" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⣿⣤⠀⠛⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠛⠀⣤⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⣿⣿⣿⣤⠀⠛⣿⣿⣿⣿⣿⣿⣿⣿⠛⠀⣤⣿⣿⣿⠀⠀⠀⣿⣤⠀⣤⣿⠀⠀⣤⠛⣤⠀⠀⠛⠛⣿⠛⠛⠀⣿⠀⠀⠀⠀⠀⣿⠛⠛⠛⣤⠀⠛⠛⣿⠛⠛⠀⣿⠛⠛⠛⠛⠀⣤⠛⠛⠛⠛⠀⣿⠛⠛⠛⠛⠀\n" +
	"⠀⠀⠀⣿⣿⣿⣿⣿⣤⠀⠛⣿⣿⣿⣿⠛⠀⣤⣿⣿⣿⣿⣿⠀⠀⠀⣿⠛⣤⠛⣿⠀⣤⠛⠀⠛⣤⠀⠀⠀⣿⠀⠀⠀⣿⠀⠀⠀⠀⠀⣿⠀⠀⠀⣿⠀⠀⠀⣿⠀⠀⠀⣿⠀⠀⠀⠀⠀⣿⠀⠀⠀⠀⠀⣿⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣤⠀⠛⠛⠀⣤⣿⣿⣿⣿⣿⣿⣿⠀⠀⠀⣿⠀⠛⠀⣿⠀⣿⠀⠀⠀⣿⠀⠀⠀⣿⠀⠀⠀⣿⠀⠀⠀⠀⠀⣿⣤⣤⣤⠛⠀⠀⠀⣿⠀⠀⠀⣿⣤⣤⣤⠀⠀⣿⠀⠀⠀⠀⠀⣿⣤⣤⣤⠀⠀\n" +
	"⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣤⣤⣿⣿⣿⣿⣿⣿⣿⣿⣿⠀⠀⠀⣿⠀⠀⠀⣿⠀⣿⠛⠛⠛⣿⠀⠀⠀⣿⠀⠀⠀⣿⠀⠀⠀⠀⠀⣿⠀⠀⠀⠀⠀⠀⠀⣿⠀⠀⠀⣿⠀⠀⠀⠀⠀⣿⠀⠀⠀⠀⠀⣿⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠀⠀⠀⣿⠀⠀⠀⣿⠀⣿⠀⠀⠀⣿⠀⠀⠀⣿⠀⠀⠀⣿⠀⠀⠀⠀⠀⣿⠀⠀⠀⠀⠀⠀⠀⣿⠀⠀⠀⣿⠀⠀⠀⠀⠀⣿⠀⠀⠀⠀⠀⣿⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠀⠀⠀⣿⠀⠀⠀⣿⠀⣿⠀⠀⠀⣿⠀⣤⣤⣿⣤⣤⠀⣿⣤⣤⣤⣤⠀⣿⠀⠀⠀⠀⠀⣤⣤⣿⣤⣤⠀⣿⣤⣤⣤⣤⠀⠛⣤⣤⣤⣤⠀⣿⣤⣤⣤⣤⠀\n" +
	"⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀"

// BannerLines is the number of lines in the Banner.
const BannerLines = 12

// BannerWidth is the rune count of the widest line.
const BannerWidth = 80

// CompactBannerWidth is the visual width of the compact banner's widest line.
const CompactBannerWidth = markColLimit + 3 + len(tagline)

const (
	wordmark = "Mailpiece"
	tagline  = "a newsletter, written and mailed"
	tagRow   = 10
	tagCol   = BannerWidth - len(tagline)
)

// BannerStyle selects the banner layout.
type BannerStyle int

const (
	// BannerFull renders the full 80-column banner with brighter colors and tagline.
	BannerFull BannerStyle = iota
	// BannerCompact renders the mark with "Mailpiece" and the tagline to the right.
	BannerCompact
)

// GetBannerStyle returns the configured banner style from MAILPIECE_BANNER.
func GetBannerStyle() BannerStyle {
	if os.Getenv("MAILPIECE_BANNER") == "compact" {
		return BannerCompact
	}
	return BannerFull
}

const blankBraille = '⠀' // ⠀

// isDense returns true for braille characters with 3+ raised dots,
// which form the visible mark and text shapes. Characters with
// 0-2 dots are rendered as blank in colored mode to eliminate
// speckling and produce crisp shapes.
func isDense(ch rune) bool {
	if ch < 0x2800 || ch > 0x28FF {
		return false
	}
	return bits.OnesCount(uint(ch-0x2800)) > 2 // #nosec G115 -- range checked above
}

// MarkColor is the brand blue of the mark.
const MarkColor = lipgloss.Color("#0169FF")

// markTint is the mid-trail tint (50% blend towards white).
const markTint = lipgloss.Color("#80B4FF")

// brightMarkColor is a lighter blue for the full banner style.
const brightMarkColor = lipgloss.Color("#418EFF")

// markZone returns the zone index (0-5) for a column position.
// Zones 0-4 are bands of the mark, revealed in a staggered cascade;
// zone 5 is the MAILPIECE text.
func markZone(col int) int {
	switch {
	case col <= 7:
		return 0
	case col <= 12:
		return 1
	case col <= 16:
		return 2
	case col <= 21:
		return 3
	case col <= 25:
		return 4
	default:
		return 5
	}
}

// bannerGrid returns the banner as a rune grid and its line count.
func bannerGrid() ([][]rune, int) {
	lines := strings.Split(Banner, "\n")
	grid := make([][]rune, len(lines))
	for i, line := range lines {
		grid[i] = []rune(line)
	}
	return grid, len(lines)
}

// noColor returns true when color output should be suppressed.
func noColor() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}

// rendererForWriter returns a lipgloss renderer bound to the given writer,
// so that color-profile detection matches the actual output destination.
func rendererForWriter(w io.Writer) *lipgloss.Renderer {
	return lipgloss.NewRenderer(w)
}

// termWidth returns the terminal width for w, or 0 if unknown.
func termWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok {
		if width, _, err := term.GetSize(f.Fd()); err == nil {
			return width
		}
	}
	return 0
}

// bannerVisualRows returns the number of terminal rows the full banner
// occupies, accounting for soft-wrapping when the terminal is narrower
// than BannerWidth.
func bannerVisualRows(cols int) int {
	if cols <= 0 || cols >= BannerWidth {
		return BannerLines
	}
	rowsPerLine := (BannerWidth + cols - 1) / cols
	return BannerLines * rowsPerLine
}

// compactVisualRows computes the visual row count for the compact banner,
// accounting for the fact that most lines are markColLimit (26) wide
// while two lines have appended text making them wider.
func compactVisualRows(cols int) int {
	if cols <= 0 || cols >= CompactBannerWidth {
		return BannerLines
	}
	total := 0
	for i := 0; i < BannerLines; i++ {
		w := markColLimit
		switch i {
		case BannerLines - 3:
			w = markColLimit + 3 + len(wordmark)
		case BannerLines - 2:
			w = CompactBannerWidth
		}
		total += max(1, (w+cols-1)/cols)
	}
	return total
}

// fullBannerPlain returns the full banner as plain text with the tagline overlay.
func fullBannerPlain() string {
	lines := strings.Split(Banner, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i == tagRow {
			runes := []rune(line)
			b.WriteString(string(runes[:tagCol]))
			b.WriteString(tagline)
			end := tagCol + len(tagline)
			if end < len(runes) {
				b.WriteString(string(runes[end:]))
			}
		} else {
			b.WriteString(line)
		}
	}
	return b.String()
}

// RenderBanner returns the banner styled according to the configured BannerStyle.
// When w is nil, returns plain text. Otherwise creates a renderer bound to w
// so that color capabilities are detected correctly.
func RenderBanner(w io.Writer) string {
	style := GetBannerStyle()
	if w == nil {
		if style == BannerFull {
			return fullBannerPlain()
		}
		return compactBannerPlain()
	}
	re := rendererForWriter(w)
	if style == BannerFull {
		return renderFullStyledBannerWith(re)
	}
	return renderCompactBannerWith(re)
}

// renderBannerWith renders the full banner using the given renderer's styles.
func renderBannerWith(re *lipgloss.Renderer) string {
	var zoneStyles [6]lipgloss.Style
	for i := 0; i < 5; i++ {
		zoneStyles[i] = re.NewStyle().Foreground(MarkColor)
	}
	zoneStyles[5] = re.NewStyle() // default foreground for text

	var b strings.Builder
	for i, line := range strings.Split(Banner, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		runes := []rune(line)
		j := 0
		for j < len(runes) {
			z := markZone(j)
			k := j + 1
			for k < len(runes) && markZone(k) == z {
				k++
			}
			var run strings.Builder
			for x := j; x < k; x++ {
				if isDense(runes[x]) {
					run.WriteRune(runes[x])
				} else {
					run.WriteRune(blankBraille)
				}
			}
			b.WriteString(zoneStyles[z].Render(run.String()))
			j = k
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Compact banner (mark + side text)
// ---------------------------------------------------------------------------

const markColLimit = 26

// compactBannerPlain returns the compact banner as plain text (no ANSI).
func compactBannerPlain() string {
	lines := strings.Split(Banner, "\n")
	numLines := len(lines)
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		runes := []rune(line)
		if len(runes) > markColLimit {
			runes = runes[:markColLimit]
		}
		b.WriteString(string(runes))
		switch i {
		case numLines - 3:
			b.WriteString("   " + wordmark)
		case numLines - 2:
			b.WriteString("   " + tagline)
		}
	}
	return b.String()
}

// renderCompactBannerWith renders the mark with styled "Mailpiece" and tagline.
func renderCompactBannerWith(re *lipgloss.Renderer) string {
	var zoneStyles [5]lipgloss.Style
	for i := 0; i < 5; i++ {
		zoneStyles[i] = re.NewStyle().Foreground(MarkColor)
	}

	name := renderWordmarkText(re)
	tag := re.NewStyle().Foreground(lipgloss.Color("#888888")).Render(tagline)

	lines := strings.Split(Banner, "\n")
	numLines := len(lines)

	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		runes := []rune(line)
		if len(runes) > markColLimit {
			runes = runes[:markColLimit]
		}
		j := 0
		for j < len(runes) {
			z := markZone(j)
			if z >= 5 {
				break
			}
			k := j + 1
			for k < len(runes) && markZone(k) == z {
				k++
			}
			var run strings.Builder
			for x := j; x < k; x++ {
				if isDense(runes[x]) {
					run.WriteRune(runes[x])
				} else {
					run.WriteRune(blankBraille)
				}
			}
			b.WriteString(zoneStyles[z].Render(run.String())) // #nosec G602 -- z < 5 guarded above
			j = k
		}
		switch i {
		case numLines - 3:
			b.WriteString("   ")
			b.WriteString(name)
		case numLines - 2:
			b.WriteString("   ")
			b.WriteString(tag)
		}
	}
	return b.String()
}

// renderWordmarkText returns "Mailpiece" in the mark's tint, bold.
func renderWordmarkText(re *lipgloss.Renderer) string {
	return re.NewStyle().Foreground(markTint).Bold(true).Render(wordmark)
}

// ---------------------------------------------------------------------------
// Full styled banner (brighter colors + bold + tagline)
// ---------------------------------------------------------------------------

// renderFullStyledBannerWith renders the full 80-col banner with brighter
// colors, bold, and the tagline under the MAILPIECE text.
func renderFullStyledBannerWith(re *lipgloss.Renderer) string {
	var zoneStyles [6]lipgloss.Style
	for i := 0; i < 5; i++ {
		zoneStyles[i] = re.NewStyle().Foreground(brightMarkColor).Bold(true)
	}
	zoneStyles[5] = re.NewStyle().Bold(true)

	dim := re.NewStyle().Foreground(lipgloss.Color("#888888"))

	var b strings.Builder
	for i, line := range strings.Split(Banner, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		runes := []rune(line)
		doOverlay := i == tagRow
		overlayEnd := tagCol + len([]rune(tagline))

		j := 0
		for j < len(runes) {
			if doOverlay && j == tagCol {
				b.WriteString(dim.Render(tagline))
				j = overlayEnd
				if j >= len(runes) {
					break
				}
				continue
			}

			z := markZone(j)
			k := j + 1
			limit := len(runes)
			if doOverlay && j < tagCol && tagCol < limit {
				limit = tagCol
			}
			for k < limit && markZone(k) == z {
				k++
			}

			var run strings.Builder
			for x := j; x < k; x++ {
				if isDense(runes[x]) {
					run.WriteRune(runes[x])
				} else {
					run.WriteRune(blankBraille)
				}
			}
			b.WriteString(zoneStyles[z].Render(run.String()))
			j = k
		}
	}
	return b.String()
}
