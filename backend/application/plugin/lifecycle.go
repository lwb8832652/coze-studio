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

package plugin

import (
	"context"
	"fmt"
	"time"

	pluginAPI "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop"
	common "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop/common"
	resCommon "github.com/coze-dev/coze-studio/backend/api/model/resource/common"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/crossdomain/plugin/model"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/dto"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/repository"
	searchEntity "github.com/coze-dev/coze-studio/backend/domain/search/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/errorx"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/ptr"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/types/errno"
)

func (p *PluginApplicationService) PublishPlugin(ctx context.Context, req *pluginAPI.PublishPluginRequest) (resp *pluginAPI.PublishPluginResponse, err error) {
	draftPlugin, err := p.validateDraftPluginAccess(ctx, req.PluginID)
	if err != nil {
		return nil, errorx.Wrapf(err, "validatePublishPluginRequest failed")
	}

	if draftPlugin.PluginType == common.PluginType_FUNC {
		if p.codeRepo == nil {
			return nil, codePluginUnavailable(fmt.Errorf("code plugin repository is unavailable"))
		}
		uid := ctxutil.GetUIDFromCtx(ctx)
		if uid == nil {
			return nil, errorx.New(errno.ErrPluginPermissionCode, errorx.KV(errno.PluginMsgKey, "session is required"))
		}
		recoverable, ok := p.codeRepo.(repository.RecoverableCodePluginRepository)
		if !ok {
			return nil, codePluginUnavailable(fmt.Errorf("recoverable code plugin repository is unavailable"))
		}
		prepared, prepareErr := recoverable.PrepareDebuggedVersion(ctx, draftPlugin.ID, req.VersionName, *uid)
		if prepareErr != nil {
			return nil, classifyCodePluginRepositoryError(prepareErr)
		}
		if prepared.State != repository.CodeVersionAlreadyPublished {
			err = p.DomainSVC.PublishPlugin(ctx, &model.PublishPluginRequest{
				PluginID:    req.PluginID,
				Version:     req.VersionName,
				VersionDesc: req.VersionDesc,
			})
		}
		if err != nil {
			published, compensationErr := recoverable.CompensatePreparedVersion(ctx, prepared)
			recovered := false
			var ensureErr error
			if published {
				if ensureErr = recoverable.EnsurePublishedVersion(ctx, prepared); ensureErr == nil {
					recovered = true
				}
			}
			if !recovered && compensationErr != nil {
				logs.CtxErrorf(ctx, "compensate code plugin publish failed, pluginID=%d, version=%s, err=%v", draftPlugin.ID, req.VersionName, compensationErr)
				return nil, codePluginUnavailable(fmt.Errorf("code plugin publish compensation failed"))
			}
			if !recovered && ensureErr != nil {
				return nil, codePluginUnavailable(fmt.Errorf("ensure published code snapshot failed: %w", ensureErr))
			}
			if !recovered {
				return nil, codePluginUnavailable(fmt.Errorf("plugin publish failed before a published outcome could be confirmed"))
			}
		} else if err = recoverable.EnsurePublishedVersion(ctx, prepared); err != nil {
			return nil, codePluginUnavailable(fmt.Errorf("ensure published code snapshot failed: %w", err))
		}
	} else {
		err = p.DomainSVC.PublishPlugin(ctx, &model.PublishPluginRequest{
			PluginID:    req.PluginID,
			Version:     req.VersionName,
			VersionDesc: req.VersionDesc,
		})
		if err != nil {
			return nil, errorx.Wrapf(err, "PublishPlugin failed, pluginID=%d", req.PluginID)
		}
	}

	err = p.eventbus.PublishResources(ctx, &searchEntity.ResourceDomainEvent{
		OpType: searchEntity.Updated,
		Resource: &searchEntity.ResourceDocument{
			ResType:       resCommon.ResType_Plugin,
			ResID:         req.PluginID,
			PublishStatus: ptr.Of(resCommon.PublishStatus_Published),
			PublishTimeMS: ptr.Of(time.Now().UnixMilli()),
		},
	})
	if err != nil {
		logs.CtxErrorf(ctx, "publish resource '%d' failed, err=%v", req.PluginID, err)
	}

	resp = &pluginAPI.PublishPluginResponse{}

	return resp, nil
}
func (p *PluginApplicationService) DelPlugin(ctx context.Context, req *pluginAPI.DelPluginRequest) (resp *pluginAPI.DelPluginResponse, err error) {
	_, err = p.validateDraftPluginAccess(ctx, req.PluginID)
	if err != nil {
		return nil, errorx.Wrapf(err, "validateDelPluginRequest failed")
	}
	err = p.DomainSVC.DeleteDraftPlugin(ctx, req.PluginID)
	if err != nil {
		return nil, codePluginUnavailable(errorx.Wrapf(err, "DeleteDraftPlugin failed, pluginID=%d", req.PluginID))
	}

	resourceErr := p.eventbus.PublishResources(ctx, &searchEntity.ResourceDomainEvent{
		OpType: searchEntity.Deleted,
		Resource: &searchEntity.ResourceDocument{
			ResType:      resCommon.ResType_Plugin,
			ResID:        req.PluginID,
			UpdateTimeMS: ptr.Of(time.Now().UnixMilli()),
		},
	})
	if resourceErr != nil {
		logs.CtxErrorf(ctx, "publish deleted resource failed after plugin commit, pluginID=%d, err=%v", req.PluginID, resourceErr)
	}

	resp = &pluginAPI.DelPluginResponse{}

	return resp, nil
}

