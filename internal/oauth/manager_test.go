package oauth

import "testing"

func TestOuraScopesContainsAllRequired(t *testing.T) {
	required := []string{
		"email", "personal", "daily", "session",
		"heartrate", "workout", "tag", "spo2",
		"stress", "heart_health", "ring_configuration",
	}
	have := make(map[string]bool)
	for _, s := range ouraScopes {
		have[s] = true
	}
	for _, r := range required {
		if !have[r] {
			t.Errorf("missing required scope: %q", r)
		}
	}
}
