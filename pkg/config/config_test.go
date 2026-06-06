package config

import (
	"os"
	"testing"
)

func TestResolveAPIKeyFallback(t *testing.T) {
	// Force load settings.json.
	ccEnv = nil
	loadCCSettings()

	if len(ccEnv) == 0 {
		t.Skip("no ~/.claude/settings.json found, skipping fallback test")
	}

	// Pick a key from settings.json env that we can test against.
	for k, v := range ccEnv {
		if v == "" {
			continue
		}

		raw := "${" + k + "}"

		// 1. Test fallback: unset OS env, should use settings.json value.
		os.Unsetenv(k)
		got := ResolveAPIKey(raw)
		if got != v {
			t.Errorf("fallback for %s: expected %q, got %q", k, v, got)
		} else {
			t.Logf("fallback for %s OK: resolved from settings.json", k)
		}

		// 2. Test OS env takes priority.
		os.Setenv(k, "os-override-value")
		got2 := ResolveAPIKey(raw)
		if got2 != "os-override-value" {
			t.Errorf("OS override for %s: expected 'os-override-value', got %q", k, got2)
		} else {
			t.Logf("OS override for %s OK", k)
		}
		os.Unsetenv(k)

		// 3. Test no ${} pattern — pass through.
		plain := ResolveAPIKey("sk-plain-key")
		if plain != "sk-plain-key" {
			t.Errorf("plain key: expected 'sk-plain-key', got %q", plain)
		}

		break // one key is enough
	}
}
