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
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
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

func TestDecodeSkillImportContentDecodesSkillArchiveEnvelope(t *testing.T) {
	content, err := decodeSkillImportContent(
		"weekly.skill",
		"base64:"+base64.StdEncoding.EncodeToString([]byte("PK archive")),
	)

	require.NoError(t, err)
	require.Equal(t, []byte("PK archive"), content)
}

func TestDecodeSkillImportContentKeepsTextDeclarations(t *testing.T) {
	content, err := decodeSkillImportContent("SKILL.md", "# Weekly Report")

	require.NoError(t, err)
	require.Equal(t, []byte("# Weekly Report"), content)
}

func TestDecodeSkillImportContentRejectsRawSkillArchive(t *testing.T) {
	_, err := decodeSkillImportContent("weekly.skill", "PK archive")

	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorContains(t, err, "base64 encoding")
}

func TestApplicationImportSkillUsesCustomDefaultType(t *testing.T) {
	archive, err := buildSkillVersionArchive(&entity.SkillVersion{SkillMD: `---
name: weekly-research
description: Research weekly market changes.
type: custom_skill
version: 1.0.0
enabled: true
---
# Weekly Research
`}, nil)
	require.NoError(t, err)
	domainSVC := &recordingSkillDomainService{
		imported: &entity.Skill{
			ID:           101,
			SpaceID:      1,
			Name:         "weekly-research",
			Description:  "Research weekly market changes.",
			Type:         entity.TypeCustomSkill,
			Version:      "1.0.0",
			Enabled:      true,
			InputSchema:  `{}`,
			OutputSchema: `{}`,
			Executor:     `{}`,
			Permissions:  `{}`,
		},
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	resp, err := app.ImportSkill(context.Background(), &skillapi.ImportSkillRequest{
		SpaceID:  1,
		FileName: "weekly-research.skill",
		Content:  "base64:" + base64.StdEncoding.EncodeToString(archive),
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), domainSVC.importSpaceID)
	require.Equal(t, "weekly-research.skill", domainSVC.importFileName)
	require.Equal(t, archive, domainSVC.importContent)
	require.Equal(t, entity.TypeCustomSkill, domainSVC.importDefaultType)
	require.Equal(t, skillapi.SkillType_CustomSkill, resp.Data.Type)
}

func TestApplicationArtifactImportPassesTrustedDevelopmentThreadIntoDomainImport(t *testing.T) {
	archive, err := buildSkillVersionArchive(&entity.SkillVersion{SkillMD: `---
name: artifact-skill
description: Imported from an authenticated artifact.
type: custom_skill
version: 1.0.0
enabled: true
---
# Artifact Skill
`}, nil)
	require.NoError(t, err)
	domainSVC := &recordingSkillDomainService{imported: &entity.Skill{
		ID: 101, SpaceID: 1, Name: "artifact-skill", Type: entity.TypeCustomSkill,
		InputSchema: `{}`, OutputSchema: `{}`, Executor: `{}`, Permissions: `{}`,
	}}
	app := &ApplicationService{DomainSVC: domainSVC}

	resp, err := app.ImportSkillFromArtifact(context.Background(), &skillapi.ImportSkillRequest{
		SpaceID: 1, FileName: "artifact-skill.skill",
		Content: "base64:" + base64.StdEncoding.EncodeToString(archive),
	}, 901)
	require.NoError(t, err)
	require.Equal(t, int64(901), domainSVC.importDevelopmentThreadID)
	require.Equal(t, int64(901), resp.Data.DevelopmentThreadID)
}

func TestApplicationRejectsMissingExpectedVersionID(t *testing.T) {
	app := &ApplicationService{}
	_, err := app.UpdateSkill(context.Background(), &skillapi.UpdateSkillRequest{ID: 101})
	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorContains(t, err, "expected version id is required")

	_, err = app.RollbackSkillVersion(context.Background(), &skillapi.RollbackSkillVersionRequest{
		SkillID: 101, VersionID: 201,
	})
	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorContains(t, err, "expected version id is required")
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

func TestApplicationExportSkillVersionBuildsSkillArchive(t *testing.T) {
	domainSVC := &recordingSkillDomainService{
		versions: []*entity.SkillVersion{
			{
				ID:      201,
				SkillID: 101,
				Version: "1.1.0",
				SkillMD: `---
name: weekly-research
description: Research weekly market changes.
---
# Weekly Research
`,
			},
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
			},
		},
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	resp, err := app.ExportSkillVersion(context.Background(), &skillapi.ExportSkillVersionRequest{
		SkillID:   101,
		VersionID: 201,
	})

	require.NoError(t, err)
	require.Equal(t, int64(101), domainSVC.listVersionsSkillID)
	require.Equal(t, int64(101), domainSVC.listResourcesSkillID)
	require.Equal(t, int64(201), domainSVC.listResourcesVersionID)
	require.Equal(t, int64(0), resp.Code)
	require.Equal(t, "success", resp.Msg)
	require.Equal(t, "skill_101_201.skill", resp.Data.FileName)
	require.Equal(t, "application/zip", resp.Data.ContentType)
	archiveBytes, err := base64.StdEncoding.DecodeString(resp.Data.ContentBase64)
	require.NoError(t, err)
	files := readZipArchive(t, archiveBytes)
	require.Equal(t, domainSVC.versions[0].SkillMD, files["SKILL.md"])
	require.Equal(t, "Use concise bullets.", files["references/prompt.md"])
}

func TestApplicationExportSkillBuildsLatestImportableArchive(t *testing.T) {
	skill := &entity.Skill{ID: 101, SpaceID: 1, Name: "weekly", Description: "safe", Type: entity.TypeCustomSkill, Version: "1.0.0", Enabled: true, InputSchema: `{}`, OutputSchema: `{}`, Executor: `{}`, Permissions: `{}`}
	domainSVC := &recordingSkillDomainService{
		skill: skill,
		versions: []*entity.SkillVersion{{ID: 201, SkillID: 101, Version: "1.0.0", SkillMD: `---
name: weekly
description: safe
type: custom_skill
version: 1.0.0
enabled: true
---
# Full body
`}},
		resources: []*entity.SkillResource{{SkillID: 101, VersionID: 201, Path: "references/prompt.md", Content: []byte("keep")}},
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	resp, err := app.ExportSkill(context.Background(), &skillapi.GetSkillRequest{SkillID: 101})
	require.NoError(t, err)
	require.Equal(t, "skill_101.skill", resp.Data.FileName)
	require.Contains(t, resp.Data.Content, "base64:")
	archive, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(resp.Data.Content, "base64:"))
	require.NoError(t, err)
	decl, err := domain.ParseDeclarationWithDefaultType(resp.Data.FileName, archive, entity.TypeCustomSkill)
	require.NoError(t, err)
	require.Contains(t, decl.SkillMD, "# Full body")
	require.Len(t, decl.Resources, 1)
	require.Equal(t, "references/prompt.md", decl.Resources[0].Path)
	require.Equal(t, []byte("keep"), decl.Resources[0].Content)
}

func TestMergeUpdateRequestPreservesOptionalAndTrustedFields(t *testing.T) {
	current := &entity.Skill{ID: 101, SpaceID: 1, Type: entity.TypeCustomSkill, IconURI: "icon://keep", UsageScenarios: "keep", DevelopmentThreadID: 901, CreatedAt: 1}
	updated, err := mergeUpdateRequest(current, &skillapi.UpdateSkillRequest{ID: 101, SpaceID: 1, Type: skillapi.SkillType_CustomSkill, Name: "updated", InputSchema: `{}`, OutputSchema: `{}`, Executor: `{}`, Permissions: `{}`})
	require.NoError(t, err)
	require.Equal(t, "icon://keep", updated.IconURI)
	require.Equal(t, "keep", updated.UsageScenarios)
	require.Equal(t, int64(901), updated.DevelopmentThreadID)
}

func TestAuthorizeSkillWriteRejectsBuiltinSkill(t *testing.T) {
	app := &ApplicationService{DomainSVC: &recordingSkillDomainService{skill: &entity.Skill{ID: 101, SpaceID: 1, Type: entity.TypeDeerSkill}}}
	_, err := app.authorizeSkillWrite(context.Background(), 101)
	require.ErrorIs(t, err, ErrSkillReadOnly)
}

func TestApplicationExportSkillVersionReturnsNotFoundForUnknownVersion(t *testing.T) {
	app := &ApplicationService{
		DomainSVC: &recordingSkillDomainService{
			versions: []*entity.SkillVersion{{ID: 201, SkillID: 101, Version: "1.1.0"}},
		},
	}

	_, err := app.ExportSkillVersion(context.Background(), &skillapi.ExportSkillVersionRequest{
		SkillID:   101,
		VersionID: 404,
	})

	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorContains(t, err, "version 404")
}

func TestApplicationRollbackSkillVersionRestoresSkill(t *testing.T) {
	domainSVC := &recordingSkillDomainService{
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
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	resp, err := app.RollbackSkillVersion(context.Background(), &skillapi.RollbackSkillVersionRequest{
		SkillID:           101,
		VersionID:         201,
		ExpectedVersionID: 301,
	})

	require.NoError(t, err)
	require.Equal(t, int64(101), domainSVC.rollbackSkillID)
	require.Equal(t, int64(201), domainSVC.rollbackVersionID)
	require.Equal(t, int64(0), resp.Code)
	require.Equal(t, "success", resp.Msg)
	require.Equal(t, int64(101), resp.Data.ID)
	require.Equal(t, "weekly-research", resp.Data.Name)
	require.Equal(t, skillapi.SkillType_DeerSkill, resp.Data.Type)
	require.Equal(t, "1.0.0", resp.Data.Version)
}

func TestApplicationDeleteSkillCallsDomainSoftDelete(t *testing.T) {
	domainSVC := &recordingSkillDomainService{
		deleted: &entity.Skill{
			ID:           101,
			SpaceID:      1,
			Name:         "weekly-research",
			Description:  "Research weekly market changes.",
			Type:         entity.TypeCustomSkill,
			Version:      "1.0.0",
			Enabled:      false,
			InputSchema:  `{}`,
			OutputSchema: `{}`,
			Executor:     `{}`,
			Permissions:  `{}`,
			CreatedAt:    1000,
			UpdatedAt:    2000,
			DeletedAt:    3000,
		},
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	resp, err := app.DeleteSkill(context.Background(), &skillapi.GetSkillRequest{
		SkillID: 101,
	})

	require.NoError(t, err)
	require.Equal(t, int64(101), domainSVC.deletedSkillID)
	require.Equal(t, int64(0), resp.Code)
	require.Equal(t, "success", resp.Msg)
	require.Equal(t, int64(101), resp.Data.ID)
	require.Equal(t, "weekly-research", resp.Data.Name)
	require.False(t, resp.Data.Enabled)
}

func TestApplicationListSkillToolCandidatesReturnsRuntimeGrantOptions(t *testing.T) {
	app := &ApplicationService{
		ToolCandidateProvider: ToolCandidateProviderFunc(
			func(_ context.Context, spaceID int64) ([]*skillapi.SkillToolCandidate, error) {
				require.Equal(t, int64(1), spaceID)

				return []*skillapi.SkillToolCandidate{
					{
						Name:        "mcp_100_search_docs",
						DisplayName: "Search docs",
						Description: "Search internal documentation through an approved MCP server.",
						Category:    "mcp",
						Visibility:  "static",
						Source:      "mcp",
						SourceID:    "100",
						SourceName:  "docs-mcp",
					},
					{
						Name:        "web_fetch",
						DisplayName: "Duplicate web fetch",
						Description: "This duplicate must not replace the builtin grant.",
						Category:    "mcp",
						Visibility:  "static",
						Source:      "mcp",
						SourceID:    "101",
						SourceName:  "duplicate-mcp",
					},
					{
						Name:        "9unsafe",
						DisplayName: "Unsafe",
						Description: "Unsafe tool names are filtered before they reach the UI.",
					},
					{
						Name:        "mcp_102_empty_description",
						DisplayName: "Empty description",
						Description: "",
					},
				}, nil
			},
		),
	}

	resp, err := app.ListSkillToolCandidates(context.Background(), &skillapi.ListSkillToolCandidatesRequest{
		SpaceID: 1,
	})

	require.NoError(t, err)
	require.Equal(t, int64(0), resp.Code)
	require.Equal(t, "success", resp.Msg)
	require.Len(t, resp.Data.Tools, 5)
	require.Equal(t, []*skillapi.SkillToolCandidate{
		{
			Name:        "web_fetch",
			DisplayName: "Web fetch",
			Description: "Fetch bounded text content from an explicitly allowed web host.",
			Category:    "web",
			Visibility:  "static",
			Source:      "builtin",
		},
		{
			Name:        "web_search",
			DisplayName: "Web search",
			Description: "Search the web using a Coze-approved search backend.",
			Category:    "web",
			Visibility:  "static",
			Source:      "builtin",
		},
		{
			Name:        "ask_user_clarification",
			DisplayName: "Ask user clarification",
			Description: "Ask the user for missing information required to continue the task.",
			Category:    "human_interaction",
			Visibility:  "static",
			Source:      "builtin",
		},
		{
			Name:        "request_human_confirmation",
			DisplayName: "Request human confirmation",
			Description: "Ask the user to approve or reject a sensitive action before continuing.",
			Category:    "human_interaction",
			Visibility:  "static",
			Source:      "builtin",
		},
		{
			Name:        "mcp_100_search_docs",
			DisplayName: "Search docs",
			Description: "Search internal documentation through an approved MCP server.",
			Category:    "mcp",
			Visibility:  "static",
			Source:      "mcp",
			SourceID:    "100",
			SourceName:  "docs-mcp",
		},
	}, resp.Data.Tools)
}

func TestApplicationListSkillToolCandidatesReturnsProviderErrors(t *testing.T) {
	app := &ApplicationService{
		ToolCandidateProvider: ToolCandidateProviderFunc(
			func(context.Context, int64) ([]*skillapi.SkillToolCandidate, error) {
				return nil, fmt.Errorf("tool registry unavailable")
			},
		),
	}

	_, err := app.ListSkillToolCandidates(context.Background(), &skillapi.ListSkillToolCandidatesRequest{
		SpaceID: 1,
	})

	require.ErrorContains(t, err, "tool registry unavailable")
}

func TestApplicationUpdateSkillVersionResourceReturnsNewVersion(t *testing.T) {
	domainSVC := &recordingSkillDomainService{
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
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	resp, err := app.UpdateSkillVersionResource(context.Background(), &skillapi.UpdateSkillVersionResourceRequest{
		SkillID:       101,
		VersionID:     201,
		Path:          "references/prompt.md",
		ContentBase64: "VXNlIGNvbmNpc2UgYnVsbGV0cy4=",
	})

	require.NoError(t, err)
	require.Equal(t, int64(101), domainSVC.updatedResourceSkillID)
	require.Equal(t, int64(201), domainSVC.updatedResourceVersionID)
	require.Equal(t, "references/prompt.md", domainSVC.updatedResourcePath)
	require.Equal(t, []byte("Use concise bullets."), domainSVC.updatedResourceContent)
	require.Equal(t, int64(0), resp.Code)
	require.Equal(t, "success", resp.Msg)
	require.Equal(t, int64(401), resp.Data.ID)
	require.Equal(t, "# Weekly Report", resp.Data.SkillMD)
}

func TestApplicationUpdateSkillVersionContentReturnsNewVersion(t *testing.T) {
	domainSVC := &recordingSkillDomainService{
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
	}
	app := &ApplicationService{DomainSVC: domainSVC}

	resp, err := app.UpdateSkillVersionContent(context.Background(), &skillapi.UpdateSkillVersionContentRequest{
		SkillID:   101,
		VersionID: 201,
		SkillMD:   "# Updated Weekly Report",
	})

	require.NoError(t, err)
	require.Equal(t, int64(101), domainSVC.updatedContentSkillID)
	require.Equal(t, int64(201), domainSVC.updatedContentVersionID)
	require.Equal(t, "# Updated Weekly Report", domainSVC.updatedSkillMD)
	require.Equal(t, int64(0), resp.Code)
	require.Equal(t, "success", resp.Msg)
	require.Equal(t, int64(402), resp.Data.ID)
	require.Equal(t, "# Updated Weekly Report", resp.Data.SkillMD)
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
	skill                     *entity.Skill
	versions                  []*entity.SkillVersion
	resources                 []*entity.SkillResource
	rolledBack                *entity.Skill
	updatedResourceVersion    *entity.SkillVersion
	updatedContentVersion     *entity.SkillVersion
	deleted                   *entity.Skill
	listVersionsSkillID       int64
	listResourcesSkillID      int64
	listResourcesVersionID    int64
	rollbackSkillID           int64
	rollbackVersionID         int64
	updatedResourceSkillID    int64
	updatedResourceVersionID  int64
	updatedResourcePath       string
	updatedResourceContent    []byte
	updatedContentSkillID     int64
	updatedContentVersionID   int64
	updatedSkillMD            string
	deletedSkillID            int64
	imported                  *entity.Skill
	importSpaceID             int64
	importFileName            string
	importContent             []byte
	importDefaultType         entity.Type
	importDevelopmentThreadID int64
}

func (s *recordingSkillDomainService) Get(context.Context, int64) (*entity.Skill, error) {
	if s.skill != nil {
		return s.skill, nil
	}
	return &entity.Skill{ID: 101, SpaceID: 1, Type: entity.TypeCustomSkill}, nil
}

func (s *recordingSkillDomainService) ImportDeclarationWithDefaultType(ctx context.Context, spaceID int64, fileName string, content []byte, defaultType entity.Type) (*entity.Skill, error) {
	s.importSpaceID = spaceID
	s.importFileName = fileName
	s.importContent = append([]byte(nil), content...)
	s.importDefaultType = defaultType
	return s.imported, nil
}

func (s *recordingSkillDomainService) ImportDeclarationWithDevelopmentThread(ctx context.Context, spaceID int64, fileName string, content []byte, defaultType entity.Type, developmentThreadID int64) (*entity.Skill, error) {
	s.importDevelopmentThreadID = developmentThreadID
	skill, err := s.ImportDeclarationWithDefaultType(ctx, spaceID, fileName, content, defaultType)
	if skill != nil {
		skill.DevelopmentThreadID = developmentThreadID
	}
	return skill, err
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

func (s *recordingSkillDomainService) RollbackVersion(ctx context.Context, skillID, versionID int64) (*entity.Skill, error) {
	s.rollbackSkillID = skillID
	s.rollbackVersionID = versionID
	return s.rolledBack, nil
}

func (s *recordingSkillDomainService) RollbackVersionCAS(ctx context.Context, skillID, versionID, _ int64) (*entity.Skill, error) {
	return s.RollbackVersion(ctx, skillID, versionID)
}

func (s *recordingSkillDomainService) Delete(ctx context.Context, skillID int64) (*entity.Skill, error) {
	s.deletedSkillID = skillID
	return s.deleted, nil
}

func (s *recordingSkillDomainService) UpdateVersionResource(ctx context.Context, skillID, versionID int64, path string, content []byte) (*entity.SkillVersion, error) {
	s.updatedResourceSkillID = skillID
	s.updatedResourceVersionID = versionID
	s.updatedResourcePath = path
	s.updatedResourceContent = append([]byte(nil), content...)
	return s.updatedResourceVersion, nil
}

func (s *recordingSkillDomainService) UpdateVersionContent(ctx context.Context, skillID, versionID int64, skillMD string) (*entity.SkillVersion, error) {
	s.updatedContentSkillID = skillID
	s.updatedContentVersionID = versionID
	s.updatedSkillMD = skillMD
	return s.updatedContentVersion, nil
}

func readZipArchive(t *testing.T, content []byte) map[string]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	require.NoError(t, err)
	files := map[string]string{}
	for _, file := range reader.File {
		rc, err := file.Open()
		require.NoError(t, err)
		bs, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		files[file.Name] = string(bs)
	}
	return files
}

func TestExportContentIncludesSkillManagementMetadata(t *testing.T) {
	content, err := exportContent(&entity.Skill{
		ID: 101, Name: "weekly-report", Description: "Create weekly reports",
		Type: entity.TypeCustomSkill, Version: "1.0.0", Enabled: true,
		InputSchema: `{}`, OutputSchema: `{}`, Executor: `{}`, Permissions: `{}`,
		IconURI: "skill-icon://ocean", UsageScenarios: "Summarize delivery progress",
	})

	require.NoError(t, err)
	require.JSONEq(t, `{
		"id":"101",
		"name":"weekly-report",
		"description":"Create weekly reports",
		"type":"custom_skill",
		"version":"1.0.0",
		"enabled":true,
		"icon_uri":"skill-icon://ocean",
		"usage_scenarios":"Summarize delivery progress",
		"input_schema":{},
		"output_schema":{},
		"executor":{},
		"permissions":{}
	}`, content)
}
