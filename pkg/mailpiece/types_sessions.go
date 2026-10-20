package mailpiece

// SessionStartResult is what SessionsService.Start returns: the token to
// carry into Verify, and, in development, the code the app would otherwise
// only send by email (sessions/magic_links_controller.rb's dev-only header).
type SessionStartResult struct {
	PendingAuthenticationToken string
	// DevCode is the magic link code from X-Magic-Link-Code, or "" outside development.
	DevCode string
}

// SessionVerifyResult is what SessionsService.Verify returns
// (sessions/magic_links_controller.rb#create's JSON body plus its session_token cookie).
type SessionVerifyResult struct {
	SessionToken string //nolint:gosec // G117: the session token is the result this type exists to return
	// RequiresSignupCompletion is true when the identity is new and must finish
	// signing up (CompleteSignup) before it reaches any account.
	RequiresSignupCompletion bool
}