func (p *PluginApplicationService) GetPluginNextVersion(ctx context.Context, req *pluginAPI.GetPluginNextVersionRequest) (resp *pluginAPI.GetPluginNextVersionResponse, err error) {
	_, err = p.validateDraftPluginAccess(ctx, req.PluginID)
	if err != nil {
		return nil, errorx.Wrapf(err, "validateGetPluginNextVersionRequest failed")
	}

	nextVersion, err := p.DomainSVC.GetPluginNextVersion(ctx, req.PluginID)
	if err != nil {
		return nil, errorx.Wrapf(err, "GetPluginNextVersion failed, pluginID=%d", req.PluginID)
	}
	resp = &pluginAPI.GetPluginNextVersionResponse{
		NextVersionName: nextVersion,
	}
	return resp, nil
}

func (p *PluginApplicationService) GetDevPluginList(ctx context.Context, req *pluginAPI.GetDevPluginListRequest) (resp *pluginAPI.GetDevPluginListResponse, err error) {

	uid := ctxutil.GetUIDFromCtx(ctx)
	if uid == nil {
		return nil, errorx.New(errno.ErrPluginPermissionCode, errorx.KV(errno.PluginMsgKey, "session is required"))
	}

	pageInfo := dto.PageInfo{
		Name:       req.Name,
		Page:       int(req.GetPage()),
		Size:       int(req.GetSize()),
		OrderByACS: ptr.Of(false),
	}
	if req.GetOrderBy() == common.OrderBy_UpdateTime {
		pageInfo.SortBy = ptr.Of(dto.SortByUpdatedAt)
	} else {
		pageInfo.SortBy = ptr.Of(dto.SortByCreatedAt)
	}

	res, err := p.DomainSVC.ListDraftPlugins(ctx, &dto.ListDraftPluginsRequest{
		SpaceID:  req.SpaceID,
		APPID:    req.ProjectID,
		PageInfo: pageInfo,
	})
	if err != nil {
		return nil, errorx.Wrapf(err, "ListDraftPlugins failed, spaceID=%d, appID=%d", req.SpaceID, req.ProjectID)
	}

	pluginList := make([]*common.PluginInfoForPlayground, 0, len(res.Plugins))
	for _, pl := range res.Plugins {

		if pl.DeveloperID > 0 && pl.DeveloperID != *uid {
			return nil, errorx.New(errno.ErrPluginPermissionCode, errorx.KV(errno.PluginMsgKey, "plugin developer is not current user"))
		}
		tools, err := p.toolRepo.GetPluginAllDraftTools(ctx, pl.ID)
		if err != nil {
			return nil, errorx.Wrapf(err, "GetPluginAllDraftTools failed, pluginID=%d", pl.ID)
		}

		pluginInfo, err := p.toPluginInfoForPlayground(ctx, pl, tools)
		if err != nil {
			return nil, err
		}

		pluginInfo.VersionTs = "0" // when you get the plugin information in the project, version ts is set to 0 by default
		pluginList = append(pluginList, pluginInfo)
	}

	resp = &pluginAPI.GetDevPluginListResponse{
		PluginList: pluginList,
		Total:      res.Total,
	}

	return resp, nil
}

func (p *PluginApplicationService) DeleteAPPAllPlugins(ctx context.Context, appID int64) (err error) {
	pluginIDs, err := p.DomainSVC.DeleteAPPAllPlugins(ctx, appID)
	if err != nil {
		return errorx.Wrapf(err, "DeleteAPPAllPlugins failed, appID=%d", appID)
	}

	for _, id := range pluginIDs {
		err = p.eventbus.PublishResources(ctx, &searchEntity.ResourceDomainEvent{
			OpType: searchEntity.Deleted,
			Resource: &searchEntity.ResourceDocument{
				ResType: resCommon.ResType_Plugin,
				ResID:   id,
			},
		})
		if err != nil {
			return errorx.Wrapf(err, "publish resource '%d' failed", id)
		}
	}

	return nil
}

