// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"

	common "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop/common"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/repository"
)

var ErrAPPCodePluginPublishUnavailable = errors.New("code plugin batch publish outcome is unavailable")

func (p *pluginServiceImpl) prepareAPPCodePluginVersions(
	ctx context.Context,
	version string,
	draftPlugins []*entity.PluginInfo,
) ([]*entity.PluginInfo, []*repository.PreparedCodeVersion, error) {
	publishable := make([]*entity.PluginInfo, 0, len(draftPlugins))
	prepared := make([]*repository.PreparedCodeVersion, 0, len(draftPlugins))

	recoverable, ok := p.codeRepo.(repository.RecoverableCodePluginRepository)
	for _, draftPlugin := range draftPlugins {
		if draftPlugin.PluginType != common.PluginType_FUNC {
			publishable = append(publishable, draftPlugin)
			continue
		}
		if p.codeRepo == nil || !ok {
			return nil, nil, fmt.Errorf("recoverable code plugin repository is unavailable")
		}

		snapshot, err := recoverable.PrepareDebuggedVersion(ctx, draftPlugin.ID, version, draftPlugin.DeveloperID)
		if err != nil {
			if compensationErr := compensatePreparedCodeVersions(ctx, recoverable, prepared); compensationErr != nil {
				return nil, nil, fmt.Errorf("prepare code snapshot: %w; compensate prepared snapshots: %v", err, compensationErr)
			}
			return nil, nil, err
		}
		prepared = append(prepared, snapshot)
		if snapshot.State != repository.CodeVersionAlreadyPublished {
			publishable = append(publishable, draftPlugin)
		}
	}

	return publishable, prepared, nil
}

func (p *pluginServiceImpl) compensateAPPCodePluginVersions(
	ctx context.Context,
	prepared []*repository.PreparedCodeVersion,
) error {
	recoverable, ok := p.codeRepo.(repository.RecoverableCodePluginRepository)
	if !ok {
		return fmt.Errorf("recoverable code plugin repository is unavailable")
	}
	return compensatePreparedCodeVersions(ctx, recoverable, prepared)
}

func compensatePreparedCodeVersions(
	ctx context.Context,
	recoverable repository.RecoverableCodePluginRepository,
	prepared []*repository.PreparedCodeVersion,
) error {
	for index := len(prepared) - 1; index >= 0; index-- {
		if _, err := recoverable.CompensatePreparedVersion(ctx, prepared[index]); err != nil {
			return err
		}
	}
	return nil
}

func (p *pluginServiceImpl) ensureAPPCodePluginVersions(
	ctx context.Context,
	prepared []*repository.PreparedCodeVersion,
) error {
	recoverable, ok := p.codeRepo.(repository.RecoverableCodePluginRepository)
	if !ok {
		return fmt.Errorf("recoverable code plugin repository is unavailable")
	}
	for _, snapshot := range prepared {
		if err := recoverable.EnsurePublishedVersion(ctx, snapshot); err != nil {
			return err
		}
	}
	return nil
}

func (p *pluginServiceImpl) reconcileAmbiguousAPPCodePluginPublish(
	ctx context.Context,
	prepared []*repository.PreparedCodeVersion,
	publishErr error,
) (bool, error) {
	if len(prepared) == 0 {
		return false, fmt.Errorf(
			"%w: main publish failed without current code-plugin evidence: %v",
			ErrAPPCodePluginPublishUnavailable,
			publishErr,
		)
	}
	recoverable, ok := p.codeRepo.(repository.RecoverableCodePluginRepository)
	if !ok {
		return false, fmt.Errorf("%w: recoverable code plugin repository is unavailable", ErrAPPCodePluginPublishUnavailable)
	}

	published := make([]*repository.PreparedCodeVersion, 0, len(prepared))
	var firstUnknown error
	for _, snapshot := range prepared {
		isPublished, err := recoverable.CompensatePreparedVersion(ctx, snapshot)
		if err != nil {
			if firstUnknown == nil {
				firstUnknown = err
			}
			continue
		}
		if isPublished {
			published = append(published, snapshot)
		}
	}

	for _, snapshot := range published {
		if err := recoverable.EnsurePublishedVersion(ctx, snapshot); err != nil && firstUnknown == nil {
			firstUnknown = err
		}
	}

	if firstUnknown == nil && len(published) == len(prepared) {
		return true, nil
	}
	if firstUnknown != nil {
		return false, fmt.Errorf("%w: authoritative publish reconciliation failed: %v", ErrAPPCodePluginPublishUnavailable, firstUnknown)
	}
	return false, fmt.Errorf("%w: main publish failed: %v", ErrAPPCodePluginPublishUnavailable, publishErr)
}

func currentAPPCodePluginPublishEvidence(
	prepared []*repository.PreparedCodeVersion,
) []*repository.PreparedCodeVersion {
	evidence := make([]*repository.PreparedCodeVersion, 0, len(prepared))
	for _, snapshot := range prepared {
		if snapshot == nil || snapshot.State == repository.CodeVersionAlreadyPublished {
			continue
		}
		evidence = append(evidence, snapshot)
	}
	return evidence
}
