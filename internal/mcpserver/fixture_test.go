package mcpserver

import "github.com/mailpiece/mailpiece-cli/internal/mcpserver/catalog"

// The fixtures domain exists in this package's tests only. It gives the
// dispatch and wire tests one of every shape an operation takes (an
// account-scoped read, a page, a query, a wrapped body, a flat body, a
// bodiless write), whatever domains the catalog ships with.
func init() {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	integer := func(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }
	object := func(properties map[string]any, required ...any) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required}
	}
	path := func(name string) catalog.Param {
		return catalog.Param{Name: name, In: "path", Required: true, Schema: str(name)}
	}
	query := func(name string, schema map[string]any) catalog.Param {
		return catalog.Param{Name: name, In: "query", Schema: schema}
	}

	catalog.Register(&catalog.Domain{
		Key:   "fixtures",
		Tool:  "mailpiece_fixtures",
		Title: "Mailpiece Fixtures",
		Blurb: "Fixtures, for this package's tests.",
		Operations: []*catalog.Operation{
			{
				Action: "activate_fixture", Method: "POST", Path: "/fixtures/{fixture_id}/activation",
				Summary: "Activate a fixture",
				Params:  []catalog.Param{path("fixture_id")},
			},
			{
				Action: "create_fixture", Method: "POST", Path: "/fixtures", BodyKey: "fixture",
				Summary: "Create a fixture",
				Body:    object(map[string]any{"name": str("Name")}, "name"),
			},
			{
				Action: "create_note", Method: "POST", Path: "/fixtures/{fixture_id}/notes",
				Summary: "Add a note to a fixture",
				Params:  []catalog.Param{path("fixture_id")},
				Body:    object(map[string]any{"text": str("Text")}, "text"),
			},
			{
				Action: "get_fixture", Method: "GET", Path: "/fixtures/{fixture_id}", ReadOnly: true,
				Summary: "Get a fixture",
				Params:  []catalog.Param{path("fixture_id")},
			},
			{
				Action: "list_fixtures", Method: "GET", Path: "/fixtures", ReadOnly: true, Paginated: true,
				Summary: "List fixtures",
				Params:  []catalog.Param{query("page", integer("Page")), query("state", str("State"))},
			},
			{
				Action: "move_fixture", Method: "PATCH", Path: "/fixtures/{fixture_id}/position",
				Summary: "Move a fixture",
				Params:  []catalog.Param{path("fixture_id")},
				Body:    object(map[string]any{"direction": str("Direction")}, "direction"),
			},
			{
				Action: "place_fixture", Method: "POST", Path: "/fixtures/{fixture_id}/shelves/{shelf_id}/placement",
				Summary: "Place a fixture on a shelf",
				Params:  []catalog.Param{path("fixture_id"), path("shelf_id")},
			},
		},
	})
}
