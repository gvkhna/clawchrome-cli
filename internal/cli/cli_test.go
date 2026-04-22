package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gvkhna/clawchrome-cli/internal/client"
)

func TestGetCommandHelp(t *testing.T) {
	t.Run("pages has help and no full flag", func(t *testing.T) {
		help := getCommandHelp("pages")
		if help == "" || !strings.Contains(help, "pages") {
			t.Fatalf("expected pages help, got %q", help)
		}
		if strings.Contains(help, "--full") {
			t.Fatalf("pages help should not include --full")
		}
	})

	t.Run("newpage includes background and full", func(t *testing.T) {
		help := getCommandHelp("newpage")
		if !strings.Contains(help, "--background") || !strings.Contains(help, "--full") {
			t.Fatalf("unexpected newpage help: %q", help)
		}
	})

	t.Run("selectpage includes full", func(t *testing.T) {
		help := getCommandHelp("selectpage")
		if !strings.Contains(help, "--full") {
			t.Fatalf("expected selectpage help to include --full")
		}
	})

	t.Run("closepage has no full flag", func(t *testing.T) {
		help := getCommandHelp("closepage")
		if strings.Contains(help, "--full") {
			t.Fatalf("closepage help should not include --full")
		}
	})

	t.Run("resize has no full flag", func(t *testing.T) {
		help := getCommandHelp("resize")
		if strings.Contains(help, "--full") {
			t.Fatalf("resize help should not include --full")
		}
	})

	t.Run("forward and reload include full", func(t *testing.T) {
		for _, command := range []string{"forward", "reload"} {
			help := getCommandHelp(command)
			if !strings.Contains(help, "--full") {
				t.Fatalf("expected %s help to include --full: %q", command, help)
			}
		}
	})

	t.Run("video has no full flag", func(t *testing.T) {
		help := getCommandHelp("video")
		if !strings.Contains(help, "video <start|stop>") {
			t.Fatalf("expected video help, got %q", help)
		}
		if strings.Contains(help, "--full") {
			t.Fatalf("video help should not include --full")
		}
	})

	t.Run("form groups control actions and includes full", func(t *testing.T) {
		help := getCommandHelp("form")
		if !strings.Contains(help, "form <action>") || !strings.Contains(help, "check @<uid>") || !strings.Contains(help, "upload @<uid> <path>") {
			t.Fatalf("expected form help to describe grouped actions, got %q", help)
		}
		if !strings.Contains(help, "--full") {
			t.Fatalf("expected form help to include --full")
		}
	})

	t.Run("snapshot includes form and text filters", func(t *testing.T) {
		help := getCommandHelp("snapshot")
		if !strings.Contains(help, "--form") || !strings.Contains(help, "--text") {
			t.Fatalf("snapshot help should include form and text filters, got %q", help)
		}
	})
}

func TestParseScreenshotArgs(t *testing.T) {
	got := parseScreenshotArgs([]string{"./shot.jpg", "--uid", "@3", "--full-page", "--format", "jpeg"})
	if got.filePath != "./shot.jpg" || got.uid != "3" || !got.fullPage || got.format != "jpeg" {
		t.Fatalf("unexpected parse result: %#v", got)
	}

	got = parseScreenshotArgs([]string{"--full-page", "./shot.png"})
	if got.filePath != "./shot.png" {
		t.Fatalf("expected filePath to be discovered from positional arg, got %#v", got)
	}
}

func TestParseSnapshotArgs(t *testing.T) {
	got := parseSnapshotArgs([]string{"--form", "--text"})
	if !got.form || !got.text || got.invalid != "" {
		t.Fatalf("unexpected parse result: %#v", got)
	}

	got = parseSnapshotArgs([]string{"--unknown"})
	if got.invalid != "--unknown" {
		t.Fatalf("expected invalid flag, got %#v", got)
	}
}

func TestParsePagesList(t *testing.T) {
	got := parsePagesList("## Pages\n0: https://a.com/\n1: https://b.com/ [selected]")
	if len(got) != 2 {
		t.Fatalf("expected 2 pages, got %#v", got)
	}
	if got[0].id != 0 || got[0].url != "https://a.com/" || got[0].selected {
		t.Fatalf("unexpected first page: %#v", got[0])
	}
	if got[1].id != 1 || got[1].url != "https://b.com/" || !got[1].selected {
		t.Fatalf("unexpected second page: %#v", got[1])
	}
}

