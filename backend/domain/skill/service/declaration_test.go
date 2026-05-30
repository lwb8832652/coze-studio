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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDeclarationYAML(t *testing.T) {
	content := []byte(`
id: weekly_report
name: Weekly Report
description: Generate a weekly report.
type: script
version: v1.0.0
enabled: true
input_schema:
  type: object
output_schema:
  type: object
executor:
  language: python
  entry: main.py
permissions:
  network: false
  filesystem_read:
    - /tmp/input
  filesystem_write:
    - /tmp/output
`)

	decl, err := ParseDeclaration("weekly_report.yaml", content)

	require.NoError(t, err)
	assert.Equal(t, "weekly_report", decl.ID)
	assert.Equal(t, "script", decl.Type)
	assert.Equal(t, "python", decl.Executor.Language)
	assert.False(t, decl.Permissions.Network)
}

func TestValidateDeclarationRequiresWorkflowID(t *testing.T) {
	decl := &Declaration{
		ID:      "customer_workflow",
		Name:    "Customer Workflow",
		Type:    "workflow",
		Enabled: true,
	}

	err := ValidateDeclaration(decl)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow_id is required")
}

func TestParseDeclarationJSON(t *testing.T) {
	content := []byte(`{
		"id": "customer_workflow",
		"name": "Customer Workflow",
		"description": "Run a customer workflow.",
		"type": "workflow",
		"version": "v1.0.0",
		"enabled": true,
		"input_schema": {"type": "object"},
		"output_schema": {"type": "object"},
		"executor": {
			"workflow_id": "workflow-123",
			"version": "v2"
		},
		"permissions": {
			"network": true
		}
	}`)

	decl, err := ParseDeclaration("customer_workflow.json", content)

	require.NoError(t, err)
	assert.Equal(t, "customer_workflow", decl.ID)
	assert.Equal(t, "workflow", decl.Type)
	assert.Equal(t, "workflow-123", decl.Executor.WorkflowID)
	assert.True(t, decl.Permissions.Network)
}

func TestParseDeclarationUnsupportedExtension(t *testing.T) {
	decl, err := ParseDeclaration("weekly_report.toml", []byte("id = weekly_report"))

	require.Error(t, err)
	assert.Nil(t, decl)
	assert.Contains(t, err.Error(), "unsupported declaration file extension")
}

func TestParseDeclarationDecodeError(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		content  []byte
		wantErr  string
	}{
		{
			name:     "yaml",
			fileName: "weekly_report.yaml",
			content:  []byte("id: ["),
			wantErr:  "unmarshal yaml declaration",
		},
		{
			name:     "json",
			fileName: "weekly_report.json",
			content:  []byte(`{"id":`),
			wantErr:  "unmarshal json declaration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl, err := ParseDeclaration(tt.fileName, tt.content)

			require.Error(t, err)
			assert.Nil(t, decl)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateDeclarationNil(t *testing.T) {
	err := ValidateDeclaration(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "declaration is required")
}

func TestValidateDeclarationUnsupportedType(t *testing.T) {
	decl := &Declaration{
		ID:   "weekly_report",
		Name: "Weekly Report",
		Type: "chat",
	}

	err := ValidateDeclaration(decl)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported declaration type")
}

func TestValidateDeclarationScriptLanguageAndEntry(t *testing.T) {
	tests := []struct {
		name    string
		decl    *Declaration
		wantErr string
	}{
		{
			name: "language",
			decl: &Declaration{
				ID:   "weekly_report",
				Name: "Weekly Report",
				Type: "script",
				Executor: ExecutorDeclaration{
					Language: "node",
					Entry:    "main.py",
				},
			},
			wantErr: "executor.language must be python",
		},
		{
			name: "entry",
			decl: &Declaration{
				ID:   "weekly_report",
				Name: "Weekly Report",
				Type: "script",
				Executor: ExecutorDeclaration{
					Language: "python",
				},
			},
			wantErr: "executor.entry or executor.code is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDeclaration(tt.decl)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseDeclarationScriptAllowsInlineCodeWithoutEntry(t *testing.T) {
	content := []byte(`
id: weekly_report
name: Weekly Report
type: script
executor:
  language: python
  code: "print('hello')"
`)

	decl, err := ParseDeclaration("weekly_report.yaml", content)

	require.NoError(t, err)
	assert.Equal(t, "print('hello')", decl.Executor.Code)
	assert.Empty(t, decl.Executor.Entry)
}

func TestValidateDeclarationNormalizesWhitespace(t *testing.T) {
	decl := &Declaration{
		ID:   " weekly_report ",
		Name: " Weekly Report ",
		Type: " script ",
		Executor: ExecutorDeclaration{
			Language:   " python ",
			Entry:      " main.py ",
			Code:       " print('hello') ",
			WorkflowID: " workflow-123 ",
		},
	}

	err := ValidateDeclaration(decl)

	require.NoError(t, err)
	assert.Equal(t, "weekly_report", decl.ID)
	assert.Equal(t, "Weekly Report", decl.Name)
	assert.Equal(t, "script", decl.Type)
	assert.Equal(t, "python", decl.Executor.Language)
	assert.Equal(t, "main.py", decl.Executor.Entry)
	assert.Equal(t, "print('hello')", decl.Executor.Code)
	assert.Equal(t, "workflow-123", decl.Executor.WorkflowID)
}
