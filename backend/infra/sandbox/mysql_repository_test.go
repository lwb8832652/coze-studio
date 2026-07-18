/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sandbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var sqliteSequence atomic.Uint64

func TestMySQLRepositoryCreateGetListAndCanonicalJSON(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	ctx := context.Background()

	first, err := repository.CreateProvider(ctx, validCreateProviderInput("provider-one"))
	if err != nil {
		t.Fatalf("CreateProvider(first) error = %v", err)
	}
	secondInput := validCreateProviderInput("provider-two")
	secondInput.Name = "Second Provider"
	second, err := repository.CreateProvider(ctx, secondInput)
	if err != nil {
		t.Fatalf("CreateProvider(second) error = %v", err)
	}

	if first.ID <= 0 || first.Version != domainsandbox.InitialVersion || first.Status != domainsandbox.ProviderStatusDisabled {
		t.Fatalf("created provider lifecycle fields = %#v", first)
	}
	if first.Health.Status != domainsandbox.HealthStatusUnknown || len(first.Health.Capabilities) != 0 {
		t.Fatalf("created provider health = %#v", first.Health)
	}
	if first.CreatedAt.IsZero() || !first.CreatedAt.Equal(first.UpdatedAt) || first.DeletedAt != nil {
		t.Fatalf("created provider timestamps = %#v", first)
	}

	var raw struct {
		ScopesJSON                 string `gorm:"column:scopes_json"`
		PolicyJSON                 string `gorm:"column:policy_json"`
		LastHealthCapabilitiesJSON string `gorm:"column:last_health_capabilities_json"`
	}
	if err := db.Raw(`SELECT scopes_json, policy_json, last_health_capabilities_json FROM sandbox_providers WHERE id = ?`, first.ID).
		Scan(&raw).Error; err != nil {
		t.Fatalf("read canonical JSON columns: %v", err)
	}
	if raw.ScopesJSON != `["agent","mcp_stdio"]` {
		t.Fatalf("scopes_json = %q", raw.ScopesJSON)
	}
	wantPolicy := `{"timeout_seconds":60,"memory_limit_mb":512,"cpu_limit":1,"max_output_bytes":65536,"max_concurrency":8,"allow_network":true,"network_allowlist":["api.example.com","z.example.com"],"node_modules_mode":"disabled"}`
	if raw.PolicyJSON != wantPolicy {
		t.Fatalf("policy_json = %q, want %q", raw.PolicyJSON, wantPolicy)
	}
	if raw.LastHealthCapabilitiesJSON != `[]` {
		t.Fatalf("last_health_capabilities_json = %q", raw.LastHealthCapabilitiesJSON)
	}

	byID, err := repository.GetProvider(ctx, first.ID)
	if err != nil || !reflect.DeepEqual(byID, first) {
		t.Fatalf("GetProvider() = %#v, %v; want %#v", byID, err, first)
	}
	byKey, err := repository.GetProviderByKey(ctx, first.ProviderKey)
	if err != nil || !reflect.DeepEqual(byKey, first) {
		t.Fatalf("GetProviderByKey() = %#v, %v; want %#v", byKey, err, first)
	}

	tiedCreatedAt := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	if err := db.Model(&providerPO{}).Where("id IN ?", []int64{first.ID, second.ID}).
		UpdateColumn("created_at", tiedCreatedAt).Error; err != nil {
		t.Fatalf("tie provider timestamps: %v", err)
	}
	providers, total, err := repository.ListProviders(ctx, domainsandbox.ProviderListRequest{
		Type:             domainsandbox.ProviderTypeRemoteHTTP,
		Status:           domainsandbox.ProviderStatusDisabled,
		AuthorizedScopes: []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeMCPStdio},
		Limit:            10,
	})
	if err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	if total != 2 || len(providers) != 2 || providers[0].ID != second.ID || providers[1].ID != first.ID {
		t.Fatalf("stable provider list = %#v, total %d", providerIDs(providers), total)
	}
	keywordItems, keywordTotal, err := repository.ListProviders(ctx, domainsandbox.ProviderListRequest{Keyword: "Second", Limit: 10})
	if err != nil || keywordTotal != 1 || len(keywordItems) != 1 || keywordItems[0].ID != second.ID {
		t.Fatalf("keyword provider list = %#v, total %d, err %v", providerIDs(keywordItems), keywordTotal, err)
	}

	byID.Scopes[0] = domainsandbox.ScopeAppDev
	byID.Policy.NetworkAllowlist[0] = "mutated.example.com"
	again, err := repository.GetProvider(ctx, first.ID)
	if err != nil {
		t.Fatalf("GetProvider(detached check) error = %v", err)
	}
	if again.Scopes[0] != domainsandbox.ScopeAgent || again.Policy.NetworkAllowlist[0] != "api.example.com" {
		t.Fatalf("repository returned aliased provider data: %#v", again)
	}
}

func TestMySQLRepositoryListRequiresProviderScopesToBeAllowedSubset(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	ctx := context.Background()
	systemScope := domainsandbox.ScopeAgent
	workspaceScope := domainsandbox.ScopeAppDev

	create := func(providerKey string, scopes []domainsandbox.Scope) *domainsandbox.Provider {
		t.Helper()
		input := validCreateProviderInput(providerKey)
		input.Scopes = scopes
		provider, err := repository.CreateProvider(ctx, input)
		if err != nil {
			t.Fatalf("CreateProvider(%s) error = %v", providerKey, err)
		}
		return provider
	}

	systemOlder := create("subset-system-older", []domainsandbox.Scope{systemScope})
	mixed := create("subset-system-workspace", []domainsandbox.Scope{systemScope, workspaceScope})
	workspaceOnly := create("subset-workspace-only", []domainsandbox.Scope{workspaceScope})
	systemNewest := create("subset-system-newest", []domainsandbox.Scope{systemScope})
	tiedCreatedAt := time.Date(2026, 7, 15, 11, 0, 0, 0, time.UTC)
	if err := db.Model(&providerPO{}).Where("id IN ?", []int64{
		systemOlder.ID, mixed.ID, workspaceOnly.ID, systemNewest.ID,
	}).UpdateColumn("created_at", tiedCreatedAt).Error; err != nil {
		t.Fatalf("tie provider timestamps: %v", err)
	}

	first, total, err := repository.ListProviders(ctx, domainsandbox.ProviderListRequest{
		AuthorizedScopes: []domainsandbox.Scope{systemScope},
		Limit:            1,
	})
	if err != nil || total != 2 || len(first) != 1 || first[0].ID != systemNewest.ID {
		t.Fatalf("first subset page = %#v, total %d, err %v", providerIDs(first), total, err)
	}
	second, total, err := repository.ListProviders(ctx, domainsandbox.ProviderListRequest{
		AuthorizedScopes: []domainsandbox.Scope{systemScope},
		Offset:           1,
		Limit:            1,
	})
	if err != nil || total != 2 || len(second) != 1 || second[0].ID != systemOlder.ID {
		t.Fatalf("second subset page = %#v, total %d, err %v", providerIDs(second), total, err)
	}
	singleScope, total, err := repository.ListProviders(ctx, domainsandbox.ProviderListRequest{
		Scope:            systemScope,
		AuthorizedScopes: []domainsandbox.Scope{systemScope},
		Limit:            10,
	})
	if err != nil || total != 2 || !reflect.DeepEqual(
		providerIDs(singleScope),
		[]int64{systemNewest.ID, systemOlder.ID},
	) {
		t.Fatalf("single scope subset page = %#v, total %d, err %v", providerIDs(singleScope), total, err)
	}
}

