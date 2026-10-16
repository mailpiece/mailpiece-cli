package mailpiece

// SubscriberStatuses are the states a subscriber is read in, as the API names
// them (subscribers/_subscriber.json.jbuilder): the one that is mailed is
// "confirmed".
var SubscriberStatuses = []string{"confirmed", "unconfirmed", "unsubscribed", "bounced", "deactivated"}

// Subscriber is subscribers/_subscriber.json.jbuilder. Its status is read from
// the four dates: bounced, then deactivated, then unsubscribed, then confirmed.
type Subscriber struct {
	ID             string                `json:"id"`
	Email          string                `json:"email"`
	Status         string                `json:"status"`
	ConfirmedAt    *string               `json:"confirmed_at"`
	UnsubscribedAt *string               `json:"unsubscribed_at"`
	BouncedAt      *string               `json:"bounced_at"`
	DeactivatedAt  *string               `json:"deactivated_at"`
	SourcePost     *SubscriberSourcePost `json:"source_post"`
	Referrer       *string               `json:"referrer"`
	CreatedAt      string                `json:"created_at"`
	URL            string                `json:"url"`
}

// SubscriberDetail is subscribers/show.json.jbuilder: the subscriber and one
// page of the issues mailed to it, newest first.
type SubscriberDetail struct {
	Subscriber
	Deliveries []SubscriberDelivery `json:"deliveries"`
}

// SubscriberDelivery is subscribers/_delivery.json.jbuilder: one post as it
// reached, or did not reach, the subscriber. Status is sent, refused,
// unverified or pending.
type SubscriberDelivery struct {
	ID        string                 `json:"id"`
	Post      SubscriberDeliveryPost `json:"post"`
	Status    string                 `json:"status"`
	SentAt    *string                `json:"sent_at"`
	RefusedAt *string                `json:"refused_at"`
	CreatedAt string                 `json:"created_at"`
}

// SubscriberDeliveryPost names the post a delivery is of.
type SubscriberDeliveryPost struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// SubscriberSourcePost names the post a subscriber signed up from.
type SubscriberSourcePost struct {
	ID    string  `json:"id"`
	Code  string  `json:"code"`
	Title *string `json:"title"`
	URL   string  `json:"url"`
}

// CreateSubscriberRequest is the body of POST /subscribers.json.
type CreateSubscriberRequest struct {
	Email string `json:"email"`
}
