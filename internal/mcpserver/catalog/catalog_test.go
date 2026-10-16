// These tests pin the hand-written catalog: the curation validates cleanly,
// the invariants the dispatcher relies on hold, and the full rendered
// surface is snapshotted so any change shows its effect as a reviewable
// diff. The generic gateway machinery is covered by the toolkit's own suite
// in github.com/basecamp/mcp/gateway.
package catalog

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/basecamp/mcp/mcptest"
)

func load(t *testing.T) []*Domain {
	t.Helper()
	domains, err := Load()
	if err != nil {
		t.Fatalf("catalog must validate cleanly: %v", err)
	}
	return domains
}

// The domains and their counts are not listed here: the snapshot pins the
// whole surface, and a list every lane edits is a list every lane conflicts on.
func TestCatalogShape(t *testing.T) {
	for _, d := range load(t) {
		if d.Tool != "mailpiece_"+d.Key {
			t.Errorf("domain %q tool = %q", d.Key, d.Tool)
		}
		if d.Title == "" {
			t.Errorf("domain %q has no title", d.Key)
		}
		if d.Blurb == "" {
			t.Errorf("domain %q has no blurb", d.Key)
		}
		if len(d.Operations) == 0 {
			t.Errorf("domain %q has no operations", d.Key)
		}
	}
}

func TestDomainsWithoutAReadAreWritesOnly(t *testing.T) {
	var writesOnly []string
	for _, d := range load(t) {
		if _, ok := d.FilterReadOnly(); !ok {
			writesOnly = append(writesOnly, d.Key)
		}
	}
	if got := strings.Join(writesOnly, ","); got != "access_tokens" {
		t.Errorf("domains dropped read-only = %s, want access_tokens", got)
	}
}

func TestReadsAreGetsAndGetsAreReads(t *testing.T) {
	for _, d := range load(t) {
		for _, op := range d.Operations {
			if op.ReadOnly != (op.Method == "GET") {
				t.Errorf("%s/%s: method %s with readonly %v", d.Key, op.Action, op.Method, op.ReadOnly)
			}
		}
	}
}

// A summary says a write cannot be undone exactly when the write is marked
// destructive, so the tool description and the destructive hint agree.
func TestCannotBeUndoneIsDestructive(t *testing.T) {
	for _, d := range load(t) {
		for _, op := range d.Operations {
			says := strings.Contains(strings.ToLower(op.Summary), "cannot be undone")
			if says && !op.Destructive {
				t.Errorf("%s/%s: summary says it cannot be undone, but it is not marked destructive", d.Key, op.Action)
			}
			if op.Destructive && !strings.HasSuffix(op.Summary, cannotBeUndone) {
				t.Errorf("%s/%s: destructive, but its summary does not end %q", d.Key, op.Action, cannotBeUndone)
			}
		}
	}
}

