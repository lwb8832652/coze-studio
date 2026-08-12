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
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

const adaptiveBootstrapIdentitySchema = "workbench-adaptive-bootstrap.v1"

type AdaptiveBootstrapCoordinator interface {
	Bootstrap(context.Context, *RunSummary) error
}

type AdaptiveBootstrapCoordinatorFunc func(context.Context, *RunSummary) error

func (f AdaptiveBootstrapCoordinatorFunc) Bootstrap(ctx context.Context, run *RunSummary) error {
	if f == nil {
		return fmt.Errorf("adaptive bootstrap coordinator function is required")
	}
	return f(ctx, run)
}

type AdaptiveBootstrapAttemptReader interface {
	GetActiveJournalAttempt(context.Context, int64) (*domainentity.RunAttempt, error)
}

type AdaptiveBootstrapRepository interface {
	ReadAdaptiveExecutionBootstrap(
		context.Context,
		domainrepo.ReadAdaptiveExecutionBootstrapRequest,
	) (*domainrepo.CommitAdaptiveExecutionBootstrapResult, error)
	CommitAdaptiveExecutionBootstrap(
		context.Context,
		domainrepo.CommitAdaptiveExecutionBootstrapRequest,
	) (*domainrepo.CommitAdaptiveExecutionBootstrapResult, error)
}

type AdaptiveBootstrapIDGenerator interface {
	GenMultiIDs(context.Context, int) ([]int64, error)
}

type AdaptiveBootstrapCoordinatorOptions struct {
	AttemptReader AdaptiveBootstrapAttemptReader
	Repository    AdaptiveBootstrapRepository
	IDGen         AdaptiveBootstrapIDGenerator
	Now           func() int64
}

type adaptiveBootstrapCoordinator struct {
	attemptReader AdaptiveBootstrapAttemptReader
	repository    AdaptiveBootstrapRepository
	idGen         AdaptiveBootstrapIDGenerator
	now           func() int64
}

func NewAdaptiveBootstrapCoordinator(options AdaptiveBootstrapCoordinatorOptions) AdaptiveBootstrapCoordinator {
	return &adaptiveBootstrapCoordinator{
		attemptReader: options.AttemptReader,
		repository:    options.Repository,
		idGen:         options.IDGen,
		now:           options.Now,
	}
}

