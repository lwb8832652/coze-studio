// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

// Package sandboxrunner implements the Runner-side, authenticated protocol
// boundary. Scheduling, persistence, and container lifecycle are injected in
// later layers; this package never falls back to host execution.
package sandboxrunner

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

var ErrConfiguration = errors.New("sandbox runner configuration is invalid")

var errAIOUpstreamRedirect = errors.New("AIO upstream redirect is forbidden")

const aioHTTPTimeout = 30 * time.Second

type Config struct {
	DeploymentID          string
	ListenAddr            string
	TLSCertFile           string
	TLSKeyFile            string
	DebugLoopbackHTTP     bool
	AuthToken             string
	ContextVerifyKeys     sandboxidentity.Keyring
	SchedulerConfigKeys   keyringConfig
	QueueKeys             keyringConfig
	ActiveQueueKeyID      string
	RootlessEndpoint      string
	ExecutionImage        string
	SessionBackendEnabled bool
	AIOUpstreamURL        string
	AIOBearerToken        string
}

func (Config) String() string   { return "sandboxrunner.Config{secrets:<redacted>}" }
func (Config) GoString() string { return "sandboxrunner.Config{secrets:<redacted>}" }

type keyringConfig struct {
	ActiveKeyID string            `json:"active_key_id"`
	Keys        map[string]string `json:"keys"`
}

func LoadConfig(getenv func(string) string) (Config, error) {
	if getenv == nil || getenv("SANDBOX_RUNNER_ENABLED") != "true" ||
		getenv("SANDBOX_RUNNER_EMERGENCY_STOP") != "false" {
		return Config{}, ErrConfiguration
	}
	config := Config{
		DeploymentID:     strings.TrimSpace(getenv("SANDBOX_RUNNER_DEPLOYMENT_ID")),
		ListenAddr:       strings.TrimSpace(getenv("SANDBOX_RUNNER_LISTEN_ADDR")),
		TLSCertFile:      strings.TrimSpace(getenv("SANDBOX_RUNNER_TLS_CERT_FILE")),
		TLSKeyFile:       strings.TrimSpace(getenv("SANDBOX_RUNNER_TLS_KEY_FILE")),
		AuthToken:        getenv("SANDBOX_RUNNER_AUTH_TOKEN"),
		ActiveQueueKeyID: strings.TrimSpace(getenv("SANDBOX_RUNNER_ACTIVE_QUEUE_KEY_ID")),
		RootlessEndpoint: strings.TrimSpace(getenv("SANDBOX_RUNNER_ROOTLESS_ENDPOINT")),
		ExecutionImage:   strings.TrimSpace(getenv("SANDBOX_RUNNER_EXECUTION_IMAGE")),
	}
	debugValue := getenv("SANDBOX_RUNNER_DEBUG_LOOPBACK_HTTP")
	if debugValue != "" && debugValue != "true" && debugValue != "false" {
		return Config{}, ErrConfiguration
	}
	config.DebugLoopbackHTTP = debugValue == "true"
	sessionValue := getenv("SANDBOX_RUNNER_SESSION_ENABLED")
	if sessionValue != "" && sessionValue != "true" && sessionValue != "false" {
		return Config{}, ErrConfiguration
	}
	config.SessionBackendEnabled = sessionValue == "true"
	if !validKeyID(config.DeploymentID) || config.ListenAddr == "" || len(config.AuthToken) < 16 || !validUnixEndpoint(config.RootlessEndpoint) ||
		!validDigestImage(config.ExecutionImage) || !validListener(config.ListenAddr) {
		return Config{}, ErrConfiguration
	}
	if (config.TLSCertFile == "") != (config.TLSKeyFile == "") {
		return Config{}, ErrConfiguration
	}
	if config.TLSCertFile == "" && (!config.DebugLoopbackHTTP || !listenerIsLoopback(config.ListenAddr)) {
		return Config{}, ErrConfiguration
	}
	if config.TLSCertFile != "" && (!validSecretFile(config.TLSCertFile) || !validSecretFile(config.TLSKeyFile)) {
		return Config{}, ErrConfiguration
	}
	var err error
	if config.ContextVerifyKeys, err = loadIdentityKeyring(getenv("SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON")); err != nil {
		return Config{}, ErrConfiguration
	}
	if config.SchedulerConfigKeys, err = loadKeyringConfig(getenv("SANDBOX_RUNNER_SCHEDULER_CONFIG_VERIFY_KEYS_JSON")); err != nil {
		return Config{}, ErrConfiguration
	}
	if config.QueueKeys, err = loadKeyringConfig(getenv("SANDBOX_RUNNER_QUEUE_KEYS_JSON")); err != nil ||
		config.ActiveQueueKeyID == "" || config.ActiveQueueKeyID != config.QueueKeys.ActiveKeyID {
		return Config{}, ErrConfiguration
	}
	if config.SessionBackendEnabled {
		config.AIOUpstreamURL = strings.TrimSpace(getenv("SANDBOX_RUNNER_AIO_UPSTREAM_URL"))
		config.AIOBearerToken = getenv("SANDBOX_RUNNER_AIO_BEARER_TOKEN")
		if !validMySQLDSN(getenv("MYSQL_DSN")) || !validRedisAddress(getenv("REDIS_ADDR")) ||
			!validAIOUpstreamOrigin(config.AIOUpstreamURL, config.DebugLoopbackHTTP) ||
			!validHTTPHeaderValue(config.AIOBearerToken) {
			return Config{}, ErrConfiguration
		}
	}
	return config, nil
}

