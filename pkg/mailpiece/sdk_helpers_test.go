package mailpiece

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

// sdkRequest is one request an sdkServer received.
type sdkRequest struct {
	Method string
	URI    string
	Header http.Header
	Body   []byte
}

// sdkServer is an httptest server that records every request before handing it on.
type sdkServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []sdkRequest
}

func newSDKServer(t *testing.T, handler http.HandlerFunc) *sdkServer {
	t.Helper()
	s := &sdkServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.requests = append(s.requests, sdkRequest{Method: r.Method, URI: r.URL.RequestURI(), Header: r.Header.Clone(), Body: body})
		s.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *sdkServer) recorded() []sdkRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sdkRequest(nil), s.requests...)
}

// sdkReply answers every request with status and body as JSON.
func sdkReply(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// sdkClient is a client on baseURL whose retries wait a millisecond, not a second.
func sdkClient(baseURL string, opts ...ClientOption) *Client {
	opts = append([]ClientOption{WithBaseDelay(time.Millisecond), WithMaxJitter(time.Millisecond)}, opts...)
	return NewClient(&Config{BaseURL: baseURL}, &StaticTokenProvider{Token: "test-token"}, opts...)
}

func sdkAccount(baseURL string, opts ...ClientOption) *AccountClient {
	return sdkClient(baseURL, opts...).ForAccount("acme")
}

// sdkAssertJSON compares a request body with want as JSON; an empty want means no body.
func sdkAssertJSON(t *testing.T, raw []byte, want string) {
	t.Helper()
	if want == "" {
		if len(raw) != 0 {
			t.Errorf("body = %s, want none", raw)
		}
		return
	}
	var got, expected any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Errorf("body %q is not JSON: %v", raw, err)
		return
	}
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatalf("want %q is not JSON: %v", want, err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("body = %s, want %s", raw, want)
	}
}

// sdkCase is one service call and the one request it must send.
type sdkCase struct {
	name   string
	call   func(context.Context, *AccountClient) error
	method string
	uri    string
	body   string
	reply  string
}

// sdkRunCases runs each case against its own server, answering reply (an empty
// object by default), and checks the request's method, path with query, and body.
func sdkRunCases(t *testing.T, cases []sdkCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reply := tc.reply
			if reply == "" {
				reply = "{}"
			}
			srv := newSDKServer(t, sdkReply(http.StatusOK, reply))
			if err := tc.call(context.Background(), sdkAccount(srv.URL)); err != nil {
				t.Fatalf("call: %v", err)
			}
			reqs := srv.recorded()
			if len(reqs) != 1 {
				t.Fatalf("sent %d requests, want 1", len(reqs))
			}
			if reqs[0].Method != tc.method {
				t.Errorf("method = %s, want %s", reqs[0].Method, tc.method)
			}
			if reqs[0].URI != tc.uri {
				t.Errorf("uri = %s, want %s", reqs[0].URI, tc.uri)
			}
			sdkAssertJSON(t, reqs[0].Body, tc.body)
		})
	}
}

func sdkString(s string) *string { return &s }

func sdkInt(n int64) *int64 { return &n }