func (c *adaptiveBootstrapCoordinator) Bootstrap(ctx context.Context, run *RunSummary) error {
	if run == nil {
		return fmt.Errorf("adaptive bootstrap run is required")
	}
	if run.ParentRunID != 0 || (run.RunKind != "" && run.RunKind != RunKindTask) {
		return nil
	}
	if c == nil || c.attemptReader == nil || c.repository == nil || c.idGen == nil || c.now == nil {
		return fmt.Errorf("adaptive bootstrap dependencies are required")
	}
	if run.ThreadID <= 0 || run.RunID <= 0 {
		return fmt.Errorf("adaptive bootstrap run identity is invalid")
	}

	attempt, err := c.attemptReader.GetActiveJournalAttempt(ctx, run.RunID)
	if errors.Is(err, domainrepo.ErrJournalNotEnrolled) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read active journal attempt for adaptive bootstrap: %w", err)
	}
	if run.ExecutionGeneration == 0 || !adaptiveBootstrapIdentityPart(run.LeaseOwner, 191) ||
		!adaptiveBootstrapIdentityPart(run.LeaseToken, 191) {
		return fmt.Errorf("adaptive bootstrap run identity is invalid")
	}
	if !adaptiveBootstrapFreshAttempt(run, attempt) {
		return fmt.Errorf("adaptive bootstrap requires the current fresh journal attempt")
	}

	readRequest := domainrepo.ReadAdaptiveExecutionBootstrapRequest{
		ThreadID:       run.ThreadID,
		ExecutionRunID: run.RunID,
		JournalRunID:   attempt.JournalRunID,
		AttemptID:      attempt.AttemptID,
	}
	result, err := c.repository.ReadAdaptiveExecutionBootstrap(ctx, readRequest)
	if err == nil {
		if result == nil {
			return fmt.Errorf("adaptive bootstrap replay result is required")
		}
		if result.Authority.ExecutionGeneration != run.ExecutionGeneration {
			return fmt.Errorf("adaptive bootstrap replay generation does not match the current run claim")
		}
		return nil
	}
	if !errors.Is(err, domainrepo.ErrAdaptiveExecutionBootstrapNotFound) {
		return fmt.Errorf("read adaptive execution bootstrap: %w", err)
	}

	operationKey := adaptiveBootstrapStableKey("operation", run, attempt)
	decisionID := adaptiveBootstrapStableKey("decision", run, attempt)
	admission := baselineAdaptiveAdmission()
	decision, err := (BaselineDecisionProducer{}).Produce(BaselineDecisionRequest{
		Admission:           admission,
		DecisionID:          decisionID,
		DecisionRevision:    1,
		ExecutionRunID:      run.RunID,
		JournalRunID:        attempt.JournalRunID,
		AttemptID:           attempt.AttemptID,
		ExecutionGeneration: run.ExecutionGeneration,
		PlanScopeRunID:      run.RunID,
		CreatedAt:           attempt.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("produce baseline adaptive decision: %w", err)
	}
	ids, err := c.idGen.GenMultiIDs(ctx, 3)
	if err != nil {
		return fmt.Errorf("allocate adaptive bootstrap identifiers: %w", err)
	}
	if len(ids) != 3 || ids[0] <= 0 || ids[1] <= 0 || ids[2] <= 0 ||
		ids[0] == ids[1] || ids[0] == ids[2] || ids[1] == ids[2] {
		return fmt.Errorf("adaptive bootstrap identifiers are invalid")
	}
	now := c.now()
	if now <= 0 {
		return fmt.Errorf("adaptive bootstrap clock is invalid")
	}
	committed, err := c.repository.CommitAdaptiveExecutionBootstrap(ctx, domainrepo.CommitAdaptiveExecutionBootstrapRequest{
		ThreadID: run.ThreadID, ExecutionRunID: run.RunID,
		JournalRunID: attempt.JournalRunID, AttemptID: attempt.AttemptID,
		LeaseOwner: run.LeaseOwner, LeaseToken: run.LeaseToken,
		OperationKey: operationKey, Generation: run.ExecutionGeneration,
		Now: now, FactCreatedAt: attempt.CreatedAt,
		Admission: admission, Decision: decision,
		AdmissionEventID: ids[0], DecisionEventID: ids[1], CheckpointID: ids[2],
	})
	if err != nil {
		return fmt.Errorf("commit adaptive execution bootstrap: %w", err)
	}
	if committed == nil {
		return fmt.Errorf("adaptive bootstrap commit result is required")
	}
	return nil
}

func baselineAdaptiveAdmission() domainentity.AdaptiveAdmissionSnapshot {
	return domainentity.AdaptiveAdmissionSnapshot{
		Schema:             domainentity.AdaptiveAdmissionSchemaV1,
		FeatureGateEnabled: false,
		Source:             domainentity.AdaptiveAdmissionSourceFresh,
		Capabilities: domainentity.AdaptiveCapabilities{
			PlanAllowed:             true,
			HumanInteractionAllowed: true,
		},
		Limits: domainentity.AdaptiveLimits{
			MaxToolCalls: 24, MaxReplans: 2, MaxVerificationRepairs: 2,
			MaxConsecutiveNoProgress: 3, MaxActiveDurationSeconds: 1200,
		},
	}
}

func adaptiveBootstrapFreshAttempt(run *RunSummary, attempt *domainentity.RunAttempt) bool {
	return attempt != nil && attempt.ThreadID == run.ThreadID && attempt.ExecutionRunID == run.RunID &&
		attempt.JournalRunID > 0 && adaptiveBootstrapIdentityPart(attempt.AttemptID, 64) &&
		attempt.Status.IsActive() && attempt.ActiveSlot != nil && attempt.CreatedAt > 0 &&
		attempt.SourceAttemptID == nil && attempt.SourceCheckpointID == nil &&
		attempt.RecoveryIdempotencyKey == nil
}

func adaptiveBootstrapIdentityPart(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value
}

func adaptiveBootstrapStableKey(kind string, run *RunSummary, attempt *domainentity.RunAttempt) string {
	input := strings.Join([]string{
		adaptiveBootstrapIdentitySchema,
		kind,
		strconv.FormatInt(run.ThreadID, 10),
		strconv.FormatInt(run.RunID, 10),
		strconv.FormatInt(attempt.JournalRunID, 10),
		attempt.AttemptID,
		strconv.FormatUint(run.ExecutionGeneration, 10),
	}, "\x00")
	digest := sha256.Sum256([]byte(input))
	return "adaptive-" + kind + ":" + hex.EncodeToString(digest[:])
}
