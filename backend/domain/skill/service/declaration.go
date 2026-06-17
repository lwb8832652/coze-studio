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

package service

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	"gopkg.in/yaml.v3"
)

type Declaration struct {
	ID           string                `json:"id" yaml:"id"`
	Name         string                `json:"name" yaml:"name"`
	Description  string                `json:"description" yaml:"description"`
	Type         string                `json:"type" yaml:"type"`
	Version      string                `json:"version" yaml:"version"`
	Enabled      bool                  `json:"enabled" yaml:"enabled"`
	InputSchema  map[string]any        `json:"input_schema" yaml:"input_schema"`
	OutputSchema map[string]any        `json:"output_schema" yaml:"output_schema"`
	Executor     ExecutorDeclaration   `json:"executor" yaml:"executor"`
	Permissions  PermissionDeclaration `json:"permissions" yaml:"permissions"`
	Body         string                `json:"-" yaml:"-"`
	SkillMD      string                `json:"-" yaml:"-"`
}

type ExecutorDeclaration struct {
	Language   string `json:"language" yaml:"language"`
	Entry      string `json:"entry" yaml:"entry"`
	Code       string `json:"code,omitempty" yaml:"code,omitempty"`
	WorkflowID string `json:"workflow_id" yaml:"workflow_id"`
	Version    string `json:"version" yaml:"version"`
}

type PermissionDeclaration struct {
	Network         bool     `json:"network" yaml:"network"`
	FilesystemRead  []string `json:"filesystem_read,omitempty" yaml:"filesystem_read"`
	FilesystemWrite []string `json:"filesystem_write,omitempty" yaml:"filesystem_write"`
	AllowedTools    []string `json:"allowed_tools,omitempty" yaml:"allowed-tools"`
}

type skillMarkdownFrontmatter struct {
	ID                string         `yaml:"id"`
	Name              string         `yaml:"name"`
	Description       string         `yaml:"description"`
	Type              string         `yaml:"type"`
	Version           string         `yaml:"version"`
	Enabled           *bool          `yaml:"enabled"`
	InputSchema       map[string]any `yaml:"input_schema"`
	OutputSchema      map[string]any `yaml:"output_schema"`
	AllowedTools      []string       `yaml:"allowed-tools"`
	AllowedToolsSnake []string       `yaml:"allowed_tools"`
}

func ParseDeclaration(fileName string, content []byte) (*Declaration, error) {
	decl := &Declaration{}
	lowerFileName := strings.ToLower(fileName)
	baseFileName := filepath.Base(lowerFileName)

	switch {
	case baseFileName == "skill.md":
		var err error
		decl, err = parseSkillMarkdown(content)
		if err != nil {
			return nil, err
		}
	case strings.HasSuffix(lowerFileName, ".yaml"), strings.HasSuffix(lowerFileName, ".yml"):
		if err := yaml.Unmarshal(content, decl); err != nil {
			return nil, fmt.Errorf("unmarshal yaml declaration: %w", err)
		}
	case strings.HasSuffix(lowerFileName, ".json"):
		if err := json.Unmarshal(content, decl); err != nil {
			return nil, fmt.Errorf("unmarshal json declaration: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported declaration file extension: %s", fileName)
	}

	if err := ValidateDeclaration(decl); err != nil {
		return nil, err
	}

	return decl, nil
}

func parseSkillMarkdown(content []byte) (*Declaration, error) {
	skillMD := string(content)
	trimmedBOM := strings.TrimPrefix(skillMD, "\ufeff")
	lines := strings.SplitAfter(trimmedBOM, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("skill markdown frontmatter is required")
	}

	closeIndex := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closeIndex = i
			break
		}
	}
	if closeIndex < 0 {
		return nil, fmt.Errorf("skill markdown closing frontmatter delimiter is required")
	}

	frontmatter := strings.Join(lines[1:closeIndex], "")
	body := strings.Join(lines[closeIndex+1:], "")
	var meta skillMarkdownFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatter), &meta); err != nil {
		return nil, fmt.Errorf("unmarshal skill markdown frontmatter: %w", err)
	}

	enabled := true
	if meta.Enabled != nil {
		enabled = *meta.Enabled
	}
	allowedTools := meta.AllowedTools
	if len(allowedTools) == 0 {
		allowedTools = meta.AllowedToolsSnake
	}

	decl := &Declaration{
		ID:           firstNonEmpty(meta.ID, meta.Name),
		Name:         meta.Name,
		Description:  meta.Description,
		Type:         firstNonEmpty(meta.Type, string(entity.TypeDeerSkill)),
		Version:      firstNonEmpty(meta.Version, "1.0.0"),
		Enabled:      enabled,
		InputSchema:  defaultObjectSchema(meta.InputSchema),
		OutputSchema: defaultObjectSchema(meta.OutputSchema),
		Permissions: PermissionDeclaration{
			AllowedTools: allowedTools,
		},
		Body:    strings.TrimSpace(body),
		SkillMD: skillMD,
	}

	return decl, nil
}

func ValidateDeclaration(decl *Declaration) error {
	if decl == nil {
		return fmt.Errorf("declaration is required")
	}

	decl.ID = strings.TrimSpace(decl.ID)
	decl.Name = strings.TrimSpace(decl.Name)
	decl.Description = strings.TrimSpace(decl.Description)
	decl.Type = strings.TrimSpace(decl.Type)
	decl.Version = strings.TrimSpace(decl.Version)
	decl.Executor.Language = strings.TrimSpace(decl.Executor.Language)
	decl.Executor.Entry = strings.TrimSpace(decl.Executor.Entry)
	decl.Executor.Code = strings.TrimSpace(decl.Executor.Code)
	decl.Executor.WorkflowID = strings.TrimSpace(decl.Executor.WorkflowID)
	decl.Permissions.AllowedTools = normalizeStringList(decl.Permissions.AllowedTools)

	if decl.ID == "" {
		return fmt.Errorf("id is required")
	}
	if decl.Name == "" {
		return fmt.Errorf("name is required")
	}
	if decl.Type == "" {
		return fmt.Errorf("type is required")
	}

	switch decl.Type {
	case "script":
		if decl.Executor.Language != "python" {
			return fmt.Errorf("executor.language must be python")
		}
		if decl.Executor.Entry == "" && decl.Executor.Code == "" {
			return fmt.Errorf("executor.entry or executor.code is required")
		}
	case "workflow":
		if decl.Executor.WorkflowID == "" {
			return fmt.Errorf("workflow_id is required")
		}
	case string(entity.TypeDeerSkill), string(entity.TypePublicSkill), string(entity.TypeCustomSkill):
		if decl.Description == "" {
			return fmt.Errorf("description is required")
		}
		if decl.Version == "" {
			decl.Version = "1.0.0"
		}
	default:
		return fmt.Errorf("unsupported declaration type: %s", decl.Type)
	}

	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func defaultObjectSchema(schema map[string]any) map[string]any {
	if schema != nil {
		return schema
	}
	return map[string]any{}
}

func normalizeStringList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