func TestOnlyIdentityAndAccessTokensAreUnscoped(t *testing.T) {
	want := []string{"access_tokens/create_access_token", "access_tokens/delete_access_token", "identity/get_identity"}
	var got []string
	for _, d := range load(t) {
		for _, op := range d.Operations {
			if op.Unscoped {
				got = append(got, d.Key+"/"+op.Action)
			}
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("unscoped operations = %v, want %v; unscoped operations are the deliberate exception", got, want)
	}
}

func TestPaginatedOperationsTakePage(t *testing.T) {
	for _, d := range load(t) {
		for _, op := range d.Operations {
			hasPage := false
			for _, p := range op.Params {
				if p.Name == "page" && p.In == "query" {
					hasPage = true
				}
			}
			if op.Paginated != hasPage {
				t.Errorf("%s/%s: paginated and the page param travel together", d.Key, op.Action)
			}
		}
	}
}

func TestBodyRequiredFieldsExist(t *testing.T) {
	for _, d := range load(t) {
		for _, op := range d.Operations {
			if op.Body == nil {
				continue
			}
			props, ok := op.Body["properties"].(map[string]any)
			if !ok {
				t.Errorf("%s/%s: body must declare properties", d.Key, op.Action)
				continue
			}
			if required, ok := op.Body["required"].([]any); ok {
				for _, name := range required {
					field, _ := name.(string)
					if _, present := props[field]; !present {
						t.Errorf("%s/%s: required field %q must be a property", d.Key, op.Action, field)
					}
				}
			}
		}
	}
}

func TestValidateRejectsCurationMistakes(t *testing.T) {
	cases := []struct {
		name   string
		domain *Domain
		want   string
	}{
		{
			"unsorted operations",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "b", Method: "GET", Path: "/b", ReadOnly: true},
				{Action: "a", Method: "GET", Path: "/a", ReadOnly: true},
			}},
			"sorted",
		},
		{
			"reserved describe action",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "describe", Method: "GET", Path: "/x", ReadOnly: true},
			}},
			"reserved",
		},
		{
			"unsupported method",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "a", Method: "HEAD", Path: "/a"},
			}},
			"unsupported method",
		},
		{
			"write marked read-only",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "a", Method: "POST", Path: "/a", ReadOnly: true},
			}},
			"readonly must mirror the method",
		},
		{
			"read not marked read-only",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "a", Method: "GET", Path: "/a"},
			}},
			"readonly must mirror the method",
		},
		{
			"body on a GET",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "a", Method: "GET", Path: "/a", ReadOnly: true, Body: object(map[string]any{"name": str("x")})},
			}},
			"cannot carry a body",
		},
		{
			"body on a DELETE",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "a", Method: "DELETE", Path: "/a", Body: object(map[string]any{"name": str("x")})},
			}},
			"cannot carry a body",
		},
		{
			"path token without param",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "a", Method: "GET", Path: "/a/{id}", ReadOnly: true},
			}},
			"no matching path param",
		},
		{
			"path param not in path",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "a", Method: "GET", Path: "/a", ReadOnly: true,
					Params: []Param{{Name: "id", In: "path", Required: true, Schema: str("x")}}},
			}},
			"does not appear in path",
		},
		{
			"optional path param",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "a", Method: "GET", Path: "/a/{id}", ReadOnly: true,
					Params: []Param{{Name: "id", In: "path", Schema: str("x")}}},
			}},
			"must be required",
		},
		{
			"param in a header",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "a", Method: "GET", Path: "/a", ReadOnly: true,
					Params: []Param{{Name: "id", In: "header", Schema: str("x")}}},
			}},
			`in must be "path" or "query"`,
		},
		{
			"param without schema",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "a", Method: "GET", Path: "/a", ReadOnly: true,
					Params: []Param{{Name: "page", In: "query"}}},
			}},
			"has no schema",
		},
		{
			"body not an object",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "a", Method: "POST", Path: "/a", Body: map[string]any{"type": "string"}},
			}},
			"must be an object schema",
		},
		{
			"body_key without body",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "a", Method: "POST", Path: "/a", BodyKey: "thing"},
			}},
			"without a body schema",
		},
		{
			"destructive read",
			&Domain{Key: "x", Tool: "mailpiece_x", Operations: []*Operation{
				{Action: "a", Method: "GET", Path: "/a", ReadOnly: true, Destructive: true},
			}},
			"neither destructive nor idempotent",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(tc.domain)
			if err == nil {
				t.Fatal("validate accepted the mistake")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestLoadRejectsDuplicateDomains(t *testing.T) {
	cases := []struct {
		name   string
		domain *Domain
		want   string
	}{
		{"duplicate key", &Domain{Key: "identity", Tool: "mailpiece_other"}, `duplicate domain key "identity"`},
		{"duplicate tool", &Domain{Key: "other", Tool: "mailpiece_identity"}, `duplicate tool name "mailpiece_identity"`},
		{"no title", &Domain{Key: "other", Tool: "mailpiece_other"}, `domain "other" has no title`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			saved := registered
			t.Cleanup(func() { registered = saved })
			registered = append(slices.Clone(saved), tc.domain)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestPathTokens(t *testing.T) {
	got := PathTokens("/posts/{post_id}/links/{link_id}")
	if !slices.Equal(got, []string{"post_id", "link_id"}) {
		t.Errorf("tokens = %v", got)
	}
	if got := PathTokens("/posts"); got != nil {
		t.Errorf("tokens = %v, want none", got)
	}
}

func TestFilterReadOnlyKeepsOnlyReads(t *testing.T) {
	for _, d := range load(t) {
		filtered, ok := d.FilterReadOnly()
		if !ok {
			continue
		}
		if !filtered.AllReadOnly() {
			t.Errorf("%s: filtered domain still holds a write", d.Key)
		}
		if filtered.ToolName() != d.ToolName() {
			t.Errorf("%s: filtered tool = %q", d.Key, filtered.ToolName())
		}
	}
}

// TestSnapshot renders the entire served surface (tool names,
// descriptions, input schemas, and every action's describe payload) so a
// catalog change shows its full effect as a reviewable diff. Regenerate
// with `go test ./internal/mcpserver/catalog -update`.
func TestSnapshot(t *testing.T) {
	var b strings.Builder
	for _, d := range load(t) {
		b.WriteString("==== TOOL " + d.ToolName() + " ====\n")
		b.WriteString("title: " + d.ToolTitle() + "\n")
		b.WriteString(d.Description())
		b.WriteString("---- input schema ----\n")
		writeJSON(t, &b, d.InputSchema())
		for _, op := range d.Operations {
			b.WriteString("---- describe " + op.Action + " ----\n")
			payload, err := d.Describe(op.Action)
			if err != nil {
				t.Fatal(err)
			}
			writeJSON(t, &b, payload)
		}
	}
	mcptest.Snapshot(t, filepath.Join("testdata", "catalog_snapshot.txt"), []byte(b.String()))
}

func writeJSON(t *testing.T, b *strings.Builder, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b.Write(data)
	b.WriteString("\n")
}
