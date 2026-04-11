package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssetName(t *testing.T) {
	got, err := assetName("v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "clawchrome-cli_1.2.3_linux_amd64" {
		t.Fatalf("unexpected asset name: %s", got)
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("hello")
	checksums := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824  clawchrome-cli_1.2.3_linux_amd64\n"
	if err := verifyChecksum(data, checksums, "clawchrome-cli_1.2.3_linux_amd64"); err != nil {
		t.Fatalf("unexpected checksum error: %v", err)
	}
}

func TestIsNewerVersion(t *testing.T) {
	if !isNewerVersion("v1.0.0", "v1.0.1") {
		t.Fatalf("expected newer version")
	}
	if isNewerVersion("v1.2.0", "v1.1.9") {
		t.Fatalf("did not expect downgrade to compare newer")
	}
}

func TestCachedLatestVersionUsesCache(t *testing.T) {
	dir := t.TempDir()
	prev := os.Getenv("XDG_CACHE_HOME")
	if err := os.Setenv("XDG_CACHE_HOME", dir); err != nil {
		t.Fatalf("setenv failed: %v", err)
	}
	defer func() {
		_ = os.Setenv("XDG_CACHE_HOME", prev)
	}()

	cachePath, err := cacheFilePath()
	if err != nil {
		t.Fatalf("cacheFilePath failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	entry := cacheEntry{
		CheckedAt:     time.Now().UTC(),
		LatestVersion: "v9.9.9",
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	got, err := cachedLatestVersion(context.Background())
	if err != nil {
		t.Fatalf("cachedLatestVersion failed: %v", err)
	}
	if got != "v9.9.9" {
		t.Fatalf("unexpected cached version: %s", got)
	}
}

func TestLatestVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer server.Close()

	prevClient := httpClient
	httpClient = server.Client()
	prevURL := latestReleaseAPIURL
	latestReleaseAPIURL = server.URL
	defer func() {
		httpClient = prevClient
		latestReleaseAPIURL = prevURL
	}()

	got, err := latestVersion(context.Background())
	if err != nil {
		t.Fatalf("latestVersion failed: %v", err)
	}
	if got != "v1.2.3" {
		t.Fatalf("unexpected latest version: %s", got)
	}
}

func TestNoticeSkipsDevVersion(t *testing.T) {
	got, err := Notice(context.Background(), "0.1.0-dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty notice for dev version, got %q", got)
	}
}

func TestParseVersion(t *testing.T) {
	got, ok := parseVersion("v1.2.3")
	if !ok || got != [3]int{1, 2, 3} {
		t.Fatalf("unexpected parsed version: %#v ok=%v", got, ok)
	}
}

func TestBinaryURL(t *testing.T) {
	url := binaryURL("v1.2.3", "clawchrome-cli_1.2.3_linux_amd64")
	if !strings.Contains(url, "/download/v1.2.3/clawchrome-cli_1.2.3_linux_amd64") {
		t.Fatalf("unexpected binary URL: %s", url)
	}
}
