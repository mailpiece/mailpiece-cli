package mailpiece

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// JourneyTriggers are the journeys a newsletter has (Journey::TRIGGERS): one per trigger.
var JourneyTriggers = []string{"subscriber_confirmed", "subscriber_unsubscribed"}

// journeyTriggerWithoutDelay is the journey of one email mailed at once, which takes no delay.
const journeyTriggerWithoutDelay = "subscriber_unsubscribed"

// JourneysService handles the newsletter's journeys and their emails. Admins only.
type JourneysService struct{ client *AccountClient }

// NewJourneysService builds a JourneysService bound to an account client.
func NewJourneysService(c *AccountClient) *JourneysService {
	return &JourneysService{client: c}
}

// List returns every journey with its emails. The app sends them all, unpaged.
func (s *JourneysService) List(ctx context.Context) ([]Journey, *Response, error) {
	return listAt[Journey](ctx, s.client, "", "/journeys.json")
}

// Get returns a journey by its trigger, with its emails.
func (s *JourneysService) Get(ctx context.Context, trigger string) (*Journey, *Response, error) {
	if err := checkJourneyTrigger(trigger); err != nil {
		return nil, nil, err
	}
	return fetch[Journey](ctx, s.client, "/journeys/%s.json", trigger)
}

// GetEmail returns one email of a journey by id.
func (s *JourneysService) GetEmail(ctx context.Context, trigger, emailID string) (*JourneyEmail, *Response, error) {
	if err := checkJourneyTrigger(trigger); err != nil {
		return nil, nil, err
	}
	return fetch[JourneyEmail](ctx, s.client, "/journeys/%s/emails/%s.json", trigger, emailID)
}

// CreateEmail adds an email to a journey, as a draft: it is mailed to nobody until activated.
// The unsubscribe journey holds one email and answers 403 to a second.
func (s *JourneysService) CreateEmail(ctx context.Context, trigger string, req *JourneyEmailRequest) (*JourneyEmail, *Response, error) {
	if err := checkJourneyEmail(trigger, req); err != nil {
		return nil, nil, err
	}
	return send[JourneyEmail](ctx, s.client, "POST", req, "/journeys/%s/emails.json", trigger)
}

// UpdateEmail changes an email, sending only what is set. An active email cannot lose its
// subject or body.
func (s *JourneysService) UpdateEmail(ctx context.Context, trigger, emailID string, req *JourneyEmailRequest) (*JourneyEmail, *Response, error) {
	if err := checkJourneyEmail(trigger, req); err != nil {
		return nil, nil, err
	}
	return send[JourneyEmail](ctx, s.client, "PATCH", req, "/journeys/%s/emails/%s.json", trigger, emailID)
}

// DeleteEmail deletes an email for good.
func (s *JourneysService) DeleteEmail(ctx context.Context, trigger, emailID string) (*Response, error) {
	if err := checkJourneyTrigger(trigger); err != nil {
		return nil, err
	}
	return sendBare(ctx, s.client, "DELETE", nil, "/journeys/%s/emails/%s.json", trigger, emailID)
}

// ActivateEmail switches an email on: from then on it is mailed to the people its journey
// reaches. An email with no subject or no body is refused with 422.
func (s *JourneysService) ActivateEmail(ctx context.Context, trigger, emailID string) (*JourneyEmail, *Response, error) {
	if err := checkJourneyTrigger(trigger); err != nil {
		return nil, nil, err
	}
	return send[JourneyEmail](WithIdempotent(ctx), s.client, "POST", nil, "/journeys/%s/emails/%s/activation.json", trigger, emailID)
}

// DeactivateEmail switches an email off: it is a draft again and is mailed to nobody.
func (s *JourneysService) DeactivateEmail(ctx context.Context, trigger, emailID string) (*JourneyEmail, *Response, error) {
	if err := checkJourneyTrigger(trigger); err != nil {
		return nil, nil, err
	}
	return send[JourneyEmail](ctx, s.client, "DELETE", nil, "/journeys/%s/emails/%s/activation.json", trigger, emailID)
}

// TestEmail mails one copy of an email to an address, or to the signed-in person's own. The
// endpoint answers no JSON body.
func (s *JourneysService) TestEmail(ctx context.Context, trigger, emailID string, req *JourneyEmailTestRequest) (*Response, error) {
	if err := checkJourneyTrigger(trigger); err != nil {
		return nil, err
	}
	var body any
	if req != nil && req.EmailAddress != "" {
		body = req
	}
	return sendBare(ctx, s.client, "POST", body, "/journeys/%s/emails/%s/test_delivery.json", trigger, emailID)
}

// checkJourneyTrigger refuses a trigger the app answers 404 to.
func checkJourneyTrigger(trigger string) error {
	if !slices.Contains(JourneyTriggers, trigger) {
		return ErrUsage(fmt.Sprintf("%q is not a journey: %s", trigger, strings.Join(JourneyTriggers, ", ")))
	}
	return nil
}

// checkJourneyEmail refuses a request that names no field, which the app answers as a missing
// parameter, and a delay on the journey that takes none, which the app would drop without a word.
func checkJourneyEmail(trigger string, req *JourneyEmailRequest) error {
	if err := checkJourneyTrigger(trigger); err != nil {
		return err
	}
	if req == nil || (req.Subject == nil && req.Summary == nil && req.Body == nil && req.DelayDays == nil && req.SendHour == nil) {
		return nothingToChange()
	}
	if trigger == journeyTriggerWithoutDelay && (req.DelayDays != nil || req.SendHour != nil) {
		return ErrUsage("The " + trigger + " journey mails its one email at once: it takes no delay_days or send_hour")
	}
	return nil
}