func TestParseFillFormArgs(t *testing.T) {
	got := parseFillFormArgs([]string{`@1="hello"`, "@2=world", "--full"})
	if len(got.entries) != 2 {
		t.Fatalf("expected 2 entries, got %#v", got.entries)
	}
	if got.entries[0]["uid"] != "1" || got.entries[0]["value"] != "hello" {
		t.Fatalf("unexpected first entry: %#v", got.entries[0])
	}
	if got.entries[1]["uid"] != "2" || got.entries[1]["value"] != "world" {
		t.Fatalf("unexpected second entry: %#v", got.entries[1])
	}
}

func TestParseSnapshotFromResponse(t *testing.T) {
	input := "ok\n## Latest page snapshot\nRootWebArea \"Example\"\n  uid=1 link \"Docs\"\n"
	got := parseSnapshotFromResponse(input)
	if got == "" {
		t.Fatalf("expected parsed snapshot")
	}
}

func TestStopText(t *testing.T) {
	if stopText(true) != "stopped" {
		t.Fatalf("unexpected stopped text")
	}
	if stopText(false) != "stopped (no-op)" {
		t.Fatalf("unexpected no-op text")
	}
}

func TestMainPagesOutputMatchesStructuredShape(t *testing.T) {
	restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
		if name != "list_pages" {
			t.Fatalf("unexpected tool %q with args %#v", name, args)
		}
		return "## Pages\n0: https://a.com/\n1: https://b.com/ [selected]", nil
	})
	defer restore()

	var stdout bytes.Buffer
	exitCode := Main([]string{"pages"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	output := stdout.String()
	if !strings.Contains(output, "pages[2]{id,url,selected}:") {
		t.Fatalf("expected structured pages header, got %q", output)
	}
	if !strings.Contains(output, "0,https://a.com/,false") || !strings.Contains(output, "1,https://b.com/,true") {
		t.Fatalf("expected structured page rows, got %q", output)
	}
}

func TestMainSnapshotPassesFilterArgs(t *testing.T) {
	restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
		if name != "take_snapshot" {
			t.Fatalf("unexpected tool %q with args %#v", name, args)
		}
		if args["form"] != true || args["text"] != true || args["verbose"] != true {
			t.Fatalf("expected form, text, and verbose args, got %#v", args)
		}
		return "## Latest page snapshot\nRootWebArea \"Example\"\n  uid=1 textbox \"Search\"\n", nil
	})
	defer restore()

	var stdout bytes.Buffer
	exitCode := Main([]string{"snapshot", "--form", "--text", "--full"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "refs: 1") {
		t.Fatalf("expected formatted snapshot output, got %q", stdout.String())
	}
}

