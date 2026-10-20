package commands

import (
	"testing"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

func TestTokenListRejectsUnexpectedArgs(t *testing.T) {
	if err := tokenListCmd.Args(tokenListCmd, []string{"extra"}); err == nil {
		t.Fatal("expected token list to reject unexpected positional args")
	}
	if err := tokenListCmd.Args(tokenListCmd, []string{}); err != nil {
		t.Fatalf("expected token list to allow no positional args, got %v", err)
	}
}

func TestTokenCreateRejectsUnexpectedArgs(t *testing.T) {
	if err := tokenCreateCmd.Args(tokenCreateCmd, []string{"extra"}); err == nil {
		t.Fatal("expected token create to reject unexpected positional args")
	}
	if err := tokenCreateCmd.Args(tokenCreateCmd, []string{}); err != nil {
		t.Fatalf("expected token create to allow no positional args, got %v", err)
	}
}

func TestTokenDeleteRequiresOneID(t *testing.T) {
	if err := tokenDeleteCmd.Args(tokenDeleteCmd, []string{}); err == nil {
		t.Fatal("expected token delete to require a token id")
	}
	if err := tokenDeleteCmd.Args(tokenDeleteCmd, []string{"a", "b"}); err == nil {
		t.Fatal("expected token delete to reject a second id")
	}
}

func TestTokenList(t *testing.T) {
	t.Run("lists the identity's tokens", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetWithPaginationResponse = &APIResponse{
			StatusCode: 200,
			Data: []any{
				map[string]any{"id": "tok-2", "description": "CI", "permission": "write", "created_at": "2026-09-30T10:00:00Z"},
				map[string]any{"id": "tok-1", "description": "Laptop", "permission": "read", "created_at": "2026-09-29T10:00:00Z"},
			},
		}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "", "https://mailpiece.example.com")
		defer resetTest()

		err := tokenListCmd.RunE(tokenListCmd, []string{})
		assertExitCode(t, err, 0)

		if got := mock.GetWithPaginationCalls[0].Path; got != "/my/access_tokens.json" {
			t.Errorf("expected path '/my/access_tokens.json', got '%s'", got)
		}
		if result.Response.Summary != "2 access tokens" {
			t.Errorf("expected summary '2 access tokens', got %q", result.Response.Summary)
		}
		items, ok := result.Response.Data.([]any)
		if !ok || len(items) != 2 {
			t.Fatalf("expected 2 tokens, got %#v", result.Response.Data)
		}
		if _, has := items[0].(map[string]any)["token"]; has {
			t.Errorf("a listed token carries no value, got %#v", items[0])
		}
		if got := registerBreadcrumb(result, "delete"); got != "mailpiece token delete TOKEN_ID" {
			t.Errorf("expected delete breadcrumb, got %q", got)
		}
	})

	t.Run("answers to ls", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetWithPaginationResponse = &APIResponse{StatusCode: 200, Data: []any{}}
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "", "https://mailpiece.example.com")
		defer resetTest()
		configureCLIUX()

		_, err := runCobraWithArgs("token", "ls")
		assertExitCode(t, err, 0)
		if len(mock.GetWithPaginationCalls) != 1 || mock.GetWithPaginationCalls[0].Path != "/my/access_tokens.json" {
			t.Errorf("expected GET '/my/access_tokens.json', got %#v", mock.GetWithPaginationCalls)
		}
	})

	t.Run("requires authentication", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("", "", "https://mailpiece.example.com")
		defer resetTest()

		err := tokenListCmd.RunE(tokenListCmd, []string{})
		assertExitCode(t, err, errors.ExitAuth)
	})
}

