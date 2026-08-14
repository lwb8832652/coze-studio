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

package entity

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdaptiveExecutionContractHasNoTransportTagsOrRetiredControls(t *testing.T) {
	require.Equal(t, "workbench-adaptive-admission.v1", AdaptiveAdmissionSchemaV1)
	require.Equal(t, "workbench-adaptive-decision.v1", ExecutionDecisionSchemaV1)
	require.Equal(t, "workbench-adaptive-legacy-decoder.v1", AdaptiveLegacyDecoderVersionV1)

	retired := map[string]struct{}{
		"RequestedPolicy": {}, "Mode": {}, "ThinkingEnabled": {}, "ReasoningEffort": {},
		"IsPlanMode": {}, "SubagentEnabled": {}, "MaxConcurrentSubagents": {},
	}
	for _, value := range []any{
		AdaptiveAdmissionSnapshot{}, AdaptiveCapabilities{}, AdaptiveLimits{},
		ExecutionDecision{}, AdaptiveAcceptanceCheck{},
	} {
		typ := reflect.TypeOf(value)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			require.Empty(t, field.Tag.Get("json"), "%s.%s", typ.Name(), field.Name)
			_, found := retired[field.Name]
			require.False(t, found, "%s.%s", typ.Name(), field.Name)
		}
	}
}
