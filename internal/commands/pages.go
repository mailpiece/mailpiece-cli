package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/internal/render"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// pageFlags are a paged list's --page and --all.
type pageFlags struct {
	all  bool
	page int
}

func (p *pageFlags) register(cmd *cobra.Command) {
	cmd.Flags().IntVar(&p.page, "page", 0, "Page number")
	cmd.Flags().BoolVar(&p.all, "all", false, "Fetch all pages")
}

// pagedList is how a paged list prints: its columns, what it lists, the command
// that repeats it for another page, and its breadcrumbs.
type pagedList[T any] struct {
	cols        render.Columns
	noun        string
	again       string
	breadcrumbs []Breadcrumb
}

// run fetches the page asked for at the list's path, or every page with --all, and
// prints it.
func (l pagedList[T]) run(cmd *cobra.Command, p *pageFlags, path string, fetch func(ctx context.Context, path string) ([]T, *mailpiece.Response, error)) error {
	if err := checkLimitAll(p.all); err != nil {
		return err
	}
	if p.page < 0 {
		return errors.NewInvalidArgsError("--page must be 1 or more")
	}
	if p.all && p.page > 0 {
		return errors.NewInvalidArgsError("--all and --page cannot be used together")
	}
	if p.page > 0 {
		path += "?page=" + strconv.Itoa(p.page)
	}

	var items any
	var linkNext string
	if p.all {
		pages, err := getSDK().GetAll(cmd.Context(), path)
		if err != nil {
			return convertSDKError(err)
		}
		items = jsonAnySlice(pages)
	} else {
		data, resp, err := fetch(cmd.Context(), path)
		if err != nil {
			return convertSDKError(err)
		}
		items = normalizeAny(data)
		// The app links a next page past the end of a list.
		if len(data) > 0 {
			linkNext = resp.NextPageURL()
		}
	}

	summary := fmt.Sprintf("%d %s", dataCount(items), l.noun)
	if p.all {
		summary += " (all)"
	} else if p.page > 0 {
		summary += fmt.Sprintf(" (page %d)", p.page)
	}

	breadcrumbs := l.breadcrumbs
	hasNext := linkNext != ""
	if hasNext {
		nextPage := max(p.page, 1) + 1
		breadcrumbs = append(breadcrumbs, breadcrumb("next", fmt.Sprintf("%s --page %d", l.again, nextPage), "Next page"))
	}

	printListPaginated(items, l.cols, hasNext, linkNext, p.all, summary, breadcrumbs)
	return nil
}

// jsonAnySlice turns the raw pages GetAll collects into maps for the output layer.
func jsonAnySlice(items []json.RawMessage) any {
	maps := make([]map[string]any, 0, len(items))
	for _, item := range items {
		var m map[string]any
		if json.Unmarshal(item, &m) == nil {
			maps = append(maps, m)
		}
	}
	return maps
}

// listPath builds a list's own path, ids checked and escaped, for run.
func listPath(format string, args ...any) (string, error) {
	path, err := mailpiece.Path(format, args...)
	if err != nil {
		return "", convertSDKError(err)
	}
	return path, nil
}
