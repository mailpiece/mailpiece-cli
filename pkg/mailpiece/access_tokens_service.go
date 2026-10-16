package mailpiece

import (
	"context"
	"net/url"
)

// AccessTokensService handles access token operations (account-independent).
type AccessTokensService struct{ client *Client }

// List returns the identity's access tokens, newest first. The app sends them all, unpaged.
func (s *AccessTokensService) List(ctx context.Context) ([]AccessToken, *Response, error) {
	return decodeList[AccessToken](s.client.Get(ctx, "/my/access_tokens.json"))
}

// Create creates an access token. The token is in the response once and never again.
func (s *AccessTokensService) Create(ctx context.Context, req *CreateAccessTokenRequest) (*AccessToken, *Response, error) {
	if err := checkValue("permission", &req.Permission, []string{"read", "write"}); err != nil {
		return nil, nil, err
	}
	return decode[AccessToken](s.client.Post(ctx, "/my/access_tokens.json", req))
}

// Delete deletes an access token.
func (s *AccessTokensService) Delete(ctx context.Context, accessTokenID string) (*Response, error) {
	return s.client.Delete(ctx, "/my/access_tokens/"+url.PathEscape(accessTokenID)+".json")
}
