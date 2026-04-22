package bridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	DefaultPort      = 9224
	DefaultProtoVers = "2025-06-18"
)

type toolContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type toolListResult struct {
	Tools []struct {
		Name        string         `json:"name"`
		Description string         `json:"description,omitempty"`
		InputSchema map[string]any `json:"inputSchema,omitempty"`
	} `json:"tools"`
}

type internalTool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

var internalTools = []internalTool{
	{
		Name:        "scroll_page",
		Description: "Scroll the current page in a direction using native key actions.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"direction": map[string]any{
					"type": "string",
					"enum": []string{"up", "down", "top", "bottom"},
				},
			},
			"required": []string{"direction"},
		},
	},
	{
		Name:        "wait_duration",
		Description: "Wait for a number of milliseconds without using page JavaScript.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"milliseconds": map[string]any{
					"type":    "integer",
					"minimum": 0,
				},
			},
			"required": []string{"milliseconds"},
		},
	},
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type mcpClient struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  io.ReadCloser
	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan rpcEnvelope
	closed  chan struct{}
}

func BuildTransportArgs() []string {
	args := []string{"-y", "chrome-devtools-mcp@latest", "--isolated", "--experimental-screencast"}
	if os.Getenv("CLAWCHROME_CLI_HEADED") != "1" {
		args = append(args, "--headless")
	}

	if extra := strings.TrimSpace(os.Getenv("CLAWCHROME_CLI_CHROME_ARGS")); extra != "" {
		for _, arg := range strings.Fields(extra) {
			args = append(args, "--chrome-arg="+arg)
		}
	}
	return args
}

func ParseBridgeCallPayload(body []byte) (string, map[string]any, error) {
	var payload struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", nil, errors.New("invalid bridge request payload")
	}
	if payload.Name == "" {
		return "", nil, errors.New("invalid bridge request payload")
	}
	if payload.Args == nil {
		payload.Args = map[string]any{}
	}
	return payload.Name, payload.Args, nil
}

func Run(ctx context.Context) error {
	port := DefaultPort
	if raw := os.Getenv("CLAWCHROME_CLI_PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err == nil {
			port = parsed
		}
	}

	client, err := startMCPClient()
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Initialize(); err != nil {
		return err
	}

	if err := writePIDFile(os.Getpid(), port); err != nil {
		return err
	}
	defer removePIDFile()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if _, err := client.ListTools(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
		result, err := client.ListTools()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tools := make([]map[string]any, 0, len(result.Tools)+len(internalTools))
		for _, tool := range result.Tools {
			tools = append(tools, map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"inputSchema": tool.InputSchema,
			})
		}
		for _, tool := range internalTools {
			tools = append(tools, map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"inputSchema": tool.InputSchema,
			})
		}
		writeJSON(w, http.StatusOK, tools)
	})
	mux.HandleFunc("/call", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		name, args, err := ParseBridgeCallPayload(body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		result, err := callToolJSON(client, name, args)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"result": result})
	})

	server := &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-sigCtx.Done()
		_ = server.Shutdown(context.Background())
	}()

	if _, err := fmt.Fprintln(os.Stdout, "READY"); err != nil {
		return err
	}

	err = server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func callToolJSON(client *mcpClient, name string, args map[string]any) (json.RawMessage, error) {
	switch name {
	case "scroll_page":
		return callScrollPage(client, args)
	case "wait_duration":
		return callWaitDuration(args)
	default:
		return client.CallToolJSON(name, args)
	}
}

func callScrollPage(client *mcpClient, args map[string]any) (json.RawMessage, error) {
	direction, _ := args["direction"].(string)
	key, ok := map[string]string{
		"up":     "PageUp",
		"down":   "PageDown",
		"top":    "Home",
		"bottom": "End",
	}[direction]
	if !ok {
		return nil, fmt.Errorf("invalid scroll_page direction %q", direction)
	}
	if _, err := client.CallToolJSON("press_key", map[string]any{"key": key}); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"ok":        true,
		"direction": direction,
	})
}

