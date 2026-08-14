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

package application

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdaptiveExecutionProductionWiringInstallsGateOnDecisionDependencies(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	source, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "application.go"))
	require.NoError(t, err)
	text := string(source)

	require.Regexp(t, `EligibilityResolver:\s+agentthread\.NewEnvAdaptiveEligibilityResolver\(\)`, text)
	require.Regexp(t, `BaselineProducer:\s+agentthread\.BaselineAdaptiveDecisionProducer\{\}`, text)
	require.Regexp(t, `AdaptiveProducer:\s+agentthread\.DeterministicAdaptiveDecisionProducer\{\}`, text)
	require.NotContains(t, text, "EligibilityResolver: journalFeatureGate")
	require.NotContains(t, text, "EligibilityResolver: agentthread.NewJournalFeatureGate")
}
