package commands

import (
	"errors"
	"net"

	"github.com/basecamp/cli/output"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// convertSDKError converts an SDK error to a CLI output.Error.
// Returns nil for nil errors. Non-SDK errors pass through unchanged.
func convertSDKError(err error) error {
	if err == nil {
		return nil
	}

	var sdkErr *mailpiece.Error
	if errors.As(err, &sdkErr) {
		e := &output.Error{
			Code:       mapSDKCode(sdkErr.Code),
			Message:    sdkErr.Message,
			Hint:       sdkErr.Hint,
			HTTPStatus: sdkErr.HTTPStatus,
			Retryable:  sdkErr.Retryable,
		}
		// Add mailpiece-specific hint for auth errors
		if sdkErr.Code == mailpiece.CodeAuth && e.Hint == "" {
			e.Hint = "Run 'mailpiece auth login TOKEN' or set MAILPIECE_TOKEN"
		}
		return e
	}

	// Catch raw network errors that weren't wrapped by the SDK
	var netErr net.Error
	if errors.As(err, &netErr) {
		return &output.Error{
			Code:      output.CodeNetwork,
			Message:   netErr.Error(),
			Hint:      "Check your internet connection",
			Retryable: true,
		}
	}

	return err
}

// mapSDKCode maps SDK error codes to output package codes.
// Codes that exist in both packages pass through; SDK-only codes
// (e.g. "validation") are mapped to the closest output equivalent.
func mapSDKCode(code string) string {
	switch code {
	case mailpiece.CodeValidation:
		return output.CodeAPI // output package has no "validation" code
	default:
		return code
	}
}
