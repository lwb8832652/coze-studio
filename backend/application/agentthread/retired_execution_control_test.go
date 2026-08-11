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

package agentthread

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubmittedExecutionControlsRejectsRetiredFields(t *testing.T) {
	retiredFields := []string{
		"requested_policy",
		"mode",
		"thinking_enabled",
		"reasoning_effort",
		"is_plan_mode",
		"subagent_enabled",
		"max_concurrent_subagents",
	}
	paths := []struct {
		name            string
		configTemplate  string
		contextTemplate string
		pathTemplate    string
	}{
		{
			name:           "config root",
			configTemplate: `{"%s":true}`,
			pathTemplate:   "config.%s",
		},
		{
			name:            "context root",
			contextTemplate: `{"%s":true}`,
			pathTemplate:    "context.%s",
		},
		{
			name:           "config four reserved hops",
			configTemplate: `{"configurable":{"context":{"configurable":{"context":{"%s":true}}}}}`,
			pathTemplate:   "config.configurable.context.configurable.context.%s",
		},
		{
			name:            "context four reserved hops",
			contextTemplate: `{"context":{"configurable":{"context":{"configurable":{"%s":true}}}}}`,
			pathTemplate:    "context.context.configurable.context.configurable.%s",
		},
	}

	for _, path := range paths {
		path := path
		for _, field := range retiredFields {
			field := field
			t.Run(path.name+" "+field, func(t *testing.T) {
				config := ""
				if path.configTemplate != "" {
					config = fmt.Sprintf(path.configTemplate, field)
				}
				runContext := ""
				if path.contextTemplate != "" {
					runContext = fmt.Sprintf(path.contextTemplate, field)
				}

				err := validateSubmittedExecutionControls(config, runContext)

				require.ErrorIs(t, err, ErrUnsupportedExecutionControl)
				controlPath, ok := UnsupportedExecutionControlPath(err)
				require.True(t, ok)
				require.Equal(t, fmt.Sprintf(path.pathTemplate, field), controlPath)
			})
		}
	}
}

func TestSubmittedExecutionControlsCanonicalizesKeysAndPriority(t *testing.T) {
	err := validateSubmittedExecutionControls(
		`{"MoDe":"pro","ReQuEsTeD_PoLiCy":"auto"}`,
		`{"requested_policy":"context"}`,
	)
	require.ErrorIs(t, err, ErrUnsupportedExecutionControl)
	controlPath, ok := UnsupportedExecutionControlPath(err)
	require.True(t, ok)
	require.Equal(t, "config.requested_policy", controlPath)

	err = validateSubmittedExecutionControls(
		`{"context":{"mode":"nested"},"configurable":{"reasoning_effort":"high"}}`,
		"",
	)
	require.ErrorIs(t, err, ErrUnsupportedExecutionControl)
	controlPath, ok = UnsupportedExecutionControlPath(err)
	require.True(t, ok)
	require.Equal(t, "config.configurable.reasoning_effort", controlPath)
}

func TestSubmittedExecutionControlsAllowsOrdinaryBusinessObjects(t *testing.T) {
	for _, test := range []struct {
		name       string
		config     string
		runContext string
	}{
		{
			name: "legal runtime and resources",
			config: `{
				"runtime":"eino_adk",
				"model":{"id":"model"},
				"skill":{"id":"skill"},
				"mcp":{"id":"mcp"},
				"knowledge":{"id":"knowledge"},
				"database":{"id":"database"},
				"resource":{"mode":"business field"},
				"resources":{"mode":"business collection field"}
			}`,
		},
		{
			name:       "context business object",
			runContext: `{"scheduled_task":{"variables":{"mode":"daily"}}}`,
		},
		{
			name:   "strings and arrays are not scanned",
			config: `{"note":"requested_policy and mode","items":[{"mode":"business"}]}`,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, validateSubmittedExecutionControls(test.config, test.runContext))
		})
	}
}

func TestSubmittedExecutionControlsPreservesInvalidRuntimeConfig(t *testing.T) {
	err := validateSubmittedExecutionControls(`{"runtime":`, "")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidRuntimeConfig))
	require.False(t, errors.Is(err, ErrUnsupportedExecutionControl))
}
