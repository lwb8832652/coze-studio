// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"math"
	"strings"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const aioRuntimeRestartRecoveryReason = "aio_runtime_restarted"

var (
	_ domainsandbox.RuntimeSessionRepository = (*MySQLRepository)(nil)
	_ domainsandbox.AIOGenerationRepository  = (*MySQLRepository)(nil)
)

func (r *MySQLRepository) AcquireRuntimeSession(ctx context.Context, input domainsandbox.AcquireRuntimeSessionInput) (domainsandbox.RuntimeSession, error) {
	normalized, err := domainsandbox.NormalizeAcquireRuntimeSessionInput(input)
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	release, err := r.acquireOperation()
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	var result domainsandbox.RuntimeSession
	err = db.Transaction(func(tx *gorm.DB) error {
		generation, err := findAIOGenerationSettings(tx, true)
		if err != nil {
			return err
		}
		if generation.AIORuntimeDeploymentID != normalized.Key.DeploymentID || generation.AIORuntimeSentinelID == "" || generation.AIORuntimeGeneration == 0 {
			return domainsandbox.ErrConfigurationInvalid
		}
		po, err := findRuntimeSessionByKey(tx, normalized.Key)
		if err == nil {
			if normalized.RuntimeGeneration != generation.AIORuntimeGeneration ||
				po.RuntimeGeneration != generation.AIORuntimeGeneration ||
				po.State == string(domainsandbox.SessionStateRecovering) ||
				po.State == string(domainsandbox.SessionStateDestroyed) {
				return domainsandbox.ErrVersionConflict
			}
			if po.State == string(domainsandbox.SessionStateReleased) {
				if po.Version == math.MaxUint64 {
					return domainsandbox.ErrVersionConflict
				}
				update := tx.Model(&runtimeSessionPO{}).Where("session_id = ? AND version = ?", po.SessionID, po.Version).
					UpdateColumns(map[string]any{
						"state":            string(domainsandbox.SessionStateActive),
						"recovery_reason":  "",
						"last_activity_at": normalized.Now,
						"expires_at":       normalized.ExpiresAt,
						"updated_at":       normalized.Now,
						"version":          po.Version + 1,
					})
				if update.Error != nil {
					return mapRuntimeSessionSchemaError(update.Error)
				}
				if update.RowsAffected != 1 {
					return domainsandbox.ErrVersionConflict
				}
				po, err = findRuntimeSessionByKey(tx, normalized.Key)
				if err != nil {
					return err
				}
			}
			result, err = runtimeSessionDomain(po)
			return err
		}
		if !errors.Is(err, domainsandbox.ErrSessionNotFound) {
			return err
		}
		if normalized.RuntimeGeneration != generation.AIORuntimeGeneration {
			return domainsandbox.ErrVersionConflict
		}
		po, err = newRuntimeSessionPO(normalized)
		if err != nil {
			return err
		}
		if err := tx.Create(po).Error; err != nil {
			if !isRuntimeSessionUniqueConflict(err) {
				return mapRuntimeSessionSchemaError(err)
			}
			po, err = findRuntimeSessionByKey(tx, normalized.Key)
			if err != nil {
				if errors.Is(err, domainsandbox.ErrSessionNotFound) {
					return domainsandbox.ErrVersionConflict
				}
				return err
			}
		}
		result, err = runtimeSessionDomain(po)
		return err
	})
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	return result, nil
}

func (r *MySQLRepository) GetRuntimeSession(ctx context.Context, ref domainsandbox.SessionRef) (domainsandbox.RuntimeSession, error) {
	normalized, err := domainsandbox.NormalizeSessionRef(ref)
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	release, err := r.acquireOperation()
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	po, err := findRuntimeSessionByRef(db, normalized, false, false)
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	return runtimeSessionDomain(po)
}

func (r *MySQLRepository) BindRuntimeSessionCAS(ctx context.Context, input domainsandbox.BindRuntimeSessionInput) (domainsandbox.RuntimeSession, error) {
	normalized, err := domainsandbox.NormalizeBindRuntimeSessionInput(input)
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	return r.updateRuntimeSessionCAS(ctx, normalized.Ref, normalized.ExpectedVersion, normalized.Ref.RuntimeGeneration, func(_ *gorm.DB, po *runtimeSessionPO, _ *schedulerSettingsPO) (map[string]any, error) {
		if po.State != string(domainsandbox.SessionStateActive) || po.UpstreamShellID != nil {
			return nil, domainsandbox.ErrVersionConflict
		}
		return map[string]any{
			"upstream_shell_id": normalized.UpstreamShellID,
			"last_activity_at":  normalized.Now,
			"expires_at":        normalized.ExpiresAt,
			"updated_at":        normalized.Now,
		}, nil
	})
}

