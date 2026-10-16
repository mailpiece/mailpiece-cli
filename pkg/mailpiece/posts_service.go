package mailpiece

import (
	"context"
	"fmt"
)

// PostsService handles a newsletter's posts: writing them, releasing them, and the links
// and revisions they keep. Admins only.
type PostsService struct{ client *AccountClient }

// NewPostsService builds a PostsService bound to an account client.
func NewPostsService(c *AccountClient) *PostsService {
	return &PostsService{client: c}
}

// List returns what is ahead: every draft, newest first, then every scheduled post,
// soonest first. The app sends them all, unpaged.
func (s *PostsService) List(ctx context.Context) ([]Post, *Response, error) {
	return listAt[Post](ctx, s.client, "", "/posts.json")
}

// Published returns one page of the posts published and not mailed, newest first.
func (s *PostsService) Published(ctx context.Context, path string) ([]Post, *Response, error) {
	return listAt[Post](ctx, s.client, path, "/posts/published.json")
}

// Sent returns one page of the posts mailed to the list, newest first.
func (s *PostsService) Sent(ctx context.Context, path string) ([]Post, *Response, error) {
	return listAt[Post](ctx, s.client, path, "/posts/sent.json")
}

// Get returns a post by id.
func (s *PostsService) Get(ctx context.Context, postID string) (*Post, *Response, error) {
	return fetch[Post](ctx, s.client, "/posts/%s.json", postID)
}

// Create writes a draft.
func (s *PostsService) Create(ctx context.Context, req *CreatePostRequest) (*Post, *Response, error) {
	if req.Title == "" && req.Summary == "" && req.Body == "" {
		return nil, nil, ErrUsage("A post needs a title, a summary or a body")
	}
	if req.Visibility != "" {
		if err := checkValue("visibility", &req.Visibility, PostVisibilities); err != nil {
			return nil, nil, err
		}
	}
	return send[Post](ctx, s.client, "POST", req, "/posts.json")
}

// Update edits a post, sending only what is set. A published post can be edited: its page
// shows the correction, and the email already sent does not change.
func (s *PostsService) Update(ctx context.Context, postID string, req *UpdatePostRequest) (*Post, *Response, error) {
	if req.Title == nil && req.Summary == nil && req.Body == nil && req.Visibility == nil && req.PublishedAt == nil {
		return nil, nil, nothingToChange()
	}
	if err := checkValue("visibility", req.Visibility, PostVisibilities); err != nil {
		return nil, nil, err
	}
	return send[Post](ctx, s.client, "PATCH", req, "/posts/%s.json", postID)
}

// Delete deletes a post for good, in any state, with its revisions, deliveries and links:
// the links in an issue already mailed stop answering.
func (s *PostsService) Delete(ctx context.Context, postID string) (*Response, error) {
	return sendBare(ctx, s.client, "DELETE", nil, "/posts/%s.json", postID)
}

// Publish gives a post its page and mails nobody. A draft is published now, or under the
// past date the request carries; an issue mailed by email only is given a page. A post
// that already has its page is answered as it is.
func (s *PostsService) Publish(ctx context.Context, postID string, req *PublishPostRequest) (*Post, *Response, error) {
	if req == nil {
		req = &PublishPostRequest{}
	}
	if err := checkDestination(req.Destination); err != nil {
		return nil, nil, err
	}
	if err := checkMoment("published_at", req.PublishedAt, req.Day, req.Hour); err != nil {
		return nil, nil, err
	}
	return send[Post](WithIdempotent(ctx), s.client, "POST", req, "/posts/%s/publication.json", postID)
}

// Unpublish takes a post's page down. A post never mailed is a draft again; an issue
// already mailed stays sent, by email only, and its links stop answering.
func (s *PostsService) Unpublish(ctx context.Context, postID string) (*Response, error) {
	return sendBare(ctx, s.client, "DELETE", nil, "/posts/%s/publication.json", postID)
}

