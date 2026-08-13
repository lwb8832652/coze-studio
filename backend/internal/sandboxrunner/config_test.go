// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigFailsClosedForMissingOrUnsafeRuntimeSettings(t *testing.T) {
	valid := validRunnerEnvironment()
	for name, mutate := range map[string]func(map[string]string){
		"disabled":                     func(values map[string]string) { values["SANDBOX_RUNNER_ENABLED"] = "false" },
		"missing deployment id":        func(values map[string]string) { delete(values, "SANDBOX_RUNNER_DEPLOYMENT_ID") },
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

func TestLoadConfigRequiresSchedulerConfigurationVerificationKeys(t *testing.T) {
	values := validRunnerEnvironment()
	values["SANDBOX_RUNNER_LISTEN_ADDR"] = "127.0.0.1:9443"
	values["SANDBOX_RUNNER_DEBUG_LOOPBACK_HTTP"] = "true"
	delete(values, "SANDBOX_RUNNER_TLS_CERT_FILE")
	delete(values, "SANDBOX_RUNNER_TLS_KEY_FILE")
	delete(values, "SANDBOX_RUNNER_SCHEDULER_CONFIG_VERIFY_KEYS_JSON")
	if _, err := LoadConfig(mapGetenv(values)); err == nil {
		t.Fatal("LoadConfig() accepted a Runner without scheduler configuration verification keys")
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

func TestLoadConfigLeavesSessionBackendDisabledWithoutSessionDependencies(t *testing.T) {
	for _, value := range []string{"", "false"} {
		t.Run("value-"+value, func(t *testing.T) {
			values := validDebugRunnerEnvironment()
			values["SANDBOX_RUNNER_SESSION_ENABLED"] = value
			config, err := LoadConfig(mapGetenv(values))
			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}
			if config.SessionBackendEnabled || config.AIOUpstreamURL != "" || config.AIOBearerToken != "" {
				t.Fatalf("session config = %#v, want disabled and empty", config)
			}
		})
	}
}

func TestLoadConfigRequiresCompleteSessionStartupDependencies(t *testing.T) {
	valid := validSessionRunnerEnvironment()
	for name, mutate := range map[string]func(map[string]string){
		"invalid enable boolean": func(values map[string]string) {
			values["SANDBOX_RUNNER_SESSION_ENABLED"] = "sometimes"
		},
		"missing MySQL DSN": func(values map[string]string) {
			delete(values, "MYSQL_DSN")
		},
		"malformed MySQL DSN": func(values map[string]string) {
			values["MYSQL_DSN"] = "://not-a-mysql-dsn"
		},
		"missing Redis address": func(values map[string]string) {
			delete(values, "REDIS_ADDR")
		},
		"malformed Redis address": func(values map[string]string) {
			values["REDIS_ADDR"] = "redis-without-port"
		},
		"missing AIO origin": func(values map[string]string) {
			delete(values, "SANDBOX_RUNNER_AIO_UPSTREAM_URL")
		},
		"invalid session signing keyring": func(values map[string]string) {
			delete(values, "SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON")
		},
		"invalid Provider credential": func(values map[string]string) {
			delete(values, "SANDBOX_RUNNER_AUTH_TOKEN")
		},
		"invalid optional bearer": func(values map[string]string) {
			values["SANDBOX_RUNNER_AIO_BEARER_TOKEN"] = "secret\r\ninjected: value"
		},
	} {
		t.Run(name, func(t *testing.T) {
			values := cloneRunnerEnvironment(valid)
			mutate(values)
			if _, err := LoadConfig(mapGetenv(values)); !errors.Is(err, ErrConfiguration) {
				t.Fatalf("LoadConfig() error = %v, want ErrConfiguration", err)
			}
		})
	}
}

func TestLoadConfigAcceptsSharedAIOPrivateOriginsOnPort8080(t *testing.T) {
	for _, origin := range []string{
		"http://coze-sandbox-aio:8080",
		"http://10.20.0.4:8080",
		"http://172.20.0.4:8080",
		"http://192.168.20.4:8080",
		"http://[fd00::5]:8080",
	} {
		t.Run(strings.ReplaceAll(origin, "/", "_"), func(t *testing.T) {
			values := validSessionRunnerEnvironment()
			values["SANDBOX_RUNNER_AIO_UPSTREAM_URL"] = origin
			config, err := LoadConfig(mapGetenv(values))
			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}
			if !config.SessionBackendEnabled || config.AIOUpstreamURL != origin || config.AIOBearerToken != "optional-aio-bearer" {
				t.Fatalf("session config = %#v", config)
			}
		})
	}
}

func TestLoadConfigRejectsUnsafeOrNonExactAIOOrigins(t *testing.T) {
	for name, origin := range map[string]string{
		"https":               "https://coze-sandbox-aio:8080",
		"internal port":       "http://coze-sandbox-aio:8090",
		"default port":        "http://coze-sandbox-aio",
		"multi label host":    "http://sandbox.internal.example:8080",
		"public IPv4":         "http://203.0.113.10:8080",
		"userinfo":            "http://user:secret@coze-sandbox-aio:8080",
		"path":                "http://coze-sandbox-aio:8080/v1",
		"trailing slash":      "http://coze-sandbox-aio:8080/",
		"query":               "http://coze-sandbox-aio:8080?token=secret",
		"fragment":            "http://coze-sandbox-aio:8080#internal",
		"numeric hostname":    "http://2130706433:8080",
		"unspecified IPv4":    "http://0.0.0.0:8080",
		"link local IPv4":     "http://169.254.1.1:8080",
		"multicast IPv4":      "http://224.0.0.1:8080",
		"embedded whitespace": "http://coze sandbox:8080",
	} {
		t.Run(name, func(t *testing.T) {
			values := validSessionRunnerEnvironment()
			values["SANDBOX_RUNNER_AIO_UPSTREAM_URL"] = origin
			if _, err := LoadConfig(mapGetenv(values)); !errors.Is(err, ErrConfiguration) {
				t.Fatalf("LoadConfig(%q) error = %v, want ErrConfiguration", origin, err)
			}
		})
	}
}

func TestLoadConfigAllowsAIOHTTPLoopbackOnlyInExplicitLocalDebug(t *testing.T) {
	for _, origin := range []string{"http://127.0.0.1:8080", "http://[::1]:8080", "http://localhost:8080"} {
		t.Run(origin, func(t *testing.T) {
			debugValues := validSessionRunnerEnvironment()
			debugValues["SANDBOX_RUNNER_AIO_UPSTREAM_URL"] = origin
			if _, err := LoadConfig(mapGetenv(debugValues)); err != nil {
				t.Fatalf("LoadConfig(debug %q) error = %v", origin, err)
			}

			sharedValues := validSecureSessionRunnerEnvironment(t)
			sharedValues["SANDBOX_RUNNER_AIO_UPSTREAM_URL"] = origin
			if _, err := LoadConfig(mapGetenv(sharedValues)); !errors.Is(err, ErrConfiguration) {
				t.Fatalf("LoadConfig(shared %q) error = %v, want ErrConfiguration", origin, err)
			}
		})
	}
}

func TestAIOHTTPClientDisablesEnvironmentProxyAndRejectsRedirect(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:65534")

	client := NewAIOHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil {
		t.Fatalf("AIO transport = %#v, want explicit proxy disable", client.Transport)
	}
	if client.Timeout != 30*time.Second {
		t.Fatalf("AIO timeout = %v, want 30s", client.Timeout)
	}
	if err := client.CheckRedirect(&http.Request{}, []*http.Request{{}}); !errors.Is(err, errAIOUpstreamRedirect) {
		t.Fatalf("redirect error = %v, want errAIOUpstreamRedirect", err)
	}
}

func TestSessionConfigDoesNotIntroduceInternalLifecycleOrGrantFields(t *testing.T) {
	typeOf := reflect.TypeOf(Config{})
	for _, forbidden := range []string{"Sessiond", "Grant", "AIOImage", "AIOContainer", "AIODocker", "Internal8090"} {
		for index := 0; index < typeOf.NumField(); index++ {
			if strings.Contains(typeOf.Field(index).Name, forbidden) {
				t.Fatalf("Config unexpectedly exposes forbidden Session field %q", typeOf.Field(index).Name)
			}
		}
	}
}

func validRunnerEnvironment() map[string]string {
	return map[string]string{
		"SANDBOX_RUNNER_ENABLED":                           "true",
		"SANDBOX_RUNNER_DEPLOYMENT_ID":                     "runner-dev-1",
		"SANDBOX_RUNNER_LISTEN_ADDR":                       ":9443",
		"SANDBOX_RUNNER_TLS_CERT_FILE":                     "/run/secrets/sandbox-runner.crt",
		"SANDBOX_RUNNER_TLS_KEY_FILE":                      "/run/secrets/sandbox-runner.key",
		"SANDBOX_RUNNER_AUTH_TOKEN":                        "runner-auth-token-0123456789",
		"SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON":          `{"active_key_id":"key-1","keys":{"key-1":"0123456789abcdef0123456789abcdef"}}`,
		"SANDBOX_RUNNER_SCHEDULER_CONFIG_VERIFY_KEYS_JSON": `{"active_key_id":"key-1","keys":{"key-1":"0123456789abcdef0123456789abcdef"}}`,
		"SANDBOX_RUNNER_QUEUE_KEYS_JSON":                   `{"active_key_id":"key-1","keys":{"key-1":"0123456789abcdef0123456789abcdef"}}`,
		"SANDBOX_RUNNER_ACTIVE_QUEUE_KEY_ID":               "key-1",
		"SANDBOX_RUNNER_ROOTLESS_ENDPOINT":                 "unix:///run/user/10001/podman/podman.sock",
		"SANDBOX_RUNNER_EXECUTION_IMAGE":                   "registry.example/coze-sandbox-runtime@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"SANDBOX_RUNNER_EMERGENCY_STOP":                    "false",
	}
}

func validDebugRunnerEnvironment() map[string]string {
	values := validRunnerEnvironment()
	values["SANDBOX_RUNNER_LISTEN_ADDR"] = "127.0.0.1:9443"
	values["SANDBOX_RUNNER_DEBUG_LOOPBACK_HTTP"] = "true"
	delete(values, "SANDBOX_RUNNER_TLS_CERT_FILE")
	delete(values, "SANDBOX_RUNNER_TLS_KEY_FILE")
	return values
}

func validSessionRunnerEnvironment() map[string]string {
	values := validDebugRunnerEnvironment()
	values["SANDBOX_RUNNER_SESSION_ENABLED"] = "true"
	values["SANDBOX_RUNNER_AIO_UPSTREAM_URL"] = "http://coze-sandbox-aio:8080"
	values["SANDBOX_RUNNER_AIO_BEARER_TOKEN"] = "optional-aio-bearer"
	values["MYSQL_DSN"] = "runner:password@tcp(dev-mysql:3306)/coze?parseTime=true"
	values["REDIS_ADDR"] = "dev-redis:6379"
	return values
}

func validSecureSessionRunnerEnvironment(t *testing.T) map[string]string {
	t.Helper()
	values := validSessionRunnerEnvironment()
	values["SANDBOX_RUNNER_DEBUG_LOOPBACK_HTTP"] = "false"
	values["SANDBOX_RUNNER_LISTEN_ADDR"] = ":9443"
	directory := t.TempDir()
	for env, name := range map[string]string{
		"SANDBOX_RUNNER_TLS_CERT_FILE": "runner.crt",
		"SANDBOX_RUNNER_TLS_KEY_FILE":  "runner.key",
	} {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte("test-secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		values[env] = path
	}
	return values
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
