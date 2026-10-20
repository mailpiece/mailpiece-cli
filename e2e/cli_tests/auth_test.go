package clitests

import (
	"testing"

	"github.com/mailpiece/mailpiece-cli/e2e/harness"
)

func TestAuthInvalidToken(t *testing.T) {
	badCfg := *cfg
	badCfg.Token = "mailpiece_invalid_token"
	h := harness.NewWithConfig(t, &badCfg)
	assertResult(t, h.Run("identity", "show"), harness.ExitAuth)
}

func TestAuthMissingToken(t *testing.T) {
	missingCfg := *cfg
	missingCfg.Token = ""
	h := harness.NewWithConfig(t, &missingCfg)
	assertResult(t, h.RunWithEnv(map[string]string{
		"MAILPIECE_TOKEN": "",
	}, "identity", "show"), harness.ExitAuth)
}
