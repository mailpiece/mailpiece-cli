package mailpiece

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultUserAgent is the default User-Agent header value.
const DefaultUserAgent = "mailpiece-sdk-go/" + Version

// Client is an HTTP client for the Mailpiece API.
// Client holds shared resources and is used to create AccountClient instances
// for specific Mailpiece accounts via the ForAccount method.
//
// Client is safe for concurrent use after construction. Do not modify
// the Config after the client is in use by multiple goroutines.
type Client struct {
	httpClient    *http.Client
	tokenProvider TokenProvider
	authStrategy  AuthStrategy
	cfg           *Config
	userAgent     string
	logger        *slog.Logger
	httpOpts      HTTPOptions
	hooks         Hooks

	// Account-independent services
	accessTokensMu sync.Mutex
	accessTokens   *AccessTokensService
	identityMu     sync.Mutex
	identity       *IdentityService
}

// AccountClient is an HTTP client bound to a specific Mailpiece account.
// Create an AccountClient using Client.ForAccount(slug).
// AccountClient is safe for concurrent use.
//
// The Mailpiece API takes the account's slug as the first segment of every
// account-scoped path. Creating multiple AccountClients via ForAccount is lightweight.
type AccountClient struct {
	parent    *Client
	accountID string
}

// Response wraps an API response.
type Response struct {
	Data       json.RawMessage
	StatusCode int
	Headers    http.Header
}

// UnmarshalData unmarshals the response data into the given value.
func (r *Response) UnmarshalData(v any) error {
	return json.Unmarshal(r.Data, v)
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) ClientOption {
	return func(client *Client) {
		client.httpClient = c
	}
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) ClientOption {
	return func(client *Client) {
		client.userAgent = ua
	}
}

// WithLogger sets a custom slog logger for debug output.
func WithLogger(l *slog.Logger) ClientOption {
	return func(client *Client) {
		if l != nil {
			client.logger = l
		}
	}
}

// WithAuthStrategy sets a custom authentication strategy.
// The default strategy is BearerAuth.
func WithAuthStrategy(strategy AuthStrategy) ClientOption {
	return func(client *Client) {
		client.authStrategy = strategy
	}
}

// NewClient creates a new API client with spec-driven defaults.
//
// The client automatically:
//   - Retries idempotent requests (GET/PUT/PATCH/DELETE) with exponential backoff
//   - Does NOT retry POST on 429/5xx (to avoid duplicating data)
//   - Respects Retry-After headers on 429 responses
//   - Follows pagination via Link headers
//
// NewClient panics when the base URL is plain HTTP on any host but localhost.
func NewClient(cfg *Config, tokenProvider TokenProvider, opts ...ClientOption) *Client {
	cfgCopy := *cfg
	cfgCopy.BaseURL = NormalizeBaseURL(cfgCopy.BaseURL)
	c := &Client{
		tokenProvider: tokenProvider,
		cfg:           &cfgCopy,
		userAgent:     DefaultUserAgent,
		logger:        slog.New(discardHandler{}),
		hooks:         NoopHooks{},
		httpOpts:      DefaultHTTPOptions(),
	}

	for _, opt := range opts {
		opt(c)
	}

	// Default to BearerAuth if no custom auth strategy was provided
	if c.authStrategy == nil {
		c.authStrategy = &BearerAuth{TokenProvider: c.tokenProvider}
	}

	if c.httpClient == nil {
		transport := c.httpOpts.Transport
		if transport == nil {
			transport = newDefaultTransport()
		}

		transport = &loggingTransport{inner: transport, client: c}

		c.httpClient = &http.Client{
			Timeout:   c.httpOpts.Timeout,
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				if len(via) > 0 && !isSameOrigin(req.URL.String(), via[0].URL.String()) {
					req.Header.Del("Authorization")
					req.Header.Del("Cookie")
				}
				return nil
			},
		}
	}

	// Validate configuration
	if c.cfg.BaseURL != "" && !isLocalhost(c.cfg.BaseURL) {
		if err := requireHTTPS(c.cfg.BaseURL); err != nil {
			panic("mailpiece: base URL must use HTTPS: " + c.cfg.BaseURL)
		}
	}
	if c.httpOpts.Timeout <= 0 {
		panic("mailpiece: timeout must be positive")
	}
	if c.httpOpts.MaxRetries < 0 {
		panic("mailpiece: max retries must be non-negative")
	}
	if c.httpOpts.MaxPages <= 0 {
		panic("mailpiece: max pages must be positive")
	}

	return c
}

