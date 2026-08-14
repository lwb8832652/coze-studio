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
	"context"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"testing"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/stretchr/testify/require"
)

func TestEnvAdaptiveEligibilityResolverDefaultsOff(t *testing.T) {
	unsetAdaptiveEligibilityEnvForTest(t, agentThreadAdaptiveExecutionEnabledEnv)
	unsetAdaptiveEligibilityEnvForTest(t, agentThreadAdaptiveExecutionRolloutBasisPointsEnv)

	admission, err := NewEnvAdaptiveEligibilityResolver().Resolve(
		context.Background(), AdaptiveEligibilityRequest{SpaceID: 42},
	)

	require.NoError(t, err)
	require.Equal(t, baselineAdaptiveAdmission(), admission)
	require.Equal(t, entity.AdaptiveAdmissionSourceFresh, admission.Source)
}

func TestEnvAdaptiveEligibilityResolverUsesStableServerOwnedRollout(t *testing.T) {
	t.Setenv(agentThreadAdaptiveExecutionEnabledEnv, "true")
	t.Setenv(agentThreadAdaptiveExecutionRolloutBasisPointsEnv, "10000")
	resolver := NewEnvAdaptiveEligibilityResolver()

	first, err := resolver.Resolve(context.Background(), AdaptiveEligibilityRequest{SpaceID: 42})
	require.NoError(t, err)
	second, err := resolver.Resolve(context.Background(), AdaptiveEligibilityRequest{SpaceID: 42})
	require.NoError(t, err)
	require.True(t, first.FeatureGateEnabled)
	require.Equal(t, first, second)

	t.Setenv(agentThreadAdaptiveExecutionEnabledEnv, "false")
	disabled, err := resolver.Resolve(context.Background(), AdaptiveEligibilityRequest{SpaceID: 42})
	require.NoError(t, err)
	require.False(t, disabled.FeatureGateEnabled)
}

func TestAdaptiveEligibilityBucketUsesFrozenFeatureAndSpaceOnly(t *testing.T) {
	spaceID := int64(42)
	digest := sha256.Sum256([]byte("workbench_adaptive_execution_mvp\x0042"))
	expected := int(binary.BigEndian.Uint64(digest[:8]) % 10000)

	require.Equal(t, expected, adaptiveExecutionEligibilityBucket(spaceID))
	require.NotEqual(t, adaptiveExecutionEligibilityBucket(spaceID), adaptiveExecutionEligibilityBucket(spaceID+1))
	require.Equal(t, "workbench_adaptive_execution_mvp", adaptiveExecutionEligibilityFeature)
}

func TestEnvAdaptiveEligibilityResolverRejectsInvalidServerConfiguration(t *testing.T) {
	for _, test := range []struct {
		name    string
		enabled string
		rollout string
	}{
		{name: "enabled", enabled: "yes", rollout: "0"},
		{name: "numeric enabled", enabled: "1", rollout: "10000"},
		{name: "uppercase enabled", enabled: "TRUE", rollout: "10000"},
		{name: "empty enabled", enabled: "", rollout: "0"},
		{name: "rollout syntax", enabled: "true", rollout: "one"},
		{name: "rollout negative", enabled: "true", rollout: "-1"},
		{name: "rollout over maximum", enabled: "false", rollout: "10001"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(agentThreadAdaptiveExecutionEnabledEnv, test.enabled)
			t.Setenv(agentThreadAdaptiveExecutionRolloutBasisPointsEnv, test.rollout)

			_, err := NewEnvAdaptiveEligibilityResolver().Resolve(
				context.Background(), AdaptiveEligibilityRequest{SpaceID: 42},
			)

			require.Error(t, err)
		})
	}
}

func TestEnvAdaptiveEligibilityResolverRejectsInvalidSpace(t *testing.T) {
	unsetAdaptiveEligibilityEnvForTest(t, agentThreadAdaptiveExecutionEnabledEnv)
	unsetAdaptiveEligibilityEnvForTest(t, agentThreadAdaptiveExecutionRolloutBasisPointsEnv)

	_, err := NewEnvAdaptiveEligibilityResolver().Resolve(
		context.Background(), AdaptiveEligibilityRequest{SpaceID: 0},
	)

	require.Error(t, err)
}

func unsetAdaptiveEligibilityEnvForTest(t *testing.T, key string) {
	t.Helper()
	value, existed := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if existed {
			require.NoError(t, os.Setenv(key, value))
			return
		}
		require.NoError(t, os.Unsetenv(key))
	})
}
