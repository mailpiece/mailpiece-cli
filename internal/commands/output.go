package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/basecamp/cli/output"
	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/internal/render"
)

// CommandResult holds the result of a command execution for testing.
type CommandResult struct {
	Response *output.Response
}

// lastResult stores the last command result (for testing)
var lastResult *CommandResult

// testBuf captures output for test mode
var testBuf bytes.Buffer

// lastRawOutput holds the raw output from the last command (before buffer reset).
var lastRawOutput string

// errOutputWrite stores the first output rendering/writer error from the current command.
var errOutputWrite error

func recordOutputError(err error) {
	if err != nil && errOutputWrite == nil {
		errOutputWrite = err
	}
}

func writeOutputString(s string) {
	_, err := io.WriteString(outWriter, s)
	recordOutputError(err)
}

// captureResponse parses the writer buffer into lastResult after each shim call.
func captureResponse() {
	if lastResult == nil {
		return
	}
	lastRawOutput = testBuf.String()
	lastResult.Response = nil
	var resp output.Response
	if json.Unmarshal(testBuf.Bytes(), &resp) == nil {
		lastResult.Response = &resp
	}
	testBuf.Reset()
}

// printSuccess prints a success response.
func printSuccess(data any) {
	switch out.EffectiveFormat() {
	case output.FormatStyled:
		writeOutputString(renderHumanData(data, false))
		captureResponse()
	case output.FormatMarkdown:
		writeOutputString(renderHumanData(data, true))
		captureResponse()
	default:
		recordOutputError(out.OK(data))
		captureResponse()
	}
}

// breadcrumb creates a single breadcrumb.
func breadcrumb(action, cmd, description string) Breadcrumb {
	return Breadcrumb{Action: action, Cmd: cmd, Description: description}
}

// printSuccessWithBreadcrumbs prints a success response with breadcrumbs.
func printSuccessWithBreadcrumbs(data any, summary string, breadcrumbs []Breadcrumb) {
	opts := []output.ResponseOption{output.WithBreadcrumbs(breadcrumbs...)}
	if summary != "" {
		opts = append(opts, output.WithSummary(summary))
	}
	recordOutputError(out.OK(data, opts...))
	captureResponse()
}

// checkLimitAll validates that --limit and --all are not both set.
func checkLimitAll(all bool) error {
	if cfgLimit > 0 && all {
		return errors.NewInvalidArgsError("--limit and --all cannot be used together")
	}
	return nil
}

// truncateData applies --limit client-side truncation to a slice.
// Returns the (possibly truncated) data and the original count.
// Handles both []any and typed slices (e.g. []Attachment).
func truncateData(data any) (any, int) {
	if arr, ok := data.([]any); ok {
		originalCount := len(arr)
		if cfgLimit > 0 && originalCount > cfgLimit {
			return arr[:cfgLimit], originalCount
		}
		return data, originalCount
	}
	// Handle typed slices via reflect
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Slice {
		originalCount := v.Len()
		if cfgLimit > 0 && originalCount > cfgLimit {
			return v.Slice(0, cfgLimit).Interface(), originalCount
		}
		return data, originalCount
	}
	return data, 0
}

// dataCount returns the length of data if it's a slice.
func dataCount(data any) int {
	if arr, ok := data.([]any); ok {
		return len(arr)
	}
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Slice {
		return v.Len()
	}
	return 0
}

// printList renders list data with format-aware dispatch.
// For non-paginated lists (no --all flag). Applies --limit truncation.
func printList(data any, cols render.Columns, summary string, breadcrumbs []Breadcrumb) {
	data, originalCount := truncateData(data)

	// For non-paginated lists, generate a simple limit notice (no --all to suggest)
	notice := ""
	if cfgLimit > 0 && originalCount > cfgLimit {
		notice = fmt.Sprintf("Showing %d of %d results", cfgLimit, originalCount)
	}

	switch out.EffectiveFormat() {
	case output.FormatStyled:
		body := render.StyledList(toMaps(data), cols, summary)
		writeOutputString(appendHumanSections(body, notice, "", breadcrumbs, false))
		captureResponse()
	case output.FormatMarkdown:
		body := render.MarkdownList(toMaps(data), cols, summary)
		writeOutputString(appendHumanSections(body, notice, "", breadcrumbs, true))
		captureResponse()
	default:
		opts := []output.ResponseOption{output.WithBreadcrumbs(breadcrumbs...)}
		if summary != "" {
			opts = append(opts, output.WithSummary(summary))
		}
		if notice != "" {
			opts = append(opts, output.WithNotice(notice))
		}
		recordOutputError(out.OK(data, opts...))
		captureResponse()
	}
}

