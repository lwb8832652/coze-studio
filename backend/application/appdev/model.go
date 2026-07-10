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

package appdev

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	bizconfig "github.com/coze-dev/coze-studio/backend/bizpkg/config"
	"github.com/coze-dev/coze-studio/backend/bizpkg/config/modelmgr"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type ListModelsRequest struct {
	SpaceID       string
	CurrentUserID int64
	Scenario      string
}

type ModelPageData struct {
	Items []*domainappdev.Model `json:"items"`
	Total int                   `json:"total"`
}

type ModelListResponse struct {
	Code    int64         `json:"code"`
	Message string        `json:"message"`
	Data    ModelPageData `json:"data"`
}

func (s *Service) ListModels(ctx context.Context, req *ListModelsRequest) (*ModelListResponse, error) {
	if err := validateUserAndSpace(req.CurrentUserID, req.SpaceID); err != nil {
		return nil, err
	}
	if scenario := strings.TrimSpace(req.Scenario); scenario != "" && scenario != "PageApp" {
		return nil, fmt.Errorf("unsupported appdev model scenario")
	}
	modelConfig, ok := currentModelConfig()
	if !ok || modelConfig == nil {
		return successModelListResponse(nil), nil
	}

	models, err := modelConfig.GetOnlineModelList(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]*domainappdev.Model, 0, len(models))
	for _, model := range models {
		if model == nil || model.Model == nil || model.Type != adminconfig.ModelType_LLM {
			continue
		}

		name := strconv.FormatInt(model.ID, 10)
		if model.DisplayInfo != nil && strings.TrimSpace(model.DisplayInfo.Name) != "" {
			name = strings.TrimSpace(model.DisplayInfo.Name)
		}

		provider := ""
		protocol := ""
		if model.Provider != nil {
			protocol = fmt.Sprintf("%v", model.Provider.ModelClass)
			if model.Provider.Name != nil {
				provider = strings.TrimSpace(model.Provider.Name.ZhCn)
				if provider == "" {
					provider = strings.TrimSpace(model.Provider.Name.EnUs)
				}
			}
		}

		supportsMultiModal := false
		supportsImageUnderstanding := false
		if model.Capability != nil {
			supportsMultiModal = model.Capability.GetSupportMultiModal()
			supportsImageUnderstanding = model.Capability.GetImageUnderstanding()
		}

		items = append(items, &domainappdev.Model{
			ID:                         strconv.FormatInt(model.ID, 10),
			Name:                       name,
			Provider:                   provider,
			Protocol:                   protocol,
			SupportsMultiModal:         supportsMultiModal,
			SupportsImageUnderstanding: supportsImageUnderstanding,
			EnableBase64URL:            model.EnableBase64URL,
		})
	}

	return successModelListResponse(items), nil
}

func currentModelConfig() (conf *modelmgr.ModelConfig, ok bool) {
	defer func() {
		if recover() != nil {
			conf = nil
			ok = false
		}
	}()

	conf = bizconfig.ModelConf()
	return conf, conf != nil
}

func successModelListResponse(items []*domainappdev.Model) *ModelListResponse {
	if items == nil {
		items = []*domainappdev.Model{}
	}

	return &ModelListResponse{
		Code:    0,
		Message: "success",
		Data: ModelPageData{
			Items: items,
			Total: len(items),
		},
	}
}