func TestMySQLRepositoryListCombinesRequestedAndAuthorizedScopes(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	ctx := context.Background()
	create := func(providerKey string, scopes []domainsandbox.Scope) *domainsandbox.Provider {
		t.Helper()
		input := validCreateProviderInput(providerKey)
		input.Scopes = scopes
		provider, err := repository.CreateProvider(ctx, input)
		if err != nil {
			t.Fatalf("CreateProvider(%s) error = %v", providerKey, err)
		}
		return provider
	}

	agentOnly := create("combined-agent-only", []domainsandbox.Scope{domainsandbox.ScopeAgent})
	agentAppDev := create("combined-agent-appdev", []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeAppDev})
	appDevOnly := create("combined-appdev-only", []domainsandbox.Scope{domainsandbox.ScopeAppDev})
	agentUnauthorized := create("combined-agent-unauthorized", []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeMCPStdio})
	tiedCreatedAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	if err := db.Model(&providerPO{}).Where("id IN ?", []int64{
		agentOnly.ID, agentAppDev.ID, appDevOnly.ID, agentUnauthorized.ID,
	}).UpdateColumn("created_at", tiedCreatedAt).Error; err != nil {
		t.Fatalf("tie provider timestamps: %v", err)
	}

	filter := domainsandbox.ProviderListRequest{
		Scope:            domainsandbox.ScopeAgent,
		AuthorizedScopes: []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeAppDev},
		Limit:            1,
	}
	first, total, err := repository.ListProviders(ctx, filter)
	if err != nil || total != 2 || len(first) != 1 || first[0].ID != agentAppDev.ID {
		t.Fatalf("first combined scope page = %#v, total %d, err %v", providerIDs(first), total, err)
	}
	filter.Offset = 1
	second, total, err := repository.ListProviders(ctx, filter)
	if err != nil || total != 2 || len(second) != 1 || second[0].ID != agentOnly.ID {
		t.Fatalf("second combined scope page = %#v, total %d, err %v", providerIDs(second), total, err)
	}
	withoutRequestedScope, total, err := repository.ListProviders(ctx, domainsandbox.ProviderListRequest{
		AuthorizedScopes: []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeAppDev},
		Limit:            10,
	})
	if err != nil || total != 3 || !reflect.DeepEqual(
		providerIDs(withoutRequestedScope),
		[]int64{appDevOnly.ID, agentAppDev.ID, agentOnly.ID},
	) {
		t.Fatalf("authorized subset without requested scope = %#v, total %d, err %v", providerIDs(withoutRequestedScope), total, err)
	}
}

func TestMySQLRepositoryScopeSubsetSQLUsesAllowedJSONAsTarget(t *testing.T) {
	requestedCondition, requestedArguments, err := providerRequestedScopeSQL("mysql", domainsandbox.ScopeAgent)
	if err != nil {
		t.Fatalf("providerRequestedScopeSQL(mysql) error = %v", err)
	}
	if requestedCondition != "JSON_CONTAINS(scopes_json, JSON_QUOTE(?))" {
		t.Fatalf("MySQL requested scope condition = %q", requestedCondition)
	}
	if len(requestedArguments) != 1 || requestedArguments[0] != "agent" {
		t.Fatalf("MySQL requested scope arguments = %#v", requestedArguments)
	}

	condition, arguments, err := providerScopeSubsetSQL("mysql", []domainsandbox.Scope{domainsandbox.ScopeAgent})
	if err != nil {
		t.Fatalf("providerScopeSubsetSQL(mysql) error = %v", err)
	}
	if condition != "JSON_CONTAINS(?, scopes_json)" {
		t.Fatalf("MySQL subset condition = %q", condition)
	}
	if len(arguments) != 1 || arguments[0] != `["agent"]` {
		t.Fatalf("MySQL subset arguments = %#v", arguments)
	}
}

func TestMySQLRepositoryMapsDuplicateProviderKey(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	ctx := context.Background()

	if _, err := repository.CreateProvider(ctx, validCreateProviderInput("duplicate-provider")); err != nil {
		t.Fatalf("CreateProvider(first) error = %v", err)
	}
	_, err := repository.CreateProvider(ctx, validCreateProviderInput("duplicate-provider"))
	if !errors.Is(err, domainsandbox.ErrProviderAlreadyExists) {
		t.Fatalf("CreateProvider(duplicate) error = %v", err)
	}
	if err == nil || err.Error() != domainsandbox.ErrProviderAlreadyExists.Error() {
		t.Fatalf("duplicate error leaked driver details: %v", err)
	}

	var nullLegacyHashes int64
	if err := db.Model(&providerPO{}).Where("legacy_source_hash IS NULL").Count(&nullLegacyHashes).Error; err != nil {
		t.Fatalf("count NULL legacy hashes: %v", err)
	}
	if nullLegacyHashes != 1 {
		t.Fatalf("NULL legacy source hash count = %d", nullLegacyHashes)
	}
}

func TestMySQLRepositoryProviderCASAndSoftDelete(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	ctx := context.Background()

	created, err := repository.CreateProvider(ctx, validCreateProviderInput("cas-provider"))
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	update := validUpdateProviderInput(created.ID, created.Version)
	updated, err := repository.UpdateProvider(ctx, update)
	if err != nil {
		t.Fatalf("UpdateProvider() error = %v", err)
	}
	if updated.Version != 2 || updated.ProviderKey != created.ProviderKey || updated.Name != "Updated Provider" || updated.Policy.MaxConcurrency != 9 {
		t.Fatalf("updated provider = %#v", updated)
	}

	if _, err := repository.UpdateProvider(ctx, update); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("UpdateProvider(stale) error = %v", err)
	}
	missingUpdate := validUpdateProviderInput(created.ID+1000, 1)
	if _, err := repository.UpdateProvider(ctx, missingUpdate); !errors.Is(err, domainsandbox.ErrProviderNotFound) {
		t.Fatalf("UpdateProvider(missing) error = %v", err)
	}

	statusVersion, err := repository.UpdateProviderStatus(ctx, domainsandbox.UpdateProviderStatusInput{
		ProviderID: created.ID, ExpectedVersion: updated.Version,
		Status: domainsandbox.ProviderStatusEnabled, ActorUserID: 202,
	})
	if err != nil || statusVersion != 3 {
		t.Fatalf("UpdateProviderStatus() = %d, %v", statusVersion, err)
	}
	if _, err := repository.UpdateProviderStatus(ctx, domainsandbox.UpdateProviderStatusInput{
		ProviderID: created.ID, ExpectedVersion: updated.Version,
		Status: domainsandbox.ProviderStatusDisabled, ActorUserID: 202,
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("UpdateProviderStatus(stale) error = %v", err)
	}
	if _, err := repository.UpdateProviderStatus(ctx, domainsandbox.UpdateProviderStatusInput{
		ProviderID: created.ID + 1000, ExpectedVersion: 1,
		Status: domainsandbox.ProviderStatusEnabled, ActorUserID: 202,
	}); !errors.Is(err, domainsandbox.ErrProviderNotFound) {
		t.Fatalf("UpdateProviderStatus(missing) error = %v", err)
	}

	checkedAt := time.Date(2026, 7, 15, 9, 30, 0, 123000000, time.UTC)
	healthInput := domainsandbox.UpdateProviderHealthInput{
		ProviderID:      created.ID,
		ExpectedVersion: statusVersion,
		ActorUserID:     303,
		Health: domainsandbox.HealthSnapshot{
			Status:        domainsandbox.HealthStatusHealthy,
			Capabilities:  []domainsandbox.Scope{domainsandbox.ScopeMCPStdio, domainsandbox.ScopeAgent},
			Message:       "ready",
			LatencyMillis: 17,
			CheckedAt:     checkedAt,
		},
	}
	healthVersion, err := repository.UpdateProviderHealth(ctx, healthInput)
	if err != nil || healthVersion != 4 {
		t.Fatalf("UpdateProviderHealth() = %d, %v", healthVersion, err)
	}
	withHealth, err := repository.GetProvider(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetProvider(with health) error = %v", err)
	}
	wantCapabilities := []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeMCPStdio}
	if withHealth.Version != 4 || withHealth.Health.Status != domainsandbox.HealthStatusHealthy ||
		!reflect.DeepEqual(withHealth.Health.Capabilities, wantCapabilities) ||
		!withHealth.Health.CheckedAt.Equal(checkedAt) || withHealth.Health.LatencyMillis != 17 {
		t.Fatalf("persisted health = %#v", withHealth.Health)
	}
	withHealth.Health.Capabilities[0] = domainsandbox.ScopeAppDev
	again, err := repository.GetProvider(ctx, created.ID)
	if err != nil || again.Health.Capabilities[0] != domainsandbox.ScopeAgent {
		t.Fatalf("health capabilities were aliased: %#v, %v", again, err)
	}

	if _, err := repository.UpdateProviderHealth(ctx, healthInput); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("UpdateProviderHealth(stale) error = %v", err)
	}
	healthInput.ProviderID += 1000
	healthInput.ExpectedVersion = 1
	if _, err := repository.UpdateProviderHealth(ctx, healthInput); !errors.Is(err, domainsandbox.ErrProviderNotFound) {
		t.Fatalf("UpdateProviderHealth(missing) error = %v", err)
	}

	if _, err := repository.UpdateProviderStatus(ctx, domainsandbox.UpdateProviderStatusInput{
		ProviderID: created.ID, ExpectedVersion: healthVersion,
		Status: domainsandbox.ProviderStatus("invalid"), ActorUserID: 303,
	}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("UpdateProviderStatus(invalid) error = %v", err)
	}
	unchanged, err := repository.GetProvider(ctx, created.ID)
	if err != nil || unchanged.Version != healthVersion {
		t.Fatalf("invalid mutation changed provider: %#v, %v", unchanged, err)
	}

	if _, err := repository.DeleteProvider(ctx, domainsandbox.DeleteProviderInput{
		ProviderID: created.ID, ExpectedVersion: healthVersion - 1, ActorUserID: 404,
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("DeleteProvider(stale) error = %v", err)
	}
	if _, err := repository.DeleteProvider(ctx, domainsandbox.DeleteProviderInput{
		ProviderID: created.ID + 1000, ExpectedVersion: 1, ActorUserID: 404,
	}); !errors.Is(err, domainsandbox.ErrProviderNotFound) {
		t.Fatalf("DeleteProvider(missing) error = %v", err)
	}
	deletedVersion, err := repository.DeleteProvider(ctx, domainsandbox.DeleteProviderInput{
		ProviderID: created.ID, ExpectedVersion: healthVersion, ActorUserID: 404,
	})
	if err != nil || deletedVersion != 5 {
		t.Fatalf("DeleteProvider() = %d, %v", deletedVersion, err)
	}
	if _, err := repository.GetProvider(ctx, created.ID); !errors.Is(err, domainsandbox.ErrProviderNotFound) {
		t.Fatalf("GetProvider(deleted) error = %v", err)
	}
	items, total, err := repository.ListProviders(ctx, domainsandbox.ProviderListRequest{})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("ListProviders(after delete) = %#v, %d, %v", items, total, err)
	}
	var deletedRow struct {
		Version   uint64
		DeletedAt *time.Time
	}
	if err := db.Raw(`SELECT version, deleted_at FROM sandbox_providers WHERE id = ?`, created.ID).Scan(&deletedRow).Error; err != nil {
		t.Fatalf("read soft-deleted row: %v", err)
	}
	if deletedRow.Version != deletedVersion || deletedRow.DeletedAt == nil {
		t.Fatalf("soft-deleted row = %#v", deletedRow)
	}
}

