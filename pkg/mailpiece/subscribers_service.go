package mailpiece

import (
	"context"
	"strconv"
)

// SubscribersService handles a newsletter's subscribers. Admins only.
type SubscribersService struct{ client *AccountClient }

// NewSubscribersService builds a SubscribersService bound to an account client.
func NewSubscribersService(c *AccountClient) *SubscribersService {
	return &SubscribersService{client: c}
}

// SubscribersPath is the list of the subscribers in a state: the ones mailed
// ("confirmed", or "") at /subscribers.json, every other state on a list of
// its own.
func SubscribersPath(status string) (string, error) {
	if status == "" || status == "confirmed" {
		return "/subscribers.json", nil
	}
	if err := checkValue("status", &status, SubscriberStatuses); err != nil {
		return "", err
	}
	return buildPath("/subscribers/%s.json", segment(status))
}

// List returns one page of the subscribers in a state, "" being the ones mailed.
func (s *SubscribersService) List(ctx context.Context, status, path string) ([]Subscriber, *Response, error) {
	if path == "" {
		var err error
		if path, err = SubscribersPath(status); err != nil {
			return nil, nil, err
		}
	}
	return decodeList[Subscriber](s.client.Get(ctx, path))
}

// Get returns a subscriber by id with one page of its deliveries, newest
// first; page 0 is the first.
func (s *SubscribersService) Get(ctx context.Context, subscriberID string, page int) (*SubscriberDetail, *Response, error) {
	path, err := buildPath("/subscribers/%s.json", subscriberID)
	if err != nil {
		return nil, nil, err
	}
	if page > 0 {
		path += "?page=" + strconv.Itoa(page)
	}
	return decode[SubscriberDetail](s.client.Get(ctx, path))
}

// Create adds one address to the list, already confirmed: no confirmation is
// mailed and no journey starts. An address already on the list is answered as
// it is, and one that never confirmed is confirmed. One that is not an address,
// or that unsubscribed, bounced or was stopped by an admin, is refused with a
// 422 whose message says which.
func (s *SubscribersService) Create(ctx context.Context, req *CreateSubscriberRequest) (*Subscriber, *Response, error) {
	if req.Email == "" {
		return nil, nil, ErrUsage("A subscriber needs an email address")
	}
	return send[Subscriber](ctx, s.client, "POST", req, "/subscribers.json")
}

// Delete removes a subscriber for good, with its deliveries and journey history.
func (s *SubscribersService) Delete(ctx context.Context, subscriberID string) (*Response, error) {
	return sendBare(ctx, s.client, "DELETE", nil, "/subscribers/%s.json", subscriberID)
}

// Deactivate stops mailing a subscriber. It answers 204 with no body when
// stopped or already stopped, and refuses, with a 403 that says why, one that
// unsubscribed or bounced.
func (s *SubscribersService) Deactivate(ctx context.Context, subscriberID string) (*Response, error) {
	return sendBare(ctx, s.client, "DELETE", nil, "/subscribers/%s/activation.json", subscriberID)
}

// Activate resumes a subscriber an admin stopped. It undoes neither an
// unsubscribe nor a bounce. The response's status is 201 only when it resumed
// the person, and 200 when it changed nothing.
func (s *SubscribersService) Activate(ctx context.Context, subscriberID string) (*Subscriber, *Response, error) {
	return send[Subscriber](WithIdempotent(ctx), s.client, "POST", nil, "/subscribers/%s/activation.json", subscriberID)
}

// Bounce marks a subscriber bounced, so it is no longer mailed. Only the person
// undoes it, by subscribing again. One already bounced is answered the same,
// 204 either way.
func (s *SubscribersService) Bounce(ctx context.Context, subscriberID string) (*Response, error) {
	return sendBare(WithIdempotent(ctx), s.client, "POST", nil, "/subscribers/%s/bounce.json", subscriberID)
}

// BounceAddress marks bounced the subscriber with an address, as Bounce does.
// It answers 204 when marked or already bounced, and 404 when no subscriber
// has the address.
func (s *SubscribersService) BounceAddress(ctx context.Context, req *BounceAddressRequest) (*Response, error) {
	if req.EmailAddress == "" {
		return nil, ErrUsage("A bounce needs an email address")
	}
	return sendBare(WithIdempotent(ctx), s.client, "POST", req, "/subscribers/bounces.json")
}
