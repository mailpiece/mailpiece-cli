package commands

import (
	"testing"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

func TestUserCommands(t *testing.T) {
	newsletterRun(t, userCmd, []newsletterCase{
		{name: "show", args: []string{"user", "show", "u1"}, method: "GET", path: "/users/u1.json"},
		{
			name: "update", args: []string{"user", "update", "u1", "--name", "Ana Ruiz"},
			method: "PATCH", path: "/users/u1.json", body: map[string]any{"name": "Ana Ruiz"},
		},
		{name: "update needs a name", args: []string{"user", "update", "u1"}, exit: errors.ExitUsage},
	})
}

func TestUserShowPrintsTheUser(t *testing.T) {
	mock := NewMockClient().WithGetData(map[string]any{"id": "u1", "name": "Ana", "role": "admin", "active": true})
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	_, err := runCobraWithArgs("user", "show", "u1")
	assertExitCode(t, err, 0)
	if data := responseDataMap(t, lastResult); data["role"] != "admin" {
		t.Errorf("expected the user, got %#v", data)
	}
}
