package mailpiece

// Types mirror the Rails app's jbuilder views field for field. Timestamps and
// dates are strings as the API renders them; a field the view can render as
// null is a pointer.

// Identity is GET /my/identity.json (my/identities/show.json.jbuilder).
type Identity struct {
	ID       string            `json:"id"`
	Accounts []IdentityAccount `json:"accounts"`
}

// IdentityAccount is an account the identity reaches (my/identities/_account.json.jbuilder),
// with the identity's user on it.
type IdentityAccount struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	User        User   `json:"user"`
}

// User is users/_user.json.jbuilder.
type User struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Role         string  `json:"role"`
	Active       bool    `json:"active"`
	EmailAddress *string `json:"email_address"`
	CreatedAt    string  `json:"created_at"`
	URL          string  `json:"url"`
}

// AccessToken is what POST /my/access_tokens.json renders, and each row of GET /my/access_tokens.json.
// The token is shown once, at creation; a listed token has none.
type AccessToken struct {
	ID          string `json:"id"`
	Token       string `json:"token,omitempty"`
	Description string `json:"description"`
	Permission  string `json:"permission"`
	CreatedAt   string `json:"created_at"`
}

// CreateAccessTokenRequest is the body of POST /my/access_tokens.json.
type CreateAccessTokenRequest struct {
	Description string `json:"description"`
	// Permission is "read" or "write".
	Permission string `json:"permission"`
}
