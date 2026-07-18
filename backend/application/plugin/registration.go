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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"

	pluginAPI "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop"
	common "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop/common"
	resCommon "github.com/coze-dev/coze-studio/backend/api/model/resource/common"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/crossdomain/plugin/consts"
	"github.com/coze-dev/coze-studio/backend/crossdomain/plugin/convert"
	"github.com/coze-dev/coze-studio/backend/crossdomain/plugin/model"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/dto"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
	searchEntity "github.com/coze-dev/coze-studio/backend/domain/search/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/errorx"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/ptr"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	commonConsts "github.com/coze-dev/coze-studio/backend/types/consts"
	"github.com/coze-dev/coze-studio/backend/types/errno"
)

func (p *PluginApplicationService) RegisterPluginMeta(ctx context.Context, req *pluginAPI.RegisterPluginMetaRequest) (resp *pluginAPI.RegisterPluginMetaResponse, err error) {
	userID := ctxutil.GetUIDFromCtx(ctx)
	if userID == nil {
		return nil, codePluginPermission(errorx.New(errno.ErrPluginPermissionCode, errorx.KV(errno.PluginMsgKey, "session is required")))
	}

	isCodePlugin, err := validateRegisterPluginMetaApplicationRequest(req)
	if err != nil {
		return nil, codePluginInvalid(err)
	}
	if err := p.requirePluginEditor(ctx, req.GetSpaceID(), *userID); err != nil {
		return nil, err
	}

	codeRuntime := entity.CodeRuntimePython
	if isCodePlugin {
		if req.URL != nil ||
			req.Location != nil ||
			req.Key != nil ||
			req.ServiceToken != nil ||
			req.OauthInfo != nil ||
			req.SubAuthType != nil ||
			req.AuthPayload != nil ||
			req.FixedExportIP != nil ||
			len(req.CommonParams) > 0 {
			return nil, codePluginInvalid(fmt.Errorf("code plugin HTTP configuration is not supported"))
		}
		if p.codeRepo == nil {
			return nil, codePluginUnavailable(fmt.Errorf("code plugin repository is unavailable"))
		}
		codeRuntime, err = registrationCodeRuntime(req.IdeCodeRuntime)
		if err != nil {
			return nil, err
		}
	}

	authTypeValue := req.GetAuthType()
	if isCodePlugin {
		if req.AuthType != nil && authTypeValue != common.AuthorizationType_None {
			return nil, codePluginInvalid(fmt.Errorf("code plugin auth type must be none"))
		}
		authTypeValue = common.AuthorizationType_None
	} else if req.AuthType == nil {
		return nil, fmt.Errorf("plugin auth type is required")
	}

	_authType, ok := convert.ToAuthType(authTypeValue)
	if !ok {
		return nil, fmt.Errorf("invalid auth type '%d'", authTypeValue)
	}
	authType := ptr.Of(_authType)

	var authSubType *consts.AuthzSubType
	if !isCodePlugin && req.SubAuthType != nil {
		_authSubType, ok := convert.ToAuthSubType(req.GetSubAuthType())
		if !ok {
			return nil, fmt.Errorf("invalid sub authz type '%d'", req.GetSubAuthType())
		}
		authSubType = ptr.Of(_authSubType)
	}

	var loc consts.HTTPParamLocation
	if !isCodePlugin && *authType == consts.AuthzTypeOfService {
		if req.GetLocation() == common.AuthorizationServiceLocation_Query {
			loc = consts.ParamInQuery
		} else if req.GetLocation() == common.AuthorizationServiceLocation_Header {
			loc = consts.ParamInHeader
		} else {
			return nil, fmt.Errorf("invalid location '%s'", req.GetLocation())
		}
	}

	authInfo := &dto.PluginAuthInfo{
		AuthzType: authType,
	}
	if !isCodePlugin {
		authInfo.Location = ptr.Of(loc)
		authInfo.Key = req.Key
		authInfo.ServiceToken = req.ServiceToken
		authInfo.OAuthInfo = req.OauthInfo
		authInfo.AuthzSubType = authSubType
		authInfo.AuthzPayload = req.AuthPayload
	}

	r := &dto.CreateDraftPluginRequest{
		PluginType:   req.GetPluginType(),
		SpaceID:      req.GetSpaceID(),
		DeveloperID:  *userID,
		IconURI:      req.Icon.URI,
		ProjectID:    req.ProjectID,
		Name:         req.GetName(),
		Desc:         req.GetDesc(),
		ServerURL:    req.GetURL(),
		CommonParams: req.CommonParams,
		AuthInfo:     authInfo,
	}
	pluginID, err := p.DomainSVC.CreateDraftPlugin(ctx, r)
	if err != nil {
		if isCodePlugin {
			return nil, codePluginUnavailable(errorx.Wrapf(err, "CreateDraftPlugin failed"))
		}
		return nil, errorx.Wrapf(err, "CreateDraftPlugin failed")
	}

	if isCodePlugin {
		_, initErr := p.codeRepo.SaveDraftCAS(
			ctx,
			defaultCodeDraft(pluginID, req.GetSpaceID(), codeRuntime),
			0,
		)
		if initErr != nil {
			compensationErr := p.DomainSVC.DeleteDraftPlugin(ctx, pluginID)
			if compensationErr != nil {
				logs.CtxErrorf(
					ctx,
					"compensate code plugin creation failed, pluginID=%d, err=%v",
					pluginID,
					compensationErr,
				)
			}
			return nil, classifyCodePluginRepositoryError(initErr)
		}
	}

	err = p.eventbus.PublishResources(ctx, &searchEntity.ResourceDomainEvent{
		OpType: searchEntity.Created,
		Resource: &searchEntity.ResourceDocument{
			ResType:       resCommon.ResType_Plugin,
			ResSubType:    ptr.Of(int32(req.GetPluginType())),
			ResID:         pluginID,
			Name:          &req.Name,
			SpaceID:       &req.SpaceID,
			APPID:         req.ProjectID,
			OwnerID:       userID,
			PublishStatus: ptr.Of(resCommon.PublishStatus_UnPublished),
			CreateTimeMS:  ptr.Of(time.Now().UnixMilli()),
		},
	})
	if err != nil {
		logs.CtxErrorf(ctx, "publish created resource failed after plugin commit, pluginID=%d, err=%v", pluginID, err)
	}

	resp = &pluginAPI.RegisterPluginMetaResponse{
		PluginID: pluginID,
	}

	return resp, nil
}

