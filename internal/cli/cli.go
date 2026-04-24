package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/gvkhna/clawchrome-cli/internal/client"
	"github.com/gvkhna/clawchrome-cli/internal/snapshot"
	"github.com/gvkhna/clawchrome-cli/internal/toonout"
	"github.com/gvkhna/clawchrome-cli/internal/update"
)

var Version = "0.1.0-dev"

var (
	callTool                    = client.CallTool
	callToolJSON                = client.CallToolJSON
	callRuntimeHTTPTool         = client.CallRuntimeHTTPTool
	ensureBridge                = client.EnsureBridge
	getSessionSnapshotIfRunning = client.GetSessionSnapshotIfRunning
	getClientStatus             = client.GetClientStatus
	saveAuthConfig              = client.SaveAuthConfig
	stopBridge                  = client.StopBridge
	usesHTTPTransport           = client.UsesHTTPTransport
	selfUpdate                  = func(currentVersion string, targetVersion string) (string, error) {
		return update.SelfUpdate(context.Background(), currentVersion, targetVersion)
	}
	updateNotice = func(currentVersion string) (string, error) {
		return update.Notice(context.Background(), currentVersion)
	}
)

var (
	recoverableOpenErrorPattern = regexp.MustCompile(`(?i)not connected|session (?:closed|not found)|no page`)
	refArgPattern               = regexp.MustCompile(`^@[A-Za-z0-9][A-Za-z0-9_-]*$`)
)

func Main(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && args[0] == "--full") {
		output := renderHome()
		_, _ = io.WriteString(stdout, output+"\n")
		writeUpdateNotice(stderr, "")
		return 0
	}

	if len(args) == 1 {
		switch args[0] {
		case "-h", "--help":
			_, _ = io.WriteString(stdout, topHelp)
			return 0
		case "version":
			_, _ = io.WriteString(stdout, Version+"\n")
			return 0
		case "-v", "-V", "--version":
			_, _ = io.WriteString(stdout, Version+"\n")
			return 0
		}
	}

	if len(args) >= 2 && hasHelpArg(args[1:]) {
		if help := getCommandHelp(args[0]); help != "" {
			_, _ = io.WriteString(stdout, help+"\n")
			return 0
		}
		_, _ = io.WriteString(stdout, renderError(client.WrapError("Unknown command: "+args[0], client.ErrValidation, "Run `clawchrome-cli --help` to see available commands"))+"\n")
		return 2
	}

	command := args[0]
	commandArgs, full := splitFullFlag(args[1:])

	if full && command != "" && getCommandHelp(command) != "" && !CommandSupportsFullFlag(command) {
		_, _ = io.WriteString(stdout, renderCommandError(command, validationError(command, "Unexpected flag: --full"))+"\n")
		return 2
	}

	output, err := runCommand(command, commandArgs, full)
	if err != nil {
		_, _ = io.WriteString(stdout, renderCommandError(command, err)+"\n")
		return exitCode(err)
	}
	_, _ = io.WriteString(stdout, output+"\n")
	writeUpdateNotice(stderr, command)
	return 0
}

func runCommand(command string, args []string, full bool) (string, error) {
	switch command {
	case "open":
		return handleOpen(args, full)
	case "snapshot":
		return handleSnapshot(args, full)
	case "screenshot":
		return handleScreenshot(args)
	case "click":
		return handleSnapshotCommand("click", args, full)
	case "fill":
		return handleFill(args, full)
	case "type":
		return handleType(args, full)
	case "press":
		return handlePress(args, full)
	case "scroll":
		return handleScroll(args, full)
	case "mouse":
		return handleMouse(args)
	case "back":
		return handleBack(args, full)
	case "forward":
		return handleForward(args, full)
	case "reload":
		return handleReload(args, full)
	case "wait":
		return handleWait(args)
	case "hover":
		return handleSnapshotCommand("hover", args, full)
	case "drag":
		return handleDrag(args, full)
	case "fillform":
		return handleFillForm(args, full)
	case "form":
		return handleForm(args, full)
	case "dialog":
		return handleDialog(args)
	case "pages":
		return handlePages(args)
	case "newpage":
		return handleNewPage(args, full)
	case "selectpage":
		return handleSelectPage(args, full)
	case "closepage":
		return handleClosePage(args)
	case "resize":
		return handleResize(args)
	case "video":
		return handleVideo(args)
	case "start":
		return handleStart(args)
	case "status":
		return handleStatus(args)
	case "stop":
		return handleStop(args)
	case "version":
		return handleVersion(args)
	case "self-update":
		return handleSelfUpdate(args)
	case "--help":
		return topHelp, nil
	default:
		return "", client.WrapError("Unknown command: "+command, client.ErrValidation, "Run `clawchrome-cli --help` to see available commands")
	}
}

