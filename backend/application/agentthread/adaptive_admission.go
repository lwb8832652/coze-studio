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
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/adaptivecontract"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

var (
	ErrAdaptiveAdmissionInvalid      = adaptivecontract.ErrAdaptiveAdmissionInvalid
	ErrExecutionDecisionInvalid      = adaptivecontract.ErrExecutionDecisionInvalid
	ErrAdaptiveDecisionBlockedPolicy = adaptivecontract.ErrAdaptiveDecisionBlockedPolicy
)

// ValidateAdaptiveAdmissionSnapshot remains the application-facing C1 API.
func ValidateAdaptiveAdmissionSnapshot(snapshot entity.AdaptiveAdmissionSnapshot) error {
	return adaptivecontract.ValidateAdaptiveAdmissionSnapshot(snapshot)
}

// ValidateExecutionDecision remains the application-facing C1 API.
func ValidateExecutionDecision(decision entity.ExecutionDecision) error {
	return adaptivecontract.ValidateExecutionDecision(decision)
}

// ValidateExecutionDecisionAgainstAdmission remains the application-facing C1 API.
func ValidateExecutionDecisionAgainstAdmission(
	admission entity.AdaptiveAdmissionSnapshot,
	decision entity.ExecutionDecision,
) error {
	return adaptivecontract.ValidateExecutionDecisionAgainstAdmission(admission, decision)
}
