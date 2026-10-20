package mailpiece

// PostVisibilities are the values a post's visibility takes (Post::Publishable::VISIBILITIES):
// listed in the archive and the feed, reached by its link only, or with no page at all.
var PostVisibilities = []string{"listed", "unlisted", "email_only"}

// PostDestinations name where a draft goes when it is released (Post::Destination): a
// visibility plus whether the list is mailed.
var PostDestinations = []string{"web_and_email", "email", "web", "link"}

// Post is posts/_post.json.jbuilder. Body is the text as plain text and BodyHTML as it
// was written, audience boxes included. PublicURL is there only while the post has a page,
// and the three counts after DeliveriesCount only on a single post (posts/show.json.jbuilder).
type Post struct {
	ID                      string  `json:"id"`
	Title                   *string `json:"title"`
	Summary                 *string `json:"summary"`
	Slug                    string  `json:"slug"`
	DeliveriesCount         int     `json:"deliveries_count"`
	BroughtSubscribersCount *int    `json:"brought_subscribers_count,omitempty"`
	LikedCount              *int    `json:"liked_count,omitempty"`
	DislikedCount           *int    `json:"disliked_count,omitempty"`
	Body                    string  `json:"body"`
	BodyHTML                string  `json:"body_html"`
	Published               bool    `json:"published"`
	Visibility              string  `json:"visibility"`
	PublishedAt             *string `json:"published_at"`
	SentAt                  *string `json:"sent_at"`
	ScheduledPostingAt      *string `json:"scheduled_posting_at"`
	ScheduledDelivery       bool    `json:"scheduled_delivery"`
	CreatedAt               string  `json:"created_at"`
	UpdatedAt               string  `json:"updated_at"`
	URL                     string  `json:"url"`
	PublicURL               string  `json:"public_url,omitempty"`
	Creator                 User    `json:"creator"`
}

// CreatePostRequest is the body of POST /posts.json. Body is HTML.
type CreatePostRequest struct {
	Title      string `json:"title,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Body       string `json:"body,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

// UpdatePostRequest is the body of PATCH /posts/:id.json; only what is set is sent. Body is
// HTML. PublishedAt is taken only for a published post that is on the web, and is dropped
// without a word for any other.
type UpdatePostRequest struct {
	Title       *string `json:"title,omitempty"`
	Summary     *string `json:"summary,omitempty"`
	Body        *string `json:"body,omitempty"`
	Visibility  *string `json:"visibility,omitempty"`
	PublishedAt *string `json:"published_at,omitempty"`
}

// PublishPostRequest is the body of POST /posts/:id/publication.json. A publication mails
// nobody: Destination only picks the visibility. PublishedAt, or Day (YYYY-MM-DD) with Hour
// (0 to 23), releases a draft under a past date.
type PublishPostRequest struct {
	Destination string `json:"destination,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
	Day         string `json:"day,omitempty"`
	Hour        *int   `json:"hour,omitempty"`
}

// SchedulePostRequest is the body of POST /posts/:id/schedule.json. The time is
// ScheduledPostingAt, or Day (YYYY-MM-DD) with Hour (0 to 23). A named Destination decides
// whether the list is mailed; without one the post keeps its visibility and
// ScheduledDelivery decides.
type SchedulePostRequest struct {
	Destination        string `json:"destination,omitempty"`
	ScheduledPostingAt string `json:"scheduled_posting_at,omitempty"`
	Day                string `json:"day,omitempty"`
	Hour               *int   `json:"hour,omitempty"`
	ScheduledDelivery  bool   `json:"scheduled_delivery,omitempty"`
}

// TestPostDeliveryRequest is the body of POST /posts/:id/test_delivery.json. With no
// address the test goes to the signed-in person's own.
type TestPostDeliveryRequest struct {
	EmailAddress string `json:"email_address,omitempty"`
}

// PostLink is posts/links/_link.json.jbuilder: a link of an issue already mailed. URL is
// where it sends a reader now and OriginalURL what the mailed text has. Stale says the
// post's text no longer has the link and it was never corrected.
type PostLink struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Text        *string `json:"text"`
	OriginalURL string  `json:"original_url"`
	URL         string  `json:"url"`
	Stale       bool    `json:"stale"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// UpdatePostLinkRequest is the body of PATCH /posts/:id/links/:id.json.
type UpdatePostLinkRequest struct {
	URL string `json:"url"`
}

// PostRevision is posts/revisions/_revision.json.jbuilder, and with Title, Body and
// BodyHTML posts/revisions/show.json.jbuilder: the post as it stood before an edit.
type PostRevision struct {
	ID        string  `json:"id"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	URL       string  `json:"url"`
	Creator   User    `json:"creator"`
	Title     *string `json:"title,omitempty"`
	Body      string  `json:"body,omitempty"`
	BodyHTML  string  `json:"body_html,omitempty"`
}