func renderHome() string {
	if snap, ok := getSessionSnapshotIfRunning(); ok {
		snap = snapshot.StripSnapshotHeader(snap)
		page := map[string]any{"refs": snapshot.CountRefs(snap)}
		if title := snapshot.ExtractTitle(snap); title != "" {
			page["title"] = title
		}
		return joinBlocks(
			encode(map[string]any{"page": page}),
			renderHelp([]string{
				"Run `clawchrome-cli snapshot` to see page content",
				"Run `clawchrome-cli open <url>` to navigate to a URL",
				"Run `clawchrome-cli --help` to see full command list",
			}),
		)
	}

	return joinBlocks(
		encode(map[string]any{"browser": "no active session"}),
		renderHelp([]string{"Run `clawchrome-cli open <url>` to start browsing"}),
	)
}

func handleStart(args []string) (string, error) {
	parsed, err := parseStartArgs(args)
	if err != nil {
		return "", err
	}

	var savedAuth *client.AuthConfigStatus
	if parsed.saveAuth {
		auth, err := saveAuthConfig(parsed.token, parsed.agentName)
		if err != nil {
			return "", err
		}
		savedAuth = &auth
	}

	port, err := ensureBridge()
	if err != nil {
		return "", err
	}
	if usesHTTPTransport() {
		toolArgs := map[string]any{}
		if parsed.url != "" {
			toolArgs["url"] = parsed.url
		}
		if _, err := callTool("start_browser", toolArgs); err != nil {
			return "", err
		}
	}
	payload := map[string]any{"status": "ready", "port": port}
	if savedAuth != nil {
		payload["auth"] = authStatusOutput(*savedAuth)
	}
	return encode(payload), nil
}

func handleStatus(args []string) (string, error) {
	if err := requireNoArgs("status", args); err != nil {
		return "", err
	}
	status, err := getClientStatus()
	if err != nil {
		return "", err
	}
	return encode(clientStatusOutput(status)), nil
}

type startArgs struct {
	url          string
	token        string
	agentName    string
	tokenSet     bool
	agentNameSet bool
	saveAuth     bool
}

func parseStartArgs(args []string) (startArgs, error) {
	var parsed startArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--token":
			if i+1 >= len(args) {
				return startArgs{}, validationError("start", "Missing token value")
			}
			i++
			parsed.token = args[i]
			parsed.tokenSet = true
			parsed.saveAuth = true
		case strings.HasPrefix(arg, "--token="):
			parsed.token = strings.TrimPrefix(arg, "--token=")
			parsed.tokenSet = true
			parsed.saveAuth = true
		case arg == "--agent-name":
			if i+1 >= len(args) {
				return startArgs{}, validationError("start", "Missing agent name")
			}
			i++
			parsed.agentName = args[i]
			parsed.agentNameSet = true
			parsed.saveAuth = true
		case strings.HasPrefix(arg, "--agent-name="):
			parsed.agentName = strings.TrimPrefix(arg, "--agent-name=")
			parsed.agentNameSet = true
			parsed.saveAuth = true
		case strings.HasPrefix(arg, "-"):
			return startArgs{}, unexpectedArgError("start", arg)
		default:
			if parsed.url != "" {
				return startArgs{}, unexpectedArgError("start", arg)
			}
			parsed.url = arg
		}
	}
	if parsed.tokenSet && strings.TrimSpace(parsed.token) == "" {
		return startArgs{}, validationError("start", "Missing token value")
	}
	if parsed.agentNameSet && strings.TrimSpace(parsed.agentName) == "" {
		return startArgs{}, validationError("start", "Missing agent name")
	}
	if parsed.saveAuth && strings.TrimSpace(parsed.token) == "" && strings.TrimSpace(parsed.agentName) == "" {
		return startArgs{}, validationError("start", "Missing auth value: provide --token or --agent-name")
	}
	return parsed, nil
}

func clientStatusOutput(status client.ClientStatus) map[string]any {
	runtime := map[string]any{"transport": status.Status.Transport}
	if status.Status.Target != "" {
		runtime["target"] = status.Status.Target
	}
	return map[string]any{
		"status": runtime,
		"auth":   authStatusOutput(status.Auth),
	}
}

func authStatusOutput(auth client.AuthConfigStatus) map[string]any {
	output := map[string]any{
		"token":        auth.Token,
		"source":       auth.Source,
		"configPath":   auth.ConfigPath,
		"configExists": auth.ConfigExists,
	}
	if auth.AgentName != "" {
		output["agentName"] = auth.AgentName
	}
	return output
}

func handleOpen(args []string, full bool) (string, error) {
	if len(args) == 0 {
		return "", validationError("open", "Missing URL")
	}
	if strings.HasPrefix(args[0], "-") {
		return "", unexpectedArgError("open", args[0])
	}
	if len(args) > 1 {
		return "", unexpectedArgError("open", args[1])
	}
	url := args[0]

	if _, err := callTool("navigate_page", map[string]any{"type": "url", "url": url}); err != nil {
		if !isRecoverableOpenError(err) {
			return "", err
		}
		if _, retryErr := callTool("new_page", map[string]any{"url": url}); retryErr != nil {
			return "", retryErr
		}
	}

	snap, err := callTool("take_snapshot", map[string]any{})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snapshot.StripSnapshotHeader(snap), "open", url, full), nil
}

