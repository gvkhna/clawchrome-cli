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

	t.Run("numeric target uses evaluate_script", func(t *testing.T) {
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

func stubCallTool(t *testing.T, fn func(name string, args map[string]any) (string, error)) func() {
	t.Helper()
	prev := callTool
	callTool = fn
	return func() {
		callTool = prev
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