func validateRegisterPluginMetaApplicationRequest(req *pluginAPI.RegisterPluginMetaRequest) (bool, error) {
	if req == nil {
		return false, fmt.Errorf("register plugin request is required")
	}
	if req.GetName() == "" {
		return false, fmt.Errorf("plugin name is invalid")
	}
	if req.GetDesc() == "" {
		return false, fmt.Errorf("plugin desc is invalid")
	}
	if req.URL != nil && (strings.TrimSpace(*req.URL) == "" || len(*req.URL) > 512) {
		return false, fmt.Errorf("plugin url is invalid")
	}
	if req.Icon == nil || req.Icon.URI == "" || len(req.Icon.URI) > 512 {
		return false, fmt.Errorf("plugin icon is invalid")
	}
	if req.GetSpaceID() <= 0 {
		return false, fmt.Errorf("spaceID is invalid")
	}
	if req.ProjectID != nil && *req.ProjectID <= 0 {
		return false, fmt.Errorf("projectID is invalid")
	}

	pluginType := req.GetPluginType()
	creationMethod := req.GetCreationMethod()
	switch {
	case pluginType == common.PluginType_PLUGIN && creationMethod == common.CreationMethod_COZE:
		if req.URL == nil {
			return false, fmt.Errorf("plugin url is required")
		}
		return false, nil
	case pluginType == common.PluginType_FUNC && creationMethod == common.CreationMethod_IDE:
		return true, nil
	default:
		return false, fmt.Errorf(
			"unsupported plugin creation combination: plugin_type=%s, creation_method=%s",
			pluginType.String(),
			creationMethod.String(),
		)
	}
}

