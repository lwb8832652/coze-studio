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
