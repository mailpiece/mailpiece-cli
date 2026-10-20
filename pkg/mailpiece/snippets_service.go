package mailpiece

import "context"

// SnippetsService handles the newsletter's snippets. Admins only.
type SnippetsService struct{ client *AccountClient }

// NewSnippetsService builds a SnippetsService bound to an account client.
func NewSnippetsService(c *AccountClient) *SnippetsService {
	return &SnippetsService{client: c}
}

// List returns every snippet, by name. The app sends them all, unpaged.
func (s *SnippetsService) List(ctx context.Context) ([]Snippet, *Response, error) {
	return listAt[Snippet](ctx, s.client, "", "/snippets.json")
}

// Get returns a snippet by id.
func (s *SnippetsService) Get(ctx context.Context, snippetID string) (*Snippet, *Response, error) {
	return fetch[Snippet](ctx, s.client, "/snippets/%s.json", snippetID)
}

// Create adds a snippet.
func (s *SnippetsService) Create(ctx context.Context, req *SnippetRequest) (*Snippet, *Response, error) {
	return send[Snippet](ctx, s.client, "POST", req, "/snippets.json")
}

// Update changes a snippet's name or content, sending only what is set. A post it was pasted
// into does not change.
func (s *SnippetsService) Update(ctx context.Context, snippetID string, req *SnippetRequest) (*Snippet, *Response, error) {
	return send[Snippet](ctx, s.client, "PATCH", req, "/snippets/%s.json", snippetID)
}

// Delete deletes a snippet for good.
func (s *SnippetsService) Delete(ctx context.Context, snippetID string) (*Response, error) {
	return sendBare(ctx, s.client, "DELETE", nil, "/snippets/%s.json", snippetID)
}
