package mailpiece

import "context"

// UsersService handles the signed-in person's own user on the account. A user reaches only their
// own profile: anyone else's is a 404.
type UsersService struct{ client *AccountClient }

// NewUsersService builds a UsersService bound to an account client.
func NewUsersService(c *AccountClient) *UsersService {
	return &UsersService{client: c}
}

// Get returns a user by id.
func (s *UsersService) Get(ctx context.Context, userID string) (*User, *Response, error) {
	return fetch[User](ctx, s.client, "/users/%s.json", userID)
}

// Update renames a user.
func (s *UsersService) Update(ctx context.Context, userID string, req *UserRequest) (*User, *Response, error) {
	return send[User](ctx, s.client, "PATCH", req, "/users/%s.json", userID)
}
