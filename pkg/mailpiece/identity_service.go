package mailpiece

import "context"

// IdentityService handles identity operations (account-independent).
type IdentityService struct{ client *Client }

// GetMyIdentity returns the identity the token belongs to and the accounts it reaches.
func (s *IdentityService) GetMyIdentity(ctx context.Context) (*Identity, *Response, error) {
	return decode[Identity](s.client.Get(ctx, "/my/identity.json"))
}