func callWaitDuration(args map[string]any) (json.RawMessage, error) {
	value, ok := args["milliseconds"]
	if !ok {
		return nil, errors.New("missing milliseconds")
	}

	ms, ok := toInt(value)
	if !ok || ms < 0 {
		return nil, fmt.Errorf("invalid milliseconds value %v", value)
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return json.Marshal(map[string]any{
		"ok":           true,
		"milliseconds": ms,
	})
}

func toInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		if v != float64(int(v)) {
			return 0, false
		}
		return int(v), true
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func startMCPClient() (*mcpClient, error) {
	cmd := exec.Command("npx", BuildTransportArgs()...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	client := &mcpClient{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		stderr:  stderr,
		pending: map[int64]chan rpcEnvelope{},
		closed:  make(chan struct{}),
	}
	go client.readLoop()
	go io.Copy(os.Stderr, stderr)
	return client, nil
}

func (c *mcpClient) Close() error {
	close(c.closed)
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Signal(syscall.SIGTERM)
		_, _ = c.cmd.Process.Wait()
	}
	return nil
}

func (c *mcpClient) Initialize() error {
	_, err := c.call("initialize", map[string]any{
		"protocolVersion": DefaultProtoVers,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "clawchrome-cli",
			"version": "0.1.0",
		},
	})
	if err != nil {
		return err
	}
	return c.notify("notifications/initialized", map[string]any{})
}

func (c *mcpClient) ListTools() (*toolListResult, error) {
	raw, err := c.call("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result toolListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *mcpClient) CallTool(name string, args map[string]any) (string, error) {
	raw, err := c.CallToolJSON(name, args)
	if err != nil {
		return "", err
	}
	return toolResultText(raw), nil
}

func (c *mcpClient) CallToolJSON(name string, args map[string]any) (json.RawMessage, error) {
	raw, err := c.call("tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), raw...), nil
}

func toolResultText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var result struct {
		Content []toolContentBlock `json:"content"`
		Text    string             `json:"text"`
	}
	if err := json.Unmarshal(raw, &result); err == nil {
		if strings.TrimSpace(result.Text) != "" {
			return result.Text
		}
		parts := make([]string, 0)
		for _, block := range result.Content {
			if block.Type == "text" && block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err == nil {
		return compact.String()
	}
	return string(raw)
}

func (c *mcpClient) notify(method string, params any) error {
	envelope := rpcEnvelope{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.writeEnvelope(envelope)
}

func (c *mcpClient) call(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	responseCh := make(chan rpcEnvelope, 1)
	c.pending[id] = responseCh
	c.mu.Unlock()

	if err := c.writeEnvelope(rpcEnvelope{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}); err != nil {
		return nil, err
	}

	select {
	case response := <-responseCh:
		if response.Error != nil {
			return nil, errors.New(response.Error.Message)
		}
		return response.Result, nil
	case <-time.After(2 * time.Minute):
		return nil, errors.New("timeout waiting for MCP response")
	}
}

func (c *mcpClient) writeEnvelope(envelope rpcEnvelope) error {
	payload, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return err
	}
	_, err = c.stdin.Write(payload)
	return err
}

func (c *mcpClient) readLoop() {
	for {
		select {
		case <-c.closed:
			return
		default:
		}

		body, err := readFramedMessage(c.stdout)
		if err != nil {
			return
		}

		var envelope rpcEnvelope
		if err := json.Unmarshal(body, &envelope); err != nil {
			continue
		}

		idFloat, ok := envelope.ID.(float64)
		if !ok {
			continue
		}
		id := int64(idFloat)

		c.mu.Lock()
		ch := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()

		if ch != nil {
			ch <- envelope
		}
	}
}

func readFramedMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.EqualFold(parts[0], "Content-Length") {
			value := strings.TrimSpace(parts[1])
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return nil, err
			}
			contentLength = parsed
		}
	}
	if contentLength <= 0 {
		return nil, errors.New("missing content length")
	}
	body := make([]byte, contentLength)
	_, err := io.ReadFull(r, body)
	return body, err
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	buf := bytes.Buffer{}
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
	_, _ = w.Write(bytes.TrimRight(buf.Bytes(), "\n"))
}

func pidFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".clawchrome-cli", "bridge.pid")
}

func writePIDFile(pid int, port int) error {
	path := pidFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]int{"pid": pid, "port": port})
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func removePIDFile() {
	_ = os.Remove(pidFilePath())
}