func (r *MySQLRepository) TransitionRuntimeSessionCAS(ctx context.Context, input domainsandbox.TransitionRuntimeSessionInput) (domainsandbox.RuntimeSession, error) {
	normalized, err := domainsandbox.NormalizeTransitionRuntimeSessionInput(input)
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	requiredGeneration := uint64(0)
	if normalized.Action == domainsandbox.SessionActionRecover {
		requiredGeneration = normalized.NextRuntimeGeneration
	}
	return r.updateRuntimeSessionCAS(ctx, normalized.Ref, normalized.ExpectedVersion, requiredGeneration, func(_ *gorm.DB, po *runtimeSessionPO, generation *schedulerSettingsPO) (map[string]any, error) {
		nextState, changed, err := domainsandbox.TransitionSessionState(domainsandbox.SessionState(po.State), normalized.Action)
		if err != nil {
			return nil, err
		}
		if !changed {
			if normalized.Action == domainsandbox.SessionActionMarkRecovering && po.RecoveryReason != normalized.RecoveryReason {
				return nil, domainsandbox.ErrVersionConflict
			}
			if normalized.Action == domainsandbox.SessionActionRecover &&
				(po.RuntimeGeneration != normalized.NextRuntimeGeneration ||
					po.UpstreamShellID == nil || *po.UpstreamShellID != normalized.UpstreamShellID) {
				return nil, domainsandbox.ErrVersionConflict
			}
			return map[string]any{}, nil
		}
		updates := map[string]any{
			"state":            string(nextState),
			"last_activity_at": normalized.Now,
			"updated_at":       normalized.Now,
		}
		switch normalized.Action {
		case domainsandbox.SessionActionRelease, domainsandbox.SessionActionDestroy:
			updates["upstream_shell_id"] = nil
			updates["recovery_reason"] = ""
		case domainsandbox.SessionActionMarkRecovering:
			updates["upstream_shell_id"] = nil
			updates["recovery_reason"] = normalized.RecoveryReason
		case domainsandbox.SessionActionRecover:
			if generation == nil {
				return nil, domainsandbox.ErrConfigurationInvalid
			}
			if generation.AIORuntimeDeploymentID != normalized.Ref.Key.DeploymentID ||
				generation.AIORuntimeGeneration != normalized.NextRuntimeGeneration ||
				generation.AIORuntimeSentinelID == "" {
				return nil, domainsandbox.ErrVersionConflict
			}
			updates["runtime_generation"] = normalized.NextRuntimeGeneration
			updates["upstream_shell_id"] = normalized.UpstreamShellID
			updates["recovery_reason"] = ""
			updates["expires_at"] = normalized.ExpiresAt
		}
		return updates, nil
	})
}

func (r *MySQLRepository) updateRuntimeSessionCAS(
	ctx context.Context,
	ref domainsandbox.SessionRef,
	expectedVersion uint64,
	requiredGeneration uint64,
	buildUpdates func(*gorm.DB, *runtimeSessionPO, *schedulerSettingsPO) (map[string]any, error),
) (domainsandbox.RuntimeSession, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	var result domainsandbox.RuntimeSession
	err = db.Transaction(func(tx *gorm.DB) error {
		var generation *schedulerSettingsPO
		if requiredGeneration != 0 {
			generation, err = findAIOGenerationSettings(tx, true)
			if err != nil {
				return err
			}
			if generation.AIORuntimeDeploymentID != ref.Key.DeploymentID ||
				generation.AIORuntimeGeneration != requiredGeneration ||
				generation.AIORuntimeSentinelID == "" {
				return domainsandbox.ErrVersionConflict
			}
		}
		po, err := findRuntimeSessionByRef(tx, ref, true, false)
		if err != nil {
			return err
		}
		if po.Version != expectedVersion {
			return domainsandbox.ErrVersionConflict
		}
		updates, err := buildUpdates(tx, po, generation)
		if err != nil {
			return err
		}
		if len(updates) == 0 {
			result, err = runtimeSessionDomain(po)
			return err
		}
		if expectedVersion == math.MaxUint64 {
			return domainsandbox.ErrVersionConflict
		}
		updates["version"] = expectedVersion + 1
		update := tx.Model(&runtimeSessionPO{}).
			Where("session_id = ? AND version = ?", ref.SessionID, expectedVersion).
			UpdateColumns(updates)
		if update.Error != nil {
			return mapRuntimeSessionSchemaError(update.Error)
		}
		if update.RowsAffected != 1 {
			return domainsandbox.ErrVersionConflict
		}
		po, err = findRuntimeSessionByRef(tx, ref, false, false)
		if err != nil {
			if nextGeneration, changed := updates["runtime_generation"].(uint64); changed {
				lookupRef := ref
				lookupRef.RuntimeGeneration = nextGeneration
				po, err = findRuntimeSessionByRef(tx, lookupRef, false, false)
			}
		}
		if err != nil {
			return err
		}
		result, err = runtimeSessionDomain(po)
		return err
	})
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	return result, nil
}

