package clitests

import (
	"strings"
	"testing"

	"github.com/mailpiece/mailpiece-cli/e2e/harness"
)

func TestShowURL(t *testing.T) {
	h := newHarness(t)
	id := currentUserID(t, h)

	result := h.Run("show", strings.TrimSuffix(h.APIURL, "/")+"/"+h.Account+"/users/"+id)
	assertOK(t, result)
	if got := mapValueString(result.GetDataMap(), "id"); got != id {
		t.Fatalf("expected user %s, got %q", id, got)
	}
}

func TestShowURLOutsideAnyAccount(t *testing.T) {
	h := newHarness(t)
	result := h.Run("show", strings.TrimSuffix(h.APIURL, "/")+"/my/identity")
	assertOK(t, result)
	if result.GetDataMap()["accounts"] == nil {
		t.Fatalf("expected the identity's accounts, got %s", result.Stdout)
	}
}

func TestShowURLNotFound(t *testing.T) {
	h := newHarness(t)
	assertResult(t, h.Run("show", strings.TrimSuffix(h.APIURL, "/")+"/"+h.Account+"/users/0000000000000000000000000"), harness.ExitNotFound)
}

func TestAPIGet(t *testing.T) {
	h := newHarness(t)
	result := h.Run("api", "get", "users/"+currentUserID(t, h)+".json")
	assertOK(t, result)
	if result.GetDataMap() == nil {
		t.Fatalf("expected a user, got %s", result.Stdout)
	}
}

func TestAPIGetRefusesAForeignHost(t *testing.T) {
	h := newHarness(t)
	assertResult(t, h.Run("api", "get", "https://evil.example/"+h.Account+"/posts.json"), harness.ExitUsage)
}