// ForAccount returns an AccountClient bound to the specified Mailpiece account.
// The slug may carry the leading slash the API renders ("/834206594").
// ForAccount panics if the slug is empty.
func (c *Client) ForAccount(slug string) *AccountClient {
	slug = NormalizeAccountSlug(slug)
	if slug == "" {
		panic("mailpiece: ForAccount requires a non-empty account slug")
	}

	return &AccountClient{
		parent:    c,
		accountID: slug,
	}
}

func (ac *AccountClient) AccountID() string {
	return ac.accountID
}

// Get performs an account-scoped GET request.
func (ac *AccountClient) Get(ctx context.Context, path string) (*Response, error) {
	return ac.parent.doRequest(ctx, "GET", ac.accountPath(path), nil)
}

// Post performs an account-scoped POST request with a JSON body.
func (ac *AccountClient) Post(ctx context.Context, path string, body any) (*Response, error) {
	return ac.parent.doRequest(ctx, "POST", ac.accountPath(path), body)
}

// Put performs an account-scoped PUT request with a JSON body.
func (ac *AccountClient) Put(ctx context.Context, path string, body any) (*Response, error) {
	return ac.parent.doRequest(ctx, "PUT", ac.accountPath(path), body)
}

// Patch performs an account-scoped PATCH request with a JSON body.
func (ac *AccountClient) Patch(ctx context.Context, path string, body any) (*Response, error) {
	return ac.parent.doRequest(ctx, "PATCH", ac.accountPath(path), body)
}

// Delete performs an account-scoped DELETE request.
func (ac *AccountClient) Delete(ctx context.Context, path string) (*Response, error) {
	return ac.parent.doRequest(ctx, "DELETE", ac.accountPath(path), nil)
}

// GetAll fetches all pages for an account-scoped paginated resource.
func (ac *AccountClient) GetAll(ctx context.Context, path string) ([]json.RawMessage, error) {
	return ac.parent.GetAllWithLimit(ctx, ac.accountPath(path), 0)
}

// GetAllWithLimit fetches pages for an account-scoped paginated resource up to a limit.
func (ac *AccountClient) GetAllWithLimit(ctx context.Context, path string, limit int) ([]json.RawMessage, error) {
	return ac.parent.GetAllWithLimit(ctx, ac.accountPath(path), limit)
}

// accountPath prepends the account ID to the path.
// Absolute URLs are returned unchanged (e.g., pagination Link headers).
func (ac *AccountClient) accountPath(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	prefix := "/" + ac.accountID
	if strings.HasPrefix(path, prefix) {
		rest := path[len(prefix):]
		if rest == "" || rest[0] == '/' || rest[0] == '?' {
			return path
		}
	}
	return "/" + ac.accountID + path
}

// discardHandler is a slog.Handler that discards all log records.
type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (h discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return h }
func (h discardHandler) WithGroup(string) slog.Handler           { return h }

// Get performs a GET request.
func (c *Client) Get(ctx context.Context, path string) (*Response, error) {
	return c.doRequest(ctx, "GET", path, nil)
}

// Post performs a POST request with a JSON body.
func (c *Client) Post(ctx context.Context, path string, body any) (*Response, error) {
	return c.doRequest(ctx, "POST", path, body)
}

// Put performs a PUT request with a JSON body.
func (c *Client) Put(ctx context.Context, path string, body any) (*Response, error) {
	return c.doRequest(ctx, "PUT", path, body)
}

// Patch performs a PATCH request with a JSON body.
func (c *Client) Patch(ctx context.Context, path string, body any) (*Response, error) {
	return c.doRequest(ctx, "PATCH", path, body)
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) (*Response, error) {
	return c.doRequest(ctx, "DELETE", path, nil)
}

// GetAll fetches all pages for a paginated resource.
func (c *Client) GetAll(ctx context.Context, path string) ([]json.RawMessage, error) {
	return c.GetAllWithLimit(ctx, path, 0)
}