func handleSnapshot(args []string, full bool) (string, error) {
	parsed := parseSnapshotArgs(args)
	if parsed.invalid != "" {
		return "", validationError("snapshot", "Unexpected snapshot argument: "+parsed.invalid)
	}

	toolArgs := map[string]any{}
	if parsed.form {
		toolArgs["form"] = true
	}
	if parsed.text {
		toolArgs["text"] = true
	}
	if full && !parsed.form && !parsed.text {
		toolArgs["verbose"] = true
	}

	snap, err := callTool("take_snapshot", toolArgs)
	if err != nil {
		return "", err
	}
	return formatPageOutput(snapshot.StripSnapshotHeader(snap), "snapshot", "", full), nil
}

func handleScreenshot(args []string) (string, error) {
	parsed := parseScreenshotArgs(args)
	if parsed.invalid != "" {
		return "", validationError("screenshot", parsed.invalid)
	}

	format := parsed.format
	if format == "" {
		format = "png"
	}
	filePath, err := resolveOutputFilePath("screenshot", parsed.filePath, "clawchrome-cli-screenshot-*."+format)
	if err != nil {
		return "", err
	}

	toolArgs := map[string]any{"filePath": filePath}
	if parsed.fullPage {
		toolArgs["fullPage"] = true
	}
	if parsed.format != "" {
		toolArgs["format"] = parsed.format
	}

	if _, err := callTool("take_screenshot", toolArgs); err != nil {
		return "", outputPathOperationError("save screenshot", filePath, err)
	}
	return encode(map[string]any{"screenshot": map[string]any{"status": "saved", "path": filePath}}), nil
}

func handleFill(args []string, full bool) (string, error) {
	if len(args) == 0 {
		return "", validationError("fill", "Missing element ref")
	}
	uid, err := validateRefArg("fill", args[0])
	if err != nil {
		return "", err
	}
	value := strings.Join(args[1:], " ")
	if value == "" {
		return "", validationError("fill", "Missing fill text")
	}

	snap, err := callWithSnapshot("fill", map[string]any{"uid": uid, "value": value})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, "fill", "", full), nil
}

func handleType(args []string, full bool) (string, error) {
	text := strings.Join(args, " ")
	if text == "" {
		return "", validationError("type", "Missing text")
	}

	if _, err := callTool("type_text", map[string]any{"text": text}); err != nil {
		return "", err
	}
	snap, err := callTool("take_snapshot", map[string]any{})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snapshot.StripSnapshotHeader(snap), "type", "", full), nil
}

func handlePress(args []string, full bool) (string, error) {
	if len(args) == 0 {
		return "", validationError("press", "Missing key name")
	}
	if len(args) > 1 {
		return "", unexpectedArgError("press", args[1])
	}
	if strings.HasPrefix(args[0], "-") {
		return "", unexpectedArgError("press", args[0])
	}
	key := args[0]

	snap, err := callWithSnapshot("press_key", map[string]any{"key": key})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, "press", "", full), nil
}

func handleScroll(args []string, full bool) (string, error) {
	if len(args) > 1 {
		return "", unexpectedArgError("scroll", args[1])
	}
	if len(args) == 1 && strings.HasPrefix(args[0], "-") {
		return "", unexpectedArgError("scroll", args[0])
	}
	dir := strings.ToLower(firstPositionalArg(args))
	if dir == "" {
		dir = "down"
	}

	switch dir {
	case "up", "down", "top", "bottom":
	default:
		return "", validationError("scroll", "Unknown scroll direction: "+dir)
	}

	scrollResult, err := callToolJSON("scroll_page", map[string]any{"direction": dir})
	if err != nil {
		return "", err
	}
	snap, err := callTool("take_snapshot", map[string]any{})
	if err != nil {
		return "", err
	}
	pageOutput := formatPageOutput(snapshot.StripSnapshotHeader(snap), "scroll", "", full)
	if scrollOutput := formatScrollResult(scrollResult); scrollOutput != "" {
		return joinBlocks(scrollOutput, pageOutput), nil
	}
	return pageOutput, nil
}

type scrollToolResult struct {
	Message   string         `json:"message"`
	Direction string         `json:"direction"`
	Scroll    *scrollInfoDTO `json:"scroll"`
}

type scrollInfoDTO struct {
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	ViewportWidth  int     `json:"viewportWidth"`
	ViewportHeight int     `json:"viewportHeight"`
	PageWidth      int     `json:"pageWidth"`
	PageHeight     int     `json:"pageHeight"`
	MaxScrollY     float64 `json:"maxScrollY"`
	RemainingY     float64 `json:"remainingY"`
	PercentY       float64 `json:"percentY"`
	ScrollableY    bool    `json:"scrollableY"`
	AtTop          bool    `json:"atTop"`
	AtBottom       bool    `json:"atBottom"`
	URL            string  `json:"url"`
}

