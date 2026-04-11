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
	selfUpdate                  = func(currentVersion string, targetVersion string) (string, error) {
		return update.SelfUpdate(context.Background(), currentVersion, targetVersion)
	}
	updateNotice = func(currentVersion string) (string, error) {
		return update.Notice(context.Background(), currentVersion)
	}
)

var recoverableOpenErrorPattern = regexp.MustCompile(`(?i)not connected|session (?:closed|not found)|no page`)

func Main(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && args[0] == "--full") {
		output := renderHome()
		_, _ = io.WriteString(stdout, output+"\n")
		writeUpdateNotice(stderr, "")
		return 0
	}

	if len(args) == 1 {
		switch args[0] {
		case "--help":
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

	if len(args) >= 2 && args[1] == "--help" {
		if help := getCommandHelp(args[0]); help != "" {
			_, _ = io.WriteString(stdout, help+"\n")
			return 0
		}
		_, _ = io.WriteString(stdout, renderError(client.WrapError("Unknown command: "+args[0], client.ErrValidation, "Run `clawchrome-cli --help` to see available commands"))+"\n")
		return 2
	}

	command := args[0]
	commandArgs, full := splitFullFlag(args[1:])

	output, err := runCommand(command, commandArgs, full)
	if err != nil {
		_, _ = io.WriteString(stdout, renderError(err)+"\n")
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
		return handleSnapshot(full)
	case "screenshot":
		return handleScreenshot(args)
	case "click":
		return handleSnapshotCommand("click", args, full, "Run `clawchrome-cli click @<uid>` — get uid from snapshot")
	case "fill":
		return handleFill(args, full)
	case "type":
		return handleType(args, full)
	case "press":
		return handlePress(args, full)
	case "scroll":
		return handleScroll(args, full)
	case "back":
		return handleBack(full)
	case "wait":
		return handleWait(args)
	case "hover":
		return handleSnapshotCommand("hover", args, full, "Run `clawchrome-cli hover @<uid>` — get uid from snapshot")
	case "drag":
		return handleDrag(args, full)
	case "fillform":
		return handleFillForm(args, full)
	case "dialog":
		return handleDialog(args)
	case "upload":
		return handleUpload(args, full)
	case "pages":
		return handlePages()
	case "newpage":
		return handleNewPage(args, full)
	case "selectpage":
		return handleSelectPage(args, full)
	case "closepage":
		return handleClosePage(args)
	case "resize":
		return handleResize(args)
	case "start":
		port, err := ensureBridge()
		if err != nil {
			return "", err
		}
		return encode(map[string]any{"status": "ready", "port": port}), nil
	case "stop":
		return encode(map[string]any{"status": stopText(stopBridge())}), nil
	case "version":
		return Version, nil
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

func handleOpen(args []string, full bool) (string, error) {
	url := firstPositionalArg(args)
	if url == "" {
		return "", client.WrapError("Missing URL", client.ErrValidation, "Run `clawchrome-cli open https://example.com` to navigate to a page")
	}

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

func handleSnapshot(full bool) (string, error) {
	snap, err := callTool("take_snapshot", map[string]any{})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snapshot.StripSnapshotHeader(snap), "snapshot", "", full), nil
}

func handleScreenshot(args []string) (string, error) {
	parsed := parseScreenshotArgs(args)
	if parsed.filePath == "" {
		return "", client.WrapError("Missing file path", client.ErrValidation, "Run `clawchrome-cli screenshot ./page.png` to save a screenshot")
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
	uid := firstPositionalArg(args)
	if uid == "" {
		return "", client.WrapError("Missing element ref", client.ErrValidation, "Run `clawchrome-cli fill @<uid> \"text\"` to fill the field")
	}

	value := strings.Join(args[1:], " ")
	if value == "" {
		return "", client.WrapError("Missing fill text", client.ErrValidation, "Run `clawchrome-cli fill @<uid> \"text\"` to fill the field")
	}

	snap, err := callWithSnapshot("fill", map[string]any{"uid": parseUID(uid), "value": value})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, "fill", "", full), nil
}

func handleType(args []string, full bool) (string, error) {
	text := strings.Join(args, " ")
	if text == "" {
		return "", client.WrapError("Missing text", client.ErrValidation, "Run `clawchrome-cli type \"hello\"` to type text")
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
	key := firstPositionalArg(args)
	if key == "" {
		return "", client.WrapError("Missing key name", client.ErrValidation, "Run `clawchrome-cli press Enter` to press a key")
	}

	snap, err := callWithSnapshot("press_key", map[string]any{"key": key})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, "press", "", full), nil
}

func handleScroll(args []string, full bool) (string, error) {
	dir := strings.ToLower(firstPositionalArg(args))
	if dir == "" {
		dir = "down"
	}

	fn, ok := map[string]string{
		"up":     "window.scrollBy(0, -500)",
		"down":   "window.scrollBy(0, 500)",
		"top":    "window.scrollTo(0, 0)",
		"bottom": "window.scrollTo(0, document.body.scrollHeight)",
	}[dir]
	if !ok {
		return "", client.WrapError("Unknown scroll direction: "+dir, client.ErrValidation, "Run `clawchrome-cli scroll down` - directions: up, down, top, bottom")
	}

	// TODO: need to change to native api call not js.
	if _, err := callTool("evaluate_script", map[string]any{"function": fn}); err != nil {
		return "", err
	}
	snap, err := callTool("take_snapshot", map[string]any{})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snapshot.StripSnapshotHeader(snap), "scroll", "", full), nil
}

func handleBack(full bool) (string, error) {
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
	target := firstPositionalArg(args)
	if target == "" {
		return "", client.WrapError(
			"Missing wait target (milliseconds or text)",
			client.ErrValidation,
			"Run `clawchrome-cli wait 2000` to wait 2 seconds",
			"Run `clawchrome-cli wait \"Submit\"` to wait for text to appear",
		)
	}

	if isDigits(target) {
		// TODO: need to change to native api call not js.
		if _, err := callTool("evaluate_script", map[string]any{"function": fmt.Sprintf("new Promise(r => setTimeout(r, %s))", target)}); err != nil {
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
		return "", client.WrapError("Missing element refs", client.ErrValidation, "Run `clawchrome-cli drag @<from> @<to>` — get uids from snapshot")
	}

	snap, err := callWithSnapshot("drag", map[string]any{"from_uid": parseUID(args[0]), "to_uid": parseUID(args[1])})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, "drag", "", full), nil
}

func handleFillForm(args []string, full bool) (string, error) {
	parsed := parseFillFormArgs(args)
	if len(parsed.entries) == 0 {
		return "", client.WrapError("No valid field entries", client.ErrValidation, "Run `clawchrome-cli fillform @1=\"hello\" @2=\"world\"` to fill multiple fields")
	}

	snap, err := callWithSnapshot("fill_form", map[string]any{"elements": parsed.entries})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, "fillform", "", full), nil
}

func handleDialog(args []string) (string, error) {
	action := firstPositionalArg(args)
	if action == "" || (action != "accept" && action != "dismiss") {
		return "", client.WrapError("Missing or invalid action", client.ErrValidation, "Run `clawchrome-cli dialog accept` or `clawchrome-cli dialog dismiss`")
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
	uid := firstPositionalArg(args)
	if uid == "" {
		return "", client.WrapError("Missing element ref", client.ErrValidation, "Run `clawchrome-cli upload @<uid> <path>` — get uid from snapshot")
	}
	if len(args) < 2 || args[1] == "" {
		return "", client.WrapError("Missing file path", client.ErrValidation, "Run `clawchrome-cli upload @<uid> /path/to/file` to upload a file")
	}

	snap, err := callWithSnapshot("upload_file", map[string]any{"uid": parseUID(uid), "filePath": args[1]})
	if err != nil {
		return "", err
	}
	return formatPageOutput(snap, "upload", "", full), nil
}

func handlePages() (string, error) {
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
	url := firstPositionalArg(args)
	if url == "" {
		return "", client.WrapError("Missing URL", client.ErrValidation, "Run `clawchrome-cli newpage https://example.com` to open a new tab")
	}

	toolArgs := map[string]any{"url": url}
	if hasArg(args, "--background") {
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
	id := firstPositionalArg(args)
	if id == "" {
		return "", client.WrapError("Missing page ID", client.ErrValidation, "Run `clawchrome-cli selectpage <id>` — get ID from `pages` command")
	}

	pageID, err := strconv.Atoi(id)
	if err != nil {
		return "", client.WrapError("Invalid page ID: "+id, client.ErrValidation, "Run `clawchrome-cli pages` to list available page IDs")
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
	id := firstPositionalArg(args)
	if id == "" {
		return "", client.WrapError("Missing page ID", client.ErrValidation, "Run `clawchrome-cli closepage <id>` — get ID from `pages` command")
	}

	pageID, err := strconv.Atoi(id)
	if err != nil {
		return "", client.WrapError("Invalid page ID: "+id, client.ErrValidation, "Run `clawchrome-cli pages` to list available page IDs")
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
		return "", client.WrapError("Missing width and/or height", client.ErrValidation, "Run `clawchrome-cli resize 1280 720` to resize the viewport")
	}

	width, widthErr := strconv.Atoi(args[0])
	height, heightErr := strconv.Atoi(args[1])
	if widthErr != nil || heightErr != nil {
		return "", client.WrapError("Width and height must be numbers", client.ErrValidation, "Run `clawchrome-cli resize 1280 720` to resize the viewport")
	}

	if _, err := callTool("resize_page", map[string]any{"width": width, "height": height}); err != nil {
		return "", err
	}
	return encode(map[string]any{"resized": map[string]int{"width": width, "height": height}}), nil
}

func handleSelfUpdate(args []string) (string, error) {
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

func handleSnapshotCommand(command string, args []string, full bool, missingRefHelp string) (string, error) {
	uid := firstPositionalArg(args)
	if uid == "" {
		return "", client.WrapError("Missing element ref", client.ErrValidation, missingRefHelp)
	}

	snap, err := callWithSnapshot(command, map[string]any{"uid": parseUID(uid)})
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
			encode(map[string]any{"error": cdpErr.Message, "code": cdpErr.Code}),
			renderHelp(cdpErr.Suggestions),
		)
	}
	return encode(map[string]any{"error": err.Error(), "code": client.ErrUnknown})
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

func hasArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
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
