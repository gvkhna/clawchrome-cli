package client

import (
	"encoding/json"
	"errors"
	"fmt"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultConfigDirName      = "clawchrome-cli"
	defaultAuthConfigFileName = "auth.json"
	authSourceConfig          = "config"
	authSourceEnv             = "env"
	authSourceMissing         = "missing"
)

type AuthConfigStatus struct {
	Token        string `json:"token"`
	Source       string `json:"source,omitempty"`
	AgentName    string `json:"agentName,omitempty"`
	ConfigPath   string `json:"configPath"`
	ConfigExists bool   `json:"configExists"`
}

type ClientStatus struct {
	Status ClientRuntimeStatus `json:"status"`
	Auth   AuthConfigStatus    `json:"auth"`
}

type ClientRuntimeStatus struct {
	Transport string `json:"transport"`
	Target    string `json:"target,omitempty"`
}

type storedAuthConfig struct {
	Token     string `json:"token,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
}

func SaveAuthConfig(token string, agentName string) (AuthConfigStatus, error) {
	token = strings.TrimSpace(token)
	agentName = strings.TrimSpace(agentName)
	if token == "" && agentName == "" {
		return AuthConfigStatus{}, WrapError("Missing auth value: provide --token or --agent-name", ErrValidation)
	}
	if token != "" {
		if err := validateToken(token); err != nil {
			return AuthConfigStatus{}, err
		}
	}
	if agentName != "" {
		if err := validateAgentName(agentName); err != nil {
			return AuthConfigStatus{}, err
		}
	}

	cfg, path, _, err := readStoredAuthConfig()
	if err != nil {
		return AuthConfigStatus{}, err
	}
	if token != "" {
		cfg.Token = token
	}
	if agentName != "" {
		cfg.AgentName = agentName
	}
	if err := writeStoredAuthConfig(path, cfg); err != nil {
		return AuthConfigStatus{}, err
	}
	return GetAuthStatus()
}

func GetAuthStatus() (AuthConfigStatus, error) {
	cfg, path, exists, err := readStoredAuthConfig()
	if err != nil {
		return AuthConfigStatus{}, err
	}
	status := AuthConfigStatus{
		Token:        authSourceMissing,
		Source:       authSourceMissing,
		AgentName:    cfg.AgentName,
		ConfigPath:   path,
		ConfigExists: exists,
	}
	if strings.TrimSpace(os.Getenv("CLAWCHROME_CLI_HTTP_BEARER_TOKEN")) != "" {
		status.Token = "configured"
		status.Source = authSourceEnv
		return status, nil
	}
	if cfg.Token != "" {
		status.Token = "configured"
		status.Source = authSourceConfig
	}
	return status, nil
}

func GetClientStatus() (ClientStatus, error) {
	mode := strings.TrimSpace(os.Getenv("CLAWCHROME_CLI_TRANSPORT"))
	if mode == "" {
		mode = transportHTTP
	}
	if mode != transportStdio && mode != transportHTTP {
		return ClientStatus{}, WrapError("Unsupported CLAWCHROME_CLI_TRANSPORT: "+mode, ErrValidation)
	}

	auth, err := GetAuthStatus()
	if err != nil {
		return ClientStatus{}, err
	}
	status := ClientStatus{
		Status: ClientRuntimeStatus{Transport: mode},
		Auth:   auth,
	}
	if mode == transportHTTP {
		baseURL, err := resolveHTTPBaseURL()
		if err != nil {
			return ClientStatus{}, err
		}
		status.Status.Target = baseURL
	}
	return status, nil
}

func resolveStoredAuthConfig() (storedAuthConfig, string, bool, error) {
	return readStoredAuthConfig()
}

func resolveHTTPBaseURL() (string, error) {
	baseURL := strings.TrimSpace(os.Getenv("CLAWCHROME_CLI_HTTP_URL"))
	if baseURL == "" {
		baseURL = defaultRuntimeHTTPURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if err := validateHTTPBaseURL(baseURL); err != nil {
		return "", err
	}
	return baseURL, nil
}

func validateHTTPBaseURL(baseURL string) error {
	parsedURL, err := neturl.Parse(baseURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return WrapError(
			"Invalid CLAWCHROME_CLI_HTTP_URL override: expected an http or https URL",
			ErrValidation,
			"Example: CLAWCHROME_CLI_HTTP_URL=http://127.0.0.1:8091",
		)
	}
	return nil
}

func readStoredAuthConfig() (storedAuthConfig, string, bool, error) {
	path, err := authConfigFilePath()
	if err != nil {
		return storedAuthConfig{}, "", false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return storedAuthConfig{}, path, false, nil
	}
	if err != nil {
		return storedAuthConfig{}, path, false, WrapError(
			fmt.Sprintf("Unable to read auth config at %s: %v", path, err),
			ErrValidation,
			"Check the auth config file permissions",
		)
	}
	var cfg storedAuthConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return storedAuthConfig{}, path, true, WrapError(
			fmt.Sprintf("Invalid auth config at %s: %v", path, err),
			ErrValidation,
			"Run `clawchrome-cli start --token <token>` to rewrite auth config",
		)
	}
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.AgentName = strings.TrimSpace(cfg.AgentName)
	if cfg.Token != "" {
		if err := validateToken(cfg.Token); err != nil {
			return storedAuthConfig{}, path, true, err
		}
	}
	if cfg.AgentName != "" {
		if err := validateAgentName(cfg.AgentName); err != nil {
			return storedAuthConfig{}, path, true, err
		}
	}
	return cfg, path, true, nil
}

func writeStoredAuthConfig(path string, cfg storedAuthConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return WrapError(
			fmt.Sprintf("Unable to create auth config directory at %s: %v", dir, err),
			ErrValidation,
		)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return WrapError(
			fmt.Sprintf("Unable to secure auth config directory at %s: %v", dir, err),
			ErrValidation,
		)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return WrapError(
			fmt.Sprintf("Unable to write auth config at %s: %v", path, err),
			ErrValidation,
		)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return WrapError(
			fmt.Sprintf("Unable to secure auth config at %s: %v", path, err),
			ErrValidation,
		)
	}
	return nil
}

func authConfigFilePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", WrapError(
			"Unable to resolve user config directory for auth config",
			ErrValidation,
			"Set HOME or the platform user config directory environment before saving auth",
		)
	}
	return filepath.Join(configDir, defaultConfigDirName, defaultAuthConfigFileName), nil
}

func validateToken(token string) error {
	if token == "" {
		return WrapError("Missing auth token: --token value is empty", ErrValidation)
	}
	if hasControlOrWhitespace(token) {
		return WrapError("Invalid auth token: token cannot contain whitespace or control characters", ErrValidation)
	}
	return nil
}

func validateAgentName(agentName string) error {
	if agentName == "" {
		return WrapError("Missing agent name: --agent-name value is empty", ErrValidation)
	}
	if len(agentName) > 128 {
		return WrapError("Invalid agent name: must be 128 characters or fewer", ErrValidation)
	}
	for _, r := range agentName {
		if r < 32 || r == 127 {
			return WrapError("Invalid agent name: cannot contain control characters", ErrValidation)
		}
	}
	return nil
}

func hasControlOrWhitespace(value string) bool {
	for _, r := range value {
		if r <= 32 || r == 127 {
			return true
		}
	}
	return false
}