// Schedule sets when a draft is released by itself. It is still a draft until then.
func (s *PostsService) Schedule(ctx context.Context, postID string, req *SchedulePostRequest) (*Post, *Response, error) {
	if req.ScheduledPostingAt == "" && req.Day == "" {
		return nil, nil, ErrUsage("A schedule needs a time: scheduled_posting_at, or a day and an hour")
	}
	if err := checkDestination(req.Destination); err != nil {
		return nil, nil, err
	}
	if err := checkMoment("scheduled_posting_at", req.ScheduledPostingAt, req.Day, req.Hour); err != nil {
		return nil, nil, err
	}
	return send[Post](WithIdempotent(ctx), s.client, "POST", req, "/posts/%s/schedule.json", postID)
}

// Unschedule cancels a draft's schedule.
func (s *PostsService) Unschedule(ctx context.Context, postID string) (*Response, error) {
	return sendBare(ctx, s.client, "DELETE", nil, "/posts/%s/schedule.json", postID)
}

// Deliver mails a post to everyone active on the list, publishing it first if it is a
// draft. It cannot be undone, and a post is mailed once. The response's status is 201 only
// when this request started the mailing. A 200 started nothing: the post was already sent
// (SentAt is set), or nobody is active to mail and it is published and stays unsent.
func (s *PostsService) Deliver(ctx context.Context, postID string) (*Post, *Response, error) {
	return send[Post](ctx, s.client, "POST", nil, "/posts/%s/delivery.json", postID)
}

// DeliverTest mails a post as it stands to one address, the signed-in person's own when
// none is given. Nothing about the post changes.
func (s *PostsService) DeliverTest(ctx context.Context, postID string, req *TestPostDeliveryRequest) (*Response, error) {
	var body any
	if req != nil && req.EmailAddress != "" {
		body = req
	}
	return sendBare(ctx, s.client, "POST", body, "/posts/%s/test_delivery.json", postID)
}

// Relist shows a published post on the web in the archive and the feed (visibility listed).
func (s *PostsService) Relist(ctx context.Context, postID string) (*Response, error) {
	return sendBare(WithIdempotent(ctx), s.client, "POST", nil, "/posts/%s/listing.json", postID)
}

// Unlist keeps a published post's page and takes it out of the archive and the feed
// (visibility unlisted).
func (s *PostsService) Unlist(ctx context.Context, postID string) (*Response, error) {
	return sendBare(ctx, s.client, "DELETE", nil, "/posts/%s/listing.json", postID)
}

// Links returns the links of a post's mailed issue, all of them, unpaged.
func (s *PostsService) Links(ctx context.Context, postID string) ([]PostLink, *Response, error) {
	return listAt[PostLink](ctx, s.client, "", "/posts/%s/links.json", postID)
}

// UpdateLink changes where a mailed link sends a reader from now on.
func (s *PostsService) UpdateLink(ctx context.Context, postID, linkID string, req *UpdatePostLinkRequest) (*PostLink, *Response, error) {
	if req.URL == "" {
		return nil, nil, ErrUsage("A link needs a url")
	}
	return send[PostLink](ctx, s.client, "PATCH", req, "/posts/%s/links/%s.json", postID, linkID)
}

// Revisions returns a post's earlier versions, oldest first, all of them, unpaged.
func (s *PostsService) Revisions(ctx context.Context, postID string) ([]PostRevision, *Response, error) {
	return listAt[PostRevision](ctx, s.client, "", "/posts/%s/history.json", postID)
}

// Revision returns one earlier version of a post with its title and body. The id "latest"
// names the newest.
func (s *PostsService) Revision(ctx context.Context, postID, revisionID string) (*PostRevision, *Response, error) {
	return fetch[PostRevision](ctx, s.client, "/posts/%s/history/%s.json", postID, revisionID)
}

func checkDestination(destination string) error {
	if destination == "" {
		return nil
	}
	return checkValue("destination", &destination, PostDestinations)
}

// checkMoment holds a release's time to one form: the instant, or a day with a whole hour.
func checkMoment(name, at, day string, hour *int) error {
	switch {
	case at != "" && (day != "" || hour != nil):
		return ErrUsage(fmt.Sprintf("Pass %s or a day and an hour, not both", name))
	case day == "" && hour != nil:
		return ErrUsage("An hour needs a day")
	case hour != nil && (*hour < 0 || *hour > 23):
		return ErrUsage("hour must be from 0 to 23")
	}
	return nil
}
