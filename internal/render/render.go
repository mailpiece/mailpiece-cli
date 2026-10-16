// Package render provides styled and markdown output rendering for the Mailpiece CLI.
// The shared output.Writer falls back to JSON for FormatStyled/FormatMarkdown;
// this package supplies the app-specific rendering.
package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// Column maps a display header to a JSON field name.
type Column struct {
	Header string
	Field  string
}

// Columns is a slice of Column specs for table rendering.
type Columns []Column

// headerStyle is the style for table headers in styled output.
var headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).PaddingRight(1)

// cellStyle is the style for table data cells in styled output.
var cellStyle = lipgloss.NewStyle().PaddingRight(1)

// StyledList renders a slice of maps as a styled terminal table.
func StyledList(data []map[string]any, cols Columns, summary string) string {
	if len(data) == 0 {
		if summary != "" {
			return summary + "\n"
		}
		return "No results.\n"
	}

	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = c.Header
	}

	rows := make([][]string, 0, len(data))
	for _, item := range data {
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = extractCell(item, c.Field)
		}
		rows = append(rows, row)
	}

	t := table.New().
		Headers(headers...).
		Rows(rows...).
		BorderLeft(false).
		BorderRight(false).
		BorderTop(false).
		BorderBottom(false).
		BorderColumn(false).
		BorderHeader(true).
		Border(lipgloss.NormalBorder()).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return cellStyle
		})

	var sb strings.Builder
	if summary != "" {
		sb.WriteString(summary)
		sb.WriteString("\n\n")
	}
	sb.WriteString(t.String())
	sb.WriteString("\n")
	return sb.String()
}

// StyledDetail renders a single map as styled key-value pairs. A field holding a
// list of records follows as a table of its own.
func StyledDetail(data map[string]any, summary string) string {
	if data == nil {
		return "No data.\n"
	}

	fields, tables := splitDetail(data)

	var sb strings.Builder
	if summary != "" {
		sb.WriteString(SanitizeTerminal(summary))
		sb.WriteString("\n\n")
	}

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	for _, k := range fields {
		label := labelStyle.Render(k + ":")
		fmt.Fprintf(&sb, "%s %s\n", label, formatValue(data[k]))
	}
	for _, k := range tables {
		sb.WriteString("\n")
		sb.WriteString(labelStyle.Render(k + ":"))
		sb.WriteString("\n")
		records, _ := data[k].([]map[string]any)
		sb.WriteString(StyledList(records, InferColumns(records), ""))
	}
	return sb.String()
}

// splitDetail sorts a record's keys into its fields and the lists of records it
// holds, each kept apart to render as a table. A list of records is taken to
// []map[string]any in place.
func splitDetail(data map[string]any) (fields, tables []string) {
	for _, k := range sortedKeys(data) {
		if records, ok := recordList(data[k]); ok {
			data[k] = records
			tables = append(tables, k)
			continue
		}
		fields = append(fields, k)
	}
	return fields, tables
}

// recordList reports whether v is a non-empty list of records, and gives them.
func recordList(v any) ([]map[string]any, bool) {
	switch list := v.(type) {
	case []map[string]any:
		return list, len(list) > 0
	case []any:
		if len(list) == 0 {
			return nil, false
		}
		records := make([]map[string]any, 0, len(list))
		for _, item := range list {
			record, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			records = append(records, record)
		}
		return records, true
	}
	return nil, false
}

// extractCell gets a field value from a map as a table cell: one line, made safe
// for a terminal. Supports dot-separated paths for nested fields (e.g., "event.action").
func extractCell(m map[string]any, field string) string {
	parts := strings.SplitN(field, ".", 2)
	val, ok := m[parts[0]]
	if !ok {
		return ""
	}

	if len(parts) == 2 {
		if nested, ok := val.(map[string]any); ok {
			return extractCell(nested, parts[1])
		}
		return ""
	}

	return SanitizeSingleLine(formatValue(val))
}

