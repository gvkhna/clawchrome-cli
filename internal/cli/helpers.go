package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gvkhna/clawchrome-cli/internal/snapshot"
	"github.com/gvkhna/clawchrome-cli/internal/suggestions"
)

const topHelp = `usage: clawchrome-cli [command] [args] [flags]

commands:
  open <url>, snapshot [--form] [--text], screenshot [path], click @<uid>, fill @<uid> <text>,
  type <text>, press <key>, scroll <dir>, mouse <action>, back, forward, reload, wait <ms|text>,
  hover @<uid>, drag @<from> @<to>, fillform @<uid>=<val>..., dialog <action>,
  form <clear|check|uncheck|select|upload>, pages, newpage <url>, selectpage <id>, closepage <id>,
  resize <w> <h>, video <start|stop>, start, status, stop, version, self-update [version]

flags:
  --help, -v, -V, --version, --full

environment:
  CLAWCHROME_CLI_TRANSPORT=stdio   Optional local stdio transport override
  CLAWCHROME_CLI_HTTP_URL=...      Optional runtime API URL override (default: https://www.clawchrome.com)
  CLAWCHROME_CLI_HTTP_BEARER_TOKEN Optional auth token override
  CLAWCHROME_CLI_HEADED=1          Run Chrome headed instead of headless
  CLAWCHROME_CLI_CHROME_ARGS=...   Forward additional Chrome flags
  CLAWCHROME_CLI_PORT=9224         Override bridge server port
`

