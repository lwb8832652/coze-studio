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

package coze

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	diagnosticapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/diagnostic"
	appworkbench "github.com/coze-dev/coze-studio/backend/application/workbench"
)

// GetWorkbenchRuntimeDoctor .
// @router /api/workbench/runtime_doctor [GET]
func GetWorkbenchRuntimeDoctor(ctx context.Context, c *app.RequestContext) {
	var req diagnosticapi.GetWorkbenchRuntimeDoctorRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appworkbench.SVC.GetRuntimeDoctor(ctx, &req)
	if err != nil {
		workbenchRuntimeDoctorErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func workbenchRuntimeDoctorErrorResponse(
	ctx context.Context,
	c *app.RequestContext,
	err error,
) {
	if appworkbench.IsClientError(err) {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	internalServerErrorResponse(ctx, c, err)
}
