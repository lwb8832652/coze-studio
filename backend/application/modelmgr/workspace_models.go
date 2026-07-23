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

package modelmgr

import (
	"context"

	workbenchmodel "github.com/coze-dev/coze-studio/backend/api/model/workbench/model"
	"github.com/coze-dev/coze-studio/backend/bizpkg/config"
)

func (m *ModelmgrApplicationService) ListWorkspaceModels(
	ctx context.Context,
	spaceID int64,
	canManage bool,
	keyword string,
) (*workbenchmodel.ListWorkspaceModelsData, error) {
	return config.ModelConf().ListWorkspaceModels(ctx, spaceID, canManage, keyword)
}

func (m *ModelmgrApplicationService) GetWorkspaceModel(
	ctx context.Context,
	spaceID int64,
	modelID int64,
	canManage bool,
) (*workbenchmodel.WorkspaceModel, error) {
	return config.ModelConf().GetWorkspaceModel(ctx, spaceID, modelID, canManage)
}

func (m *ModelmgrApplicationService) UpsertWorkspaceModel(
	ctx context.Context,
	spaceID int64,
	creatorID int64,
	modelID *int64,
	draft *workbenchmodel.WorkspaceModelDraft,
) (*workbenchmodel.WorkspaceModel, error) {
	return config.ModelConf().UpsertWorkspaceModel(ctx, spaceID, creatorID, modelID, draft)
}

func (m *ModelmgrApplicationService) TestWorkspaceModel(
	ctx context.Context,
	spaceID int64,
	modelID *int64,
	draft *workbenchmodel.WorkspaceModelDraft,
) (*workbenchmodel.TestWorkspaceModelData, error) {
	return config.ModelConf().TestWorkspaceModelConnection(ctx, spaceID, modelID, draft)
}

func (m *ModelmgrApplicationService) SetWorkspaceModelStatus(
	ctx context.Context,
	spaceID int64,
	modelID int64,
	enabled bool,
) error {
	return config.ModelConf().SetWorkspaceModelStatus(ctx, spaceID, modelID, enabled)
}

func (m *ModelmgrApplicationService) DeleteWorkspaceModel(ctx context.Context, spaceID, modelID int64) error {
	return config.ModelConf().DeleteWorkspaceModel(ctx, spaceID, modelID)
}
