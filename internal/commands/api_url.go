package commands

import (
	"strings"

	"github.com/basecamp/cli/output"
)

// noAPIURLHint is the one way out of a missing address: Mailpiece is self-hosted,
// so there is no address to fall back to.
const noAPIURLHint = "Pass --api-url https://your-install, set MAILPIECE_API_URL, or run 'mailpiece setup'"

func errNoAPIURL() error {
	return output.ErrUsageHint("No Mailpiece address configured", noAPIURLHint)
}

// requireAPIURL refuses a command that reaches Mailpiece while no address is set.
func requireAPIURL(apiURL string) error {
	if strings.TrimSpace(apiURL) == "" {
		return errNoAPIURL()
	}
	return nil
}