func TestMainSnapshotRejectsUnexpectedArgs(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := Main([]string{"snapshot", "extra"}, &stdout, &bytes.Buffer{})
	if exitCode != 2 {
		t.Fatalf("expected validation exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "Unexpected snapshot argument: extra") {
		t.Fatalf("expected validation message, got %q", stdout.String())
	}
}

func TestMainClosePageNoOpWhenLastPageOpen(t *testing.T) {
	var calls []string
	restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
		calls = append(calls, name)
		if name == "list_pages" {
			return "## Pages\n1: https://example.com/ [selected]", nil
		}
		t.Fatalf("unexpected tool %q with args %#v", name, args)
		return "", nil
	})
	defer restore()

	var stdout bytes.Buffer
	exitCode := Main([]string{"closepage", "1"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if len(calls) != 1 || calls[0] != "list_pages" {
		t.Fatalf("unexpected call sequence: %#v", calls)
	}

	output := stdout.String()
	if !strings.Contains(output, "cannot close the last open page (no-op)") {
		t.Fatalf("expected no-op status, got %q", output)
	}
	if !strings.Contains(output, "newpage <url>") || !strings.Contains(output, "stop") {
		t.Fatalf("expected closepage help suggestions, got %q", output)
	}
}

func TestMainClosePageClosesWhenMultiplePagesOpen(t *testing.T) {
	var calls []string
	restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
		calls = append(calls, name)
		switch name {
		case "list_pages":
			return "## Pages\n0: https://a.com/\n1: https://b.com/ [selected]", nil
		case "close_page":
			if args["pageId"] != 1 {
				t.Fatalf("unexpected close_page args: %#v", args)
			}
			return "", nil
		default:
			t.Fatalf("unexpected tool %q with args %#v", name, args)
			return "", nil
		}
	})
	defer restore()

	var stdout bytes.Buffer
	exitCode := Main([]string{"closepage", "1"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if strings.Join(calls, ",") != "list_pages,close_page" {
		t.Fatalf("unexpected call sequence: %#v", calls)
	}
	if !strings.Contains(stdout.String(), "status: closed") || !strings.Contains(stdout.String(), "pageId: 1") {
		t.Fatalf("expected closed output, got %q", stdout.String())
	}
}

func TestMainWaitValidationAndDispatch(t *testing.T) {
	t.Run("missing target shows both examples", func(t *testing.T) {
		var stdout bytes.Buffer
		exitCode := Main([]string{"wait"}, &stdout, &bytes.Buffer{})
		if exitCode != 2 {
			t.Fatalf("expected validation exit code 2, got %d", exitCode)
		}
		output := stdout.String()
		if !strings.Contains(output, "Missing wait target") {
			t.Fatalf("expected wait validation error, got %q", output)
		}
		if !strings.Contains(output, "wait 2000") || !strings.Contains(output, "wait \"Submit\"") {
			t.Fatalf("expected both wait examples, got %q", output)
		}
	})

	t.Run("numeric target uses native wait_duration", func(t *testing.T) {
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "wait_duration" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			if args["milliseconds"] != 2000 {
				t.Fatalf("unexpected wait_duration args: %#v", args)
			}
			return "", nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"wait", "2000"}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
		if !strings.Contains(stdout.String(), "waited: \"2000\"") {
			t.Fatalf("expected waited output, got %q", stdout.String())
		}
	})

	t.Run("text target uses wait_for", func(t *testing.T) {
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "wait_for" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			want := []string{"Submit"}
			got, ok := args["text"].([]string)
			if !ok || len(got) != len(want) || got[0] != want[0] {
				t.Fatalf("unexpected wait_for args: %#v", args)
			}
			return "", nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"wait", "Submit"}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
		if !strings.Contains(stdout.String(), "Run `clawchrome-cli snapshot` to see current page state") {
			t.Fatalf("expected wait suggestion, got %q", stdout.String())
		}
	})
}

func TestMainScrollUsesNativeTool(t *testing.T) {
	restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
		switch name {
		case "scroll_page":
			if args["direction"] != "down" {
				t.Fatalf("unexpected scroll_page args: %#v", args)
			}
			return "", nil
		case "take_snapshot":
			return "## Latest page snapshot\nRootWebArea \"Example\"\n", nil
		default:
			t.Fatalf("unexpected tool %q with args %#v", name, args)
			return "", nil
		}
	})
	defer restore()

	var stdout bytes.Buffer
	exitCode := Main([]string{"scroll", "down"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "snapshot:") {
		t.Fatalf("expected snapshot output, got %q", stdout.String())
	}
}

func TestMainForwardAndReloadUseNavigatePage(t *testing.T) {
	for _, tc := range []struct {
		command string
		navType string
	}{
		{command: "forward", navType: "forward"},
		{command: "reload", navType: "reload"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			var calls []string
			restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
				calls = append(calls, name)
				switch name {
				case "navigate_page":
					if args["type"] != tc.navType {
						t.Fatalf("unexpected navigate_page args: %#v", args)
					}
					return "", nil
				case "take_snapshot":
					return "## Latest page snapshot\nRootWebArea \"Example\"\n", nil
				default:
					t.Fatalf("unexpected tool %q with args %#v", name, args)
					return "", nil
				}
			})
			defer restore()

			var stdout bytes.Buffer
			exitCode := Main([]string{tc.command}, &stdout, &bytes.Buffer{})
			if exitCode != 0 {
				t.Fatalf("expected exit code 0, got %d", exitCode)
			}
			if strings.Join(calls, ",") != "navigate_page,take_snapshot" {
				t.Fatalf("unexpected call sequence: %#v", calls)
			}
			if !strings.Contains(stdout.String(), "snapshot:") {
				t.Fatalf("expected snapshot output, got %q", stdout.String())
			}
		})
	}
}

