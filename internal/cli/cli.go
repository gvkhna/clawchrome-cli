package cli

import (
	"context"
	"fmt"
	"io"
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
	ensureBridge                = client.EnsureBridge
	getSessionSnapshotIfRunning = client.GetSessionSnapshotIfRunning
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
	case "back":
		return handleBack(args, full)
	case "wait":
		return handleWait(args)
	case "hover":
		return handleSnapshotCommand("hover", args, full)
	case "drag":
		return handleDrag(args, full)
	case "fillform":
		return handleFillForm(args, full)
	case "dialog":
		return handleDialog(args)
	case "upload":
		return handleUpload(args, full)
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
	case "start":
		return handleStart(args)
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
	var url string
	switch len(args) {
	case 0:
	case 1:
		if strings.HasPrefix(args[0], "-") {
			return "", unexpectedArgError("start", args[0])
		}
		url = args[0]
	default:
		return "", unexpectedArgError("start", args[1])
	}

	port, err := ensureBridge()
	if err != nil {
		return "", err
	}
	if usesHTTPTransport() {
		toolArgs := map[string]any{}
		if url != "" {
			toolArgs["url"] = url
		}
		if _, err := callTool("start_browser", toolArgs); err != nil {
			return "", err
		}
	}
	return encode(map[string]any{"status": "ready", "port": port}), nil
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
	if full {
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
	if parsed.filePath == "" {
		return "", validationError("screenshot", "Missing file path")
	}

	toolArgs := map[string]any{"filePath": parsed.filePath}
	if parsed.uid != "" {
		toolArgs["uid"] = parsed.uid
	}
	if parsed.fullPage {
		toolArgs["fullPage"] = true
	}
	if parsed.format != "" {
		toolArgs["format"] = parsed.format
	}

	if _, err := callTool("take_screenshot", toolArgs); err != nil {
		return "", err
	}
	return encode(map[string]any{"screenshot": parsed.filePath}), nil
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

	if _, err := callTool("scroll_page", map[string]any{"direction": dir}); err != nil {
		return "", err
	}
	snap, err := callTool("take_snapshot", map[string]any{})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snapshot.StripSnapshotHeader(snap), "scroll", "", full), nil
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

func handleUpload(args []string, full bool) (string, error) {
	if len(args) == 0 {
		return "", validationError("upload", "Missing element ref")
	}
	if len(args) < 2 || args[1] == "" {
		return "", validationError("upload", "Missing file path")
	}
	if len(args) > 2 {
		return "", unexpectedArgError("upload", args[2])
	}
	uid, err := validateRefArg("upload", args[0])
	if err != nil {
		return "", err
	}

	snap, err := callWithSnapshot("upload_file", map[string]any{"uid": uid, "filePath": args[1]})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, "upload", "", full), nil
}

func handlePages(args []string) (string, error) {
	if err := requireNoArgs("pages", args); err != nil {
		return "", err
	}
	result, err := callTool("list_pages", map[string]any{})
	if err != nil {
		return "", err
	}
	pages := parsePagesList(result)
	if len(pages) == 0 {
		return "pages: 0 pages open", nil
	}

	rows := make([]string, 0, len(pages))
	for _, page := range pages {
		rows = append(rows, fmt.Sprintf("  %d,%s,%t", page.id, page.url, page.selected))
	}

	return joinBlocks(
		fmt.Sprintf("pages[%d]{id,url,selected}:\n%s", len(pages), strings.Join(rows, "\n")),
		renderHelp([]string{
			"Run `clawchrome-cli selectpage <id>` to switch tabs",
			"Run `clawchrome-cli newpage <url>` to open a new tab",
		}),
	), nil
}

func handleNewPage(args []string, full bool) (string, error) {
	var url string
	background := false
	for _, arg := range args {
		switch arg {
		case "--background":
			background = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", unexpectedArgError("newpage", arg)
			}
			if url != "" {
				return "", unexpectedArgError("newpage", arg)
			}
			url = arg
		}
	}
	if url == "" {
		return "", validationError("newpage", "Missing URL")
	}

	toolArgs := map[string]any{"url": url}
	if background {
		toolArgs["background"] = true
	}
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

	before, err := callTool("list_pages", map[string]any{})
	if err != nil {
		return "", err
	}
	if len(parsePagesList(before)) <= 1 {
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

	result, err := callTool(name, callArgs)
	if err != nil {
		return "", err
	}
	if parsed := parseSnapshotFromResponse(result); parsed != "" {
		return snapshot.StripSnapshotHeader(parsed), nil
	}

	fallback, err := callTool("take_snapshot", map[string]any{})
	if err != nil {
		return "", err
	}
	return snapshot.StripSnapshotHeader(fallback), nil
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
			if args[i] == "--uid" || args[i] == "--format" {
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
