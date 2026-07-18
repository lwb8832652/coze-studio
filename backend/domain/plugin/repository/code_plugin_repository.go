// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"errors"

	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
)

var (
	ErrCodeDraftConflict      = errors.New("code plugin draft revision conflict")
	ErrCodeDraftNotFound      = errors.New("code plugin draft not found")
	ErrCodeDraftNotDebugged   = errors.New("code plugin current draft revision is not debugged")
	ErrCodeVersionExists      = errors.New("code plugin version already exists")
	ErrCodeMainVersionMissing = errors.New("main plugin version is not published")
	ErrCodeSpaceMismatch      = errors.New("code plugin space mismatch")
)

type CodeVersionPreparationState string

const (
	CodeVersionPrepared         CodeVersionPreparationState = "Prepared"
	CodeVersionAlreadyPublished CodeVersionPreparationState = "AlreadyPublished"
	CodeVersionRecovered        CodeVersionPreparationState = "Recovered"
)

type PreparedCodeVersion struct {
	Version *entity.CodeVersion
	Created bool
	State   CodeVersionPreparationState
}

type CodePluginRepository interface {
	GetDraft(ctx context.Context, pluginID int64) (*entity.CodeDraft, bool, error)
	SaveDraftCAS(ctx context.Context, draft *entity.CodeDraft, expectedRevision int64) (*entity.CodeDraft, error)
	MarkDebuggedCAS(ctx context.Context, pluginID, revision int64) error
	PublishDebuggedVersion(ctx context.Context, pluginID int64, version string, operatorID int64) error
	GetVersion(ctx context.Context, pluginID int64, version string) (*entity.CodeVersion, bool, error)
	CopyDraft(ctx context.Context, sourcePluginID, targetPluginID, targetSpaceID int64) error
	DeletePluginData(ctx context.Context, pluginID int64) error
}

// RecoverableCodePluginRepository exposes durable steps for the application
// publish Saga. plugin_version is the authoritative published marker.
type RecoverableCodePluginRepository interface {
	PrepareDebuggedVersion(ctx context.Context, pluginID int64, version string, operatorID int64) (*PreparedCodeVersion, error)
	CompensatePreparedVersion(ctx context.Context, prepared *PreparedCodeVersion) (published bool, err error)
	EnsurePublishedVersion(ctx context.Context, prepared *PreparedCodeVersion) error
}