func TestTokenCreate(t *testing.T) {
	t.Run("creates a token and shows it once", func(t *testing.T) {
		mock := NewMockClient()
		mock.PostResponse = &APIResponse{
			StatusCode: 201,
			Data: map[string]any{
				"id":          "tok-1",
				"token":       "secret-value",
				"description": "Laptop",
				"permission":  "write",
			},
		}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "", "https://mailpiece.example.com")
		defer resetTest()

		tokenCreateDescription = "Laptop"
		tokenCreatePermission = "write"
		err := tokenCreateCmd.RunE(tokenCreateCmd, []string{})
		tokenCreateDescription = ""
		tokenCreatePermission = ""

		assertExitCode(t, err, 0)
		if got := mock.PostCalls[0].Path; got != "/my/access_tokens.json" {
			t.Errorf("expected path '/my/access_tokens.json', got '%s'", got)
		}
		body := mock.PostCalls[0].Body.(map[string]any)
		if body["description"] != "Laptop" || body["permission"] != "write" {
			t.Errorf("expected flat description and permission, got %#v", body)
		}
		if data := responseDataMap(t, result); data["token"] != "secret-value" {
			t.Errorf("expected the token in the response, got %#v", data)
		}
		if result.Response.Summary != "Save the token now: it will not be shown again." {
			t.Errorf("unexpected summary %q", result.Response.Summary)
		}
		if got := registerBreadcrumb(result, "delete"); got != "mailpiece token delete tok-1" {
			t.Errorf("expected delete breadcrumb, got %q", got)
		}
	})

	t.Run("no delete breadcrumb without an id", func(t *testing.T) {
		mock := NewMockClient()
		mock.PostResponse = &APIResponse{StatusCode: 201, Data: map[string]any{"token": "secret-value"}}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "", "https://mailpiece.example.com")
		defer resetTest()

		tokenCreateDescription = "Laptop"
		tokenCreatePermission = "read"
		err := tokenCreateCmd.RunE(tokenCreateCmd, []string{})
		tokenCreateDescription = ""
		tokenCreatePermission = ""

		assertExitCode(t, err, 0)
		if got := registerBreadcrumb(result, "delete"); got != "" {
			t.Errorf("expected no delete breadcrumb, got %q", got)
		}
		if got := registerBreadcrumb(result, "list"); got != "mailpiece token list" {
			t.Errorf("expected list breadcrumb, got %q", got)
		}
	})

	t.Run("requires description flag", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "", "https://mailpiece.example.com")
		defer resetTest()

		tokenCreatePermission = "read"
		err := tokenCreateCmd.RunE(tokenCreateCmd, []string{})
		tokenCreatePermission = ""

		assertExitCode(t, err, errors.ExitInvalidArgs)
	})

	t.Run("requires permission flag", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "", "https://mailpiece.example.com")
		defer resetTest()

		tokenCreateDescription = "Laptop"
		err := tokenCreateCmd.RunE(tokenCreateCmd, []string{})
		tokenCreateDescription = ""

		assertExitCode(t, err, errors.ExitInvalidArgs)
	})

	t.Run("refuses a permission other than read or write", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "", "https://mailpiece.example.com")
		defer resetTest()

		tokenCreateDescription = "Laptop"
		tokenCreatePermission = "admin"
		err := tokenCreateCmd.RunE(tokenCreateCmd, []string{})
		tokenCreateDescription = ""
		tokenCreatePermission = ""

		assertExitCode(t, err, errors.ExitInvalidArgs)
		if len(mock.PostCalls) != 0 {
			t.Errorf("expected no request, got %d", len(mock.PostCalls))
		}
	})

	t.Run("requires authentication", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("", "", "https://mailpiece.example.com")
		defer resetTest()

		err := tokenCreateCmd.RunE(tokenCreateCmd, []string{})
		assertExitCode(t, err, errors.ExitAuth)
	})
}

func TestTokenDelete(t *testing.T) {
	t.Run("deletes a token", func(t *testing.T) {
		mock := NewMockClient()
		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "", "https://mailpiece.example.com")
		defer resetTest()

		err := tokenDeleteCmd.RunE(tokenDeleteCmd, []string{"tok-1"})
		assertExitCode(t, err, 0)

		if got := mock.DeleteCalls[0].Path; got != "/my/access_tokens/tok-1.json" {
			t.Errorf("expected DELETE '/my/access_tokens/tok-1.json', got '%s'", got)
		}
		data := responseDataMap(t, result)
		if data["deleted"] != true || data["id"] != "tok-1" {
			t.Errorf("expected deleted token id, got %#v", data)
		}
		if got := registerBreadcrumb(result, "list"); got != "mailpiece token list" {
			t.Errorf("expected list breadcrumb, got %q", got)
		}
	})

	t.Run("answers to rm", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "", "https://mailpiece.example.com")
		defer resetTest()
		configureCLIUX()

		_, err := runCobraWithArgs("token", "rm", "tok-1")
		assertExitCode(t, err, 0)
		if len(mock.DeleteCalls) != 1 || mock.DeleteCalls[0].Path != "/my/access_tokens/tok-1.json" {
			t.Errorf("expected DELETE '/my/access_tokens/tok-1.json', got %#v", mock.DeleteCalls)
		}
	})

	t.Run("handles not found", func(t *testing.T) {
		mock := NewMockClient()
		mock.DeleteError = errors.NewNotFoundError("Token not found")
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "", "https://mailpiece.example.com")
		defer resetTest()

		err := tokenDeleteCmd.RunE(tokenDeleteCmd, []string{"missing"})
		assertExitCode(t, err, errors.ExitNotFound)
	})
}