var commandHelp = map[string]string{
	"open": `usage: clawchrome-cli open <url> [--full]
Navigate to a URL and capture an accessibility snapshot.

args:
  <url>   URL to navigate to (required)

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli open https://example.com
  clawchrome-cli open https://example.com --full`,
	"screenshot": `usage: clawchrome-cli screenshot [path] [--uid @<uid>] [--full-page] [--format png|jpeg|webp]
Save a screenshot to a file.

args:
  [path]  File path to save the screenshot. Defaults to a unique file in the system temp directory.

flags:
  --uid @<uid>    Capture a specific element instead of the full viewport
  --full-page     Capture the entire scrollable page
  --format <fmt>  Image format: png (default), jpeg, or webp

examples:
  clawchrome-cli screenshot
  clawchrome-cli screenshot ./page.png
  clawchrome-cli screenshot --uid @3
  clawchrome-cli screenshot ./full.jpeg --full-page --format jpeg`,
	"snapshot": `usage: clawchrome-cli snapshot [--form] [--text] [--full]
Capture the current page accessibility snapshot.

flags:
  --form  Request a snapshot filtered to form controls
  --text  Request a snapshot filtered to readable text content
  --full  Request the full accessibility tree and show output without truncation

examples:
  clawchrome-cli snapshot
  clawchrome-cli snapshot --form
  clawchrome-cli snapshot --text
  clawchrome-cli snapshot --full`,
	"click": `usage: clawchrome-cli click @<uid> [--full]
Click an interactive element by its ref from the snapshot.

args:
  @<uid>  Element ref from snapshot (required)

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli click @1
  clawchrome-cli click @12 --full`,
	"fill": `usage: clawchrome-cli fill @<uid> <text> [--full]
Fill a form field with text.

args:
  @<uid>  Element ref from snapshot (required)
  <text>  Text to fill (required)

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli fill @3 "hello world"
  clawchrome-cli fill @3 "search query" --full`,
	"type": `usage: clawchrome-cli type <text> [--full]
Type text at the currently focused element.

args:
  <text>  Text to type (required)

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli type "hello"
  clawchrome-cli type "search query" --full`,
	"press": `usage: clawchrome-cli press <key> [--full]
Press a keyboard key.

args:
  <key>  Key name, e.g. Enter, Tab, Escape, ArrowDown (required)

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli press Enter
  clawchrome-cli press Tab --full`,
	"scroll": `usage: clawchrome-cli scroll <direction> [--full]
Scroll the page in a direction using page navigation keys.

args:
  <direction>  up, down, top, or bottom (default: down)

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli scroll down
  clawchrome-cli scroll top --full`,
	"mouse": `usage: clawchrome-cli mouse <action> [args]
Perform low-level mouse actions through runtime HTTP.

args:
  move <x> <y>                         Move the pointer to page coordinates
  click <x> <y>                        Left-click at page coordinates
  drag <startX> <startY> <endX> <endY> Drag between page coordinates
  down [left|right|middle]             Press a mouse button (default: left)
  up [left|right|middle]               Release a mouse button (default: left)
  wheel <deltaX> <deltaY>              Scroll the mouse wheel by explicit deltas

examples:
  clawchrome-cli mouse move 120 240
  clawchrome-cli mouse click 120 240
  clawchrome-cli mouse wheel 0 500`,
	"back": `usage: clawchrome-cli back [--full]
Navigate back in browser history.

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli back
  clawchrome-cli back --full`,
	"forward": `usage: clawchrome-cli forward [--full]
Navigate forward in browser history.

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli forward
  clawchrome-cli forward --full`,
	"reload": `usage: clawchrome-cli reload [--full]
Reload the current page.

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli reload
  clawchrome-cli reload --full`,
	"wait": `usage: clawchrome-cli wait <ms|text>
Wait for a duration or for text to appear on the page.

args:
  <ms>    Milliseconds to wait (numeric)
  <text>  Text to wait for (string)

examples:
  clawchrome-cli wait 2000
  clawchrome-cli wait "Submit"`,
	"start": `usage: clawchrome-cli start [url] [--token <token>] [--agent-name <name>]
Start the bridge server (launches headless Chrome).

args:
  [url]  Optional URL to open when using the http runtime transport

flags:
  --token <token>       Save an HTTP bearer token in the user config directory
  --agent-name <name>   Save an agent name sent with runtime HTTP requests

examples:
  clawchrome-cli start
  clawchrome-cli start --token <token> --agent-name codex-worker
  clawchrome-cli start https://example.com`,
	"status": `usage: clawchrome-cli status
Show local CLI transport and auth status without printing the token.

examples:
  clawchrome-cli status`,
	"stop": `usage: clawchrome-cli stop
Stop the bridge server and close the browser.

examples:
  clawchrome-cli stop`,
	"version": `usage: clawchrome-cli version
Print the current clawchrome-cli version.

examples:
  clawchrome-cli version`,
	"self-update": `usage: clawchrome-cli self-update [version]
Download and replace the current binary with the latest release or a specific version.

args:
  [version]  Optional release tag such as v0.1.0

examples:
  clawchrome-cli self-update
  clawchrome-cli self-update v0.1.0`,
	"pages": `usage: clawchrome-cli pages
List all open pages/tabs in the browser.

examples:
  clawchrome-cli pages`,
	"newpage": `usage: clawchrome-cli newpage <url> [--full]
Open a new tab and navigate to a URL.

args:
  <url>  URL to open (required)

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli newpage https://example.com`,
	"selectpage": `usage: clawchrome-cli selectpage <id> [--full]
Switch to a tab by page ID.

args:
  <id>  Page ID from the pages command (required)

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli selectpage 1`,
	"closepage": `usage: clawchrome-cli closepage <id>
Close a tab by page ID. The last open page cannot be closed.

args:
  <id>  Page ID from the pages command (required)

examples:
  clawchrome-cli closepage 2`,
	"resize": `usage: clawchrome-cli resize <width> <height>
Resize the browser viewport.

args:
  <width>   Width in pixels (required)
  <height>  Height in pixels (required)

examples:
  clawchrome-cli resize 1280 720
  clawchrome-cli resize 390 844`,
	"video": `usage: clawchrome-cli video <start|stop> [path]
Start or stop page video recording.

args:
  <start|stop>  Video action to perform (required)
  [path]        Output path for video start. Defaults to a unique .mp4 file in the system temp directory.

examples:
  clawchrome-cli video start
  clawchrome-cli video start ./capture.mp4
  clawchrome-cli video stop`,
	"hover": `usage: clawchrome-cli hover @<uid> [--full]
Hover over an element to trigger hover states.

args:
  @<uid>  Element ref from snapshot (required)

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli hover @5`,
	"drag": `usage: clawchrome-cli drag @<from> @<to> [--full]
Drag an element onto another element.

args:
  @<from>  Element to drag (required)
  @<to>    Element to drop onto (required)

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli drag @3 @7`,
	"fillform": `usage: clawchrome-cli fillform @<uid>=<value>... [--full]
Fill multiple form fields at once.

args:
  @<uid>=<value>  One or more field entries (required)

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli fillform @1="hello" @2="world"
  clawchrome-cli fillform @3="user@email.com" @4="password123"`,
	"dialog": `usage: clawchrome-cli dialog <accept|dismiss> [text]
Handle a browser dialog (alert, confirm, prompt).

args:
  <action>  accept or dismiss (required)
  [text]    Optional text to enter into a prompt dialog

examples:
  clawchrome-cli dialog accept
  clawchrome-cli dialog dismiss
  clawchrome-cli dialog accept "confirmed"`,
	"form": `usage: clawchrome-cli form <action> [args] [--full]
Perform a targeted form control action.

args:
  clear @<uid>           Clear an input or text area
  check @<uid>            Check a checkbox or radio control
  uncheck @<uid>          Uncheck a checkbox control
  select @<uid> <value>   Select an option by value or label
  upload @<uid> <path>    Upload a file through a file input

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli form clear @2
  clawchrome-cli form check @3
  clawchrome-cli form select @4 "United States"
  clawchrome-cli form upload @5 ./photo.jpg`,
}

