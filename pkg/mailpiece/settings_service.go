package mailpiece

import "context"

// NewsletterLocales are the languages a newsletter is written in (Account::Localizable::LOCALES).
var NewsletterLocales = []string{"en", "es"}

// SettingsService handles the newsletter's own configuration, each part a resource of its own
// under /account, and the signed-in admin's own notifications. Admins only.
type SettingsService struct{ client *AccountClient }

// NewSettingsService builds a SettingsService bound to an account client.
func NewSettingsService(c *AccountClient) *SettingsService {
	return &SettingsService{client: c}
}

// Newsletter returns the newsletter's name, description and language.
func (s *SettingsService) Newsletter(ctx context.Context) (*Newsletter, *Response, error) {
	return fetch[Newsletter](ctx, s.client, "/account/newsletter.json")
}

// UpdateNewsletter changes the newsletter's name, description or language, sending only what is set.
func (s *SettingsService) UpdateNewsletter(ctx context.Context, req *NewsletterRequest) (*Newsletter, *Response, error) {
	return send[Newsletter](ctx, s.client, "PATCH", req, "/account/newsletter.json")
}

// Handle returns the newsletter's handle.
func (s *SettingsService) Handle(ctx context.Context) (*Handle, *Response, error) {
	return fetch[Handle](ctx, s.client, "/account/handle.json")
}

// UpdateHandle changes the newsletter's handle, and an empty one clears it. The old name is free
// for another newsletter at once, and a link already mailed with it stops working.
func (s *SettingsService) UpdateHandle(ctx context.Context, req *HandleRequest) (*Handle, *Response, error) {
	return send[Handle](ctx, s.client, "PATCH", req, "/account/handle.json")
}

// Sender returns who the newsletter's own mail is from.
func (s *SettingsService) Sender(ctx context.Context) (*Sender, *Response, error) {
	return fetch[Sender](ctx, s.client, "/account/sending.json")
}

// UpdateSender changes who the newsletter's own mail is from, sending only what is set.
func (s *SettingsService) UpdateSender(ctx context.Context, req *SenderRequest) (*Sender, *Response, error) {
	return send[Sender](ctx, s.client, "PATCH", req, "/account/sending.json")
}

// UpdateRelay sets the relay the newsletter's mail leaves through, replacing an SMTP server if one
// was set, and answers the sending settings. On a relay already set only the URL changes and the
// token is refused.
func (s *SettingsService) UpdateRelay(ctx context.Context, req *RelayRequest) (*Sender, *Response, error) {
	return send[Sender](ctx, s.client, "PATCH", req, "/account/sending/relay.json")
}

// RemoveRelay removes the relay and its token. The endpoint answers no JSON body.
func (s *SettingsService) RemoveRelay(ctx context.Context) (*Response, error) {
	return sendBare(ctx, s.client, "DELETE", nil, "/account/sending/relay.json")
}

// UpdateSMTP sets the SMTP server the newsletter's mail leaves through, replacing a relay if one was
// set, and answers the sending settings. On a server already set the password is refused.
func (s *SettingsService) UpdateSMTP(ctx context.Context, req *SMTPRequest) (*Sender, *Response, error) {
	return send[Sender](ctx, s.client, "PATCH", req, "/account/sending/smtp.json")
}

// RemoveSMTP removes the SMTP server and its password. The endpoint answers no JSON body.
func (s *SettingsService) RemoveSMTP(ctx context.Context) (*Response, error) {
	return sendBare(ctx, s.client, "DELETE", nil, "/account/sending/smtp.json")
}

// SubscribeForm returns where the embedded subscription form sends a person back to.
func (s *SettingsService) SubscribeForm(ctx context.Context) (*SubscribeForm, *Response, error) {
	return fetch[SubscribeForm](ctx, s.client, "/account/subscribe_form.json")
}

// UpdateSubscribeForm sets where the embedded subscription form sends a person back to.
func (s *SettingsService) UpdateSubscribeForm(ctx context.Context, req *SubscribeFormRequest) (*SubscribeForm, *Response, error) {
	return send[SubscribeForm](ctx, s.client, "PATCH", req, "/account/subscribe_form.json")
}

// JoinCode returns the newsletter's current join code.
func (s *SettingsService) JoinCode(ctx context.Context) (*JoinCode, *Response, error) {
	return fetch[JoinCode](ctx, s.client, "/account/join_code.json")
}

// UpdateJoinCode changes the join code's usage limit. The endpoint answers no JSON body.
func (s *SettingsService) UpdateJoinCode(ctx context.Context, req *JoinCodeRequest) (*Response, error) {
	return sendBare(ctx, s.client, "PATCH", req, "/account/join_code.json")
}

// ResetJoinCode retires the current join code and issues a new one, used no times. The endpoint
// answers no JSON body.
func (s *SettingsService) ResetJoinCode(ctx context.Context) (*Response, error) {
	return sendBare(ctx, s.client, "DELETE", nil, "/account/join_code.json")
}

// Notifications returns whether the signed-in admin is emailed when someone confirms a subscription.
func (s *SettingsService) Notifications(ctx context.Context) (*Notifications, *Response, error) {
	return fetch[Notifications](ctx, s.client, "/notifications/settings.json")
}

// UpdateNotifications sets whether the signed-in admin is emailed when someone confirms a
// subscription. The endpoint answers no JSON body.
func (s *SettingsService) UpdateNotifications(ctx context.Context, req *NotificationsRequest) (*Response, error) {
	return sendBare(ctx, s.client, "PATCH", req, "/notifications/settings.json")
}
