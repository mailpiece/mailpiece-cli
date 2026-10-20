package clitests

import "testing"

func TestConfigShow(t *testing.T) {
	h := newHarness(t)
	result := h.Run("config", "show")
	assertOK(t, result)
	data := result.GetDataMap()
	if data == nil {
		t.Fatal("expected config show to return an object")
	}
	if got := stringifyID(data["account"]); got != h.Account {
		t.Fatalf("expected resolved account %q in config show response, got %q", h.Account, got)
	}
	if stringifyID(data["api_url"]) == "" {
		t.Fatal("expected api_url in config show response")
	}
	if asMap(data["token"]) == nil {
		t.Fatal("expected token in config show response")
	}
}

func TestConfigExplain(t *testing.T) {
	result := newHarness(t).Run("config", "explain")
	assertOK(t, result)
	data := result.GetDataMap()
	if data == nil {
		t.Fatal("expected config explain to return an object")
	}
	if asMap(data["token"]) == nil {
		t.Fatal("expected token explanation in config explain response")
	}
	if asMap(data["account"]) == nil {
		t.Fatal("expected account explanation in config explain response")
	}
	if asMap(data["profile"]) == nil {
		t.Fatal("expected profile explanation in config explain response")
	}
}

func TestDoctor(t *testing.T) {
	result := newHarness(t).Run("doctor")
	assertOK(t, result)
	data := result.GetDataMap()
	if data == nil {
		t.Fatal("expected doctor to return an object")
	}
	checks := asSlice(data["checks"])
	if len(checks) == 0 {
		t.Fatal("expected doctor to report checks")
	}
	foundAuthentication := false
	for _, item := range checks {
		check := asMap(item)
		if check == nil {
			continue
		}
		if stringifyID(check["name"]) == "Authentication" {
			foundAuthentication = true
			break
		}
	}
	if !foundAuthentication {
		t.Fatal("expected doctor checks to include Authentication")
	}
}

func TestCommands(t *testing.T) {
	result := newHarness(t).Run("commands")
	assertOK(t, result)
	if len(result.GetDataArray()) == 0 {
		t.Fatal("expected commands to return at least one command group")
	}
}

func TestVersion(t *testing.T) {
	result := newHarness(t).Run("version")
	assertOK(t, result)
	if result.GetDataString("version") == "" {
		t.Fatal("expected version string in response")
	}
}
