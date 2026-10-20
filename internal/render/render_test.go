package render

import (
	"strings"
	"testing"
)

func TestStyledListEmpty(t *testing.T) {
	result := StyledList(nil, Columns{{Header: "ID", Field: "id"}}, "")
	if result != "No results.\n" {
		t.Errorf("expected 'No results.', got %q", result)
	}
}

func TestStyledListEmptyWithSummary(t *testing.T) {
	result := StyledList(nil, Columns{{Header: "ID", Field: "id"}}, "0 posts")
	if result != "0 posts\n" {
		t.Errorf("expected '0 posts', got %q", result)
	}
}

func TestStyledListRendersTable(t *testing.T) {
	data := []map[string]any{
		{"id": "1", "name": "Alpha"},
		{"id": "2", "name": "Beta"},
	}
	cols := Columns{
		{Header: "ID", Field: "id"},
		{Header: "Name", Field: "name"},
	}
	result := StyledList(data, cols, "2 posts")
	if !strings.Contains(result, "Alpha") {
		t.Error("expected output to contain 'Alpha'")
	}
	if !strings.Contains(result, "Beta") {
		t.Error("expected output to contain 'Beta'")
	}
	if !strings.Contains(result, "2 posts") {
		t.Error("expected output to contain summary")
	}
}

func TestStyledListNestedField(t *testing.T) {
	data := []map[string]any{
		{"id": "1", "journey": map[string]any{"name": "Welcome"}},
	}
	cols := Columns{{Header: "Journey", Field: "journey.name"}}
	result := StyledList(data, cols, "")
	if !strings.Contains(result, "Welcome") {
		t.Error("expected nested field 'Welcome' in output")
	}
}

func TestStyledDetailNil(t *testing.T) {
	result := StyledDetail(nil, "")
	if result != "No data.\n" {
		t.Errorf("expected 'No data.', got %q", result)
	}
}

func TestStyledDetailRendersKV(t *testing.T) {
	data := map[string]any{"id": "42", "name": "Test Post"}
	result := StyledDetail(data, "Post: Test Post")
	if !strings.Contains(result, "42") {
		t.Error("expected output to contain '42'")
	}
	if !strings.Contains(result, "Test Post") {
		t.Error("expected output to contain 'Test Post'")
	}
}

func TestStyledSummaryIncludesDataWhenPresent(t *testing.T) {
	data := map[string]any{"id": "42", "name": "Test Post"}
	result := StyledSummary(data, "Saved")
	if !strings.Contains(result, "✓ Saved") {
		t.Errorf("expected summary line, got %q", result)
	}
	if !strings.Contains(result, "Test Post") {
		t.Errorf("expected detail output, got %q", result)
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		input    any
		expected string
	}{
		{nil, ""},
		{"hello", "hello"},
		{true, "yes"},
		{false, "no"},
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{[]any{1, 2, 3}, "1, 2, 3"},
		{[]any{map[string]any{"name": "A"}, map[string]any{"title": "B"}}, "A, B"},
		{map[string]any{"name": "Foo"}, "Foo"},
		{map[string]any{"id": "123"}, "123"},
	}
	for _, tt := range tests {
		got := formatValue(tt.input)
		if got != tt.expected {
			t.Errorf("formatValue(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSortedKeys(t *testing.T) {
	m := map[string]any{
		"zzz":    1,
		"id":     2,
		"name":   3,
		"aaa":    4,
		"title":  5,
		"number": 6,
	}
	keys := sortedKeys(m)
	// Priority keys should come first: id, number, name, title, then alphabetical
	expected := []string{"id", "number", "name", "title", "aaa", "zzz"}
	for i, k := range expected {
		if keys[i] != k {
			t.Errorf("position %d: expected %q, got %q (full: %v)", i, k, keys[i], keys)
			break
		}
	}
}

func TestStyledDetailRendersRecordListsAsTables(t *testing.T) {
	data := map[string]any{
		"name": "Ada",
		"deliveries": []any{
			map[string]any{"id": "d1", "status": "sent"},
			map[string]any{"id": "d2", "status": "bounced"},
		},
	}
	result := StyledDetail(data, "")
	if !strings.Contains(result, "deliveries:") {
		t.Errorf("expected the list's label, got %q", result)
	}
	for _, want := range []string{"d1", "sent", "d2", "bounced"} {
		if !strings.Contains(result, want) {
			t.Errorf("expected %q in the nested table, got %q", want, result)
		}
	}
	if strings.Index(result, "Ada") > strings.Index(result, "deliveries:") {
		t.Errorf("expected the fields before the tables, got %q", result)
	}
}

func TestMarkdownDetailRendersRecordListsAsTables(t *testing.T) {
	data := map[string]any{
		"name":       "Ada",
		"deliveries": []any{map[string]any{"id": "d1", "status": "sent"}},
	}
	result := MarkdownDetail(data, "")
	if !strings.Contains(result, "**deliveries:**") || !strings.Contains(result, "| d1 | sent |") {
		t.Errorf("expected a nested table, got:\n%s", result)
	}
}

func TestStyledListSanitizesCells(t *testing.T) {
	data := []map[string]any{{"name": "line one\nline\x1b[31m two\x07"}}
	result := StyledList(data, Columns{{Header: "Name", Field: "name"}}, "")
	if strings.ContainsAny(result, "\x1b\x07") {
		t.Errorf("expected escapes stripped, got %q", result)
	}
	if !strings.Contains(result, "line one line two") {
		t.Errorf("expected one line, got %q", result)
	}
}
