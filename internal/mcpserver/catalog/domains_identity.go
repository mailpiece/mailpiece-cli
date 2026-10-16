package catalog

// The shell's own domain. Each operation mirrors one JSON endpoint of the
// Mailpiece app; operations within a domain are sorted by action name.
func init() {
	Register(&Domain{
		Key:   "identity",
		Tool:  "mailpiece_identity",
		Title: "Mailpiece Identity",
		Blurb: "Who you are on Mailpiece, a newsletter app where one account is one newsletter: the accounts (newsletters) your token can reach and your user and role in each. Call this first to learn which newsletter you are working in.",
		Operations: []*Operation{
			{
				Action: "get_identity", Method: "GET", Path: "/my/identity", ReadOnly: true, Unscoped: true,
				Summary: "List the accounts the token can reach, with your user in each",
				Doc:     "Each account carries a slug naming it in API paths: always its number. A handle also answers in a path but changes, so configuration keeps the number. The other tools run against the account the server is configured for; this action shows every account the token can reach.",
			},
		},
	},
		&Domain{
			Key:   "access_tokens",
			Tool:  "mailpiece_access_tokens",
			Title: "Mailpiece Access Tokens",
			Blurb: "Mailpiece access tokens: the signed-in person's personal access tokens, which let a program such as this one act as them. A token is read or write; read allows looking only.",
			Operations: []*Operation{
				{
					Action: "create_access_token", Method: "POST", Path: "/my/access_tokens", BodyKey: "access_token", Unscoped: true,
					Summary: "Create an access token for the signed-in person. The token is in the response once and never again.",
					Body: object(map[string]any{
						"description": str("What the token is for"),
						"permission":  strEnum("read (looking only) or write", "read", "write"),
					}, "description", "permission"),
				},
				{
					Action: "delete_access_token", Method: "DELETE", Path: "/my/access_tokens/{access_token_id}", Unscoped: true,
					Destructive: true, Idempotent: true,
					Summary: "Delete one of the signed-in person's access tokens; whatever uses it stops working. " + cannotBeUndone,
					Params:  []Param{pathParam("access_token_id", "The access token's ID (from its create)")},
				},
			},
		})
}
