package agent

import "testing"

func TestReasoningProfilesIncludeUltraDefault(t *testing.T) {
	profile, ok := ReasoningProfileFor(DefaultReasoningLevel)
	if !ok {
		t.Fatal("default reasoning level should have a profile")
	}
	if profile.ID != ReasoningUltra {
		t.Fatalf("default reasoning = %s, want %s", profile.ID, ReasoningUltra)
	}
	if profile.Budget.MaxToolCalls != 32 {
		t.Fatalf("ultra max tool calls = %d, want 32", profile.Budget.MaxToolCalls)
	}
}

func TestParseReasoningLevelRejectsUnknown(t *testing.T) {
	if _, err := ParseReasoningLevel("extreme"); err == nil {
		t.Fatal("expected unknown reasoning level error")
	}
}