func formatScrollResult(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" || raw[0] == '"' {
		return ""
	}
	var result scrollToolResult
	if err := json.Unmarshal(raw, &result); err != nil || result.Scroll == nil {
		return ""
	}
	status := "scrolled"
	if strings.TrimSpace(result.Direction) != "" {
		status += " " + strings.TrimSpace(result.Direction)
	}
	scroll := map[string]any{
		"status":      status,
		"y":           roundedDisplayFloat(result.Scroll.Y),
		"remainingY":  roundedDisplayFloat(result.Scroll.RemainingY),
		"percentY":    roundedDisplayFloat(result.Scroll.PercentY),
		"scrollableY": result.Scroll.ScrollableY,
		"atTop":       result.Scroll.AtTop,
		"atBottom":    result.Scroll.AtBottom,
	}
	if result.Scroll.MaxScrollY > 0 {
		scroll["maxY"] = roundedDisplayFloat(result.Scroll.MaxScrollY)
	}
	if result.Scroll.ViewportWidth > 0 || result.Scroll.ViewportHeight > 0 {
		scroll["viewport"] = fmt.Sprintf("%dx%d", result.Scroll.ViewportWidth, result.Scroll.ViewportHeight)
	}
	if result.Scroll.PageWidth > 0 || result.Scroll.PageHeight > 0 {
		scroll["page"] = fmt.Sprintf("%dx%d", result.Scroll.PageWidth, result.Scroll.PageHeight)
	}
	return encode(map[string]any{"scroll": scroll})
}

func roundedDisplayFloat(value float64) any {
	rounded := math.Round(value)
	if math.Abs(value-rounded) < 0.01 {
		return int(rounded)
	}
	return math.Round(value*10) / 10
}

func handleMouse(args []string) (string, error) {
	if len(args) == 0 {
		return "", validationError("mouse", "Missing mouse action")
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "move":
		return handleMouseMove(args[1:])
	case "click":
		return handleMouseClick(args[1:])
	case "drag":
		return handleMouseDrag(args[1:])
	case "down":
		return handleMouseButton(args[1:], "down", "browser_mouse_down")
	case "up":
		return handleMouseButton(args[1:], "up", "browser_mouse_up")
	case "wheel":
		return handleMouseWheel(args[1:])
	default:
		if strings.HasPrefix(args[0], "-") {
			return "", unexpectedArgError("mouse", args[0])
		}
		return "", validationError("mouse", "Unknown mouse action: "+args[0])
	}
}

func handleMouseMove(args []string) (string, error) {
	if len(args) < 2 {
		return "", validationError("mouse", "Missing mouse coordinates")
	}
	if len(args) > 2 {
		return "", unexpectedArgError("mouse", args[2])
	}
	x, y, err := parseMousePointArgs(args, "x", "y")
	if err != nil {
		return "", err
	}
	return callMouseTool("move", "browser_mouse_move_xy", map[string]any{"x": x, "y": y})
}

func handleMouseClick(args []string) (string, error) {
	if len(args) < 2 {
		return "", validationError("mouse", "Missing mouse coordinates")
	}
	if len(args) > 2 {
		return "", unexpectedArgError("mouse", args[2])
	}
	x, y, err := parseMousePointArgs(args, "x", "y")
	if err != nil {
		return "", err
	}
	return callMouseTool("click", "browser_mouse_click_xy", map[string]any{"x": x, "y": y})
}

func handleMouseDrag(args []string) (string, error) {
	if len(args) < 4 {
		return "", validationError("mouse", "Missing drag coordinates")
	}
	if len(args) > 4 {
		return "", unexpectedArgError("mouse", args[4])
	}
	startX, startY, err := parseMousePointArgs(args[0:2], "startX", "startY")
	if err != nil {
		return "", err
	}
	endX, endY, err := parseMousePointArgs(args[2:4], "endX", "endY")
	if err != nil {
		return "", err
	}
	return callMouseTool("drag", "browser_mouse_drag_xy", map[string]any{
		"startX": startX,
		"startY": startY,
		"endX":   endX,
		"endY":   endY,
	})
}

func handleMouseButton(args []string, action string, tool string) (string, error) {
	if len(args) > 1 {
		return "", unexpectedArgError("mouse", args[1])
	}
	button := "left"
	if len(args) == 1 {
		if strings.HasPrefix(args[0], "-") {
			return "", unexpectedArgError("mouse", args[0])
		}
		switch strings.ToLower(args[0]) {
		case "left", "right", "middle":
			button = strings.ToLower(args[0])
		default:
			return "", validationError("mouse", "Invalid mouse button: "+args[0])
		}
	}
	return callMouseTool(action, tool, map[string]any{"button": button})
}

func handleMouseWheel(args []string) (string, error) {
	if len(args) < 2 {
		return "", validationError("mouse", "Missing wheel deltas")
	}
	if len(args) > 2 {
		return "", unexpectedArgError("mouse", args[2])
	}
	deltaX, err := parseMouseNumberArg("mouse", "deltaX", args[0], true)
	if err != nil {
		return "", err
	}
	deltaY, err := parseMouseNumberArg("mouse", "deltaY", args[1], true)
	if err != nil {
		return "", err
	}
	return callMouseTool("wheel", "browser_mouse_wheel", map[string]any{"deltaX": deltaX, "deltaY": deltaY})
}