func TestMySQLRepositoryDefaultsAreMonotonicAndProtectProviders(t *testing.T) {
	repository, _ := newSQLiteRepository(t)
	ctx := context.Background()

	first, err := repository.CreateProvider(ctx, validCreateProviderInput("default-one"))
	if err != nil {
		t.Fatalf("CreateProvider(first) error = %v", err)
	}
	second, err := repository.CreateProvider(ctx, validCreateProviderInput("default-two"))
	if err != nil {
		t.Fatalf("CreateProvider(second) error = %v", err)
	}
	if _, err := repository.GetProviderDefault(ctx, domainsandbox.ScopeAgent); !errors.Is(err, domainsandbox.ErrDefaultMissing) {
		t.Fatalf("GetProviderDefault(missing) error = %v", err)
	}
	if _, err := repository.SetProviderDefault(ctx, domainsandbox.SetProviderDefaultInput{
		Scope: domainsandbox.ScopeAgent, ProviderID: second.ID, ExpectedVersion: 2, ActorUserID: 501,
	}); !errors.Is(err, domainsandbox.ErrDefaultMissing) {
		t.Fatalf("SetProviderDefault(missing with positive version) error = %v", err)
	}

	initial, err := repository.SetProviderDefault(ctx, domainsandbox.SetProviderDefaultInput{
		Scope: domainsandbox.ScopeAgent, ProviderID: second.ID, ExpectedVersion: 0, ActorUserID: 501,
	})
	if err != nil || initial.Version != 1 || initial.ProviderID != second.ID {
		t.Fatalf("SetProviderDefault(initial) = %#v, %v", initial, err)
	}
	if _, err := repository.SetProviderDefault(ctx, domainsandbox.SetProviderDefaultInput{
		Scope: domainsandbox.ScopeAgent, ProviderID: first.ID, ExpectedVersion: 0, ActorUserID: 502,
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("SetProviderDefault(existing with zero version) error = %v", err)
	}

	if _, err := repository.DeleteProvider(ctx, domainsandbox.DeleteProviderInput{
		ProviderID: second.ID, ExpectedVersion: second.Version + 10, ActorUserID: 503,
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("DeleteProvider(stale referenced) error = %v", err)
	}
	if _, err := repository.DeleteProvider(ctx, domainsandbox.DeleteProviderInput{
		ProviderID: second.ID + 1000, ExpectedVersion: 1, ActorUserID: 503,
	}); !errors.Is(err, domainsandbox.ErrProviderNotFound) {
		t.Fatalf("DeleteProvider(missing before reference check) error = %v", err)
	}
	if _, err := repository.DeleteProvider(ctx, domainsandbox.DeleteProviderInput{
		ProviderID: second.ID, ExpectedVersion: second.Version, ActorUserID: 503,
	}); !errors.Is(err, domainsandbox.ErrProviderInUse) {
		t.Fatalf("DeleteProvider(referenced) error = %v", err)
	}

	updated, err := repository.SetProviderDefault(ctx, domainsandbox.SetProviderDefaultInput{
		Scope: domainsandbox.ScopeAgent, ProviderID: first.ID, ExpectedVersion: initial.Version, ActorUserID: 504,
	})
	if err != nil || updated.Version != 2 || updated.ProviderID != first.ID {
		t.Fatalf("SetProviderDefault(update) = %#v, %v", updated, err)
	}
	if !updated.CreatedAt.Equal(initial.CreatedAt) || updated.UpdatedAt.Before(initial.UpdatedAt) {
		t.Fatalf("default timestamps reset: initial %#v, updated %#v", initial, updated)
	}
	appdev, err := repository.SetProviderDefault(ctx, domainsandbox.SetProviderDefaultInput{
		Scope: domainsandbox.ScopeAppDev, ProviderID: first.ID, ExpectedVersion: 0, ActorUserID: 504,
	})
	if err != nil || appdev.Version != 1 {
		t.Fatalf("SetProviderDefault(appdev) = %#v, %v", appdev, err)
	}
	defaults, err := repository.ListProviderDefaults(ctx)
	if err != nil || len(defaults) != 2 || defaults[0].Scope != domainsandbox.ScopeAgent || defaults[1].Scope != domainsandbox.ScopeAppDev {
		t.Fatalf("ListProviderDefaults() = %#v, %v", defaults, err)
	}

	deletedVersion, err := repository.DeleteProvider(ctx, domainsandbox.DeleteProviderInput{
		ProviderID: second.ID, ExpectedVersion: second.Version, ActorUserID: 505,
	})
	if err != nil || deletedVersion != 2 {
		t.Fatalf("DeleteProvider(unreferenced) = %d, %v", deletedVersion, err)
	}
}

func TestMySQLRepositoryAuditIsAppendOnlyAndStable(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	ctx := context.Background()

	inputs := []domainsandbox.AppendProviderAuditEventInput{
		{ProviderID: 41, ActorUserID: 601, Action: "provider.create", Result: "success", RequestID: "request-1", Metadata: map[string]string{domainsandbox.AuditMetadataKeyVersion: "1", domainsandbox.AuditMetadataKeyScope: "agent"}},
		{ProviderID: 42, ActorUserID: 602, Action: "provider.update", Result: "success", RequestID: "request-2", Metadata: map[string]string{domainsandbox.AuditMetadataKeyVersion: "2"}},
		{ProviderID: 41, ActorUserID: 603, Action: "provider.update", Result: "denied", RequestID: "request-3", Metadata: map[string]string{domainsandbox.AuditMetadataKeyVersion: "3"}},
	}
	events := make([]*domainsandbox.ProviderAuditEvent, 0, len(inputs))
	for _, input := range inputs {
		event, err := repository.AppendProviderAuditEvent(ctx, input)
		if err != nil {
			t.Fatalf("AppendProviderAuditEvent(%s) error = %v", input.RequestID, err)
		}
		events = append(events, event)
	}
	tiedCreatedAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	if err := db.Model(&providerAuditEventPO{}).Where("event_id IN ?", []int64{events[0].ID, events[1].ID, events[2].ID}).
		UpdateColumn("created_at", tiedCreatedAt).Error; err != nil {
		t.Fatalf("tie audit timestamps: %v", err)
	}

	pageOne, total, err := repository.ListProviderAuditEvents(ctx, domainsandbox.ProviderAuditListRequest{Limit: 2})
	if err != nil || total != 3 || len(pageOne) != 2 || pageOne[0].ID != events[2].ID || pageOne[1].ID != events[1].ID {
		t.Fatalf("audit page one = %#v, total %d, err %v", auditIDs(pageOne), total, err)
	}
	pageTwo, total, err := repository.ListProviderAuditEvents(ctx, domainsandbox.ProviderAuditListRequest{Offset: 2, Limit: 2})
	if err != nil || total != 3 || len(pageTwo) != 1 || pageTwo[0].ID != events[0].ID {
		t.Fatalf("audit page two = %#v, total %d, err %v", auditIDs(pageTwo), total, err)
	}
	filtered, filteredTotal, err := repository.ListProviderAuditEvents(ctx, domainsandbox.ProviderAuditListRequest{
		ProviderID: 41, Action: "provider.update", Result: "denied", Limit: 10,
	})
	if err != nil || filteredTotal != 1 || len(filtered) != 1 || filtered[0].ID != events[2].ID {
		t.Fatalf("filtered audits = %#v, total %d, err %v", auditIDs(filtered), filteredTotal, err)
	}

	var metadataJSON string
	if err := db.Raw(`SELECT metadata_json FROM sandbox_provider_audit_events WHERE event_id = ?`, events[0].ID).
		Scan(&metadataJSON).Error; err != nil {
		t.Fatalf("read metadata_json: %v", err)
	}
	if metadataJSON != `{"scope":"agent","version":"1"}` {
		t.Fatalf("metadata_json = %q", metadataJSON)
	}
	pageTwo[0].Metadata[domainsandbox.AuditMetadataKeyVersion] = "mutated"
	again, _, err := repository.ListProviderAuditEvents(ctx, domainsandbox.ProviderAuditListRequest{Offset: 2, Limit: 1})
	if err != nil || again[0].Metadata[domainsandbox.AuditMetadataKeyVersion] != "1" {
		t.Fatalf("audit metadata was aliased: %#v, %v", again, err)
	}
}

func TestMySQLUnitOfWorkCommitRollbackAndExactlyOnce(t *testing.T) {
	repository, _ := newSQLiteRepository(t)
	ctx := context.Background()

	commitCalls := 0
	var committed *domainsandbox.Provider
	var escaped domainsandbox.TransactionRepositories
	err := repository.WithinTransaction(ctx, func(txCtx context.Context, repositories domainsandbox.TransactionRepositories) error {
		commitCalls++
		escaped = repositories
		var err error
		committed, err = repositories.Providers.CreateProvider(txCtx, validCreateProviderInput("uow-commit"))
		if err != nil {
			return err
		}
		if _, err = repositories.Defaults.SetProviderDefault(txCtx, domainsandbox.SetProviderDefaultInput{
			Scope: domainsandbox.ScopeAgent, ProviderID: committed.ID, ExpectedVersion: 0, ActorUserID: 701,
		}); err != nil {
			return err
		}
		_, err = repositories.Audits.AppendProviderAuditEvent(txCtx, domainsandbox.AppendProviderAuditEventInput{
			ProviderID: committed.ID, ActorUserID: 701, Action: "provider.create", Result: "success",
			RequestID: "uow-commit-request", Metadata: map[string]string{domainsandbox.AuditMetadataKeyVersion: "1"},
		})
		return err
	})
	if err != nil || commitCalls != 1 {
		t.Fatalf("WithinTransaction(commit) calls = %d, error = %v", commitCalls, err)
	}
	if _, err := repository.GetProvider(ctx, committed.ID); err != nil {
		t.Fatalf("committed provider missing: %v", err)
	}
	if _, err := repository.GetProviderDefault(ctx, domainsandbox.ScopeAgent); err != nil {
		t.Fatalf("committed default missing: %v", err)
	}
	if audits, total, err := repository.ListProviderAuditEvents(ctx, domainsandbox.ProviderAuditListRequest{ProviderID: committed.ID}); err != nil || total != 1 || len(audits) != 1 {
		t.Fatalf("committed audit = %#v, total %d, err %v", audits, total, err)
	}
	if _, err := escaped.Providers.GetProvider(ctx, committed.ID); !errors.Is(err, errTransactionRepositoriesClosed) {
		t.Fatalf("escaped transaction repository error = %v", err)
	}

	rollbackCause := errors.New("rollback requested")
	rollbackCalls := 0
	var rolledBackID int64
	err = repository.WithinTransaction(ctx, func(txCtx context.Context, repositories domainsandbox.TransactionRepositories) error {
		rollbackCalls++
		provider, err := repositories.Providers.CreateProvider(txCtx, validCreateProviderInput("uow-rollback"))
		if err != nil {
			return err
		}
		rolledBackID = provider.ID
		if _, err = repositories.Audits.AppendProviderAuditEvent(txCtx, domainsandbox.AppendProviderAuditEventInput{
			ProviderID: provider.ID, ActorUserID: 702, Action: "provider.create", Result: "success",
			RequestID: "uow-rollback-request", Metadata: map[string]string{domainsandbox.AuditMetadataKeyVersion: "1"},
		}); err != nil {
			return err
		}
		return rollbackCause
	})
	if !errors.Is(err, rollbackCause) || rollbackCalls != 1 {
		t.Fatalf("WithinTransaction(rollback) calls = %d, error = %v", rollbackCalls, err)
	}
	if _, err := repository.GetProviderByKey(ctx, "uow-rollback"); !errors.Is(err, domainsandbox.ErrProviderNotFound) {
		t.Fatalf("rolled-back provider error = %v", err)
	}
	if audits, total, err := repository.ListProviderAuditEvents(ctx, domainsandbox.ProviderAuditListRequest{ProviderID: rolledBackID}); err != nil || total != 0 || len(audits) != 0 {
		t.Fatalf("rolled-back audit = %#v, total %d, err %v", audits, total, err)
	}
}

func TestMySQLUnitOfWorkCloseWaitsForInFlightRepositoryOperation(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	testCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	provider, err := repository.CreateProvider(testCtx, validCreateProviderInput("guard-provider"))
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}

	operationStarted := make(chan struct{})
	attemptNested := make(chan struct{})
	callbackReturning := make(chan struct{})
	operationDone := make(chan error, 1)
	nestedDone := make(chan error, 1)
	var startOnce sync.Once
	var bound *MySQLRepository
	if err := db.Callback().Query().Before("gorm:query").Register("sandbox:test_guard_block", func(tx *gorm.DB) {
		if tx.Statement.Context.Value(transactionGuardContextKey{}) != true {
			return
		}
		startOnce.Do(func() { close(operationStarted) })
		select {
		case <-attemptNested:
			_, nestedErr := bound.GetProvider(testCtx, provider.ID)
			nestedDone <- nestedErr
		case <-testCtx.Done():
			nestedDone <- testCtx.Err()
		}
	}); err != nil {
		t.Fatalf("register blocking query callback: %v", err)
	}

	uowDone := make(chan error, 1)
	go func() {
		uowDone <- repository.WithinTransaction(testCtx, func(
			txCtx context.Context,
			repositories domainsandbox.TransactionRepositories,
		) error {
			bound = repositories.Providers.(*MySQLRepository)
			go func() {
				_, err := repositories.Providers.GetProvider(
					context.WithValue(txCtx, transactionGuardContextKey{}, true),
					provider.ID,
				)
				operationDone <- err
			}()
			select {
			case <-operationStarted:
			case <-testCtx.Done():
				return testCtx.Err()
			}
			close(callbackReturning)
			return nil
		})
	}()
	select {
	case <-callbackReturning:
	case err := <-uowDone:
		t.Fatalf("WithinTransaction returned before callback return: %v", err)
	case <-testCtx.Done():
		t.Fatal("timed out waiting for callback return")
	}

	closeStarted := make(chan struct{})
	go func() {
		bound.txState.mu.Lock()
		for !bound.txState.closing {
			bound.txState.cond.Wait()
		}
		bound.txState.mu.Unlock()
		close(closeStarted)
	}()
	select {
	case <-closeStarted:
	case err := <-uowDone:
		t.Fatalf("WithinTransaction returned before close waited: %v", err)
	case <-testCtx.Done():
		t.Fatal("timed out waiting for transaction close to start")
	}
	close(attemptNested)
	select {
	case err := <-nestedDone:
		if !errors.Is(err, errTransactionRepositoriesClosed) {
			t.Fatalf("nested operation during close error = %v", err)
		}
	case <-testCtx.Done():
		t.Fatal("nested operation deadlocked while transaction was closing")
	}
	select {
	case err := <-operationDone:
		if err != nil {
			t.Fatalf("in-flight repository operation error = %v", err)
		}
	case <-testCtx.Done():
		t.Fatal("in-flight operation did not finish after nested rejection")
	}
	select {
	case err := <-uowDone:
		if err != nil {
			t.Fatalf("WithinTransaction() error = %v", err)
		}
	case <-testCtx.Done():
		t.Fatal("transaction did not finish after in-flight operation")
	}
	if _, err := bound.GetProvider(testCtx, provider.ID); !errors.Is(err, errTransactionRepositoriesClosed) {
		t.Fatalf("post-close repository operation error = %v", err)
	}
}

func TestMySQLIntegrationGateFailsClosed(t *testing.T) {
	validDSN := "sandbox:sandbox@tcp(127.0.0.1:3306)/coze_sandbox_it_task2?parseTime=true"
	tests := []struct {
		name     string
		env      map[string]string
		wantSkip bool
		wantErr  bool
	}{
		{name: "missing DSN skips", env: map[string]string{}, wantSkip: true},
		{name: "malformed DSN fails", env: map[string]string{"SANDBOX_MYSQL_INTEGRATION_DSN": "%%%"}, wantErr: true},
		{name: "unsafe database fails", env: map[string]string{
			"SANDBOX_MYSQL_INTEGRATION_DSN":               validDSN[:strings.Index(validDSN, "coze_sandbox_it_task2")] + "shared_database",
			"SANDBOX_MYSQL_INTEGRATION_CONFIRM_EXCLUSIVE": "YES_I_OWN_THIS_SCHEMA",
			"SANDBOX_MYSQL_INTEGRATION_GUARD":             "guard-token",
		}, wantErr: true},
		{name: "missing exclusive confirmation fails", env: map[string]string{
			"SANDBOX_MYSQL_INTEGRATION_DSN":   validDSN,
			"SANDBOX_MYSQL_INTEGRATION_GUARD": "guard-token",
		}, wantErr: true},
		{name: "missing guard fails", env: map[string]string{
			"SANDBOX_MYSQL_INTEGRATION_DSN":               validDSN,
			"SANDBOX_MYSQL_INTEGRATION_CONFIRM_EXCLUSIVE": "YES_I_OWN_THIS_SCHEMA",
		}, wantErr: true},
		{name: "fully explicit gate passes static checks", env: map[string]string{
			"SANDBOX_MYSQL_INTEGRATION_DSN":               validDSN,
			"SANDBOX_MYSQL_INTEGRATION_CONFIRM_EXCLUSIVE": "YES_I_OWN_THIS_SCHEMA",
			"SANDBOX_MYSQL_INTEGRATION_GUARD":             "guard-token",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, skipReason, err := loadMySQLIntegrationGate(func(key string) string { return test.env[key] })
			if (skipReason != "") != test.wantSkip || (err != nil) != test.wantErr {
				t.Fatalf("loadMySQLIntegrationGate() skip = %q, err = %v", skipReason, err)
			}
		})
	}
}

func TestMySQLRepositoryUnsignedProjectionBoundaries(t *testing.T) {
	normalized, err := domainsandbox.NormalizeCreateProviderInput(validCreateProviderInput("unsigned-boundary"))
	if err != nil {
		t.Fatalf("NormalizeCreateProviderInput() error = %v", err)
	}
	po, err := newProviderPO(normalized, time.Now().UTC())
	if err != nil {
		t.Fatalf("newProviderPO() error = %v", err)
	}
	po.ID = uint64(math.MaxInt64)
	po.CreatedBy = uint64(math.MaxInt64)
	po.UpdatedBy = uint64(math.MaxInt64)
	provider, err := po.toDomain()
	if err != nil || provider.ID != math.MaxInt64 || provider.CreatedBy != math.MaxInt64 || provider.UpdatedBy != math.MaxInt64 {
		t.Fatalf("MaxInt64 provider projection = %#v, %v", provider, err)
	}
	po.ID = uint64(math.MaxInt64) + 1
	if _, err := po.toDomain(); !errors.Is(err, errInvalidPersistedNumericProjection) {
		t.Fatalf("overflow provider ID projection error = %v", err)
	}
	po.ID = 1
	po.MaxConcurrency = uint32(math.MaxInt32) + 1
	if _, err := po.toDomain(); !errors.Is(err, errInvalidPersistedNumericProjection) {
		t.Fatalf("overflow provider int projection error = %v", err)
	}
	if value, err := positiveDomainInt64ToUint64(math.MaxInt64); err != nil || value != uint64(math.MaxInt64) {
		t.Fatalf("positiveDomainInt64ToUint64(MaxInt64) = %d, %v", value, err)
	}
	if _, err := positiveDomainInt64ToUint64(0); !errors.Is(err, errInvalidPersistedNumericProjection) {
		t.Fatalf("positiveDomainInt64ToUint64(0) error = %v", err)
	}
	if message := errInvalidPersistedNumericProjection.Error(); len(message) > 128 {
		t.Fatalf("numeric projection error is not bounded: %d bytes", len(message))
	}
}

func TestMySQLRepositoryDuplicateClassificationUsesTypedErrorsOnly(t *testing.T) {
	for _, err := range []error{
		errors.New("unrelated duplicate entry warning"),
		errors.New("unrelated unique constraint warning"),
		&mysqldriver.MySQLError{Number: 1061, Message: "duplicate key name"},
	} {
		if isDuplicateDatabaseError(err) {
			t.Errorf("untyped/non-row duplicate error was classified: %T %v", err, err)
		}
	}
	if !isDuplicateDatabaseError(gorm.ErrDuplicatedKey) {
		t.Error("gorm.ErrDuplicatedKey was not classified")
	}
	if !isDuplicateDatabaseError(&mysqldriver.MySQLError{Number: 1062, Message: "typed duplicate"}) {
		t.Error("MySQL 1062 was not classified")
	}
}

func TestMySQLRepositoryListCountAndPageShareSnapshotTransaction(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	ctx := context.Background()
	provider, err := repository.CreateProvider(ctx, validCreateProviderInput("snapshot-provider"))
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	if _, err := repository.AppendProviderAuditEvent(ctx, domainsandbox.AppendProviderAuditEventInput{
		ProviderID: provider.ID, ActorUserID: 901, Action: "provider.create", Result: "success",
		RequestID: "snapshot-audit", Metadata: map[string]string{domainsandbox.AuditMetadataKeyVersion: "1"},
	}); err != nil {
		t.Fatalf("AppendProviderAuditEvent() error = %v", err)
	}

	type observation struct {
		table string
		pool  *sql.Tx
	}
	observations := make(chan observation, 8)
	if err := db.Callback().Query().Before("gorm:query").Register("sandbox:test_snapshot_transaction", func(tx *gorm.DB) {
		if tx.Statement.Context.Value(snapshotContextKey{}) != true {
			return
		}
		if tx.Statement.Table != "sandbox_providers" && tx.Statement.Table != "sandbox_provider_audit_events" {
			return
		}
		pool, _ := tx.Statement.ConnPool.(*sql.Tx)
		observations <- observation{table: tx.Statement.Table, pool: pool}
	}); err != nil {
		t.Fatalf("register snapshot callback: %v", err)
	}
	markedCtx := context.WithValue(ctx, snapshotContextKey{}, true)
	if _, _, err := repository.ListProviders(markedCtx, domainsandbox.ProviderListRequest{Limit: 10}); err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	if _, _, err := repository.ListProviderAuditEvents(markedCtx, domainsandbox.ProviderAuditListRequest{Limit: 10}); err != nil {
		t.Fatalf("ListProviderAuditEvents() error = %v", err)
	}
	close(observations)
	byTable := make(map[string][]*sql.Tx)
	for observed := range observations {
		byTable[observed.table] = append(byTable[observed.table], observed.pool)
	}
	for _, table := range []string{"sandbox_providers", "sandbox_provider_audit_events"} {
		pools := byTable[table]
		if len(pools) != 2 || pools[0] == nil || pools[1] == nil || pools[0] != pools[1] {
			t.Errorf("%s Count/Find transaction pools = %#v", table, pools)
		}
	}
	if err := repository.WithinTransaction(markedCtx, func(txCtx context.Context, repositories domainsandbox.TransactionRepositories) error {
		unmarkedCtx := context.WithValue(txCtx, snapshotContextKey{}, false)
		_, _, err := repositories.Providers.ListProviders(unmarkedCtx, domainsandbox.ProviderListRequest{Limit: 10})
		return err
	}); err != nil {
		t.Fatalf("nested snapshot list error = %v", err)
	}
}

func TestMySQLAuditFilterIndexesDeclared(t *testing.T) {
	actionField, _ := reflect.TypeOf(providerAuditEventPO{}).FieldByName("Action")
	resultField, _ := reflect.TypeOf(providerAuditEventPO{}).FieldByName("Result")
	createdField, _ := reflect.TypeOf(providerAuditEventPO{}).FieldByName("CreatedAt")
	eventIDField, _ := reflect.TypeOf(providerAuditEventPO{}).FieldByName("EventID")
	checks := []struct {
		name string
		tag  string
		want string
	}{
		{name: "action", tag: actionField.Tag.Get("gorm"), want: "index:idx_sandbox_audit_action_created_event,priority:1"},
		{name: "result", tag: resultField.Tag.Get("gorm"), want: "index:idx_sandbox_audit_result_created_event,priority:1"},
		{name: "action created", tag: createdField.Tag.Get("gorm"), want: "index:idx_sandbox_audit_action_created_event,priority:2"},
		{name: "result created", tag: createdField.Tag.Get("gorm"), want: "index:idx_sandbox_audit_result_created_event,priority:2"},
		{name: "action event", tag: eventIDField.Tag.Get("gorm"), want: "index:idx_sandbox_audit_action_created_event,priority:3"},
		{name: "result event", tag: eventIDField.Tag.Get("gorm"), want: "index:idx_sandbox_audit_result_created_event,priority:3"},
	}
	for _, check := range checks {
		if !strings.Contains(check.tag, check.want) {
			t.Errorf("%s tag = %q, want %q", check.name, check.tag, check.want)
		}
	}
}

func TestMySQLRepositoryStatusCASClassifiesBeforeConcurrentDelete(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	ctx := context.Background()
	provider, err := repository.CreateProvider(ctx, validCreateProviderInput("status-lock-provider"))
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	currentVersion, err := repository.UpdateProviderStatus(ctx, domainsandbox.UpdateProviderStatusInput{
		ProviderID: provider.ID, ExpectedVersion: provider.Version,
		Status: domainsandbox.ProviderStatusEnabled, ActorUserID: 810,
	})
	if err != nil {
		t.Fatalf("prepare provider version: %v", err)
	}

	phase := make(chan string, 1)
	releaseStatus := make(chan struct{})
	var phaseOnce sync.Once
	blockStatus := func(name string, tx *gorm.DB) {
		if tx.Statement.Context.Value(statusCASContextKey{}) != true || tx.Statement.Table != "sandbox_providers" {
			return
		}
		phaseOnce.Do(func() {
			phase <- name
			<-releaseStatus
		})
	}
	if err := db.Callback().Query().After("gorm:query").Register("sandbox:test_status_query", func(tx *gorm.DB) {
		blockStatus("query", tx)
	}); err != nil {
		t.Fatalf("register status query callback: %v", err)
	}
	if err := db.Callback().Update().After("gorm:update").Register("sandbox:test_status_update", func(tx *gorm.DB) {
		blockStatus("update", tx)
	}); err != nil {
		t.Fatalf("register status update callback: %v", err)
	}

	statusDone := make(chan error, 1)
	go func() {
		_, err := repository.UpdateProviderStatus(
			context.WithValue(ctx, statusCASContextKey{}, true),
			domainsandbox.UpdateProviderStatusInput{
				ProviderID: provider.ID, ExpectedVersion: provider.Version,
				Status: domainsandbox.ProviderStatusDisabled, ActorUserID: 811,
			},
		)
		statusDone <- err
	}()

	observedPhase := <-phase
	var deleteVersion uint64
	var deleteErr error
	if observedPhase == "update" {
		deleteVersion, deleteErr = repository.DeleteProvider(ctx, domainsandbox.DeleteProviderInput{
			ProviderID: provider.ID, ExpectedVersion: currentVersion, ActorUserID: 812,
		})
		close(releaseStatus)
	} else {
		close(releaseStatus)
	}
	statusErr := <-statusDone
	if observedPhase == "query" {
		deleteVersion, deleteErr = repository.DeleteProvider(ctx, domainsandbox.DeleteProviderInput{
			ProviderID: provider.ID, ExpectedVersion: currentVersion, ActorUserID: 812,
		})
	}
	if observedPhase != "query" {
		t.Errorf("status CAS classified after mutation attempt; first phase = %q", observedPhase)
	}
	if !errors.Is(statusErr, domainsandbox.ErrVersionConflict) {
		t.Errorf("UpdateProviderStatus(stale with concurrent delete) error = %v", statusErr)
	}
	if deleteErr != nil || deleteVersion != currentVersion+1 {
		t.Errorf("DeleteProvider() = %d, %v", deleteVersion, deleteErr)
	}
}

func TestMySQLRepositoryGetProviderForUpdateUsesBoundTransaction(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	ctx := context.Background()
	provider, err := repository.CreateProvider(ctx, validCreateProviderInput("provider-row-lock"))
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	type lockContextKey struct{}
	markedCtx := context.WithValue(ctx, lockContextKey{}, true)
	var observedPool *sql.Tx
	var observedLock bool
	if err := db.Callback().Query().Before("gorm:query").Register("sandbox:test_provider_for_update", func(tx *gorm.DB) {
		if tx.Statement.Context.Value(lockContextKey{}) != true || tx.Statement.Table != "sandbox_providers" {
			return
		}
		observedPool, _ = tx.Statement.ConnPool.(*sql.Tx)
		_, observedLock = tx.Statement.Clauses["FOR"]
	}); err != nil {
		t.Fatalf("register row-lock callback: %v", err)
	}
	if err := repository.WithinTransaction(markedCtx, func(txCtx context.Context, repositories domainsandbox.TransactionRepositories) error {
		locked, err := repositories.Providers.GetProviderForUpdate(txCtx, provider.ID)
		if err != nil {
			return err
		}
		if locked.ID != provider.ID || locked.Version != provider.Version {
			t.Fatalf("locked provider = %#v", locked)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithinTransaction(GetProviderForUpdate) error = %v", err)
	}
	if observedPool == nil || !observedLock {
		t.Fatalf("GetProviderForUpdate pool/lock = %#v/%t", observedPool, observedLock)
	}
}

func TestMySQLRepositoryResetHealthIsAtomicAndCASProtected(t *testing.T) {
	repository, _ := newSQLiteRepository(t)
	ctx := context.Background()
	provider, err := repository.CreateProvider(ctx, validCreateProviderInput("provider-reset-health"))
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	health := domainsandbox.HealthSnapshot{
		Status: domainsandbox.HealthStatusHealthy, Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent},
		ReasonCode: "OK", Message: "ready", LatencyMillis: 17,
		CheckedAt: time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC),
	}
	healthVersion, err := repository.UpdateProviderHealth(ctx, domainsandbox.UpdateProviderHealthInput{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Health: health, ActorUserID: 301,
	})
	if err != nil {
		t.Fatalf("UpdateProviderHealth() error = %v", err)
	}
	update := validUpdateProviderInput(provider.ID, healthVersion)
	update.ResetHealth = true
	updated, err := repository.UpdateProvider(ctx, update)
	if err != nil {
		t.Fatalf("UpdateProvider(reset health) error = %v", err)
	}
	if updated.Health.Status != domainsandbox.HealthStatusUnknown || len(updated.Health.Capabilities) != 0 ||
		updated.Health.ReasonCode != "" || updated.Health.Message != "" || updated.Health.LatencyMillis != 0 ||
		!updated.Health.CheckedAt.IsZero() {
		t.Fatalf("reset health = %#v", updated.Health)
	}

	newHealthVersion, err := repository.UpdateProviderHealth(ctx, domainsandbox.UpdateProviderHealthInput{
		ProviderID: provider.ID, ExpectedVersion: updated.Version, Health: health, ActorUserID: 302,
	})
	if err != nil {
		t.Fatalf("UpdateProviderHealth(new CAS) error = %v", err)
	}
	stale := validUpdateProviderInput(provider.ID, updated.Version)
	stale.ResetHealth = true
	if _, err := repository.UpdateProvider(ctx, stale); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("UpdateProvider(stale reset) error = %v", err)
	}
	stored, err := repository.GetProvider(ctx, provider.ID)
	if err != nil || stored.Version != newHealthVersion || !reflect.DeepEqual(stored.Health, health) {
		t.Fatalf("new health after stale reset = %#v, %v", stored, err)
	}
}

