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
