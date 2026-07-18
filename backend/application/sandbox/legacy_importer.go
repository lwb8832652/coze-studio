// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const (
	LegacyProviderName = "Legacy local sandbox"

	legacyProviderKey       = "legacy-local-sandbox"
	legacyImportActorUserID = int64(1)
	legacyAuditAction       = "provider.legacy_import"
	legacyAuditRequestID    = "legacy-sandbox-import"
)

var (
	ErrInvalidLegacySandboxConfig = errors.New("legacy sandbox configuration invalid")
	ErrLegacySandboxImportFailed  = errors.New("legacy sandbox import failed")
)

type LegacySandboxConfigLoader func(context.Context) (*adminconfig.SandboxConfig, error)

type LegacyImportProviderRepository interface {
	ListProviders(context.Context, domainsandbox.ProviderListRequest) ([]*domainsandbox.Provider, int64, error)
	CreateProvider(context.Context, domainsandbox.CreateProviderInput) (*domainsandbox.Provider, error)
	UpdateProviderStatus(context.Context, domainsandbox.UpdateProviderStatusInput) (uint64, error)
}

type LegacyImportDefaultRepository interface {
	SetProviderDefault(context.Context, domainsandbox.SetProviderDefaultInput) (*domainsandbox.ProviderDefault, error)
}

type LegacyImportAuditRepository interface {
	AppendProviderAuditEvent(context.Context, domainsandbox.AppendProviderAuditEventInput) (*domainsandbox.ProviderAuditEvent, error)
}

type LegacyImportRepositories struct {
	Providers LegacyImportProviderRepository
	Defaults  LegacyImportDefaultRepository
	Audits    LegacyImportAuditRepository
}

type LegacyImportStore interface {
	WithinProviderCreateTransaction(context.Context, func(context.Context, LegacyImportRepositories) error) error
}

type legacyUnitOfWorkStore struct {
	unitOfWork domainsandbox.ProviderCreateUnitOfWork
}

func NewLegacyImportStore(unitOfWork domainsandbox.ProviderCreateUnitOfWork) LegacyImportStore {
	if unitOfWork == nil {
		return nil
	}
	return &legacyUnitOfWorkStore{unitOfWork: unitOfWork}
}

func (s *legacyUnitOfWorkStore) WithinProviderCreateTransaction(
	ctx context.Context,
	callback func(context.Context, LegacyImportRepositories) error,
) error {
	if s == nil || s.unitOfWork == nil || callback == nil {
		return domainsandbox.ErrInvalidInput
	}
	return s.unitOfWork.WithinProviderCreateTransaction(ctx, func(txCtx context.Context, repositories domainsandbox.TransactionRepositories) error {
		return callback(txCtx, LegacyImportRepositories{
			Providers: repositories.Providers,
			Defaults:  repositories.Defaults,
			Audits:    repositories.Audits,
		})
	})
}

type LegacyImporter struct {
	loadConfig LegacySandboxConfigLoader
	store      LegacyImportStore
}

func NewLegacyImporter(loadConfig LegacySandboxConfigLoader, store LegacyImportStore) (*LegacyImporter, error) {
	if loadConfig == nil || store == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	return &LegacyImporter{loadConfig: loadConfig, store: store}, nil
}

func ImportLegacySandboxConfigIfEnabled(
	ctx context.Context,
	loadConfig LegacySandboxConfigLoader,
	store LegacyImportStore,
) error {
	if os.Getenv("SANDBOX_CONTROL_PLANE_ENABLED") != "true" {
		return nil
	}
	importer, err := NewLegacyImporter(loadConfig, store)
	if err != nil {
		return err
	}
	return importer.Import(ctx)
}