func TestMySQLRepositorySecretPersistenceRequiresCanonicalProjection(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	ctx := context.Background()
	unmasked := validCreateProviderInput("provider-unmasked-hint")
	unmasked.EndpointHint = "https://sandbox.example.com"
	if _, err := repository.CreateProvider(ctx, unmasked); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("CreateProvider(unmasked hint) error = %v", err)
	}
	wideFingerprint := validCreateProviderInput("provider-wide-fingerprint")
	wideFingerprint.CredentialFingerprint = strings.Repeat("a", 64)
	if _, err := repository.CreateProvider(ctx, wideFingerprint); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("CreateProvider(wide fingerprint) error = %v", err)
	}

	provider, err := repository.CreateProvider(ctx, validCreateProviderInput("provider-corrupt-projection"))
	if err != nil {
		t.Fatalf("CreateProvider(valid) error = %v", err)
	}
	if err := db.Model(&providerPO{}).Where("id = ?", provider.ID).UpdateColumn("endpoint_hint", "https://sandbox.example.com").Error; err != nil {
		t.Fatalf("corrupt endpoint hint: %v", err)
	}
	if _, err := repository.GetProvider(ctx, provider.ID); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("GetProvider(unmasked hint) error = %v", err)
	}
	if err := db.Model(&providerPO{}).Where("id = ?", provider.ID).Updates(map[string]any{
		"endpoint_hint": "https://***.com", "credential_fingerprint": strings.Repeat("b", 64),
	}).Error; err != nil {
		t.Fatalf("corrupt fingerprint: %v", err)
	}
	if _, err := repository.GetProvider(ctx, provider.ID); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("GetProvider(wide fingerprint) error = %v", err)
	}
}