func (p *PluginApplicationService) CopyPlugin(ctx context.Context, req *dto.CopyPluginRequest) (resp *dto.CopyPluginResponse, err error) {
	res, err := p.DomainSVC.CopyPlugin(ctx, &dto.CopyPluginRequest{
		UserID:      req.UserID,
		PluginID:    req.PluginID,
		CopyScene:   req.CopyScene,
		TargetAPPID: req.TargetAPPID,
	})
	if err != nil {
		return nil, errorx.Wrapf(err, "CopyPlugin failed, pluginID=%d", req.PluginID)
	}

	plugin := res.Plugin
	if plugin.PluginType == common.PluginType_FUNC {
		if plugin.Published() {
			return nil, p.compensateCodePluginCopy(
				ctx,
				plugin.ID,
				"published_target",
				"copy code plugin target inherited published state",
				nil,
			)
		}
		if p.codeRepo == nil {
			return nil, p.compensateCodePluginCopy(
				ctx,
				plugin.ID,
				"repository_unavailable",
				"copy code plugin source failed",
				nil,
			)
		}
		copyErr := p.codeRepo.CopyDraft(ctx, req.PluginID, plugin.ID, plugin.SpaceID)
		if copyErr != nil {
			return nil, p.compensateCodePluginCopy(
				ctx,
				plugin.ID,
				"draft_copy_failed",
				"copy code plugin source failed",
				copyErr,
			)
		}
	}

	now := time.Now().UnixMilli()
	resDoc := &searchEntity.ResourceDocument{
		ResType:       resCommon.ResType_Plugin,
		ResSubType:    ptr.Of(int32(plugin.PluginType)),
		ResID:         plugin.ID,
		Name:          ptr.Of(plugin.GetName()),
		SpaceID:       &plugin.SpaceID,
		APPID:         plugin.APPID,
		OwnerID:       &req.UserID,
		PublishStatus: ptr.Of(resCommon.PublishStatus_UnPublished),
		CreateTimeMS:  ptr.Of(now),
	}
	if plugin.Published() {
		resDoc.PublishStatus = ptr.Of(resCommon.PublishStatus_Published)
		resDoc.PublishTimeMS = ptr.Of(now)
	}

	err = p.eventbus.PublishResources(ctx, &searchEntity.ResourceDomainEvent{
		OpType:   searchEntity.Created,
		Resource: resDoc,
	})
	if err != nil {
		logs.CtxErrorf(ctx, "publish resource failed after plugin copy committed, pluginID=%d, err=%v", plugin.ID, err)
	}

	resp = &dto.CopyPluginResponse{
		Plugin: res.Plugin,
		Tools:  res.Tools,
	}

	return resp, nil
}

func (p *PluginApplicationService) compensateCodePluginCopy(
	ctx context.Context,
	targetPluginID int64,
	reason string,
	publicMessage string,
	copyErr error,
) error {
	logs.CtxErrorf(
		ctx,
		"code plugin copy failed, targetPluginID=%d, reason=%s, err_type=%T",
		targetPluginID,
		reason,
		copyErr,
	)
	compensationErr := p.DomainSVC.DeleteDraftPlugin(ctx, targetPluginID)
	if compensationErr != nil {
		logs.CtxErrorf(
			ctx,
			"compensate code plugin copy failed, targetPluginID=%d, reason=%s, err_type=%T",
			targetPluginID,
			reason,
			compensationErr,
		)
		return fmt.Errorf("%s; compensation incomplete", publicMessage)
	}
	return fmt.Errorf("%s", publicMessage)
}

func (p *PluginApplicationService) MoveAPPPluginToLibrary(ctx context.Context, pluginID int64) (plugin *entity.PluginInfo, err error) {
	plugin, err = p.DomainSVC.MoveAPPPluginToLibrary(ctx, pluginID)
	if err != nil {
		return nil, errorx.Wrapf(err, "MoveAPPPluginToLibrary failed, pluginID=%d", pluginID)
	}

	now := time.Now().UnixMilli()

	err = p.eventbus.PublishResources(ctx, &searchEntity.ResourceDomainEvent{
		OpType: searchEntity.Updated,
		Resource: &searchEntity.ResourceDocument{
			ResType:       resCommon.ResType_Plugin,
			ResID:         pluginID,
			APPID:         ptr.Of(int64(0)),
			PublishStatus: ptr.Of(resCommon.PublishStatus_Published),
			PublishTimeMS: ptr.Of(now),
			UpdateTimeMS:  ptr.Of(now),
		},
	})
	if err != nil {
		return nil, errorx.Wrapf(err, "publish resource '%d' failed", pluginID)
	}

	return plugin, nil
}