// GetAllWithLimit fetches pages for a paginated resource up to a limit.
func (c *Client) GetAllWithLimit(ctx context.Context, path string, limit int) ([]json.RawMessage, error) {
	var allResults []json.RawMessage
	baseURL, err := c.buildURL(path)
	if err != nil {
		return nil, err
	}
	ctx = markCallerURL(ctx, path)
	url := baseURL
	var page int

	for page = 1; page <= c.httpOpts.MaxPages; page++ {
		resp, err := c.doRequestURL(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}

		var items []json.RawMessage
		if err := json.Unmarshal(resp.Data, &items); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		// The app links a next page past the end of a list.
		if len(items) == 0 {
			break
		}
		allResults = append(allResults, items...)

		if limit > 0 && len(allResults) >= limit {
			allResults = allResults[:limit]
			break
		}

		nextURL := parseNextLink(resp.Headers.Get("Link"))
		if nextURL == "" {
			break
		}
		nextURL = resolveURL(url, nextURL)
		if !isSameOrigin(nextURL, baseURL) {
			return nil, fmt.Errorf("pagination Link header points to different origin: %s", nextURL)
		}
		url = nextURL
	}

	if page > c.httpOpts.MaxPages {
		c.logger.Warn("pagination capped", "maxPages", c.httpOpts.MaxPages)
	}

	return allResults, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body any) (*Response, error) {
	url, err := c.buildURL(path)
	if err != nil {
		return nil, err
	}
	return c.doRequestURL(markCallerURL(ctx, path), method, url, body)
}

// markCallerURL marks ctx when path is a caller's absolute URL rather than an API
// path: it can be a signed one, on any origin, and the hooks and the network error
// see it projected.
func markCallerURL(ctx context.Context, path string) context.Context {
	if strings.HasPrefix(path, "https://") {
		return markProjectedRequest(ctx)
	}
	return ctx
}

// trustedOrigin is the API origin a network error on this request may keep its cause
// beneath, or none for a request the hooks see projected.
func (c *Client) trustedOrigin(ctx context.Context) string {
	if isProjectedRequest(ctx) {
		return ""
	}
	return c.cfg.BaseURL
}

func (c *Client) doRequestURL(ctx context.Context, method, url string, body any) (*Response, error) {
	// The retry hook sees the URL as the request hooks do: projected on a request the
	// transport projects.
	displayURL := url
	if isProjectedRequest(ctx) {
		displayURL = projectURL(url, false)
	}
	// POST and operations that opt out via WithNoRetry: single attempt only.
	// Exception: POST requests marked as idempotent via WithIdempotent get full retry.
	if (method == "POST" && !isIdempotent(ctx)) || isNoRetry(ctx) {
		resp, err := c.singleRequest(ctx, method, url, body, 1)
		if err == nil {
			return resp, nil
		}
		// Only retry if this was a 401 that triggered successful token refresh
		var apiErr *Error
		if errors.As(err, &apiErr) && apiErr.Retryable && apiErr.Code == CodeAuth {
			c.logger.Debug("token refreshed, retrying mutation", "method", method)
			info := RequestInfo{Method: method, URL: displayURL, Attempt: 1}
			c.hooks.OnRetry(ctx, info, 2, err)
			return c.singleRequest(ctx, method, url, body, 2)
		}
		return nil, err
	}

	// Idempotent requests (GET, PUT, PATCH, DELETE): retry with exponential backoff
	var attempt int
	var lastErr error

	// MaxRetries is the total attempt count, floored at one: a cap of 0 means "no
	// retries", not "no request". The floor lives here because this loop is pre-check
	// and a directly-struct-built Client never passes through NewClient's validation.
	maxAttempts := max(c.httpOpts.MaxRetries, 1)

	for attempt = 1; attempt <= maxAttempts; attempt++ {
		resp, err := c.singleRequest(ctx, method, url, body, attempt)
		if err == nil {
			return resp, nil
		}

		var retryAfter time.Duration
		var retryable *retryableError
		var apiErr *Error
		switch {
		case errors.As(err, &retryable):
			lastErr = retryable.err
			retryAfter = retryable.retryAfter
		case errors.As(err, &apiErr):
			if !apiErr.Retryable {
				return nil, err
			}
			lastErr = err
		default:
			return nil, err
		}

		// After the final attempt there is no retry: don't compute or sleep a
		// backoff, or announce an attempt that will never be made.
		if attempt >= maxAttempts {
			break
		}

		delay := retryAfter
		if delay <= 0 {
			delay = c.backoffDelay(attempt)
		}

		c.logger.Debug("retrying request", "attempt", attempt, "maxAttempts", maxAttempts, "delay", delay, "error", lastErr)

		info := RequestInfo{Method: method, URL: displayURL, Attempt: attempt}
		c.hooks.OnRetry(ctx, info, attempt+1, lastErr)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			continue
		}
	}

	noun := "attempts"
	if maxAttempts == 1 {
		noun = "attempt"
	}
	return nil, fmt.Errorf("request failed after %d %s: %w", maxAttempts, noun, lastErr)
}

