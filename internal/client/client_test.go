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

func TestCallRuntimeHTTPToolUsesDedicatedToolEndpoint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
	t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "secret-token")

	var toolSession string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		switch r.URL.Path {
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		case "/api/tools/browser_mouse_move_xy":
			toolSession = r.Header.Get(sessionHeaderName)
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode tool body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":      true,
				"action":  "mouse_move_xy",
				"backend": "runtime-core",
				"message": "mouse moved",
				"data":    map[string]any{"x": 10, "y": 20},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("CLAWCHROME_CLI_HTTP_URL", server.URL)

	result, err := CallRuntimeHTTPTool("browser_mouse_move_xy", map[string]any{"x": 10, "y": 20})
	if err != nil {
		t.Fatalf("CallRuntimeHTTPTool failed: %v", err)
	}
	if result.Action != "mouse_move_xy" || result.Backend != "runtime-core" || result.Message != "mouse moved" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if toolSession == "" {
		t.Fatalf("expected session header on tool request")
	}
	if gotBody["x"] != float64(10) || gotBody["y"] != float64(20) {
		t.Fatalf("unexpected tool body: %#v", gotBody)
	}
}

func TestCallRuntimeHTTPToolRequiresHTTPTransportWithoutStartingBridge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAWCHROME_CLI_TRANSPORT", "stdio")

	if _, err := CallRuntimeHTTPTool("browser_mouse_move_xy", map[string]any{"x": 10, "y": 20}); err == nil {
		t.Fatalf("expected validation error")
	} else if cdpErr, ok := err.(*CdpError); !ok || cdpErr.Code != ErrValidation {
		t.Fatalf("expected validation CdpError, got %#v", err)
	}
}

func TestHTTPTransportDefaultsToProductionURL(t *testing.T) {
	t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
	t.Setenv("CLAWCHROME_CLI_HTTP_URL", "")
	t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "secret-token")

	cfg, err := loadTransportConfig()
	if err != nil {
		t.Fatalf("loadTransportConfig failed: %v", err)
	}
	if cfg.baseURL != defaultRuntimeHTTPURL {
		t.Fatalf("expected default runtime URL %q, got %q", defaultRuntimeHTTPURL, cfg.baseURL)
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
