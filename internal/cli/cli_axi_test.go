package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gvkhna/clawchrome-cli/internal/client"
)

func TestAXITopLevelHelpAliases(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := Main([]string{arg}, &stdout, &stderr)
			if exitCode != 0 {
				t.Fatalf("expected help exit code 0, got %d; output:\n%s", exitCode, stdout.String())
			}
			assertContainsAll(t, stdout.String(), []string{
				"usage: clawchrome-cli",
				"commands:",
				"open",
				"snapshot",
				"CLAWCHROME_CLI_TRANSPORT",
				"CLAWCHROME_CLI_HTTP_URL",
				"CLAWCHROME_CLI_HTTP_BEARER_TOKEN",
			})
			if stderr.String() != "" {
				t.Fatalf("expected help stderr to be quiet, got %q", stderr.String())
			}
		})
	}
}

func TestAXIEveryCommandSupportsHelpAliasesWithoutSideEffects(t *testing.T) {
	for _, command := range SupportedCommands() {
		for _, helpFlag := range []string{"--help", "-h"} {
			t.Run(command+" "+helpFlag, func(t *testing.T) {
				defer forbidSideEffects(t)()

				var stdout, stderr bytes.Buffer
				exitCode := Main([]string{command, helpFlag}, &stdout, &stderr)
				if exitCode != 0 {
					t.Fatalf("expected help exit code 0, got %d; output:\n%s", exitCode, stdout.String())
				}
				assertContainsAll(t, stdout.String(), []string{
					"usage: clawchrome-cli " + command,
					"examples:",
				})
				if CommandSupportsFullFlag(command) {
					assertContainsAll(t, stdout.String(), []string{"--full"})
				}
				if stderr.String() != "" {
					t.Fatalf("expected command help stderr to be quiet, got %q", stderr.String())
				}
			})
		}
	}
}

func TestAXICommandHelpAliasesCanFollowFlagsOrArgs(t *testing.T) {
	cases := [][]string{
		{"snapshot", "--full", "-h"},
		{"click", "@1", "--help"},
		{"form", "check", "@1", "--help"},
	}

	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			defer forbidSideEffects(t)()

			var stdout, stderr bytes.Buffer
			exitCode := Main(args, &stdout, &stderr)
			if exitCode != 0 {
				t.Fatalf("expected help exit code 0, got %d; output:\n%s", exitCode, stdout.String())
			}
			assertContainsAll(t, stdout.String(), []string{
				"usage: clawchrome-cli " + args[0],
				"examples:",
			})
			if stderr.String() != "" {
				t.Fatalf("expected command help stderr to be quiet, got %q", stderr.String())
			}
		})
	}
}

