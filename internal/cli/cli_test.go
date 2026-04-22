package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
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

	t.Run("screenshot path is optional and defaults to temp", func(t *testing.T) {
		help := getCommandHelp("screenshot")
		if !strings.Contains(help, "screenshot [path]") || !strings.Contains(help, "system temp directory") {
			t.Fatalf("expected screenshot help to describe optional temp path, got %q", help)
		}
	})

	t.Run("video start path is optional and defaults to temp", func(t *testing.T) {
		help := getCommandHelp("video")
		if !strings.Contains(help, "unique .mp4 file in the system temp directory") {
			t.Fatalf("expected video help to describe optional temp path, got %q", help)
		}
	})

	t.Run("start includes auth flags", func(t *testing.T) {
		help := getCommandHelp("start")
		if !strings.Contains(help, "--token <token>") || !strings.Contains(help, "--agent-name <name>") {
			t.Fatalf("expected start help to include auth flags, got %q", help)
		}
	})

	t.Run("status has help", func(t *testing.T) {
		help := getCommandHelp("status")
		if !strings.Contains(help, "status") || !strings.Contains(help, "auth status") {
			t.Fatalf("expected status help, got %q", help)
		}
	})

	t.Run("mouse help is grouped and has no full flag", func(t *testing.T) {
		help := getCommandHelp("mouse")
		if !strings.Contains(help, "mouse <action>") || !strings.Contains(help, "move <x> <y>") || !strings.Contains(help, "wheel <deltaX> <deltaY>") {
			t.Fatalf("expected mouse help to describe grouped actions, got %q", help)
		}
		if strings.Contains(help, "--full") {
			t.Fatalf("mouse help should not include --full")
		}
	})

	t.Run("form groups control actions and includes full", func(t *testing.T) {
		help := getCommandHelp("form")
		if !strings.Contains(help, "form <action>") || !strings.Contains(help, "clear @<uid>") || !strings.Contains(help, "check @<uid>") || !strings.Contains(help, "upload @<uid> <path>") {
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
	got := parsePagesList("## Pages\n1001: https://a.com/ window=501\n1002: https://b.com/ [selected] window=502")
	if len(got) != 2 {
		t.Fatalf("expected 2 pages, got %#v", got)
	}
	if got[0].id != 1001 || got[0].windowID != 501 || got[0].url != "https://a.com/" || got[0].selected {
		t.Fatalf("unexpected first page: %#v", got[0])
	}
	if got[1].id != 1002 || got[1].windowID != 502 || got[1].url != "https://b.com/" || !got[1].selected {
		t.Fatalf("unexpected second page: %#v", got[1])
	}
}

func TestParsePagesResultUsesStructuredPages(t *testing.T) {
	raw := mustJSON(t, map[string]any{
		"pages": []map[string]any{
			{"pageId": 1001, "url": "https://a.com/", "selected": false, "windowId": 501},
			{"pageId": 1002, "url": "https://b.com/", "selected": true, "windowId": 502},
		},
	})
	got := parsePagesResult(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 pages, got %#v", got)
	}
	if got[0].id != 1001 || got[0].windowID != 501 || got[0].url != "https://a.com/" || got[0].selected {
		t.Fatalf("unexpected first page: %#v", got[0])
	}
	if got[1].id != 1002 || got[1].windowID != 502 || got[1].url != "https://b.com/" || !got[1].selected {
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

func TestMainScreenshotResolvesOutputPath(t *testing.T) {
	t.Run("defaults to unique temp png path", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TMPDIR", tmpDir)

		var gotPath string
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "take_screenshot" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			gotPath, _ = args["filePath"].(string)
			if filepath.Dir(gotPath) != tmpDir {
				t.Fatalf("expected temp screenshot in %s, got %q", tmpDir, gotPath)
			}
			if filepath.Ext(gotPath) != ".png" {
				t.Fatalf("expected default .png screenshot path, got %q", gotPath)
			}
			if _, ok := args["format"]; ok {
				t.Fatalf("did not expect explicit format arg for default png: %#v", args)
			}
			return "", nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"screenshot"}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d; output:\n%s", exitCode, stdout.String())
		}
		if gotPath == "" || !strings.Contains(stdout.String(), gotPath) {
			t.Fatalf("expected output to include generated screenshot path %q, got %q", gotPath, stdout.String())
		}
		assertContainsAll(t, stdout.String(), []string{
			"screenshot:",
			"status: saved",
			"path: " + gotPath,
		})
	})

	t.Run("default path extension follows requested format", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TMPDIR", tmpDir)

		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "take_screenshot" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			gotPath, _ := args["filePath"].(string)
			if filepath.Dir(gotPath) != tmpDir || filepath.Ext(gotPath) != ".jpeg" {
				t.Fatalf("expected generated jpeg path in %s, got %q", tmpDir, gotPath)
			}
			if args["format"] != "jpeg" {
				t.Fatalf("expected jpeg format arg, got %#v", args)
			}
			return "", nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"screenshot", "--format", "jpeg"}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d; output:\n%s", exitCode, stdout.String())
		}
	})

	t.Run("explicit path is canonicalized", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "shot.png")
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "take_screenshot" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			if args["filePath"] != path {
				t.Fatalf("expected canonical screenshot path %q, got %#v", path, args["filePath"])
			}
			return "", nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"screenshot", path}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d; output:\n%s", exitCode, stdout.String())
		}
		if !strings.Contains(stdout.String(), path) {
			t.Fatalf("expected output to include screenshot path %q, got %q", path, stdout.String())
		}
		assertContainsAll(t, stdout.String(), []string{
			"screenshot:",
			"status: saved",
			"path: " + path,
		})
	})

	t.Run("missing parent directory is a validation error before backend", func(t *testing.T) {
		defer forbidSideEffects(t)()

		path := filepath.Join(t.TempDir(), "missing", "shot.png")
		var stdout bytes.Buffer
		exitCode := Main([]string{"screenshot", path}, &stdout, &bytes.Buffer{})
		if exitCode != 2 {
			t.Fatalf("expected validation exit code 2, got %d; output:\n%s", exitCode, stdout.String())
		}
		assertContainsAll(t, stdout.String(), []string{
			"Output directory does not exist",
			filepath.Dir(path),
			path,
			"usage: clawchrome-cli screenshot",
		})
	})

	t.Run("missing temp directory is a validation error before backend", func(t *testing.T) {
		defer forbidSideEffects(t)()

		tmpDir := filepath.Join(t.TempDir(), "missing")
		t.Setenv("TMPDIR", tmpDir)
		var stdout bytes.Buffer
		exitCode := Main([]string{"screenshot"}, &stdout, &bytes.Buffer{})
		if exitCode != 2 {
			t.Fatalf("expected validation exit code 2, got %d; output:\n%s", exitCode, stdout.String())
		}
		assertContainsAll(t, stdout.String(), []string{
			"Temporary output directory does not exist",
			tmpDir,
			"usage: clawchrome-cli screenshot",
		})
	})

	t.Run("directory path is a validation error before backend", func(t *testing.T) {
		defer forbidSideEffects(t)()

		path := t.TempDir()
		var stdout bytes.Buffer
		exitCode := Main([]string{"screenshot", path}, &stdout, &bytes.Buffer{})
		if exitCode != 2 {
			t.Fatalf("expected validation exit code 2, got %d; output:\n%s", exitCode, stdout.String())
		}
		assertContainsAll(t, stdout.String(), []string{
			"Output path is a directory",
			path,
			"usage: clawchrome-cli screenshot",
		})
	})

	t.Run("backend write failure includes output path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "shot.png")
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "take_screenshot" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			return "", client.WrapError("permission denied", client.ErrBrowser)
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"screenshot", path}, &stdout, &bytes.Buffer{})
		if exitCode != 1 {
			t.Fatalf("expected operation failure exit code 1, got %d; output:\n%s", exitCode, stdout.String())
		}
		assertContainsAll(t, stdout.String(), []string{
			"Failed to save screenshot",
			path,
			"permission denied",
		})
	})
}

