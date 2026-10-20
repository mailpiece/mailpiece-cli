package mailpiece

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestNewSessionsServiceRequiresASecureEndpoint(t *testing.T) {
	if _, err := NewSessionsService("http://mailpiece.example.com"); err == nil {
		t.Fatal("plain HTTP off localhost should be refused")
	}
	if s, err := NewSessionsService("http://localhost:3000/"); err != nil || s.baseURL != "http://localhost:3000" {
		t.Fatalf("localhost should pass with its slash trimmed, got %v", err)
	}
}

func TestSessionsSignIn(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session.json":
			http.SetCookie(w, &http.Cookie{Name: "pending_authentication_token", Value: "pending-1", Path: "/"})
			w.Header().Set("X-Magic-Link-Code", "123456")
			w.WriteHeader(http.StatusCreated)
		case "/session/magic_link.json":
			if c, err := r.Cookie("pending_authentication_token"); err != nil || c.Value != "pending-2" {
				t.Errorf("Verify should carry the pending token it was given, got %v", c)
			}
			http.SetCookie(w, &http.Cookie{Name: "session_token", Value: "session-1", Path: "/"})
			_, _ = w.Write([]byte(`{"requires_signup_completion":true}`))
		case "/my/identity.json":
			if c, err := r.Cookie("session_token"); err != nil || c.Value != "session-2" {
				t.Errorf("Identity should carry the session token it was given, got %v", c)
			}
			_, _ = w.Write([]byte(`{"id":"i1","accounts":[{"id":"a1","slug":"/834206594"}]}`))
		case "/834206594/my/access_tokens.json":
			if c, err := r.Cookie("session_token"); err != nil || c.Value != "session-3" {
				t.Errorf("CreateAccessToken should carry the session token it was given, got %v", c)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"t1","token":"secret","permission":"write"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
	})

	s, err := NewSessionsService(srv.URL)
	if err != nil {
		t.Fatalf("NewSessionsService: %v", err)
	}
	ctx := context.Background()

	started, err := s.Start(ctx, "ana@example.com")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if started.PendingAuthenticationToken != "pending-1" || started.DevCode != "123456" {
		t.Errorf("got %+v", started)
	}

	verified, err := s.Verify(ctx, "pending-2", "654321")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if verified.SessionToken != "session-1" || !verified.RequiresSignupCompletion {
		t.Errorf("got %+v", verified)
	}

	identity, err := s.Identity(ctx, "session-2")
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if identity.ID != "i1" || identity.Accounts[0].Slug != "/834206594" {
		t.Errorf("got %+v", identity)
	}

	token, err := s.CreateAccessToken(ctx, "session-3", "/834206594", "mailpiece-cli", "write")
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	if token.Token != "secret" {
		t.Errorf("got %+v", token)
	}

	reqs := srv.recorded()
	if len(reqs) != 4 {
		t.Fatalf("sent %d requests, want 4", len(reqs))
	}
	for i, method := range []string{"POST", "POST", "GET", "POST"} {
		if reqs[i].Method != method {
			t.Errorf("request %d method = %s, want %s", i, reqs[i].Method, method)
		}
		if ua := reqs[i].Header.Get("User-Agent"); ua != DefaultUserAgent {
			t.Errorf("request %d User-Agent = %q", i, ua)
		}
		if auth := reqs[i].Header.Get("Authorization"); auth != "" {
			t.Errorf("request %d carries an Authorization header: %q", i, auth)
		}
	}
	sdkAssertJSON(t, reqs[0].Body, `{"email_address":"ana@example.com"}`)
	sdkAssertJSON(t, reqs[1].Body, `{"code":"654321"}`)
	sdkAssertJSON(t, reqs[3].Body, `{"access_token":{"description":"mailpiece-cli","permission":"write"}}`)
}

func TestSessionsStartWithoutAPendingCookie(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusCreated, ``))
	s, _ := NewSessionsService(srv.URL)

	if _, err := s.Start(context.Background(), "ana@example.com"); err == nil || !strings.Contains(err.Error(), "pending_authentication_token") {
		t.Fatalf("got %v", err)
	}
}

func TestSessionsVerifyWithoutASessionCookie(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusOK, `{}`))
	s, _ := NewSessionsService(srv.URL)

	if _, err := s.Verify(context.Background(), "pending", "123456"); err == nil || !strings.Contains(err.Error(), "session_token") {
		t.Fatalf("got %v", err)
	}
}

func TestSessionsHTTPErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"a wrong code", http.StatusUnauthorized, `{"message":"That code didn't work"}`, "That code didn't work"},
		{"an error key", http.StatusUnprocessableEntity, `{"error":"Invalid email"}`, "Invalid email"},
		{"an errors list", http.StatusUnprocessableEntity, `{"errors":["Full name can't be blank","Full name is too long"]}`, "Full name can't be blank, Full name is too long"},
		{"no body", http.StatusTooManyRequests, ``, "HTTP 429"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newSDKServer(t, sdkReply(tc.status, tc.body))
			s, _ := NewSessionsService(srv.URL)

			_, err := s.Verify(context.Background(), "pending", "000000")
			var httpErr *SessionHTTPError
			if !errors.As(err, &httpErr) {
				t.Fatalf("expected a *SessionHTTPError, got %T: %v", err, err)
			}
			if httpErr.StatusCode != tc.status || httpErr.Message != tc.want {
				t.Errorf("got %d %q, want %d %q", httpErr.StatusCode, httpErr.Message, tc.status, tc.want)
			}
		})
	}
}

func TestSessionsNetworkError(t *testing.T) {
	s, _ := NewSessionsService("http://localhost:1")

	_, err := s.Start(context.Background(), "ana@example.com")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeNetwork {
		t.Fatalf("expected a network *Error, got %v", err)
	}
}

func TestSessionsRefuseARedirectToPlainHTTP(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://evil.example.com/session.json", http.StatusTemporaryRedirect)
	})
	s, _ := NewSessionsService(srv.URL)

	_, err := s.Start(context.Background(), "ana@example.com")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeNetwork {
		t.Fatalf("expected the redirect refused, got %v", err)
	}
	if strings.Contains(err.Error(), "/session.json") {
		t.Errorf("the refused URL should be projected to its origin, got %q", err)
	}
	if n := len(srv.recorded()); n != 1 {
		t.Errorf("sent %d requests, want 1", n)
	}
}

func TestSessionsCompleteSignup(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/signup/completion.json" {
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
		if c, err := r.Cookie("session_token"); err != nil || c.Value != "session-1" {
			t.Errorf("CompleteSignup should carry the session token it was given, got %v", c)
		}
		w.WriteHeader(http.StatusCreated)
	})
	s, _ := NewSessionsService(srv.URL)

	if err := s.CompleteSignup(context.Background(), "session-1", "Ana Pérez"); err != nil {
		t.Fatalf("CompleteSignup: %v", err)
	}

	reqs := srv.recorded()
	if len(reqs) != 1 {
		t.Fatalf("sent %d requests, want 1", len(reqs))
	}
	sdkAssertJSON(t, reqs[0].Body, `{"signup":{"full_name":"Ana Pérez"}}`)
}