func (c *Client) singleRequest(ctx context.Context, method, url string, body any, attempt int) (*Response, error) {
	ctx = contextWithAttempt(ctx, attempt)

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	if err = c.authStrategy.Authenticate(ctx, req); err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	logURL := url
	if isProjectedRequest(ctx) {
		logURL = projectURL(url, false)
	}
	c.logger.Debug("http request", "method", method, "url", logURL, "attempt", attempt)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, networkError(err, c.trustedOrigin(ctx))
	}
	defer func() { _ = resp.Body.Close() }()

	c.logger.Debug("http response", "status", resp.StatusCode)
	return readResponse(resp, method, url)
}

// readResponse maps an API response to its data, or to the refusal it carries.
func readResponse(resp *http.Response, method, url string) (*Response, error) {
	requestID := resp.Header.Get("X-Request-Id")

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		respBody, err := limitedReadAll(resp.Body, MaxResponseBodyBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode == http.StatusNoContent {
			respBody = json.RawMessage("null")
		}

		return &Response{
			Data:       respBody,
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
		}, nil

	case http.StatusTooManyRequests: // 429
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, &retryableError{
			err: &Error{
				Code:       CodeRateLimit,
				Message:    "Rate limited",
				HTTPStatus: 429,
				Retryable:  true,
				RequestID:  requestID,
			},
			retryAfter: time.Duration(retryAfter) * time.Second,
		}

	case http.StatusUnauthorized: // 401
		// The app answers a read token's write with 401, not 403.
		if !isRead(method) {
			return nil, &Error{Code: CodeAuth, Message: "Authentication failed", Hint: ReadOnlyTokenHint, HTTPStatus: 401, RequestID: requestID}
		}
		return nil, &Error{Code: CodeAuth, Message: "Authentication failed", HTTPStatus: 401, RequestID: requestID}

	case http.StatusForbidden: // 403
		respBody, _ := limitedReadAll(resp.Body, MaxErrorBodyBytes)
		return nil, &Error{Code: CodeForbidden, Message: forbiddenMessage(respBody), HTTPStatus: 403, RequestID: requestID}

	case http.StatusNotFound: // 404
		return nil, &Error{Code: CodeNotFound, Message: fmt.Sprintf("Resource not found: %s", url), HTTPStatus: 404, RequestID: requestID}

	case http.StatusUnprocessableEntity: // 422
		respBody, _ := limitedReadAll(resp.Body, MaxErrorBodyBytes)
		return nil, &Error{Code: CodeValidation, Message: validationMessage(respBody), HTTPStatus: 422, RequestID: requestID}

	case http.StatusInternalServerError: // 500
		return nil, &Error{Code: CodeAPI, Message: "Server error (500)", HTTPStatus: 500, Retryable: true, RequestID: requestID}

	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout: // 502, 503, 504
		return nil, &Error{
			Code:       CodeAPI,
			Message:    fmt.Sprintf("Gateway error (%d)", resp.StatusCode),
			HTTPStatus: resp.StatusCode,
			Retryable:  true,
			RequestID:  requestID,
		}

	default:
		// A status with no handling of its own: say what was asked, the query left out.
		hint := fmt.Sprintf("The API answered HTTP %d to %s %s.", resp.StatusCode, method, projectURL(url, true))
		respBody, _ := limitedReadAll(resp.Body, MaxErrorBodyBytes)
		var apiErr struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &apiErr) == nil {
			msg := apiErr.Error
			if msg == "" {
				msg = apiErr.Message
			}
			if msg != "" {
				return nil, &Error{Code: CodeAPI, Message: truncateString(msg, MaxErrorMessageBytes), Hint: hint, HTTPStatus: resp.StatusCode, RequestID: requestID}
			}
		}
		return nil, &Error{Code: CodeAPI, Message: fmt.Sprintf("Request failed (HTTP %d)", resp.StatusCode), Hint: hint, HTTPStatus: resp.StatusCode, RequestID: requestID}
	}
}

