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
	"regexp"
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
	ErrAuth           ErrorCode = "AUTH_ERROR"
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

var uidRefPattern = regexp.MustCompile(`(?i)\buid[=:\s]+@?([A-Za-z0-9_-]+)`)

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
	interpreted := extractErrorPayload(message)
	combined := strings.TrimSpace(message + "\n" + interpreted)
	lower := strings.ToLower(combined)

	switch {
	case strings.Contains(lower, "http 401"), strings.Contains(lower, "http 403"), strings.Contains(lower, "unauthorized"), strings.Contains(lower, "forbidden"):
		return &CdpError{
			Message: "target API auth failed. Check CLAWCHROME_CLI_HTTP_BEARER_TOKEN.",
			Code:    ErrAuth,
			Suggestions: []string{
				"Set CLAWCHROME_CLI_HTTP_BEARER_TOKEN to a token accepted by the target runtime API",
			},
		}
	case strings.Contains(lower, "econnrefused"), strings.Contains(lower, "econnreset"), strings.Contains(lower, "connection refused"), strings.Contains(lower, "no such host"):
		return &CdpError{
			Message: "target API is unreachable. Check CLAWCHROME_CLI_HTTP_URL and that the runtime is running.",
			Code:    ErrBridgeNotReady,
		}
	case strings.Contains(lower, "uid") || strings.Contains(lower, "element"):
		ref := extractRef(combined)
		if ref == "" {
			return &CdpError{
				Message: "No element matched the provided ref in the current snapshot.",
				Code:    ErrRefNotFound,
				Suggestions: []string{
					"Run `clawchrome-cli snapshot` to see available element refs",
					"Use refs exactly as shown, for example `@1`",
				},
			}
		}
		return &CdpError{
			Message: "No element with ref " + ref + " was found in the current snapshot.",
			Code:    ErrRefNotFound,
			Suggestions: []string{
				"Run `clawchrome-cli snapshot` to see available element refs",
				"Use refs exactly as shown, for example `@1`",
			},
		}
	case strings.Contains(lower, "timeout"):
		return &CdpError{Message: "Timed out waiting for the browser runtime.", Code: ErrTimeout, Suggestions: []string{"Run `clawchrome-cli snapshot` to see current page state"}}
	default:
		if interpreted != "" {
			return &CdpError{Message: interpreted, Code: ErrBrowser, Suggestions: []string{"Run `clawchrome-cli snapshot` to see current page state"}}
		}
		return &CdpError{Message: message, Code: ErrUnknown}
	}
}

func extractErrorPayload(message string) string {
	for _, candidate := range []string{message, jsonSubstring(message)} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		var payload struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(candidate), &payload) == nil && payload.Error != "" {
			return payload.Error
		}
	}
	return ""
}

func jsonSubstring(message string) string {
	start := strings.Index(message, "{")
	if start < 0 {
		return ""
	}
	return message[start:]
}

func extractRef(message string) string {
	if match := uidRefPattern.FindStringSubmatch(message); len(match) == 2 && match[1] != "" {
		return "@" + strings.TrimPrefix(match[1], "@")
	}
	for _, field := range strings.Fields(message) {
		field = strings.Trim(field, `"'.,;:()[]{}<>`)
		if strings.HasPrefix(field, "@") && len(field) > 1 {
			return field
		}
	}
	return ""
}

func transportSetupErrorMessage(err error) string {
	if err == nil {
		return "target API is unreachable. Check CLAWCHROME_CLI_HTTP_URL and that the runtime is running."
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "auth") || strings.Contains(lower, "permission") || strings.Contains(lower, "401") || strings.Contains(lower, "403") {
		return "target API auth failed. Check CLAWCHROME_CLI_HTTP_BEARER_TOKEN."
	}
	return "target API is unreachable. Check CLAWCHROME_CLI_HTTP_URL and that the runtime is running."
}

func httpStatusMessage(status int, body []byte) string {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "target API auth failed. Check CLAWCHROME_CLI_HTTP_BEARER_TOKEN."
	default:
		if msg := extractErrorPayload(string(body)); msg != "" {
			return fmt.Sprintf("target API health check failed with HTTP %d: %s", status, msg)
		}
		return fmt.Sprintf("target API health check failed with HTTP %d", status)
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
		return sessionState{}, WrapError(
			"Missing target API URL: set CLAWCHROME_CLI_HTTP_URL for http transport",
			ErrValidation,
			"Set CLAWCHROME_CLI_HTTP_URL to the runtime API base URL",
		)
	}
	if cfg.bearerToken == "" {
		return sessionState{}, WrapError(
			"Missing auth token: set CLAWCHROME_CLI_HTTP_BEARER_TOKEN for http transport",
			ErrValidation,
			"Set CLAWCHROME_CLI_HTTP_BEARER_TOKEN to the runtime API bearer token",
		)
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
			return sessionState{}, WrapError(transportSetupErrorMessage(err), ErrBridgeNotReady)
		}
		return sessionState{}, WrapError("target API is unreachable. Check CLAWCHROME_CLI_HTTP_URL and that the runtime is running.", ErrBridgeNotReady)
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
			return transportConfig{}, WrapError(
				"Missing target API URL: set CLAWCHROME_CLI_HTTP_URL for http transport",
				ErrValidation,
				"Set CLAWCHROME_CLI_HTTP_URL to the runtime API base URL",
			)
		}
		parsedURL, err := neturl.Parse(baseURL)
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			return transportConfig{}, WrapError(
				"Invalid target API URL: set CLAWCHROME_CLI_HTTP_URL to an http or https URL",
				ErrValidation,
				"Example: CLAWCHROME_CLI_HTTP_URL=http://127.0.0.1:8091",
			)
		}
		cfg.baseURL = baseURL
		cfg.bearerToken = strings.TrimSpace(os.Getenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN"))
		if cfg.bearerToken == "" {
			return transportConfig{}, WrapError(
				"Missing auth token: set CLAWCHROME_CLI_HTTP_BEARER_TOKEN for http transport",
				ErrValidation,
				"Set CLAWCHROME_CLI_HTTP_BEARER_TOKEN to the runtime API bearer token",
			)
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
		data, _ := io.ReadAll(resp.Body)
		return false, errors.New(httpStatusMessage(resp.StatusCode, data))
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
		return nil, errors.New(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data))))
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
