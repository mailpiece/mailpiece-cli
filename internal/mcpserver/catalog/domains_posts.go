package catalog

// The posts domains: a newsletter's posts, their release, links and revisions, and
// the search. Each operation mirrors one JSON endpoint of the Mailpiece app;
// operations within a domain are sorted by action name.

const (
	postBodyDoc = "The post's text as HTML (Markdown is not converted here). A box for one audience is <aside data-audience=\"web\">...</aside> or <aside data-audience=\"email\">...</aside>: send it back exactly as body_html has it."

	postDestinationDoc = "web_and_email (listed, mailed), email (no page, mailed), web (listed) or link (unlisted: reached by its link only)"

	postDayDoc  = "Day, YYYY-MM-DD, read with hour in the app's time zone"
	postHourDoc = "Whole hour of day, 0 to 23 (0 when left out)"
)

func postIDParam() Param {
	return pathParam("post_id", "The post's ID")
}

func init() {
	Register(&Domain{
		Key:   "posts",
		Tool:  "mailpiece_posts",
		Title: "Mailpiece Posts",
		Blurb: "A newsletter's posts: drafts, their release to the web and to the list of subscribers, and the links and earlier versions a post keeps. Admins only. A post is published once and sent once; a scheduled post is still a draft. Mailing the list cannot be undone: never deliver, or schedule a delivery, unless the person asked for exactly that.",
		Operations: []*Operation{
			{
				Action: "create_post", Method: "POST", Path: "/posts", BodyKey: "post",
				Summary: "Write a draft. Nothing is published or mailed.",
				Doc:     "Send at least one of title, summary and body.",
				Body: object(map[string]any{
					"title":      str("Title, which is also the email's subject"),
					"summary":    str("The writer's own line about the post, shown in place of how it begins"),
					"body":       str(postBodyDoc),
					"visibility": strEnum("listed (the default), unlisted (reached by its link only) or email_only (no page)", "listed", "unlisted", "email_only"),
				}),
			},
			{
				Action: "delete_post", Method: "DELETE", Path: "/posts/{post_id}",
				Destructive: true, Idempotent: true,
				Summary: "Delete a post in any state, with its revisions, deliveries and links: the links in an issue already mailed stop answering. " + cannotBeUndone,
				Params:  []Param{postIDParam()},
			},
			{
				Action: "deliver_post", Method: "POST", Path: "/posts/{post_id}/delivery",
				Destructive: true, Idempotent: true,
				Summary: "Mail a post now to every active subscriber, publishing it first if it is a draft. " + cannotBeUndone,
				Doc:     "The app answers 201 only when this call started the mailing, and 200 when it started nothing: a post is sent once, so one already sent is answered as it is and mailed to nobody again, and with nobody active on the list the post is published and stays unsent (sent_at is null). The result here is the post either way, without the status: read sent_at with get_post first to tell a new mailing from a post already sent. 422 while the newsletter has no sender.",
				Params:  []Param{postIDParam()},
			},
			{
				Action: "get_post", Method: "GET", Path: "/posts/{post_id}", ReadOnly: true,
				Summary: "Get a post in any state",
				Doc:     "body is the text as plain text; body_html is the text as it was written, audience boxes included. public_url is there only while the post has a page.",
				Params:  []Param{postIDParam()},
			},
			{
				Action: "get_post_revision", Method: "GET", Path: "/posts/{post_id}/history/{revision_id}", ReadOnly: true,
				Summary: "Get one earlier version of a post, with its title and text",
				Params: []Param{
					postIDParam(),
					pathParam("revision_id", "The revision's ID, or latest for the newest"),
				},
			},
			{
				Action: "list_post_links", Method: "GET", Path: "/posts/{post_id}/links", ReadOnly: true,
				Summary: "List the links a post's issue was mailed with",
				Doc:     "original_url is what the email has and url where the link sends a reader now. stale says the post's text no longer has the link and it was never corrected. A post never mailed has none.",
				Params:  []Param{postIDParam()},
			},
			{
				Action: "list_post_revisions", Method: "GET", Path: "/posts/{post_id}/history", ReadOnly: true,
				Summary: "List a post's earlier versions, oldest first, without their text",
				Params:  []Param{postIDParam()},
			},
			{
				Action: "list_posts", Method: "GET", Path: "/posts", ReadOnly: true,
				Summary: "List what is ahead: the drafts, newest first, then the scheduled posts, soonest first",
				Doc:     "All of them on one page. Published and sent posts have lists of their own.",
			},
			{
				Action: "list_published_posts", Method: "GET", Path: "/posts/published", ReadOnly: true, Paginated: true,
				Summary: "List the posts published and not mailed, newest first",
				Params:  []Param{pageParam()},
			},
			{
				Action: "list_sent_posts", Method: "GET", Path: "/posts/sent", ReadOnly: true, Paginated: true,
				Summary: "List the posts mailed to the list, newest first",
				Params:  []Param{pageParam()},
			},
			{
				Action: "publish_post", Method: "POST", Path: "/posts/{post_id}/publication",
				Idempotent: true,
				Summary:    "Give a post its page, mailing nobody",
				Doc:        "A draft is published now with its own visibility, or the one destination names; with published_at, or day and hour, it is released under that past date instead. An issue mailed by email only is given a page, listed. A post that already has its page is answered as it is. 403 for an email-only draft, which is never published without being sent.",
				Params:     []Param{postIDParam()},
				Body: object(map[string]any{
					"destination":  strEnum("Picks the visibility only, since a publication mails nobody: web (listed) or link (unlisted). Default: the post's own visibility", "web_and_email", "email", "web", "link"),
					"published_at": str("Release a draft under this past date, ISO 8601"),
					"day":          str(postDayDoc + ", to release a draft under a past date"),
					"hour":         integer(postHourDoc),
				}),
			},
			{
				Action: "relist_post", Method: "POST", Path: "/posts/{post_id}/listing",
				Idempotent: true,
				Summary:    "Show a published post that is on the web in the archive and the feed (visibility listed)",
				Doc:        "403 for a draft or an email-only post.",
				Params:     []Param{postIDParam()},
			},
			{
				Action: "schedule_post", Method: "POST", Path: "/posts/{post_id}/schedule",
				Idempotent: true,
				Summary:    "Set when a draft is released by itself; scheduling again moves the time. A scheduled delivery mails every active subscriber when it runs, and that mail cannot be taken back; unschedule_post stops it before then.",
				Doc:        "Give the time as scheduled_posting_at, or as day and hour; it must be still to come. Without destination the post keeps its visibility and scheduled_delivery decides whether the list is mailed. A named destination decides both by itself, and the two that mail are offered only while the newsletter has a sender and someone active on its list. 403 for a post that is not a draft or a destination not offered; 422 for a time not ahead or an email-only post that would not be mailed.",
				Params:     []Param{postIDParam()},
				Body: object(map[string]any{
					"destination":          strEnum(postDestinationDoc, "web_and_email", "email", "web", "link"),
					"scheduled_posting_at": str("When to release it, ISO 8601"),
					"day":                  str(postDayDoc),
					"hour":                 integer(postHourDoc),
					"scheduled_delivery":   boolean("Also mail the whole list then. Read only without destination"),
				}),
			},
			{
				Action: "test_post", Method: "POST", Path: "/posts/{post_id}/test_delivery",
				Destructive: true,
				Summary:     "Mail a test of a post as it stands to one address. The list is not mailed and the post does not change, but the test is real mail. " + cannotBeUndone,
				Doc:         "422 for an address that is not one or while the newsletter has no sender. At most 10 tests in 3 minutes.",
				Params:      []Param{postIDParam()},
				Body: object(map[string]any{
					"email_address": str("Where the test goes (default: the signed-in person's own address)"),
				}),
			},
			{
				Action: "unlist_post", Method: "DELETE", Path: "/posts/{post_id}/listing",
				Idempotent: true,
				Summary:    "Keep a published post's page and take it out of the archive and the feed (visibility unlisted)",
				Doc:        "403 for a draft or an email-only post.",
				Params:     []Param{postIDParam()},
			},
			{
				Action: "unpublish_post", Method: "DELETE", Path: "/posts/{post_id}/publication",
				Idempotent: true,
				Summary:    "Take a post's page down",
				Doc:        "A post never mailed is a draft again. An issue already mailed is never a draft and never mailed twice: it stays sent, by email only, and its links stop answering until publish_post gives it a page again.",
				Params:     []Param{postIDParam()},
			},
			{
				Action: "unschedule_post", Method: "DELETE", Path: "/posts/{post_id}/schedule",
				Idempotent: true,
				Summary:    "Cancel a draft's schedule; it stays a draft",
				Params:     []Param{postIDParam()},
			},
			{
				Action: "update_post", Method: "PATCH", Path: "/posts/{post_id}", BodyKey: "post",
				Idempotent: true,
				Summary:    "Edit a post, changing only what is sent",
				Doc:        "A published post can be edited: its page shows the correction, and the email already sent does not change. Send at least one field.",
				Params:     []Param{postIDParam()},
				Body: object(map[string]any{
					"title":        str("Title, which is also the email's subject"),
					"summary":      str("The writer's own line about the post, shown in place of how it begins"),
					"body":         str(postBodyDoc),
					"visibility":   strEnum("listed, unlisted (reached by its link only) or email_only (no page)", "listed", "unlisted", "email_only"),
					"published_at": str("Corrected date of a published post that is on the web, ISO 8601, now or earlier. Dropped without a word for any other post"),
				}),
			},
			{
				Action: "update_post_link", Method: "PATCH", Path: "/posts/{post_id}/links/{link_id}", BodyKey: "post_link",
				Idempotent: true,
				Summary:    "Correct where a mailed link sends a reader from now on; the email already sent keeps its text",
				Params: []Param{
					postIDParam(),
					pathParam("link_id", "The link's ID (from list_post_links)"),
				},
				Body: object(map[string]any{
					"url": str("A web address, http or https"),
				}, "url"),
			},
		},
	},
		&Domain{
			Key:   "search",
			Tool:  "mailpiece_search",
			Title: "Mailpiece Search",
			Blurb: "Search a newsletter: the posts and the subscribers a query matches. Admins only.",
			Operations: []*Operation{
				{
					Action: "search", Method: "GET", Path: "/search", ReadOnly: true, Paginated: true,
					Summary: "Find the posts and the subscribers matching a query, newest first",
					Doc:     "Answers {posts, subscribers}. subscribers holds one page of the matches and page pages them; posts is whole and comes again on every page. Without q both are empty.",
					Params: []Param{
						{Name: "q", In: "query", Required: true, Description: "What to look for", Schema: str("What to look for")},
						pageParam(),
					},
				},
			},
		})
}
