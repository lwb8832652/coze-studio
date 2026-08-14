// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	mysqldriver "github.com/go-sql-driver/mysql"
	goredis "github.com/redis/go-redis/v9"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

const (
	aioCoreE2EEnableEnv        = "SANDBOX_AIO_CORE_E2E"
	aioCoreE2EIsolationAck     = "ISOLATED_TEST_DEPLOYMENT_WITH_NO_LIVE_TRAFFIC"
	aioCoreE2EExactCleanupAck  = "EXACT_PREFIX_ONLY_ON_EXISTING_DEV_DEPENDENCIES"
	aioCoreE2EDevDBFixturesAck = "EXACT_COMMIT_AND_CLEANUP_ON_ISOLATED_DEV_DATABASE"
	aioCoreE2EMySQLRollbackAck = "ROLLBACK_ONLY_ON_EXISTING_DEV_DATABASE"
)

var errAIOCoreE2EUnsafe = errors.New("AIO Core E2E configuration is unsafe")
var errAIOCoreE2ERuntime = errors.New("AIO Core E2E runtime verification failed")

const (
	aioCoreE2EDeleteSessionsSQL = `DELETE FROM sandbox_runtime_sessions
WHERE session_id = ? AND deployment_id = ? AND provider_id = ? AND space_id = ? AND user_id = ? AND thread_id = ? AND profile = ?
  AND state = 'destroyed' AND upstream_shell_id IS NULL AND recovery_reason = ''`
	aioCoreE2EDeleteProviderSQL = `DELETE FROM sandbox_providers
WHERE id = ? AND provider_key = ? AND created_by = ?`
	aioCoreE2EWorkspaceCleanupCommand = "find . -mindepth 1 -depth -delete"
	aioCoreE2EWorkspaceRoot           = "/mnt/user-data/workspace"
	aioCoreE2EMaxHTTPResponseBytes    = 64 << 20
	aioCoreE2EOperationTimeout        = 45 * time.Second
	aioCoreE2EControlTimeout          = 2 * time.Minute
)

type aioCoreE2EConfig struct {
	prefix        string
	deploymentID  string
	runnerURL     *url.URL
	runnerCAFile  string
	controlHelper string
	codeSHA       string
	imageID       string
	spaceID       int64
	userIDA       int64
	userIDB       int64
	authToken     string
	contextKeys   sandboxidentity.Keyring
	mysqlConfig   *mysqldriver.Config
	databaseName  string
	redisAddr     string
	redisPassword string
	redisDB       int
}

type aioCoreE2EFixtureScope struct {
	prefix       string
	deploymentID string
	providerID   int64
	providerKey  string
	spaceID      int64
	userIDs      map[int64]struct{}
	threadIDs    map[string]struct{}
	sessionIDs   map[string]struct{}
}

type aioCoreE2ESessionFixture struct {
	ref   domainsandbox.SessionRef
	runID string
}

type aioCoreE2ESessionBusinessRow struct {
	ref             domainsandbox.SessionRef
	state           domainsandbox.SessionState
	upstreamShellID sql.NullString
	recoveryReason  string
}

type aioCoreE2EHealthProjection struct {
	ProtocolVersion string                          `json:"protocol_version"`
	Status          domainsandbox.HealthStatus      `json:"status"`
	Capabilities    []domainsandbox.Scope           `json:"capabilities"`
	Features        []domainsandbox.ProviderFeature `json:"features"`
}

type aioCoreE2ERedisScan func(context.Context, uint64, string, int64) ([]string, uint64, error)

type aioCoreE2ERuntime struct {
	config        *aioCoreE2EConfig
	httpClient    *http.Client
	sqlDB         *sql.DB
	db            *gorm.DB
	redis         *goredis.Client
	repository    *infrasandbox.MySQLRepository
	scope         aioCoreE2EFixtureScope
	providerActor int64
	sessions      []*aioCoreE2ESessionFixture
	sequence      uint64
	aioStopped    bool
}

func (scope aioCoreE2EFixtureScope) ownsProvider(providerID int64, providerKey string) bool {
	return validAIOCoreE2EPrefix(scope.prefix) && scope.deploymentID == scope.prefix &&
		providerID > 0 && providerID == scope.providerID && providerKey == scope.providerKey &&
		strings.HasPrefix(providerKey, scope.prefix+"-")
}

func (scope aioCoreE2EFixtureScope) ownsSession(ref domainsandbox.SessionRef) bool {
	normalized, err := domainsandbox.NormalizeSessionRef(ref)
	if err != nil || normalized != ref || ref.RuntimeGeneration == 0 ||
		ref.Key.DeploymentID != scope.deploymentID || ref.Key.ProviderID != scope.providerID ||
		ref.Key.SpaceID != scope.spaceID || ref.Key.Profile != domainsandbox.SessionProfileCore {
		return false
	}
	if _, ok := scope.userIDs[ref.Key.UserID]; !ok {
		return false
	}
	if _, ok := scope.threadIDs[ref.Key.ThreadID]; !ok {
		return false
	}
	_, ok := scope.sessionIDs[ref.SessionID]
	return ok
}

func (scope aioCoreE2EFixtureScope) ownsSessionKey(key domainsandbox.SessionKey) bool {
	normalized, err := domainsandbox.NormalizeSessionKey(key)
	if err != nil || normalized != key || key.DeploymentID != scope.deploymentID || key.ProviderID != scope.providerID ||
		key.SpaceID != scope.spaceID || key.Profile != domainsandbox.SessionProfileCore {
		return false
	}
	if _, ok := scope.userIDs[key.UserID]; !ok {
		return false
	}
	_, ok := scope.threadIDs[key.ThreadID]
	return ok
}

