package suggestions

import (
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	t.Run("wait only suggests snapshot", func(t *testing.T) {
		got := Get(Context{Command: "wait"})
		if len(got) != 1 || !strings.Contains(got[0], "snapshot") {
			t.Fatalf("unexpected wait suggestions: %#v", got)
		}
	})
}

func TestGetMatchesSupportedSurface(t *testing.T) {
	t.Run("fill suggests submit without eval tip", func(t *testing.T) {
		snapshot := "RootWebArea \"Login\"\n  uid=1 textbox \"Username\"\n  uid=2 button \"Submit\""
		got := Get(Context{Command: "fill", Snapshot: snapshot})
		if !containsSuggestion(got, "Submit") {
			t.Fatalf("expected submit suggestion, got %#v", got)
		}
		if containsSuggestion(got, "eval <expr>") {
			t.Fatalf("did not expect eval tip, got %#v", got)
		}
	})

	t.Run("open suggests fill and click actions without eval tip", func(t *testing.T) {
		snapshot := "RootWebArea \"Login\"\n  uid=1 textbox \"Username\"\n  uid=2 button \"Sign In\"\n  uid=3 link \"Home\""
		got := Get(Context{Command: "open", Snapshot: snapshot})
		if !containsSuggestion(got, "fill @1") {
			t.Fatalf("expected fill suggestion, got %#v", got)
		}
		if !containsSuggestion(got, "click @2") {
			t.Fatalf("expected button suggestion, got %#v", got)
		}
		if !containsSuggestion(got, "click @3") {
			t.Fatalf("expected link suggestion, got %#v", got)
		}
		if containsSuggestion(got, "eval <expr>") {
			t.Fatalf("did not expect eval tip, got %#v", got)
		}
	})

	t.Run("open suggests actions from playwright refs", func(t *testing.T) {
		snapshot := "- textbox \"Username\" [ref=e1]\n- button \"Sign In\" [ref=e2]\n- link \"Home\" [ref=e3]"
		got := Get(Context{Command: "open", Snapshot: snapshot})
		if !containsSuggestion(got, "fill @e1") {
			t.Fatalf("expected fill suggestion, got %#v", got)
		}
		if !containsSuggestion(got, "click @e2") {
			t.Fatalf("expected button suggestion, got %#v", got)
		}
		if !containsSuggestion(got, "click @e3") {
			t.Fatalf("expected link suggestion, got %#v", got)
		}
		if containsSuggestion(got, "scroll down") {
			t.Fatalf("did not expect scroll tip for small actionable surface, got %#v", got)
		}
	})
}

func containsSuggestion(lines []string, needle string) bool {
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}
