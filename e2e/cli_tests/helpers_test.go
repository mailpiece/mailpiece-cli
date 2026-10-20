package clitests

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/mailpiece/mailpiece-cli/e2e/harness"
)

func newHarness(t *testing.T) *harness.Harness {
	t.Helper()
	return harness.NewWithConfig(t, cfg)
}

// newWriteHarness returns a harness bound to MAILPIECE_TEST_WRITE_ACCOUNT, the
// only one whose Write runs. It skips without that account and fails on the demo.
func newWriteHarness(t *testing.T) *harness.Harness {
	t.Helper()
	return harness.NewWriteHarness(t, cfg)
}

func assertResult(t *testing.T, result *harness.Result, wantExit int) {
	t.Helper()
	if result.ExitCode != wantExit {
		t.Fatalf("expected exit code %d, got %d\nstdout: %s\nstderr: %s", wantExit, result.ExitCode, result.Stdout, result.Stderr)
	}
	if result.Response == nil {
		t.Fatalf("expected JSON response envelope, got none\nstdout: %s\nstderr: %s", result.Stdout, result.Stderr)
	}
	wantOK := wantExit == harness.ExitSuccess
	if result.Response.OK != wantOK {
		t.Fatalf("expected ok=%v, got %v (error: %q)", wantOK, result.Response.OK, result.Response.Error)
	}
}

func assertOK(t *testing.T, result *harness.Result) {
	t.Helper()
	assertResult(t, result, harness.ExitSuccess)
}

// firstListValue runs a list command and returns the first item's value for
// the first of keys it has. It skips when the list is empty or the token's
// role cannot read it.
func firstListValue(t *testing.T, h *harness.Harness, keys []string, args ...string) string {
	t.Helper()
	result := h.Run(args...)
	if result.ExitCode == harness.ExitForbidden {
		t.Skipf("the token's role cannot read %v", args)
	}
	assertOK(t, result)
	for _, item := range result.GetDataArray() {
		if m := asMap(item); m != nil {
			if v := mapValueString(m, keys...); v != "" {
				return v
			}
		}
	}
	t.Skipf("no record listed by %v", args)
	return ""
}

func stringifyID(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.Itoa(int(x))
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func mapValueString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if s := stringifyID(v); s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// currentUserID returns the signed-in user's id on the harness's account,
// from accounts[].user.id in `identity show`.
func currentUserID(t *testing.T, h *harness.Harness) string {
	t.Helper()
	identity := h.Run("identity", "show")
	assertOK(t, identity)
	data := identity.GetDataMap()
	if data == nil {
		t.Skip("identity response had no object payload")
	}

	for _, item := range asSlice(data["accounts"]) {
		account := asMap(item)
		if account == nil {
			continue
		}
		slug := mapValueString(account, "slug")
		if slug != h.Account && slug != "/"+h.Account {
			continue
		}
		if id := mapValueString(asMap(account["user"]), "id"); id != "" {
			return id
		}
	}

	t.Skip("could not determine current account-scoped user ID")
	return ""
}