func TestAXIValidationErrorsAreScopedAndDoNotCallBackend(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantError string
		wantUsage string
	}{
		{"open missing url", []string{"open"}, "Missing URL", "usage: clawchrome-cli open"},
		{"open unknown flag", []string{"open", "https://example.com", "--bogus"}, "Unexpected", "usage: clawchrome-cli open"},
		{"snapshot unknown flag", []string{"snapshot", "--bogus"}, "Unexpected", "usage: clawchrome-cli snapshot"},
		{"screenshot unknown flag", []string{"screenshot", "./page.png", "--bogus"}, "Unexpected", "usage: clawchrome-cli screenshot"},
		{"click missing ref", []string{"click"}, "Missing element ref", "usage: clawchrome-cli click"},
		{"fill missing text", []string{"fill", "@1"}, "Missing fill text", "usage: clawchrome-cli fill"},
		{"type missing text", []string{"type"}, "Missing text", "usage: clawchrome-cli type"},
		{"press missing key", []string{"press"}, "Missing key name", "usage: clawchrome-cli press"},
		{"press unknown flag", []string{"press", "--bogus"}, "Unexpected", "usage: clawchrome-cli press"},
		{"scroll invalid direction", []string{"scroll", "sideways"}, "Unknown scroll direction", "usage: clawchrome-cli scroll"},
		{"scroll unknown flag", []string{"scroll", "--bogus"}, "Unexpected", "usage: clawchrome-cli scroll"},
		{"mouse missing action", []string{"mouse"}, "Missing mouse action", "usage: clawchrome-cli mouse"},
		{"mouse invalid action", []string{"mouse", "doubleclick"}, "Unknown mouse action", "usage: clawchrome-cli mouse"},
		{"mouse move missing coords", []string{"mouse", "move", "10"}, "Missing mouse coordinates", "usage: clawchrome-cli mouse"},
		{"mouse move invalid x", []string{"mouse", "move", "left", "20"}, "Invalid x value", "usage: clawchrome-cli mouse"},
		{"mouse click unexpected arg", []string{"mouse", "click", "10", "20", "extra"}, "Unexpected", "usage: clawchrome-cli mouse"},
		{"mouse drag missing coords", []string{"mouse", "drag", "1", "2", "3"}, "Missing drag coordinates", "usage: clawchrome-cli mouse"},
		{"mouse down invalid button", []string{"mouse", "down", "primary"}, "Invalid mouse button", "usage: clawchrome-cli mouse"},
		{"mouse wheel missing deltas", []string{"mouse", "wheel", "100"}, "Missing wheel deltas", "usage: clawchrome-cli mouse"},
		{"mouse unexpected full", []string{"mouse", "--full"}, "Unexpected", "usage: clawchrome-cli mouse"},
		{"back unexpected arg", []string{"back", "extra"}, "Unexpected", "usage: clawchrome-cli back"},
		{"forward unexpected arg", []string{"forward", "extra"}, "Unexpected", "usage: clawchrome-cli forward"},
		{"reload unexpected arg", []string{"reload", "extra"}, "Unexpected", "usage: clawchrome-cli reload"},
		{"wait missing target", []string{"wait"}, "Missing wait target", "usage: clawchrome-cli wait"},
		{"hover missing ref", []string{"hover"}, "Missing element ref", "usage: clawchrome-cli hover"},
		{"drag missing target", []string{"drag", "@1"}, "Missing element refs", "usage: clawchrome-cli drag"},
		{"fillform invalid entry", []string{"fillform", "name=value"}, "No valid field entries", "usage: clawchrome-cli fillform"},
		{"fillform unknown flag", []string{"fillform", "--bogus"}, "Unexpected", "usage: clawchrome-cli fillform"},
		{"dialog invalid action", []string{"dialog", "close"}, "Missing or invalid action", "usage: clawchrome-cli dialog"},
		{"form missing action", []string{"form"}, "Missing form action", "usage: clawchrome-cli form"},
		{"form invalid action", []string{"form", "toggle"}, "Unknown form action", "usage: clawchrome-cli form"},
		{"form clear missing ref", []string{"form", "clear"}, "Missing element ref", "usage: clawchrome-cli form"},
		{"form clear unexpected arg", []string{"form", "clear", "@1", "extra"}, "Unexpected", "usage: clawchrome-cli form"},
		{"form check missing ref", []string{"form", "check"}, "Missing element ref", "usage: clawchrome-cli form"},
		{"form check unexpected arg", []string{"form", "check", "@1", "extra"}, "Unexpected", "usage: clawchrome-cli form"},
		{"form select missing value", []string{"form", "select", "@2"}, "Missing select value", "usage: clawchrome-cli form"},
		{"form upload missing path", []string{"form", "upload", "@1"}, "Missing file path", "usage: clawchrome-cli form"},
		{"pages unexpected arg", []string{"pages", "extra"}, "Unexpected", "usage: clawchrome-cli pages"},
		{"newpage missing url", []string{"newpage"}, "Missing URL", "usage: clawchrome-cli newpage"},
		{"newpage unknown flag", []string{"newpage", "https://example.com", "--bogus"}, "Unexpected", "usage: clawchrome-cli newpage"},
		{"selectpage invalid id", []string{"selectpage", "abc"}, "Invalid page ID", "usage: clawchrome-cli selectpage"},
		{"selectpage negative id", []string{"selectpage", "-1"}, "Invalid page ID", "usage: clawchrome-cli selectpage"},
		{"closepage invalid id", []string{"closepage", "abc"}, "Invalid page ID", "usage: clawchrome-cli closepage"},
		{"closepage negative id", []string{"closepage", "-1"}, "Invalid page ID", "usage: clawchrome-cli closepage"},
		{"resize invalid size", []string{"resize", "wide", "tall"}, "Width and height must be numbers", "usage: clawchrome-cli resize"},
		{"resize non-positive size", []string{"resize", "0", "720"}, "positive numbers", "usage: clawchrome-cli resize"},
		{"video missing action", []string{"video"}, "Missing video action", "usage: clawchrome-cli video"},
		{"video invalid action", []string{"video", "pause"}, "Unknown video action", "usage: clawchrome-cli video"},
		{"video start unknown flag", []string{"video", "start", "--bogus"}, "Unexpected", "usage: clawchrome-cli video"},
		{"video start unexpected arg", []string{"video", "start", "./a.mp4", "extra"}, "Unexpected", "usage: clawchrome-cli video"},
		{"video stop unexpected arg", []string{"video", "stop", "extra"}, "Unexpected", "usage: clawchrome-cli video"},
		{"video unexpected full", []string{"video", "--full"}, "Unexpected", "usage: clawchrome-cli video"},
		{"start unknown flag", []string{"start", "--bogus"}, "Unexpected", "usage: clawchrome-cli start"},
		{"start missing token value", []string{"start", "--token"}, "Missing token value", "usage: clawchrome-cli start"},
		{"start empty token value", []string{"start", "--token="}, "Missing token value", "usage: clawchrome-cli start"},
		{"start missing agent name", []string{"start", "--agent-name"}, "Missing agent name", "usage: clawchrome-cli start"},
		{"status unexpected arg", []string{"status", "extra"}, "Unexpected", "usage: clawchrome-cli status"},
		{"stop unexpected arg", []string{"stop", "extra"}, "Unexpected", "usage: clawchrome-cli stop"},
		{"version unexpected arg", []string{"version", "extra"}, "Unexpected", "usage: clawchrome-cli version"},
		{"self-update unexpected arg", []string{"self-update", "v1.2.3", "extra"}, "Unexpected", "usage: clawchrome-cli self-update"},
		{"self-update unknown flag", []string{"self-update", "--bogus"}, "Unexpected", "usage: clawchrome-cli self-update"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer forbidSideEffects(t)()

			var stdout, stderr bytes.Buffer
			exitCode := Main(tc.args, &stdout, &stderr)
			if exitCode != 2 {
				t.Fatalf("expected validation exit code 2, got %d; output:\n%s", exitCode, stdout.String())
			}
			assertContainsAll(t, stdout.String(), []string{
				"error",
				tc.wantError,
				tc.wantUsage,
			})
			if stderr.String() != "" {
				t.Fatalf("expected validation stderr to be quiet, got %q", stderr.String())
			}
		})
	}
}

