package cli

import (
	"fmt"
	"strings"

	"github.com/gvkhna/clawchrome-cli/internal/snapshot"
	"github.com/gvkhna/clawchrome-cli/internal/suggestions"
)

const topHelp = `usage: clawchrome-cli [command] [args] [flags]

commands:
  open <url>, snapshot, screenshot <path>, click @<uid>, fill @<uid> <text>,
  type <text>, press <key>, scroll <dir>, back, wait <ms|text>,
  hover @<uid>, drag @<from> @<to>, fillform @<uid>=<val>..., dialog <action>,
  upload @<uid> <path>, pages, newpage <url>, selectpage <id>, closepage <id>,
  resize <w> <h>, start, stop

flags:
  --help, -v, -V, --version, --full

environment:
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
	"screenshot": `usage: clawchrome-cli screenshot <path> [--uid @<uid>] [--full-page] [--format png|jpeg|webp]
Save a screenshot to a file.

args:
  <path>  File path to save the screenshot (required)

flags:
  --uid @<uid>    Capture a specific element instead of the full viewport
  --full-page     Capture the entire scrollable page
  --format <fmt>  Image format: png (default), jpeg, or webp

examples:
  clawchrome-cli screenshot ./page.png
  clawchrome-cli screenshot ./element.png --uid @3
  clawchrome-cli screenshot ./full.png --full-page --format jpeg`,
	"snapshot": `usage: clawchrome-cli snapshot [--full]
Capture the current page accessibility snapshot.

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli snapshot
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
Scroll the page in a direction.

args:
  <direction>  up, down, top, or bottom (default: down)

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli scroll down
  clawchrome-cli scroll top --full`,
	"back": `usage: clawchrome-cli back [--full]
Navigate back in browser history.

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli back
  clawchrome-cli back --full`,
	"wait": `usage: clawchrome-cli wait <ms|text>
Wait for a duration or for text to appear on the page.

args:
  <ms>    Milliseconds to wait (numeric)
  <text>  Text to wait for (string)

examples:
  clawchrome-cli wait 2000
  clawchrome-cli wait "Submit"`,
	"start": `usage: clawchrome-cli start
Start the bridge server (launches headless Chrome).

examples:
  clawchrome-cli start`,
	"stop": `usage: clawchrome-cli stop
Stop the bridge server and close the browser.

examples:
  clawchrome-cli stop`,
	"pages": `usage: clawchrome-cli pages
List all open pages/tabs in the browser.

examples:
  clawchrome-cli pages`,
	"newpage": `usage: clawchrome-cli newpage <url> [--background] [--full]
Open a new tab and navigate to a URL.

args:
  <url>  URL to open (required)

flags:
  --background  Open in background without bringing to front
  --full        Show complete snapshot without truncation

examples:
  clawchrome-cli newpage https://example.com
  clawchrome-cli newpage https://example.com --background`,
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
	"upload": `usage: clawchrome-cli upload @<uid> <path> [--full]
Upload a file through a file input element.

args:
  @<uid>  File input element ref from snapshot (required)
  <path>  Local file path to upload (required)

flags:
  --full  Show complete snapshot without truncation

examples:
  clawchrome-cli upload @5 ./photo.jpg`,
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
	"hover":      true,
	"drag":       true,
	"fillform":   true,
	"upload":     true,
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
	"back",
	"wait",
	"hover",
	"drag",
	"fillform",
	"dialog",
	"upload",
	"pages",
	"newpage",
	"selectpage",
	"closepage",
	"resize",
	"start",
	"stop",
}

type screenshotArgs struct {
	filePath string
	uid      string
	fullPage bool
	format   string
}

type ScreenshotArgs struct {
	FilePath string
	UID      string
	FullPage bool
	Format   string
}

type PageInfo struct {
	ID       int
	URL      string
	Selected bool
}

type pageInfo struct {
	id       int
	url      string
	selected bool
}

type fillFormArgs struct {
	entries []map[string]string
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
	parsed := ParseScreenshotArgs(args)
	return screenshotArgs{
		filePath: parsed.FilePath,
		uid:      parsed.UID,
		fullPage: parsed.FullPage,
		format:   parsed.Format,
	}
}

func ParsePagesList(text string) []PageInfo {
	internal := parsePagesList(text)
	pages := make([]PageInfo, 0, len(internal))
	for _, page := range internal {
		pages = append(pages, PageInfo{ID: page.id, URL: page.url, Selected: page.selected})
	}
	return pages
}

func parsePagesList(text string) []pageInfo {
	pages := make([]pageInfo, 0)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		var (
			id       int
			url      string
			selected string
		)
		if _, err := fmt.Sscanf(line, "%d: %s %s", &id, &url, &selected); err == nil {
			pages = append(pages, pageInfo{id: id, url: url, selected: selected == "[selected]"})
			continue
		}
		if _, err := fmt.Sscanf(line, "%d: %s", &id, &url); err == nil {
			pages = append(pages, pageInfo{id: id, url: url})
		}
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
		rows = append(rows, fmt.Sprintf("  %d,%s,%t", page.id, page.url, page.selected))
	}

	return joinBlocks(
		fmt.Sprintf("pages[%d]{id,url,selected}:\n%s", len(pages), strings.Join(rows, "\n")),
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
		if !strings.HasPrefix(arg, "@") {
			continue
		}
		parts := strings.SplitN(arg[1:], "=", 2)
		if len(parts) != 2 {
			continue
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