func (i *LegacyImporter) Import(ctx context.Context) error {
	if i == nil || i.loadConfig == nil || i.store == nil || ctx == nil {
		return ErrLegacySandboxImportFailed
	}
	err := i.store.WithinProviderCreateTransaction(ctx, func(txCtx context.Context, repositories LegacyImportRepositories) error {
		if repositories.Providers == nil || repositories.Audits == nil {
			return ErrLegacySandboxImportFailed
		}
		_, total, err := repositories.Providers.ListProviders(txCtx, domainsandbox.ProviderListRequest{Limit: 1})
		if err != nil {
			return ErrLegacySandboxImportFailed
		}
		if total > 0 {
			return nil
		}

		legacyConfig, err := i.loadConfig(txCtx)
		if err != nil {
			return ErrLegacySandboxImportFailed
		}
		if legacyConfig == nil {
			return nil
		}
		policy, sourceHash, err := canonicalLegacyRuntimePolicy(legacyConfig)
		if err != nil {
			return ErrInvalidLegacySandboxConfig
		}
		provider, err := repositories.Providers.CreateProvider(txCtx, domainsandbox.CreateProviderInput{
			ProviderKey:      legacyProviderKey,
			Name:             LegacyProviderName,
			Type:             domainsandbox.ProviderTypeLocalDebug,
			Scopes:           []domainsandbox.Scope{domainsandbox.ScopeAgent},
			Policy:           policy,
			LegacySourceHash: sourceHash,
			ActorUserID:      legacyImportActorUserID,
		})
		if errors.Is(err, domainsandbox.ErrProviderAlreadyExists) {
			return nil
		}
		if err != nil || provider == nil {
			return ErrLegacySandboxImportFailed
		}

		if legacyLocalDebugAllowed() {
			nextVersion, err := repositories.Providers.UpdateProviderStatus(txCtx, domainsandbox.UpdateProviderStatusInput{
				ProviderID: provider.ID, ExpectedVersion: provider.Version,
				Status: domainsandbox.ProviderStatusEnabled, ActorUserID: legacyImportActorUserID,
			})
			if err != nil {
				return ErrLegacySandboxImportFailed
			}
			provider.Status = domainsandbox.ProviderStatusEnabled
			provider.Version = nextVersion
		}

		_, err = repositories.Audits.AppendProviderAuditEvent(txCtx, domainsandbox.AppendProviderAuditEventInput{
			ProviderID: provider.ID, ActorUserID: legacyImportActorUserID,
			Action: legacyAuditAction, Result: "success", RequestID: legacyAuditRequestID,
			Metadata: map[string]string{
				domainsandbox.AuditMetadataKeyScope:         string(domainsandbox.ScopeAgent),
				domainsandbox.AuditMetadataKeyChangedFields: "policy,status",
				domainsandbox.AuditMetadataKeyVersion:       strconv.FormatUint(provider.Version, 10),
			},
		})
		if err != nil {
			return ErrLegacySandboxImportFailed
		}
		return nil
	})
	if err == nil || errors.Is(err, ErrInvalidLegacySandboxConfig) || errors.Is(err, ErrLegacySandboxImportFailed) {
		return err
	}
	return errors.Join(ErrLegacySandboxImportFailed, err)
}

const legacySourceHashVersion = "legacy-runtime-policy-hash-v1"

// legacySourceHashDocument is intentionally an allowlist. Filesystem paths,
// work directories, node-module paths, environment material, credentials, and
// raw runtime configuration must never influence or be recoverable from the
// migration fingerprint.
type legacySourceHashDocument struct {
	Version          string   `json:"version"`
	TimeoutSeconds   int      `json:"timeout_seconds"`
	MemoryLimitMB    int      `json:"memory_limit_mb"`
	CPULimit         float64  `json:"cpu_limit"`
	MaxOutputBytes   int64    `json:"max_output_bytes"`
	MaxConcurrency   int      `json:"max_concurrency"`
	AllowNetwork     bool     `json:"allow_network"`
	NetworkAllowlist []string `json:"network_allowlist"`
}

