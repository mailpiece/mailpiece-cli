package mailpiece

// Journey is journeys/_journey.json.jbuilder: the emails mailed, in order, to a person after
// what its trigger names. It is known by its trigger and carries its emails in full.
type Journey struct {
	Trigger string         `json:"trigger"`
	Emails  []JourneyEmail `json:"emails"`
}

// JourneyEmail is journeys/emails/_email.json.jbuilder. Body is the text alone, BodyHTML what
// was written.
type JourneyEmail struct {
	ID        string  `json:"id"`
	Subject   *string `json:"subject"`
	Summary   *string `json:"summary"`
	Body      string  `json:"body"`
	BodyHTML  string  `json:"body_html"`
	DelayDays int64   `json:"delay_days"`
	SendHour  *int64  `json:"send_hour"`
	TimeZone  *string `json:"time_zone"`
	Active    bool    `json:"active"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	URL       string  `json:"url"`
}

// JourneyEmailRequest is the body of POST /journeys/:trigger/emails.json and PATCH
// /journeys/:trigger/emails/:id.json, sent flat: the controller wraps it under "journey_email".
// Body is HTML. A nil field is left as it is. DelayDays and SendHour are taken only by a journey
// whose emails wait (subscriber_confirmed).
type JourneyEmailRequest struct {
	Subject   *string `json:"subject,omitempty"`
	Summary   *string `json:"summary,omitempty"`
	Body      *string `json:"body,omitempty"`
	DelayDays *int64  `json:"delay_days,omitempty"`
	SendHour  *int64  `json:"send_hour,omitempty"`
}

// JourneyEmailTestRequest is the body of POST /journeys/:trigger/emails/:id/test_delivery.json.
// With no address the test goes to the signed-in person's own.
type JourneyEmailTestRequest struct {
	EmailAddress string `json:"email_address,omitempty"`
}
