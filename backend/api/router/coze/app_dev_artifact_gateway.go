// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"errors"

	"github.com/cloudwego/hertz/pkg/route"

	handlercoze "github.com/coze-dev/coze-studio/backend/api/handler/coze"
)

const AppDevArtifactGatewayProviderPathPrefix = "/internal/provider/app-dev/artifacts"

var errAppDevArtifactGatewayRouteConfig = errors.New("appdev artifact gateway route configuration is invalid")

type AppDevArtifactGatewayRouteConfig struct {
	Root *route.RouterGroup
}

// RegisterAppDevArtifactGatewayRoutes explicitly installs only the fixed
// provider-internal PUT/GET endpoints. Task9.5 supplies the authenticated
// provider group and production dependencies; this function has no bypass.
func RegisterAppDevArtifactGatewayRoutes(config *AppDevArtifactGatewayRouteConfig, handler *handlercoze.AppDevArtifactGatewayHandler) error {
	if config == nil || config.Root == nil || handler == nil {
		return errAppDevArtifactGatewayRouteConfig
	}
	artifacts := config.Root.Group(AppDevArtifactGatewayProviderPathPrefix)
	artifacts.PUT("/:grant_id", handler.Upload)
	artifacts.GET("/:grant_id", handler.Download)
	return nil
}
