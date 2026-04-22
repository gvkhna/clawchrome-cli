package client

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const DefaultPort = 9224

type ErrorCode string

const (
	ErrBridgeNotReady ErrorCode = "BRIDGE_NOT_READY"
	ErrRefNotFound    ErrorCode = "REF_NOT_FOUND"
	ErrTimeout        ErrorCode = "TIMEOUT"
	ErrBrowser        ErrorCode = "BROWSER_ERROR"
	ErrValidation     ErrorCode = "VALIDATION_ERROR"
	ErrUnknown        ErrorCode = "UNKNOWN"
)

const (
	transportStdio            = "stdio"
	transportHTTP             = "http"
	sessionHeaderName         = "X-Clawchrome-Session"
	defaultRemoteHealthPath   = "/health"
	defaultRemoteCallPath     = "/call"
	defaultRemoteToolsPath    = "/tools"
	defaultRemoteAuthHeader   = "Authorization"
	defaultStateDirName       = ".clawchrome-cli"
	defaultSessionStateName   = "session.json"
	defaultBridgePIDFileName  = "bridge.pid"
	defaultHealthTimeout      = 2 * time.Second
	defaultCallTimeout        = 2 * time.Minute
	defaultSnapshotTimeout    = 5 * time.Second
	defaultBridgeStartTimeout = 30 * time.Second
)

type CdpError struct {
	Message     string
	Code        ErrorCode
	Suggestions []string
}

type sessionState struct {
	SessionID string `json:"session_id"`
	Transport string `json:"transport"`
	BaseURL   string `json:"base_url,omitempty"`
	PID       int    `json:"pid,omitempty"`
	Port      int    `json:"port,omitempty"`
}

type transportConfig struct {
	mode        string
	baseURL     string
	bearerToken string
}

func (e *CdpError) Error() string { return e.Message }

func EnsureBridge() (int, error) {
	cfg, err := loadTransportConfig()
	if err != nil {
		return 0, err
	}

	switch cfg.mode {
	case transportHTTP:
		state, err := ensureHTTPSession(cfg)
		if err != nil {
			return 0, err
		}
		return state.Port, nil
	case transportStdio:
		state, err := ensureStdioSession()
		if err != nil {
			return 0, err
		}
		return state.Port, nil
	default:
		return 0, WrapError("Unsupported transport mode: "+cfg.mode, ErrValidation)
	}
}

func UsesHTTPTransport() bool {
	cfg, err := loadTransportConfig()
	return err == nil && cfg.mode == transportHTTP
}