func parseMousePointArgs(args []string, xName string, yName string) (float64, float64, error) {
	x, err := parseMouseNumberArg("mouse", xName, args[0], false)
	if err != nil {
		return 0, 0, err
	}
	y, err := parseMouseNumberArg("mouse", yName, args[1], false)
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

func parseMouseNumberArg(command string, name string, raw string, allowNegative bool) (float64, error) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		if strings.HasPrefix(raw, "-") {
			return 0, unexpectedArgError(command, raw)
		}
		return 0, validationError(command, "Invalid "+name+" value: "+raw)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, validationError(command, "Invalid "+name+" value: "+raw)
	}
	if !allowNegative && value < 0 {
		return 0, validationError(command, name+" must be non-negative")
	}
	return value, nil
}

func callMouseTool(action string, tool string, args map[string]any) (string, error) {
	result, err := callRuntimeHTTPTool(tool, args)
	if err != nil {
		return "", err
	}
	payload := map[string]any{
		"mouse": map[string]any{
			"status": mouseStatus(action),
			"action": action,
			"tool":   tool,
		},
	}
	mouse := payload["mouse"].(map[string]any)
	if result.Message != "" {
		mouse["message"] = result.Message
	}
	if result.Backend != "" {
		mouse["backend"] = result.Backend
	}
	if result.Data != nil {
		mouse["data"] = result.Data
	}
	return encode(payload), nil
}

func mouseStatus(action string) string {
	switch action {
	case "move":
		return "moved"
	case "click":
		return "clicked"
	case "drag":
		return "dragged"
	case "down":
		return "pressed"
	case "up":
		return "released"
	case "wheel":
		return "scrolled"
	default:
		return "ok"
	}
}

func handleBack(args []string, full bool) (string, error) {
	if err := requireNoArgs("back", args); err != nil {
		return "", err
	}
	if _, err := callTool("navigate_page", map[string]any{"type": "back"}); err != nil {
		return "", err
	}
	snap, err := callTool("take_snapshot", map[string]any{})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snapshot.StripSnapshotHeader(snap), "back", "", full), nil
}

func handleForward(args []string, full bool) (string, error) {
	if err := requireNoArgs("forward", args); err != nil {
		return "", err
	}
	return handleHistoryNavigation("forward", full)
}

func handleReload(args []string, full bool) (string, error) {
	if err := requireNoArgs("reload", args); err != nil {
		return "", err
	}
	return handleHistoryNavigation("reload", full)
}

func handleHistoryNavigation(navType string, full bool) (string, error) {
	if _, err := callTool("navigate_page", map[string]any{"type": navType}); err != nil {
		return "", err
	}
	snap, err := callTool("take_snapshot", map[string]any{})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snapshot.StripSnapshotHeader(snap), navType, "", full), nil
}

func handleWait(args []string) (string, error) {
	target := strings.Join(args, " ")
	if target == "" {
		return "", validationError("wait", "Missing wait target (milliseconds or text)")
	}

	if isDigits(target) {
		ms, err := strconv.Atoi(target)
		if err != nil {
			return "", validationError("wait", "Invalid wait duration: "+target)
		}
		if _, err := callTool("wait_duration", map[string]any{"milliseconds": ms}); err != nil {
			return "", err
		}
	} else {
		if _, err := callTool("wait_for", map[string]any{"text": []string{target}}); err != nil {
			return "", err
		}
	}

	return joinBlocks(
		encode(map[string]any{"waited": target}),
		renderHelp([]string{"Run `clawchrome-cli snapshot` to see current page state"}),
	), nil
}

func handleDrag(args []string, full bool) (string, error) {
	if len(args) < 2 {
		return "", validationError("drag", "Missing element refs")
	}
	if len(args) > 2 {
		return "", unexpectedArgError("drag", args[2])
	}
	fromUID, err := validateRefArg("drag", args[0])
	if err != nil {
		return "", err
	}
	toUID, err := validateRefArg("drag", args[1])
	if err != nil {
		return "", err
	}

	snap, err := callWithSnapshot("drag", map[string]any{"from_uid": fromUID, "to_uid": toUID})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, "drag", "", full), nil
}

func handleFillForm(args []string, full bool) (string, error) {
	parsed := parseFillFormArgs(args)
	if parsed.invalid != "" {
		return "", validationError("fillform", parsed.invalid)
	}
	if len(parsed.entries) == 0 {
		return "", validationError("fillform", "No valid field entries")
	}

	snap, err := callWithSnapshot("fill_form", map[string]any{"elements": parsed.entries})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, "fillform", "", full), nil
}

func handleDialog(args []string) (string, error) {
	if len(args) == 0 {
		return "", validationError("dialog", "Missing or invalid action")
	}
	action := args[0]
	if action == "" || (action != "accept" && action != "dismiss") {
		return "", validationError("dialog", "Missing or invalid action")
	}

	params := map[string]any{"action": action}
	if len(args) > 1 {
		params["promptText"] = strings.Join(args[1:], " ")
	}
	if _, err := callTool("handle_dialog", params); err != nil {
		return "", err
	}
	return encode(map[string]any{"dialog": action}), nil
}

