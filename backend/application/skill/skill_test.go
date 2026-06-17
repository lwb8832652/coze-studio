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

package skill

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	skillapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/skill"
	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	domain "github.com/coze-dev/coze-studio/backend/domain/skill/service"
)

func TestIsClientErrorWrapsDomainClassification(t *testing.T) {
	require.True(t, IsClientError(domain.InvalidArgumentErrorf("bad request")))
	require.False(t, IsClientError(nil))
}

func TestApplicationListSkillVersionsMapsDomainVersions(t *testing.T) {
	domainSVC := &recordingSkillDomainService{
		versions: []*entity.SkillVersion{
			{
				ID:           201,
				SkillID:      101,
				Version:      "1.1.0",
				SkillMD:      "# Weekly Report",
				InputSchema:  `{"type":"object"}`,
				OutputSchema: `{"type":"object"}`,
				Executor:     `{"language":"python"}`,
				Permissions:  `{"network":false}`,
				CreatedAt:    1000,
			},
		},
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	resp, err := app.ListSkillVersions(context.Background(), &skillapi.ListSkillVersionsRequest{SkillID: 101})

	require.NoError(t, err)
	require.Equal(t, int64(101), domainSVC.listVersionsSkillID)
	require.Equal(t, int64(0), resp.Code)
	require.Equal(t, "success", resp.Msg)
	require.Len(t, resp.Data.Versions, 1)
	require.Equal(t, int64(201), resp.Data.Versions[0].ID)
	require.Equal(t, int64(101), resp.Data.Versions[0].SkillID)
	require.Equal(t, "1.1.0", resp.Data.Versions[0].Version)
	require.Equal(t, "# Weekly Report", resp.Data.Versions[0].SkillMD)
	require.Equal(t, `{"language":"python"}`, resp.Data.Versions[0].Executor)
}

func TestApplicationListSkillVersionResourcesMapsDomainResources(t *testing.T) {
	domainSVC := &recordingSkillDomainService{
		resources: []*entity.SkillResource{
			{
				ID:        301,
				SkillID:   101,
				VersionID: 201,
				Path:      "references/prompt.md",
				Content:   []byte("Use concise bullets."),
				Size:      20,
				SHA256:    "hash-prompt",
				CreatedAt: 1000,
			},
		},
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	resp, err := app.ListSkillVersionResources(context.Background(), &skillapi.ListSkillVersionResourcesRequest{
		SkillID:   101,
		VersionID: 201,
	})

	require.NoError(t, err)
	require.Equal(t, int64(101), domainSVC.listResourcesSkillID)
	require.Equal(t, int64(201), domainSVC.listResourcesVersionID)
	require.Equal(t, int64(0), resp.Code)
	require.Equal(t, "success", resp.Msg)
	require.Len(t, resp.Data.Resources, 1)
	require.Equal(t, int64(301), resp.Data.Resources[0].ID)
	require.Equal(t, "references/prompt.md", resp.Data.Resources[0].Path)
	require.Equal(t, "VXNlIGNvbmNpc2UgYnVsbGV0cy4=", resp.Data.Resources[0].ContentBase64)
	require.Equal(t, "hash-prompt", resp.Data.Resources[0].SHA256)
}

func TestEntityToAPIMapsDeerSkillType(t *testing.T) {
	apiSkill, err := entityToAPI(&entity.Skill{
		ID:           101,
		SpaceID:      1,
		Name:         "weekly-research",
		Description:  "Research weekly market changes.",
		Type:         entity.TypeDeerSkill,
		Version:      "1.0.0",
		Enabled:      true,
		InputSchema:  "{}",
		OutputSchema: "{}",
		Executor:     "{}",
		Permissions:  "{}",
	})

	require.NoError(t, err)
	require.Equal(t, skillapi.SkillType_DeerSkill, apiSkill.Type)
}

type recordingSkillDomainService struct {
	domain.SkillService
	versions               []*entity.SkillVersion
	resources              []*entity.SkillResource
	listVersionsSkillID    int64
	listResourcesSkillID   int64
	listResourcesVersionID int64
}

func (s *recordingSkillDomainService) ListVersions(ctx context.Context, skillID int64) ([]*entity.SkillVersion, error) {
	s.listVersionsSkillID = skillID
	return s.versions, nil
}

func (s *recordingSkillDomainService) ListVersionResources(ctx context.Context, skillID, versionID int64) ([]*entity.SkillResource, error) {
	s.listResourcesSkillID = skillID
	s.listResourcesVersionID = versionID
	return s.resources, nil
}
