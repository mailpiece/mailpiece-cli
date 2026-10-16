package mailpiece

// Snippet is snippets/_snippet.json.jbuilder: a name and rich text a newsletter keeps, pasted into
// a post from the editor's menu. Content is the text alone, ContentHTML what was written.
type Snippet struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Content     string `json:"content"`
	ContentHTML string `json:"content_html"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	URL         string `json:"url"`
}

// SnippetRequest is the body of POST /snippets.json and PATCH /snippets/:id.json, sent flat: the
// controller wraps it under "snippet". Content is HTML. A nil field is left as it is.
type SnippetRequest struct {
	Name    *string `json:"name,omitempty"`
	Content *string `json:"content,omitempty"`
}