func TestMainPagesOutputMatchesStructuredShape(t *testing.T) {
	restore := stubCallToolJSON(t, func(name string, args map[string]any) (json.RawMessage, error) {
		if name != "list_pages" {
			t.Fatalf("unexpected tool %q with args %#v", name, args)
		}
		return mustJSON(t, map[string]any{
			"pages": []map[string]any{
				{"pageId": 1001, "url": "https://a.com/", "selected": false, "windowId": 501},
				{"pageId": 1002, "url": "https://b.com/", "selected": true, "windowId": 502},
			},
		}), nil
	})
	defer restore()

	var stdout bytes.Buffer
	exitCode := Main([]string{"pages"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	output := stdout.String()
	if !strings.Contains(output, "pages[2]{id,window,url,selected}:") {
		t.Fatalf("expected structured pages header, got %q", output)
	}
	if !strings.Contains(output, "1001,501,https://a.com/,false") || !strings.Contains(output, "1002,502,https://b.com/,true") {
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
	restore := stubCallToolJSON(t, func(name string, args map[string]any) (json.RawMessage, error) {
		calls = append(calls, name)
		if name == "list_pages" {
			return mustJSON(t, map[string]any{
				"pages": []map[string]any{
					{"pageId": 1001, "url": "https://example.com/", "selected": true, "windowId": 501},
				},
			}), nil
		}
		t.Fatalf("unexpected tool %q with args %#v", name, args)
		return nil, nil
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
	restore := stubCallToolJSON(t, func(name string, args map[string]any) (json.RawMessage, error) {
		calls = append(calls, name)
		switch name {
		case "list_pages":
			return mustJSON(t, map[string]any{
				"pages": []map[string]any{
					{"pageId": 1001, "url": "https://a.com/", "selected": false, "windowId": 501},
					{"pageId": 1002, "url": "https://b.com/", "selected": true, "windowId": 502},
				},
			}), nil
		case "close_page":
			if args["pageId"] != 1002 {
				t.Fatalf("unexpected close_page args: %#v", args)
			}
			return mustJSON(t, map[string]any{"ok": true}), nil
		default:
			t.Fatalf("unexpected tool %q with args %#v", name, args)
			return nil, nil
		}
	})
	defer restore()

	var stdout bytes.Buffer
	exitCode := Main([]string{"closepage", "1002"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if strings.Join(calls, ",") != "list_pages,close_page" {
		t.Fatalf("unexpected call sequence: %#v", calls)
	}
	if !strings.Contains(stdout.String(), "status: closed") || !strings.Contains(stdout.String(), "pageId: 1002") {
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

func TestMainMouseUsesRuntimeHTTPTools(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantTool   string
		wantArgs   map[string]any
		wantStatus string
	}{
		{
			name:       "move",
			args:       []string{"mouse", "move", "10", "20"},
			wantTool:   "browser_mouse_move_xy",
			wantArgs:   map[string]any{"x": 10.0, "y": 20.0},
			wantStatus: "moved",
		},
		{
			name:       "click",
			args:       []string{"mouse", "click", "10.5", "20.25"},
			wantTool:   "browser_mouse_click_xy",
			wantArgs:   map[string]any{"x": 10.5, "y": 20.25},
			wantStatus: "clicked",
		},
		{
			name:       "drag",
			args:       []string{"mouse", "drag", "1", "2", "3", "4"},
			wantTool:   "browser_mouse_drag_xy",
			wantArgs:   map[string]any{"startX": 1.0, "startY": 2.0, "endX": 3.0, "endY": 4.0},
			wantStatus: "dragged",
		},
		{
			name:       "down default",
			args:       []string{"mouse", "down"},
			wantTool:   "browser_mouse_down",
			wantArgs:   map[string]any{"button": "left"},
			wantStatus: "pressed",
		},
		{
			name:       "up right",
			args:       []string{"mouse", "up", "right"},
			wantTool:   "browser_mouse_up",
			wantArgs:   map[string]any{"button": "right"},
			wantStatus: "released",
		},
		{
			name:       "wheel",
			args:       []string{"mouse", "wheel", "0", "-500"},
			wantTool:   "browser_mouse_wheel",
			wantArgs:   map[string]any{"deltaX": 0.0, "deltaY": -500.0},
			wantStatus: "scrolled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := stubCallRuntimeHTTPTool(t, func(name string, args map[string]any) (client.RuntimeHTTPToolResponse, error) {
				if name != tc.wantTool {
					t.Fatalf("unexpected tool %q, want %q", name, tc.wantTool)
				}
				for key, want := range tc.wantArgs {
					if args[key] != want {
						t.Fatalf("unexpected arg %s=%#v, want %#v; args=%#v", key, args[key], want, args)
					}
				}
				return client.RuntimeHTTPToolResponse{
					OK:      true,
					Action:  strings.TrimPrefix(tc.wantTool, "browser_"),
					Backend: "runtime-core",
					Message: "ok",
					Data:    args,
				}, nil
			})
			defer restore()

			var stdout bytes.Buffer
			exitCode := Main(tc.args, &stdout, &bytes.Buffer{})
			if exitCode != 0 {
				t.Fatalf("expected exit code 0, got %d; output:\n%s", exitCode, stdout.String())
			}
			assertContainsAll(t, stdout.String(), []string{
				"mouse:",
				"status: " + tc.wantStatus,
				"tool: " + tc.wantTool,
				"message: ok",
				"backend: runtime-core",
			})
		})
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

	t.Run("clear uses fill empty value", func(t *testing.T) {
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "fill" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			if args["uid"] != "4" || args["value"] != "" || args["includeSnapshot"] != true {
				t.Fatalf("unexpected fill args: %#v", args)
			}
			return snapshotText, nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"form", "clear", "@4"}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
		if !strings.Contains(stdout.String(), "snapshot:") {
			t.Fatalf("expected snapshot output, got %q", stdout.String())
		}
	})

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
		path := filepath.Join(t.TempDir(), "capture.mp4")
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "screencast_start" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			if args["path"] != path {
				t.Fatalf("unexpected screencast_start args: %#v", args)
			}
			return "recording started", nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"video", "start", path}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
		if !strings.Contains(stdout.String(), "status: started") || !strings.Contains(stdout.String(), "path: "+path) {
			t.Fatalf("unexpected video start output: %q", stdout.String())
		}
	})

	t.Run("start without path defaults to temp mp4", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TMPDIR", tmpDir)

		var gotPath string
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "screencast_start" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			gotPath, _ = args["path"].(string)
			if filepath.Dir(gotPath) != tmpDir || filepath.Ext(gotPath) != ".mp4" {
				t.Fatalf("expected generated mp4 path in %s, got %q", tmpDir, gotPath)
			}
			return "recording started", nil
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"video", "start"}, &stdout, &bytes.Buffer{})
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d; output:\n%s", exitCode, stdout.String())
		}
		if gotPath == "" || !strings.Contains(stdout.String(), gotPath) {
			t.Fatalf("expected output to include generated video path %q, got %q", gotPath, stdout.String())
		}
	})

	t.Run("start missing parent directory is a validation error before backend", func(t *testing.T) {
		defer forbidSideEffects(t)()

		path := filepath.Join(t.TempDir(), "missing", "capture.mp4")
		var stdout bytes.Buffer
		exitCode := Main([]string{"video", "start", path}, &stdout, &bytes.Buffer{})
		if exitCode != 2 {
			t.Fatalf("expected validation exit code 2, got %d; output:\n%s", exitCode, stdout.String())
		}
		assertContainsAll(t, stdout.String(), []string{
			"Output directory does not exist",
			filepath.Dir(path),
			path,
			"usage: clawchrome-cli video",
		})
	})

	t.Run("start backend failure includes output path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "capture.mp4")
		restore := stubCallTool(t, func(name string, args map[string]any) (string, error) {
			if name != "screencast_start" {
				t.Fatalf("unexpected tool %q with args %#v", name, args)
			}
			return "", client.WrapError("permission denied", client.ErrBrowser)
		})
		defer restore()

		var stdout bytes.Buffer
		exitCode := Main([]string{"video", "start", path}, &stdout, &bytes.Buffer{})
		if exitCode != 1 {
			t.Fatalf("expected operation failure exit code 1, got %d; output:\n%s", exitCode, stdout.String())
		}
		assertContainsAll(t, stdout.String(), []string{
			"Failed to start video recording",
			path,
			"permission denied",
		})
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

func TestStartSavesAuthFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("CLAWCHROME_CLI_TRANSPORT", "stdio")
	t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "")

	restoreEnsureBridge := stubEnsureBridge(t, func() (int, error) {
		return 9224, nil
	})
	defer restoreEnsureBridge()
	restoreUsesHTTPTransport := stubUsesHTTPTransport(t, func() bool {
		return false
	})
	defer restoreUsesHTTPTransport()

	var stdout bytes.Buffer
	exitCode := Main([]string{"start", "--token", "saved-token", "--agent-name", "codex-worker"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; output:\n%s", exitCode, stdout.String())
	}
	assertContainsAll(t, stdout.String(), []string{
		"status: ready",
		"port: 9224",
		"auth:",
		"token: configured",
		"source: config",
		"agentName: codex-worker",
	})
	assertNotContainsAny(t, stdout.String(), []string{"saved-token"})

	auth, err := client.GetAuthStatus()
	if err != nil {
		t.Fatalf("GetAuthStatus failed: %v", err)
	}
	if auth.Token != "configured" || auth.Source != "config" || auth.AgentName != "codex-worker" {
		t.Fatalf("unexpected auth status: %#v", auth)
	}
}

func TestStatusShowsAuthWithoutToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
	t.Setenv("CLAWCHROME_CLI_HTTP_URL", "")
	t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "")

	if _, err := client.SaveAuthConfig("saved-token", "codex-worker"); err != nil {
		t.Fatalf("SaveAuthConfig failed: %v", err)
	}

	var stdout bytes.Buffer
	exitCode := Main([]string{"status"}, &stdout, &bytes.Buffer{})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; output:\n%s", exitCode, stdout.String())
	}
	assertContainsAll(t, stdout.String(), []string{
		"status:",
		"transport: http",
		`target: "https://www.clawchrome.com"`,
		"auth:",
		"token: configured",
		"source: config",
		"agentName: codex-worker",
	})
	assertNotContainsAny(t, stdout.String(), []string{"saved-token"})
}

func stubCallTool(t *testing.T, fn func(name string, args map[string]any) (string, error)) func() {
	t.Helper()
	prev := callTool
	prevJSON := callToolJSON
	callTool = fn
	callToolJSON = func(name string, args map[string]any) (json.RawMessage, error) {
		text, err := fn(name, args)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(text)
		if err != nil {
			return nil, err
		}
		return raw, nil
	}
	return func() {
		callTool = prev
		callToolJSON = prevJSON
	}
}

func stubCallToolJSON(t *testing.T, fn func(name string, args map[string]any) (json.RawMessage, error)) func() {
	t.Helper()
	prev := callTool
	prevJSON := callToolJSON
	callToolJSON = fn
	callTool = func(name string, args map[string]any) (string, error) {
		raw, err := fn(name, args)
		if err != nil {
			return "", err
		}
		return client.CallToolResultText(raw), nil
	}
	return func() {
		callTool = prev
		callToolJSON = prevJSON
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json fixture: %v", err)
	}
	return raw
}

func stubCallRuntimeHTTPTool(t *testing.T, fn func(name string, args map[string]any) (client.RuntimeHTTPToolResponse, error)) func() {
	t.Helper()
	prev := callRuntimeHTTPTool
	callRuntimeHTTPTool = fn
	return func() {
		callRuntimeHTTPTool = prev
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