func TestAXIRefArgumentsValidateBeforeBackendCalls(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantUsage string
	}{
		{"click missing sigil", []string{"click", "button1"}, "usage: clawchrome-cli click"},
		{"click empty ref", []string{"click", "@"}, "usage: clawchrome-cli click"},
		{"fill missing sigil", []string{"fill", "name", "Ada"}, "usage: clawchrome-cli fill"},
		{"hover missing sigil", []string{"hover", "menu"}, "usage: clawchrome-cli hover"},
		{"drag bad source", []string{"drag", "source", "@2"}, "usage: clawchrome-cli drag"},
		{"drag bad target", []string{"drag", "@1", "target"}, "usage: clawchrome-cli drag"},
		{"form upload missing sigil", []string{"form", "upload", "file-input", "./photo.jpg"}, "usage: clawchrome-cli form"},
		{"screenshot bad uid", []string{"screenshot", "./button.png", "--uid", "button"}, "usage: clawchrome-cli screenshot"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer forbidSideEffects(t)()

			var stdout, stderr bytes.Buffer
			exitCode := Main(tc.args, &stdout, &stderr)
			if exitCode != 2 {
				t.Fatalf("expected validation exit code 2, got %d; output:\n%s", exitCode, stdout.String())
			}
			assertContainsAll(t, stdout.String(), []string{
				"error",
				"ref",
				tc.wantUsage,
			})
			if stderr.String() != "" {
				t.Fatalf("expected ref validation stderr to be quiet, got %q", stderr.String())
			}
		})
	}
}

