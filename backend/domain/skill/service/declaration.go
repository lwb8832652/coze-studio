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
	"strings"

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
}

type ExecutorDeclaration struct {
	Language   string `json:"language" yaml:"language"`
	Entry      string `json:"entry" yaml:"entry"`
	WorkflowID string `json:"workflow_id" yaml:"workflow_id"`
	Version    string `json:"version" yaml:"version"`
}

type PermissionDeclaration struct {
	Network         bool     `json:"network" yaml:"network"`
	FilesystemRead  []string `json:"filesystem_read" yaml:"filesystem_read"`
	FilesystemWrite []string `json:"filesystem_write" yaml:"filesystem_write"`
}

func ParseDeclaration(fileName string, content []byte) (*Declaration, error) {
	decl := &Declaration{}
	lowerFileName := strings.ToLower(fileName)

	switch {
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

func ValidateDeclaration(decl *Declaration) error {
	if decl == nil {
		return fmt.Errorf("declaration is required")
	}

	if strings.TrimSpace(decl.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(decl.Name) == "" {
		return fmt.Errorf("name is required")
	}

	declType := strings.TrimSpace(decl.Type)
	if declType == "" {
		return fmt.Errorf("type is required")
	}

	switch declType {
	case "script":
		if strings.TrimSpace(decl.Executor.Language) != "python" {
			return fmt.Errorf("executor.language must be python")
		}
		if strings.TrimSpace(decl.Executor.Entry) == "" {
			return fmt.Errorf("executor.entry is required")
		}
	case "workflow":
		if strings.TrimSpace(decl.Executor.WorkflowID) == "" {
			return fmt.Errorf("workflow_id is required")
		}
	default:
		return fmt.Errorf("unsupported declaration type: %s", decl.Type)
	}

	return nil
}
