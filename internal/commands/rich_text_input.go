package commands

import (
	"context"
	"fmt"
	"html"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/basecamp/cli/editor"
	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// maxStdinText caps what piped stdin may give a rich text; more is refused, never cut.
const maxStdinText = 10 << 20

var (
	stdinIsTerminal  = func() bool { return isTerminal(os.Stdin) }
	stdoutIsTerminal = func() bool { return isTerminal(os.Stdout) }
	openEditor       = editor.Open
)

// stdinIsPiped reports whether r is a pipe or a file redirected in. A terminal or /dev/null
// is not, so a command run with nothing piped never waits on it.
func stdinIsPiped(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return r != nil
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeNamedPipe != 0 || info.Mode().IsRegular()
}

// richTextFromInput is richTextFromFlags, then, when implicit and neither flag was passed,
// piped stdin, then $EDITOR when stdin and stdout are a terminal. The editor starts from
// what current returns (nil starts it empty). Empty stdin gives nothing.
func richTextFromInput(cmd *cobra.Command, name, value, file string, implicit bool, current func() (string, error)) (string, bool, error) {
	text, given, err := richTextFromFlags(cmd, name, value, file)
	if given || err != nil || !implicit {
		return text, given, err
	}

	if in := cmd.InOrStdin(); stdinIsPiped(in) {
		data, readErr := io.ReadAll(io.LimitReader(in, maxStdinText+1))
		if readErr != nil {
			return "", false, errors.NewInvalidArgsError(fmt.Sprintf("Cannot read --%s from stdin: %v", name, readErr))
		}
		if len(data) > maxStdinText {
			return "", false, errors.NewInvalidArgsError(fmt.Sprintf("stdin for --%s is over %d bytes", name, maxStdinText))
		}
		if strings.TrimSpace(string(data)) == "" {
			return "", false, nil
		}
		return markdownToHTML(string(data)), true, nil
	}

	if !stdinIsTerminal() || !stdoutIsTerminal() {
		return "", false, nil
	}
	initial := ""
	if current != nil {
		if initial, err = current(); err != nil {
			return "", false, err
		}
	}
	edited, err := openEditor(initial)
	if err != nil {
		return "", false, errors.NewInvalidArgsError(fmt.Sprintf("No --%s from $EDITOR: %v", name, err))
	}
	return markdownToHTML(edited), true, nil
}

// usageWithHint is a usage error that shows how the command is meant to be given its text.
func usageWithHint(message, hint string) error {
	e := errors.NewInvalidArgsError(message)
	e.Hint = hint
	return e
}

// noFlagChanged reports whether none of names was passed: an update given nothing else
// reads its text from stdin or the editor.
func noFlagChanged(cmd *cobra.Command, names ...string) bool {
	for _, name := range names {
		if cmd.Flags().Changed(name) {
			return false
		}
	}
	return true
}

// readAttachments reads every --attach file before anything is sent, so a missing one
// refuses the command whole.
func readAttachments(paths []string) ([]attachmentFile, error) {
	files := make([]attachmentFile, 0, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			return nil, errors.NewInvalidArgsError("--attach needs a file")
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, errors.NewInvalidArgsError(fmt.Sprintf("Cannot read --attach %s: %v", path, err))
		}
		if info.IsDir() {
			return nil, errors.NewInvalidArgsError(fmt.Sprintf("--attach %s is a directory", path))
		}
		data, err := os.ReadFile(path) // #nosec G304 -- the caller names the file to attach
		if err != nil {
			return nil, errors.NewInvalidArgsError(fmt.Sprintf("Cannot read --attach %s: %v", path, err))
		}
		files = append(files, attachmentFile{path: path, data: data})
	}
	return files, nil
}

type attachmentFile struct {
	path string
	data []byte
}

// appendAttachments uploads each file by direct upload and appends it to body as the
// editor embeds one.
func appendAttachments(ctx context.Context, body string, files []attachmentFile) (string, error) {
	if len(files) == 0 {
		return body, nil
	}
	uploads := mailpiece.NewUploadsService(getSDK())
	tags := make([]string, 0, len(files))
	for _, file := range files {
		blob, err := uploads.Upload(ctx, file.path, file.data)
		if err != nil {
			return "", convertSDKError(err)
		}
		tags = append(tags, attachmentTag(blob))
	}
	attachments := strings.Join(tags, "\n")
	switch {
	case body == "":
		return attachments, nil
	case strings.HasSuffix(body, "\n"):
		return body + attachments, nil
	}
	return body + "\n" + attachments, nil
}

// attachmentTag is the <action-text-attachment> the editor writes for an uploaded file; the
// app fills in its url and dimensions from the blob when it renders it.
func attachmentTag(blob *mailpiece.Blob) string {
	return `<action-text-attachment sgid="` + html.EscapeString(blob.AttachableSGID) +
		`" content-type="` + html.EscapeString(blob.ContentType) +
		`" filename="` + html.EscapeString(blob.Filename) +
		`" filesize="` + strconv.FormatInt(blob.ByteSize, 10) +
		`" presentation="gallery"></action-text-attachment>`
}