func handleForm(args []string, full bool) (string, error) {
	if len(args) == 0 {
		return "", validationError("form", "Missing form action")
	}

	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "clear":
		return handleFormClear(args[1:], full)
	case "check":
		return handleFormCheck(args[1:], true, full)
	case "uncheck":
		return handleFormCheck(args[1:], false, full)
	case "select":
		return handleFormSelect(args[1:], full)
	case "upload":
		return handleFormUpload(args[1:], full)
	default:
		if strings.HasPrefix(args[0], "-") {
			return "", unexpectedArgError("form", args[0])
		}
		return "", validationError("form", "Unknown form action: "+args[0])
	}
}

func handleFormClear(args []string, full bool) (string, error) {
	if len(args) == 0 {
		return "", validationError("form", "Missing element ref")
	}
	if len(args) > 1 {
		return "", unexpectedArgError("form", args[1])
	}
	uid, err := validateRefArg("form", args[0])
	if err != nil {
		return "", err
	}
	snap, err := callWithSnapshot("fill", map[string]any{"uid": uid, "value": ""})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, "form clear @"+uid, "", full), nil
}

func handleFormCheck(args []string, checked bool, full bool) (string, error) {
	if len(args) == 0 {
		return "", validationError("form", "Missing element ref")
	}
	if len(args) > 1 {
		return "", unexpectedArgError("form", args[1])
	}
	uid, err := validateRefArg("form", args[0])
	if err != nil {
		return "", err
	}

	elements := []map[string]any{{
		"uid":   uid,
		"type":  "checkbox",
		"value": checked,
	}}
	snap, err := callWithSnapshot("fill_form", map[string]any{"elements": elements})
	if err != nil {
		return "", err
	}

	action := "check"
	if !checked {
		action = "uncheck"
	}
	return formatPageOutput(snap, "form "+action+" @"+uid, "", full), nil
}

func handleFormSelect(args []string, full bool) (string, error) {
	if len(args) == 0 {
		return "", validationError("form", "Missing element ref")
	}
	uid, err := validateRefArg("form", args[0])
	if err != nil {
		return "", err
	}
	value := strings.Join(args[1:], " ")
	if value == "" {
		return "", validationError("form", "Missing select value")
	}

	elements := []map[string]any{{
		"uid":   uid,
		"type":  "select",
		"value": value,
	}}
	snap, err := callWithSnapshot("fill_form", map[string]any{"elements": elements})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, "form select @"+uid, "", full), nil
}

func handleFormUpload(args []string, full bool) (string, error) {
	if len(args) == 0 {
		return "", validationError("form", "Missing element ref")
	}
	if len(args) < 2 || args[1] == "" {
		return "", validationError("form", "Missing file path")
	}
	if len(args) > 2 {
		return "", unexpectedArgError("form", args[2])
	}
	uid, err := validateRefArg("form", args[0])
	if err != nil {
		return "", err
	}

	snap, err := callWithSnapshot("upload_file", map[string]any{"uid": uid, "filePath": args[1]})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, "form upload @"+uid, "", full), nil
}

func handlePages(args []string) (string, error) {
	if err := requireNoArgs("pages", args); err != nil {
		return "", err
	}
	result, err := callToolJSON("list_pages", map[string]any{})
	if err != nil {
		return "", err
	}
	pages := parsePagesResult(result)
	if len(pages) == 0 {
		return "pages: 0 pages open", nil
	}

	rows := make([]string, 0, len(pages))
	for _, page := range pages {
		rows = append(rows, fmt.Sprintf("  %d,%d,%s,%t", page.id, page.windowID, page.url, page.selected))
	}

	return joinBlocks(
		fmt.Sprintf("pages[%d]{id,window,url,selected}:\n%s", len(pages), strings.Join(rows, "\n")),
		renderHelp([]string{
			"Run `clawchrome-cli selectpage <id>` to switch tabs",
			"Run `clawchrome-cli newpage <url>` to open a new tab",
		}),
	), nil
}

func handleNewPage(args []string, full bool) (string, error) {
	var url string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return "", unexpectedArgError("newpage", arg)
		}
		if url != "" {
			return "", unexpectedArgError("newpage", arg)
		}
		url = arg
	}
	if url == "" {
		return "", validationError("newpage", "Missing URL")
	}

	toolArgs := map[string]any{"url": url}
	if _, err := callTool("new_page", toolArgs); err != nil {
		return "", err
	}

	snap, err := callTool("take_snapshot", map[string]any{})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snapshot.StripSnapshotHeader(snap), "newpage", url, full), nil
}

func handleSelectPage(args []string, full bool) (string, error) {
	if len(args) == 0 {
		return "", validationError("selectpage", "Missing page ID")
	}
	if len(args) > 1 {
		return "", unexpectedArgError("selectpage", args[1])
	}
	id := args[0]

	pageID, err := strconv.Atoi(id)
	if err != nil || pageID < 0 {
		return "", validationError("selectpage", "Invalid page ID: "+id)
	}

	if _, err := callTool("select_page", map[string]any{"pageId": pageID}); err != nil {
		return "", err
	}
	snap, err := callTool("take_snapshot", map[string]any{})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snapshot.StripSnapshotHeader(snap), "selectpage", "", full), nil
}

