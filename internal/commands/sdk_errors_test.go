package commands

import (
	stderrors "errors"
	"net"
	"testing"

	"github.com/basecamp/cli/output"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

func TestConvertSDKErrorNil(t *testing.T) {
	if err := convertSDKError(nil); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestConvertSDKErrorMapsSDKErrors(t *testing.T) {
	tests := []struct {
		name     string
		sdkErr   *mailpiece.Error
		wantCode string
		wantExit int
		wantHint string
	}{
		{name: "not found", sdkErr: &mailpiece.Error{Code: mailpiece.CodeNotFound, Message: "Not found", HTTPStatus: 404}, wantCode: output.CodeNotFound, wantExit: 2},
		{name: "auth gets the login hint", sdkErr: &mailpiece.Error{Code: mailpiece.CodeAuth, Message: "Unauthorized", HTTPStatus: 401}, wantCode: output.CodeAuth, wantExit: 3, wantHint: "Run 'mailpiece auth login TOKEN' or set MAILPIECE_TOKEN"},
		{name: "auth keeps its own hint", sdkErr: &mailpiece.Error{Code: mailpiece.CodeAuth, Message: "Unauthorized", Hint: "A read token cannot write", HTTPStatus: 401}, wantCode: output.CodeAuth, wantExit: 3, wantHint: "A read token cannot write"},
		{name: "forbidden", sdkErr: &mailpiece.Error{Code: mailpiece.CodeForbidden, Message: "Forbidden", HTTPStatus: 403}, wantCode: output.CodeForbidden, wantExit: 4},
		{name: "validation is an API error", sdkErr: &mailpiece.Error{Code: mailpiece.CodeValidation, Message: "Name can't be blank", HTTPStatus: 422}, wantCode: output.CodeAPI, wantExit: 7},
		{name: "rate limit", sdkErr: &mailpiece.Error{Code: mailpiece.CodeRateLimit, Message: "Slow down", HTTPStatus: 429, Retryable: true}, wantCode: output.CodeRateLimit, wantExit: 5},
		{name: "ambiguous", sdkErr: &mailpiece.Error{Code: mailpiece.CodeAmbiguous, Message: "Several"}, wantCode: output.CodeAmbiguous, wantExit: 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := convertSDKError(tt.sdkErr)
			var e *output.Error
			if !stderrors.As(err, &e) {
				t.Fatalf("expected *output.Error, got %T", err)
			}
			if e.Code != tt.wantCode {
				t.Errorf("code: want %q, got %q", tt.wantCode, e.Code)
			}
			if e.Message != tt.sdkErr.Message {
				t.Errorf("message: want %q, got %q", tt.sdkErr.Message, e.Message)
			}
			if e.HTTPStatus != tt.sdkErr.HTTPStatus || e.Retryable != tt.sdkErr.Retryable {
				t.Errorf("status and retryable not carried over: %#v", e)
			}
			if e.Hint != tt.wantHint {
				t.Errorf("hint: want %q, got %q", tt.wantHint, e.Hint)
			}
			assertExitCode(t, err, tt.wantExit)
		})
	}
}

func TestConvertSDKErrorWrapsNetworkErrors(t *testing.T) {
	netErr := &net.OpError{Op: "dial", Net: "tcp", Err: stderrors.New("connection refused")}

	err := convertSDKError(netErr)
	var e *output.Error
	if !stderrors.As(err, &e) {
		t.Fatalf("expected *output.Error, got %T", err)
	}
	if e.Code != output.CodeNetwork || !e.Retryable || e.Hint != "Check your internet connection" {
		t.Fatalf("expected a retryable network error, got %#v", e)
	}
	assertExitCode(t, err, 6)
}

func TestConvertSDKErrorPassesOtherErrorsThrough(t *testing.T) {
	plain := stderrors.New("something else")
	if err := convertSDKError(plain); err != plain {
		t.Fatalf("expected the same error, got %v", err)
	}
}

func TestMapSDKCode(t *testing.T) {
	tests := map[string]string{
		mailpiece.CodeValidation: output.CodeAPI,
		mailpiece.CodeNotFound:   output.CodeNotFound,
		mailpiece.CodeAuth:       output.CodeAuth,
		mailpiece.CodeNetwork:    output.CodeNetwork,
		mailpiece.CodeUsage:      output.CodeUsage,
	}
	for in, want := range tests {
		if got := mapSDKCode(in); got != want {
			t.Errorf("mapSDKCode(%q) = %q, want %q", in, got, want)
		}
	}
}