func (r *MySQLRepository) ListRecoverableRuntimeSessions(ctx context.Context, input domainsandbox.ListRecoverableRuntimeSessionsInput) ([]domainsandbox.RuntimeSession, error) {
	normalized, err := domainsandbox.NormalizeListRecoverableRuntimeSessionsInput(input)
	if err != nil {
		return nil, err
	}
	release, err := r.acquireOperation()
	if err != nil {
		return nil, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []runtimeSessionPO
	if err := db.Where("deployment_id = ? AND runtime_generation < ? AND state <> ?", normalized.DeploymentID, normalized.BeforeGeneration, string(domainsandbox.SessionStateDestroyed)).
		Order("runtime_generation ASC, session_id ASC").Limit(normalized.Limit).Find(&rows).Error; err != nil {
		return nil, mapRuntimeSessionSchemaError(err)
	}
	result := make([]domainsandbox.RuntimeSession, 0, len(rows))
	for index := range rows {
		session, err := runtimeSessionDomain(&rows[index])
		if err != nil {
			return nil, err
		}
		result = append(result, session)
	}
	return result, nil
}

func (r *MySQLRepository) GetAIOGeneration(ctx context.Context, deploymentID string) (domainsandbox.AIOGenerationState, error) {
	normalized, err := domainsandbox.NormalizeAIOGenerationDeploymentID(deploymentID)
	if err != nil {
		return domainsandbox.AIOGenerationState{}, err
	}
	release, err := r.acquireOperation()
	if err != nil {
		return domainsandbox.AIOGenerationState{}, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return domainsandbox.AIOGenerationState{}, err
	}
	po, err := findAIOGenerationSettings(db, false)
	if err != nil {
		return domainsandbox.AIOGenerationState{}, err
	}
	return aioGenerationDomain(po, normalized)
}

func (r *MySQLRepository) CompareAndReplaceAIOSentinel(ctx context.Context, input domainsandbox.CompareAndReplaceAIOSentinelInput) (domainsandbox.AIOGenerationState, bool, error) {
	normalized, err := domainsandbox.NormalizeCompareAndReplaceAIOSentinelInput(input)
	if err != nil {
		return domainsandbox.AIOGenerationState{}, false, err
	}
	release, err := r.acquireOperation()
	if err != nil {
		return domainsandbox.AIOGenerationState{}, false, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return domainsandbox.AIOGenerationState{}, false, err
	}
	var result domainsandbox.AIOGenerationState
	replaced := false
	err = db.Transaction(func(tx *gorm.DB) error {
		po, err := findAIOGenerationSettings(tx, true)
		if err != nil {
			return err
		}
		if _, err := aioGenerationDomain(po, normalized.DeploymentID); err != nil {
			return err
		}
		if po.AIORuntimeDeploymentID != "" && po.AIORuntimeDeploymentID != normalized.DeploymentID {
			return domainsandbox.ErrConfigurationInvalid
		}
		if po.AIORuntimeSentinelID != normalized.ExpectedSentinelID {
			result, err = aioGenerationDomain(po, normalized.DeploymentID)
			return err
		}
		if po.AIORuntimeGeneration == math.MaxUint64 {
			return domainsandbox.ErrConfigurationInvalid
		}
		nextGeneration := po.AIORuntimeGeneration + 1
		update := tx.Model(&schedulerSettingsPO{}).Where("id = ? AND aio_runtime_generation = ? AND aio_runtime_sentinel_id = ?", 1, po.AIORuntimeGeneration, po.AIORuntimeSentinelID).
			UpdateColumns(map[string]any{
				"aio_runtime_deployment_id": normalized.DeploymentID,
				"aio_runtime_sentinel_id":   normalized.CandidateSentinelID,
				"aio_runtime_generation":    nextGeneration,
			})
		if update.Error != nil {
			return mapRuntimeSessionSchemaError(update.Error)
		}
		if update.RowsAffected != 1 {
			return domainsandbox.ErrVersionConflict
		}
		if po.AIORuntimeGeneration > 0 {
			if err := markOldRuntimeSessionsRecovering(tx, normalized.DeploymentID, nextGeneration); err != nil {
				return err
			}
		}
		result = domainsandbox.AIOGenerationState{DeploymentID: normalized.DeploymentID, Generation: nextGeneration, SentinelID: normalized.CandidateSentinelID}
		replaced = true
		return nil
	})
	if err != nil {
		return domainsandbox.AIOGenerationState{}, false, err
	}
	return result, replaced, nil
}

func markOldRuntimeSessionsRecovering(db *gorm.DB, deploymentID string, nextGeneration uint64) error {
	now := persistenceNow()
	update := db.Model(&runtimeSessionPO{}).
		Where("deployment_id = ? AND runtime_generation < ? AND state IN ?", deploymentID, nextGeneration, []string{string(domainsandbox.SessionStateActive), string(domainsandbox.SessionStateReleased)}).
		UpdateColumns(map[string]any{
			"state":             string(domainsandbox.SessionStateRecovering),
			"upstream_shell_id": nil,
			"recovery_reason":   aioRuntimeRestartRecoveryReason,
			"version":           gorm.Expr("version + 1"),
			"updated_at":        now,
		})
	return mapRuntimeSessionSchemaError(update.Error)
}

func findAIOGenerationSettings(db *gorm.DB, lock bool) (*schedulerSettingsPO, error) {
	var po schedulerSettingsPO
	query := db.Select("id", "aio_runtime_generation", "aio_runtime_deployment_id", "aio_runtime_sentinel_id").Where("id = ?", 1)
	if lock {
		query = withUpdateLock(query)
	}
	if err := query.First(&po).Error; err != nil {
		return nil, mapRuntimeSessionSchemaError(err)
	}
	return &po, nil
}

func aioGenerationDomain(po *schedulerSettingsPO, deploymentID string) (domainsandbox.AIOGenerationState, error) {
	if po == nil || po.ID != 1 || (po.AIORuntimeDeploymentID != "" && po.AIORuntimeDeploymentID != deploymentID) {
		return domainsandbox.AIOGenerationState{}, domainsandbox.ErrConfigurationInvalid
	}
	return domainsandbox.NormalizeAIOGenerationState(domainsandbox.AIOGenerationState{
		DeploymentID: po.AIORuntimeDeploymentID,
		Generation:   po.AIORuntimeGeneration,
		SentinelID:   po.AIORuntimeSentinelID,
	})
}

func newRuntimeSessionPO(input domainsandbox.AcquireRuntimeSessionInput) (*runtimeSessionPO, error) {
	providerID, err := positiveDomainInt64ToUint64(input.Key.ProviderID)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	spaceID, err := positiveDomainInt64ToUint64(input.Key.SpaceID)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	userID, err := positiveDomainInt64ToUint64(input.Key.UserID)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	return &runtimeSessionPO{
		SessionID: input.CandidateSessionID, DeploymentID: input.Key.DeploymentID,
		ProviderID: providerID, SpaceID: spaceID, UserID: userID, ThreadID: input.Key.ThreadID,
		Profile: string(input.Key.Profile), State: string(domainsandbox.SessionStateActive),
		RuntimeGeneration: input.RuntimeGeneration, RecoveryReason: "", Version: domainsandbox.InitialVersion,
		LastActivityAt: input.Now, ExpiresAt: input.ExpiresAt, CreatedAt: input.Now, UpdatedAt: input.Now,
	}, nil
}

func findRuntimeSessionByKey(db *gorm.DB, key domainsandbox.SessionKey) (*runtimeSessionPO, error) {
	providerID, _ := positiveDomainInt64ToUint64(key.ProviderID)
	spaceID, _ := positiveDomainInt64ToUint64(key.SpaceID)
	userID, _ := positiveDomainInt64ToUint64(key.UserID)
	var po runtimeSessionPO
	err := db.Where("deployment_id = ? AND provider_id = ? AND space_id = ? AND user_id = ? AND thread_id = ? AND profile = ?", key.DeploymentID, providerID, spaceID, userID, key.ThreadID, string(key.Profile)).First(&po).Error
	if err != nil {
		return nil, mapRuntimeSessionSchemaError(err)
	}
	return &po, nil
}

func findRuntimeSessionByRef(db *gorm.DB, ref domainsandbox.SessionRef, lock bool, ignoreGeneration bool) (*runtimeSessionPO, error) {
	providerID, _ := positiveDomainInt64ToUint64(ref.Key.ProviderID)
	spaceID, _ := positiveDomainInt64ToUint64(ref.Key.SpaceID)
	userID, _ := positiveDomainInt64ToUint64(ref.Key.UserID)
	var po runtimeSessionPO
	query := db.Where("session_id = ? AND deployment_id = ? AND provider_id = ? AND space_id = ? AND user_id = ? AND thread_id = ? AND profile = ?", ref.SessionID, ref.Key.DeploymentID, providerID, spaceID, userID, ref.Key.ThreadID, string(ref.Key.Profile))
	if !ignoreGeneration {
		query = query.Where("runtime_generation = ?", ref.RuntimeGeneration)
	}
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&po).Error; err != nil {
		return nil, mapRuntimeSessionSchemaError(err)
	}
	return &po, nil
}

