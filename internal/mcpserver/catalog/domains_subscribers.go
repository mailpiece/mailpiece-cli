package catalog

var subscriberID = pathParam("subscriber_id", "The subscriber's ID (from a subscribers list)")

// subscriberList is the list of the subscribers in one state.
func subscriberList(action, path, summary string) *Operation {
	return &Operation{
		Action: action, Method: "GET", Path: path, ReadOnly: true, Paginated: true,
		Summary: summary,
		Params:  []Param{pageParam()},
	}
}

// The subscribers lane: the people on the newsletter's list.
func init() {
	Register(&Domain{
		Key:   "subscribers",
		Tool:  "mailpiece_subscribers",
		Title: "Mailpiece Subscribers",
		Blurb: "Mailpiece subscribers: the people on the newsletter's list. A subscriber's status is one of, read in this order: bounced (the mail provider stopped delivering to the address), deactivated (an admin stopped it), unsubscribed (the person left), confirmed, or unconfirmed (it has not answered its confirmation yet). Only a confirmed subscriber is mailed. Only the person undoes an unsubscribe or a bounce, by subscribing again from the public form and confirming from the link mailed to them: no action here does it for them. Admins only.",
		Operations: []*Operation{
			{
				Action: "activate_subscriber", Method: "POST", Path: "/subscribers/{subscriber_id}/activation",
				Idempotent: true,
				Summary:    "Resume a subscriber an admin stopped. It undoes neither an unsubscribe nor a bounce.",
				Doc:        "Reverses deactivate_subscriber and nothing else: the subscriber goes back to the state it was in, confirmed and mailed again or still unconfirmed. The app answers 201 only when it resumed the person and 200 when it changed nothing. The result here is the subscriber either way, without the status: one that was not stopped comes back unchanged, so read its status with get_subscriber first to tell the two apart.",
				Params:     []Param{subscriberID},
			},
			{
				Action: "create_subscriber", Method: "POST", Path: "/subscribers", BodyKey: "subscriber",
				Summary: "Add one address to the list, already confirmed: no confirmation is mailed. Refused for an address that unsubscribed, bounced or was stopped.",
				Doc:     "The confirmation's journey does not start, since the person did not confirm it themselves. An address already on the list is answered as it is (200, not 201), and one that had not confirmed yet is confirmed. An address that unsubscribed, bounced or was stopped is never brought back this way: it is refused with a 422, as is one that is not an address, and the error says under email which of the four it was. Only the person undoes an unsubscribe or a bounce; a stopped subscriber is resumed with activate_subscriber.",
				Body: object(map[string]any{
					"email": str("The email address to add"),
				}, "email"),
			},
			{
				Action: "deactivate_subscriber", Method: "DELETE", Path: "/subscribers/{subscriber_id}/activation",
				Idempotent: true,
				Summary:    "Stop mailing a subscriber, keeping it and its history; activate_subscriber resumes it.",
				Doc:        "The admin's own switch. Only a confirmed or unconfirmed subscriber is stopped; one that unsubscribed or bounced is refused with a 403 that says so, since it is not mailed as it is. One already stopped is answered unchanged.",
				Params:     []Param{subscriberID},
			},
			{
				Action: "delete_subscriber", Method: "DELETE", Path: "/subscribers/{subscriber_id}",
				Destructive: true, Idempotent: true,
				Summary: "Delete a subscriber for good, with its deliveries and journey history. " + cannotBeUndone,
				Doc:     "Nothing then remembers that the address unsubscribed or bounced, so it can be added again as new: deleting is the one way an admin gets past either. Confirm with the person before running it. To stop mailing someone and keep their history, use deactivate_subscriber.",
				Params:  []Param{subscriberID},
			},
			{
				Action: "get_subscriber", Method: "GET", Path: "/subscribers/{subscriber_id}", ReadOnly: true, Paginated: true,
				Summary: "Show a subscriber with the issues mailed to it, newest first",
				Doc:     "deliveries holds one page of the posts mailed to the subscriber, each with its id, created_at and status: sent, refused, unverified (handed over and never answered, so never sent again) or pending. page pages the deliveries.",
				Params:  []Param{subscriberID, pageParam()},
			},
			subscriberList("list_bounced_subscribers", "/subscribers/bounced", "List the subscribers the mail provider stopped delivering to, most recently bounced first"),
			subscriberList("list_deactivated_subscribers", "/subscribers/deactivated", "List the subscribers an admin stopped, most recently stopped first"),
			subscriberList("list_subscribers", "/subscribers", "List the confirmed subscribers, the people an issue is mailed to, newest first"),
			subscriberList("list_unconfirmed_subscribers", "/subscribers/unconfirmed", "List the subscribers that have not answered their confirmation yet, newest first"),
			subscriberList("list_unsubscribed_subscribers", "/subscribers/unsubscribed", "List the subscribers that left, most recently unsubscribed first"),
		},
	})
}
