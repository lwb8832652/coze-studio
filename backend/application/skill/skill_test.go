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

	resp, err := app.ListSkillVersions(context.Background(), &ListSkillVersionsRequest{SkillID: 101})

	require.NoError(t, err)
	require.Equal(t, int64(101), domainSVC.listVersionsSkillID)
	require.Len(t, resp.Versions, 1)
	require.Equal(t, int64(201), resp.Versions[0].ID)
	require.Equal(t, int64(101), resp.Versions[0].SkillID)
	require.Equal(t, "1.1.0", resp.Versions[0].Version)
	require.Equal(t, "# Weekly Report", resp.Versions[0].SkillMD)
	require.Equal(t, `{"language":"python"}`, resp.Versions[0].Executor)
}

type recordingSkillDomainService struct {
	domain.SkillService
	versions            []*entity.SkillVersion
	listVersionsSkillID int64
}

func (s *recordingSkillDomainService) ListVersions(ctx context.Context, skillID int64) ([]*entity.SkillVersion, error) {
	s.listVersionsSkillID = skillID
	return s.versions, nil
}
