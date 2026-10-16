package mailpiece

// Newsletter is account/newsletters/show.json.jbuilder: the newsletter's name, its description
// (Description the text alone, DescriptionHTML what was written) and the language it is written in.
type Newsletter struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	DescriptionHTML string `json:"description_html"`
	Locale          string `json:"locale"`
	AvatarURL       string `json:"avatar_url,omitempty"`
}

// NewsletterRequest is the body of PATCH /account/newsletter.json, sent flat: the controller wraps
// it under "account". Description is HTML. A nil field is left as it is.
type NewsletterRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	// Locale is "en" or "es".
	Locale *string `json:"locale,omitempty"`
}

// Handle is account/handles/show.json.jbuilder: the word the newsletter answers at beside its
// number, null while it has none.
type Handle struct {
	Handle *string `json:"handle"`
}

// HandleRequest is the body of PATCH /account/handle.json. An empty handle clears it.
type HandleRequest struct {
	Handle string `json:"handle"`
}

// Sender is account/senders/show.json.jbuilder: who the newsletter's own mail is from.
type Sender struct {
	FromName       *string `json:"from_name"`
	FromAddress    *string `json:"from_address"`
	ReplyToAddress *string `json:"reply_to_address"`
}

// SenderRequest is the body of PATCH /account/sending.json, sent flat: the controller wraps it
// under "account_sender". A nil field is left as it is.
type SenderRequest struct {
	FromName       *string `json:"from_name,omitempty"`
	FromAddress    *string `json:"from_address,omitempty"`
	ReplyToAddress *string `json:"reply_to_address,omitempty"`
}

// SubscribeForm is account/subscribe_forms/show.json.jbuilder: where the embedded form sends a
// person back to, null when it has nowhere.
type SubscribeForm struct {
	SubscriptionReturnURL *string `json:"subscription_return_url"`
}

// SubscribeFormRequest is the body of PATCH /account/subscribe_form.json. An empty URL clears it.
type SubscribeFormRequest struct {
	SubscriptionReturnURL string `json:"subscription_return_url"`
}

// JoinCode is account/join_codes/show.json.jbuilder: the code a person uses to join the
// newsletter's team, good while it has been used fewer times than its limit.
type JoinCode struct {
	Code       string `json:"code"`
	UsageCount int64  `json:"usage_count"`
	UsageLimit int64  `json:"usage_limit"`
	URL        string `json:"url"`
	Active     bool   `json:"active"`
}

// JoinCodeRequest is the body of PATCH /account/join_code.json.
type JoinCodeRequest struct {
	UsageLimit int64 `json:"usage_limit"`
}

// Notifications is notifications/settings/show.json.jbuilder: whether the signed-in admin is
// emailed when someone confirms a subscription. It is the token's own user's setting.
type Notifications struct {
	NewSubscriberNotifications bool `json:"new_subscriber_notifications"`
}

// NotificationsRequest is the body of PATCH /notifications/settings.json.
type NotificationsRequest struct {
	NewSubscriberNotifications bool `json:"new_subscriber_notifications"`
}
