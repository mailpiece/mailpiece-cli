package commands

import (
	"testing"
)

func TestParseAppURL(t *testing.T) {
	tests := []struct {
		input   string
		account string
		path    string
	}{
		{"https://mailpiece.example.com/834206594/posts/abc", "834206594", "/posts/abc.json"},
		{"https://mailpiece.example.com/834206594/posts/abc/", "834206594", "/posts/abc.json"},
		{"https://mailpiece.example.com/834206594/posts/p7#events", "834206594", "/posts/p7.json"},
		{"https://mailpiece.example.com/834206594/subscribers?page=2", "834206594", "/subscribers.json?page=2"},
		{"https://mailpiece.example.com/834206594/posts/abc.json", "834206594", "/posts/abc.json"},
		{"HTTPS://mailpiece.example.com/834206594/posts/p1", "834206594", "/posts/p1.json"},
		{"http://app.mailpiece.localhost:3020/834206594/posts/p1", "834206594", "/posts/p1.json"},
		// An account answers at its handle too.
		{"https://mailpiece.example.com/acme-news/posts/abc", "acme-news", "/posts/abc.json"},
		// A page outside any account.
		{"https://mailpiece.example.com/my/identity", "", "/my/identity.json"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseAppURL(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Account != tt.account || got.Path != tt.path {
				t.Errorf("got %+v, want account %q path %q", got, tt.account, tt.path)
			}
		})
	}
}

func TestParseAppURLRefusals(t *testing.T) {
	for _, input := range []string{
		"posts/abc",
		"/834206594/posts/abc",
		"ftp://mailpiece.example.com/834206594/posts/abc",
		"https://mailpiece.example.com",
		"https://mailpiece.example.com/",
		"https://mailpiece.example.com/834206594",
		"https://mailpiece.example.com/834206594/",
	} {
		t.Run(input, func(t *testing.T) {
			if got, err := parseAppURL(input); err == nil {
				t.Errorf("expected a refusal, got %+v", got)
			}
		})
	}
}