// printListPaginated renders paginated list data with format-aware dispatch.
// For paginated lists (commands with --all flag). Applies --limit truncation, and
// a next page's link makes the notice that the list goes on: the app's pages grow
// in size, so a count cannot tell.
func printListPaginated(data any, cols render.Columns, hasNext bool, nextURL string, all bool, summary string, breadcrumbs []Breadcrumb) {
	data, originalCount := truncateData(data)
	notice := ""
	switch {
	case cfgLimit > 0 && originalCount > cfgLimit:
		notice = fmt.Sprintf("Showing %d of %d results", cfgLimit, originalCount)
	case hasNext && !all:
		notice = fmt.Sprintf("Showing %d results: there are more (use --page, or --all for the complete list)", dataCount(data))
	}

	switch out.EffectiveFormat() {
	case output.FormatStyled:
		body := render.StyledList(toMaps(data), cols, summary)
		writeOutputString(appendHumanSections(body, notice, "", breadcrumbs, false))
		captureResponse()
	case output.FormatMarkdown:
		body := render.MarkdownList(toMaps(data), cols, summary)
		writeOutputString(appendHumanSections(body, notice, "", breadcrumbs, true))
		captureResponse()
	default:
		opts := []output.ResponseOption{output.WithBreadcrumbs(breadcrumbs...)}
		if summary != "" {
			opts = append(opts, output.WithSummary(summary))
		}
		if notice != "" {
			opts = append(opts, output.WithNotice(notice))
		}
		if hasNext || nextURL != "" {
			opts = append(opts, output.WithContext("pagination", map[string]any{
				"has_next": hasNext,
				"next_url": nextURL,
			}))
		}
		recordOutputError(out.OK(data, opts...))
		captureResponse()
	}
}

// printDetail renders a single object with format-aware dispatch.
func printDetail(data any, summary string, breadcrumbs []Breadcrumb) {
	printDetailPaginated(data, summary, breadcrumbs, false, "")
}

// printDetailPaginated renders a single object and includes pagination context when present.
func printDetailPaginated(data any, summary string, breadcrumbs []Breadcrumb, hasNext bool, nextURL string) {
	switch out.EffectiveFormat() {
	case output.FormatStyled:
		body := render.StyledDetail(toMap(data), summary)
		writeOutputString(appendHumanSections(body, "", "", breadcrumbs, false))
		captureResponse()
	case output.FormatMarkdown:
		body := render.MarkdownDetail(toMap(data), summary)
		writeOutputString(appendHumanSections(body, "", "", breadcrumbs, true))
		captureResponse()
	default:
		opts := []output.ResponseOption{output.WithBreadcrumbs(breadcrumbs...)}
		if summary != "" {
			opts = append(opts, output.WithSummary(summary))
		}
		if hasNext || nextURL != "" {
			opts = append(opts, output.WithContext("pagination", map[string]any{
				"has_next": hasNext,
				"next_url": nextURL,
			}))
		}
		recordOutputError(out.OK(data, opts...))
		captureResponse()
	}
}

// printMutation renders a mutation result with format-aware dispatch.
// For styled/markdown, uses summary rendering for simple confirmations.
func printMutation(data any, summary string, breadcrumbs []Breadcrumb) {
	switch out.EffectiveFormat() {
	case output.FormatStyled:
		body := render.StyledSummary(toMap(data), summary)
		writeOutputString(appendHumanSections(body, "", "", breadcrumbs, false))
		captureResponse()
	case output.FormatMarkdown:
		body := render.MarkdownSummary(toMap(data), summary)
		writeOutputString(appendHumanSections(body, "", "", breadcrumbs, true))
		captureResponse()
	default:
		printSuccessWithBreadcrumbs(data, summary, breadcrumbs)
	}
}