// ReadOnlyTokenHint is the hint on a write refused as unauthenticated.
const ReadOnlyTokenHint = "The token may be read-only: changes need a token with write permission"

// isRead reports whether method only reads, which a read token may send.
func isRead(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

func (c *Client) buildURL(path string) (string, error) {
	if strings.HasPrefix(path, "https://") {
		return path, nil
	}
	if strings.HasPrefix(path, "http://") {
		if isLocalhost(path) {
			return path, nil
		}
		return "", fmt.Errorf("URL must use HTTPS, got: %s", describeOrigin(path))
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.cfg.BaseURL + path, nil
}

func (c *Client) backoffDelay(attempt int) time.Duration {
	delay := c.httpOpts.BaseDelay * time.Duration(1<<(attempt-1))
	if c.httpOpts.MaxJitter <= 0 {
		return delay
	}
	jitter := time.Duration(rand.Int63n(int64(c.httpOpts.MaxJitter))) // #nosec G404 -- jitter doesn't need cryptographic randomness
	return delay + jitter
}

// parseRetryAfter parses the Retry-After header value.
func parseRetryAfter(header string) int {
	const maxRetryDelay = 300 // 5 minutes; anything larger is almost certainly bogus

	if header == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
		return min(seconds, maxRetryDelay)
	}
	if t, err := http.ParseTime(header); err == nil {
		delay := time.Until(t)
		if delay > maxRetryDelay*time.Second {
			return maxRetryDelay
		}
		if seconds := int(delay.Seconds()); seconds > 0 {
			return seconds
		}
	}
	return 0
}

// Config returns a copy of the client configuration.
func (c *Client) Config() Config {
	return *c.cfg
}

// --- Account-independent services (on Client) ---

// AccessTokens returns the AccessTokensService for access token operations.
func (c *Client) AccessTokens() *AccessTokensService {
	c.accessTokensMu.Lock()
	defer c.accessTokensMu.Unlock()
	if c.accessTokens == nil {
		c.accessTokens = &AccessTokensService{client: c}
	}
	return c.accessTokens
}

// Identity returns the IdentityService for identity operations.
func (c *Client) Identity() *IdentityService {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()
	if c.identity == nil {
		c.identity = &IdentityService{client: c}
	}
	return c.identity
}

// forbiddenMessage reads a 403 body: the reason the app gives for a refusal, as
// {"error": "..."} or {"message": "..."}. A 403 with no reason says only that
// access was denied.
func forbiddenMessage(body []byte) string {
	var parsed map[string]json.RawMessage
	if json.Unmarshal(body, &parsed) == nil {
		if reason := errorSentence(parsed); reason != "" {
			return reason
		}
	}
	return "Access denied"
}

// errorSentence is the sentence an error body carries under "error" or "message".
func errorSentence(parsed map[string]json.RawMessage) string {
	for _, key := range []string{"error", "message"} {
		var text string
		if raw, ok := parsed[key]; ok && json.Unmarshal(raw, &text) == nil && text != "" {
			return truncateString(text, MaxErrorMessageBytes)
		}
	}
	return ""
}

// validationMessage reads a 422 body: {"error": "..."} or {"message": "..."}, or
// the app's record errors, a bare object of arrays ({"name": ["can't be blank"]}),
// rendered as "name: can't be blank" joined with "; ". An empty or unreadable body
// says only that validation failed.
func validationMessage(body []byte) string {
	msg := "Validation failed"
	var parsed map[string]json.RawMessage
	if json.Unmarshal(body, &parsed) != nil || len(parsed) == 0 {
		return msg
	}
	if text := errorSentence(parsed); text != "" {
		return text
	}

	fields := make([]string, 0, len(parsed))
	for field := range parsed {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	var parts []string
	for _, field := range fields {
		var messages []string
		if json.Unmarshal(parsed[field], &messages) != nil {
			return msg + ": " + truncateString(strings.TrimSpace(string(body)), MaxErrorMessageBytes)
		}
		for _, m := range messages {
			parts = append(parts, field+": "+m)
		}
	}
	if len(parts) == 0 {
		return msg
	}
	return truncateString(strings.Join(parts, "; "), MaxErrorMessageBytes)
}
