package commands

import (
	stderrors "errors"
	"strings"
	"testing"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

// APIResponse is what the mock answers a request with. The httptest server
// behind the SDK writes Data as the JSON body, or Body as raw bytes when set.
type APIResponse struct {
	StatusCode int
	Body       []byte
	Location   string
	LinkNext   string
	Data       any
}

// MockClient records the requests the SDK sends to the httptest server that
// SetTestModeWithSDK starts, and answers them.
type MockClient struct {
	// Response to return for each method
	GetResponse               *APIResponse
	PostResponse              *APIResponse
	PatchResponse             *APIResponse
	PutResponse               *APIResponse
	DeleteResponse            *APIResponse
	GetWithPaginationResponse *APIResponse

	// Path-based GET response routing (checked before GetResponse)
	getPathResponses map[string]*APIResponse

	// Errors to return for each method
	GetError               error
	PostError              error
	PatchError             error
	PutError               error
	DeleteError            error
	GetWithPaginationError error

	// Captured calls for verification
	GetCalls               []MockCall
	PostCalls              []MockCall
	PatchCalls             []MockCall
	PutCalls               []MockCall
	DeleteCalls            []MockCall
	GetWithPaginationCalls []MockCall

	// UploadFileCalls holds the file names of every multipart part sent.
	UploadFileCalls []string
}

// MockCall represents a captured API call.
type MockCall struct {
	Path string
	Body any
}

// NewMockClient creates a new mock client with default success responses.
func NewMockClient() *MockClient {
	return &MockClient{
		GetResponse: &APIResponse{
			StatusCode: 200,
			Data:       map[string]any{},
		},
		PostResponse: &APIResponse{
			StatusCode: 201,
			Location:   "/resource/123",
			Data:       map[string]any{"id": "123"},
		},
		PatchResponse: &APIResponse{
			StatusCode: 200,
			Data:       map[string]any{},
		},
		PutResponse: &APIResponse{
			StatusCode: 200,
			Data:       map[string]any{},
		},
		DeleteResponse: &APIResponse{
			StatusCode: 204,
			Data:       map[string]any{},
		},
		GetWithPaginationResponse: &APIResponse{
			StatusCode: 200,
			Data:       []any{},
		},
	}
}

func (m *MockClient) Get(path string) (*APIResponse, error) {
	m.GetCalls = append(m.GetCalls, MockCall{Path: path})
	if m.GetError != nil {
		return nil, m.GetError
	}
	// Check path-based responses first
	if m.getPathResponses != nil {
		if resp, ok := m.getPathResponses[path]; ok {
			return resp, nil
		}
		// Try prefix matching (strip query string)
		basePath := path
		if idx := strings.Index(basePath, "?"); idx >= 0 {
			basePath = basePath[:idx]
		}
		if resp, ok := m.getPathResponses[basePath]; ok {
			return resp, nil
		}
	}
	return m.GetResponse, nil
}

// OnGet sets a response for a specific GET path.
func (m *MockClient) OnGet(path string, resp *APIResponse) *MockClient {
	if m.getPathResponses == nil {
		m.getPathResponses = make(map[string]*APIResponse)
	}
	m.getPathResponses[path] = resp
	return m
}

func (m *MockClient) Post(path string, body any) (*APIResponse, error) {
	m.PostCalls = append(m.PostCalls, MockCall{Path: path, Body: body})
	if m.PostError != nil {
		return nil, m.PostError
	}
	return m.PostResponse, nil
}

func (m *MockClient) Patch(path string, body any) (*APIResponse, error) {
	m.PatchCalls = append(m.PatchCalls, MockCall{Path: path, Body: body})
	if m.PatchError != nil {
		return nil, m.PatchError
	}
	return m.PatchResponse, nil
}

func (m *MockClient) Put(path string, body any) (*APIResponse, error) {
	m.PutCalls = append(m.PutCalls, MockCall{Path: path, Body: body})
	if m.PutError != nil {
		return nil, m.PutError
	}
	return m.PutResponse, nil
}

func (m *MockClient) Delete(path string) (*APIResponse, error) {
	m.DeleteCalls = append(m.DeleteCalls, MockCall{Path: path})
	if m.DeleteError != nil {
		return nil, m.DeleteError
	}
	return m.DeleteResponse, nil
}

// Helper functions for creating common responses

// WithGetData sets the data returned by Get calls.
func (m *MockClient) WithGetData(data any) *MockClient {
	m.GetResponse.Data = data
	return m
}

// WithPostData sets the data returned by Post calls.
func (m *MockClient) WithPostData(data any) *MockClient {
	m.PostResponse.Data = data
	return m
}

// WithPatchData sets the data returned by Patch calls.
func (m *MockClient) WithPatchData(data any) *MockClient {
	m.PatchResponse.Data = data
	return m
}

// WithListData sets the data returned by list GETs.
func (m *MockClient) WithListData(data []any) *MockClient {
	m.GetWithPaginationResponse.Data = data
	return m
}

// WithNotFoundError sets a 404 error for Get calls.
func (m *MockClient) WithNotFoundError() *MockClient {
	m.GetError = errors.NewNotFoundError("Not found")
	return m
}

// WithAuthError sets a 401 error for Get calls.
func (m *MockClient) WithAuthError() *MockClient {
	m.GetError = errors.NewAuthError("Unauthorized")
	return m
}

// WithValidationError sets a 422 error for Post calls.
func (m *MockClient) WithValidationError(message string) *MockClient {
	m.PostError = errors.NewValidationError(message)
	return m
}

// assertExitCode checks that an error has the expected exit code.
// For exit code 0, it asserts that err is nil.
func assertExitCode(t *testing.T, err error, expected int) {
	t.Helper()
	if expected == 0 {
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		return
	}
	if err == nil {
		t.Errorf("expected error with exit code %d, got nil", expected)
		return
	}
	var cliErr *errors.CLIError
	if !stderrors.As(err, &cliErr) {
		t.Errorf("expected CLIError with exit code %d, got non-CLIError: %v", expected, err)
		return
	}
	if cliErr.ExitCode() != expected {
		t.Errorf("expected exit code %d, got %d (error: %v)", expected, cliErr.ExitCode(), err)
	}
}
