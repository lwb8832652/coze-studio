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
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appskill "github.com/coze-dev/coze-studio/backend/application/skill"
	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	domain "github.com/coze-dev/coze-studio/backend/domain/skill/service"
)

func TestListSkillVersionsHandlerReturnsVersions(t *testing.T) {
	h := server.Default()
	h.GET("/api/workbench/skills/:skill_id/versions", ListSkillVersions)
	installSkillVersionTestService(t)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/skills/101/versions", nil)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(101), appskill.SVC.DomainSVC.(*skillVersionDomainService).listVersionsSkillID)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"msg":"success"`)
	require.Contains(t, body, `"skill_id":"101"`)
	require.Contains(t, body, `"version":"1.1.0"`)
	require.Contains(t, body, `"skill_md":"# Weekly Report"`)
	require.Contains(t, body, `"executor":"{\"language\":\"python\"}"`)
}

func installSkillVersionTestService(t *testing.T) {
	t.Helper()
	previous := appskill.SVC
	appskill.SVC = &appskill.ApplicationService{
		DomainSVC: &skillVersionDomainService{
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
		},
	}
	t.Cleanup(func() {
		appskill.SVC = previous
	})
}

type skillVersionDomainService struct {
	domain.SkillService
	versions            []*entity.SkillVersion
	listVersionsSkillID int64
}

func (s *skillVersionDomainService) ListVersions(ctx context.Context, skillID int64) ([]*entity.SkillVersion, error) {
	s.listVersionsSkillID = skillID
	return s.versions, nil
}