var commandSupportsFullFlag = map[string]bool{
	"open":       true,
	"snapshot":   true,
	"click":      true,
	"fill":       true,
	"type":       true,
	"press":      true,
	"scroll":     true,
	"back":       true,
	"forward":    true,
	"reload":     true,
	"hover":      true,
	"drag":       true,
	"fillform":   true,
	"form":       true,
	"newpage":    true,
	"selectpage": true,
}

var commandOrder = []string{
	"open",
	"snapshot",
	"screenshot",
	"click",
	"fill",
	"type",
	"press",
	"scroll",
	"mouse",
	"back",
	"forward",
	"reload",
	"wait",
	"hover",
	"drag",
	"fillform",
	"dialog",
	"form",
	"pages",
	"newpage",
	"selectpage",
	"closepage",
	"resize",
	"video",
	"start",
	"status",
	"stop",
	"version",
	"self-update",
}

type screenshotArgs struct {
	filePath string
	uid      string
	fullPage bool
	format   string
	invalid  string
}

type ScreenshotArgs struct {
	FilePath string
	UID      string
	FullPage bool
	Format   string
}

type PageInfo struct {
	ID       int
	WindowID int
	URL      string
	Selected bool
}

type pageInfo struct {
	id       int
	windowID int
	url      string
	selected bool
}

type fillFormArgs struct {
	entries []map[string]string
	invalid string
}

type snapshotArgs struct {
	form    bool
	text    bool
	invalid string
}

func getCommandHelp(command string) string {
	return commandHelp[command]
}

func CommandHelpText(command string) (string, bool) {
	text, ok := commandHelp[command]
	return text, ok
}

func CommandSupportsFullFlag(command string) bool {
	return commandSupportsFullFlag[command]
}

func SupportedCommands() []string {
	out := make([]string, len(commandOrder))
	copy(out, commandOrder)
	return out
}

func splitFullFlag(args []string) ([]string, bool) {
	filtered := make([]string, 0, len(args))
	full := false
	for _, arg := range args {
		if arg == "--full" {
			full = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered, full
}

func ParseScreenshotArgs(args []string) ScreenshotArgs {
	var parsed ScreenshotArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--uid":
			if i+1 < len(args) {
				parsed.UID = parseUID(args[i+1])
				i++
			}
		case "--full-page":
			parsed.FullPage = true
		case "--format":
			if i+1 < len(args) {
				parsed.Format = args[i+1]
				i++
			}
		default:
			if !strings.HasPrefix(args[i], "--") {
				parsed.FilePath = args[i]
			}
		}
	}
	return parsed
}

