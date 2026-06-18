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
	"bytes"
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

func TestListSkillVersionResourcesHandlerReturnsResources(t *testing.T) {
	h := server.Default()
	h.GET("/api/workbench/skills/:skill_id/versions/:version_id/resources", ListSkillVersionResources)
	installSkillVersionTestService(t)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/skills/101/versions/201/resources", nil)
	body := string(w.Result().Body())
	domainSVC := appskill.SVC.DomainSVC.(*skillVersionDomainService)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(101), domainSVC.listResourcesSkillID)
	require.Equal(t, int64(201), domainSVC.listResourcesVersionID)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"skill_id":"101"`)
	require.Contains(t, body, `"version_id":"201"`)
	require.Contains(t, body, `"path":"references/prompt.md"`)
	require.Contains(t, body, `"content_base64":"VXNlIGNvbmNpc2UgYnVsbGV0cy4="`)
	require.Contains(t, body, `"sha256":"hash-prompt"`)
}

func TestExportSkillVersionHandlerReturnsArchive(t *testing.T) {
	h := server.Default()
	h.GET("/api/workbench/skills/:skill_id/versions/:version_id/export", ExportSkillVersion)
	installSkillVersionTestService(t)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/skills/101/versions/201/export", nil)
	body := string(w.Result().Body())
	domainSVC := appskill.SVC.DomainSVC.(*skillVersionDomainService)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(101), domainSVC.listVersionsSkillID)
	require.Equal(t, int64(101), domainSVC.listResourcesSkillID)
	require.Equal(t, int64(201), domainSVC.listResourcesVersionID)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"file_name":"skill_101_201.skill"`)
	require.Contains(t, body, `"content_type":"application/zip"`)
	require.Contains(t, body, `"content_base64":"`)
}

func TestRollbackSkillVersionHandlerRestoresSkill(t *testing.T) {
	h := server.Default()
	h.POST("/api/workbench/skills/:skill_id/versions/:version_id/rollback", RollbackSkillVersion)
	installSkillVersionTestService(t)

	w := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/skills/101/versions/201/rollback", nil)
	body := string(w.Result().Body())
	domainSVC := appskill.SVC.DomainSVC.(*skillVersionDomainService)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(101), domainSVC.rollbackSkillID)
	require.Equal(t, int64(201), domainSVC.rollbackVersionID)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"name":"weekly-research"`)
	require.Contains(t, body, `"description":"Original research skill."`)
	require.Contains(t, body, `"version":"1.0.0"`)
}

func TestUpdateSkillVersionResourceHandlerReturnsNewVersion(t *testing.T) {
	h := server.Default()
	h.PUT("/api/workbench/skills/:skill_id/versions/:version_id/resources", UpdateSkillVersionResource)
	installSkillVersionTestService(t)

	payload := []byte(`{"path":"references/prompt.md","content_base64":"VXNlIGNvbmNpc2UgYnVsbGV0cy4="}`)
	w := ut.PerformRequest(
		h.Engine,
		http.MethodPut,
		"/api/workbench/skills/101/versions/201/resources",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(w.Result().Body())
	domainSVC := appskill.SVC.DomainSVC.(*skillVersionDomainService)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(101), domainSVC.updatedResourceSkillID)
	require.Equal(t, int64(201), domainSVC.updatedResourceVersionID)
	require.Equal(t, "references/prompt.md", domainSVC.updatedResourcePath)
	require.Equal(t, []byte("Use concise bullets."), domainSVC.updatedResourceContent)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"id":"401"`)
	require.Contains(t, body, `"skill_md":"# Weekly Report"`)
}

