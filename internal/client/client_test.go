package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTPTransportUsesBearerAndSessionHeaders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
	t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "secret-token")

	var healthSession string
	var callSession string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		switch r.URL.Path {
		case "/health":
			healthSession = r.Header.Get(sessionHeaderName)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		case "/call":
			callSession = r.Header.Get(sessionHeaderName)
			_ = json.NewEncoder(w).Encode(map[string]any{"result": "ok"})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("CLAWCHROME_CLI_HTTP_URL", server.URL)

	port, err := EnsureBridge()
	if err != nil {
		t.Fatalf("EnsureBridge failed: %v", err)
	}
	if port == 0 {
		t.Fatalf("expected parsed port from server URL")
	}

	result, err := CallTool("take_snapshot", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result != "ok" {
		t.Fatalf("unexpected result: %q", result)
	}
	if healthSession == "" || callSession == "" {
		t.Fatalf("expected session headers to be sent")
	}
	if healthSession != callSession {
		t.Fatalf("expected stable session header, got health=%q call=%q", healthSession, callSession)
	}

	state, ok := readState()
	if !ok {
		t.Fatalf("expected session state to be written")
	}
	if state.Transport != transportHTTP || state.BaseURL != server.URL {
		t.Fatalf("unexpected state: %#v", state)
	}
	if state.SessionID != callSession {
		t.Fatalf("expected stored session id to match request header")
	}
}

func TestStopBridgeClearsHTTPState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	state := sessionState{
		SessionID: "session-1",
		Transport: transportHTTP,
		BaseURL:   "https://example.com",
		Port:      443,
	}
	if err := writeState(state); err != nil {
		t.Fatalf("writeState failed: %v", err)
	}

	if !StopBridge() {
		t.Fatalf("expected stop to clear http session")
	}
	if _, ok := readState(); ok {
		t.Fatalf("expected state file to be removed")
	}
	if _, err := os.Stat(filepath.Join(home, defaultStateDirName, defaultSessionStateName)); !os.IsNotExist(err) {
		t.Fatalf("expected session file removal, err=%v", err)
	}
}

func TestGetSessionSnapshotIfRunningUsesExistingHTTPSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
	t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "secret-token")

	sessionID := "existing-session"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if got := r.Header.Get(sessionHeaderName); got != sessionID {
			t.Fatalf("unexpected session header: %q", got)
		}
		switch r.URL.Path {
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		case "/call":
			_ = json.NewEncoder(w).Encode(map[string]any{"result": "snapshot"})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("CLAWCHROME_CLI_HTTP_URL", server.URL)

	if err := writeState(sessionState{
		SessionID: sessionID,
		Transport: transportHTTP,
		BaseURL:   server.URL,
		Port:      parsePortFromURL(server.URL),
	}); err != nil {
		t.Fatalf("writeState failed: %v", err)
	}

	snapshot, ok := GetSessionSnapshotIfRunning()
	if !ok {
		t.Fatalf("expected snapshot to be available")
	}
	if snapshot != "snapshot" {
		t.Fatalf("unexpected snapshot: %q", snapshot)
	}
}