func handleClosePage(args []string) (string, error) {
	if len(args) == 0 {
		return "", validationError("closepage", "Missing page ID")
	}
	if len(args) > 1 {
		return "", unexpectedArgError("closepage", args[1])
	}
	id := args[0]

	pageID, err := strconv.Atoi(id)
	if err != nil || pageID < 0 {
		return "", validationError("closepage", "Invalid page ID: "+id)
	}

	before, err := callToolJSON("list_pages", map[string]any{})
	if err != nil {
		return "", err
	}
	if len(parsePagesResult(before)) <= 1 {
		return joinBlocks(
			encode(map[string]any{"status": "cannot close the last open page (no-op)"}),
			renderHelp([]string{
				"Run `clawchrome-cli newpage <url>` to open another tab first",
				"Run `clawchrome-cli stop` to shut down the browser entirely",
			}),
		), nil
	}

	if _, err := callTool("close_page", map[string]any{"pageId": pageID}); err != nil {
		return "", err
	}
	return encode(map[string]any{"status": "closed", "pageId": pageID}), nil
}

func handleResize(args []string) (string, error) {
	if len(args) < 2 {
		return "", validationError("resize", "Missing width and/or height")
	}
	if len(args) > 2 {
		return "", unexpectedArgError("resize", args[2])
	}

	width, widthErr := strconv.Atoi(args[0])
	height, heightErr := strconv.Atoi(args[1])
	if widthErr != nil || heightErr != nil {
		return "", validationError("resize", "Width and height must be numbers")
	}
	if width <= 0 || height <= 0 {
		return "", validationError("resize", "Width and height must be positive numbers")
	}

	if _, err := callTool("resize_page", map[string]any{"width": width, "height": height}); err != nil {
		return "", err
	}
	return encode(map[string]any{"resized": map[string]int{"width": width, "height": height}}), nil
}

func handleVideo(args []string) (string, error) {
	if len(args) == 0 {
		return "", validationError("video", "Missing video action")
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "start":
		return handleVideoStart(args[1:])
	case "stop":
		return handleVideoStop(args[1:])
	default:
		if strings.HasPrefix(args[0], "-") {
			return "", unexpectedArgError("video", args[0])
		}
		return "", validationError("video", "Unknown video action: "+args[0])
	}
}

func handleVideoStart(args []string) (string, error) {
	if len(args) > 1 {
		return "", unexpectedArgError("video", args[1])
	}
	pathArg := ""
	if len(args) == 1 {
		if strings.HasPrefix(args[0], "-") {
			return "", unexpectedArgError("video", args[0])
		}
		pathArg = args[0]
	}
	path, err := resolveOutputFilePath("video", pathArg, "clawchrome-cli-video-*.mp4")
	if err != nil {
		return "", err
	}
	toolArgs := map[string]any{"path": path}
	result, err := callToolJSON("screencast_start", toolArgs)
	if err != nil {
		return "", outputPathOperationError("start video recording", path, err)
	}
	payload := map[string]any{"video": map[string]any{"status": "started", "path": path}}
	if message := messageFromToolResult(result); message != "" {
		payload["video"].(map[string]any)["result"] = message
	}
	return encode(payload), nil
}

func handleVideoStop(args []string) (string, error) {
	if err := requireNoArgs("video", args); err != nil {
		return "", err
	}
	result, err := callToolJSON("screencast_stop", map[string]any{})
	if err != nil {
		return "", err
	}
	payload := map[string]any{"video": map[string]any{"status": "stopped"}}
	if message := messageFromToolResult(result); message != "" {
		payload["video"].(map[string]any)["result"] = message
	}
	return encode(payload), nil
}

func handleSelfUpdate(args []string) (string, error) {
	if len(args) > 1 {
		return "", unexpectedArgError("self-update", args[1])
	}
	if len(args) == 1 && strings.HasPrefix(args[0], "-") {
		return "", unexpectedArgError("self-update", args[0])
	}
	targetVersion := firstPositionalArg(args)
	version, err := selfUpdate(Version, targetVersion)
	if err != nil {
		return "", client.WrapError(err.Error(), client.ErrUnknown)
	}
	return encode(map[string]any{
		"status":  "updated",
		"version": version,
	}), nil
}

func handleStop(args []string) (string, error) {
	if err := requireNoArgs("stop", args); err != nil {
		return "", err
	}
	return encode(map[string]any{"status": stopText(stopBridge())}), nil
}

func handleVersion(args []string) (string, error) {
	if err := requireNoArgs("version", args); err != nil {
		return "", err
	}
	return Version, nil
}

func handleSnapshotCommand(command string, args []string, full bool) (string, error) {
	if len(args) == 0 {
		return "", validationError(command, "Missing element ref")
	}
	if len(args) > 1 {
		return "", unexpectedArgError(command, args[1])
	}
	uid, err := validateRefArg(command, args[0])
	if err != nil {
		return "", err
	}

	snap, err := callWithSnapshot(command, map[string]any{"uid": uid})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, command, "", full), nil
}

