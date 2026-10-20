package catalog

// The newsletter domains: what a newsletter keeps beside its posts and its
// subscribers. Snippets, journeys and their emails, the settings and the
// signed-in person's own user. Every write sends its fields flat; the
// controller wraps them itself. Rich text goes in as HTML.
func init() {
	trigger := pathParam("trigger", "The journey's trigger: subscriber_confirmed or subscriber_unsubscribed")
	emailID := pathParam("email_id", "The email's ID (from its journey)")
	emailFields := func() map[string]any {
		return map[string]any{
			"subject":    str("Subject"),
			"summary":    str("The writer's own line about the email, shown by an inbox beside the subject"),
			"body":       str("Body, as HTML"),
			"delay_days": integer("Days to wait after the trigger before mailing it; subscriber_confirmed only, dropped by the other journey"),
			"send_hour":  integer("Hour of the day to mail it at, 0 to 23; subscriber_confirmed only, dropped by the other journey"),
		}
	}

	Register(&Domain{
		Key:   "snippets",
		Tool:  "mailpiece_snippets",
		Title: "Mailpiece Snippets",
		Blurb: "Mailpiece snippets: named pieces of rich text a newsletter keeps and pastes into its posts. A snippet is pasted as ordinary text, so changing or deleting one never changes a post already written. Admins only.",
		Operations: []*Operation{
			{
				Action: "create_snippet", Method: "POST", Path: "/snippets",
				Summary: "Add a snippet",
				Body: object(map[string]any{
					"name":    str("Snippet name"),
					"content": str("Content, as HTML"),
				}, "name"),
			},
			{
				Action: "delete_snippet", Method: "DELETE", Path: "/snippets/{snippet_id}",
				Destructive: true, Idempotent: true,
				Summary: "Delete a snippet for good. " + cannotBeUndone,
				Params:  []Param{pathParam("snippet_id", "The snippet's ID")},
			},
			{
				Action: "get_snippet", Method: "GET", Path: "/snippets/{snippet_id}", ReadOnly: true,
				Summary: "Get a snippet",
				Doc:     "content is the text alone and content_html what was written.",
				Params:  []Param{pathParam("snippet_id", "The snippet's ID")},
			},
			{
				Action: "list_snippets", Method: "GET", Path: "/snippets", ReadOnly: true,
				Summary: "List every snippet, by name, each with its content",
			},
			{
				Action: "update_snippet", Method: "PATCH", Path: "/snippets/{snippet_id}",
				Idempotent: true,
				Summary:    "Change a snippet's name or content; only what is passed changes",
				Params:     []Param{pathParam("snippet_id", "The snippet's ID")},
				Body: object(map[string]any{
					"name":    str("Snippet name"),
					"content": str("Content, as HTML"),
				}),
			},
		},
	},
		&Domain{
			Key:   "journeys",
			Tool:  "mailpiece_journeys",
			Title: "Mailpiece Journeys",
			Blurb: "Mailpiece journeys: the emails a newsletter mails by itself to a person after they confirm their subscription (subscriber_confirmed, each email after its own delay) or after they unsubscribe (subscriber_unsubscribed, one email, at once). A journey is known by its trigger and always exists. An email is a draft until activated. Admins only.",
			Operations: []*Operation{
				{
					Action: "activate_journey_email", Method: "POST", Path: "/journeys/{trigger}/emails/{email_id}/activation",
					Destructive: true, Idempotent: true,
					Summary: "Switch an email on: from then on it is mailed by itself to real people as they reach it, until it is switched off; mail that has gone out stays sent. " + cannotBeUndone,
					Doc:     "An email with no subject or no body is refused with 422. Activating an active email changes nothing. Confirm with the person before calling it.",
					Params:  []Param{trigger, emailID},
				},
				{
					Action: "create_journey_email", Method: "POST", Path: "/journeys/{trigger}/emails",
					Summary: "Add an email to a journey, as a draft mailed to nobody until activated",
					Doc:     "The subscriber_unsubscribed journey holds one email: a second is refused with 403.",
					Params:  []Param{trigger},
					Body:    object(emailFields(), "subject"),
				},
				{
					Action: "deactivate_journey_email", Method: "DELETE", Path: "/journeys/{trigger}/emails/{email_id}/activation",
					Idempotent: true,
					Summary:    "Switch an email off: it is a draft again and is mailed to nobody",
					Doc:        "The app answers 204 with no body; read the email with get_journey_email to see it.",
					Params:     []Param{trigger, emailID},
				},
				{
					Action: "delete_journey_email", Method: "DELETE", Path: "/journeys/{trigger}/emails/{email_id}",
					Destructive: true, Idempotent: true,
					Summary: "Delete an email of a journey for good, active or not. " + cannotBeUndone,
					Params:  []Param{trigger, emailID},
				},
				{
					Action: "get_journey", Method: "GET", Path: "/journeys/{trigger}", ReadOnly: true,
					Summary: "Get a journey with its emails in full",
					Doc:     "Each email carries body (the text alone), body_html (what was written), delay_days, send_hour, time_zone and active.",
					Params:  []Param{trigger},
				},
				{
					Action: "get_journey_email", Method: "GET", Path: "/journeys/{trigger}/emails/{email_id}", ReadOnly: true,
					Summary: "Get one email of a journey in full, active or not",
					Doc:     "It carries body (the text alone), body_html (what was written), delay_days, send_hour, time_zone and active.",
					Params:  []Param{trigger, emailID},
				},
				{
					Action: "list_journeys", Method: "GET", Path: "/journeys", ReadOnly: true,
					Summary: "List every journey, each with its emails in full",
				},
				{
					Action: "test_journey_email", Method: "POST", Path: "/journeys/{trigger}/emails/{email_id}/test_delivery",
					Destructive: true,
					Summary:     "Mail one test copy of an email, active or not, to an address or to the signed-in person's own. It is real mail. " + cannotBeUndone,
					Doc:         "Refused with 422 while the newsletter has no sender or the email has no subject or body, and with 429 after 10 in 3 minutes.",
					Params:      []Param{trigger, emailID},
					Body: object(map[string]any{
						"email_address": str("Address to mail the test to; the signed-in person's own when left out"),
					}),
				},
				{
					Action: "update_journey_email", Method: "PATCH", Path: "/journeys/{trigger}/emails/{email_id}",
					Idempotent: true,
					Summary:    "Change an email of a journey; only what is passed changes",
					Doc:        "Pass at least one field. An active email is changed for everyone it has yet to reach, and cannot lose its subject or its body.",
					Params:     []Param{trigger, emailID},
					Body:       object(emailFields()),
				},
			},
		},
		&Domain{
			Key:   "settings",
			Tool:  "mailpiece_settings",
			Title: "Mailpiece Settings",
			Blurb: "Mailpiece settings: the newsletter's own configuration, each part a record of its own. Its name, description and language; the handle it answers at; who its mail is from; where its embedded subscription form sends a person back to; the code people join its team with; and whether the signed-in admin is emailed about new subscribers. Admins only.",
			Operations: []*Operation{
				{
					Action: "get_handle", Method: "GET", Path: "/account/handle", ReadOnly: true,
					Summary: "Get the handle the newsletter answers at beside its number, null while it has none",
				},
				{
					Action: "get_join_code", Method: "GET", Path: "/account/join_code", ReadOnly: true,
					Summary: "Get the code people join the newsletter's team with, its link, and how often it has been used",
					Doc:     "active is false once usage_count has reached usage_limit.",
				},
				{
					Action: "get_newsletter", Method: "GET", Path: "/account/newsletter", ReadOnly: true,
					Summary: "Get the newsletter's name, description and language",
					Doc:     "description is the text alone and description_html what was written. avatar_url is there only when the newsletter has an avatar.",
				},
				{
					Action: "get_notifications", Method: "GET", Path: "/notifications/settings", ReadOnly: true,
					Summary: "Get whether the signed-in admin is emailed when someone confirms a subscription",
					Doc:     "It is the token's own user's setting, on until switched off.",
				},
				{
					Action: "get_sending", Method: "GET", Path: "/account/sending", ReadOnly: true,
					Summary: "Get who the newsletter's mail is from and how it leaves: from name, from address, reply-to and transport",
					Doc: "A newsletter mails nothing until it has a from name, a from address and a transport. transport is null, relay or smtp, " +
						"with the relay and SMTP settings beside it. The relay token and the SMTP password are never returned.",
				},
				{
					Action: "get_subscribe_form", Method: "GET", Path: "/account/subscribe_form", ReadOnly: true,
					Summary: "Get where the embedded subscription form sends a person back to",
				},
				{
					Action: "remove_sending_relay", Method: "DELETE", Path: "/account/sending/relay",
					Destructive: true, Idempotent: true,
					Summary: "Remove the relay and its token; the newsletter sends nothing until a transport is set again. " + cannotBeUndone,
					Doc:     "The stored credentials are deleted with it. The app answers 204 with no body. Confirm with the person before calling it.",
				},
				{
					Action: "remove_sending_smtp", Method: "DELETE", Path: "/account/sending/smtp",
					Destructive: true, Idempotent: true,
					Summary: "Remove the SMTP server and its password; the newsletter sends nothing until a transport is set again. " + cannotBeUndone,
					Doc:     "The stored credentials are deleted with it. The app answers 204 with no body. Confirm with the person before calling it.",
				},
				{
					Action: "reset_join_code", Method: "DELETE", Path: "/account/join_code",
					Destructive: true,
					Summary:     "Retire the join code and issue a new one, used no times; the old code and every link carrying it stop working. " + cannotBeUndone,
				},
				{
					Action: "update_handle", Method: "PATCH", Path: "/account/handle",
					Destructive: true, Idempotent: true,
					Summary: "Change the newsletter's handle, or clear it so the newsletter answers at its number alone. Every link already mailed with the old handle stops working. " + cannotBeUndone,
					Doc:     "The old handle is free for another newsletter at once. The number keeps working beside the handle. Confirm with the person before calling it.",
					Body: object(map[string]any{
						"handle": str("The word the newsletter answers at: 3 to 40 lowercase letters, digits and hyphens, starting with a letter; empty clears it"),
					}, "handle"),
				},
				{
					Action: "update_join_code", Method: "PATCH", Path: "/account/join_code",
					Idempotent: true,
					Summary:    "Change how many times the join code can be used",
					Body: object(map[string]any{
						"usage_limit": integer("Times the code can be used"),
					}, "usage_limit"),
				},
				{
					Action: "update_newsletter", Method: "PATCH", Path: "/account/newsletter",
					Idempotent: true,
					Summary:    "Change the newsletter's name, description or language; only what is passed changes",
					Doc:        "Pass at least one field. The name and description are public, and the language is the one every page and every email of the newsletter is written in.",
					Body: object(map[string]any{
						"name":        str("Newsletter name"),
						"description": str("Description, as HTML"),
						"locale":      strEnum("Language the newsletter is written in", "en", "es"),
					}),
				},
				{
					Action: "update_notifications", Method: "PATCH", Path: "/notifications/settings",
					Idempotent: true,
					Summary:    "Set whether the signed-in admin is emailed when someone confirms a subscription",
					Doc:        "It changes the token's own user's setting and nobody else's.",
					Body: object(map[string]any{
						"new_subscriber_notifications": boolean("Email the signed-in admin when someone confirms a subscription"),
					}, "new_subscriber_notifications"),
				},
				{
					Action: "update_sending", Method: "PATCH", Path: "/account/sending",
					Idempotent: true,
					Summary:    "Change who the newsletter's mail is from; only what is passed changes",
					Doc:        "Pass at least one field.",
					Body: object(map[string]any{
						"from_name":        str("Name the mail is from"),
						"from_address":     str("Address the mail is from"),
						"reply_to_address": str("Address replies go to; empty clears it"),
					}),
				},
				{
					Action: "update_sending_relay", Method: "PATCH", Path: "/account/sending/relay",
					Idempotent: true,
					Summary:    "Send the newsletter's mail through a relay; replaces an SMTP server if one was set",
					Doc:        "A newsletter has one transport, relay or SMTP. relay_url is always required. relay_token is needed to set a relay up and refused (422) on one already set: to change it, remove the relay and set it up again. relay_token is write-only, never returned.",
					Body: object(map[string]any{
						"relay_url":   str("Address of the relay"),
						"relay_token": str("Relay token; write-only, never returned"),
					}, "relay_url"),
				},
				{
					Action: "update_sending_smtp", Method: "PATCH", Path: "/account/sending/smtp",
					Idempotent: true,
					Summary:    "Send the newsletter's mail through an SMTP server; replaces a relay if one was set",
					Doc:        "A newsletter has one transport, relay or SMTP. smtp_address and smtp_port are always required. smtp_username and smtp_password are optional, and setting up with a username needs a password. smtp_password is refused (422) on a server already set: to change it, remove the server and set it up again. smtp_password is write-only, never returned.",
					Body: object(map[string]any{
						"smtp_address":  str("SMTP server address"),
						"smtp_port":     integer("SMTP server port"),
						"smtp_username": str("SMTP username"),
						"smtp_password": str("SMTP password; write-only, never returned"),
					}, "smtp_address", "smtp_port"),
				},
				{
					Action: "update_subscribe_form", Method: "PATCH", Path: "/account/subscribe_form",
					Idempotent: true,
					Summary:    "Set where the embedded subscription form sends a person back to",
					Body: object(map[string]any{
						"subscription_return_url": str("Address to send a person back to; empty clears it"),
					}, "subscription_return_url"),
				},
			},
		},
		&Domain{
			Key:   "users",
			Tool:  "mailpiece_users",
			Title: "Mailpiece Users",
			Blurb: "Mailpiece users: the signed-in person's own user on the newsletter. A user reaches only their own profile: anyone else's is a 404. The identity tool shows your user's ID on each account.",
			Operations: []*Operation{
				{
					Action: "get_user", Method: "GET", Path: "/users/{user_id}", ReadOnly: true,
					Summary: "Get your own user: name, role, email address",
					Params:  []Param{pathParam("user_id", "Your user's ID on this account (from the identity tool)")},
				},
				{
					Action: "update_user", Method: "PATCH", Path: "/users/{user_id}",
					Idempotent: true,
					Summary:    "Change your own name",
					Params:     []Param{pathParam("user_id", "Your user's ID on this account (from the identity tool)")},
					Body: object(map[string]any{
						"name": str("Your name"),
					}, "name"),
				},
			},
		})
}
