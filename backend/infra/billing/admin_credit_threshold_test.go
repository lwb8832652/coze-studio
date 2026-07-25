// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
)

func TestAdminCreditThresholdConfigDefaultOverrideDisableAndSubjectIsolation(t *testing.T) {
	repository, db := newAdminCreditThresholdTestRepository(t)
	ctx := context.Background()

	userDefault, err := repository.GetCreditThresholdConfig(
		ctx,
		domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 42},
	)
	require.NoError(t, err)
	require.True(t, userDefault.Inherited)
	require.Equal(t, int64(0), userDefault.SourceSubjectID)
	require.True(t, userDefault.Enabled)
	require.Equal(t, int64(500), userDefault.ThresholdMicros)
	require.Zero(t, userDefault.Version)
	require.Equal(t, int64(1), userDefault.SourceVersion)

	override, err := repository.SaveCreditThresholdConfig(ctx, SaveCreditThresholdConfigInput{
		Subject:              domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 42},
		Enabled:              true,
		ThresholdMicros:      200,
		RecoveryMarginMicros: 25,
		ExpectedVersion:      userDefault.Version,
	}, 9001)
	require.NoError(t, err)
	require.False(t, override.Inherited)
	require.Equal(t, int64(42), override.SourceSubjectID)
	require.Equal(t, int64(1), override.Version)
	require.Equal(t, override.Version, override.SourceVersion)

	otherUser, err := repository.GetCreditThresholdConfig(
		ctx,
		domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 43},
	)
	require.NoError(t, err)
	require.True(t, otherUser.Inherited)
	require.Equal(t, int64(500), otherUser.ThresholdMicros)

	workspace, err := repository.GetCreditThresholdConfig(
		ctx,
		domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 42},
	)
	require.NoError(t, err)
	require.True(t, workspace.Inherited)
	require.False(t, workspace.Enabled)

	workspaceDefault, err := repository.GetCreditThresholdConfig(
		ctx,
		domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 0},
	)
	require.NoError(t, err)
	updatedWorkspaceDefault, err := repository.SaveCreditThresholdConfig(
		ctx,
		SaveCreditThresholdConfigInput{
			Subject: domainbilling.Subject{
				Type: domainbilling.SubjectTypeWorkspace,
				ID:   0,
			},
			Enabled:              true,
			ThresholdMicros:      800,
			RecoveryMarginMicros: 80,
			ExpectedVersion:      workspaceDefault.Version,
		},
		9001,
	)
	require.NoError(t, err)
	require.Equal(t, int64(0), updatedWorkspaceDefault.SourceSubjectID)
	require.Equal(t, int64(2), updatedWorkspaceDefault.Version)
	workspaceAfterDefaultUpdate, err := repository.GetCreditThresholdConfig(
		ctx,
		domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 42},
	)
	require.NoError(t, err)
	require.True(t, workspaceAfterDefaultUpdate.Inherited)
	require.Equal(t, int64(800), workspaceAfterDefaultUpdate.ThresholdMicros)
	require.Zero(t, workspaceAfterDefaultUpdate.Version)
	require.Equal(t, int64(2), workspaceAfterDefaultUpdate.SourceVersion)

	disabled, err := repository.SaveCreditThresholdConfig(ctx, SaveCreditThresholdConfigInput{
		Subject:         domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 42},
		Enabled:         false,
		ExpectedVersion: override.Version,
	}, 9001)
	require.NoError(t, err)
	require.False(t, disabled.Enabled)
	require.Zero(t, disabled.ThresholdMicros)
	require.Zero(t, disabled.RecoveryMarginMicros)
	require.False(t, disabled.Inherited)

	reloaded, err := repository.GetCreditThresholdConfig(
		ctx,
		domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 42},
	)
	require.NoError(t, err)
	require.False(t, reloaded.Enabled)
	require.False(t, reloaded.Inherited)
	require.Equal(t, int64(2), reloaded.Version)

	var updatedBy int64
	require.NoError(t, db.Table("billing_credit_threshold_configs").
		Select("updated_by").
		Where("subject_type = ? AND subject_id = ?", string(domainbilling.SubjectTypeUser), 42).
		Scan(&updatedBy).Error)
	require.Equal(t, int64(9001), updatedBy)

	responseJSON, err := json.Marshal(reloaded)
	require.NoError(t, err)
	for _, forbidden := range []string{
		"actor",
		"recipient",
		"ledger",
		"metadata",
		"credential",
		"updated_by",
	} {
		require.NotContains(t, strings.ToLower(string(responseJSON)), forbidden)
	}
}

