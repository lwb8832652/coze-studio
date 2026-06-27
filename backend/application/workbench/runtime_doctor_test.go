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

package workbench

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/stretchr/testify/require"

	skillapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/skill"
	"github.com/coze-dev/coze-studio/backend/internal/testutil"
)

func TestRuntimeDoctorModelCheckReportsConfiguredDefaultModelWithoutCallingIt(t *testing.T) {
	ctx := context.Background()
	var requestedModelType int64 = -1
	chatModel := &testutil.UTChatModel{}

	check := runtimeDoctorModelCheck(ctx, func(_ context.Context, modelType int64) (model.BaseChatModel, bool, error) {
		requestedModelType = modelType
		return chatModel, true, nil
	})

	require.Equal(t, int64(0), requestedModelType)
	require.Equal(t, "model.default", check.Name)
	require.Equal(t, "model", check.Category)
	require.Equal(t, runtimeDoctorStatusReady, check.Status)
	require.Contains(t, check.Message, "configured")
	require.Zero(t, chatModel.Index)
}

func TestRuntimeDoctorModelCheckReportsProviderErrorsSafely(t *testing.T) {
	ctx := context.Background()

	check := runtimeDoctorModelCheck(ctx, func(context.Context, int64) (model.BaseChatModel, bool, error) {
		return nil, false, errors.New("provider failed with api_key=sk-test-secret")
	})

	require.Equal(t, "model.default", check.Name)
	require.Equal(t, "model", check.Category)
	require.Equal(t, runtimeDoctorStatusError, check.Status)
	require.Contains(t, check.Message, "provider failed")
	require.NotContains(t, check.Message, "sk-test-secret")
	require.NotContains(t, check.Message, "api_key")
}

func TestRuntimeDoctorModelCheckConvertsProviderPanicToDiagnosticError(t *testing.T) {
	ctx := context.Background()

	check := runtimeDoctorModelCheck(ctx, func(context.Context, int64) (model.BaseChatModel, bool, error) {
		panic("provider panic password=hunter2")
	})

	require.Equal(t, "model.default", check.Name)
	require.Equal(t, "model", check.Category)
	require.Equal(t, runtimeDoctorStatusError, check.Status)
	require.Contains(t, check.Message, "provider panic")
	require.NotContains(t, check.Message, "hunter2")
	require.NotContains(t, check.Message, "password")
}

func TestRuntimeDoctorSkillCheckSummarizesCountsAndSafeNames(t *testing.T) {
	ctx := context.Background()
	service := &fakeRuntimeDoctorSkillService{
		resp: &skillapi.ListSkillsResponse{
			Data: &skillapi.ListSkillsData{
				Skills: []*skillapi.Skill{
					{
						Name:        "research",
						Enabled:     true,
						Permissions: `{"token":"secret"}`,
					},
					{
						Name:         "writer\n<script>",
						Enabled:      true,
						InputSchema:  `{"prompt":"hidden"}`,
						OutputSchema: `{"result":"hidden"}`,
					},
					{
						Name:    "api_key=skill-name-secret",
						Enabled: true,
					},
					{
						Name:    "disabled",
						Enabled: false,
					},
				},
			},
		},
	}

	check := runtimeDoctorSkillCheck(ctx, service, 100)

	require.Equal(t, int64(100), service.req.SpaceID)
	require.Equal(t, "skills.runtime_catalog", check.Name)
	require.Equal(t, "skills", check.Category)
	require.Equal(t, runtimeDoctorStatusReady, check.Status)
	require.Contains(t, check.Message, "3 enabled / 4 total skills")
	require.Contains(t, check.Message, "research")
	require.Contains(t, check.Message, "writer script")
	require.NotContains(t, check.Message, "api_key")
	require.NotContains(t, check.Message, "secret")
	require.NotContains(t, check.Message, "hidden")
}

func TestRuntimeDoctorSkillCheckHandlesListFailureAsDiagnosticCheck(t *testing.T) {
	ctx := context.Background()
	service := &fakeRuntimeDoctorSkillService{
		err: errors.New("skill store failed password=hunter2"),
	}

	check := runtimeDoctorSkillCheck(ctx, service, 100)

	require.Equal(t, "skills.runtime_catalog", check.Name)
	require.Equal(t, "skills", check.Category)
	require.Equal(t, runtimeDoctorStatusError, check.Status)
	require.Contains(t, check.Message, "skill store failed")
	require.NotContains(t, check.Message, "hunter2")
	require.NotContains(t, check.Message, "password")
}

func TestRuntimeDoctorSkillCheckTreatsUninitializedServiceAsDisabled(t *testing.T) {
	ctx := context.Background()
	service := &fakeRuntimeDoctorSkillService{
		err: errors.New("skill service is not initialized"),
	}

	check := runtimeDoctorSkillCheck(ctx, service, 100)

	require.Equal(t, "skills.runtime_catalog", check.Name)
	require.Equal(t, "skills", check.Category)
	require.Equal(t, runtimeDoctorStatusDisabled, check.Status)
	require.Contains(t, check.Message, "not configured")
}

type fakeRuntimeDoctorSkillService struct {
	resp *skillapi.ListSkillsResponse
	err  error
	req  *skillapi.ListSkillsRequest
}

func (f *fakeRuntimeDoctorSkillService) ListSkills(
	_ context.Context,
	req *skillapi.ListSkillsRequest,
) (*skillapi.ListSkillsResponse, error) {
	f.req = req
	return f.resp, f.err
}