func scanAIOCoreE2ERedisCursorToZero(ctx context.Context, prefix string, count int64, scan aioCoreE2ERedisScan, visit func([]string) error) error {
	if ctx == nil || prefix == "" || count <= 0 || scan == nil || visit == nil {
		return errAIOCoreE2ERuntime
	}
	var cursor uint64
	for {
		keys, next, err := scan(ctx, cursor, prefix+"*", count)
		if err != nil {
			return errAIOCoreE2ERuntime
		}
		for _, key := range keys {
			if !strings.HasPrefix(key, prefix) {
				return errAIOCoreE2ERuntime
			}
		}
		if err := visit(keys); err != nil {
			return err
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func cleanAIOCoreE2EWorkspacesBeforeDestroy(sessions []*aioCoreE2ESessionFixture, clean, destroy func(*aioCoreE2ESessionFixture) error) error {
	if len(sessions) == 0 || clean == nil || destroy == nil {
		return errAIOCoreE2ERuntime
	}
	failed := false
	for _, session := range sessions {
		if session == nil || clean(session) != nil {
			failed = true
		}
	}
	if failed {
		return errAIOCoreE2ERuntime
	}
	for _, session := range sessions {
		if destroy(session) != nil {
			failed = true
		}
	}
	if failed {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func validateAIOCoreE2EHealthProjection(projection aioCoreE2EHealthProjection, expectCore bool) error {
	wantCapabilities := []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopePlugin}
	wantFeatures := []domainsandbox.ProviderFeature{
		domainsandbox.ProviderFeatureQueueStatusV1,
		domainsandbox.ProviderFeatureSignedExecutionContext,
	}
	if expectCore {
		wantFeatures = append(wantFeatures,
			domainsandbox.ProviderFeatureSandboxSessionV1,
			domainsandbox.ProviderFeatureSignedSessionContextV2,
		)
	}
	if projection.ProtocolVersion != "v1" || projection.Status != domainsandbox.HealthStatusHealthy ||
		fmt.Sprint(projection.Capabilities) != fmt.Sprint(wantCapabilities) || fmt.Sprint(projection.Features) != fmt.Sprint(wantFeatures) {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func aioCoreE2ECleanupStatements() []string {
	return []string{aioCoreE2EDeleteSessionsSQL, aioCoreE2EDeleteProviderSQL}
}

func aioCoreE2EControlVerbs() []string {
	return []string{"stop-aio", "start-aio", "recreate-aio-preserve-volumes", "restart-runner"}
}

func (*aioCoreE2EConfig) String() string {
	return "sandboxrunner.aioCoreE2EConfig{secrets:<redacted>}"
}
func (*aioCoreE2EConfig) GoString() string {
	return "sandboxrunner.aioCoreE2EConfig{secrets:<redacted>}"
}
func (*aioCoreE2EConfig) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandboxrunner.aioCoreE2EConfig{secrets:<redacted>}")
}

func loadAIOCoreE2EConfig(getenv func(string) string) (*aioCoreE2EConfig, string, error) {
	if getenv == nil {
		return nil, "AIO Core E2E environment reader is unavailable", nil
	}
	enabled := getenv(aioCoreE2EEnableEnv)
	if enabled == "" {
		return nil, aioCoreE2EEnableEnv + " is not set", nil
	}
	if enabled != "1" {
		return nil, "", errAIOCoreE2EUnsafe
	}
	required := []string{
		"SANDBOX_AIO_CORE_E2E_PREFIX",
		"SANDBOX_RUNNER_DEPLOYMENT_ID",
		"SANDBOX_AIO_CORE_E2E_ISOLATED_DEPLOYMENT",
		"SANDBOX_AIO_CORE_E2E_EXACT_CLEANUP",
		"SANDBOX_AIO_CORE_E2E_DEV_DB_FIXTURES",
		"SANDBOX_AIO_CORE_E2E_RUNNER_URL",
		"SANDBOX_AIO_CORE_E2E_RUNNER_CA_FILE",
		"SANDBOX_AIO_CORE_E2E_CONTROL_HELPER",
		"SANDBOX_AIO_CORE_E2E_CODE_SHA",
		"SANDBOX_AIO_CORE_E2E_IMAGE_ID",
		"SANDBOX_AIO_CORE_E2E_SPACE_ID",
		"SANDBOX_AIO_CORE_E2E_USER_ID_A",
		"SANDBOX_AIO_CORE_E2E_USER_ID_B",
		"SANDBOX_RUNNER_AUTH_TOKEN",
		"SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON",
		"SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DSN",
		"SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DATABASE",
		"SANDBOX_RUNTIME_SESSION_DEV_MYSQL_ROLLBACK_ONLY",
		"MYSQL_DSN",
		"REDIS_ADDR",
		"REDIS_DB",
	}
	missing := false
	for _, name := range required {
		if getenv(name) == "" {
			missing = true
		}
	}
	if missing {
		return nil, "", errAIOCoreE2EUnsafe
	}
	prefix := getenv("SANDBOX_AIO_CORE_E2E_PREFIX")
	deploymentID := getenv("SANDBOX_RUNNER_DEPLOYMENT_ID")
	if prefix != deploymentID || !validAIOCoreE2EPrefix(prefix) ||
		getenv("SANDBOX_AIO_CORE_E2E_ISOLATED_DEPLOYMENT") != aioCoreE2EIsolationAck ||
		getenv("SANDBOX_AIO_CORE_E2E_EXACT_CLEANUP") != aioCoreE2EExactCleanupAck ||
		getenv("SANDBOX_AIO_CORE_E2E_DEV_DB_FIXTURES") != aioCoreE2EDevDBFixturesAck ||
		getenv("SANDBOX_RUNTIME_SESSION_DEV_MYSQL_ROLLBACK_ONLY") != aioCoreE2EMySQLRollbackAck {
		return nil, "", errAIOCoreE2EUnsafe
	}
	runnerURL, err := parseAIOCoreE2ERunnerURL(getenv("SANDBOX_AIO_CORE_E2E_RUNNER_URL"))
	if err != nil {
		return nil, "", errAIOCoreE2EUnsafe
	}
	runnerCAFile := getenv("SANDBOX_AIO_CORE_E2E_RUNNER_CA_FILE")
	controlHelper := getenv("SANDBOX_AIO_CORE_E2E_CONTROL_HELPER")
	if !validAIOCoreE2EAbsolutePath(runnerCAFile) || !validAIOCoreE2EAbsolutePath(controlHelper) ||
		!validLowerHex(getenv("SANDBOX_AIO_CORE_E2E_CODE_SHA"), 40) ||
		!strings.HasPrefix(getenv("SANDBOX_AIO_CORE_E2E_IMAGE_ID"), "sha256:") ||
		!validLowerHex(strings.TrimPrefix(getenv("SANDBOX_AIO_CORE_E2E_IMAGE_ID"), "sha256:"), 64) {
		return nil, "", errAIOCoreE2EUnsafe
	}
	spaceID, err := parseAIOCoreE2EPositiveID(getenv("SANDBOX_AIO_CORE_E2E_SPACE_ID"))
	if err != nil {
		return nil, "", errAIOCoreE2EUnsafe
	}
	userIDA, err := parseAIOCoreE2EPositiveID(getenv("SANDBOX_AIO_CORE_E2E_USER_ID_A"))
	if err != nil {
		return nil, "", errAIOCoreE2EUnsafe
	}
	userIDB, err := parseAIOCoreE2EPositiveID(getenv("SANDBOX_AIO_CORE_E2E_USER_ID_B"))
	if err != nil || userIDA == userIDB {
		return nil, "", errAIOCoreE2EUnsafe
	}
	authToken := getenv("SANDBOX_RUNNER_AUTH_TOKEN")
	contextKeys, err := loadIdentityKeyring(getenv("SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON"))
	if err != nil || len(authToken) < 16 || strings.ContainsAny(authToken, "\r\n") {
		return nil, "", errAIOCoreE2EUnsafe
	}
	databaseName := getenv("SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DATABASE")
	mysqlConfig, err := parseAIOCoreE2EMySQLConfig(getenv("SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DSN"), databaseName)
	if err != nil {
		return nil, "", errAIOCoreE2EUnsafe
	}
	runnerMySQL, err := parseAIOCoreE2EMySQLConfig(getenv("MYSQL_DSN"), databaseName)
	if err != nil || runnerMySQL.Net != mysqlConfig.Net || runnerMySQL.Addr != mysqlConfig.Addr || runnerMySQL.DBName != mysqlConfig.DBName {
		return nil, "", errAIOCoreE2EUnsafe
	}
	redisAddr := getenv("REDIS_ADDR")
	redisDB, err := parseAIOCoreE2ERedisDB(getenv("REDIS_DB"))
	if err != nil || !validRedisAddress(redisAddr) || strings.ContainsAny(getenv("REDIS_PASSWORD"), "\r\n") {
		return nil, "", errAIOCoreE2EUnsafe
	}
	return &aioCoreE2EConfig{
		prefix: prefix, deploymentID: deploymentID, runnerURL: runnerURL,
		runnerCAFile: runnerCAFile, controlHelper: controlHelper,
		codeSHA: getenv("SANDBOX_AIO_CORE_E2E_CODE_SHA"), imageID: getenv("SANDBOX_AIO_CORE_E2E_IMAGE_ID"),
		spaceID: spaceID, userIDA: userIDA, userIDB: userIDB,
		authToken: authToken, contextKeys: contextKeys,
		mysqlConfig: mysqlConfig, databaseName: databaseName,
		redisAddr: redisAddr, redisPassword: getenv("REDIS_PASSWORD"), redisDB: redisDB,
	}, "", nil
}

func validAIOCoreE2EPrefix(value string) bool {
	if len(value) < len("aio-e2e-")+8 || len(value) > 48 || !strings.HasPrefix(value, "aio-e2e-") || !validKeyID(value) {
		return false
	}
	for _, character := range value[len("aio-e2e-"):] {
		if character != '-' && (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func parseAIOCoreE2ERunnerURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" ||
		parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return nil, errAIOCoreE2EUnsafe
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil || host == "" || port == "" || strings.ContainsAny(host, " \t\r\n") {
		return nil, errAIOCoreE2EUnsafe
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, errAIOCoreE2EUnsafe
	}
	return parsed, nil
}

func validAIOCoreE2EAbsolutePath(value string) bool {
	return value != "" && filepath.IsAbs(value) && filepath.Clean(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}

func validLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func parseAIOCoreE2EPositiveID(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 || strconv.FormatInt(value, 10) != raw {
		return 0, errAIOCoreE2EUnsafe
	}
	return value, nil
}

func parseAIOCoreE2EMySQLConfig(raw, databaseName string) (*mysqldriver.Config, error) {
	if raw == "" || strings.TrimSpace(raw) != raw || databaseName == "" || strings.TrimSpace(databaseName) != databaseName {
		return nil, errAIOCoreE2EUnsafe
	}
	config, err := mysqldriver.ParseDSN(raw)
	if err != nil || config.Net != "tcp" || config.Addr == "" || config.DBName != databaseName || config.MultiStatements {
		return nil, errAIOCoreE2EUnsafe
	}
	config.ParseTime = true
	config.Loc = time.UTC
	return config, nil
}

func parseAIOCoreE2ERedisDB(raw string) (int, error) {
	if raw == "" {
		return 0, errAIOCoreE2EUnsafe
	}
	for _, character := range raw {
		if character < '0' || character > '9' {
			return 0, errAIOCoreE2EUnsafe
		}
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || strconv.Itoa(value) != raw {
		return 0, errAIOCoreE2EUnsafe
	}
	return value, nil
}

func TestAIOCoreE2EGateSkipsWithoutExplicitEnablement(t *testing.T) {
	config, skipReason, err := loadAIOCoreE2EConfig(func(string) string { return "" })
	if err != nil || config != nil || skipReason == "" {
		t.Fatalf("empty AIO Core E2E gate = config %v, skip %q, error %v; want a clean skip", config, skipReason, err)
	}
}

func TestAIOCoreE2EGateAcceptsOnlyCompleteIsolatedDevConfiguration(t *testing.T) {
	environment := validAIOCoreE2EEnvironment()
	config, skipReason, err := loadAIOCoreE2EConfig(mapAIOCoreE2EGetenv(environment))
	if err != nil || skipReason != "" || config == nil {
		t.Fatalf("valid AIO Core E2E gate = config %v, skip %q, error %v", config, skipReason, err)
	}
	if config.deploymentID != environment["SANDBOX_AIO_CORE_E2E_PREFIX"] ||
		config.spaceID <= 0 || config.userIDA <= 0 || config.userIDB <= 0 || config.userIDA == config.userIDB {
		t.Fatal("valid AIO Core E2E gate did not preserve the scoped non-secret identity contract")
	}
}

func TestAIOCoreE2EGateRejectsPartialOrUnsafeConfiguration(t *testing.T) {
	tests := map[string]func(map[string]string){
		"explicitly disabled": func(values map[string]string) { values[aioCoreE2EEnableEnv] = "0" },
		"partial enablement": func(values map[string]string) {
			for key := range values {
				if key != aioCoreE2EEnableEnv {
					delete(values, key)
				}
			}
		},
		"deployment mismatch":      func(values map[string]string) { values["SANDBOX_RUNNER_DEPLOYMENT_ID"] += "-other" },
		"not isolated":             func(values map[string]string) { values["SANDBOX_AIO_CORE_E2E_ISOLATED_DEPLOYMENT"] = "yes" },
		"cleanup not acknowledged": func(values map[string]string) { values["SANDBOX_AIO_CORE_E2E_EXACT_CLEANUP"] = "yes" },
		"committed fixtures not acknowledged": func(values map[string]string) {
			values["SANDBOX_AIO_CORE_E2E_DEV_DB_FIXTURES"] = "yes"
		},
		"committed fixture gate missing": func(values map[string]string) { delete(values, "SANDBOX_AIO_CORE_E2E_DEV_DB_FIXTURES") },
		"rollback gate missing":          func(values map[string]string) { delete(values, "SANDBOX_RUNTIME_SESSION_DEV_MYSQL_ROLLBACK_ONLY") },
		"database mismatch":              func(values map[string]string) { values["SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DATABASE"] = "other_dev" },
		"multi statements": func(values map[string]string) {
			values["SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DSN"] += "&multiStatements=true"
		},
		"runner database mismatch": func(values map[string]string) {
			values["MYSQL_DSN"] = strings.Replace(values["MYSQL_DSN"], "/coze_dev?", "/other_dev?", 1)
		},
		"insecure runner URL": func(values map[string]string) {
			values["SANDBOX_AIO_CORE_E2E_RUNNER_URL"] = "http://runner.e2e.invalid:9443"
		},
		"runner URL query": func(values map[string]string) { values["SANDBOX_AIO_CORE_E2E_RUNNER_URL"] += "?secret=forbidden" },
		"malformed prefix": func(values map[string]string) {
			values["SANDBOX_AIO_CORE_E2E_PREFIX"] = "shared-dev"
			values["SANDBOX_RUNNER_DEPLOYMENT_ID"] = "shared-dev"
		},
		"same users": func(values map[string]string) {
			values["SANDBOX_AIO_CORE_E2E_USER_ID_B"] = values["SANDBOX_AIO_CORE_E2E_USER_ID_A"]
		},
		"malformed redis db": func(values map[string]string) { values["REDIS_DB"] = "-1" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			environment := validAIOCoreE2EEnvironment()
			mutate(environment)
			config, skipReason, err := loadAIOCoreE2EConfig(mapAIOCoreE2EGetenv(environment))
			if config != nil || skipReason != "" || err == nil {
				t.Fatalf("unsafe AIO Core E2E config = config %v, skip %q, error %v; want hard error", config, skipReason, err)
			}
		})
	}
}

func TestAIOCoreE2EConfigFormattingRedactsAllSensitiveValues(t *testing.T) {
	environment := validAIOCoreE2EEnvironment()
	config, skipReason, err := loadAIOCoreE2EConfig(mapAIOCoreE2EGetenv(environment))
	if err != nil || skipReason != "" || config == nil {
		t.Fatal("load valid redaction fixture failed")
	}
	formatted := fmt.Sprintf("%v %#v", config, config)
	for _, secret := range []string{
		environment["SANDBOX_RUNNER_AUTH_TOKEN"],
		environment["SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON"],
		environment["SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DSN"],
		environment["MYSQL_DSN"],
		environment["REDIS_PASSWORD"],
		environment["SANDBOX_AIO_CORE_E2E_RUNNER_URL"],
		environment["SANDBOX_AIO_CORE_E2E_RUNNER_CA_FILE"],
		environment["SANDBOX_AIO_CORE_E2E_CONTROL_HELPER"],
	} {
		if secret != "" && strings.Contains(formatted, secret) {
			t.Fatal("AIO Core E2E config formatting leaked a sensitive value")
		}
	}
	if !strings.Contains(formatted, "secrets:<redacted>") {
		t.Fatal("AIO Core E2E config formatting omitted the redaction marker")
	}
}

func TestAIOCoreE2EFixtureScopeRejectsEveryForeignIdentityDimension(t *testing.T) {
	scope := aioCoreE2EFixtureScope{
		prefix: "aio-e2e-0123456789abcdef", deploymentID: "aio-e2e-0123456789abcdef",
		providerID: 501, providerKey: "aio-e2e-0123456789abcdef-provider", spaceID: 601,
		userIDs: map[int64]struct{}{701: {}, 702: {}},
		threadIDs: map[string]struct{}{
			"aio-e2e-0123456789abcdef-thread-a": {},
			"aio-e2e-0123456789abcdef-thread-b": {},
		},
		sessionIDs: map[string]struct{}{"550e8400-e29b-41d4-a716-446655440000": {}},
	}
	valid := domainsandbox.SessionRef{SessionID: "550e8400-e29b-41d4-a716-446655440000", RuntimeGeneration: 3,
		Key: domainsandbox.SessionKey{DeploymentID: scope.deploymentID, ProviderID: scope.providerID, SpaceID: scope.spaceID,
			UserID: 701, ThreadID: "aio-e2e-0123456789abcdef-thread-a", Profile: domainsandbox.SessionProfileCore}}
	if !scope.ownsSession(valid) {
		t.Fatal("valid E2E Session fixture was not recognized")
	}
	mutations := map[string]func(*domainsandbox.SessionRef){
		"session":    func(ref *domainsandbox.SessionRef) { ref.SessionID = "foreign-session" },
		"generation": func(ref *domainsandbox.SessionRef) { ref.RuntimeGeneration = 0 },
		"deployment": func(ref *domainsandbox.SessionRef) { ref.Key.DeploymentID += "-foreign" },
		"provider":   func(ref *domainsandbox.SessionRef) { ref.Key.ProviderID++ },
		"space":      func(ref *domainsandbox.SessionRef) { ref.Key.SpaceID++ },
		"user":       func(ref *domainsandbox.SessionRef) { ref.Key.UserID = 999 },
		"thread":     func(ref *domainsandbox.SessionRef) { ref.Key.ThreadID += "-foreign" },
		"profile":    func(ref *domainsandbox.SessionRef) { ref.Key.Profile = domainsandbox.SessionProfile("host") },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if scope.ownsSession(candidate) {
				t.Fatal("fixture scope accepted a foreign Session dimension")
			}
		})
	}
	if scope.ownsProvider(scope.providerID, scope.providerKey+"-foreign") || scope.ownsProvider(scope.providerID+1, scope.providerKey) {
		t.Fatal("fixture scope accepted a foreign provider identity")
	}
}

func TestAIOCoreE2ECleanupPlanIsExactAndContainsNoBroadDestructiveOperation(t *testing.T) {
	statements := aioCoreE2ECleanupStatements()
	if len(statements) != 2 {
		t.Fatalf("cleanup SQL count = %d, want two exact statements", len(statements))
	}
	for _, statement := range statements {
		canonical := strings.ToUpper(strings.Join(strings.Fields(statement), " "))
		for _, forbidden := range []string{"DROP ", "TRUNCATE ", "ALTER ", "CREATE ", "FLUSH", " LIKE ", " IN ", "DELETE FROM SANDBOX_RUNTIME_SESSIONS WHERE 1", "DELETE FROM SANDBOX_PROVIDERS WHERE 1"} {
			if strings.Contains(canonical, forbidden) {
				t.Fatal("cleanup plan contains a broad destructive operation")
			}
		}
		if !strings.Contains(canonical, " WHERE ") || strings.Count(statement, "?") < 2 {
			t.Fatal("cleanup plan lacks exact parameterized predicates")
		}
	}
	if !strings.Contains(statements[0], "session_id = ? AND deployment_id = ? AND provider_id = ? AND space_id = ? AND user_id = ? AND thread_id = ? AND profile = ?") ||
		!strings.Contains(statements[0], "state = 'destroyed' AND upstream_shell_id IS NULL AND recovery_reason = ''") ||
		!strings.Contains(statements[1], "id = ? AND provider_key = ? AND created_by = ?") {
		t.Fatal("cleanup plan does not bind each recorded session/provider primary fixture scope")
	}
	if aioCoreE2EWorkspaceCleanupCommand != "find . -mindepth 1 -depth -delete" ||
		strings.ContainsAny(aioCoreE2EWorkspaceCleanupCommand, "/\\") || strings.Contains(aioCoreE2EWorkspaceCleanupCommand, "sudo") {
		t.Fatal("workspace cleanup command is not fixed to the authenticated logical workspace")
	}
}

func TestAIOCoreE2EControlHelperAllowsOnlyFixedNonSensitiveVerbs(t *testing.T) {
	want := []string{"stop-aio", "start-aio", "recreate-aio-preserve-volumes", "restart-runner"}
	if got := aioCoreE2EControlVerbs(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatal("control helper verb allowlist drifted")
	}
	for _, forbidden := range []string{"dsn", "token", "identity", "path", "volume-clear", "database"} {
		for _, verb := range aioCoreE2EControlVerbs() {
			if strings.Contains(verb, forbidden) {
				t.Fatal("control helper verb carries a sensitive or destructive argument")
			}
		}
	}
}

func TestAIOCoreE2ESoakDurationAcceptsOnlyFrozenModes(t *testing.T) {
	for raw, want := range map[string]time.Duration{"2m": 2 * time.Minute, "30m": 30 * time.Minute} {
		got, err := parseAIOCoreE2ESoakDuration(raw)
		if err != nil || got != want {
			t.Fatalf("soak duration %q = %s, %v; want %s", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "1m", "2m0s", " 2m", "30m ", "1h"} {
		if got, err := parseAIOCoreE2ESoakDuration(raw); err == nil || got != 0 {
			t.Fatalf("unsafe soak duration %q = %s, %v; want rejection", raw, got, err)
		}
	}
}

func TestAIOCoreE2EIdleFixtureStatusRejectsSentinelCountDrift(t *testing.T) {
	status := SessionRuntimeStatusProjection{
		Schema: sessionRuntimeStatusSchemaV1, Available: true, AppliedConfigVersion: 1,
		RuntimeGeneration: 7, CoreEnabled: true, RawAIOReady: true, GenerationState: "ready",
		CoreMemoryReserveState: memoryReserveAvailable,
		TotalWeight:            coreSessionTotalWeight, ActiveSessions: 3, IdleShells: 3,
	}
	if !aioCoreE2EIdleFixtureStatus(status, 7, 3) {
		t.Fatal("exact idle fixture aggregate was rejected")
	}
	mutations := map[string]func(*SessionRuntimeStatusProjection){
		"extra active session": func(value *SessionRuntimeStatusProjection) { value.ActiveSessions++ },
		"idle session":         func(value *SessionRuntimeStatusProjection) { value.IdleSessions++ },
		"active shell":         func(value *SessionRuntimeStatusProjection) { value.ActiveShells++ },
		"extra idle shell":     func(value *SessionRuntimeStatusProjection) { value.IdleShells++ },
		"queued operation":     func(value *SessionRuntimeStatusProjection) { value.QueueDepth++ },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := status
			mutate(&candidate)
			if aioCoreE2EIdleFixtureStatus(candidate, 7, 3) {
				t.Fatal("idle fixture aggregate accepted a sentinel or workload count drift")
			}
		})
	}
}

func TestAIOCoreE2ERedisScanProofContinuesThroughEmptyNonzeroCursor(t *testing.T) {
	var cursors []uint64
	residual := errors.New("residual exact-prefix key")
	err := scanAIOCoreE2ERedisCursorToZero(context.Background(), "coze:sandbox:core:fixture:", 128,
		func(_ context.Context, cursor uint64, _ string, _ int64) ([]string, uint64, error) {
			cursors = append(cursors, cursor)
			switch cursor {
			case 0:
				return nil, 41, nil
			case 41:
				return []string{"coze:sandbox:core:fixture:late"}, 0, nil
			default:
				return nil, 0, errors.New("unexpected cursor")
			}
		}, func(keys []string) error {
			if len(keys) != 0 {
				return residual
			}
			return nil
		})
	if !errors.Is(err, residual) || fmt.Sprint(cursors) != "[0 41]" {
		t.Fatalf("cursor proof = error %v, cursors %v; want late residual detection after cursor 41", err, cursors)
	}
}

func TestAIOCoreE2EWorkspaceFailurePreventsEveryDestroy(t *testing.T) {
	first := &aioCoreE2ESessionFixture{runID: "first"}
	second := &aioCoreE2ESessionFixture{runID: "second"}
	var cleaned, destroyed []string
	err := cleanAIOCoreE2EWorkspacesBeforeDestroy([]*aioCoreE2ESessionFixture{first, second},
		func(session *aioCoreE2ESessionFixture) error {
			cleaned = append(cleaned, session.runID)
			if session == second {
				return errors.New("workspace unavailable")
			}
			return nil
		}, func(session *aioCoreE2ESessionFixture) error {
			destroyed = append(destroyed, session.runID)
			return nil
		})
	if err == nil || fmt.Sprint(cleaned) != "[first second]" || len(destroyed) != 0 {
		t.Fatalf("cleanup barrier = error %v, cleaned %v, destroyed %v; want all workspace attempts and zero destroy", err, cleaned, destroyed)
	}
}

func TestAIOCoreE2EOnlyDurablyDestroyedRowsAreDirectlyDeletable(t *testing.T) {
	safe := aioCoreE2ESessionBusinessRow{state: domainsandbox.SessionStateDestroyed}
	if !safeAIOCoreE2EDestroyedBusinessRow(safe) {
		t.Fatal("durably destroyed row was rejected")
	}
	mutations := map[string]func(*aioCoreE2ESessionBusinessRow){
		"active":     func(row *aioCoreE2ESessionBusinessRow) { row.state = domainsandbox.SessionStateActive },
		"released":   func(row *aioCoreE2ESessionBusinessRow) { row.state = domainsandbox.SessionStateReleased },
		"recovering": func(row *aioCoreE2ESessionBusinessRow) { row.state = domainsandbox.SessionStateRecovering },
		"shell retained": func(row *aioCoreE2ESessionBusinessRow) {
			row.upstreamShellID = sql.NullString{String: "shell", Valid: true}
		},
		"recovery reason": func(row *aioCoreE2ESessionBusinessRow) { row.recoveryReason = "retryable" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := safe
			mutate(&candidate)
			if safeAIOCoreE2EDestroyedBusinessRow(candidate) {
				t.Fatal("cleanup-only accepted a row that must be preserved for signed retry")
			}
		})
	}
}

func TestAIOCoreE2EQueueBoundFitsFrozen2C4GProfile(t *testing.T) {
	if maxSessionOperationQueueDepth != 64 || coreSessionTotalWeight != 2 {
		t.Fatalf("Core active operation bound/weight = %d/%d, want frozen 64/2", maxSessionOperationQueueDepth, coreSessionTotalWeight)
	}
	if queuedAfterTwoRunning := maxSessionOperationQueueDepth - coreSessionTotalWeight; queuedAfterTwoRunning != 62 {
		t.Fatalf("queued capacity after two running = %d, want 62", queuedAfterTwoRunning)
	}
}

func TestAIOCoreE2EHealthRequiresFrozenLegacyAndSessionContracts(t *testing.T) {
	ready := aioCoreE2EHealthProjection{
		ProtocolVersion: "v1", Status: domainsandbox.HealthStatusHealthy,
		Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopePlugin},
		Features: []domainsandbox.ProviderFeature{
			domainsandbox.ProviderFeatureQueueStatusV1,
			domainsandbox.ProviderFeatureSignedExecutionContext,
			domainsandbox.ProviderFeatureSandboxSessionV1,
			domainsandbox.ProviderFeatureSignedSessionContextV2,
		},
	}
	if validateAIOCoreE2EHealthProjection(ready, true) != nil {
		t.Fatal("exact ready health contract was rejected")
	}
	notReady := ready
	notReady.Features = append([]domainsandbox.ProviderFeature(nil), ready.Features[:2]...)
	if validateAIOCoreE2EHealthProjection(notReady, false) != nil {
		t.Fatal("exact not-ready legacy health contract was rejected")
	}
	mutations := map[string]func(*aioCoreE2EHealthProjection){
		"missing capability":    func(value *aioCoreE2EHealthProjection) { value.Capabilities = value.Capabilities[:1] },
		"missing queue feature": func(value *aioCoreE2EHealthProjection) { value.Features = value.Features[1:] },
		"one Session feature":   func(value *aioCoreE2EHealthProjection) { value.Features = value.Features[:3] },
		"unexpected legacy feature": func(value *aioCoreE2EHealthProjection) {
			value.Features = append(value.Features, domainsandbox.ProviderFeature("future"))
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := ready
			candidate.Capabilities = append([]domainsandbox.Scope(nil), ready.Capabilities...)
			candidate.Features = append([]domainsandbox.ProviderFeature(nil), ready.Features...)
			mutate(&candidate)
			if validateAIOCoreE2EHealthProjection(candidate, true) == nil {
				t.Fatal("health validator accepted a frozen contract drift")
			}
		})
	}
}

func TestAIOCoreE2ESessionBusinessIdentityIsRegisteredBeforeAcquire(t *testing.T) {
	const prefix = "aio-e2e-0123456789abcdef"
	runtime := &aioCoreE2ERuntime{
		config: &aioCoreE2EConfig{prefix: prefix, deploymentID: prefix, spaceID: 601},
		scope: aioCoreE2EFixtureScope{
			prefix: prefix, deploymentID: prefix, providerID: 501, providerKey: prefix + "-provider", spaceID: 601,
			userIDs: map[int64]struct{}{701: {}}, threadIDs: map[string]struct{}{prefix + "-thread-a": {}},
			sessionIDs: make(map[string]struct{}),
		},
	}
	fixture, err := runtime.registerSessionBusinessIdentity(701, prefix+"-thread-a", prefix+"-run-1")
	if err != nil || fixture == nil || len(runtime.sessions) != 1 || runtime.sessions[0] != fixture ||
		fixture.ref.SessionID != "" || fixture.ref.RuntimeGeneration != 0 || fixture.runID != prefix+"-run-1" ||
		fixture.ref.Key != (domainsandbox.SessionKey{DeploymentID: prefix, ProviderID: 501, SpaceID: 601,
			UserID: 701, ThreadID: prefix + "-thread-a", Profile: domainsandbox.SessionProfileCore}) {
		t.Fatalf("pre-registered Session identity = fixture %#v, sessions %d, error %v", fixture, len(runtime.sessions), err)
	}
}

func TestAIOCoreE2EUncertainAcquireReadbackUsesCompleteBusinessKey(t *testing.T) {
	const prefix = "aio-e2e-0123456789abcdef"
	const threadID = prefix + "-thread-a"
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runtime := &aioCoreE2ERuntime{
		config: &aioCoreE2EConfig{prefix: prefix, deploymentID: prefix, spaceID: 601}, sqlDB: db,
		scope: aioCoreE2EFixtureScope{
			prefix: prefix, deploymentID: prefix, providerID: 501, providerKey: prefix + "-provider", spaceID: 601,
			userIDs: map[int64]struct{}{701: {}}, threadIDs: map[string]struct{}{threadID: {}}, sessionIDs: make(map[string]struct{}),
		},
	}
	fixture, err := runtime.registerSessionBusinessIdentity(701, threadID, prefix+"-run-1")
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "550e8400-e29b-41d4-a716-446655440000"
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT session_id, runtime_generation, state, upstream_shell_id, recovery_reason
FROM sandbox_runtime_sessions
WHERE deployment_id = ? AND provider_id = ? AND space_id = ? AND user_id = ? AND thread_id = ? AND profile = ?`)).
		WithArgs(prefix, int64(501), int64(601), int64(701), threadID, domainsandbox.SessionProfileCore).
		WillReturnRows(sqlmock.NewRows([]string{"session_id", "runtime_generation", "state", "upstream_shell_id", "recovery_reason"}).
			AddRow(sessionID, uint64(3), domainsandbox.SessionStateActive, "business-shell", ""))
	found, err := runtime.reconcileSessionBusinessIdentity(context.Background(), fixture)
	if err != nil || !found || fixture.ref.SessionID != sessionID || fixture.ref.RuntimeGeneration != 3 || !runtime.scope.ownsSession(fixture.ref) {
		t.Fatalf("uncertain Acquire readback = found %t, fixture %#v, error %v", found, fixture, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAIOCoreE2EUncertainProviderReadbackUsesKeyAndActor(t *testing.T) {
	const prefix = "aio-e2e-0123456789abcdef"
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runtime := &aioCoreE2ERuntime{
		config: &aioCoreE2EConfig{prefix: prefix}, sqlDB: db, providerActor: 701,
		scope: aioCoreE2EFixtureScope{
			prefix: prefix, deploymentID: prefix, providerKey: prefix + "-provider", spaceID: 601,
			userIDs: map[int64]struct{}{}, threadIDs: map[string]struct{}{}, sessionIDs: map[string]struct{}{},
		},
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, provider_key, created_by
FROM sandbox_providers WHERE provider_key = ? AND created_by = ?`)).
		WithArgs(prefix+"-provider", int64(701)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "provider_key", "created_by"}).AddRow(int64(501), prefix+"-provider", int64(701)))
	found, err := runtime.reconcileProviderBusinessIdentity(context.Background())
	if err != nil || !found || !runtime.scope.ownsProvider(501, prefix+"-provider") {
		t.Fatalf("uncertain Provider readback = found %t, provider %d, error %v", found, runtime.scope.providerID, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func parseAIOCoreE2ESoakDuration(raw string) (time.Duration, error) {
	switch raw {
	case "2m":
		return 2 * time.Minute, nil
	case "30m":
		return 30 * time.Minute, nil
	default:
		return 0, errAIOCoreE2EUnsafe
	}
}

func TestAIOCoreE2E(t *testing.T) {
	config, skipReason, err := loadAIOCoreE2EConfig(os.Getenv)
	if skipReason != "" {
		t.Skip(skipReason)
	}
	if err != nil || config == nil {
		t.Fatal("AIO Core E2E configuration failed its safety gate")
	}
	if err := runAIOCoreE2E(t, config); err != nil {
		t.Fatal("AIO Core E2E failed; sensitive runtime details are intentionally redacted")
	}
}

func TestAIOCoreE2ESoakWorkload(t *testing.T) {
	config, skipReason, err := loadAIOCoreE2EConfig(os.Getenv)
	if skipReason != "" {
		t.Skip(skipReason)
	}
	if err != nil || config == nil {
		t.Fatal("AIO Core E2E soak configuration failed its safety gate")
	}
	duration, err := parseAIOCoreE2ESoakDuration(os.Getenv("SANDBOX_AIO_CORE_E2E_SOAK_DURATION"))
	if err != nil {
		t.Fatal("AIO Core E2E soak duration failed its safety gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration+2*time.Minute)
	defer cancel()
	runtime, err := newAIOCoreE2ERuntime(ctx, config)
	if err != nil {
		t.Fatal("AIO Core E2E soak dependency preflight failed; sensitive details are intentionally redacted")
	}
	runtime.registerFixtureCleanup(t)
	status, err := runtime.runtimeStatus(ctx)
	if err != nil || !aioCoreE2EReadyStatus(status) {
		t.Fatal("AIO Core E2E soak target is not ready")
	}
	configuration, err := runtime.sessionConfiguration(ctx)
	if err != nil || configuration.Version != status.AppliedConfigVersion || !configuration.Settings.CoreEnabled ||
		configuration.Settings.InteractiveEnabled || configuration.Settings.HostShellEnabled || configuration.Settings.CoreWeight != 1 {
		t.Fatal("AIO Core E2E soak target settings are not the frozen Core-only profile")
	}
	generation, err := runtime.databaseGeneration(ctx)
	if err != nil || generation.Generation != status.RuntimeGeneration {
		t.Fatal("AIO Core E2E soak generation preflight failed")
	}
	if err := runtime.createProvider(ctx); err != nil {
		t.Fatal("AIO Core E2E soak provider fixture creation failed")
	}
	sessionA, err := runtime.acquireSession(ctx, config.userIDA, config.prefix+"-thread-a")
	if err != nil {
		t.Fatal("AIO Core E2E soak Session fixture creation failed")
	}
	sessionB, err := runtime.acquireSession(ctx, config.userIDB, config.prefix+"-thread-c")
	if err != nil {
		t.Fatal("AIO Core E2E soak Session fixture creation failed")
	}
	if runtime.verifySoakParallelStart(ctx, sessionA, sessionB) != nil {
		t.Fatal("AIO Core E2E soak parallel admission verification failed")
	}
	if runtime.runSoakWorkloads(ctx, sessionA, sessionB, duration) != nil {
		t.Fatal("AIO Core E2E soak mixed workload failed")
	}
}

func TestAIOCoreE2ESoakSnapshot(t *testing.T) {
	config, skipReason, err := loadAIOCoreE2EConfig(os.Getenv)
	if skipReason != "" {
		t.Skip(skipReason)
	}
	if err != nil || config == nil {
		t.Fatal("AIO Core E2E snapshot configuration failed its safety gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runtime, err := newAIOCoreE2ERuntime(ctx, config)
	if err != nil {
		t.Fatal("AIO Core E2E snapshot dependency preflight failed; sensitive details are intentionally redacted")
	}
	t.Cleanup(func() {
		if runtime.closeDependencies() != nil {
			t.Error("AIO Core E2E snapshot dependency close failed")
		}
	})
	status, err := runtime.runtimeStatus(ctx)
	if err != nil || !aioCoreE2EReadyStatus(status) {
		t.Fatal("AIO Core E2E snapshot status failed validation")
	}
	generation, err := runtime.databaseGeneration(ctx)
	if err != nil || generation.Generation != status.RuntimeGeneration {
		t.Fatal("AIO Core E2E snapshot generation failed validation")
	}
	fmt.Printf("AIO_CORE_E2E_SNAPSHOT queue=%d running=%d weight=%d sessions=%d shells=%d generation=%d\n",
		status.QueueDepth, status.Running, status.UsedWeight, status.ActiveSessions+status.IdleSessions,
		status.ActiveShells+status.IdleShells, status.RuntimeGeneration)
}

// TestAIOCoreE2ECleanupOnly is the fixed post-snapshot cleanup entrypoint. It
// deliberately constructs no HTTP client and sends no signed request; only the
// pre-registered exact provider/business identities may be read or removed.
func TestAIOCoreE2ECleanupOnly(t *testing.T) {
	config, skipReason, err := loadAIOCoreE2EConfig(os.Getenv)
	if skipReason != "" {
		t.Skip(skipReason)
	}
	if err != nil || config == nil {
		t.Fatal("AIO Core E2E cleanup-only configuration failed its safety gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*aioCoreE2EControlTimeout)
	defer cancel()
	runtime, err := newAIOCoreE2ECleanupRuntime(ctx, config)
	if err != nil {
		t.Fatal("AIO Core E2E cleanup-only dependency preflight failed; sensitive details are intentionally redacted")
	}
	if runtime.cleanupOnly(ctx) != nil {
		t.Fatal("AIO Core E2E cleanup-only exact cleanup failed; a retryable runtime row may have been preserved")
	}
}

func (runtime *aioCoreE2ERuntime) verifySoakParallelStart(ctx context.Context, firstSession, secondSession *aioCoreE2ESessionFixture) error {
	first, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
		Command: "sleep 2; printf a", CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
	}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	second, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
		Command: "sleep 2; printf b", CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
	}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	type operationResult struct {
		projection SessionOperationProjection
		err        error
	}
	start := func(session *aioCoreE2ESessionFixture, operation aioCoreE2EPreparedOperation) <-chan operationResult {
		done := make(chan operationResult, 1)
		go func() {
			projection, submitErr := runtime.submitPreparedOperation(ctx, session, operation)
			done <- operationResult{projection: projection, err: submitErr}
		}()
		return done
	}
	firstDone := start(firstSession, first)
	secondDone := start(secondSession, second)
	if runtime.waitForOperationState(ctx, firstSession, first, SessionOperationRunning, 15*time.Second) != nil ||
		runtime.waitForOperationState(ctx, secondSession, second, SessionOperationRunning, 15*time.Second) != nil {
		return errAIOCoreE2ERuntime
	}
	status, err := runtime.runtimeStatus(ctx)
	if err != nil || status.Running != 2 || status.UsedWeight != status.TotalWeight {
		return errAIOCoreE2ERuntime
	}
	for _, done := range []<-chan operationResult{firstDone, secondDone} {
		select {
		case result := <-done:
			if result.err != nil || result.projection.State != SessionOperationSucceeded {
				return errAIOCoreE2ERuntime
			}
		case <-time.After(10 * time.Second):
			return errAIOCoreE2ERuntime
		}
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) runSoakWorkloads(ctx context.Context, execSession, fileSession *aioCoreE2ESessionFixture, duration time.Duration) error {
	if runtime == nil || ctx == nil || duration <= 0 {
		return errAIOCoreE2ERuntime
	}
	endsAt := time.Now().Add(duration)
	counts := make(chan int, 2)
	errors := make(chan error, 2)
	go func() {
		path := aioCoreE2EWorkspaceRoot + "/contract.txt"
		if runtime.writeFile(ctx, execSession, path, []byte("user-b"), false) != nil {
			errors <- errAIOCoreE2ERuntime
			return
		}
		var shellPID []byte
		iterations := 0
		for time.Now().Before(endsAt) {
			terminal, _, err := runtime.execute(ctx, execSession, "printf '%s' \"$$\"", 1024)
			if err != nil || terminal.ExitCode != 0 || len(terminal.Stdout) == 0 || len(terminal.Stderr) != 0 ||
				len(shellPID) != 0 && !bytes.Equal(shellPID, terminal.Stdout) {
				errors <- errAIOCoreE2ERuntime
				return
			}
			shellPID = append(shellPID[:0], terminal.Stdout...)
			iterations++
			if iterations%8 == 0 && runtime.verifyCancellation(ctx, execSession) != nil {
				errors <- errAIOCoreE2ERuntime
				return
			}
			if iterations%16 == 0 {
				if runtime.verifyReleaseAndReacquire(ctx, execSession) != nil {
					errors <- errAIOCoreE2ERuntime
					return
				}
				shellPID = nil
			}
			select {
			case <-ctx.Done():
				errors <- errAIOCoreE2ERuntime
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
		counts <- iterations
	}()
	go func() {
		path := aioCoreE2EWorkspaceRoot + "/soak-marker.txt"
		large := make([]byte, 1<<20)
		copy(large, []byte("needle\n"))
		for index := len("needle\n"); index < len(large); index++ {
			large[index] = 'x'
		}
		small := []byte("needle\nok\n")
		iterations := 0
		for time.Now().Before(endsAt) {
			content := small
			if iterations%8 == 0 {
				content = large
			}
			if len(content) == len(large) {
				middle := len(content) / 2
				if runtime.writeFile(ctx, fileSession, path, content[:middle], false) != nil ||
					runtime.writeFile(ctx, fileSession, path, content[middle:], true) != nil {
					errors <- errAIOCoreE2ERuntime
					return
				}
			} else if runtime.writeFile(ctx, fileSession, path, content, false) != nil {
				errors <- errAIOCoreE2ERuntime
				return
			}
			read, err := runtime.readFile(ctx, fileSession, path, 2<<20)
			if err != nil || !bytes.Equal(read, content) {
				errors <- errAIOCoreE2ERuntime
				return
			}
			if runtime.verifySoakFileQueries(ctx, fileSession, path) != nil {
				errors <- errAIOCoreE2ERuntime
				return
			}
			iterations++
			select {
			case <-ctx.Done():
				errors <- errAIOCoreE2ERuntime
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
		counts <- iterations
	}()
	total := 0
	failed := false
	for completed := 0; completed < 2; {
		select {
		case <-errors:
			failed = true
			completed++
		case count := <-counts:
			if count <= 0 {
				failed = true
			}
			total += count
			completed++
		}
	}
	if failed || total < 2 {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) verifySoakFileQueries(ctx context.Context, session *aioCoreE2ESessionFixture, expectedPath string) error {
	glob, err := runtime.prepareOperation(SessionOperationGlob, SessionOperationPayload{
		Path: aioCoreE2EWorkspaceRoot, Pattern: "soak-marker.txt", Limit: 20,
	}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	globProjection, err := runtime.submitPreparedOperation(ctx, session, glob)
	if err != nil || globProjection.State != SessionOperationSucceeded {
		return errAIOCoreE2ERuntime
	}
	var entries []infrasandbox.FileEntry
	if decodeStrictJSON(globProjection.Result, &entries) != nil || len(entries) != 1 || entries[0].Path != expectedPath {
		return errAIOCoreE2ERuntime
	}
	grep, err := runtime.prepareOperation(SessionOperationGrep, SessionOperationPayload{
		Path: expectedPath, Pattern: "needle", Limit: 20,
	}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	grepProjection, err := runtime.submitPreparedOperation(ctx, session, grep)
	if err != nil || grepProjection.State != SessionOperationSucceeded {
		return errAIOCoreE2ERuntime
	}
	var matches []infrasandbox.GrepMatch
	if decodeStrictJSON(grepProjection.Result, &matches) != nil || len(matches) != 1 || matches[0].Path != expectedPath || !strings.Contains(matches[0].Text, "needle") {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func runAIOCoreE2E(t *testing.T, config *aioCoreE2EConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	runtime, err := newAIOCoreE2ERuntime(ctx, config)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	runtime.registerFixtureCleanup(t)
	initialStatus, err := runtime.runtimeStatus(ctx)
	if err != nil || !aioCoreE2EReadyStatus(initialStatus) {
		return errAIOCoreE2ERuntime
	}
	if advertised, err := runtime.healthAdvertisesCore(ctx); err != nil || !advertised {
		return errAIOCoreE2ERuntime
	}
	configuration, err := runtime.sessionConfiguration(ctx)
	if err != nil || configuration.Version != initialStatus.AppliedConfigVersion || !configuration.Settings.CoreEnabled ||
		configuration.Settings.InteractiveEnabled || configuration.Settings.HostShellEnabled || configuration.Settings.CoreWeight != 1 ||
		configuration.Settings.PerUserActiveLimit != 1 {
		return errAIOCoreE2ERuntime
	}
	generation, err := runtime.databaseGeneration(ctx)
	if err != nil || generation.DeploymentID != config.deploymentID || generation.Generation != initialStatus.RuntimeGeneration || generation.SentinelID == "" {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.createProvider(ctx); err != nil {
		return errAIOCoreE2ERuntime
	}
	threadA := config.prefix + "-thread-a"
	threadB := config.prefix + "-thread-b"
	threadC := config.prefix + "-thread-c"
	sessionA, err := runtime.acquireSession(ctx, config.userIDA, threadA)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	sessionB, err := runtime.acquireSession(ctx, config.userIDA, threadB)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	sessionC, err := runtime.acquireSession(ctx, config.userIDB, threadC)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	if sessionA.ref.RuntimeGeneration != initialStatus.RuntimeGeneration ||
		sessionB.ref.RuntimeGeneration != initialStatus.RuntimeGeneration ||
		sessionC.ref.RuntimeGeneration != initialStatus.RuntimeGeneration {
		return errAIOCoreE2ERuntime
	}
	if _, err := runtime.waitRuntimeStatus(ctx, 15*time.Second, func(status SessionRuntimeStatusProjection) bool {
		return aioCoreE2EIdleFixtureStatus(status, initialStatus.RuntimeGeneration, len(runtime.sessions))
	}); err != nil {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.verifyWorkspaceContracts(ctx, sessionA, sessionB, sessionC); err != nil {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.verifyConcurrentScheduling(ctx, sessionA, sessionB, sessionC); err != nil {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.verifyQueuedCancellation(ctx, sessionA, sessionB, sessionC); err != nil {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.verifyCancellation(ctx, sessionA); err != nil {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.verifyFileHTTPContextCancellation(ctx, sessionA); err != nil {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.verifyTimeoutAndOutputBounds(ctx, sessionA); err != nil {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.verifyResultRecoveryAndNoReplay(ctx, sessionA); err != nil {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.verifyReleaseAndReacquire(ctx, sessionC); err != nil {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.verifyAIOGenerations(ctx, initialStatus.RuntimeGeneration, sessionA); err != nil {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.verifyRunnerRestartNoReplay(ctx, sessionA, sessionB); err != nil {
		return errAIOCoreE2ERuntime
	}
	// Phase 1 intentionally has no safe public observation for multi-Runner
	// generation-CAS injection, cross-tenant hardlinks, sudo flags, removal of
	// the empty physical thread roots retained by Destroy, or re-observing the
	// workspace after Destroy. This real test neither mutates global settings
	// nor claims those unsupported properties.
	return nil
}

func aioCoreE2EReadyStatus(status SessionRuntimeStatusProjection) bool {
	return validSessionRuntimeStatusProjection(status) && status.Available && status.CoreEnabled && !status.InteractiveEnabled &&
		!status.HostShellEnabled && !status.HostShellAvailable && status.RawAIOReady &&
		status.GenerationState == "ready" && status.RuntimeGeneration > 0 && status.TotalWeight == coreSessionTotalWeight &&
		status.CoreMemoryReserveState == memoryReserveAvailable
}

func aioCoreE2EIdleFixtureStatus(status SessionRuntimeStatusProjection, generation uint64, fixtureCount int) bool {
	return generation > 0 && fixtureCount > 0 && aioCoreE2EReadyStatus(status) && status.RuntimeGeneration == generation &&
		status.QueueDepth == 0 && status.Running == 0 && status.UsedWeight == 0 &&
		status.ActiveSessions == fixtureCount && status.IdleSessions == 0 &&
		status.ActiveShells == 0 && status.IdleShells == fixtureCount
}

func (runtime *aioCoreE2ERuntime) sessionConfiguration(ctx context.Context) (SessionConfigurationProjection, error) {
	var projection SessionConfigurationProjection
	claims := sandboxidentity.SessionRequest{DeploymentID: runtime.config.deploymentID}
	if err := runtime.doSignedRequest(ctx, http.MethodGet, "/v1/session-configuration", nil, claims, http.StatusOK, &projection); err != nil {
		return SessionConfigurationProjection{}, errAIOCoreE2ERuntime
	}
	settings, err := domainsandbox.NormalizeSessionRuntimeSettings(projection.Settings)
	if err != nil || projection.Schema != sessionConfigurationSchemaV1 || projection.Version == 0 || settings != projection.Settings || projection.Version != projection.Settings.Version {
		return SessionConfigurationProjection{}, errAIOCoreE2ERuntime
	}
	return projection, nil
}

func (runtime *aioCoreE2ERuntime) databaseGeneration(ctx context.Context) (domainsandbox.AIOGenerationState, error) {
	if runtime == nil || runtime.repository == nil {
		return domainsandbox.AIOGenerationState{}, errAIOCoreE2ERuntime
	}
	state, err := runtime.repository.GetAIOGeneration(ctx, runtime.config.deploymentID)
	if err != nil || state.DeploymentID != runtime.config.deploymentID || state.Generation == 0 || state.SentinelID == "" {
		return domainsandbox.AIOGenerationState{}, errAIOCoreE2ERuntime
	}
	return state, nil
}

func (runtime *aioCoreE2ERuntime) healthAdvertisesCore(ctx context.Context) (bool, error) {
	if runtime == nil || runtime.httpClient == nil || ctx == nil {
		return false, errAIOCoreE2ERuntime
	}
	target := *runtime.config.runnerURL
	target.Path, target.RawPath, target.RawQuery, target.Fragment = "/v1/health", "", "", ""
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return false, errAIOCoreE2ERuntime
	}
	response, err := runtime.httpClient.Do(request)
	if err != nil || response == nil {
		return false, errAIOCoreE2ERuntime
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode != http.StatusOK || len(body) == 0 {
		return false, errAIOCoreE2ERuntime
	}
	var projection aioCoreE2EHealthProjection
	if decodeStrictJSON(body, &projection) != nil {
		return false, errAIOCoreE2ERuntime
	}
	if validateAIOCoreE2EHealthProjection(projection, true) == nil {
		return true, nil
	}
	if validateAIOCoreE2EHealthProjection(projection, false) == nil {
		return false, nil
	}
	return false, errAIOCoreE2ERuntime
}

func (runtime *aioCoreE2ERuntime) execute(ctx context.Context, session *aioCoreE2ESessionFixture, command string, maximum int64) (executionTerminalResult, aioCoreE2EPreparedOperation, error) {
	return runtime.executeAt(ctx, session, command, aioCoreE2EWorkspaceRoot, maximum)
}

func (runtime *aioCoreE2ERuntime) executeAt(ctx context.Context, session *aioCoreE2ESessionFixture, command, cwd string, maximum int64) (executionTerminalResult, aioCoreE2EPreparedOperation, error) {
	operation, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
		Command: command, CWD: cwd, MaxOutputBytes: maximum,
	}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
	if err != nil {
		return executionTerminalResult{}, aioCoreE2EPreparedOperation{}, errAIOCoreE2ERuntime
	}
	projection, err := runtime.submitPreparedOperation(ctx, session, operation)
	if err != nil || projection.State != SessionOperationSucceeded || len(projection.Result) == 0 {
		return executionTerminalResult{}, operation, errAIOCoreE2ERuntime
	}
	var terminal executionTerminalResult
	if decodeStrictJSON(projection.Result, &terminal) != nil || int64(len(terminal.Stdout)+len(terminal.Stderr)) > maximum {
		return executionTerminalResult{}, operation, errAIOCoreE2ERuntime
	}
	return terminal, operation, nil
}

func (runtime *aioCoreE2ERuntime) writeFile(ctx context.Context, session *aioCoreE2ESessionFixture, path string, content []byte, appendFile bool) error {
	kind := SessionOperationWrite
	if appendFile {
		kind = SessionOperationAppend
	}
	operation, err := runtime.prepareOperation(kind, SessionOperationPayload{Path: path, Content: append([]byte(nil), content...)}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	projection, err := runtime.submitPreparedOperation(ctx, session, operation)
	if err != nil || projection.State != SessionOperationSucceeded || len(projection.Result) == 0 {
		return errAIOCoreE2ERuntime
	}
	var result map[string]bool
	if decodeStrictJSON(projection.Result, &result) != nil || len(result) != 1 || !result["written"] {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) readFile(ctx context.Context, session *aioCoreE2ESessionFixture, path string, maximum int64) ([]byte, error) {
	operation, err := runtime.prepareOperation(SessionOperationRead, SessionOperationPayload{Path: path, MaxBytes: maximum}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
	if err != nil {
		return nil, errAIOCoreE2ERuntime
	}
	projection, err := runtime.submitPreparedOperation(ctx, session, operation)
	if err != nil || projection.State != SessionOperationSucceeded || len(projection.Result) == 0 {
		return nil, errAIOCoreE2ERuntime
	}
	var content infrasandbox.FileContent
	if decodeStrictJSON(projection.Result, &content) != nil || int64(len(content.Data)) > maximum {
		return nil, errAIOCoreE2ERuntime
	}
	return append([]byte(nil), content.Data...), nil
}

func (runtime *aioCoreE2ERuntime) verifyWorkspaceContracts(ctx context.Context, sessionA, sessionB, sessionC *aioCoreE2ESessionFixture) error {
	expectedWorkspace := "/mnt/user-data/" + strconv.FormatInt(sessionA.ref.Key.SpaceID, 10) + "/" +
		strconv.FormatInt(sessionA.ref.Key.UserID, 10) + "/" + sessionA.ref.Key.ThreadID + "/workspace"
	pwd, _, err := runtime.execute(ctx, sessionA, "pwd", 2048)
	if err != nil || pwd.ExitCode != 0 || !bytes.Equal(pwd.Stdout, []byte(expectedWorkspace+"\n")) || len(pwd.Stderr) != 0 {
		return errAIOCoreE2ERuntime
	}
	if made, _, err := runtime.execute(ctx, sessionA, "mkdir -p subdir", 1024); err != nil || made.ExitCode != 0 {
		return errAIOCoreE2ERuntime
	}
	subdirPWD, _, err := runtime.executeAt(ctx, sessionA, "pwd", aioCoreE2EWorkspaceRoot+"/subdir", 2048)
	if err != nil || subdirPWD.ExitCode != 0 || !bytes.Equal(subdirPWD.Stdout, []byte(expectedWorkspace+"/subdir\n")) || len(subdirPWD.Stderr) != 0 {
		return errAIOCoreE2ERuntime
	}
	firstPID, _, err := runtime.execute(ctx, sessionA, "printf '%s' \"$$\"", 1024)
	if err != nil || firstPID.ExitCode != 0 || len(firstPID.Stdout) == 0 || len(firstPID.Stderr) != 0 {
		return errAIOCoreE2ERuntime
	}
	secondPID, _, err := runtime.execute(ctx, sessionA, "printf '%s' \"$$\"", 1024)
	if err != nil || secondPID.ExitCode != 0 || !bytes.Equal(firstPID.Stdout, secondPID.Stdout) || len(secondPID.Stderr) != 0 {
		return errAIOCoreE2ERuntime
	}
	path := aioCoreE2EWorkspaceRoot + "/contract.txt"
	contentA := []byte("alpha\nneedle\n")
	if runtime.writeFile(ctx, sessionA, path, contentA, false) != nil ||
		runtime.writeFile(ctx, sessionA, path, []byte("tail\n"), true) != nil {
		return errAIOCoreE2ERuntime
	}
	operation, err := runtime.prepareOperation(SessionOperationReplace, SessionOperationPayload{
		Path: path, Old: []byte("needle"), New: []byte("replaced"),
	}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	projection, err := runtime.submitPreparedOperation(ctx, sessionA, operation)
	if err != nil || projection.State != SessionOperationSucceeded {
		return errAIOCoreE2ERuntime
	}
	wantA := []byte("alpha\nreplaced\ntail\n")
	gotA, err := runtime.readFile(ctx, sessionA, path, 4096)
	if err != nil || !bytes.Equal(gotA, wantA) {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.verifyFileQueryOperations(ctx, sessionA, path, wantA); err != nil {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.verifyMaximumFileAndRequestBounds(ctx, sessionA); err != nil {
		return errAIOCoreE2ERuntime
	}
	if runtime.writeFile(ctx, sessionB, path, []byte("thread-b"), false) != nil ||
		runtime.writeFile(ctx, sessionC, path, []byte("user-b"), false) != nil {
		return errAIOCoreE2ERuntime
	}
	gotB, errB := runtime.readFile(ctx, sessionB, path, 4096)
	gotC, errC := runtime.readFile(ctx, sessionC, path, 4096)
	gotAAgain, errA := runtime.readFile(ctx, sessionA, path, 4096)
	if errA != nil || errB != nil || errC != nil || !bytes.Equal(gotAAgain, wantA) ||
		!bytes.Equal(gotB, []byte("thread-b")) || !bytes.Equal(gotC, []byte("user-b")) {
		return errAIOCoreE2ERuntime
	}
	streams, _, err := runtime.execute(ctx, sessionA, "printf out; printf err >&2; exit 7", 1024)
	if err != nil || streams.ExitCode != 7 || !bytes.Equal(streams.Stdout, []byte("out")) || !bytes.Equal(streams.Stderr, []byte("err")) {
		return errAIOCoreE2ERuntime
	}
	bounded, _, err := runtime.execute(ctx, sessionA, "printf '%01024d' 0", 2048)
	if err != nil || bounded.ExitCode != 0 || len(bounded.Stdout) != 1024 || len(bounded.Stderr) != 0 {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.verifyPathRejection(ctx, sessionA); err != nil {
		return errAIOCoreE2ERuntime
	}
	secretProbe := "test -z \"${SANDBOX_RUNNER_AUTH_TOKEN+x}\" && test -z \"${MYSQL_DSN+x}\" && test -z \"${REDIS_PASSWORD+x}\" && test -z \"${SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON+x}\""
	secretResult, _, err := runtime.execute(ctx, sessionA, secretProbe, 1024)
	if err != nil || secretResult.ExitCode != 0 || len(secretResult.Stdout) != 0 || len(secretResult.Stderr) != 0 {
		return errAIOCoreE2ERuntime
	}
	environment, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
		Command: "printf ok", Env: map[string]string{"AIO_CORE_E2E_UNFROZEN": "value"}, CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
	}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
	if err != nil || runtime.expectRejectedOperation(ctx, sessionA, environment) != nil {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) verifyFileQueryOperations(ctx context.Context, session *aioCoreE2ESessionFixture, path string, want []byte) error {
	tests := []struct {
		kind    SessionOperationKind
		payload SessionOperationPayload
		check   func(json.RawMessage) bool
	}{
		{kind: SessionOperationList, payload: SessionOperationPayload{Path: aioCoreE2EWorkspaceRoot, Limit: 20}, check: func(raw json.RawMessage) bool {
			var entries []infrasandbox.FileEntry
			if decodeStrictJSON(raw, &entries) != nil || len(entries) == 0 {
				return false
			}
			for _, entry := range entries {
				if entry.Path == path && !entry.Directory {
					return true
				}
			}
			return false
		}},
		{kind: SessionOperationGlob, payload: SessionOperationPayload{Path: aioCoreE2EWorkspaceRoot, Pattern: "*.txt", Limit: 20}, check: func(raw json.RawMessage) bool {
			var entries []infrasandbox.FileEntry
			return decodeStrictJSON(raw, &entries) == nil && len(entries) == 1 && entries[0].Path == path
		}},
		{kind: SessionOperationGrep, payload: SessionOperationPayload{Path: aioCoreE2EWorkspaceRoot, Pattern: "replaced", Limit: 20}, check: func(raw json.RawMessage) bool {
			var matches []infrasandbox.GrepMatch
			return decodeStrictJSON(raw, &matches) == nil && len(matches) == 1 && matches[0].Path == path && strings.Contains(matches[0].Text, "replaced")
		}},
		{kind: SessionOperationDownload, payload: SessionOperationPayload{Path: path, MaxBytes: 4096}, check: func(raw json.RawMessage) bool {
			var content []byte
			return decodeStrictJSON(raw, &content) == nil && bytes.Equal(content, want)
		}},
	}
	for _, test := range tests {
		operation, err := runtime.prepareOperation(test.kind, test.payload, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
		if err != nil {
			return errAIOCoreE2ERuntime
		}
		projection, err := runtime.submitPreparedOperation(ctx, session, operation)
		if err != nil || projection.State != SessionOperationSucceeded || !test.check(projection.Result) {
			return errAIOCoreE2ERuntime
		}
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) verifyMaximumFileAndRequestBounds(ctx context.Context, session *aioCoreE2ESessionFixture) error {
	const chunkBytes = 512 * 1024
	path := aioCoreE2EWorkspaceRoot + "/maximum-file.bin"
	want := make([]byte, infrasandbox.MaxSessionFileBytes)
	for index := range want {
		want[index] = byte('a' + index%23)
	}
	for offset := 0; offset < len(want); offset += chunkBytes {
		end := offset + chunkBytes
		if end > len(want) {
			end = len(want)
		}
		if runtime.writeFile(ctx, session, path, want[offset:end], offset != 0) != nil {
			return errAIOCoreE2ERuntime
		}
	}
	wantDigest := sha256.Sum256(want)
	read, err := runtime.readFile(ctx, session, path, infrasandbox.MaxSessionFileBytes)
	if err != nil || len(read) != infrasandbox.MaxSessionFileBytes || sha256.Sum256(read) != wantDigest {
		return errAIOCoreE2ERuntime
	}
	download, err := runtime.prepareOperation(SessionOperationDownload, SessionOperationPayload{
		Path: path, MaxBytes: infrasandbox.MaxSessionFileBytes,
	}, time.Now().UTC().Add(2*time.Minute))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	projection, err := runtime.submitPreparedOperation(ctx, session, download)
	if err != nil || projection.State != SessionOperationSucceeded {
		return errAIOCoreE2ERuntime
	}
	var downloaded []byte
	if decodeStrictJSON(projection.Result, &downloaded) != nil || len(downloaded) != infrasandbox.MaxSessionFileBytes || sha256.Sum256(downloaded) != wantDigest {
		return errAIOCoreE2ERuntime
	}
	for _, kind := range []SessionOperationKind{SessionOperationRead, SessionOperationDownload} {
		operation, err := runtime.prepareOperation(kind, SessionOperationPayload{
			Path: path, MaxBytes: infrasandbox.MaxSessionFileBytes + 1,
		}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
		if err != nil || runtime.expectRejectedOperation(ctx, session, operation) != nil {
			return errAIOCoreE2ERuntime
		}
	}
	operationID, err := runtime.nextIdentifier("oversized-http")
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	claims, err := runtime.sessionClaims(session, operationID)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	var publicError map[string]any
	requestPath := "/v1/sessions/" + session.ref.SessionID + "/operations"
	if runtime.doSignedRequest(ctx, http.MethodPost, requestPath, bytes.Repeat([]byte("x"), maxRequestBytes+1), claims,
		http.StatusBadRequest, &publicError) != nil || len(publicError) == 0 {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) verifyPathRejection(ctx context.Context, session *aioCoreE2ESessionFixture) error {
	for _, forbiddenPath := range []string{
		aioCoreE2EWorkspaceRoot + "/../escape",
		"/tmp/aio-core-e2e-escape",
		"/mnt/user-data/999/999/foreign/workspace/escape",
	} {
		operation, err := runtime.prepareOperation(SessionOperationRead, SessionOperationPayload{Path: forbiddenPath, MaxBytes: 1024}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
		if err != nil || runtime.expectRejectedOperation(ctx, session, operation) != nil {
			return errAIOCoreE2ERuntime
		}
	}
	for _, forbiddenCWD := range []string{
		"/mnt/user-data/uploads", "/mnt/user-data/outputs", "/mnt/skills",
		"/mnt/user-data/999/999/foreign/workspace", aioCoreE2EWorkspaceRoot + "/../escape",
	} {
		operation, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
			Command: "printf ok", CWD: forbiddenCWD, MaxOutputBytes: 1024,
		}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
		if err != nil || runtime.expectRejectedOperation(ctx, session, operation) != nil {
			return errAIOCoreE2ERuntime
		}
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) expectRejectedOperation(ctx context.Context, session *aioCoreE2ESessionFixture, operation aioCoreE2EPreparedOperation) error {
	claims, err := runtime.sessionClaims(session, operation.operationID)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	var publicError map[string]any
	requestPath := "/v1/sessions/" + session.ref.SessionID + "/operations"
	if err := runtime.doSignedRequest(ctx, http.MethodPost, requestPath, operation.body, claims, http.StatusBadRequest, &publicError); err != nil || len(publicError) == 0 {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) verifyConcurrentScheduling(ctx context.Context, sessionA, sessionB, sessionC *aioCoreE2ESessionFixture) error {
	type operationResult struct {
		projection SessionOperationProjection
		err        error
	}
	prepare := func(command string) (aioCoreE2EPreparedOperation, error) {
		return runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
			Command: command, CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
		}, time.Now().UTC().Add(time.Minute))
	}
	start := func(session *aioCoreE2ESessionFixture, operation aioCoreE2EPreparedOperation) <-chan operationResult {
		done := make(chan operationResult, 1)
		go func() {
			projection, err := runtime.submitPreparedOperation(ctx, session, operation)
			done <- operationResult{projection: projection, err: err}
		}()
		return done
	}
	first, err := prepare("sleep 4; printf a")
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	firstDone := start(sessionA, first)
	if runtime.waitForOperationState(ctx, sessionA, first, SessionOperationRunning, 15*time.Second) != nil {
		return errAIOCoreE2ERuntime
	}
	sameShell, err := prepare("printf b")
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	sameShellDone := start(sessionA, sameShell)
	if runtime.waitForOperationState(ctx, sessionA, sameShell, SessionOperationQueued, 15*time.Second) != nil {
		return errAIOCoreE2ERuntime
	}
	parallel, err := prepare("sleep 1; printf c")
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	parallelDone := start(sessionC, parallel)
	if runtime.waitForOperationState(ctx, sessionC, parallel, SessionOperationRunning, 15*time.Second) != nil {
		return errAIOCoreE2ERuntime
	}
	third, err := prepare("printf d")
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	thirdDone := start(sessionB, third)
	if runtime.waitForOperationState(ctx, sessionB, third, SessionOperationQueued, 15*time.Second) != nil {
		return errAIOCoreE2ERuntime
	}
	status, err := runtime.runtimeStatus(ctx)
	if err != nil || status.Running != 2 || status.QueueDepth < 2 || status.UsedWeight != status.TotalWeight {
		return errAIOCoreE2ERuntime
	}
	select {
	case result := <-parallelDone:
		if result.err != nil || result.projection.State != SessionOperationSucceeded {
			return errAIOCoreE2ERuntime
		}
	case <-time.After(10 * time.Second):
		return errAIOCoreE2ERuntime
	}
	for _, queued := range []struct {
		session   *aioCoreE2ESessionFixture
		operation aioCoreE2EPreparedOperation
	}{{sessionA, sameShell}, {sessionB, third}} {
		projection, err := runtime.getOperation(ctx, queued.session, queued.operation)
		if err != nil || projection.State != SessionOperationQueued {
			return errAIOCoreE2ERuntime
		}
	}
	for _, done := range []<-chan operationResult{firstDone, sameShellDone, thirdDone} {
		select {
		case result := <-done:
			if result.err != nil || result.projection.State != SessionOperationSucceeded {
				return errAIOCoreE2ERuntime
			}
		case <-time.After(15 * time.Second):
			return errAIOCoreE2ERuntime
		}
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) verifyTimeoutAndOutputBounds(ctx context.Context, session *aioCoreE2ESessionFixture) error {
	timed, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
		Command: "sleep 30", CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
	}, time.Now().UTC().Add(2*time.Second))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	timedProjection, err := runtime.submitPreparedOperation(ctx, session, timed)
	if err != nil || timedProjection.State != SessionOperationTimedOut || len(timedProjection.Result) != 0 {
		return errAIOCoreE2ERuntime
	}
	timedStored, err := runtime.getOperation(ctx, session, timed)
	if err != nil || timedStored.State != SessionOperationTimedOut || timedStored.ReasonCode != timedProjection.ReasonCode || len(timedStored.Result) != 0 {
		return errAIOCoreE2ERuntime
	}
	overflow, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
		Command: "printf '%02048d' 0", CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
	}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	overflowProjection, err := runtime.submitPreparedOperation(ctx, session, overflow)
	if err != nil || overflowProjection.State != SessionOperationSucceeded || overflowProjection.ReasonCode != "" || len(overflowProjection.Result) == 0 {
		return errAIOCoreE2ERuntime
	}
	var overflowResult executionTerminalResult
	if decodeStrictJSON(overflowProjection.Result, &overflowResult) != nil || overflowResult.ExitCode != 0 ||
		len(overflowResult.Stdout) != 1024 || len(overflowResult.Stderr) != 0 {
		return errAIOCoreE2ERuntime
	}
	overflowStored, err := runtime.getOperation(ctx, session, overflow)
	if err != nil || overflowStored.State != SessionOperationSucceeded || overflowStored.ReasonCode != "" ||
		overflowStored.ResultDigest != overflowProjection.ResultDigest || !bytes.Equal(overflowStored.Result, overflowProjection.Result) {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) verifyReleaseAndReacquire(ctx context.Context, session *aioCoreE2ESessionFixture) error {
	operationID, err := runtime.nextIdentifier("release")
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	claims, err := runtime.sessionClaims(session, operationID)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	requestPath := "/v1/sessions/" + session.ref.SessionID + ":release"
	if err := runtime.doSignedRequest(ctx, http.MethodPost, requestPath, []byte("{}"), claims, http.StatusNoContent, nil); err != nil {
		return errAIOCoreE2ERuntime
	}
	released, err := runtime.repository.GetRuntimeSession(ctx, session.ref)
	if err != nil || released.Ref != session.ref || released.State != domainsandbox.SessionStateReleased ||
		released.UpstreamShellID != "" || released.RecoveryReason != "" {
		return errAIOCoreE2ERuntime
	}
	if err := runtime.reacquireSession(ctx, session); err != nil {
		return errAIOCoreE2ERuntime
	}
	reacquired, err := runtime.repository.GetRuntimeSession(ctx, session.ref)
	if err != nil || reacquired.Ref != session.ref || reacquired.State != domainsandbox.SessionStateActive ||
		reacquired.UpstreamShellID == "" || reacquired.RecoveryReason != "" {
		return errAIOCoreE2ERuntime
	}
	content, err := runtime.readFile(ctx, session, aioCoreE2EWorkspaceRoot+"/contract.txt", 64)
	if err != nil || !bytes.Equal(content, []byte("user-b")) {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) reacquireSession(ctx context.Context, session *aioCoreE2ESessionFixture) error {
	runID, err := runtime.nextIdentifier("run")
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	operationID, err := runtime.nextIdentifier("reacquire")
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	wire := sessionAcquireWire{
		Schema: sessionAcquireSchemaV1, DeploymentID: runtime.config.deploymentID,
		ProviderID: runtime.scope.providerID, Scope: sandboxidentity.ScopeAgent,
		SpaceID: session.ref.Key.SpaceID, UserID: session.ref.Key.UserID, ThreadID: session.ref.Key.ThreadID,
		RunID: runID, OperationID: operationID, Profile: string(domainsandbox.SessionProfileCore),
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	claims := sandboxidentity.SessionRequest{
		DeploymentID: runtime.config.deploymentID, ProviderID: runtime.scope.providerID,
		Scope: sandboxidentity.ScopeAgent, SpaceID: session.ref.Key.SpaceID, UserID: session.ref.Key.UserID,
		ThreadID: session.ref.Key.ThreadID, RunID: runID, OperationID: operationID, Profile: string(domainsandbox.SessionProfileCore),
	}
	var projection SessionProjection
	if err := runtime.doSignedRequest(ctx, http.MethodPost, "/v1/sessions:acquire", body, claims, http.StatusOK, &projection); err != nil ||
		projection.Schema != sessionProjectionSchemaV1 || projection.SessionID != session.ref.SessionID ||
		projection.State != domainsandbox.SessionStateActive || projection.RuntimeGeneration != session.ref.RuntimeGeneration ||
		projection.Profile != domainsandbox.SessionProfileCore {
		return errAIOCoreE2ERuntime
	}
	session.runID = runID
	return nil
}

func (runtime *aioCoreE2ERuntime) verifyCancellation(ctx context.Context, session *aioCoreE2ESessionFixture) error {
	operation, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
		Command: "sleep 30", CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
	}, time.Now().UTC().Add(time.Minute))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	type operationResult struct {
		projection SessionOperationProjection
		err        error
	}
	done := make(chan operationResult, 1)
	go func() {
		projection, submitErr := runtime.submitPreparedOperation(ctx, session, operation)
		done <- operationResult{projection: projection, err: submitErr}
	}()
	if runtime.waitForOperationState(ctx, session, operation, SessionOperationRunning, 15*time.Second) != nil {
		return errAIOCoreE2ERuntime
	}
	canceled, err := runtime.cancelOperation(ctx, session, operation)
	if err != nil || canceled.State != SessionOperationRunning {
		return errAIOCoreE2ERuntime
	}
	select {
	case result := <-done:
		if result.err != nil || result.projection.State != SessionOperationCanceled || len(result.projection.Result) != 0 {
			return errAIOCoreE2ERuntime
		}
	case <-time.After(15 * time.Second):
		return errAIOCoreE2ERuntime
	}
	terminal, err := runtime.getOperation(ctx, session, operation)
	if err != nil || terminal.State != SessionOperationCanceled || len(terminal.Result) != 0 {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) verifyQueuedCancellation(ctx context.Context, firstBlockerSession, queuedSession, secondBlockerSession *aioCoreE2ESessionFixture) error {
	if maxSessionOperationQueueDepth != 64 || coreSessionTotalWeight != 2 {
		return errAIOCoreE2ERuntime
	}
	type operationResult struct {
		projection SessionOperationProjection
		err        error
	}
	blockerSessions := []*aioCoreE2ESessionFixture{firstBlockerSession, secondBlockerSession}
	blockers := make([]aioCoreE2EPreparedOperation, len(blockerSessions))
	blockerDone := make([]<-chan operationResult, len(blockerSessions))
	for index, session := range blockerSessions {
		operation, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
			Command: "sleep 120", CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
		}, time.Now().UTC().Add(3*time.Minute))
		if err != nil {
			return errAIOCoreE2ERuntime
		}
		blockers[index] = operation
		done := make(chan operationResult, 1)
		blockerDone[index] = done
		go func(target *aioCoreE2ESessionFixture, prepared aioCoreE2EPreparedOperation, result chan<- operationResult) {
			projection, submitErr := runtime.submitPreparedOperation(ctx, target, prepared)
			result <- operationResult{projection: projection, err: submitErr}
		}(session, operation, done)
		if runtime.waitForOperationState(ctx, session, operation, SessionOperationRunning, 15*time.Second) != nil {
			return errAIOCoreE2ERuntime
		}
	}
	queuedCount := maxSessionOperationQueueDepth - len(blockers)
	queued := make([]aioCoreE2EPreparedOperation, queuedCount)
	queuedDone := make([]<-chan operationResult, queuedCount)
	for index := range queued {
		operation, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
			Command: "printf should-not-run >> queue-bound-cancel-marker", CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
		}, time.Now().UTC().Add(3*time.Minute))
		if err != nil {
			return errAIOCoreE2ERuntime
		}
		queued[index] = operation
		done := make(chan operationResult, 1)
		queuedDone[index] = done
		go func(prepared aioCoreE2EPreparedOperation, result chan<- operationResult) {
			projection, submitErr := runtime.submitPreparedOperation(ctx, queuedSession, prepared)
			result <- operationResult{projection: projection, err: submitErr}
		}(operation, done)
	}
	if _, err := runtime.waitRuntimeStatus(ctx, 30*time.Second, func(status SessionRuntimeStatusProjection) bool {
		return status.Running == len(blockers) && status.UsedWeight == status.TotalWeight && status.QueueDepth == queuedCount
	}); err != nil {
		return errAIOCoreE2ERuntime
	}
	overflow, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
		Command: "printf should-not-run >> queue-bound-cancel-marker", CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
	}, time.Now().UTC().Add(3*time.Minute))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	claims, err := runtime.sessionClaims(queuedSession, overflow.operationID)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	var publicError map[string]any
	requestPath := "/v1/sessions/" + queuedSession.ref.SessionID + "/operations"
	if runtime.doSignedRequest(ctx, http.MethodPost, requestPath, overflow.body, claims, http.StatusServiceUnavailable, &publicError) != nil || len(publicError) == 0 {
		return errAIOCoreE2ERuntime
	}
	for index, operation := range queued {
		canceled, err := runtime.cancelOperation(ctx, queuedSession, operation)
		if err != nil || canceled.State != SessionOperationCanceled || len(canceled.Result) != 0 {
			return errAIOCoreE2ERuntime
		}
		select {
		case result := <-queuedDone[index]:
			if result.err != nil || result.projection.State != SessionOperationCanceled || len(result.projection.Result) != 0 {
				return errAIOCoreE2ERuntime
			}
		case <-time.After(15 * time.Second):
			return errAIOCoreE2ERuntime
		}
	}
	if _, err := runtime.waitRuntimeStatus(ctx, 30*time.Second, func(status SessionRuntimeStatusProjection) bool {
		return status.Running == len(blockers) && status.UsedWeight == status.TotalWeight && status.QueueDepth == 0
	}); err != nil {
		return errAIOCoreE2ERuntime
	}
	firstCanceled, err := runtime.cancelOperation(ctx, firstBlockerSession, blockers[0])
	if err != nil || firstCanceled.State != SessionOperationRunning {
		return errAIOCoreE2ERuntime
	}
	select {
	case result := <-blockerDone[0]:
		if result.err != nil || result.projection.State != SessionOperationCanceled {
			return errAIOCoreE2ERuntime
		}
	case <-time.After(15 * time.Second):
		return errAIOCoreE2ERuntime
	}
	healthy, _, err := runtime.execute(ctx, queuedSession, "test ! -e queue-bound-cancel-marker && printf queue-bound-ok", 1024)
	if err != nil || healthy.ExitCode != 0 || !bytes.Equal(healthy.Stdout, []byte("queue-bound-ok")) || len(healthy.Stderr) != 0 {
		return errAIOCoreE2ERuntime
	}
	secondCanceled, err := runtime.cancelOperation(ctx, secondBlockerSession, blockers[1])
	if err != nil || secondCanceled.State != SessionOperationRunning {
		return errAIOCoreE2ERuntime
	}
	select {
	case result := <-blockerDone[1]:
		if result.err != nil || result.projection.State != SessionOperationCanceled {
			return errAIOCoreE2ERuntime
		}
	case <-time.After(15 * time.Second):
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) verifyFileHTTPContextCancellation(ctx context.Context, session *aioCoreE2ESessionFixture) error {
	operation, err := runtime.prepareOperation(SessionOperationDownload, SessionOperationPayload{
		Path: aioCoreE2EWorkspaceRoot + "/maximum-file.bin", MaxBytes: infrasandbox.MaxSessionFileBytes,
	}, time.Now().UTC().Add(2*time.Minute))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	type operationResult struct {
		projection SessionOperationProjection
		err        error
	}
	requestCtx, cancelRequest := context.WithCancel(ctx)
	done := make(chan operationResult, 1)
	go func() {
		projection, submitErr := runtime.submitPreparedOperation(requestCtx, session, operation)
		done <- operationResult{projection: projection, err: submitErr}
	}()
	if runtime.waitForOperationState(ctx, session, operation, SessionOperationRunning, 15*time.Second) != nil {
		cancelRequest()
		return errAIOCoreE2ERuntime
	}
	cancelRequest()
	select {
	case result := <-done:
		if result.err == nil && result.projection.State != SessionOperationCanceled {
			return errAIOCoreE2ERuntime
		}
	case <-time.After(15 * time.Second):
		return errAIOCoreE2ERuntime
	}
	if runtime.waitForOperationState(ctx, session, operation, SessionOperationCanceled, 15*time.Second) != nil {
		return errAIOCoreE2ERuntime
	}
	healthy, _, err := runtime.execute(ctx, session, "printf file-context-cancel-ok", 1024)
	if err != nil || healthy.ExitCode != 0 || !bytes.Equal(healthy.Stdout, []byte("file-context-cancel-ok")) || len(healthy.Stderr) != 0 {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) waitForOperationState(ctx context.Context, session *aioCoreE2ESessionFixture, operation aioCoreE2EPreparedOperation, state SessionOperationState, maximum time.Duration) error {
	deadline := time.Now().Add(maximum)
	for time.Now().Before(deadline) {
		projection, err := runtime.getOperation(ctx, session, operation)
		if err == nil && projection.State == state {
			return nil
		}
		select {
		case <-ctx.Done():
			return errAIOCoreE2ERuntime
		case <-time.After(100 * time.Millisecond):
		}
	}
	return errAIOCoreE2ERuntime
}

func (runtime *aioCoreE2ERuntime) verifyResultRecoveryAndNoReplay(ctx context.Context, session *aioCoreE2ESessionFixture) error {
	operation, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
		Command: "printf x >> replay-marker; wc -c < replay-marker", CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
	}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	first, err := runtime.submitPreparedOperation(ctx, session, operation)
	if err != nil || first.State != SessionOperationSucceeded || len(first.Result) == 0 || first.ResultDigest == "" {
		return errAIOCoreE2ERuntime
	}
	recovered, err := runtime.getOperation(ctx, session, operation)
	if err != nil || recovered.State != SessionOperationSucceeded || recovered.ResultDigest != first.ResultDigest || !bytes.Equal(recovered.Result, first.Result) {
		return errAIOCoreE2ERuntime
	}
	replayed, err := runtime.submitPreparedOperation(ctx, session, operation)
	if err != nil || replayed.State != SessionOperationSucceeded || replayed.ResultDigest != first.ResultDigest || !bytes.Equal(replayed.Result, first.Result) {
		return errAIOCoreE2ERuntime
	}
	marker, err := runtime.readFile(ctx, session, aioCoreE2EWorkspaceRoot+"/replay-marker", 16)
	if err != nil || !bytes.Equal(marker, []byte("x")) {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) waitRuntimeStatus(ctx context.Context, maximum time.Duration, accept func(SessionRuntimeStatusProjection) bool) (SessionRuntimeStatusProjection, error) {
	if runtime == nil || ctx == nil || maximum <= 0 || accept == nil {
		return SessionRuntimeStatusProjection{}, errAIOCoreE2ERuntime
	}
	deadline := time.Now().Add(maximum)
	for time.Now().Before(deadline) {
		status, err := runtime.runtimeStatus(ctx)
		if err == nil && accept(status) {
			return status, nil
		}
		select {
		case <-ctx.Done():
			return SessionRuntimeStatusProjection{}, errAIOCoreE2ERuntime
		case <-time.After(250 * time.Millisecond):
		}
	}
	return SessionRuntimeStatusProjection{}, errAIOCoreE2ERuntime
}

func (runtime *aioCoreE2ERuntime) recoverAllSessions(ctx context.Context, expectedGeneration uint64) error {
	for _, session := range runtime.sessions {
		if runtime.recoverSession(ctx, session) != nil || session.ref.RuntimeGeneration != expectedGeneration {
			return errAIOCoreE2ERuntime
		}
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) verifyAIOGenerations(ctx context.Context, initialGeneration uint64, persistentSession *aioCoreE2ESessionFixture) error {
	if initialGeneration == 0 || initialGeneration == ^uint64(0) {
		return errAIOCoreE2ERuntime
	}
	initialDatabaseGeneration, err := runtime.databaseGeneration(ctx)
	if err != nil || initialDatabaseGeneration.Generation != initialGeneration {
		return errAIOCoreE2ERuntime
	}
	runtime.aioStopped = true
	if runtime.callControlHelper(ctx, "stop-aio") != nil {
		return errAIOCoreE2ERuntime
	}
	if _, err := runtime.waitRuntimeStatus(ctx, 30*time.Second, func(status SessionRuntimeStatusProjection) bool {
		return !status.Available && !status.RawAIOReady && !status.HostShellAvailable &&
			(status.GenerationState == "unknown" || status.GenerationState == "recovering")
	}); err != nil {
		return errAIOCoreE2ERuntime
	}
	if advertised, err := runtime.healthAdvertisesCore(ctx); err != nil || advertised {
		return errAIOCoreE2ERuntime
	}
	stoppedDatabaseGeneration, err := runtime.databaseGeneration(ctx)
	if err != nil || stoppedDatabaseGeneration.Generation != initialGeneration || stoppedDatabaseGeneration.SentinelID != initialDatabaseGeneration.SentinelID {
		return errAIOCoreE2ERuntime
	}
	if runtime.callControlHelper(ctx, "start-aio") != nil {
		return errAIOCoreE2ERuntime
	}
	runtime.aioStopped = false
	firstGeneration := initialGeneration + 1
	if _, err := runtime.waitRuntimeStatus(ctx, aioCoreE2EControlTimeout, func(status SessionRuntimeStatusProjection) bool {
		return status.CoreEnabled && status.RawAIOReady && status.GenerationState == "ready" &&
			status.RuntimeGeneration == firstGeneration && !status.HostShellAvailable
	}); err != nil {
		return errAIOCoreE2ERuntime
	}
	if advertised, err := runtime.healthAdvertisesCore(ctx); err != nil || !advertised {
		return errAIOCoreE2ERuntime
	}
	firstDatabaseGeneration, err := runtime.databaseGeneration(ctx)
	if err != nil || firstDatabaseGeneration.Generation != firstGeneration || firstDatabaseGeneration.SentinelID == initialDatabaseGeneration.SentinelID {
		return errAIOCoreE2ERuntime
	}
	if runtime.recoverAllSessions(ctx, firstGeneration) != nil {
		return errAIOCoreE2ERuntime
	}
	if _, err := runtime.waitRuntimeStatus(ctx, 30*time.Second, func(status SessionRuntimeStatusProjection) bool {
		return aioCoreE2EIdleFixtureStatus(status, firstGeneration, len(runtime.sessions))
	}); err != nil {
		return errAIOCoreE2ERuntime
	}
	marker, err := runtime.readFile(ctx, persistentSession, aioCoreE2EWorkspaceRoot+"/replay-marker", 16)
	if err != nil || !bytes.Equal(marker, []byte("x")) {
		return errAIOCoreE2ERuntime
	}
	if firstGeneration == ^uint64(0) {
		return errAIOCoreE2ERuntime
	}
	runtime.aioStopped = true
	if runtime.callControlHelper(ctx, "recreate-aio-preserve-volumes") != nil {
		return errAIOCoreE2ERuntime
	}
	runtime.aioStopped = false
	secondGeneration := firstGeneration + 1
	if _, err := runtime.waitRuntimeStatus(ctx, aioCoreE2EControlTimeout, func(status SessionRuntimeStatusProjection) bool {
		return status.CoreEnabled && status.RawAIOReady && status.GenerationState == "ready" &&
			status.RuntimeGeneration == secondGeneration && !status.HostShellAvailable
	}); err != nil {
		return errAIOCoreE2ERuntime
	}
	secondDatabaseGeneration, err := runtime.databaseGeneration(ctx)
	if err != nil || secondDatabaseGeneration.Generation != secondGeneration || secondDatabaseGeneration.SentinelID == firstDatabaseGeneration.SentinelID {
		return errAIOCoreE2ERuntime
	}
	if runtime.recoverAllSessions(ctx, secondGeneration) != nil {
		return errAIOCoreE2ERuntime
	}
	if _, err := runtime.waitRuntimeStatus(ctx, 30*time.Second, func(status SessionRuntimeStatusProjection) bool {
		return aioCoreE2EIdleFixtureStatus(status, secondGeneration, len(runtime.sessions))
	}); err != nil {
		return errAIOCoreE2ERuntime
	}
	marker, err = runtime.readFile(ctx, persistentSession, aioCoreE2EWorkspaceRoot+"/replay-marker", 16)
	if err != nil || !bytes.Equal(marker, []byte("x")) {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) verifyRunnerRestartNoReplay(ctx context.Context, blockerSession, queuedSession *aioCoreE2ESessionFixture) error {
	before, err := runtime.runtimeStatus(ctx)
	if err != nil || !aioCoreE2EReadyStatus(before) {
		return errAIOCoreE2ERuntime
	}
	databaseBefore, err := runtime.databaseGeneration(ctx)
	if err != nil || databaseBefore.Generation != before.RuntimeGeneration {
		return errAIOCoreE2ERuntime
	}
	blocker, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
		Command: "sleep 60", CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
	}, time.Now().UTC().Add(2*time.Minute))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	queued, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
		Command: "printf x >> queued-marker", CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
	}, time.Now().UTC().Add(2*time.Minute))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	type operationResult struct {
		projection SessionOperationProjection
		err        error
	}
	blockerDone := make(chan operationResult, 1)
	go func() {
		projection, submitErr := runtime.submitPreparedOperation(ctx, blockerSession, blocker)
		blockerDone <- operationResult{projection: projection, err: submitErr}
	}()
	if runtime.waitForOperationState(ctx, blockerSession, blocker, SessionOperationRunning, 15*time.Second) != nil {
		return errAIOCoreE2ERuntime
	}
	queuedDone := make(chan operationResult, 1)
	go func() {
		projection, submitErr := runtime.submitPreparedOperation(ctx, queuedSession, queued)
		queuedDone <- operationResult{projection: projection, err: submitErr}
	}()
	if runtime.waitForOperationState(ctx, queuedSession, queued, SessionOperationQueued, 15*time.Second) != nil {
		return errAIOCoreE2ERuntime
	}
	queuedStatus, err := runtime.runtimeStatus(ctx)
	if err != nil || queuedStatus.QueueDepth < 1 || queuedStatus.Running < 1 || queuedStatus.RuntimeGeneration != before.RuntimeGeneration {
		return errAIOCoreE2ERuntime
	}
	if runtime.callControlHelper(ctx, "restart-runner") != nil {
		return errAIOCoreE2ERuntime
	}
	if _, err := runtime.waitRuntimeStatus(ctx, aioCoreE2EControlTimeout, func(status SessionRuntimeStatusProjection) bool {
		return status.CoreEnabled && status.RawAIOReady && status.GenerationState == "ready" &&
			status.RuntimeGeneration == before.RuntimeGeneration && !status.HostShellAvailable
	}); err != nil {
		return errAIOCoreE2ERuntime
	}
	databaseAfter, err := runtime.databaseGeneration(ctx)
	if err != nil || databaseAfter.Generation != databaseBefore.Generation || databaseAfter.SentinelID != databaseBefore.SentinelID {
		return errAIOCoreE2ERuntime
	}
	if runtime.waitForOperationState(ctx, blockerSession, blocker, SessionOperationUnknown, 30*time.Second) != nil ||
		runtime.waitForOperationState(ctx, queuedSession, queued, SessionOperationUnknown, 30*time.Second) != nil {
		return errAIOCoreE2ERuntime
	}
	blockerRow, blockerErr := runtime.repository.GetRuntimeSession(ctx, blockerSession.ref)
	queuedRow, queuedErr := runtime.repository.GetRuntimeSession(ctx, queuedSession.ref)
	if blockerErr != nil || queuedErr != nil || blockerRow.State != domainsandbox.SessionStateRecovering ||
		blockerRow.RecoveryReason != domainsandbox.SessionOperationFenceReason || queuedRow.State != domainsandbox.SessionStateActive ||
		queuedRow.RecoveryReason != "" || blockerRow.Ref != blockerSession.ref || queuedRow.Ref != queuedSession.ref {
		return errAIOCoreE2ERuntime
	}
	for _, done := range []<-chan operationResult{blockerDone, queuedDone} {
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			return errAIOCoreE2ERuntime
		}
	}
	if runtime.recoverSession(ctx, blockerSession) != nil {
		return errAIOCoreE2ERuntime
	}
	if _, err := runtime.waitRuntimeStatus(ctx, 30*time.Second, func(status SessionRuntimeStatusProjection) bool {
		return aioCoreE2EReadyStatus(status) && status.RuntimeGeneration == before.RuntimeGeneration
	}); err != nil {
		return errAIOCoreE2ERuntime
	}
	replayed, err := runtime.submitPreparedOperation(ctx, queuedSession, queued)
	if err != nil || replayed.State != SessionOperationUnknown || len(replayed.Result) != 0 {
		return errAIOCoreE2ERuntime
	}
	absent, _, err := runtime.execute(ctx, queuedSession, "test ! -e queued-marker", 1024)
	if err != nil || absent.ExitCode != 0 || len(absent.Stdout) != 0 || len(absent.Stderr) != 0 {
		return errAIOCoreE2ERuntime
	}
	return nil
}

type aioCoreE2EPreparedOperation struct {
	operationID string
	kind        SessionOperationKind
	deadline    time.Time
	payload     SessionOperationPayload
	body        []byte
}

func newAIOCoreE2ECleanupRuntime(ctx context.Context, config *aioCoreE2EConfig) (*aioCoreE2ERuntime, error) {
	if ctx == nil || config == nil || !validAIOCoreE2EPrefix(config.prefix) || config.deploymentID != config.prefix {
		return nil, errAIOCoreE2ERuntime
	}
	connector, err := mysqldriver.NewConnector(config.mysqlConfig)
	if err != nil {
		return nil, errAIOCoreE2ERuntime
	}
	sqlDB := sql.OpenDB(connector)
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(2 * time.Minute)
	if err := sqlDB.PingContext(ctx); err != nil || checkAIOCoreE2EDevSchema(ctx, sqlDB, config.databaseName) != nil {
		_ = sqlDB.Close()
		return nil, errAIOCoreE2ERuntime
	}
	redisClient := goredis.NewClient(&goredis.Options{
		Addr: config.redisAddr, Password: config.redisPassword, DB: config.redisDB,
		DialTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, MaxRetries: 1,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		_ = redisClient.Close()
		_ = sqlDB.Close()
		return nil, errAIOCoreE2ERuntime
	}
	threadA, threadB, threadC := config.prefix+"-thread-a", config.prefix+"-thread-b", config.prefix+"-thread-c"
	return &aioCoreE2ERuntime{
		config: config, sqlDB: sqlDB, redis: redisClient, providerActor: config.userIDA,
		scope: aioCoreE2EFixtureScope{
			prefix: config.prefix, deploymentID: config.deploymentID, providerKey: config.prefix + "-provider", spaceID: config.spaceID,
			userIDs:   map[int64]struct{}{config.userIDA: {}, config.userIDB: {}},
			threadIDs: map[string]struct{}{threadA: {}, threadB: {}, threadC: {}}, sessionIDs: make(map[string]struct{}),
		},
	}, nil
}

func (runtime *aioCoreE2ERuntime) cleanupOnly(ctx context.Context) (cleanupErr error) {
	if runtime == nil || runtime.httpClient != nil || runtime.repository != nil || runtime.db != nil || ctx == nil {
		return errAIOCoreE2ERuntime
	}
	defer func() {
		if runtime.closeDependencies() != nil {
			cleanupErr = errAIOCoreE2ERuntime
		}
	}()
	providerFound, err := runtime.reconcileProviderBusinessIdentity(ctx)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	if !providerFound {
		return runtime.cleanupRedisNamespace(ctx)
	}
	identities := []struct {
		userID   int64
		threadID string
		label    string
	}{
		{runtime.config.userIDA, runtime.config.prefix + "-thread-a", "cleanup-a"},
		{runtime.config.userIDA, runtime.config.prefix + "-thread-b", "cleanup-b"},
		{runtime.config.userIDB, runtime.config.prefix + "-thread-c", "cleanup-c"},
	}
	for _, identity := range identities {
		fixture, err := runtime.registerSessionBusinessIdentity(identity.userID, identity.threadID, runtime.config.prefix+"-"+identity.label)
		if err != nil {
			return errAIOCoreE2ERuntime
		}
		row, found, err := runtime.readSessionBusinessRow(ctx, fixture.ref.Key)
		if err != nil {
			return errAIOCoreE2ERuntime
		}
		if !found {
			continue
		}
		if !safeAIOCoreE2EDestroyedBusinessRow(row) {
			return errAIOCoreE2ERuntime
		}
		fixture.ref = row.ref
		runtime.scope.sessionIDs[row.ref.SessionID] = struct{}{}
		if runtime.deleteDestroyedBusinessRow(ctx, fixture) != nil {
			return errAIOCoreE2ERuntime
		}
	}
	var providerSessions int
	if err := runtime.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_runtime_sessions WHERE provider_id = ?`, runtime.scope.providerID).Scan(&providerSessions); err != nil || providerSessions != 0 {
		return errAIOCoreE2ERuntime
	}
	result, err := runtime.sqlDB.ExecContext(ctx, aioCoreE2EDeleteProviderSQL, runtime.scope.providerID, runtime.scope.providerKey, runtime.providerActor)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return errAIOCoreE2ERuntime
	}
	var remaining int
	if err := runtime.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_providers
WHERE id = ? AND provider_key = ? AND created_by = ?`, runtime.scope.providerID, runtime.scope.providerKey, runtime.providerActor).Scan(&remaining); err != nil || remaining != 0 {
		return errAIOCoreE2ERuntime
	}
	return runtime.cleanupRedisNamespace(ctx)
}

func newAIOCoreE2ERuntime(ctx context.Context, config *aioCoreE2EConfig) (*aioCoreE2ERuntime, error) {
	if ctx == nil || config == nil || !validAIOCoreE2EPrefix(config.prefix) || config.deploymentID != config.prefix {
		return nil, errAIOCoreE2ERuntime
	}
	if err := validateAIOCoreE2ERuntimeFile(config.runnerCAFile, false); err != nil {
		return nil, errAIOCoreE2ERuntime
	}
	if err := validateAIOCoreE2ERuntimeFile(config.controlHelper, true); err != nil {
		return nil, errAIOCoreE2ERuntime
	}
	certificatePEM, err := os.ReadFile(config.runnerCAFile)
	if err != nil || len(certificatePEM) == 0 || len(certificatePEM) > 1<<20 {
		return nil, errAIOCoreE2ERuntime
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificatePEM) {
		return nil, errAIOCoreE2ERuntime
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: aioCoreE2EOperationTimeout,
		IdleConnTimeout:       30 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   aioCoreE2EControlTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errAIOCoreE2ERuntime
		},
	}
	connector, err := mysqldriver.NewConnector(config.mysqlConfig)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, errAIOCoreE2ERuntime
	}
	sqlDB := sql.OpenDB(connector)
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(2 * time.Minute)
	if err := sqlDB.PingContext(ctx); err != nil || checkAIOCoreE2EDevSchema(ctx, sqlDB, config.databaseName) != nil {
		_ = sqlDB.Close()
		transport.CloseIdleConnections()
		return nil, errAIOCoreE2ERuntime
	}
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		transport.CloseIdleConnections()
		return nil, errAIOCoreE2ERuntime
	}
	redisClient := goredis.NewClient(&goredis.Options{
		Addr: config.redisAddr, Password: config.redisPassword, DB: config.redisDB,
		DialTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second,
		MaxRetries: 1,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		_ = redisClient.Close()
		_ = sqlDB.Close()
		transport.CloseIdleConnections()
		return nil, errAIOCoreE2ERuntime
	}
	threadA := config.prefix + "-thread-a"
	threadB := config.prefix + "-thread-b"
	threadC := config.prefix + "-thread-c"
	runtime := &aioCoreE2ERuntime{
		config: config, httpClient: httpClient, sqlDB: sqlDB, db: db, redis: redisClient,
		repository: infrasandbox.NewMySQLRepository(db), providerActor: config.userIDA,
		scope: aioCoreE2EFixtureScope{
			prefix: config.prefix, deploymentID: config.deploymentID, spaceID: config.spaceID,
			userIDs:    map[int64]struct{}{config.userIDA: {}, config.userIDB: {}},
			threadIDs:  map[string]struct{}{threadA: {}, threadB: {}, threadC: {}},
			sessionIDs: make(map[string]struct{}),
		},
	}
	return runtime, nil
}

func validateAIOCoreE2ERuntimeFile(path string, executable bool) error {
	if !validAIOCoreE2EAbsolutePath(path) {
		return errAIOCoreE2ERuntime
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || executable && info.Mode().Perm()&0111 == 0 {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func checkAIOCoreE2EDevSchema(ctx context.Context, db *sql.DB, databaseName string) error {
	if ctx == nil || db == nil || databaseName == "" {
		return errAIOCoreE2ERuntime
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	var actualDatabase string
	if err := tx.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&actualDatabase); err != nil || actualDatabase != databaseName {
		return errAIOCoreE2ERuntime
	}
	var description string
	var applied, total int
	var migrationError sql.NullString
	if err := tx.QueryRowContext(ctx, requiredMigrationQuery, requiredMigrationVersion).Scan(&description, &applied, &total, &migrationError); err != nil ||
		description != requiredMigrationDescription || total <= 0 || applied != total || migrationError.Valid && migrationError.String != "" {
		return errAIOCoreE2ERuntime
	}
	var requiredColumns int
	if err := tx.QueryRowContext(ctx, requiredMigrationSchemaQuery, databaseName).Scan(&requiredColumns); err != nil || requiredColumns != requiredMigrationSchemaColumnCount {
		return errAIOCoreE2ERuntime
	}
	var tableCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = ? AND table_name IN ('sandbox_providers', 'sandbox_runtime_sessions')`, databaseName).Scan(&tableCount); err != nil || tableCount != 2 {
		return errAIOCoreE2ERuntime
	}
	if err := tx.Rollback(); err != nil {
		return errAIOCoreE2ERuntime
	}
	rollback = false
	return nil
}

func (runtime *aioCoreE2ERuntime) createProvider(ctx context.Context) error {
	if runtime == nil || runtime.repository == nil || runtime.scope.providerID != 0 || ctx == nil {
		return errAIOCoreE2ERuntime
	}
	providerKey := runtime.config.prefix + "-provider"
	runtime.scope.providerKey = providerKey
	provider, err := runtime.repository.CreateProvider(ctx, domainsandbox.CreateProviderInput{
		ProviderKey:    providerKey,
		Name:           "AIO Core isolated E2E provider",
		Type:           domainsandbox.ProviderTypeRemoteHTTP,
		EndpointSecret: "encrypted:aio-core-isolated-e2e-endpoint",
		EndpointHint:   "https://***.invalid",
		Scopes:         []domainsandbox.Scope{domainsandbox.ScopeAgent},
		Policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds: 60, MemoryLimitMB: 512, CPULimit: 1,
			MaxOutputBytes: 64 * 1024, MaxConcurrency: 8,
		},
		ActorUserID: runtime.providerActor,
	})
	if err != nil || !validAIOCoreE2EProviderBusinessRow(provider, providerKey, runtime.providerActor) {
		readbackCtx, cancelReadback := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancelReadback()
		provider, err = runtime.repository.GetProviderByKey(readbackCtx, providerKey)
		if err != nil || !validAIOCoreE2EProviderBusinessRow(provider, providerKey, runtime.providerActor) {
			return errAIOCoreE2ERuntime
		}
	}
	runtime.scope.providerID = provider.ID
	if !runtime.scope.ownsProvider(provider.ID, providerKey) {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func validAIOCoreE2EProviderBusinessRow(provider *domainsandbox.Provider, providerKey string, actor int64) bool {
	return provider != nil && provider.ID > 0 && provider.ProviderKey == providerKey && provider.CreatedBy == actor &&
		provider.Type == domainsandbox.ProviderTypeRemoteHTTP && provider.DeletedAt == nil
}

func (runtime *aioCoreE2ERuntime) nextIdentifier(label string) (string, error) {
	if runtime == nil || !validSessionProtocolIdentifier(label) {
		return "", errAIOCoreE2ERuntime
	}
	sequence := atomic.AddUint64(&runtime.sequence, 1)
	value := runtime.config.prefix + "-" + label + "-" + strconv.FormatUint(sequence, 36)
	if !validSessionProtocolIdentifier(value) {
		return "", errAIOCoreE2ERuntime
	}
	return value, nil
}

func (runtime *aioCoreE2ERuntime) doSignedRequest(ctx context.Context, method, requestPath string, body []byte, claims sandboxidentity.SessionRequest, expectedStatus int, output any) error {
	if runtime == nil || runtime.httpClient == nil || ctx == nil || requestPath == "" || requestPath[0] != '/' || strings.Contains(requestPath, "?") {
		return errAIOCoreE2ERuntime
	}
	digest := sha256.Sum256(body)
	claims.RequestDigest = append([]byte(nil), digest[:]...)
	signed, err := runtime.config.contextKeys.SignSession(claims, method, requestPath)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	target := *runtime.config.runnerURL
	target.Path = requestPath
	target.RawPath, target.RawQuery, target.Fragment = "", "", ""
	request, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	request.Header.Set("Authorization", "Bearer "+runtime.config.authToken)
	request.Header.Set(sandboxidentity.SessionContextHeader, signed.Context)
	request.Header.Set(sandboxidentity.SessionContextSignatureHeader, signed.Signature)
	if method == http.MethodPost || method == http.MethodPut {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := runtime.httpClient.Do(request)
	if err != nil || response == nil {
		return errAIOCoreE2ERuntime
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, aioCoreE2EMaxHTTPResponseBytes+1))
	if err != nil || len(responseBody) > aioCoreE2EMaxHTTPResponseBytes || response.StatusCode != expectedStatus {
		return errAIOCoreE2ERuntime
	}
	if output == nil {
		if len(responseBody) != 0 {
			return errAIOCoreE2ERuntime
		}
		return nil
	}
	if len(responseBody) == 0 || decodeStrictJSON(responseBody, output) != nil {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) runtimeStatus(ctx context.Context) (SessionRuntimeStatusProjection, error) {
	var projection SessionRuntimeStatusProjection
	claims := sandboxidentity.SessionRequest{DeploymentID: runtime.config.deploymentID}
	if err := runtime.doSignedRequest(ctx, http.MethodGet, "/v1/session-runtime-status", nil, claims, http.StatusOK, &projection); err != nil ||
		!validSessionRuntimeStatusProjection(projection) || projection.HostShellAvailable || projection.InteractiveEnabled {
		return SessionRuntimeStatusProjection{}, errAIOCoreE2ERuntime
	}
	return projection, nil
}

func (runtime *aioCoreE2ERuntime) registerSessionBusinessIdentity(userID int64, threadID, runID string) (*aioCoreE2ESessionFixture, error) {
	if runtime == nil || runtime.config == nil || !validSessionProtocolIdentifier(runID) {
		return nil, errAIOCoreE2ERuntime
	}
	key := domainsandbox.SessionKey{
		DeploymentID: runtime.config.deploymentID, ProviderID: runtime.scope.providerID,
		SpaceID: runtime.config.spaceID, UserID: userID, ThreadID: threadID, Profile: domainsandbox.SessionProfileCore,
	}
	if !runtime.scope.ownsSessionKey(key) {
		return nil, errAIOCoreE2ERuntime
	}
	for _, existing := range runtime.sessions {
		if existing != nil && existing.ref.Key == key {
			return nil, errAIOCoreE2ERuntime
		}
	}
	fixture := &aioCoreE2ESessionFixture{ref: domainsandbox.SessionRef{Key: key}, runID: runID}
	runtime.sessions = append(runtime.sessions, fixture)
	return fixture, nil
}

func (runtime *aioCoreE2ERuntime) readSessionBusinessRow(ctx context.Context, key domainsandbox.SessionKey) (aioCoreE2ESessionBusinessRow, bool, error) {
	if runtime == nil || runtime.sqlDB == nil || ctx == nil || !runtime.scope.ownsSessionKey(key) {
		return aioCoreE2ESessionBusinessRow{}, false, errAIOCoreE2ERuntime
	}
	var row aioCoreE2ESessionBusinessRow
	row.ref.Key = key
	err := runtime.sqlDB.QueryRowContext(ctx, `SELECT session_id, runtime_generation, state, upstream_shell_id, recovery_reason
FROM sandbox_runtime_sessions
WHERE deployment_id = ? AND provider_id = ? AND space_id = ? AND user_id = ? AND thread_id = ? AND profile = ?`,
		key.DeploymentID, key.ProviderID, key.SpaceID, key.UserID, key.ThreadID, key.Profile).
		Scan(&row.ref.SessionID, &row.ref.RuntimeGeneration, &row.state, &row.upstreamShellID, &row.recoveryReason)
	if errors.Is(err, sql.ErrNoRows) {
		return aioCoreE2ESessionBusinessRow{}, false, nil
	}
	if err != nil {
		return aioCoreE2ESessionBusinessRow{}, false, errAIOCoreE2ERuntime
	}
	switch row.state {
	case domainsandbox.SessionStateActive, domainsandbox.SessionStateRecovering, domainsandbox.SessionStateReleased, domainsandbox.SessionStateDestroyed:
	default:
		return aioCoreE2ESessionBusinessRow{}, false, errAIOCoreE2ERuntime
	}
	if normalized, normalizeErr := domainsandbox.NormalizeSessionRef(row.ref); normalizeErr != nil || normalized != row.ref {
		return aioCoreE2ESessionBusinessRow{}, false, errAIOCoreE2ERuntime
	}
	return row, true, nil
}

func (runtime *aioCoreE2ERuntime) reconcileSessionBusinessIdentity(ctx context.Context, fixture *aioCoreE2ESessionFixture) (bool, error) {
	if runtime == nil || fixture == nil || !runtime.scope.ownsSessionKey(fixture.ref.Key) {
		return false, errAIOCoreE2ERuntime
	}
	row, found, err := runtime.readSessionBusinessRow(ctx, fixture.ref.Key)
	if err != nil || !found {
		return found, err
	}
	if fixture.ref.SessionID != "" && fixture.ref.SessionID != row.ref.SessionID {
		return false, errAIOCoreE2ERuntime
	}
	if _, duplicate := runtime.scope.sessionIDs[row.ref.SessionID]; duplicate && fixture.ref.SessionID == "" {
		return false, errAIOCoreE2ERuntime
	}
	fixture.ref = row.ref
	runtime.scope.sessionIDs[row.ref.SessionID] = struct{}{}
	if !runtime.scope.ownsSession(fixture.ref) {
		return false, errAIOCoreE2ERuntime
	}
	return true, nil
}

func (runtime *aioCoreE2ERuntime) acquireSession(ctx context.Context, userID int64, threadID string) (*aioCoreE2ESessionFixture, error) {
	if runtime == nil || runtime.scope.providerID <= 0 || ctx == nil {
		return nil, errAIOCoreE2ERuntime
	}
	if _, ok := runtime.scope.userIDs[userID]; !ok {
		return nil, errAIOCoreE2ERuntime
	}
	if _, ok := runtime.scope.threadIDs[threadID]; !ok {
		return nil, errAIOCoreE2ERuntime
	}
	runID, err := runtime.nextIdentifier("run")
	if err != nil {
		return nil, errAIOCoreE2ERuntime
	}
	fixture, err := runtime.registerSessionBusinessIdentity(userID, threadID, runID)
	if err != nil {
		return nil, errAIOCoreE2ERuntime
	}
	operationID, err := runtime.nextIdentifier("acquire")
	if err != nil {
		return nil, errAIOCoreE2ERuntime
	}
	wire := sessionAcquireWire{
		Schema: sessionAcquireSchemaV1, DeploymentID: runtime.config.deploymentID,
		ProviderID: runtime.scope.providerID, Scope: sandboxidentity.ScopeAgent,
		SpaceID: runtime.config.spaceID, UserID: userID, ThreadID: threadID,
		RunID: runID, OperationID: operationID, Profile: string(domainsandbox.SessionProfileCore),
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, errAIOCoreE2ERuntime
	}
	claims := sandboxidentity.SessionRequest{
		DeploymentID: runtime.config.deploymentID, ProviderID: runtime.scope.providerID,
		Scope: sandboxidentity.ScopeAgent, SpaceID: runtime.config.spaceID, UserID: userID,
		ThreadID: threadID, RunID: runID, OperationID: operationID,
		Profile: string(domainsandbox.SessionProfileCore),
	}
	var projection SessionProjection
	requestErr := runtime.doSignedRequest(ctx, http.MethodPost, "/v1/sessions:acquire", body, claims, http.StatusOK, &projection)
	if requestErr != nil ||
		projection.Schema != sessionProjectionSchemaV1 || !validSessionProtocolIdentifier(projection.SessionID) ||
		projection.State != domainsandbox.SessionStateActive || projection.RuntimeGeneration == 0 || projection.Profile != domainsandbox.SessionProfileCore {
		readbackCtx, cancelReadback := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		_, _ = runtime.reconcileSessionBusinessIdentity(readbackCtx, fixture)
		cancelReadback()
		return nil, errAIOCoreE2ERuntime
	}
	if _, duplicate := runtime.scope.sessionIDs[projection.SessionID]; duplicate {
		return nil, errAIOCoreE2ERuntime
	}
	fixture.ref.SessionID = projection.SessionID
	fixture.ref.RuntimeGeneration = projection.RuntimeGeneration
	runtime.scope.sessionIDs[projection.SessionID] = struct{}{}
	if !runtime.scope.ownsSession(fixture.ref) {
		delete(runtime.scope.sessionIDs, projection.SessionID)
		return nil, errAIOCoreE2ERuntime
	}
	return fixture, nil
}

func (runtime *aioCoreE2ERuntime) sessionClaims(session *aioCoreE2ESessionFixture, operationID string) (sandboxidentity.SessionRequest, error) {
	if runtime == nil || session == nil || !runtime.scope.ownsSession(session.ref) || !validSessionProtocolIdentifier(operationID) {
		return sandboxidentity.SessionRequest{}, errAIOCoreE2ERuntime
	}
	return sandboxidentity.SessionRequest{
		DeploymentID: runtime.config.deploymentID, ProviderID: runtime.scope.providerID,
		Scope: sandboxidentity.ScopeAgent, SpaceID: session.ref.Key.SpaceID, UserID: session.ref.Key.UserID,
		ThreadID: session.ref.Key.ThreadID, RunID: session.runID, OperationID: operationID,
		Profile: string(domainsandbox.SessionProfileCore),
	}, nil
}

func (runtime *aioCoreE2ERuntime) prepareOperation(kind SessionOperationKind, payload SessionOperationPayload, deadline time.Time) (aioCoreE2EPreparedOperation, error) {
	operationID, err := runtime.nextIdentifier("operation")
	if err != nil || deadline.IsZero() {
		return aioCoreE2EPreparedOperation{}, errAIOCoreE2ERuntime
	}
	operation := aioCoreE2EPreparedOperation{operationID: operationID, kind: kind, deadline: deadline.UTC(), payload: payload}
	operation.body, err = json.Marshal(sessionOperationWire{
		Schema: sessionOperationSchemaV1, OperationID: operationID, Kind: kind,
		Deadline: operation.deadline.Format(time.RFC3339Nano), Payload: payload,
	})
	if err != nil {
		return aioCoreE2EPreparedOperation{}, errAIOCoreE2ERuntime
	}
	return operation, nil
}

func (runtime *aioCoreE2ERuntime) submitPreparedOperation(ctx context.Context, session *aioCoreE2ESessionFixture, operation aioCoreE2EPreparedOperation) (SessionOperationProjection, error) {
	claims, err := runtime.sessionClaims(session, operation.operationID)
	if err != nil {
		return SessionOperationProjection{}, errAIOCoreE2ERuntime
	}
	requestPath := "/v1/sessions/" + session.ref.SessionID + "/operations"
	var projection SessionOperationProjection
	if err := runtime.doSignedRequest(ctx, http.MethodPost, requestPath, operation.body, claims, http.StatusAccepted, &projection); err != nil ||
		validateAIOCoreE2EOperationProjection(projection, session.ref.SessionID, operation.operationID, operation.kind) != nil {
		return SessionOperationProjection{}, errAIOCoreE2ERuntime
	}
	return projection, nil
}

func validateAIOCoreE2EOperationProjection(projection SessionOperationProjection, sessionID, operationID string, kind SessionOperationKind) error {
	if projection.Schema != sessionOperationSchemaV1 || projection.SessionID != sessionID || projection.OperationID != operationID || projection.Kind != kind ||
		!validSessionOperationState(projection.State) || len(projection.ReasonCode) > 64 ||
		projection.ResultDigest != "" && !validSHA256Digest(projection.ResultDigest) {
		return errAIOCoreE2ERuntime
	}
	if len(projection.Result) != 0 && (projection.State != SessionOperationSucceeded || !sessionResultDigestMatches(projection.Result, projection.ResultDigest)) {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) getOperation(ctx context.Context, session *aioCoreE2ESessionFixture, operation aioCoreE2EPreparedOperation) (SessionOperationProjection, error) {
	claims, err := runtime.sessionClaims(session, operation.operationID)
	if err != nil {
		return SessionOperationProjection{}, errAIOCoreE2ERuntime
	}
	requestPath := "/v1/sessions/" + session.ref.SessionID + "/operations/" + operation.operationID
	var projection SessionOperationProjection
	if err := runtime.doSignedRequest(ctx, http.MethodGet, requestPath, nil, claims, http.StatusOK, &projection); err != nil ||
		validateAIOCoreE2EOperationProjection(projection, session.ref.SessionID, operation.operationID, operation.kind) != nil {
		return SessionOperationProjection{}, errAIOCoreE2ERuntime
	}
	return projection, nil
}

func (runtime *aioCoreE2ERuntime) cancelOperation(ctx context.Context, session *aioCoreE2ESessionFixture, operation aioCoreE2EPreparedOperation) (SessionOperationProjection, error) {
	claims, err := runtime.sessionClaims(session, operation.operationID)
	if err != nil {
		return SessionOperationProjection{}, errAIOCoreE2ERuntime
	}
	requestPath := "/v1/sessions/" + session.ref.SessionID + "/operations/" + operation.operationID + ":cancel"
	var projection SessionOperationProjection
	if err := runtime.doSignedRequest(ctx, http.MethodPost, requestPath, []byte("{}"), claims, http.StatusOK, &projection); err != nil ||
		validateAIOCoreE2EOperationProjection(projection, session.ref.SessionID, operation.operationID, operation.kind) != nil {
		return SessionOperationProjection{}, errAIOCoreE2ERuntime
	}
	return projection, nil
}

func (runtime *aioCoreE2ERuntime) getSession(ctx context.Context, session *aioCoreE2ESessionFixture) (SessionProjection, error) {
	operationID, err := runtime.nextIdentifier("get")
	if err != nil {
		return SessionProjection{}, errAIOCoreE2ERuntime
	}
	claims, err := runtime.sessionClaims(session, operationID)
	if err != nil {
		return SessionProjection{}, errAIOCoreE2ERuntime
	}
	var projection SessionProjection
	requestPath := "/v1/sessions/" + session.ref.SessionID
	if err := runtime.doSignedRequest(ctx, http.MethodGet, requestPath, nil, claims, http.StatusOK, &projection); err != nil ||
		projection.Schema != sessionProjectionSchemaV1 || projection.SessionID != session.ref.SessionID || projection.RuntimeGeneration == 0 ||
		projection.Profile != domainsandbox.SessionProfileCore {
		return SessionProjection{}, errAIOCoreE2ERuntime
	}
	return projection, nil
}

func (runtime *aioCoreE2ERuntime) recoverSession(ctx context.Context, session *aioCoreE2ESessionFixture) error {
	operationID, err := runtime.nextIdentifier("recover")
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	claims, err := runtime.sessionClaims(session, operationID)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	var projection SessionProjection
	requestPath := "/v1/sessions/" + session.ref.SessionID + ":recover"
	if err := runtime.doSignedRequest(ctx, http.MethodPost, requestPath, []byte("{}"), claims, http.StatusOK, &projection); err != nil ||
		projection.Schema != sessionProjectionSchemaV1 || projection.SessionID != session.ref.SessionID ||
		projection.State != domainsandbox.SessionStateActive || projection.RuntimeGeneration == 0 || projection.Profile != domainsandbox.SessionProfileCore {
		return errAIOCoreE2ERuntime
	}
	session.ref.RuntimeGeneration = projection.RuntimeGeneration
	if !runtime.scope.ownsSession(session.ref) {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) destroySession(ctx context.Context, session *aioCoreE2ESessionFixture) error {
	operationID, err := runtime.nextIdentifier("destroy")
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	claims, err := runtime.sessionClaims(session, operationID)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	requestPath := "/v1/sessions/" + session.ref.SessionID + ":destroy"
	if err := runtime.doSignedRequest(ctx, http.MethodPost, requestPath, []byte("{}"), claims, http.StatusNoContent, nil); err != nil {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) callControlHelper(parent context.Context, verb string) error {
	allowed := false
	for _, candidate := range aioCoreE2EControlVerbs() {
		if verb == candidate {
			allowed = true
			break
		}
	}
	if runtime == nil || parent == nil || !allowed {
		return errAIOCoreE2ERuntime
	}
	ctx, cancel := context.WithTimeout(parent, aioCoreE2EControlTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, runtime.config.controlHelper, verb)
	command.Stdin, command.Stdout, command.Stderr = nil, io.Discard, io.Discard
	command.Env = os.Environ()
	if err := command.Run(); err != nil {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) registerFixtureCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*aioCoreE2EControlTimeout)
		defer cancel()
		if runtime.cleanupFixtures(ctx) != nil {
			t.Error("AIO Core E2E exact fixture cleanup failed; sensitive details are intentionally redacted")
		}
	})
}

func safeAIOCoreE2EDestroyedBusinessRow(row aioCoreE2ESessionBusinessRow) bool {
	return row.state == domainsandbox.SessionStateDestroyed && !row.upstreamShellID.Valid && row.recoveryReason == ""
}

func (runtime *aioCoreE2ERuntime) reconcileProviderBusinessIdentity(ctx context.Context) (bool, error) {
	if runtime == nil || runtime.sqlDB == nil || ctx == nil || runtime.providerActor <= 0 ||
		runtime.scope.providerKey != runtime.config.prefix+"-provider" || !strings.HasPrefix(runtime.scope.providerKey, runtime.scope.prefix+"-") {
		return false, errAIOCoreE2ERuntime
	}
	var providerID, createdBy int64
	var providerKey string
	err := runtime.sqlDB.QueryRowContext(ctx, `SELECT id, provider_key, created_by
FROM sandbox_providers WHERE provider_key = ? AND created_by = ?`, runtime.scope.providerKey, runtime.providerActor).
		Scan(&providerID, &providerKey, &createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil || providerID <= 0 || providerKey != runtime.scope.providerKey || createdBy != runtime.providerActor {
		return false, errAIOCoreE2ERuntime
	}
	if runtime.scope.providerID != 0 && runtime.scope.providerID != providerID {
		return false, errAIOCoreE2ERuntime
	}
	runtime.scope.providerID = providerID
	if !runtime.scope.ownsProvider(providerID, providerKey) {
		return false, errAIOCoreE2ERuntime
	}
	return true, nil
}

func (runtime *aioCoreE2ERuntime) cleanSessionWorkspace(ctx context.Context, session *aioCoreE2ESessionFixture) error {
	if runtime == nil || !runtime.scope.ownsSession(session.ref) {
		return errAIOCoreE2ERuntime
	}
	projection, err := runtime.getSession(ctx, session)
	if err != nil || projection.State != domainsandbox.SessionStateActive || projection.RuntimeGeneration != session.ref.RuntimeGeneration {
		if runtime.recoverSession(ctx, session) != nil {
			return errAIOCoreE2ERuntime
		}
	}
	operation, err := runtime.prepareOperation(SessionOperationExec, SessionOperationPayload{
		Command: aioCoreE2EWorkspaceCleanupCommand, CWD: aioCoreE2EWorkspaceRoot, MaxOutputBytes: 1024,
	}, time.Now().UTC().Add(aioCoreE2EOperationTimeout))
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	result, err := runtime.submitPreparedOperation(ctx, session, operation)
	if err != nil || result.State != SessionOperationSucceeded {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) deleteDestroyedBusinessRow(ctx context.Context, session *aioCoreE2ESessionFixture) error {
	if runtime == nil || runtime.sqlDB == nil || session == nil || !runtime.scope.ownsSession(session.ref) {
		return errAIOCoreE2ERuntime
	}
	row, found, err := runtime.readSessionBusinessRow(ctx, session.ref.Key)
	if err != nil || !found || row.ref != session.ref || !safeAIOCoreE2EDestroyedBusinessRow(row) {
		return errAIOCoreE2ERuntime
	}
	key := session.ref.Key
	result, err := runtime.sqlDB.ExecContext(ctx, aioCoreE2EDeleteSessionsSQL,
		session.ref.SessionID, key.DeploymentID, key.ProviderID, key.SpaceID, key.UserID, key.ThreadID, key.Profile)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return errAIOCoreE2ERuntime
	}
	if _, found, err := runtime.readSessionBusinessRow(ctx, key); err != nil || found {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func (runtime *aioCoreE2ERuntime) cleanupFixtures(ctx context.Context) (cleanupErr error) {
	if runtime == nil || ctx == nil {
		return errAIOCoreE2ERuntime
	}
	defer func() {
		if runtime.closeDependencies() != nil {
			cleanupErr = errAIOCoreE2ERuntime
		}
	}()
	if runtime.aioStopped {
		if runtime.callControlHelper(ctx, "start-aio") != nil {
			return errAIOCoreE2ERuntime
		}
		runtime.aioStopped = false
	}
	if runtime.scope.providerKey == "" {
		runtime.scope.providerKey = runtime.config.prefix + "-provider"
	}
	providerFound, err := runtime.reconcileProviderBusinessIdentity(ctx)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	if !providerFound {
		if len(runtime.scope.sessionIDs) != 0 {
			return errAIOCoreE2ERuntime
		}
		return runtime.cleanupRedisNamespace(ctx)
	}
	ownedSessions := make([]*aioCoreE2ESessionFixture, 0, len(runtime.sessions))
	for _, session := range runtime.sessions {
		if session == nil || !runtime.scope.ownsSessionKey(session.ref.Key) {
			return errAIOCoreE2ERuntime
		}
		found, err := runtime.reconcileSessionBusinessIdentity(ctx, session)
		if err != nil {
			return errAIOCoreE2ERuntime
		}
		if found {
			ownedSessions = append(ownedSessions, session)
		}
	}
	if len(ownedSessions) > 0 && cleanAIOCoreE2EWorkspacesBeforeDestroy(ownedSessions,
		func(session *aioCoreE2ESessionFixture) error { return runtime.cleanSessionWorkspace(ctx, session) },
		func(session *aioCoreE2ESessionFixture) error { return runtime.destroySession(ctx, session) }) != nil {
		return errAIOCoreE2ERuntime
	}
	for _, session := range ownedSessions {
		if runtime.deleteDestroyedBusinessRow(ctx, session) != nil {
			return errAIOCoreE2ERuntime
		}
	}
	var providerSessions int
	if err := runtime.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_runtime_sessions WHERE provider_id = ?`, runtime.scope.providerID).Scan(&providerSessions); err != nil || providerSessions != 0 {
		return errAIOCoreE2ERuntime
	}
	if !runtime.scope.ownsProvider(runtime.scope.providerID, runtime.scope.providerKey) {
		return errAIOCoreE2ERuntime
	}
	result, err := runtime.sqlDB.ExecContext(ctx, aioCoreE2EDeleteProviderSQL, runtime.scope.providerID, runtime.scope.providerKey, runtime.providerActor)
	if err != nil {
		return errAIOCoreE2ERuntime
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return errAIOCoreE2ERuntime
	}
	var remaining int
	if err := runtime.sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_providers
WHERE id = ? AND provider_key = ? AND created_by = ?`, runtime.scope.providerID, runtime.scope.providerKey, runtime.providerActor).Scan(&remaining); err != nil || remaining != 0 {
		return errAIOCoreE2ERuntime
	}
	return runtime.cleanupRedisNamespace(ctx)
}

func (runtime *aioCoreE2ERuntime) cleanupRedisNamespace(ctx context.Context) error {
	if runtime == nil || runtime.redis == nil || ctx == nil || runtime.scope.deploymentID != runtime.scope.prefix || !validAIOCoreE2EPrefix(runtime.scope.prefix) {
		return errAIOCoreE2ERuntime
	}
	prefix := "coze:sandbox:core:" + redisHash(runtime.scope.deploymentID) + ":"
	scan := func(scanCtx context.Context, cursor uint64, pattern string, count int64) ([]string, uint64, error) {
		return runtime.redis.Scan(scanCtx, cursor, pattern, count).Result()
	}
	if err := scanAIOCoreE2ERedisCursorToZero(ctx, prefix, 128, scan, func(keys []string) error {
		if len(keys) > 0 && runtime.redis.Del(ctx, keys...).Err() != nil {
			return errAIOCoreE2ERuntime
		}
		return nil
	}); err != nil {
		return errAIOCoreE2ERuntime
	}
	return scanAIOCoreE2ERedisCursorToZero(ctx, prefix, 128, scan, func(keys []string) error {
		if len(keys) != 0 {
			return errAIOCoreE2ERuntime
		}
		return nil
	})
}

func (runtime *aioCoreE2ERuntime) closeDependencies() error {
	if runtime == nil {
		return errAIOCoreE2ERuntime
	}
	failed := false
	if runtime.httpClient != nil {
		if transport, ok := runtime.httpClient.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
		runtime.httpClient = nil
	}
	if runtime.redis != nil {
		if err := runtime.redis.Close(); err != nil {
			failed = true
		}
		runtime.redis = nil
	}
	if runtime.sqlDB != nil {
		if err := runtime.sqlDB.Close(); err != nil {
			failed = true
		}
		runtime.sqlDB = nil
	}
	if failed {
		return errAIOCoreE2ERuntime
	}
	return nil
}

func validAIOCoreE2EEnvironment() map[string]string {
	const mysqlDSN = "aio_e2e@tcp(mysql.dev.invalid:3306)/coze_dev?parseTime=true&loc=UTC"
	return map[string]string{
		aioCoreE2EEnableEnv:                               "1",
		"SANDBOX_AIO_CORE_E2E_PREFIX":                     "aio-e2e-0123456789abcdef",
		"SANDBOX_RUNNER_DEPLOYMENT_ID":                    "aio-e2e-0123456789abcdef",
		"SANDBOX_AIO_CORE_E2E_ISOLATED_DEPLOYMENT":        "ISOLATED_TEST_DEPLOYMENT_WITH_NO_LIVE_TRAFFIC",
		"SANDBOX_AIO_CORE_E2E_EXACT_CLEANUP":              "EXACT_PREFIX_ONLY_ON_EXISTING_DEV_DEPENDENCIES",
		"SANDBOX_AIO_CORE_E2E_DEV_DB_FIXTURES":            "EXACT_COMMIT_AND_CLEANUP_ON_ISOLATED_DEV_DATABASE",
		"SANDBOX_AIO_CORE_E2E_RUNNER_URL":                 "https://runner.e2e.invalid:9443",
		"SANDBOX_AIO_CORE_E2E_RUNNER_CA_FILE":             "/tmp/aio-e2e-ca.pem",
		"SANDBOX_AIO_CORE_E2E_CONTROL_HELPER":             "/tmp/aio-e2e-control-helper",
		"SANDBOX_AIO_CORE_E2E_CODE_SHA":                   strings.Repeat("a", 40),
		"SANDBOX_AIO_CORE_E2E_IMAGE_ID":                   "sha256:" + strings.Repeat("b", 64),
		"SANDBOX_AIO_CORE_E2E_SPACE_ID":                   "420001",
		"SANDBOX_AIO_CORE_E2E_USER_ID_A":                  "430001",
		"SANDBOX_AIO_CORE_E2E_USER_ID_B":                  "430002",
		"SANDBOX_RUNNER_AUTH_TOKEN":                       "runner-auth-token-0123456789",
		"SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON":         `{"active_key_id":"key-1","keys":{"key-1":"0123456789abcdef0123456789abcdef"}}`,
		"SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DSN":           mysqlDSN,
		"SANDBOX_RUNTIME_SESSION_DEV_MYSQL_DATABASE":      "coze_dev",
		"SANDBOX_RUNTIME_SESSION_DEV_MYSQL_ROLLBACK_ONLY": "ROLLBACK_ONLY_ON_EXISTING_DEV_DATABASE",
		"MYSQL_DSN":      mysqlDSN,
		"REDIS_ADDR":     "redis.dev.invalid:6379",
		"REDIS_PASSWORD": "redis-secret-value",
		"REDIS_DB":       "7",
	}
}

func mapAIOCoreE2EGetenv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}