func TestAXIHTTPTransportSetupErrorsAreActionable(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	t.Run("http transport uses default target and missing token explains auth", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
		t.Setenv("CLAWCHROME_CLI_HTTP_URL", "")
		t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "")

		code, stdout, stderr := runMainForAXITest([]string{"pages"})
		if code != 2 {
			t.Fatalf("expected validation exit code 2, got %d; output:\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, []string{
			"CLAWCHROME_CLI_HTTP_BEARER_TOKEN",
			"auth",
			"start --token",
			"help[",
		})
		assertNotContainsAny(t, stdout, []string{"Missing target API URL"})
		assertQuietStderr(t, stderr)
	})

	t.Run("missing token explains auth env", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
		t.Setenv("CLAWCHROME_CLI_HTTP_URL", "http://127.0.0.1:9")
		t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "")

		code, stdout, stderr := runMainForAXITest([]string{"pages"})
		if code != 2 {
			t.Fatalf("expected validation exit code 2, got %d; output:\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, []string{
			"CLAWCHROME_CLI_HTTP_BEARER_TOKEN",
			"auth",
			"start --token",
			"help[",
		})
		assertQuietStderr(t, stderr)
	})

	t.Run("invalid url is a validation error with an example", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
		t.Setenv("CLAWCHROME_CLI_HTTP_URL", "localhost:8091")
		t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "secret")

		code, stdout, stderr := runMainForAXITest([]string{"pages"})
		if code != 2 {
			t.Fatalf("expected validation exit code 2, got %d; output:\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, []string{
			"Invalid CLAWCHROME_CLI_HTTP_URL override",
			"CLAWCHROME_CLI_HTTP_URL=http://127.0.0.1:8091",
			"usage: clawchrome-cli pages",
		})
		assertQuietStderr(t, stderr)
	})

	t.Run("unreachable target is interpreted without raw socket details", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
		t.Setenv("CLAWCHROME_CLI_HTTP_URL", "http://127.0.0.1:9")
		t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "secret")

		code, stdout, stderr := runMainForAXITest([]string{"pages"})
		if code != 1 {
			t.Fatalf("expected operation failure exit code 1, got %d; output:\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, []string{
			"BRIDGE_NOT_READY",
			"error",
			"target",
			"unreachable",
		})
		assertNotContainsAny(t, stdout, []string{"dial tcp", "connection refused"})
		assertQuietStderr(t, stderr)
	})

	t.Run("health auth failure is not reported as generic reachability", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
		t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "bad-token")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/health" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized"})
		}))
		defer server.Close()
		t.Setenv("CLAWCHROME_CLI_HTTP_URL", server.URL)

		code, stdout, stderr := runMainForAXITest([]string{"pages"})
		if code != 1 {
			t.Fatalf("expected operation failure exit code 1, got %d; output:\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, []string{"AUTH_ERROR"})
		assertContainsAny(t, strings.ToLower(stdout), []string{"auth", "permission"})
		assertNotContainsAny(t, stdout, []string{"not reachable", "raw", "unauthorized\n"})
		assertQuietStderr(t, stderr)
	})

	t.Run("call auth failure is translated", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
		t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "bad-token")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/health":
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
			case "/call":
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "forbidden"})
			default:
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
		}))
		defer server.Close()
		t.Setenv("CLAWCHROME_CLI_HTTP_URL", server.URL)

		code, stdout, stderr := runMainForAXITest([]string{"pages"})
		if code != 1 {
			t.Fatalf("expected operation failure exit code 1, got %d; output:\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, []string{"AUTH_ERROR"})
		assertContainsAny(t, strings.ToLower(stdout), []string{"auth", "permission"})
		assertNotContainsAny(t, stdout, []string{`{"error":"forbidden"}`, "BROWSER_ERROR"})
		assertQuietStderr(t, stderr)
	})

	t.Run("call server error is translated with server code", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
		t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "secret")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/health":
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
			case "/call":
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "internal failure"})
			default:
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
		}))
		defer server.Close()
		t.Setenv("CLAWCHROME_CLI_HTTP_URL", server.URL)

		code, stdout, stderr := runMainForAXITest([]string{"pages"})
		if code != 1 {
			t.Fatalf("expected operation failure exit code 1, got %d; output:\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, []string{
			"SERVER_ERROR",
			"HTTP 500",
			"internal failure",
		})
		assertNotContainsAny(t, stdout, []string{`{"error":"internal failure"}`})
		assertQuietStderr(t, stderr)
	})

	t.Run("health rate limit is translated with retry context", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
		t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "secret")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/health" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "slow down"})
		}))
		defer server.Close()
		t.Setenv("CLAWCHROME_CLI_HTTP_URL", server.URL)

		code, stdout, stderr := runMainForAXITest([]string{"pages"})
		if code != 1 {
			t.Fatalf("expected operation failure exit code 1, got %d; output:\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, []string{
			"RATE_LIMITED",
			"rate limited",
			"Retry after 5",
			"slow down",
		})
		if count := strings.Count(stdout, "Retry after 5"); count != 1 {
			t.Fatalf("expected Retry-After guidance once, got %d; output:\n%s", count, stdout)
		}
		assertNotContainsAny(t, stdout, []string{`{"error":"slow down"}`})
		assertQuietStderr(t, stderr)
	})

	t.Run("call rate limit is translated with retry context", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
		t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "secret")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/health":
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
			case "/call":
				w.Header().Set("Retry-After", "10")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "quota exceeded"})
			default:
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
		}))
		defer server.Close()
		t.Setenv("CLAWCHROME_CLI_HTTP_URL", server.URL)

		code, stdout, stderr := runMainForAXITest([]string{"pages"})
		if code != 1 {
			t.Fatalf("expected operation failure exit code 1, got %d; output:\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, []string{
			"RATE_LIMITED",
			"/call",
			"Retry after 10",
			"quota exceeded",
		})
		if count := strings.Count(stdout, "Retry after 10"); count != 1 {
			t.Fatalf("expected Retry-After guidance once, got %d; output:\n%s", count, stdout)
		}
		assertNotContainsAny(t, stdout, []string{`{"error":"quota exceeded"}`})
		assertQuietStderr(t, stderr)
	})

	t.Run("temporary server failure is translated with retry context", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
		t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "secret")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/health":
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
			case "/call":
				w.Header().Set("Retry-After", "30")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]any{"detail": "maintenance"})
			default:
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
		}))
		defer server.Close()
		t.Setenv("CLAWCHROME_CLI_HTTP_URL", server.URL)

		code, stdout, stderr := runMainForAXITest([]string{"pages"})
		if code != 1 {
			t.Fatalf("expected operation failure exit code 1, got %d; output:\n%s", code, stdout)
		}
		assertContainsAll(t, stdout, []string{
			"BRIDGE_NOT_READY",
			"temporarily unavailable",
			"Retry after 30",
			"maintenance",
		})
		if count := strings.Count(stdout, "Retry after 30"); count != 1 {
			t.Fatalf("expected Retry-After guidance once, got %d; output:\n%s", count, stdout)
		}
		assertNotContainsAny(t, stdout, []string{`{"detail":"maintenance"}`})
		assertQuietStderr(t, stderr)
	})
}

