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
	"fmt"
	"os"
	"strconv"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

const (
	agentThreadAdaptiveExecutionEnabledEnv            = "AGENT_THREAD_ADAPTIVE_EXECUTION_ENABLED"
	agentThreadAdaptiveExecutionRolloutBasisPointsEnv = "AGENT_THREAD_ADAPTIVE_EXECUTION_ROLLOUT_BASIS_POINTS"
	adaptiveExecutionEligibilityFeature               = "workbench_adaptive_execution_mvp"
	adaptiveExecutionRolloutMaximumBasisPoints        = 10000
)

type AdaptiveEligibilityRequest struct {
	SpaceID int64
}

type AdaptiveEligibilityResolver interface {
	Resolve(context.Context, AdaptiveEligibilityRequest) (entity.AdaptiveAdmissionSnapshot, error)
}

type envAdaptiveEligibilityResolver struct{}

func NewEnvAdaptiveEligibilityResolver() AdaptiveEligibilityResolver {
	return envAdaptiveEligibilityResolver{}
}

func (envAdaptiveEligibilityResolver) Resolve(
	ctx context.Context,
	request AdaptiveEligibilityRequest,
) (entity.AdaptiveAdmissionSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return entity.AdaptiveAdmissionSnapshot{}, err
	}
	if request.SpaceID <= 0 {
		return entity.AdaptiveAdmissionSnapshot{}, fmt.Errorf("adaptive eligibility space identity is invalid")
	}
	enabled, err := strictAdaptiveEligibilityBoolEnv(agentThreadAdaptiveExecutionEnabledEnv, false)
	if err != nil {
		return entity.AdaptiveAdmissionSnapshot{}, err
	}
	rollout, err := strictAdaptiveEligibilityIntEnv(
		agentThreadAdaptiveExecutionRolloutBasisPointsEnv,
		0,
	)
	if err != nil {
		return entity.AdaptiveAdmissionSnapshot{}, err
	}
	if rollout < 0 || rollout > adaptiveExecutionRolloutMaximumBasisPoints {
		return entity.AdaptiveAdmissionSnapshot{}, fmt.Errorf(
			"%s must be between 0 and %d",
			agentThreadAdaptiveExecutionRolloutBasisPointsEnv,
			adaptiveExecutionRolloutMaximumBasisPoints,
		)
	}
	if !enabled {
		rollout = 0
	}

	admission := baselineAdaptiveAdmission()
	admission.FeatureGateEnabled = rollout > adaptiveExecutionEligibilityBucket(request.SpaceID)
	return admission, nil
}

func strictAdaptiveEligibilityBoolEnv(key string, fallback bool) (bool, error) {
	raw, exists := os.LookupEnv(key)
	if !exists {
		return fallback, nil
	}
	switch raw {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be exactly true or false", key)
	}
}

func strictAdaptiveEligibilityIntEnv(key string, fallback int) (int, error) {
	raw, exists := os.LookupEnv(key)
	if !exists {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return value, nil
}

func adaptiveExecutionEligibilityBucket(spaceID int64) int {
	digest := sha256.Sum256([]byte(
		adaptiveExecutionEligibilityFeature + "\x00" + strconv.FormatInt(spaceID, 10),
	))
	return int(binary.BigEndian.Uint64(digest[:8]) % adaptiveExecutionRolloutMaximumBasisPoints)
}