func parseScreenshotArgs(args []string) screenshotArgs {
	var parsed screenshotArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--uid":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				parsed.invalid = "Missing value for --uid"
				return parsed
			}
			uid, err := validateRefArg("screenshot", args[i+1])
			if err != nil {
				parsed.invalid = "Invalid element ref: expected a snapshot ref such as @1"
				return parsed
			}
			parsed.uid = uid
			i++
		case "--full-page":
			parsed.fullPage = true
		case "--format":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				parsed.invalid = "Missing value for --format"
				return parsed
			}
			switch args[i+1] {
			case "png", "jpeg", "webp":
				parsed.format = args[i+1]
				i++
			default:
				parsed.invalid = "Invalid screenshot format: " + args[i+1]
				return parsed
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				parsed.invalid = "Unexpected flag: " + args[i]
				return parsed
			}
			if parsed.filePath != "" {
				parsed.invalid = "Unexpected argument: " + args[i]
				return parsed
			}
			parsed.filePath = args[i]
		}
	}
	return parsed
}

func parseSnapshotArgs(args []string) snapshotArgs {
	var parsed snapshotArgs
	for _, arg := range args {
		switch arg {
		case "--form":
			parsed.form = true
		case "--text":
			parsed.text = true
		default:
			parsed.invalid = arg
			return parsed
		}
	}
	return parsed
}

func ParsePagesList(text string) []PageInfo {
	internal := parsePagesList(text)
	pages := make([]PageInfo, 0, len(internal))
	for _, page := range internal {
		pages = append(pages, PageInfo{ID: page.id, WindowID: page.windowID, URL: page.url, Selected: page.selected})
	}
	return pages
}

func ParsePagesResult(raw json.RawMessage) []PageInfo {
	internal := parsePagesResult(raw)
	pages := make([]PageInfo, 0, len(internal))
	for _, page := range internal {
		pages = append(pages, PageInfo{ID: page.id, WindowID: page.windowID, URL: page.url, Selected: page.selected})
	}
	return pages
}

func parsePagesResult(raw json.RawMessage) []pageInfo {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var legacy string
	if err := json.Unmarshal(raw, &legacy); err == nil {
		return parsePagesList(legacy)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	if pages := parsePageArray(obj["pages"], cliIntValue(obj["activePageId"])); len(pages) > 0 {
		return pages
	}
	if structured, ok := cliMapValue(obj["structuredContent"]); ok {
		if pages := parsePageArray(structured["pages"], cliIntValue(structured["activePageId"])); len(pages) > 0 {
			return pages
		}
	}
	return parseTabArray(obj["tabs"], strings.TrimSpace(cliStringValue(obj["activeId"])))
}

func parsePageArray(raw any, activePageID int) []pageInfo {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	pages := make([]pageInfo, 0, len(items))
	for _, item := range items {
		obj, ok := cliMapValue(item)
		if !ok {
			continue
		}
		id := firstPositiveCLIInt(obj["pageId"], obj["id"], obj["tabId"])
		if id <= 0 {
			continue
		}
		page := pageInfo{
			id:       id,
			windowID: firstPositiveCLIInt(obj["windowId"]),
			url:      strings.TrimSpace(cliStringValue(obj["url"])),
			selected: cliBoolValue(obj["selected"]) || (activePageID > 0 && id == activePageID),
		}
		pages = append(pages, page)
	}
	return pages
}

func parseTabArray(raw any, activeID string) []pageInfo {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	pages := make([]pageInfo, 0, len(items))
	for _, item := range items {
		obj, ok := cliMapValue(item)
		if !ok {
			continue
		}
		id := firstPositiveCLIInt(obj["pageId"], obj["tabId"])
		if id <= 0 {
			continue
		}
		handle := strings.TrimSpace(cliStringValue(obj["id"]))
		page := pageInfo{
			id:       id,
			windowID: firstPositiveCLIInt(obj["windowId"]),
			url:      strings.TrimSpace(cliStringValue(obj["url"])),
			selected: cliBoolValue(obj["focused"]) || (handle != "" && handle == activeID),
		}
		pages = append(pages, page)
	}
	return pages
}

func cliMapValue(value any) (map[string]any, bool) {
	obj, ok := value.(map[string]any)
	return obj, ok
}

func firstPositiveCLIInt(values ...any) int {
	for _, value := range values {
		if out := cliIntValue(value); out > 0 {
			return out
		}
	}
	return 0
}

func cliIntValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		if typed == float64(int(typed)) {
			return int(typed)
		}
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func cliStringValue(value any) string {
	text, _ := value.(string)
	return text
}

func cliBoolValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func parsePagesList(text string) []pageInfo {
	pages := make([]pageInfo, 0)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasSuffix(fields[0], ":") {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSuffix(fields[0], ":"))
		if err != nil {
			continue
		}
		page := pageInfo{id: id, url: fields[1]}
		for _, token := range fields[2:] {
			switch {
			case token == "[selected]":
				page.selected = true
			case strings.HasPrefix(token, "window="):
				page.windowID, _ = strconv.Atoi(strings.TrimPrefix(token, "window="))
			case strings.HasPrefix(token, "windowId="):
				page.windowID, _ = strconv.Atoi(strings.TrimPrefix(token, "windowId="))
			}
		}
		pages = append(pages, page)
	}
	return pages
}

