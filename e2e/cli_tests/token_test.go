package clitests

import (
	"strconv"
	"testing"
	"time"

	"github.com/mailpiece/mailpiece-cli/e2e/harness"
)

func TestAccessTokenCRUD(t *testing.T) {
	h := newWriteHarness(t)
	description := "CLI Test Token " + strconv.FormatInt(time.Now().UnixNano(), 10)

	create := h.Write("token", "create", "--description", description, "--permission", "read")
	assertOK(t, create)
	tokenID := create.GetDataString("id")
	if tokenID == "" {
		t.Fatal("no token ID in create response")
	}
	h.Cleanup.Add("token", "delete", tokenID)
	rawToken := create.GetDataString("token")
	if rawToken == "" {
		t.Fatal("expected raw token value in create response")
	}
	if got := create.GetDataString("permission"); got != "read" {
		t.Fatalf("expected permission %q, got %q", "read", got)
	}
	if got := create.GetDataString("description"); got != description {
		t.Fatalf("expected description %q, got %q", description, got)
	}

	list := h.Run("token", "list")
	assertOK(t, list)
	found := false
	for _, item := range list.GetDataArray() {
		if mapValueString(asMap(item), "id") == tokenID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected token list to include %q", tokenID)
	}

	tokenCfg := *cfg
	tokenCfg.Token = rawToken
	assertOK(t, harness.NewWithConfig(t, &tokenCfg).Run("identity", "show"))

	deleteResult := h.Write("token", "delete", tokenID)
	assertOK(t, deleteResult)
	if !deleteResult.GetDataBool("deleted") {
		t.Fatal("expected deleted=true")
	}
	if got := deleteResult.GetDataString("id"); got != tokenID {
		t.Fatalf("expected deleted id %q, got %q", tokenID, got)
	}

	assertResult(t, harness.NewWithConfig(t, &tokenCfg).Run("identity", "show"), harness.ExitAuth)
}

func TestAccessTokenCreateRejectsUnknownPermission(t *testing.T) {
	h := newWriteHarness(t)
	assertResult(t, h.Write("token", "create", "--description", "CLI Test Token", "--permission", "admin"), harness.ExitUsage)
}
