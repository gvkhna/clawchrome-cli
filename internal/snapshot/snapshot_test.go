package snapshot

import "testing"

func TestExtractTitle(t *testing.T) {
	title := ExtractTitle(`RootWebArea "Example Domain"`)
	if title != "Example Domain" {
		t.Fatalf("unexpected title: %q", title)
	}
}

func TestTruncateSnapshot(t *testing.T) {
	input := "RootWebArea \"Big\"\n" + `uid=1 link "A"` + "\n" + `uid=2 link "B"`
	result := TruncateSnapshot(input, false, 20)
	if !result.Truncated {
		t.Fatalf("expected truncation")
	}
}

func TestCountRefs(t *testing.T) {
	if CountRefs("uid=1\nuid=2\nuid=3") != 3 {
		t.Fatalf("unexpected ref count")
	}
}

func TestCountRefsSupportsPlaywrightRefs(t *testing.T) {
	input := "- textbox \"Name\" [ref=e217]\n- button \"Commit\" [ref=e218]\n- button \"Commit\" [ref=e218]"
	if CountRefs(input) != 2 {
		t.Fatalf("unexpected ref count")
	}
}

func TestExtractRefsSupportsPlaywrightSnapshot(t *testing.T) {
	input := "- textbox \"Name\" [ref=e217]\n- button \"Commit\" [ref=e218]\n- status [ref=e213]: Waiting"
	got := ExtractRefs(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 refs, got %#v", got)
	}
	if got[0].Ref != "e217" || got[0].Type != "textbox" || got[0].Label != "Name" {
		t.Fatalf("unexpected textbox ref: %#v", got[0])
	}
	if got[1].Ref != "e218" || got[1].Type != "button" || got[1].Label != "Commit" {
		t.Fatalf("unexpected button ref: %#v", got[1])
	}
	if got[2].Ref != "e213" || got[2].Type != "status" || got[2].Label != "Waiting" {
		t.Fatalf("unexpected status ref: %#v", got[2])
	}
}

func TestStripSnapshotHeaderSupportsRuntimeSections(t *testing.T) {
	input := "### Page\n- Page URL: http://example.test\n- Page Title: Example\n### Snapshot\n```yaml\n- generic [active] [ref=e1]:\n  - button \"Go\" [ref=e2]\n```"
	got := StripSnapshotHeader(input)
	want := "- generic [active] [ref=e1]:\n  - button \"Go\" [ref=e2]"
	if got != want {
		t.Fatalf("unexpected stripped snapshot:\n%s", got)
	}
}