func TestUpdateSkillVersionContentHandlerReturnsNewVersion(t *testing.T) {
	h := server.Default()
	h.PUT("/api/workbench/skills/:skill_id/versions/:version_id/content", UpdateSkillVersionContent)
	installSkillVersionTestService(t)

	payload := []byte(`{"skill_md":"# Updated Weekly Report"}`)
	w := ut.PerformRequest(
		h.Engine,
		http.MethodPut,
		"/api/workbench/skills/101/versions/201/content",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(w.Result().Body())
	domainSVC := appskill.SVC.DomainSVC.(*skillVersionDomainService)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, int64(101), domainSVC.updatedContentSkillID)
	require.Equal(t, int64(201), domainSVC.updatedContentVersionID)
	require.Equal(t, "# Updated Weekly Report", domainSVC.updatedSkillMD)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"id":"402"`)
	require.Contains(t, body, `"skill_md":"# Updated Weekly Report"`)
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
			rolledBack: &entity.Skill{
				ID:           101,
				SpaceID:      1,
				Name:         "weekly-research",
				Description:  "Original research skill.",
				Type:         entity.TypeDeerSkill,
				Version:      "1.0.0",
				Enabled:      true,
				InputSchema:  `{"type":"object"}`,
				OutputSchema: `{"type":"object"}`,
				Executor:     `{"mode":"agent"}`,
				Permissions:  `{"network":false}`,
				CreatedAt:    1000,
				UpdatedAt:    2000,
			},
			updatedResourceVersion: &entity.SkillVersion{
				ID:           401,
				SkillID:      101,
				Version:      "1.0.0",
				SkillMD:      "# Weekly Report",
				InputSchema:  `{"type":"object"}`,
				OutputSchema: `{"type":"object"}`,
				Executor:     `{"mode":"agent"}`,
				Permissions:  `{"network":false}`,
				CreatedAt:    3000,
			},
			updatedContentVersion: &entity.SkillVersion{
				ID:           402,
				SkillID:      101,
				Version:      "1.2.0",
				SkillMD:      "# Updated Weekly Report",
				InputSchema:  `{"type":"object"}`,
				OutputSchema: `{"type":"object"}`,
				Executor:     `{"mode":"agent"}`,
				Permissions:  `{"network":false}`,
				CreatedAt:    3000,
			},
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
		},
	}
	t.Cleanup(func() {
		appskill.SVC = previous
	})
}

type skillVersionDomainService struct {
	domain.SkillService
	versions                 []*entity.SkillVersion
	resources                []*entity.SkillResource
	rolledBack               *entity.Skill
	updatedResourceVersion   *entity.SkillVersion
	updatedContentVersion    *entity.SkillVersion
	listVersionsSkillID      int64
	listResourcesSkillID     int64
	listResourcesVersionID   int64
	rollbackSkillID          int64
	rollbackVersionID        int64
	updatedResourceSkillID   int64
	updatedResourceVersionID int64
	updatedResourcePath      string
	updatedResourceContent   []byte
	updatedContentSkillID    int64
	updatedContentVersionID  int64
	updatedSkillMD           string
}

func (s *skillVersionDomainService) ListVersions(ctx context.Context, skillID int64) ([]*entity.SkillVersion, error) {
	s.listVersionsSkillID = skillID
	return s.versions, nil
}

func (s *skillVersionDomainService) ListVersionResources(ctx context.Context, skillID, versionID int64) ([]*entity.SkillResource, error) {
	s.listResourcesSkillID = skillID
	s.listResourcesVersionID = versionID
	return s.resources, nil
}

func (s *skillVersionDomainService) RollbackVersion(ctx context.Context, skillID, versionID int64) (*entity.Skill, error) {
	s.rollbackSkillID = skillID
	s.rollbackVersionID = versionID
	return s.rolledBack, nil
}

func (s *skillVersionDomainService) UpdateVersionResource(ctx context.Context, skillID, versionID int64, path string, content []byte) (*entity.SkillVersion, error) {
	s.updatedResourceSkillID = skillID
	s.updatedResourceVersionID = versionID
	s.updatedResourcePath = path
	s.updatedResourceContent = append([]byte(nil), content...)
	return s.updatedResourceVersion, nil
}

func (s *skillVersionDomainService) UpdateVersionContent(ctx context.Context, skillID, versionID int64, skillMD string) (*entity.SkillVersion, error) {
	s.updatedContentSkillID = skillID
	s.updatedContentVersionID = versionID
	s.updatedSkillMD = skillMD
	return s.updatedContentVersion, nil
}
