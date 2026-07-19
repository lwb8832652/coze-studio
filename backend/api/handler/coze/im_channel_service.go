// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	appimchannel "github.com/coze-dev/coze-studio/backend/application/imchannel"
	domain "github.com/coze-dev/coze-studio/backend/domain/imchannel"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

type imChannelMutationRequest struct {
	SpaceID     string `json:"space_id"`
	Name        string `json:"name"`
	AppID       string `json:"app_id"`
	AppSecret   string `json:"app_secret"`
	AgentID     string `json:"agent_id"`
	Enabled     bool   `json:"enabled"`
	ReplyMode   string `json:"reply_mode"`
	GroupPolicy string `json:"group_policy"`
}

type imChannelSpaceRequest struct {
	SpaceID string `json:"space_id"`
}

type imChannelConfigAPI struct {
	ID               string `json:"id"`
	SpaceID          string `json:"space_id"`
	CreatorID        string `json:"creator_id"`
	AgentID          string `json:"agent_id"`
	AgentName        string `json:"agent_name"`
	ChannelType      string `json:"channel_type"`
	Name             string `json:"name"`
	AppID            string `json:"app_id"`
	SecretConfigured bool   `json:"secret_configured"`
	Enabled          bool   `json:"enabled"`
	ReplyMode        string `json:"reply_mode"`
	GroupPolicy      string `json:"group_policy"`
	RuntimeStatus    string `json:"runtime_status"`
	RuntimeError     string `json:"runtime_error,omitempty"`
	BotOpenID        string `json:"bot_open_id,omitempty"`
	BotName          string `json:"bot_name,omitempty"`
	LastConnectedAt  int64  `json:"last_connected_at,omitempty"`
	LastTestedAt     int64  `json:"last_tested_at,omitempty"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
}

func ListIMChannels(ctx context.Context, c *app.RequestContext) {
	spaceID, err := parseIMChannelID(c.Query("space_id"))
	if err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	result, err := appimchannel.SVC.List(ctx, workbenchViewerIDFromCtx(ctx), spaceID)
	if err != nil {
		imChannelErrorResponse(ctx, c, err)
		return
	}
	configs := make([]*imChannelConfigAPI, 0, len(result.Configs))
	for _, config := range result.Configs {
		configs = append(configs, imChannelConfigToAPI(config))
	}
	imChannelSuccess(c, map[string]any{
		"configs":          configs,
		"can_manage":       result.CanManage,
		"credential_ready": result.CredentialReady,
	})
}

func CreateIMChannel(ctx context.Context, c *app.RequestContext) {
	var req imChannelMutationRequest
	if err := c.BindAndValidate(&req); err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	spaceID, agentID, err := parseIMChannelMutationIDs(req.SpaceID, req.AgentID)
	if err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	config, err := appimchannel.SVC.Create(ctx, appimchannel.CreateRequest{
		SpaceID:     spaceID,
		ActorID:     workbenchViewerIDFromCtx(ctx),
		Name:        req.Name,
		AppID:       req.AppID,
		AppSecret:   req.AppSecret,
		AgentID:     agentID,
		Enabled:     req.Enabled,
		ReplyMode:   domain.ReplyMode(req.ReplyMode),
		GroupPolicy: domain.GroupPolicy(req.GroupPolicy),
	})
	if err != nil {
		imChannelErrorResponse(ctx, c, err)
		return
	}
	imChannelSuccess(c, imChannelConfigToAPI(config))
}

func UpdateIMChannel(ctx context.Context, c *app.RequestContext) {
	var req imChannelMutationRequest
	if err := c.BindAndValidate(&req); err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	configID, err := parseIMChannelID(c.Param("channel_id"))
	if err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	spaceID, agentID, err := parseIMChannelMutationIDs(req.SpaceID, req.AgentID)
	if err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	config, err := appimchannel.SVC.Update(ctx, appimchannel.UpdateRequest{
		SpaceID:     spaceID,
		ConfigID:    configID,
		ActorID:     workbenchViewerIDFromCtx(ctx),
		Name:        req.Name,
		AppID:       req.AppID,
		AppSecret:   req.AppSecret,
		AgentID:     agentID,
		ReplyMode:   domain.ReplyMode(req.ReplyMode),
		GroupPolicy: domain.GroupPolicy(req.GroupPolicy),
	})
	if err != nil {
		imChannelErrorResponse(ctx, c, err)
		return
	}
	imChannelSuccess(c, imChannelConfigToAPI(config))
}

func EnableIMChannel(ctx context.Context, c *app.RequestContext) {
	setIMChannelEnabled(ctx, c, true)
}

func DisableIMChannel(ctx context.Context, c *app.RequestContext) {
	setIMChannelEnabled(ctx, c, false)
}

func setIMChannelEnabled(ctx context.Context, c *app.RequestContext, enabled bool) {
	var req imChannelSpaceRequest
	if err := c.BindAndValidate(&req); err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	configID, err := parseIMChannelID(c.Param("channel_id"))
	if err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	spaceID, err := parseIMChannelID(req.SpaceID)
	if err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	config, err := appimchannel.SVC.SetEnabled(
		ctx,
		workbenchViewerIDFromCtx(ctx),
		spaceID,
		configID,
		enabled,
	)
	if err != nil {
		imChannelErrorResponse(ctx, c, err)
		return
	}
	imChannelSuccess(c, imChannelConfigToAPI(config))
}

func TestIMChannelConnection(ctx context.Context, c *app.RequestContext) {
	var req imChannelSpaceRequest
	if err := c.BindAndValidate(&req); err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	configID, err := parseIMChannelID(c.Param("channel_id"))
	if err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	spaceID, err := parseIMChannelID(req.SpaceID)
	if err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	identity, err := appimchannel.SVC.TestConnection(
		ctx,
		workbenchViewerIDFromCtx(ctx),
		spaceID,
		configID,
	)
	if err != nil {
		imChannelErrorResponse(ctx, c, err)
		return
	}
	imChannelSuccess(c, map[string]string{
		"bot_open_id": identity.OpenID,
		"bot_name":    identity.Name,
	})
}

func DeleteIMChannel(ctx context.Context, c *app.RequestContext) {
	configID, err := parseIMChannelID(c.Param("channel_id"))
	if err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	spaceID, err := parseIMChannelID(c.Query("space_id"))
	if err != nil {
		imChannelErrorResponse(ctx, c, domain.ErrInvalidInput)
		return
	}
	err = appimchannel.SVC.Delete(
		ctx,
		workbenchViewerIDFromCtx(ctx),
		spaceID,
		configID,
	)
	if err != nil {
		imChannelErrorResponse(ctx, c, err)
		return
	}
	imChannelSuccess(c, map[string]bool{"deleted": true})
}

func imChannelConfigToAPI(config *appimchannel.ConfigView) *imChannelConfigAPI {
	if config == nil {
		return nil
	}
	return &imChannelConfigAPI{
		ID:               strconv.FormatInt(config.ID, 10),
		SpaceID:          strconv.FormatInt(config.SpaceID, 10),
		CreatorID:        strconv.FormatInt(config.CreatorID, 10),
		AgentID:          strconv.FormatInt(config.AgentID, 10),
		AgentName:        config.AgentName,
		ChannelType:      config.ChannelType,
		Name:             config.Name,
		AppID:            config.AppID,
		SecretConfigured: config.SecretConfigured,
		Enabled:          config.Enabled,
		ReplyMode:        string(config.ReplyMode),
		GroupPolicy:      string(config.GroupPolicy),
		RuntimeStatus:    string(config.RuntimeStatus),
		RuntimeError:     config.RuntimeError,
		BotOpenID:        config.BotOpenID,
		BotName:          config.BotName,
		LastConnectedAt:  imChannelTime(config.LastConnectedAt),
		LastTestedAt:     imChannelTime(config.LastTestedAt),
		CreatedAt:        config.CreatedAt.UnixMilli(),
		UpdatedAt:        config.UpdatedAt.UnixMilli(),
	}
}

func imChannelTime(value *time.Time) int64 {
	if value == nil || value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}

func parseIMChannelMutationIDs(spaceID, agentID string) (int64, int64, error) {
	space, err := parseIMChannelID(spaceID)
	if err != nil {
		return 0, 0, err
	}
	agent, err := parseIMChannelID(agentID)
	if err != nil {
		return 0, 0, err
	}
	return space, agent, nil
}

func parseIMChannelID(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.ErrInvalidInput
	}
	return id, nil
}

func imChannelSuccess(c *app.RequestContext, data any) {
	c.JSON(http.StatusOK, map[string]any{
		"code": 0,
		"msg":  "",
		"data": data,
	})
}

func imChannelErrorResponse(ctx context.Context, c *app.RequestContext, err error) {
	status := http.StatusInternalServerError
	code := "IM_CHANNEL_INTERNAL_ERROR"
	message := "IM 机器人服务暂时不可用"
	switch {
	case errors.Is(err, domain.ErrUnauthenticated):
		status, code, message = http.StatusUnauthorized, "IM_CHANNEL_UNAUTHENTICATED", "请先登录"
	case errors.Is(err, domain.ErrForbidden):
		status, code, message = http.StatusForbidden, "IM_CHANNEL_FORBIDDEN", "仅工作空间所有者或管理员可执行此操作"
	case errors.Is(err, domain.ErrInvalidInput):
		status, code, message = http.StatusBadRequest, "IM_CHANNEL_INVALID_INPUT", "请检查机器人配置"
	case errors.Is(err, domain.ErrNotFound):
		status, code, message = http.StatusNotFound, "IM_CHANNEL_NOT_FOUND", "机器人配置不存在"
	case errors.Is(err, domain.ErrConflict):
		status, code, message = http.StatusConflict, "IM_CHANNEL_CONFLICT", "该飞书应用已在当前工作空间配置"
	case errors.Is(err, domain.ErrCredentialCodecMissing):
		status, code, message = http.StatusServiceUnavailable, "IM_CHANNEL_CREDENTIAL_UNAVAILABLE", "服务端尚未配置 IM 凭据加密密钥"
	case errors.Is(err, domain.ErrAgentUnavailable):
		status, code, message = http.StatusBadRequest, "IM_CHANNEL_AGENT_UNAVAILABLE", "请选择当前工作空间内已发布的 Agent"
	case errors.Is(err, domain.ErrConnectionTestFailed):
		status, code, message = http.StatusBadGateway, "IM_CHANNEL_CONNECTION_FAILED", "飞书连接检测失败，请检查 App ID、App Secret 与应用状态"
	case errors.Is(err, domain.ErrRuntimeUnavailable):
		status, code, message = http.StatusServiceUnavailable, "IM_CHANNEL_RUNTIME_UNAVAILABLE", "IM 机器人运行时尚未就绪"
	default:
		logs.CtxErrorf(ctx, "IM channel request failed: %v", err)
	}
	c.JSON(status, map[string]any{
		"code": code,
		"msg":  message,
	})
}
