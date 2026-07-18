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
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/stretchr/testify/require"
)

func TestAppDevProviderBuildFailureResponseUsesServerAllowlist(t *testing.T) {
	malicious := "https://provider.invalid/build?token=secret Bearer api-key minio://bucket/private/object\n" + strings.Repeat("raw", 200)
	handler := &appDevProviderHTTPHandler{}
	facade := appdev.NewProviderAPIFacade(nil, &adversarialProviderBuildControl{projection: &appdev.ProviderBuildProjection{
		Generation:    31,
		State:         appdev.ProviderBuildStateFailed,
		SafeErrorCode: "provider_raw_failure",
		SafeMessage:   malicious,
	}}, nil, nil)
	h := server.New()
	h.GET("/build", func(ctx context.Context, c *app.RequestContext) {
		projection, err := facade.BeginBuild(ctx, appdev.ProviderBuildOperationRequest{
			SpaceID: "1001", ProjectID: "project-1", OperationID: "operation-123",
		})
		require.NoError(t, err)
		handler.writeBuildProjection(c, projection, consts.StatusOK)
	})

	response := ut.PerformRequest(h.Engine, consts.MethodGet, "/build", nil)
	require.Equal(t, consts.StatusOK, response.Code)
	body := response.Body.String()
	require.Contains(t, body, `"safe_error_code":"`+infrasandbox.BuildSafeErrorCodeBuildFailed+`"`)
	require.Contains(t, body, `"safe_message":"`+infrasandbox.BuildSafeErrorMessageBuildFailed+`"`)
	for _, secret := range []string{"provider.invalid", "token=secret", "Bearer", "api-key", "minio://", "private/object", "rawraw"} {
		require.NotContains(t, body, secret)
	}
}

type adversarialProviderBuildControl struct {
	projection *appdev.ProviderBuildProjection
}

func (control *adversarialProviderBuildControl) BeginBuild(context.Context, appdev.ProviderBuildBeginInput) (*appdev.ProviderBuildProjection, error) {
	copy := *control.projection
	return &copy, nil
}

func (control *adversarialProviderBuildControl) PollBuild(context.Context, appdev.ProviderBuildPollInput) (*appdev.ProviderBuildProjection, error) {
	copy := *control.projection
	return &copy, nil
}

func (control *adversarialProviderBuildControl) RecoverBuild(context.Context, appdev.ProviderBuildRecoverInput) (*appdev.ProviderBuildProjection, error) {
	copy := *control.projection
	return &copy, nil
}
