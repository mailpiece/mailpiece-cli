package commands

import (
	"testing"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

func TestSearch(t *testing.T) {
	t.Run("sends the words as one query and counts both lists", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.WithGetData(map[string]any{
			"posts":       []any{map[string]any{"id": "p1", "title": "Welcome back"}},
			"subscribers": []any{map[string]any{"id": "s1", "email": "ana@example.com"}, map[string]any{"id": "s2", "email": "bea@example.com"}},
		})

		assertExitCode(t, searchCmd.RunE(searchCmd, []string{"welcome", "back"}), 0)

		if got := mock.GetCalls[0].Path; got != "/search.json?q=welcome+back" {
			t.Errorf("path = %q", got)
		}
		if result.Response.Summary != `1 posts and 2 subscribers for "welcome back"` {
			t.Errorf("summary = %q", result.Response.Summary)
		}
		data := responseDataMap(t, result)
		if subscribers, _ := data["subscribers"].([]any); len(subscribers) != 2 {
			t.Errorf("subscribers = %#v", data["subscribers"])
		}
	})

	t.Run("refuses a blank query", func(t *testing.T) {
		mock, _ := mockTest(t)
		assertExitCode(t, searchCmd.RunE(searchCmd, []string{"  "}), errors.ExitInvalidArgs)
		if len(mock.GetCalls) != 0 {
			t.Errorf("a refused search sent %d requests", len(mock.GetCalls))
		}
	})
}