func TestMySQLRepositoryModelsDeclareUnsignedMySQLTypes(t *testing.T) {
	tests := []struct {
		model  any
		fields map[string]string
	}{
		{model: providerPO{}, fields: map[string]string{
			"ID": "bigint unsigned", "MaxConcurrency": "int unsigned", "LastHealthLatencyMS": "int unsigned",
			"Version": "bigint unsigned", "CreatedBy": "bigint unsigned", "UpdatedBy": "bigint unsigned",
		}},
		{model: providerDefaultPO{}, fields: map[string]string{
			"ProviderID": "bigint unsigned", "Version": "bigint unsigned", "UpdatedBy": "bigint unsigned",
		}},
		{model: providerAuditEventPO{}, fields: map[string]string{
			"EventID": "bigint unsigned", "ProviderID": "bigint unsigned", "ActorUserID": "bigint unsigned",
		}},
	}
	for _, test := range tests {
		modelType := reflect.TypeOf(test.model)
		for fieldName, mysqlType := range test.fields {
			field, ok := modelType.FieldByName(fieldName)
			if !ok {
				t.Fatalf("%s.%s is missing", modelType.Name(), fieldName)
			}
			tag := strings.ToLower(field.Tag.Get("gorm"))
			if !strings.Contains(tag, "type:"+mysqlType) {
				t.Errorf("%s.%s gorm tag = %q, want type:%s", modelType.Name(), fieldName, tag, mysqlType)
			}
		}
	}
}

