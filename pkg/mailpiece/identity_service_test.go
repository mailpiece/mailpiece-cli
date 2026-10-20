package mailpiece

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIdentityGetMyIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/my/identity.json" {
			t.Fatalf("path = %s, want /my/identity.json", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"i1","accounts":[{"id":"a1","name":"Acme","slug":"/834206594","user":{"id":"u1","role":"admin","active":true}}]}`))
	}))
	defer server.Close()

	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test"})
	identity, _, err := client.Identity().GetMyIdentity(context.Background())
	if err != nil {
		t.Fatalf("GetMyIdentity: %v", err)
	}
	if identity.ID != "i1" || len(identity.Accounts) != 1 {
		t.Fatalf("got %+v", identity)
	}
	account := identity.Accounts[0]
	if account.Slug != "/834206594" || account.User.Role != "admin" || !account.User.Active {
		t.Errorf("got %+v", account)
	}
	if service := client.Identity(); service != client.Identity() {
		t.Error("the service should be built once")
	}
}

func TestAccessTokensService(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		srv := newSDKServer(t, sdkReply(http.StatusOK, `[{"id":"t2","description":"CI","permission":"write","created_at":"2026-09-30T10:00:00Z"},{"id":"t1","description":"cli","permission":"read","created_at":"2026-09-29T10:00:00Z"}]`))
		tokens, _, err := sdkClient(srv.URL).AccessTokens().List(context.Background())
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(tokens) != 2 || tokens[0].ID != "t2" || tokens[0].Permission != "write" || tokens[1].Description != "cli" {
			t.Errorf("got %+v", tokens)
		}
		if tokens[0].Token != "" {
			t.Errorf("a listed token has no value, got %q", tokens[0].Token)
		}
		reqs := srv.recorded()
		if len(reqs) != 1 || reqs[0].Method != "GET" || reqs[0].URI != "/my/access_tokens.json" {
			t.Fatalf("got %+v", reqs)
		}
	})

	t.Run("create", func(t *testing.T) {
		srv := newSDKServer(t, sdkReply(http.StatusCreated, `{"id":"t1","token":"secret","permission":"read"}`))
		token, _, err := sdkClient(srv.URL).AccessTokens().Create(context.Background(), &CreateAccessTokenRequest{Description: "cli", Permission: "read"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if token.Token != "secret" {
			t.Errorf("got %+v", token)
		}
		reqs := srv.recorded()
		if len(reqs) != 1 || reqs[0].Method != "POST" || reqs[0].URI != "/my/access_tokens.json" {
			t.Fatalf("got %+v", reqs)
		}
		sdkAssertJSON(t, reqs[0].Body, `{"description":"cli","permission":"read"}`)
	})

	t.Run("delete", func(t *testing.T) {
		srv := newSDKServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
		if _, err := sdkClient(srv.URL).AccessTokens().Delete(context.Background(), "t 1"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		reqs := srv.recorded()
		if len(reqs) != 1 || reqs[0].Method != "DELETE" || reqs[0].URI != "/my/access_tokens/t%201.json" {
			t.Fatalf("got %+v", reqs)
		}
	})

}