// formatValue converts any value to a display string, made safe for a terminal.
// A list shows each item by its label, comma-separated, as basecamp-cli does.
func formatValue(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return SanitizeTerminal(val)
	case bool:
		if val {
			return "yes"
		}
		return "no"
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case map[string]any:
		return recordLabel(val)
	case []any:
		items := make([]string, 0, len(val))
		for _, item := range val {
			if record, ok := item.(map[string]any); ok {
				items = append(items, recordLabel(record))
			} else {
				items = append(items, SanitizeSingleLine(formatValue(item)))
			}
		}
		return strings.Join(items, ", ")
	case []map[string]any:
		items := make([]string, 0, len(val))
		for _, record := range val {
			items = append(items, recordLabel(record))
		}
		return strings.Join(items, ", ")
	default:
		return SanitizeTerminal(fmt.Sprintf("%v", val))
	}
}

// recordLabel names a record by its first display field.
func recordLabel(record map[string]any) string {
	for _, key := range []string{"name", "title", "filename", "text", "number", "slug", "id"} {
		if s, ok := record[key]; ok && s != nil {
			return SanitizeSingleLine(formatValue(s))
		}
	}
	return SanitizeSingleLine(fmt.Sprintf("%v", record))
}

// StyledSummary renders a summary message for mutations.
// If structured data is present, include it below the summary for human readability.
func StyledSummary(data map[string]any, summary string) string {
	if summary != "" {
		line := lipgloss.NewStyle().Bold(true).Render("✓ " + summary)
		if len(data) == 0 {
			return line + "\n"
		}
		return line + "\n\n" + StyledDetail(data, "")
	}
	if len(data) == 0 {
		return lipgloss.NewStyle().Bold(true).Render("✓ Done") + "\n"
	}
	return StyledDetail(data, "")
}

func sortedKeys(m map[string]any) []string {
	// Put common identifying fields first, then alphabetical
	priority := map[string]int{
		"id": 0, "number": 1, "name": 2, "title": 3,
		"email": 4, "status": 5,
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		pi, oki := priority[keys[i]]
		pj, okj := priority[keys[j]]
		if oki && okj {
			return pi < pj
		}
		if oki {
			return true
		}
		if okj {
			return false
		}
		return keys[i] < keys[j]
	})
	return keys
}

// InferColumns picks up to four columns for records with no columns of their
// own: identifying fields first, then the rest alphabetically. A field holding a
// record or a list is left out, as basecamp-cli leaves nested objects out.
func InferColumns(data []map[string]any) Columns {
	if len(data) == 0 {
		return Columns{{Header: "Value", Field: "id"}}
	}

	priority := map[string]int{
		"number":      0,
		"id":          1,
		"profile":     2,
		"name":        3,
		"title":       4,
		"text":        4,
		"description": 5,
		"active":      6,
		"status":      7,
		"base_url":    8,
	}

	keys := make([]string, 0, len(data[0]))
	for key, value := range data[0] {
		switch value.(type) {
		case map[string]any, []any, []map[string]any:
			continue
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		pi, okI := priority[keys[i]]
		pj, okJ := priority[keys[j]]
		if okI && okJ {
			if pi != pj {
				return pi < pj
			}
			return keys[i] < keys[j]
		}
		if okI {
			return true
		}
		if okJ {
			return false
		}
		return keys[i] < keys[j]
	})

	if len(keys) > 4 {
		keys = keys[:4]
	}

	cols := make(Columns, 0, len(keys))
	for _, key := range keys {
		cols = append(cols, Column{Header: humanizeFieldName(key), Field: key})
	}
	return cols
}

func humanizeFieldName(name string) string {
	parts := strings.Split(name, "_")
	for i, part := range parts {
		if part == "id" {
			parts[i] = "ID"
			continue
		}
		parts[i] = titleWord(part)
	}
	return strings.Join(parts, " ")
}

func titleWord(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(strings.ToLower(s))
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}