func TestMainFormActionsUseCompatibilityTools(t *testing.T) {
	snapshotText := "ok\n## Latest page snapshot\nRootWebArea \"Example\"\n  uid=1 checkbox \"Terms\"\n"

	t.Run("check uses fill_form checkbox true", func(t *testing.T) {
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "fill_form" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			if args["includeSnapshot"] != true {
				t.Fatalf("expected includeSnapshot=true, got %#v", args)
			}
			elements, ok := args["elements"].([]map[string]any)
			if !ok || len(elements) != 1 {
				t.Fatalf("unexpected fill_form elements: %#v", args["elements"])
			}
			if elements[0]["uid"] != "1" || elements[0]["type"] != "checkbox" || elements[0]["value"] != true {
				t.Fatalf("unexpected checkbox element: %#v", elements[0])
			}
			return snapshotText, nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"form", "check", "@1"}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
		if !strings.Contains(stdout.String(), "snapshot:") {
			t.Fatalf("expected snapshot output, got %q", stdout.String())
		}
	})

	t.Run("uncheck uses fill_form checkbox false", func(t *testing.T) {
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "fill_form" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			elements, ok := args["elements"].([]map[string]any)
			if !ok || len(elements) != 1 {
				t.Fatalf("unexpected fill_form elements: %#v", args["elements"])
			}
			if elements[0]["uid"] != "1" || elements[0]["type"] != "checkbox" || elements[0]["value"] != false {
				t.Fatalf("unexpected checkbox element: %#v", elements[0])
			}
			return snapshotText, nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"form", "uncheck", "@1"}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
	})

	t.Run("select uses fill_form select value", func(t *testing.T) {
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "fill_form" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			elements, ok := args["elements"].([]map[string]any)
			if !ok || len(elements) != 1 {
				t.Fatalf("unexpected fill_form elements: %#v", args["elements"])
			}
			if elements[0]["uid"] != "2" || elements[0]["type"] != "select" || elements[0]["value"] != "United States" {
				t.Fatalf("unexpected select element: %#v", elements[0])
			}
			return snapshotText, nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"form", "select", "@2", "United", "States"}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
	})

	t.Run("upload uses upload_file", func(t *testing.T) {
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "upload_file" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			if args["uid"] != "5" || args["filePath"] != "./photo.jpg" || args["includeSnapshot"] != true {
				t.Fatalf("unexpected upload_file args: %#v", args)
			}
			return snapshotText, nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"form", "upload", "@5", "./photo.jpg"}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
	})
}

func TestMainVideoUsesScreencastTools(t *testing.T) {
	t.Run("start with path", func(t *testing.T) {
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "screencast_start" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			if args["path"] != "./capture.mp4" {
				t.Fatalf("unexpected screencast_start args: %#v", args)
			}
			return "recording started", nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"video", "start", "./capture.mp4"}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
		if !strings.Contains(stdout.String(), "status: started") || !strings.Contains(stdout.String(), "path: ./capture.mp4") {
			t.Fatalf("unexpected video start output: %q", stdout.String())
		}
	})

	t.Run("stop", func(t *testing.T) {
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "screencast_stop" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			if len(args) != 0 {
				t.Fatalf("unexpected screencast_stop args: %#v", args)
			}
			return "recording stopped", nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"video", "stop"}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
		if !strings.Contains(stdout.String(), "status: stopped") || !strings.Contains(stdout.String(), "recording stopped") {
			t.Fatalf("unexpected video stop output: %q", stdout.String())
		}
	})
}

