package commands

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/mcpserver/catalog"
	"github.com/mailpiece/mailpiece-cli/internal/render"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// The fixture commands exist in this package's tests only, under a hidden parent
// that help, the catalogue and SURFACE.txt leave out. They give the output, paging and format tests an account-scoped list
// and a record to print, whatever commands the CLI ships with.
type fixture struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var fixtureColumns = render.Columns{
	{Header: "Name", Field: "name"},
	{Header: "ID", Field: "id"},
}

var fixtureCmd = &cobra.Command{Use: "fixture", Hidden: true}

var fixtureListPages pageFlags

var fixtureListCmd = &cobra.Command{
	Use:   "list",
	Short: "List fixtures",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		return pagedList[fixture]{
			cols: fixtureColumns, noun: "fixtures", again: "mailpiece fixture list",
			breadcrumbs: []Breadcrumb{breadcrumb("show", "mailpiece fixture show ID", "View a fixture")},
		}.run(cmd, &fixtureListPages, "/fixtures.json", listFixtures)
	},
}

var fixtureShowCmd = &cobra.Command{
	Use:   "show ID",
	Short: "Show a fixture",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		path, err := listPath("/fixtures/%s.json", args[0])
		if err != nil {
			return err
		}
		resp, err := getSDK().Get(cmd.Context(), path)
		if err != nil {
			return convertSDKError(err)
		}
		var data fixture
		if err := resp.UnmarshalData(&data); err != nil {
			return convertSDKError(err)
		}
		printDetail(normalizeAny(data), "", nil)
		return nil
	},
}

func listFixtures(ctx context.Context, path string) ([]fixture, *mailpiece.Response, error) {
	resp, err := getSDK().Get(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	var data []fixture
	if err := resp.UnmarshalData(&data); err != nil {
		return nil, resp, err
	}
	return data, resp, nil
}

func init() {
	fixtureListPages.register(fixtureListCmd)
	fixtureCmd.AddCommand(fixtureListCmd, fixtureShowCmd)
	rootCmd.AddCommand(fixtureCmd)

	// The same record as an MCP domain, for the tests of `mailpiece mcp`: one
	// read and one write, so --writes and --domains have something to narrow.
	text := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	catalog.Register(&catalog.Domain{
		Key:   "fixtures",
		Tool:  "mailpiece_fixtures",
		Title: "Mailpiece Fixtures",
		Blurb: "Fixtures, for this package's tests.",
		Operations: []*catalog.Operation{
			{
				Action: "create_fixture", Method: "POST", Path: "/fixtures", BodyKey: "fixture",
				Summary: "Create a fixture",
				Body:    map[string]any{"type": "object", "properties": map[string]any{"name": text("Name")}, "required": []any{"name"}},
			},
			{
				Action: "list_fixtures", Method: "GET", Path: "/fixtures", ReadOnly: true,
				Summary: "List fixtures",
			},
		},
	})
}

// mcpToolCount is how many tools the catalog serves: every domain with writes,
// and without them only the domains that have a read.
func mcpToolCount(t interface{ Fatal(...any) }, writes bool) int {
	domains, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, d := range domains {
		if _, ok := d.FilterReadOnly(); ok || writes {
			count++
		}
	}
	return count
}