func TestAXIRuntimeRefNotFoundErrorsAreTranslated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAWCHROME_CLI_TRANSPORT", "http")
	t.Setenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN", "secret")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		case "/call":
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "AT-SPI object uid=missing not found in runtime tree"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("CLAWCHROME_CLI_HTTP_URL", server.URL)

	code, stdout, stderr := runMainForAXITest([]string{"click", "@missing"})
	if code != 1 {
		t.Fatalf("expected operation failure exit code 1, got %d; output:\n%s", code, stdout)
	}
	assertContainsAll(t, stdout, []string{
		"error",
		"@missing",
		"snapshot",
	})
	assertNotContainsAny(t, stdout, []string{"AT-SPI", "runtime tree", `{"error":`})
	assertQuietStderr(t, stderr)
}

func runMainForAXITest(args []string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	exitCode := Main(args, &stdout, &stderr)
	return exitCode, stdout.String(), stderr.String()
}

func forbidSideEffects(t *testing.T) func() {
	t.Helper()

	restores := []func(){
		stubCallTool(t, func(name string, args map[string]any) (string, error) {
			t.Fatalf("validation/help path called backend tool %q with args %#v", name, args)
			return "", nil
		}),
		stubCallRuntimeHTTPTool(t, func(name string, args map[string]any) (client.RuntimeHTTPToolResponse, error) {
			t.Fatalf("validation/help path called runtime HTTP tool %q with args %#v", name, args)
			return client.RuntimeHTTPToolResponse{}, nil
		}),
		stubEnsureBridge(t, func() (int, error) {
			t.Fatalf("validation/help path attempted to ensure bridge")
			return 0, nil
		}),
		stubStopBridge(t, func() bool {
			t.Fatalf("validation/help path attempted to stop bridge")
			return false
		}),
		stubSelfUpdate(t, func(_ string, _ string) (string, error) {
			t.Fatalf("validation/help path attempted self-update")
			return "", nil
		}),
	}

	return func() {
		for i := len(restores) - 1; i >= 0; i-- {
			restores[i]()
		}
	}
}

func stubStopBridge(t *testing.T, fn func() bool) func() {
	t.Helper()
	prev := stopBridge
	stopBridge = fn
	return func() {
		stopBridge = prev
	}
}

func assertContainsAll(t *testing.T, text string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("expected output to contain %q; output:\n%s", want, text)
		}
	}
}

func assertContainsAny(t *testing.T, text string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if strings.Contains(text, want) {
			return
		}
	}
	t.Fatalf("expected output to contain one of %#v; output:\n%s", wants, text)
}

func assertNotContainsAny(t *testing.T, text string, bad []string) {
	t.Helper()
	for _, value := range bad {
		if strings.Contains(text, value) {
			t.Fatalf("expected output not to contain %q; output:\n%s", value, text)
		}
	}
}

func assertQuietStderr(t *testing.T, stderr string) {
	t.Helper()
	if stderr != "" {
		t.Fatalf("expected stderr to be quiet, got %q", stderr)
	}
}
