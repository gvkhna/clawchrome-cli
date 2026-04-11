package bridge

import (
	"strings"
	"testing"
	"time"
)

func TestBuildTransportArgsDefaultsToHeadless(t *testing.T) {
	t.Setenv("CLAWCHROME_CLI_HEADED", "")
	t.Setenv("CLAWCHROME_CLI_CHROME_ARGS", "")

	args := BuildTransportArgs()
	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d: %#v", len(args), args)
	}
	if args[0] != "-y" || args[1] != "chrome-devtools-mcp@latest" || args[2] != "--isolated" || args[3] != "--headless" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildTransportArgsForwardsChromeArgs(t *testing.T) {
	t.Setenv("CLAWCHROME_CLI_HEADED", "1")
	t.Setenv("CLAWCHROME_CLI_CHROME_ARGS", "--enable-gpu --ignore-gpu-blocklist")

	args := BuildTransportArgs()
	if args[3] != "--chrome-arg=--enable-gpu" || args[4] != "--chrome-arg=--ignore-gpu-blocklist" {
		t.Fatalf("unexpected chrome args: %#v", args)
	}
}

func TestParseBridgeCallPayload(t *testing.T) {
	name, args, err := ParseBridgeCallPayload([]byte(`{"name":"take_snapshot"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "take_snapshot" {
		t.Fatalf("unexpected name: %s", name)
	}
	if len(args) != 0 {
		t.Fatalf("expected empty args, got %#v", args)
	}
}

func TestCallWaitDuration(t *testing.T) {
	start := time.Now()
	if _, err := callWaitDuration(map[string]any{"milliseconds": 1}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(start) < time.Millisecond {
		t.Fatalf("expected wait_duration to sleep")
	}
}

func TestCallScrollPageRejectsInvalidDirection(t *testing.T) {
	_, err := callScrollPage(nil, map[string]any{"direction": "sideways"})
	if err == nil || !strings.Contains(err.Error(), "invalid scroll_page direction") {
		t.Fatalf("unexpected error: %v", err)
	}
}