func TestAdminCreditThresholdConfigRejectsInvalidActorSubjectBoundsAndCAS(t *testing.T) {
	repository, _ := newAdminCreditThresholdTestRepository(t)
	ctx := context.Background()
	valid := SaveCreditThresholdConfigInput{
		Subject:              domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 44},
		Enabled:              true,
		ThresholdMicros:      500,
		RecoveryMarginMicros: 50,
		ExpectedVersion:      0,
	}

	testCases := []struct {
		name  string
		input SaveCreditThresholdConfigInput
		actor int64
	}{
		{name: "missing actor", input: valid, actor: 0},
		{name: "invalid subject type", input: func() SaveCreditThresholdConfigInput {
			input := valid
			input.Subject.Type = domainbilling.SubjectType("organization")
			return input
		}(), actor: 9001},
		{name: "negative subject", input: func() SaveCreditThresholdConfigInput {
			input := valid
			input.Subject.ID = -1
			return input
		}(), actor: 9001},
		{name: "threshold below safe minimum", input: func() SaveCreditThresholdConfigInput {
			input := valid
			input.ThresholdMicros = domainbilling.MinCreditThresholdMicros - 1
			return input
		}(), actor: 9001},
		{name: "threshold above safe maximum", input: func() SaveCreditThresholdConfigInput {
			input := valid
			input.ThresholdMicros = domainbilling.MaxCreditThresholdMicros + 1
			return input
		}(), actor: 9001},
		{name: "margin below safe minimum", input: func() SaveCreditThresholdConfigInput {
			input := valid
			input.RecoveryMarginMicros = domainbilling.MinCreditRecoveryMarginMicros - 1
			return input
		}(), actor: 9001},
		{name: "margin above safe maximum", input: func() SaveCreditThresholdConfigInput {
			input := valid
			input.RecoveryMarginMicros = domainbilling.MaxCreditRecoveryMarginMicros + 1
			return input
		}(), actor: 9001},
		{name: "disabled with threshold", input: func() SaveCreditThresholdConfigInput {
			input := valid
			input.Enabled = false
			return input
		}(), actor: 9001},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := repository.SaveCreditThresholdConfig(ctx, testCase.input, testCase.actor)
			require.ErrorIs(t, err, domainbilling.ErrInvalidInput)
		})
	}

	created, err := repository.SaveCreditThresholdConfig(ctx, valid, 9001)
	require.NoError(t, err)
	_, err = repository.SaveCreditThresholdConfig(ctx, valid, 9002)
	require.ErrorIs(t, err, domainbilling.ErrVersionConflict)
	valid.ExpectedVersion = created.Version + 1
	_, err = repository.SaveCreditThresholdConfig(ctx, valid, 9001)
	require.ErrorIs(t, err, domainbilling.ErrVersionConflict)
}

func TestCreditThresholdConfigWriteContentionMapsToVersionConflict(t *testing.T) {
	for _, code := range []uint16{1062, 1205, 1213} {
		mapped := normalizeCreditThresholdConfigWriteError(&mysqldriver.MySQLError{
			Number:  code,
			Message: "redacted database contention",
		})
		require.ErrorIs(t, mapped, domainbilling.ErrVersionConflict)
		require.NotContains(t, mapped.Error(), "redacted database contention")
	}
	mysqlStorageFailure := &mysqldriver.MySQLError{
		Number:  1406,
		Message: "data too long",
	}
	require.Same(
		t,
		mysqlStorageFailure,
		normalizeCreditThresholdConfigWriteError(mysqlStorageFailure),
	)
	require.NotErrorIs(
		t,
		normalizeCreditThresholdConfigWriteError(mysqlStorageFailure),
		domainbilling.ErrVersionConflict,
	)
	storageFailure := errors.New("storage failure")
	require.ErrorIs(
		t,
		normalizeCreditThresholdConfigWriteError(storageFailure),
		storageFailure,
	)
}

func newAdminCreditThresholdTestRepository(t *testing.T) (*AdminRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(
		sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.Exec(
		`CREATE TABLE billing_credit_threshold_configs (
			subject_type TEXT NOT NULL,
			subject_id INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			threshold_micros INTEGER NOT NULL DEFAULT 0,
			recovery_margin_micros INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			updated_by INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY(subject_type, subject_id)
		)`,
	).Error)
	now := time.Date(2026, time.July, 25, 9, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec(
		`INSERT INTO billing_credit_threshold_configs
		 (subject_type, subject_id, enabled, threshold_micros, recovery_margin_micros, version, updated_by, created_at, updated_at)
		 VALUES (?, 0, 1, 500, 50, 1, 9001, ?, ?),
		        (?, 0, 0, 0, 0, 1, 9001, ?, ?)`,
		string(domainbilling.SubjectTypeUser), now, now,
		string(domainbilling.SubjectTypeWorkspace), now, now,
	).Error)
	return NewAdminRepository(db, nil), db
}
