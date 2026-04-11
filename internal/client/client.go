package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

type CdpError struct {
	Message     string
	Code        ErrorCode
	Suggestions []string
}

func (e *CdpError) Error() string { return e.Message }

func EnsureBridge() (int, error) {
	port := DefaultPort
	if raw := os.Getenv("CLAWCHROME_CLI_PORT"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			port = parsed
		}
	}

	if pid, runningPort, ok := readPIDFile(); ok && isProcessAlive(pid) {
		if healthy, _ := checkBridgeHealth(runningPort); healthy {
			return runningPort, nil
		}
		_ = terminateProcess(pid)
	}

	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}

	cmd := exec.Command(exe, "__bridge")
	cmd.Env = append(os.Environ(), fmt.Sprintf("CLAWCHROME_CLI_PORT=%d", port))
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = bridgeSysProcAttr()

	if err := cmd.Start(); err != nil {
		return 0, err
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		healthy, _ := checkBridgeHealth(port)
		if healthy {
			return port, nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return 0, &CdpError{
		Message: "Bridge failed to start within 30s",
		Code:    ErrBridgeNotReady,
		Suggestions: []string{
			"Check that chrome-devtools-mcp is available via `npx -y chrome-devtools-mcp@latest --help`",
		},
	}
}

func CallTool(name string, args map[string]any) (string, error) {
	port, err := EnsureBridge()
	if err != nil {
		return "", err
	}

	payload, err := postJSON(port, "/call", map[string]any{
		"name": name,
		"args": args,
	}, 2*time.Minute)
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
	pid, port, ok := readPIDFile()
	if !ok || !isProcessAlive(pid) {
		return "", false
	}
	healthy, _ := checkBridgeHealth(port)
	if !healthy {
		return "", false
	}
	payload, err := postJSON(port, "/call", map[string]any{
		"name": "take_snapshot",
		"args": map[string]any{},
	}, 5*time.Second)
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
	pid, _, ok := readPIDFile()
	if !ok || !isProcessAlive(pid) {
		return false
	}
	_ = terminateProcess(pid)
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

func checkBridgeHealth(port int) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/health", port), nil)
	if err != nil {
		return false, err
	}
	client := &http.Client{Timeout: 2 * time.Second}
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

func postJSON(port int, path string, body any, timeout time.Duration) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d%s", port, path), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
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

func pidFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".clawchrome-cli", "bridge.pid")
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

func isProcessAlive(pid int) bool {
	return processAlive(pid)
}