func runtimeSessionDomain(po *runtimeSessionPO) (domainsandbox.RuntimeSession, error) {
	if po == nil || po.ProviderID == 0 || po.ProviderID > math.MaxInt64 || po.SpaceID == 0 || po.SpaceID > math.MaxInt64 || po.UserID == 0 || po.UserID > math.MaxInt64 {
		return domainsandbox.RuntimeSession{}, domainsandbox.ErrConfigurationInvalid
	}
	ref, err := domainsandbox.NormalizeSessionRef(domainsandbox.SessionRef{
		SessionID:         po.SessionID,
		Key:               domainsandbox.SessionKey{DeploymentID: po.DeploymentID, ProviderID: int64(po.ProviderID), SpaceID: int64(po.SpaceID), UserID: int64(po.UserID), ThreadID: po.ThreadID, Profile: domainsandbox.SessionProfile(po.Profile)},
		RuntimeGeneration: po.RuntimeGeneration,
	})
	if err != nil || po.Version < domainsandbox.InitialVersion || po.LastActivityAt.IsZero() || po.ExpiresAt.IsZero() || po.CreatedAt.IsZero() || po.UpdatedAt.IsZero() {
		return domainsandbox.RuntimeSession{}, domainsandbox.ErrConfigurationInvalid
	}
	if _, _, err := domainsandbox.TransitionSessionState(domainsandbox.SessionState(po.State), domainsandbox.SessionActionGet); err != nil {
		return domainsandbox.RuntimeSession{}, domainsandbox.ErrConfigurationInvalid
	}
	upstreamShellID := ""
	if po.UpstreamShellID != nil {
		upstreamShellID = *po.UpstreamShellID
	}
	state := domainsandbox.SessionState(po.State)
	switch state {
	case domainsandbox.SessionStateActive:
		if po.RecoveryReason != "" || (upstreamShellID != "" && (!validPersistedRuntimeIdentifier(upstreamShellID) || strings.HasPrefix(upstreamShellID, "newx-generation-"))) {
			return domainsandbox.RuntimeSession{}, domainsandbox.ErrConfigurationInvalid
		}
	case domainsandbox.SessionStateRecovering:
		if upstreamShellID != "" || len(po.RecoveryReason) > domainsandbox.MaxSessionRecoveryReasonBytes || !validPersistedRuntimeIdentifier(po.RecoveryReason) {
			return domainsandbox.RuntimeSession{}, domainsandbox.ErrConfigurationInvalid
		}
	case domainsandbox.SessionStateReleased, domainsandbox.SessionStateDestroyed:
		if upstreamShellID != "" || po.RecoveryReason != "" {
			return domainsandbox.RuntimeSession{}, domainsandbox.ErrConfigurationInvalid
		}
	}
	return domainsandbox.RuntimeSession{
		Ref: ref, State: state, UpstreamShellID: upstreamShellID,
		RecoveryReason: po.RecoveryReason, Version: po.Version, LastActivityAt: po.LastActivityAt.UTC(),
		ExpiresAt: po.ExpiresAt.UTC(), CreatedAt: po.CreatedAt.UTC(), UpdatedAt: po.UpdatedAt.UTC(),
	}, nil
}

func validPersistedRuntimeIdentifier(value string) bool {
	if value == "" || len(value) > domainsandbox.MaxSessionIdentifierBytes || strings.TrimSpace(value) != value || value == "." || value == ".." {
		return false
	}
	for index := range value {
		character := value[index]
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') && character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}

func isRuntimeSessionUniqueConflict(err error) bool {
	var mysqlError *mysqldriver.MySQLError
	if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") || strings.Contains(message, "duplicate entry")
}

func mapRuntimeSessionSchemaError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainsandbox.ErrSessionNotFound
	}
	var mysqlError *mysqldriver.MySQLError
	if (errors.As(err, &mysqlError) && mysqlError.Number == 1054) || strings.Contains(strings.ToLower(err.Error()), "no such column") || strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return domainsandbox.ErrConfigurationInvalid
	}
	return err
}