func FormatPagesOutput(text string) string {
	pages := parsePagesList(text)
	if len(pages) == 0 {
		return "pages: 0 pages open"
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
	)
}

func CanClosePage(pageCount int) bool {
	return pageCount > 1
}

func FormatClosePageNoOpOutput() string {
	return joinBlocks(
		encode(map[string]any{"status": "cannot close the last open page (no-op)"}),
		renderHelp([]string{
			"Run `clawchrome-cli newpage <url>` to open another tab first",
			"Run `clawchrome-cli stop` to shut down the browser entirely",
		}),
	)
}

func FormatClosePageClosedOutput(pageID int) string {
	return encode(map[string]any{"status": "closed", "pageId": pageID})
}

func parseFillFormArgs(args []string) fillFormArgs {
	entries := make([]map[string]string, 0)
	for _, arg := range args {
		if arg == "--full" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return fillFormArgs{invalid: "Unexpected flag: " + arg}
		}
		if !strings.HasPrefix(arg, "@") {
			return fillFormArgs{invalid: "No valid field entries"}
		}
		parts := strings.SplitN(arg[1:], "=", 2)
		if len(parts) != 2 {
			return fillFormArgs{invalid: "Invalid fillform entry: " + arg}
		}
		if _, err := validateRefArg("fillform", "@"+parts[0]); err != nil {
			return fillFormArgs{invalid: "Invalid element ref: expected a snapshot ref such as @1"}
		}
		value := parts[1]
		if len(value) >= 2 {
			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) || (strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}
		}
		entries = append(entries, map[string]string{"uid": parts[0], "value": value})
	}
	return fillFormArgs{entries: entries}
}

func formatPageOutput(snap string, command string, url string, full bool) string {
	page := map[string]any{"refs": snapshot.CountRefs(snap)}
	if title := snapshot.ExtractTitle(snap); title != "" {
		page["title"] = title
	}
	if url != "" {
		page["url"] = url
	}

	tr := snapshot.TruncateSnapshot(snap, full, 16000)
	snapshotBlock := "snapshot:\n" + strings.TrimRight(tr.Text, "\n")
	if tr.Truncated {
		snapshotBlock += fmt.Sprintf("\n    ... (truncated, %d chars total)", tr.TotalLength)
	}

	sugs := suggestions.Get(suggestions.Context{Command: command, URL: url, Snapshot: snap})
	if tr.Truncated {
		suffix := command
		if url != "" {
			suffix += " " + url
		}
		sugs = append(sugs, fmt.Sprintf("Run `clawchrome-cli %s --full` to see complete snapshot", suffix))
	}

	return joinBlocks(
		encode(map[string]any{"page": page}),
		snapshotBlock,
		renderHelp(sugs),
	)
}

func JoinBlocks(blocks ...string) string {
	return joinBlocks(blocks...)
}

func RenderHelp(lines []string) string {
	return renderHelp(lines)
}
