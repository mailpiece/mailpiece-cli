package harness

import (
	"fmt"
	"strings"
)

// CleanupTracker holds the commands that undo what a test wrote: each entry
// is a whole command, such as "token delete ID".
type CleanupTracker struct {
	Commands [][]string
}

// NewCleanupTracker creates a new cleanup tracker.
func NewCleanupTracker() *CleanupTracker {
	return &CleanupTracker{Commands: make([][]string, 0)}
}

// Add records a command to run at cleanup.
func (c *CleanupTracker) Add(args ...string) {
	c.Commands = append(c.Commands, args)
}

// CleanupAll runs the recorded commands in reverse order through the write
// harness. A command answered with not found counts as done.
func (c *CleanupTracker) CleanupAll(h *Harness) []error {
	var errors []error
	for i := len(c.Commands) - 1; i >= 0; i-- {
		args := c.Commands[i]
		result := h.Write(args...)
		if result.ExitCode != ExitSuccess && result.ExitCode != ExitNotFound {
			errors = append(errors, fmt.Errorf("%s: exit %d", strings.Join(args, " "), result.ExitCode))
		}
	}
	c.Commands = c.Commands[:0]
	return errors
}
