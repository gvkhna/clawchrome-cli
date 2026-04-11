package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	repoOwner         = "gvkhna"
	repoName          = "clawchrome-cli"
	checkTimeout      = 5 * time.Second
	downloadTimeout   = 30 * time.Second
	cacheTTL          = 24 * time.Hour
	checksumAssetName = "checksums.txt"
)

var httpClient = &http.Client{Timeout: checkTimeout}
var latestReleaseAPIURL = apiLatestReleaseURL()

type release struct {
	TagName string `json:"tag_name"`
}

type cacheEntry struct {
	CheckedAt     time.Time `json:"checked_at"`
	LatestVersion string    `json:"latest_version"`
}

func Notice(ctx context.Context, currentVersion string) (string, error) {
	if currentVersion == "" || strings.Contains(currentVersion, "dev") {
		return "", nil
	}
	latest, err := cachedLatestVersion(ctx)
	if err != nil || latest == "" {
		return "", err
	}
	if !isNewerVersion(currentVersion, latest) {
		return "", nil
	}
	return fmt.Sprintf("update available: %s -> run `clawchrome-cli self-update`", latest), nil
}

func SelfUpdate(ctx context.Context, currentVersion string, targetVersion string) (string, error) {
	if runtime.GOOS == "windows" {
		return "", errors.New("self-update is not supported on windows yet")
	}

	version := targetVersion
	if version == "" {
		latest, err := latestVersion(ctx)
		if err != nil {
			return "", err
		}
		version = latest
	}
	if version == "" {
		return "", errors.New("could not determine release version")
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	if currentVersion == version {
		return version, nil
	}

	assetName, err := assetName(version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	checksums, err := downloadText(ctx, checksumsURL(version))
	if err != nil {
		return "", err
	}
	binaryData, err := downloadBinary(ctx, binaryURL(version, assetName))
	if err != nil {
		return "", err
	}
	if err := verifyChecksum(binaryData, checksums, assetName); err != nil {
		return "", err
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, ".clawchrome-cli-update-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(binaryData); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		return "", err
	}

	return version, nil
}

func latestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseAPIURL, nil)
	if err != nil {
		return "", err
	}
	client := *httpClient
	client.Timeout = checkTimeout
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", repoName)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("release lookup failed with status %d", resp.StatusCode)
	}
	var r release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	return r.TagName, nil
}

func cachedLatestVersion(ctx context.Context) (string, error) {
	cachePath, err := cacheFilePath()
	if err != nil {
		return "", err
	}
	if data, err := os.ReadFile(cachePath); err == nil {
		var entry cacheEntry
		if json.Unmarshal(data, &entry) == nil && time.Since(entry.CheckedAt) < cacheTTL {
			return entry.LatestVersion, nil
		}
	}

	latest, err := latestVersion(ctx)
	if err != nil {
		return "", err
	}
	entry := cacheEntry{
		CheckedAt:     time.Now().UTC(),
		LatestVersion: latest,
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
		if data, err := json.Marshal(entry); err == nil {
			_ = os.WriteFile(cachePath, data, 0o644)
		}
	}
	return latest, nil
}

func cacheFilePath() (string, error) {
	if cacheDir := os.Getenv("XDG_CACHE_HOME"); cacheDir != "" {
		return filepath.Join(cacheDir, repoName, "update.json"), nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".cache", repoName, "update.json"), nil
}

func apiLatestReleaseURL() string {
	return fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
}

func checksumsURL(version string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", repoOwner, repoName, version, checksumAssetName)
}

func binaryURL(version string, assetName string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", repoOwner, repoName, version, assetName)
}

func assetName(version string, goos string, goarch string) (string, error) {
	switch goos {
	case "linux", "darwin":
	case "windows":
	default:
		return "", fmt.Errorf("unsupported OS %q for self-update", goos)
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported architecture %q for self-update", goarch)
	}
	name := fmt.Sprintf("%s_%s_%s_%s", repoName, strings.TrimPrefix(version, "v"), goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name, nil
}

func downloadText(ctx context.Context, url string) (string, error) {
	data, err := downloadBinary(ctx, url)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func downloadBinary(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := *httpClient
	client.Timeout = downloadTimeout
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", repoName)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func verifyChecksum(binaryData []byte, checksums string, assetName string) error {
	sum := sha256.Sum256(binaryData)
	want := hex.EncodeToString(sum[:])
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name == assetName {
			if fields[0] != want {
				return fmt.Errorf("checksum mismatch for %s", assetName)
			}
			return nil
		}
	}
	return fmt.Errorf("checksum entry not found for %s", assetName)
}

func isNewerVersion(currentVersion string, latestVersion string) bool {
	current, okCurrent := parseVersion(currentVersion)
	latest, okLatest := parseVersion(latestVersion)
	if !okCurrent || !okLatest {
		return false
	}
	for i := 0; i < 3; i++ {
		if latest[i] > current[i] {
			return true
		}
		if latest[i] < current[i] {
			return false
		}
	}
	return false
}

func parseVersion(version string) ([3]int, bool) {
	var parsed [3]int
	version = strings.TrimPrefix(version, "v")
	parts := strings.SplitN(version, "-", 2)
	fields := strings.Split(parts[0], ".")
	if len(fields) != 3 {
		return parsed, false
	}
	for i := 0; i < 3; i++ {
		value := 0
		for _, r := range fields[i] {
			if r < '0' || r > '9' {
				return parsed, false
			}
			value = value*10 + int(r-'0')
		}
		parsed[i] = value
	}
	return parsed, true
}