func TestMySQLRepositorySecretWritesDoNotLeakThroughConfiguredLogger(t *testing.T) {
	recorder := newRecordingGORMLogger()
	repository, _ := newSQLiteRepositoryWithLogger(t, recorder)
	recorder.reset()
	ctx := context.Background()
	input := validCreateProviderInput("secret-logger-provider")
	input.EndpointSecret = "endpoint-secret-must-not-appear"
	input.CredentialSecret = "credential-secret-must-not-appear"
	provider, err := repository.CreateProvider(ctx, input)
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	update := validUpdateProviderInput(provider.ID, provider.Version)
	update.EndpointSecret = "updated-endpoint-secret-must-not-appear"
	update.CredentialSecret = "updated-credential-secret-must-not-appear"
	if _, err := repository.UpdateProvider(ctx, update); err != nil {
		t.Fatalf("UpdateProvider() error = %v", err)
	}
	if _, err := repository.GetProvider(ctx, provider.ID); err != nil {
		t.Fatalf("GetProvider() error = %v", err)
	}

	logs := recorder.String()
	for _, secret := range []string{
		input.EndpointSecret, input.CredentialSecret, update.EndpointSecret, update.CredentialSecret,
	} {
		if strings.Contains(logs, secret) {
			t.Errorf("configured GORM logger captured secret %q", secret)
		}
	}
	if !strings.Contains(strings.ToUpper(logs), "SELECT") || !strings.Contains(logs, "sandbox_providers") {
		t.Fatalf("non-secret repository queries were not recorded: %q", logs)
	}
}