func CallTool(name string, args map[string]any) (string, error) {
	state, cfg, err := ensureSession()
	if err != nil {
		return "", err
	}

	payload, err := postJSON(state, cfg, defaultRemoteCallPath, map[string]any{
		"name": name,
		"args": args,
	}, defaultCallTimeout)
	if err != nil {
		return "", mapError(err.Error())
	}

	var response struct {
		Result string `json:"result"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return "", err
	}
	if response.Error != "" {
		return "", mapError(response.Error)
	}
	return response.Result, nil
}

func GetSessionSnapshotIfRunning() (string, bool) {
	state, cfg, ok := currentSession()
	if !ok {
		return "", false
	}
	healthy, _ := checkBridgeHealth(state, cfg)
	if !healthy {
		return "", false
	}
	payload, err := postJSON(state, cfg, defaultRemoteCallPath, map[string]any{
		"name": "take_snapshot",
		"args": map[string]any{},
	}, defaultSnapshotTimeout)
	if err != nil {
		return "", false
	}
	var response struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return "", false
	}
	return response.Result, response.Result != ""
}

func StopBridge() bool {
	state, ok := readState()
	if !ok {
		if pid, _, ok := readPIDFile(); ok && isProcessAlive(pid) {
			_ = terminateProcess(pid)
			return true
		}
		return false
	}

	defer removeState()
	if state.Transport == transportHTTP {
		return true
	}
	if state.PID <= 0 || !isProcessAlive(state.PID) {
		return false
	}
	_ = terminateProcess(state.PID)
	return true
}

func WrapError(message string, code ErrorCode, suggestions ...string) error {
	return &CdpError{Message: message, Code: code, Suggestions: suggestions}
}

func mapError(message string) error {
	switch {
	case bytes.Contains([]byte(message), []byte("ECONNREFUSED")), bytes.Contains([]byte(message), []byte("ECONNRESET")):
		return &CdpError{Message: "Bridge is not running", Code: ErrBridgeNotReady, Suggestions: []string{"Run `clawchrome-cli open <url>` to start the bridge automatically"}}
	case bytes.Contains([]byte(message), []byte("uid")) || bytes.Contains([]byte(message), []byte("element")):
		return &CdpError{Message: message, Code: ErrRefNotFound, Suggestions: []string{"Run `clawchrome-cli snapshot` to see available @uid refs"}}
	case bytes.Contains([]byte(message), []byte("timeout")):
		return &CdpError{Message: message, Code: ErrTimeout, Suggestions: []string{"Run `clawchrome-cli snapshot` to see current page state"}}
	default:
		var payload struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(message), &payload) == nil && payload.Error != "" {
			return &CdpError{Message: payload.Error, Code: ErrBrowser, Suggestions: []string{"Run `clawchrome-cli snapshot` to see current page state"}}
		}
		return &CdpError{Message: message, Code: ErrUnknown}
	}
}

func ensureSession() (sessionState, transportConfig, error) {
	cfg, err := loadTransportConfig()
	if err != nil {
		return sessionState{}, transportConfig{}, err
	}

	switch cfg.mode {
	case transportHTTP:
		state, err := ensureHTTPSession(cfg)
		return state, cfg, err
	case transportStdio:
		state, err := ensureStdioSession()
		return state, cfg, err
	default:
		return sessionState{}, transportConfig{}, WrapError("Unsupported transport mode: "+cfg.mode, ErrValidation)
	}
}

func currentSession() (sessionState, transportConfig, bool) {
	cfg, err := loadTransportConfig()
	if err != nil {
		return sessionState{}, transportConfig{}, false
	}

	state, ok := readState()
	if !ok || state.Transport != cfg.mode || state.SessionID == "" {
		return sessionState{}, transportConfig{}, false
	}
	if cfg.mode == transportHTTP && state.BaseURL != cfg.baseURL {
		return sessionState{}, transportConfig{}, false
	}
	if cfg.mode == transportStdio && (state.PID <= 0 || state.Port <= 0 || !isProcessAlive(state.PID)) {
		return sessionState{}, transportConfig{}, false
	}
	return state, cfg, true
}

func ensureHTTPSession(cfg transportConfig) (sessionState, error) {
	if cfg.baseURL == "" {
		return sessionState{}, WrapError("Missing CLAWCHROME_CLI_HTTP_URL for http transport", ErrValidation)
	}
	if cfg.bearerToken == "" {
		return sessionState{}, WrapError("Missing CLAWCHROME_CLI_HTTP_BEARER_TOKEN for http transport", ErrValidation)
	}

	state, ok := readState()
	if ok && state.Transport == transportHTTP && state.BaseURL == cfg.baseURL && state.SessionID != "" {
		healthy, err := checkBridgeHealth(state, cfg)
		if err == nil && healthy {
			return state, nil
		}
	}

	state = sessionState{
		SessionID: generateSessionID(),
		Transport: transportHTTP,
		BaseURL:   cfg.baseURL,
		Port:      parsePortFromURL(cfg.baseURL),
	}
	healthy, err := checkBridgeHealth(state, cfg)
	if err != nil || !healthy {
		if err != nil {
			return sessionState{}, WrapError("Remote HTTP bridge is not reachable: "+err.Error(), ErrBridgeNotReady)
		}
		return sessionState{}, WrapError("Remote HTTP bridge is not reachable", ErrBridgeNotReady)
	}
	if err := writeState(state); err != nil {
		return sessionState{}, err
	}
	return state, nil
}

func ensureStdioSession() (sessionState, error) {
	port := DefaultPort
	if raw := os.Getenv("CLAWCHROME_CLI_PORT"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			port = parsed
		}
	}

	if state, ok := readState(); ok && state.Transport == transportStdio && state.PID > 0 && state.Port > 0 && isProcessAlive(state.PID) {
		healthy, _ := checkBridgeHealth(state, transportConfig{mode: transportStdio})
		if healthy {
			return state, nil
		}
		_ = terminateProcess(state.PID)
		removeState()
	}

	if pid, runningPort, ok := readPIDFile(); ok && isProcessAlive(pid) {
		state := sessionState{
			SessionID: generateSessionID(),
			Transport: transportStdio,
			PID:       pid,
			Port:      runningPort,
		}
		healthy, _ := checkBridgeHealth(state, transportConfig{mode: transportStdio})
		if healthy {
			if err := writeState(state); err != nil {
				return sessionState{}, err
			}
			return state, nil
		}
		_ = terminateProcess(pid)
	}

	exe, err := os.Executable()
	if err != nil {
		return sessionState{}, err
	}

	cmd := exec.Command(exe, "__bridge")
	cmd.Env = append(os.Environ(), fmt.Sprintf("CLAWCHROME_CLI_PORT=%d", port))
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = bridgeSysProcAttr()

	if err := cmd.Start(); err != nil {
		return sessionState{}, err
	}

	state := sessionState{
		SessionID: generateSessionID(),
		Transport: transportStdio,
		PID:       cmd.Process.Pid,
		Port:      port,
	}

	deadline := time.Now().Add(defaultBridgeStartTimeout)
	for time.Now().Before(deadline) {
		healthy, _ := checkBridgeHealth(state, transportConfig{mode: transportStdio})
		if healthy {
			if err := writeState(state); err != nil {
				return sessionState{}, err
			}
			return state, nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return sessionState{}, &CdpError{
		Message: "Bridge failed to start within 30s",
		Code:    ErrBridgeNotReady,
		Suggestions: []string{
			"Check that chrome-devtools-mcp is available via `npx -y chrome-devtools-mcp@latest --help`",
		},
	}
}

func loadTransportConfig() (transportConfig, error) {
	mode := strings.TrimSpace(os.Getenv("CLAWCHROME_CLI_TRANSPORT"))
	if mode == "" {
		mode = transportStdio
	}
	if mode != transportStdio && mode != transportHTTP {
		return transportConfig{}, WrapError("Unsupported CLAWCHROME_CLI_TRANSPORT: "+mode, ErrValidation)
	}

	cfg := transportConfig{mode: mode}
	if mode == transportHTTP {
		baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("CLAWCHROME_CLI_HTTP_URL")), "/")
		if baseURL == "" {
			return transportConfig{}, WrapError("Missing CLAWCHROME_CLI_HTTP_URL for http transport", ErrValidation)
		}
		if _, err := neturl.Parse(baseURL); err != nil {
			return transportConfig{}, WrapError("Invalid CLAWCHROME_CLI_HTTP_URL: "+err.Error(), ErrValidation)
		}
		cfg.baseURL = baseURL
		cfg.bearerToken = strings.TrimSpace(os.Getenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN"))
		if cfg.bearerToken == "" {
			return transportConfig{}, WrapError("Missing CLAWCHROME_CLI_HTTP_BEARER_TOKEN for http transport", ErrValidation)
		}
	}
	return cfg, nil
}

func checkBridgeHealth(state sessionState, cfg transportConfig) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, requestURL(state, cfg, defaultRemoteHealthPath), nil)
	if err != nil {
		return false, err
	}
	setCommonHeaders(req, state, cfg)
	client := &http.Client{Timeout: defaultHealthTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, err
	}
	return payload.Status == "ok", nil
}

func postJSON(state sessionState, cfg transportConfig, path string, body any, timeout time.Duration) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, requestURL(state, cfg, path), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	setCommonHeaders(req, state, cfg)
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, errors.New(string(data))
	}
	return data, nil
}

func setCommonHeaders(req *http.Request, state sessionState, cfg transportConfig) {
	if state.SessionID != "" {
		req.Header.Set(sessionHeaderName, state.SessionID)
	}
	if cfg.mode == transportHTTP && cfg.bearerToken != "" {
		req.Header.Set(defaultRemoteAuthHeader, "Bearer "+cfg.bearerToken)
	}
}

func requestURL(state sessionState, cfg transportConfig, path string) string {
	if cfg.mode == transportHTTP {
		return cfg.baseURL + path
	}
	return fmt.Sprintf("http://127.0.0.1:%d%s", state.Port, path)
}

func stateDirPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, defaultStateDirName)
}

func stateFilePath() string {
	return filepath.Join(stateDirPath(), defaultSessionStateName)
}

func pidFilePath() string {
	return filepath.Join(stateDirPath(), defaultBridgePIDFileName)
}

func writeState(state sessionState) error {
	if state.SessionID == "" {
		return errors.New("missing session ID")
	}
	if err := os.MkdirAll(stateDirPath(), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(stateFilePath(), data, 0o644)
}

func readState() (sessionState, bool) {
	data, err := os.ReadFile(stateFilePath())
	if err != nil {
		return sessionState{}, false
	}
	var state sessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return sessionState{}, false
	}
	if state.SessionID == "" || state.Transport == "" {
		return sessionState{}, false
	}
	return state, true
}

func removeState() {
	_ = os.Remove(stateFilePath())
}

func readPIDFile() (int, int, bool) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return 0, 0, false
	}
	var payload struct {
		PID  int `json:"pid"`
		Port int `json:"port"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, 0, false
	}
	if payload.PID <= 0 || payload.Port <= 0 {
		return 0, 0, false
	}
	return payload.PID, payload.Port, true
}

func generateSessionID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf[:])
}

func parsePortFromURL(raw string) int {
	parsed, err := neturl.Parse(raw)
	if err != nil {
		return 0
	}
	if parsed.Port() != "" {
		port, err := strconv.Atoi(parsed.Port())
		if err == nil {
			return port
		}
	}
	switch parsed.Scheme {
	case "https":
		return 443
	case "http":
		return 80
	default:
		return 0
	}
}

func isProcessAlive(pid int) bool {
	return processAlive(pid)
}