func TestMainCommandHelp(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := Main([]string{"newpage", "--help"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	output := stdout.String()
	if !strings.Contains(output, "usage: clawchrome-cli newpage <url> [--background] [--full]") {
		t.Fatalf("unexpected help output: %q", output)
	}
}

func TestMainVersion(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := Main([]string{"version"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if strings.TrimSpace(stdout.String()) != Version {
		t.Fatalf("unexpected version output: %q", stdout.String())
	}
}

func TestMainSelfUpdate(t *testing.T) {
	restoreSelfUpdate := stubSelfUpdate(t, func(_ string, target string) (string, error) {
		if target != "v1.2.3" {
			t.Fatalf("unexpected target version: %q", target)
		}
		return "v1.2.3", nil
	})
	defer restoreSelfUpdate()

	var stdout bytes.Buffer
	exitCode := Main([]string{"self-update", "v1.2.3"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "status: updated") || !strings.Contains(stdout.String(), "version: v1.2.3") {
		t.Fatalf("unexpected self-update output: %q", stdout.String())
	}
}

func TestMainWritesUpdateNoticeToStderr(t *testing.T) {
	restoreUpdateNotice := stubUpdateNotice(t, func(_ string) (string, error) {
		return "update available: v9.9.9 -> run `clawchrome-cli self-update`", nil
	})
	defer restoreUpdateNotice()

	restoreCallTool := stubCallTool(t, func(name string, args map[string]any) (string, error) {
		if name != "list_pages" {
			t.Fatalf("unexpected tool %q with args %#v", name, args)
		}
		return "", nil
	})
	defer restoreCallTool()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Main([]string{"pages"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "update available: v9.9.9") {
		t.Fatalf("expected update notice on stderr, got %q", stderr.String())
	}
}

func TestStartUsesRuntimeHTTPStartBrowserTool(t *testing.T) {
	restoreEnsureBridge := stubEnsureBridge(t, func() (int, error) {
		return 8091, nil
	})
	defer restoreEnsureBridge()
	restoreUsesHTTPTransport := stubUsesHTTPTransport(t, func() bool {
		return true
	})
	defer restoreUsesHTTPTransport()
	restoreCallTool := stubCallTool(t, func(name string, args map[string]any) (string, error) {
		if name != "start_browser" {
			t.Fatalf("unexpected tool %q", name)
		}
		if got, _ := args["url"].(string); got != "https://example.com" {
			t.Fatalf("expected start_browser url=https://example.com, got %#v", args["url"])
		}
		return "", nil
	})
	defer restoreCallTool()

	var stdout bytes.Buffer
	exitCode := Main([]string{"start", "https://example.com"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "status: ready") || !strings.Contains(stdout.String(), "port: 8091") {
		t.Fatalf("unexpected start output: %q", stdout.String())
	}
}

func stubCallTool(t *testing.T, fn func(name string, args map[string]any) (string, error)) func() {
	t.Helper()
	prev := callTool
	callTool = fn
	return func() {
		callTool = prev
	}
}

func stubEnsureBridge(t *testing.T, fn func() (int, error)) func() {
	t.Helper()
	prev := ensureBridge
	ensureBridge = fn
	return func() {
		ensureBridge = prev
	}
}

func stubUsesHTTPTransport(t *testing.T, fn func() bool) func() {
	t.Helper()
	prev := usesHTTPTransport
	usesHTTPTransport = fn
	return func() {
		usesHTTPTransport = prev
	}
}

func stubSelfUpdate(t *testing.T, fn func(currentVersion string, targetVersion string) (string, error)) func() {
	t.Helper()
	prev := selfUpdate
	selfUpdate = func(currentVersion string, targetVersion string) (string, error) {
		return fn(currentVersion, targetVersion)
	}
	return func() {
		selfUpdate = prev
	}
}

func stubUpdateNotice(t *testing.T, fn func(currentVersion string) (string, error)) func() {
	t.Helper()
	prev := updateNotice
	updateNotice = func(currentVersion string) (string, error) {
		return fn(currentVersion)
	}
	return func() {
		updateNotice = prev
	}
}

func TestRecoverableOpenError(t *testing.T) {
	if !isRecoverableOpenError(&client.CdpError{Message: "session not found", Code: client.ErrBrowser}) {
		t.Fatalf("expected session not found to be recoverable")
	}
	if isRecoverableOpenError(&client.CdpError{Message: "boom", Code: client.ErrUnknown}) {
		t.Fatalf("expected non-browser errors to be non-recoverable")
	}
}
