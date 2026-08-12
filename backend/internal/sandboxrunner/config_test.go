// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFailsClosedForMissingOrUnsafeRuntimeSettings(t *testing.T) {
	valid := validRunnerEnvironment()
	for name, mutate := range map[string]func(map[string]string){
		"disabled":                     func(values map[string]string) { values["SANDBOX_RUNNER_ENABLED"] = "false" },
		"missing authentication token": func(values map[string]string) { delete(values, "SANDBOX_RUNNER_AUTH_TOKEN") },
		"mutable image tag": func(values map[string]string) {
			values["SANDBOX_RUNNER_EXECUTION_IMAGE"] = "registry.example/runtime:latest"
		},
		"non unix runtime endpoint": func(values map[string]string) { values["SANDBOX_RUNNER_ROOTLESS_ENDPOINT"] = "tcp://runner:2375" },
		"emergency stop":            func(values map[string]string) { values["SANDBOX_RUNNER_EMERGENCY_STOP"] = "true" },
		"tls omitted outside loopback debug": func(values map[string]string) {
			values["SANDBOX_RUNNER_DEBUG_LOOPBACK_HTTP"] = "false"
			values["SANDBOX_RUNNER_LISTEN_ADDR"] = ":9443"
			delete(values, "SANDBOX_RUNNER_TLS_CERT_FILE")
			delete(values, "SANDBOX_RUNNER_TLS_KEY_FILE")
		},
		"context keyring trailing data": func(values map[string]string) { values["SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON"] += " {}" },
		"context keyring duplicate key": func(values map[string]string) {
			values["SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON"] = `{"active_key_id":"key-1","active_key_id":"key-2","keys":{"key-1":"0123456789abcdef0123456789abcdef"}}`
		},
		"invalid debug boolean": func(values map[string]string) {
			values["SANDBOX_RUNNER_DEBUG_LOOPBACK_HTTP"] = "sometimes"
		},
		"invalid listener port": func(values map[string]string) {
			values["SANDBOX_RUNNER_LISTEN_ADDR"] = "127.0.0.1:not-a-port"
		},
	} {
		t.Run(name, func(t *testing.T) {
			values := cloneRunnerEnvironment(valid)
			mutate(values)
			if _, err := LoadConfig(mapGetenv(values)); err == nil {
				t.Fatal("LoadConfig() unexpectedly succeeded")
			}
		})
	}
}

func TestLoadConfigRejectsMissingAndPermissiveTLSSecrets(t *testing.T) {
	values := validRunnerEnvironment()
	values["SANDBOX_RUNNER_DEBUG_LOOPBACK_HTTP"] = "false"
	values["SANDBOX_RUNNER_LISTEN_ADDR"] = ":9443"
	values["SANDBOX_RUNNER_TLS_CERT_FILE"] = filepath.Join(t.TempDir(), "missing.crt")
	values["SANDBOX_RUNNER_TLS_KEY_FILE"] = filepath.Join(t.TempDir(), "missing.key")
	if _, err := LoadConfig(mapGetenv(values)); err == nil {
		t.Fatal("LoadConfig() accepted missing TLS secret files")
	}

	directory := t.TempDir()
	cert := filepath.Join(directory, "runner.crt")
	key := filepath.Join(directory, "runner.key")
	for _, path := range []string{cert, key} {
		if err := os.WriteFile(path, []byte("test-secret"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	values["SANDBOX_RUNNER_TLS_CERT_FILE"] = cert
	values["SANDBOX_RUNNER_TLS_KEY_FILE"] = key
	if _, err := LoadConfig(mapGetenv(values)); err == nil {
		t.Fatal("LoadConfig() accepted world-readable TLS secrets")
	}
	if err := os.Chmod(cert, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(key, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(mapGetenv(values)); err != nil {
		t.Fatalf("LoadConfig() rejected secure TLS secret files: %v", err)
	}
}

func TestLoadConfigAllowsLoopbackDebugHTTPOnly(t *testing.T) {
	values := validRunnerEnvironment()
	values["SANDBOX_RUNNER_LISTEN_ADDR"] = "127.0.0.1:9443"
	values["SANDBOX_RUNNER_DEBUG_LOOPBACK_HTTP"] = "true"
	delete(values, "SANDBOX_RUNNER_TLS_CERT_FILE")
	delete(values, "SANDBOX_RUNNER_TLS_KEY_FILE")
	config, err := LoadConfig(mapGetenv(values))
	if err != nil || !config.DebugLoopbackHTTP || config.ListenAddr != "127.0.0.1:9443" {
		t.Fatalf("LoadConfig() config/error = %#v/%v", config, err)
	}
}

func TestLoadConfigRejectsMalformedLoopbackDebugSettings(t *testing.T) {
	for name, mutate := range map[string]func(map[string]string){
		"invalid boolean": func(values map[string]string) {
			values["SANDBOX_RUNNER_DEBUG_LOOPBACK_HTTP"] = "sometimes"
		},
		"invalid port": func(values map[string]string) {
			values["SANDBOX_RUNNER_LISTEN_ADDR"] = "127.0.0.1:not-a-port"
		},
	} {
		t.Run(name, func(t *testing.T) {
			values := validRunnerEnvironment()
			values["SANDBOX_RUNNER_LISTEN_ADDR"] = "127.0.0.1:9443"
			values["SANDBOX_RUNNER_DEBUG_LOOPBACK_HTTP"] = "true"
			delete(values, "SANDBOX_RUNNER_TLS_CERT_FILE")
			delete(values, "SANDBOX_RUNNER_TLS_KEY_FILE")
			mutate(values)
			if _, err := LoadConfig(mapGetenv(values)); err == nil {
				t.Fatal("LoadConfig() unexpectedly accepted malformed debug setting")
			}
		})
	}
}

func validRunnerEnvironment() map[string]string {
	return map[string]string{
		"SANDBOX_RUNNER_ENABLED":                  "true",
		"SANDBOX_RUNNER_LISTEN_ADDR":              ":9443",
		"SANDBOX_RUNNER_TLS_CERT_FILE":            "/run/secrets/sandbox-runner.crt",
		"SANDBOX_RUNNER_TLS_KEY_FILE":             "/run/secrets/sandbox-runner.key",
		"SANDBOX_RUNNER_AUTH_TOKEN":               "runner-auth-token-0123456789",
		"SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON": `{"active_key_id":"key-1","keys":{"key-1":"0123456789abcdef0123456789abcdef"}}`,
		"SANDBOX_RUNNER_QUEUE_KEYS_JSON":          `{"active_key_id":"key-1","keys":{"key-1":"0123456789abcdef0123456789abcdef"}}`,
		"SANDBOX_RUNNER_ACTIVE_QUEUE_KEY_ID":      "key-1",
		"SANDBOX_RUNNER_ROOTLESS_ENDPOINT":        "unix:///run/user/10001/podman/podman.sock",
		"SANDBOX_RUNNER_EXECUTION_IMAGE":          "registry.example/coze-sandbox-runtime@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"SANDBOX_RUNNER_EMERGENCY_STOP":           "false",
	}
}

func mapGetenv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func cloneRunnerEnvironment(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