func renderHumanData(data any, markdown bool) string {
	switch v := data.(type) {
	case nil:
		if markdown {
			return appendHumanSections(render.MarkdownSummary(nil, ""), "", "", nil, true)
		}
		return appendHumanSections(render.StyledSummary(nil, ""), "", "", nil, false)
	case map[string]any:
		if markdown {
			return appendHumanSections(render.MarkdownDetail(v, ""), "", "", nil, true)
		}
		return appendHumanSections(render.StyledDetail(v, ""), "", "", nil, false)
	}

	if maps := toMaps(data); maps != nil {
		cols := inferColumns(maps)
		if markdown {
			return appendHumanSections(render.MarkdownList(maps, cols, ""), "", "", nil, true)
		}
		return appendHumanSections(render.StyledList(maps, cols, ""), "", "", nil, false)
	}

	if m := toMap(data); m != nil {
		if markdown {
			return appendHumanSections(render.MarkdownDetail(m, ""), "", "", nil, true)
		}
		return appendHumanSections(render.StyledDetail(m, ""), "", "", nil, false)
	}

	value := fmt.Sprintf("%v\n", data)
	return appendHumanSections(value, "", "", nil, markdown)
}

func appendHumanSections(body, notice, location string, breadcrumbs []Breadcrumb, markdown bool) string {
	location = render.SanitizeSingleLine(location)
	body = strings.TrimRight(body, "\n")
	var sb strings.Builder
	if body != "" {
		sb.WriteString(body)
		sb.WriteString("\n")
	}
	if notice != "" {
		sb.WriteString("\n")
		if markdown {
			sb.WriteString("> ")
		}
		sb.WriteString(notice)
		sb.WriteString("\n")
	}
	if location != "" {
		sb.WriteString("\n")
		if markdown {
			sb.WriteString("**Location:** `")
			sb.WriteString(location)
			sb.WriteString("`\n")
		} else {
			sb.WriteString("Location: ")
			sb.WriteString(location)
			sb.WriteString("\n")
		}
	}
	if len(breadcrumbs) > 0 {
		sb.WriteString("\n")
		if markdown {
			sb.WriteString("### Next steps\n")
			for _, crumb := range breadcrumbs {
				sb.WriteString("- `")
				sb.WriteString(crumb.Cmd)
				sb.WriteString("`")
				if crumb.Description != "" {
					sb.WriteString(": ")
					sb.WriteString(crumb.Description)
				}
				sb.WriteString("\n")
			}
		} else {
			sb.WriteString("Next steps:\n")
			for _, crumb := range breadcrumbs {
				sb.WriteString("  ")
				sb.WriteString(crumb.Cmd)
				if crumb.Description != "" {
					sb.WriteString("  # ")
					sb.WriteString(crumb.Description)
				}
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}

func inferColumns(data []map[string]any) render.Columns {
	return render.InferColumns(data)
}

// toMaps converts any (expected []any of map[string]any) to []map[string]any.
// Falls back to JSON round-trip for typed slices (e.g., []Attachment).
func toMaps(data any) []map[string]any {
	if data == nil {
		return nil
	}
	if maps, ok := data.([]map[string]any); ok {
		return maps
	}
	if slice, ok := data.([]any); ok {
		result := make([]map[string]any, 0, len(slice))
		for _, item := range slice {
			if m, ok := item.(map[string]any); ok {
				result = append(result, m)
			}
		}
		return result
	}
	// JSON round-trip for typed structs
	b, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var result []map[string]any
	if json.Unmarshal(b, &result) == nil {
		return result
	}
	return nil
}

// toMap converts any (expected map[string]any) to map[string]any.
// Falls back to JSON round-trip for typed structs.
func toMap(data any) map[string]any {
	if m, ok := data.(map[string]any); ok {
		return m
	}
	// JSON round-trip for typed structs
	b, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) == nil {
		return m
	}
	return nil
}

// printMutationWithLocation renders a mutation result that includes a location URL.
func printMutationWithLocation(data any, location string, breadcrumbs []Breadcrumb) {
	switch out.EffectiveFormat() {
	case output.FormatStyled:
		body := render.StyledDetail(toMap(data), "")
		writeOutputString(appendHumanSections(body, "", location, breadcrumbs, false))
		captureResponse()
	case output.FormatMarkdown:
		body := render.MarkdownDetail(toMap(data), "")
		writeOutputString(appendHumanSections(body, "", location, breadcrumbs, true))
		captureResponse()
	default:
		printSuccessWithLocationAndBreadcrumbs(data, location, breadcrumbs)
	}
}

// printSuccessWithLocationAndBreadcrumbs prints a success response with both location and breadcrumbs.
func printSuccessWithLocationAndBreadcrumbs(data any, location string, breadcrumbs []Breadcrumb) {
	recordOutputError(out.OK(data,
		output.WithBreadcrumbs(breadcrumbs...),
		output.WithContext("location", location),
	))
	captureResponse()
}
