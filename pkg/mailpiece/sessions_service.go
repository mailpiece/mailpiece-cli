package mailpiece

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// sessionsMaxResponseBytes bounds a session endpoint's response body.
const sessionsMaxResponseBytes = 1 << 20

// SessionsService signs a person in or up with an email and a one-time code,
// the way the browser does, no password: POST /session.json to send the code,
// then POST /session/magic_link.json to redeem it. A new identity
// (RequiresSignupCompletion) finishes with POST /signup/completion.json.
//
// This runs before any access token exists, so it does not go through Client:
// it carries the pending_authentication_token and session_token cookies the
// controllers set, in a cookie jar of its own, exactly as a browser would.
type SessionsService struct {
	httpClient *http.Client
	baseURL    string
}

// NewSessionsService builds a SessionsService against baseURL. Plain HTTP is
// only allowed for localhost, as the rest of the SDK requires.
func NewSessionsService(baseURL string) (*SessionsService, error) {
	baseURL = NormalizeBaseURL(baseURL)
	if err := RequireSecureEndpoint(baseURL); err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}
	return &SessionsService{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				return RequireSecureEndpoint(req.URL.String())
			},
		},
	}, nil
}

// Start requests a magic link code for emailAddress: POST /session.json.
func (s *SessionsService) Start(ctx context.Context, emailAddress string) (*SessionStartResult, error) {
	_, header, err := s.post(ctx, "/session.json", map[string]any{"email_address": emailAddress})
	if err != nil {
		return nil, err
	}
	pending := s.cookie("pending_authentication_token")
	if pending == "" {
		return nil, fmt.Errorf("server did not set a pending_authentication_token cookie")
	}
	return &SessionStartResult{
		PendingAuthenticationToken: pending,
		DevCode:                    header.Get("X-Magic-Link-Code"),
	}, nil
}

// Verify redeems code against the pending token a prior Start returned:
// POST /session/magic_link.json.
func (s *SessionsService) Verify(ctx context.Context, pendingAuthenticationToken, code string) (*SessionVerifyResult, error) {
	s.setCookie("pending_authentication_token", pendingAuthenticationToken)

	data, _, err := s.post(ctx, "/session/magic_link.json", map[string]any{"code": code})
	if err != nil {
		return nil, err
	}
	sessionToken := s.cookie("session_token")
	if sessionToken == "" {
		return nil, fmt.Errorf("server did not set a session_token cookie")
	}
	requiresCompletion, _ := data["requires_signup_completion"].(bool)
	return &SessionVerifyResult{
		SessionToken:             sessionToken,
		RequiresSignupCompletion: requiresCompletion,
	}, nil
}

// CompleteSignup finishes a new identity's signup, creating its account with
// fullName as the owner's name, using sessionToken from a prior Verify:
// POST /signup/completion.json (Signups::CompletionsController#create).
func (s *SessionsService) CompleteSignup(ctx context.Context, sessionToken, fullName string) error {
	s.setCookie("session_token", sessionToken)

	_, _, err := s.post(ctx, "/signup/completion.json", map[string]any{
		"signup": map[string]any{
			"full_name": fullName,
		},
	})
	return err
}

// Identity fetches GET /my/identity.json using sessionToken from a prior Verify.
func (s *SessionsService) Identity(ctx context.Context, sessionToken string) (*Identity, error) {
	s.setCookie("session_token", sessionToken)

	data, err := s.get(ctx, "/my/identity.json")
	if err != nil {
		return nil, err
	}
	var identity Identity
	if err := remarshal(data, &identity); err != nil {
		return nil, err
	}
	return &identity, nil
}

// CreateAccessToken creates a personal access token for the CLI under
// accountSlug, using sessionToken from a prior Verify:
// POST /:slug/my/access_tokens.json (My::AccessTokensController#create).
func (s *SessionsService) CreateAccessToken(ctx context.Context, sessionToken, accountSlug, description, permission string) (*AccessToken, error) {
	s.setCookie("session_token", sessionToken)

	path := "/" + url.PathEscape(NormalizeAccountSlug(accountSlug)) + "/my/access_tokens.json"
	data, _, err := s.post(ctx, path, map[string]any{
		"access_token": map[string]any{
			"description": description,
			"permission":  permission,
		},
	})
	if err != nil {
		return nil, err
	}
	var token AccessToken
	if err := remarshal(data, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

// SessionHTTPError is an error response from a session endpoint, carrying the
// status code so a caller can tell a wrong code (401) from anything else.
type SessionHTTPError struct {
	StatusCode int
	Message    string
}

func (e *SessionHTTPError) Error() string { return e.Message }

// post makes a JSON POST request against the session flow's own cookie-jar
// client and returns the parsed response body and headers.
func (s *SessionsService) post(ctx context.Context, path string, body any) (map[string]any, http.Header, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", DefaultUserAgent)

	return s.do(req)
}

// get makes a GET request against the session flow's own cookie-jar client.
func (s *SessionsService) get(ctx context.Context, path string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", DefaultUserAgent)

	data, _, err := s.do(req)
	return data, err
}

func (s *SessionsService) do(req *http.Request) (map[string]any, http.Header, error) {
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, nil, ErrNetwork(err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := limitedReadAll(resp.Body, sessionsMaxResponseBytes)
	if err != nil {
		return nil, resp.Header, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var parsed struct {
			Error   string   `json:"error"`
			Message string   `json:"message"`
			Errors  []string `json:"errors"`
		}
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if json.Unmarshal(respBody, &parsed) == nil {
			switch {
			case parsed.Message != "":
				msg = parsed.Message
			case parsed.Error != "":
				msg = parsed.Error
			case len(parsed.Errors) > 0:
				msg = strings.Join(parsed.Errors, ", ")
			}
		}
		return nil, resp.Header, &SessionHTTPError{StatusCode: resp.StatusCode, Message: msg}
	}

	if len(respBody) == 0 {
		return map[string]any{}, resp.Header, nil
	}
	var data map[string]any
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, resp.Header, fmt.Errorf("parsing response: %w", err)
	}
	return data, resp.Header, nil
}

// cookie returns a cookie's value from the jar, or "".
func (s *SessionsService) cookie(name string) string {
	u, err := url.Parse(s.baseURL)
	if err != nil {
		return ""
	}
	for _, c := range s.httpClient.Jar.Cookies(u) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// setCookie sets a cookie in the jar for the API's origin.
func (s *SessionsService) setCookie(name, value string) {
	u, err := url.Parse(s.baseURL)
	if err != nil {
		return
	}
	s.httpClient.Jar.SetCookies(u, []*http.Cookie{
		{Name: name, Value: value, Secure: u.Scheme == "https", HttpOnly: true},
	})
}

// remarshal JSON-round-trips a decoded response body into a typed value.
func remarshal(data any, v any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}