func canonicalLegacyRuntimePolicy(legacy *adminconfig.SandboxConfig) (domainsandbox.RuntimePolicy, string, error) {
	if legacy == nil || math.IsNaN(legacy.TimeoutSeconds) || math.IsInf(legacy.TimeoutSeconds, 0) ||
		math.Trunc(legacy.TimeoutSeconds) != legacy.TimeoutSeconds ||
		legacy.TimeoutSeconds < float64(domainsandbox.MinTimeoutSeconds) ||
		legacy.TimeoutSeconds > float64(domainsandbox.MaxTimeoutSeconds) ||
		legacy.MemoryLimitMb < int64(domainsandbox.MinMemoryLimitMB) ||
		legacy.MemoryLimitMb > int64(domainsandbox.MaxMemoryLimitMB) {
		return domainsandbox.RuntimePolicy{}, "", ErrInvalidLegacySandboxConfig
	}
	for _, raw := range []string{
		legacy.AllowEnv, legacy.AllowRead, legacy.AllowWrite, legacy.AllowRun,
		legacy.AllowNet, legacy.AllowFfi, legacy.NodeModulesDir,
	} {
		if strings.HasPrefix(strings.TrimSpace(raw), "env:") {
			return domainsandbox.RuntimePolicy{}, "", ErrInvalidLegacySandboxConfig
		}
	}
	rawLists := []string{
		legacy.AllowEnv, legacy.AllowRead, legacy.AllowWrite,
		legacy.AllowRun, legacy.AllowNet, legacy.AllowFfi,
	}
	lists := make([][]string, len(rawLists))
	for index, raw := range rawLists {
		var err error
		lists[index], err = splitLegacyPolicyList(raw)
		if err != nil {
			return domainsandbox.RuntimePolicy{}, "", ErrInvalidLegacySandboxConfig
		}
	}
	policy := domainsandbox.RuntimePolicy{
		TimeoutSeconds: int(legacy.TimeoutSeconds), MemoryLimitMB: int(legacy.MemoryLimitMb),
		CPULimit: 1, MaxOutputBytes: 4 * 1024 * 1024, MaxConcurrency: 1,
		AllowEnv: lists[0], AllowRead: lists[1], AllowWrite: lists[2], AllowRun: lists[3],
		NetworkAllowlist: lists[4], AllowFFI: lists[5],
		NodeModulesDir: strings.TrimSpace(legacy.NodeModulesDir),
	}
	policy.AllowNetwork = len(policy.NetworkAllowlist) > 0
	policy, err := domainsandbox.NormalizeRuntimePolicy(policy)
	if err != nil {
		return domainsandbox.RuntimePolicy{}, "", ErrInvalidLegacySandboxConfig
	}
	policy, err = domainsandbox.NormalizeLegacyRuntimePolicy(policy)
	if err != nil {
		return domainsandbox.RuntimePolicy{}, "", ErrInvalidLegacySandboxConfig
	}
	sourceHash, err := legacySourceHash(policy)
	if err != nil {
		return domainsandbox.RuntimePolicy{}, "", ErrInvalidLegacySandboxConfig
	}
	return policy, sourceHash, nil
}

func legacySourceHash(policy domainsandbox.RuntimePolicy) (string, error) {
	networkAllowlist := append([]string(nil), policy.NetworkAllowlist...)
	sort.Strings(networkAllowlist)
	document := legacySourceHashDocument{
		Version:          legacySourceHashVersion,
		TimeoutSeconds:   policy.TimeoutSeconds,
		MemoryLimitMB:    policy.MemoryLimitMB,
		CPULimit:         policy.CPULimit,
		MaxOutputBytes:   policy.MaxOutputBytes,
		MaxConcurrency:   policy.MaxConcurrency,
		AllowNetwork:     policy.AllowNetwork,
		NetworkAllowlist: networkAllowlist,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", ErrInvalidLegacySandboxConfig
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func splitLegacyPolicyList(value string) ([]string, error) {
	unique := make(map[string]struct{})
	totalBytes := 0
	for _, raw := range strings.Split(value, ",") {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if _, exists := unique[item]; exists {
			continue
		}
		if len(item) > domainsandbox.MaxLegacyPolicyItemBytes {
			return nil, ErrInvalidLegacySandboxConfig
		}
		totalBytes += len(item)
		if totalBytes > domainsandbox.MaxLegacyPolicyTotalBytes || len(unique) >= domainsandbox.MaxLegacyPolicyEntries {
			return nil, ErrInvalidLegacySandboxConfig
		}
		unique[item] = struct{}{}
	}
	items := make([]string, 0, len(unique))
	for item := range unique {
		items = append(items, item)
	}
	sort.Strings(items)
	return items, nil
}

func legacyLocalDebugAllowed() bool {
	return os.Getenv("APP_ENV") == "debug" && os.Getenv("APP_DEV_HOST_RUNTIME_ENABLED") == "true"
}
