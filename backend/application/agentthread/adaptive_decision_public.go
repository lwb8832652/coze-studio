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
	"errors"
	"fmt"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func ProjectPublicAdaptiveExecution(
	admission entity.AdaptiveAdmissionSnapshot,
	decision entity.ExecutionDecision,
) (*PublicAdaptiveExecutionSummary, error) {
	if err := ValidateExecutionDecisionAgainstAdmission(admission, decision); err != nil {
		return nil, fmt.Errorf("validate public adaptive execution: %w", err)
	}

	var mode PublicAdaptiveExecutionMode
	switch {
	case decision.Decision == entity.ExecutionDecisionDirect &&
		decision.ExecutionShape == entity.ExecutionShapeEmpty:
		mode = PublicAdaptiveExecutionModeDirect
	case decision.Decision == entity.ExecutionDecisionExecute &&
		decision.ExecutionShape == entity.ExecutionShapeSingleStep:
		mode = PublicAdaptiveExecutionModeSingleStep
	case decision.Decision == entity.ExecutionDecisionExecute &&
		decision.ExecutionShape == entity.ExecutionShapeMultiStep:
		mode = PublicAdaptiveExecutionModeMultiStep
	case decision.Decision == entity.ExecutionDecisionClarification &&
		decision.ExecutionShape == entity.ExecutionShapeEmpty:
		mode = PublicAdaptiveExecutionModeClarification
	default:
		return nil, fmt.Errorf("public adaptive execution decision form is unsupported")
	}

	return &PublicAdaptiveExecutionSummary{
		Schema:                PublicAdaptiveExecutionSchemaV1,
		Enabled:               admission.FeatureGateEnabled,
		Mode:                  mode,
		SafeSummary:           decision.SafeSummary,
		ClarificationQuestion: clonePublicAdaptiveExecutionString(decision.ClarificationQuestion),
	}, nil
}

func clonePublicAdaptiveExecution(
	input *PublicAdaptiveExecutionSummary,
) *PublicAdaptiveExecutionSummary {
	if input == nil {
		return nil
	}
	copy := *input
	copy.ClarificationQuestion = clonePublicAdaptiveExecutionString(input.ClarificationQuestion)
	return &copy
}

func clonePublicAdaptiveExecutionString(input *string) *string {
	if input == nil {
		return nil
	}
	copy := *input
	return &copy
}

func (s *ApplicationService) hydratePublicAdaptiveExecution(
	ctx context.Context,
	run *RunSummary,
) error {
	if s == nil || run == nil || s.AdaptiveBootstrapByRunReader == nil {
		return nil
	}
	if run.ThreadID <= 0 || run.RunID <= 0 {
		return fmt.Errorf("public adaptive execution run identity is invalid")
	}
	result, err := s.AdaptiveBootstrapByRunReader.
		ReadAdaptiveExecutionBootstrapByRun(ctx, repository.ReadAdaptiveExecutionBootstrapByRunRequest{
			ThreadID: run.ThreadID, ExecutionRunID: run.RunID,
		})
	if errors.Is(err, repository.ErrAdaptiveExecutionBootstrapNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("public adaptive execution repository returned an empty aggregate")
	}
	projected, err := ProjectPublicAdaptiveExecution(result.Admission, result.Decision)
	if err != nil {
		return err
	}
	run.AdaptiveExecution = projected
	return nil
}