func TestMySQLRepositoryRejectsMalformedPersistedJSON(t *testing.T) {
	repository, db := newSQLiteRepository(t)
	ctx := context.Background()
	tests := []struct {
		name   string
		column string
		value  string
	}{
		{name: "JSON null", column: "last_health_capabilities_json", value: "null"},
		{name: "wrong array shape", column: "scopes_json", value: `{}`},
		{name: "invalid scope semantics", column: "scopes_json", value: `["not-a-scope"]`},
		{name: "invalid policy semantics", column: "policy_json", value: `{}`},
		{name: "wrong metadata shape", column: "metadata_json", value: `[]`},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.column == "metadata_json" {
				po := &providerAuditEventPO{MetadataJSON: test.value}
				_, err := po.toDomain()
				assertSafePersistedJSONError(t, err)
				return
			}
			provider, err := repository.CreateProvider(ctx, validCreateProviderInput(fmt.Sprintf("malformed-json-%d", index)))
			if err != nil {
				t.Fatalf("CreateProvider() error = %v", err)
			}
			if err := db.Model(&providerPO{}).Where("id = ?", provider.ID).UpdateColumn(test.column, test.value).Error; err != nil {
				t.Fatalf("write malformed persisted JSON: %v", err)
			}
			_, err = repository.GetProvider(ctx, provider.ID)
			assertSafePersistedJSONError(t, err)
		})
	}
}

func newSQLiteRepository(t *testing.T) (*MySQLRepository, *gorm.DB) {
	return newSQLiteRepositoryWithLogger(t, logger.Default.LogMode(logger.Silent))
}

func newSQLiteRepositoryWithLogger(t *testing.T, configuredLogger logger.Interface) (*MySQLRepository, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:sandbox-repository-%d?mode=memory&cache=shared&_foreign_keys=on", sqliteSequence.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: configuredLogger})
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get SQLite sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := migrateSQLiteSandboxTestSchema(db); err != nil {
		t.Fatalf("create SQLite repository schema: %v", err)
	}
	return NewMySQLRepository(db), db
}

func validCreateProviderInput(providerKey string) domainsandbox.CreateProviderInput {
	return domainsandbox.CreateProviderInput{
		ProviderKey:           providerKey,
		Name:                  "Remote Provider",
		Type:                  domainsandbox.ProviderTypeRemoteHTTP,
		EndpointSecret:        "encrypted:endpoint",
		EndpointHint:          "https://***.com",
		CredentialSecret:      "encrypted:credential",
		CredentialFingerprint: strings.Repeat("a", 32),
		Scopes:                []domainsandbox.Scope{domainsandbox.ScopeMCPStdio, domainsandbox.ScopeAgent, domainsandbox.ScopeAgent},
		Policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds:   60,
			MemoryLimitMB:    512,
			CPULimit:         1,
			MaxOutputBytes:   64 * 1024,
			MaxConcurrency:   8,
			AllowNetwork:     true,
			NetworkAllowlist: []string{" Z.example.com ", "api.example.com"},
		},
		ActorUserID: 101,
	}
}