func (p *PluginApplicationService) RegisterPlugin(ctx context.Context, req *pluginAPI.RegisterPluginRequest) (resp *pluginAPI.RegisterPluginResponse, err error) {
	userID := ctxutil.GetUIDFromCtx(ctx)
	if userID == nil {
		return nil, errorx.New(errno.ErrPluginPermissionCode, errorx.KV(errno.PluginMsgKey, "session is required"))
	}

	mf := &model.PluginManifest{}
	err = sonic.UnmarshalString(req.AiPlugin, &mf)
	if err != nil {
		return nil, errorx.New(errno.ErrPluginInvalidManifest, errorx.KV(errno.PluginMsgKey, err.Error()))
	}

	mf.LogoURL = commonConsts.DefaultPluginIcon

	doc, err := openapi3.NewLoader().LoadFromData([]byte(req.Openapi))
	if err != nil {
		return nil, errorx.New(errno.ErrPluginInvalidOpenapi3Doc, errorx.KV(errno.PluginMsgKey, err.Error()))
	}

	res, err := p.DomainSVC.CreateDraftPluginWithCode(ctx, &dto.CreateDraftPluginWithCodeRequest{
		SpaceID:     req.GetSpaceID(),
		DeveloperID: *userID,
		ProjectID:   req.ProjectID,
		Manifest:    mf,
		OpenapiDoc:  ptr.Of(model.Openapi3T(*doc)),
	})
	if err != nil {
		return nil, errorx.Wrapf(err, "CreateDraftPluginWithCode failed")
	}

	err = p.eventbus.PublishResources(ctx, &searchEntity.ResourceDomainEvent{
		OpType: searchEntity.Created,
		Resource: &searchEntity.ResourceDocument{
			ResType:       resCommon.ResType_Plugin,
			ResSubType:    ptr.Of(int32(res.Plugin.PluginType)),
			ResID:         res.Plugin.ID,
			Name:          ptr.Of(res.Plugin.GetName()),
			APPID:         req.ProjectID,
			SpaceID:       &req.SpaceID,
			OwnerID:       userID,
			PublishStatus: ptr.Of(resCommon.PublishStatus_UnPublished),
			CreateTimeMS:  ptr.Of(time.Now().UnixMilli()),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("publish resource '%d' failed, err=%v", res.Plugin.ID, err)
	}

	resp = &pluginAPI.RegisterPluginResponse{
		Data: &common.RegisterPluginData{
			PluginID: res.Plugin.ID,
			Openapi:  req.Openapi,
		},
	}

	return resp, nil
}

func (p *PluginApplicationService) Convert2OpenAPI(ctx context.Context, req *pluginAPI.Convert2OpenAPIRequest) (resp *pluginAPI.Convert2OpenAPIResponse, err error) {
	res := p.DomainSVC.ConvertToOpenapi3Doc(ctx, &dto.ConvertToOpenapi3DocRequest{
		RawInput:        req.Data,
		PluginServerURL: req.PluginURL,
	})

	if res.ErrMsg != "" {
		return &pluginAPI.Convert2OpenAPIResponse{
			Code:              errno.ErrPluginInvalidThirdPartyCode,
			Msg:               res.ErrMsg,
			DuplicateAPIInfos: []*common.DuplicateAPIInfo{},
			PluginDataFormat:  ptr.Of(res.Format),
		}, nil
	}

	doc, err := yaml.Marshal(res.OpenapiDoc)
	if err != nil {
		return nil, fmt.Errorf("marshal openapi doc failed, err=%v", err)
	}
	mf, err := json.Marshal(res.Manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest failed, err=%v", err)
	}

	resp = &pluginAPI.Convert2OpenAPIResponse{
		PluginDataFormat:  ptr.Of(res.Format),
		Openapi:           ptr.Of(string(doc)),
		AiPlugin:          ptr.Of(string(mf)),
		DuplicateAPIInfos: []*common.DuplicateAPIInfo{},
	}

	return resp, nil
}
