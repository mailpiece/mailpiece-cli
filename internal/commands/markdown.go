package commands

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(html.WithUnsafe()),
)

// backtickAttachmentRegex matches backtick-wrapped action-text-attachment tags
// that goldmark didn't convert (because they were inside HTML blocks).
var backtickAttachmentRegex = regexp.MustCompile("`(<action-text-attachment[^>]*>)(</action-text-attachment>)`")
var blockMarkdownRegex = regexp.MustCompile("(?m)^(#{1,6}\\s|[*+-]\\s|\\d+\\.\\s|>\\s|```|~~~)")
var inlineMarkdownRegex = regexp.MustCompile(`(\*\*[^*]+\*\*|__[^_]+__|~~[^~]+~~|\[[^\]]+\]\([^)]+\)|!\[[^\]]*\]\([^)]+\)|(^|[[:space:][:punct:]])[*_][^*_\n]+[*_]([[:space:][:punct:]]|$))`)

// containsMarkdownOrHTML checks whether content has HTML tags or markdown
// syntax that would benefit from conversion.
func containsMarkdownOrHTML(content string) bool {
	if strings.ContainsAny(content, "<>`") {
		return true
	}
	return blockMarkdownRegex.MatchString(content) || inlineMarkdownRegex.MatchString(content)
}

// markdownToHTML converts markdown content to HTML. Raw HTML in the input is
// passed through unchanged, while markdown syntax (e.g. backtick code spans)
// is converted to proper HTML with entities escaped. This prevents content
// inside markdown code spans (like `<action-text-attachment>`) from being
// parsed as real HTML by Action Text.
//
// Plain text without any HTML or markdown syntax is returned unchanged to
// avoid wrapping simple text in unnecessary <p> tags.
//
// For mixed HTML/markdown content where goldmark treats backtick-wrapped tags
// as raw HTML, we also escape any action-text-attachment tags that remain
// wrapped in literal backticks after conversion.
func markdownToHTML(content string) string {
	if !containsMarkdownOrHTML(content) {
		return content
	}

	var buf bytes.Buffer
	if err := md.Convert([]byte(content), &buf); err != nil {
		return content
	}
	result := buf.String()

	// Handle backtick-wrapped attachment tags that goldmark didn't convert
	// because they were inside HTML blocks. Replace them with escaped versions
	// inside <code> tags.
	result = backtickAttachmentRegex.ReplaceAllStringFunc(result, func(match string) string {
		// Strip the backticks and escape the HTML
		inner := match[1 : len(match)-1]
		escaped := strings.ReplaceAll(inner, "<", "&lt;")
		escaped = strings.ReplaceAll(escaped, ">", "&gt;")
		escaped = strings.ReplaceAll(escaped, "\"", "&quot;")
		return "<code>" + escaped + "</code>"
	})

	return result
}

// registerRichTextFlags adds --name, with its one-letter short form when short is not empty,
// and --name-file: a text given as Markdown or HTML, on the command line, in a file, or on stdin.
func registerRichTextFlags(cmd *cobra.Command, value, file *string, name, short, usage string) {
	cmd.Flags().StringVarP(value, name, short, "", usage)
	cmd.Flags().StringVar(file, name+"-file", "", "Read --"+name+" from a file, or from stdin with -")
}

// richTextFromFlags is the text --name or --name-file gives, as the HTML the app stores,
// and whether either flag was passed. A file named "-" is stdin.
func richTextFromFlags(cmd *cobra.Command, name, value, file string) (string, bool, error) {
	fileFlag := name + "-file"
	given, fileGiven := cmd.Flags().Changed(name), cmd.Flags().Changed(fileFlag)
	switch {
	case given && fileGiven:
		return "", false, errors.NewInvalidArgsError(fmt.Sprintf("--%s and --%s cannot be used together", name, fileFlag))
	case fileGiven:
		var data []byte
		var err error
		if file == "-" {
			data, err = io.ReadAll(cmd.InOrStdin())
		} else {
			data, err = os.ReadFile(file) // #nosec G304 -- the caller names the file to send
		}
		if err != nil {
			return "", false, errors.NewInvalidArgsError(fmt.Sprintf("Cannot read --%s: %v", fileFlag, err))
		}
		return markdownToHTML(string(data)), true, nil
	case given:
		return markdownToHTML(value), true, nil
	}
	return "", false, nil
}
