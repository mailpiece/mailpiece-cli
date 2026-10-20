package commands

import (
	"testing"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

func TestIdentityShow(t *testing.T) {
	t.Run("shows identity", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetResponse = &APIResponse{
			StatusCode: 200,
			Data: map[string]any{
				"id": "ident-123",
				"accounts": []any{
					map[string]any{
						"id":   "acc-1",
						"name": "Acme",
						"slug": "/834206594",
						"user": map[string]any{"id": "u1", "name": "Ana", "role": "admin"},
					},
				},
			},
		}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "", "https://mailpiece.example.com")
		defer resetTest()

		err := identityShowCmd.RunE(identityShowCmd, []string{})
		assertExitCode(t, err, 0)

		if got := mock.GetCalls[0].Path; got != "/my/identity.json" {
			t.Errorf("expected path '/my/identity.json', got '%s'", got)
		}
		data := responseDataMap(t, result)
		accounts, ok := data["accounts"].([]any)
		if !ok || len(accounts) != 1 {
			t.Fatalf("expected one account, got %#v", data)
		}
		user := accounts[0].(map[string]any)["user"].(map[string]any)
		if user["role"] != "admin" {
			t.Errorf("expected the user's role on the account, got %#v", user)
		}
		if got := registerBreadcrumb(result, "status"); got != "mailpiece auth status" {
			t.Errorf("expected status breadcrumb, got %q", got)
		}
		if got := registerBreadcrumb(result, "doctor"); got != "mailpiece doctor" {
			t.Errorf("expected doctor breadcrumb, got %q", got)
		}
	})

	t.Run("bare identity shows it too", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetResponse = &APIResponse{StatusCode: 200, Data: map[string]any{"id": "ident-123", "accounts": []any{}}}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "", "https://mailpiece.example.com")
		defer resetTest()

		err := identityCmd.RunE(identityCmd, []string{})
		assertExitCode(t, err, 0)
		if data := responseDataMap(t, result); data["id"] != "ident-123" {
			t.Errorf("expected identity data, got %#v", data)
		}
	})

	t.Run("requires authentication", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("", "", "https://mailpiece.example.com") // No token
		defer resetTest()

		err := identityShowCmd.RunE(identityShowCmd, []string{})
		assertExitCode(t, err, errors.ExitAuth)
	})

	t.Run("handles a rejected token", func(t *testing.T) {
		mock := NewMockClient().WithAuthError()
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "", "https://mailpiece.example.com")
		defer resetTest()

		err := identityShowCmd.RunE(identityShowCmd, []string{})
		assertExitCode(t, err, errors.ExitAuth)
	})
}