func callWithSnapshot(name string, args map[string]any) (string, error) {
	callArgs := map[string]any{}
	for k, v := range args {
		callArgs[k] = v
	}
	callArgs["includeSnapshot"] = true

	result, err := callToolJSON(name, callArgs)
	if err != nil {
		return "", err
	}
	if parsed := snapshotFromToolResult(result); parsed != "" {
		return snapshot.StripSnapshotHeader(parsed), nil
	}

	fallback, err := callToolJSON("take_snapshot", map[string]any{})
	if err != nil {
		return "", err
	}
	return snapshot.StripSnapshotHeader(snapshotFromToolResult(fallback)), nil
}

func snapshotFromToolResult(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var legacy string
	if err := json.Unmarshal(raw, &legacy); err == nil {
		if parsed := parseSnapshotFromResponse(legacy); parsed != "" {
			return parsed
		}
		return legacy
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	for _, key := range []string{"snapshotYaml", "text"} {
		if value := rawStringField(obj, key); value != "" {
			return value
		}
	}
	if dataRaw, ok := obj["data"]; ok {
		if value := snapshotFromToolResult(dataRaw); value != "" {
			return value
		}
	}
	return ""
}

func rawStringField(obj map[string]json.RawMessage, key string) string {
	raw, ok := obj[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func messageFromToolResult(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var legacy string
	if err := json.Unmarshal(raw, &legacy); err == nil {
		return strings.TrimSpace(legacy)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	for _, key := range []string{"message", "text"} {
		if value := rawStringField(obj, key); value != "" {
			return value
		}
	}
	return ""
}

func parseSnapshotFromResponse(text string) string {
	const marker = "## Latest page snapshot"
	idx := strings.Index(text, marker)
	if idx == -1 {
		return ""
	}
	after := strings.TrimLeft(text[idx+len(marker):], "\n")
	if next := strings.Index(after, "\n## "); next >= 0 {
		return strings.TrimRight(after[:next], "\n")
	}
	return strings.TrimRight(after, "\n")
}

func parseUID(uid string) string {
	return strings.TrimPrefix(uid, "@")
}

func stopText(stopped bool) string {
	if stopped {
		return "stopped"
	}
	return "stopped (no-op)"
}

func renderError(err error) string {
	if cdpErr, ok := err.(*client.CdpError); ok {
		return joinBlocks(
			encode(map[string]any{"error": cdpErr.Message, "code": string(cdpErr.Code)}),
			renderHelp(cdpErr.Suggestions),
		)
	}
	return encode(map[string]any{"error": err.Error(), "code": string(client.ErrUnknown)})
}

func renderCommandError(command string, err error) string {
	if cdpErr, ok := err.(*client.CdpError); ok {
		blocks := []string{encode(map[string]any{"error": cdpErr.Message, "code": string(cdpErr.Code)})}
		if cdpErr.Code == client.ErrValidation {
			if help := getCommandHelp(command); help != "" {
				blocks = append(blocks, help)
			}
		}
		blocks = append(blocks, renderHelp(cdpErr.Suggestions))
		return joinBlocks(blocks...)
	}
	return renderError(err)
}

func validationError(command string, message string, suggestions ...string) error {
	return client.WrapError(message, client.ErrValidation, suggestions...)
}

func unexpectedArgError(command string, arg string) error {
	if strings.HasPrefix(arg, "-") {
		return validationError(command, "Unexpected flag: "+arg)
	}
	return validationError(command, "Unexpected argument: "+arg)
}

func requireNoArgs(command string, args []string) error {
	if len(args) > 0 {
		return unexpectedArgError(command, args[0])
	}
	return nil
}

func validateRefArg(command string, ref string) (string, error) {
	if !refArgPattern.MatchString(ref) {
		return "", validationError(command, "Invalid element ref: expected a snapshot ref such as @1")
	}
	return parseUID(ref), nil
}

func isHelpArg(arg string) bool {
	return arg == "-h" || arg == "--help"
}

func hasHelpArg(args []string) bool {
	for _, arg := range args {
		if isHelpArg(arg) {
			return true
		}
	}
	return false
}

func exitCode(err error) int {
	if cdpErr, ok := err.(*client.CdpError); ok && cdpErr.Code == client.ErrValidation {
		return 2
	}
	return 1
}

func writeUpdateNotice(stderr io.Writer, command string) {
	if stderr == nil {
		return
	}
	switch command {
	case "--help", "version", "self-update":
		return
	}
	notice, err := updateNotice(Version)
	if err != nil || notice == "" {
		return
	}
	_, _ = io.WriteString(stderr, notice+"\n")
}

func firstPositionalArg(args []string) string {
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			if args[i] == "--format" {
				i++
			}
			continue
		}
		return args[i]
	}
	return ""
}

func isDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isRecoverableOpenError(err error) bool {
	cdpErr, ok := err.(*client.CdpError)
	if !ok || cdpErr.Code != client.ErrBrowser {
		return false
	}
	return recoverableOpenErrorPattern.MatchString(cdpErr.Message)
}

func encode(v any) string {
	return toonout.Encode(v)
}

func renderHelp(lines []string) string {
	return toonout.RenderHelp(lines)
}

func joinBlocks(blocks ...string) string {
	return toonout.JoinBlocks(blocks...)
}