func validUpdateProviderInput(providerID int64, expectedVersion uint64) domainsandbox.UpdateProviderInput {
	return domainsandbox.UpdateProviderInput{
		ProviderID:            providerID,
		ExpectedVersion:       expectedVersion,
		Name:                  " Updated Provider ",
		Type:                  domainsandbox.ProviderTypeRemoteHTTP,
		EndpointSecret:        "encrypted:new-endpoint",
		EndpointHint:          " https://***.com ",
		CredentialSecret:      "encrypted:new-credential",
		CredentialFingerprint: strings.Repeat("b", 32),
		Scopes:                []domainsandbox.Scope{domainsandbox.ScopeMCPStdio, domainsandbox.ScopeAgent},
		Policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds:   90,
			MemoryLimitMB:    1024,
			CPULimit:         2,
			MaxOutputBytes:   128 * 1024,
			MaxConcurrency:   9,
			AllowNetwork:     true,
			NetworkAllowlist: []string{"worker.example.com"},
		},
		ActorUserID: 202,
	}
}

func providerIDs(providers []*domainsandbox.Provider) []int64 {
	ids := make([]int64, 0, len(providers))
	for _, provider := range providers {
		ids = append(ids, provider.ID)
	}
	return ids
}

func auditIDs(events []*domainsandbox.ProviderAuditEvent) []int64 {
	ids := make([]int64, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}

type transactionGuardContextKey struct{}

type statusCASContextKey struct{}

type snapshotContextKey struct{}

func TestMySQLRepositoryTransactionOptionsSelection(t *testing.T) {
	rootOptions, startRoot := consistentReadTransactionOptions(false)
	if !startRoot || rootOptions == nil {
		t.Fatal("root list must start a transaction")
	}
	if rootOptions.Isolation != sql.LevelRepeatableRead || !rootOptions.ReadOnly {
		t.Fatalf("root list options = %#v, want read-only repeatable-read", rootOptions)
	}

	boundOptions, startBound := consistentReadTransactionOptions(true)
	if startBound || boundOptions != nil {
		t.Fatalf("bound list options = %#v, start = %t; want outer transaction reuse", boundOptions, startBound)
	}

	writeOptions := unitOfWorkTransactionOptions()
	if writeOptions.Isolation != sql.LevelRepeatableRead || writeOptions.ReadOnly {
		t.Fatalf("UnitOfWork options = %#v, want writable repeatable-read", writeOptions)
	}
}

func TestMySQLRepositoryMaxConcurrencyPersistenceBoundaries(t *testing.T) {
	for _, writePath := range []string{"create", "update"} {
		t.Run(writePath+" accepts persistence maximum", func(t *testing.T) {
			got, err := providerMaxConcurrencyToPO(int(maxProviderConcurrencyPersistence))
			if err != nil || got != maxProviderConcurrencyPersistence {
				t.Fatalf("conversion = %d, %v; want %d", got, err, maxProviderConcurrencyPersistence)
			}
		})

		t.Run(writePath+" rejects above persistence maximum", func(t *testing.T) {
			_, err := providerMaxConcurrencyToPO(int(maxProviderConcurrencyPersistence) + 1)
			if !errors.Is(err, domainsandbox.ErrInvalidInput) {
				t.Fatalf("conversion error = %v, want ErrInvalidInput", err)
			}
		})
	}

	t.Run("read accepts persistence maximum", func(t *testing.T) {
		got, err := providerMaxConcurrencyFromPO(maxProviderConcurrencyPersistence)
		if err != nil || got != int(maxProviderConcurrencyPersistence) {
			t.Fatalf("projection = %d, %v; want %d", got, err, maxProviderConcurrencyPersistence)
		}
	})

	t.Run("read rejects corrupt value above persistence maximum", func(t *testing.T) {
		_, err := providerMaxConcurrencyFromPO(maxProviderConcurrencyPersistence + 1)
		if !errors.Is(err, errInvalidPersistedNumericProjection) {
			t.Fatalf("projection error = %v, want bounded numeric projection error", err)
		}
	})
}

func TestMySQLSessionLockWaitTimeoutStatementIsBounded(t *testing.T) {
	if got := mysqlSessionLockWaitTimeoutStatement(1); got != "SET SESSION innodb_lock_wait_timeout = 1" {
		t.Fatalf("timeout statement = %q", got)
	}
	if got := mysqlSessionLockWaitTimeoutStatement(17); got != "SET SESSION innodb_lock_wait_timeout = 17" {
		t.Fatalf("restore statement = %q", got)
	}
}

type recordingGORMLogger struct {
	mu      sync.Mutex
	entries []string
}

func newRecordingGORMLogger() *recordingGORMLogger {
	return &recordingGORMLogger{}
}

func (l *recordingGORMLogger) LogMode(logger.LogLevel) logger.Interface {
	return l
}

func (l *recordingGORMLogger) Info(_ context.Context, message string, data ...interface{}) {
	l.record(fmt.Sprintf(message, data...))
}

func (l *recordingGORMLogger) Warn(_ context.Context, message string, data ...interface{}) {
	l.record(fmt.Sprintf(message, data...))
}

func (l *recordingGORMLogger) Error(_ context.Context, message string, data ...interface{}) {
	l.record(fmt.Sprintf(message, data...))
}

func (l *recordingGORMLogger) Trace(_ context.Context, _ time.Time, sql func() (string, int64), _ error) {
	statement, _ := sql()
	l.record(statement)
}

func (l *recordingGORMLogger) record(entry string) {
	l.mu.Lock()
	l.entries = append(l.entries, entry)
	l.mu.Unlock()
}

func (l *recordingGORMLogger) reset() {
	l.mu.Lock()
	l.entries = nil
	l.mu.Unlock()
}

func (l *recordingGORMLogger) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.entries, "\n")
}

func assertSafePersistedJSONError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("malformed persisted JSON was silently accepted")
	}
	message := err.Error()
	if len(message) > 256 {
		t.Fatalf("persisted JSON error is not bounded: %d bytes", len(message))
	}
	for _, sensitive := range []string{"encrypted:endpoint", "encrypted:credential", "must-not-appear"} {
		if strings.Contains(message, sensitive) {
			t.Fatalf("persisted JSON error leaked sensitive data: %q", message)
		}
	}
}

func migrateSQLiteSandboxTestSchema(db *gorm.DB) error {
	statements := []string{
		`CREATE TABLE sandbox_providers (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            provider_key TEXT NOT NULL UNIQUE,
            name TEXT NOT NULL,
            provider_type TEXT NOT NULL,
            endpoint_secret TEXT NULL,
            endpoint_hint TEXT NOT NULL DEFAULT '',
            credential_secret TEXT NULL,
            credential_fingerprint TEXT NOT NULL DEFAULT '',
            scopes_json JSON NOT NULL,
            policy_json JSON NOT NULL,
            max_concurrency INTEGER NOT NULL DEFAULT 1,
            status TEXT NOT NULL,
            health_status TEXT NOT NULL,
            last_health_capabilities_json JSON NOT NULL,
            last_health_code TEXT NOT NULL DEFAULT '',
            last_health_message TEXT NOT NULL DEFAULT '',
            last_health_latency_ms INTEGER NOT NULL DEFAULT 0,
            last_health_at DATETIME NULL,
            legacy_source_hash TEXT NULL UNIQUE,
            version INTEGER NOT NULL DEFAULT 1,
            created_by INTEGER NOT NULL,
            updated_by INTEGER NOT NULL,
            created_at DATETIME NOT NULL,
            updated_at DATETIME NOT NULL,
            deleted_at DATETIME NULL
        )`,
		`CREATE TABLE sandbox_provider_defaults (
            scope TEXT PRIMARY KEY,
            provider_id INTEGER NOT NULL,
            version INTEGER NOT NULL DEFAULT 1,
            updated_by INTEGER NOT NULL,
            created_at DATETIME NOT NULL,
            updated_at DATETIME NOT NULL
        )`,
		`CREATE TABLE sandbox_provider_audit_events (
            event_id INTEGER PRIMARY KEY AUTOINCREMENT,
            provider_id INTEGER NULL,
            actor_user_id INTEGER NOT NULL,
            action TEXT NOT NULL,
            result TEXT NOT NULL,
            request_id TEXT NOT NULL,
            metadata_json JSON NOT NULL,
            created_at DATETIME NOT NULL
        )`,
		`CREATE INDEX idx_sandbox_audit_action_created_event ON sandbox_provider_audit_events (action, created_at, event_id)`,
		`CREATE INDEX idx_sandbox_audit_result_created_event ON sandbox_provider_audit_events (result, created_at, event_id)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