// NewAIOHTTPClient returns a bounded client that never follows redirects or
// inherits environment proxy configuration. AIO is addressed only through the
// validated, exact private origin from Config.
func NewAIOHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &http.Client{
		Transport: transport,
		Timeout:   aioHTTPTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errAIOUpstreamRedirect
		},
	}
}

func validMySQLDSN(raw string) bool {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return false
	}
	_, err := mysqldriver.ParseDSN(raw)
	return err == nil
}

func validRedisAddress(raw string) bool {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return false
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil || host == "" || port == "" {
		return false
	}
	value, err := strconv.Atoi(port)
	return err == nil && value > 0 && value <= 65535
}

func validAIOUpstreamOrigin(raw string, allowLoopback bool) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Opaque != "" || parsed.User != nil ||
		parsed.Host == "" || parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" || parsed.Port() != "8080" {
		return false
	}
	host := parsed.Hostname()
	if host == "" || strings.ContainsAny(host, " \t\r\n") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return allowLoopback
		}
		return isPrivateAIOIP(ip)
	}
	if host == "localhost" {
		return allowLoopback
	}
	return validComposeServiceName(host)
}

func isPrivateAIOIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4[0] == 10 || ipv4[0] == 172 && ipv4[1] >= 16 && ipv4[1] <= 31 || ipv4[0] == 192 && ipv4[1] == 168
	}
	bytes := ip.To16()
	return bytes != nil && bytes[0]&0xfe == 0xfc
}

func validComposeServiceName(host string) bool {
	if host == "" || len(host) > 63 || strings.Contains(host, ".") || host[0] == '-' || host[len(host)-1] == '-' {
		return false
	}
	hasLetter := false
	for _, char := range host {
		switch {
		case char >= 'a' && char <= 'z':
			hasLetter = true
		case char >= '0' && char <= '9', char == '-':
		default:
			return false
		}
	}
	return hasLetter
}

func validHTTPHeaderValue(value string) bool {
	return !strings.ContainsAny(value, "\r\n")
}

func loadIdentityKeyring(raw string) (sandboxidentity.Keyring, error) {
	keyring, err := loadKeyringConfig(raw)
	if err != nil {
		return sandboxidentity.Keyring{}, err
	}
	keys := make(map[string][]byte, len(keyring.Keys))
	for id, value := range keyring.Keys {
		keys[id] = []byte(value)
	}
	return sandboxidentity.NewKeyring(keyring.ActiveKeyID, keys, 5*time.Minute)
}

func loadKeyringConfig(raw string) (keyringConfig, error) {
	var value keyringConfig
	if err := rejectDuplicateJSONKeys([]byte(raw)); err != nil {
		return keyringConfig{}, ErrConfiguration
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if raw == "" || decoder.Decode(&value) != nil || value.ActiveKeyID == "" || len(value.Keys) == 0 || value.Keys[value.ActiveKeyID] == "" {
		return keyringConfig{}, ErrConfiguration
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return keyringConfig{}, ErrConfiguration
	}
	for id, secret := range value.Keys {
		if !validKeyID(id) || len(secret) < 16 {
			return keyringConfig{}, ErrConfiguration
		}
	}
	return value, nil
}

func validSecretFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o077 == 0
}

func validListener(value string) bool {
	_, port, err := net.SplitHostPort(value)
	if err != nil || port == "" {
		return false
	}
	number, err := strconv.Atoi(port)
	return err == nil && number > 0 && number <= 65535
}

func listenerIsLoopback(value string) bool {
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validUnixEndpoint(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "unix" && parsed.Host == "" && strings.HasPrefix(parsed.Path, "/")
}

func validDigestImage(value string) bool {
	parts := strings.Split(value, "@sha256:")
	if len(parts) != 2 || parts[0] == "" || len(parts[1]) != 64 {
		return false
	}
	for _, char := range parts[1] {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return false
		}
	}
	return true
}

func validKeyID(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.') {
			return false
		}
	}
	return true
}
