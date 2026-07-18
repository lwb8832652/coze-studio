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

package appdev

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

func providerExecutionStartSubmissionInput(
	t testing.TB,
	cas domainappdev.ProviderExecutionOwnerCAS,
	operationID string,
) domainappdev.StartProviderSubmissionInput {
	t.Helper()
	operationHash, err := domainappdev.HashProviderExecutionLaunchOperationID(operationID, cas)
	if err != nil {
		t.Fatalf("hash provider launch operation: %v", err)
	}
	return domainappdev.StartProviderSubmissionInput{
		OwnerCAS:            cas,
		OperationHash:       operationHash,
		ProviderOperationID: "provider-operation-" + operationID,
		RequestDigest: domainappdev.ProviderExecutionLaunchRequestDigest(
			sha256.Sum256([]byte("canonical-launch-" + operationID)),
		),
		LeaseDuration: time.Minute,
	}
}

func providerExecutionServiceStartSubmissionRequest(
	owner applicationappdev.ProviderExecutionOwner,
	expectedVersion uint64,
	operationID string,
) applicationappdev.StartProviderExecutionSubmissionRequest {
	return applicationappdev.StartProviderExecutionSubmissionRequest{
		Owner: owner, ExpectedVersion: expectedVersion, OperationID: operationID,
		ProviderOperationID: "provider-operation-" + operationID,
		RequestDigest: domainappdev.ProviderExecutionLaunchRequestDigest(
			sha256.Sum256([]byte("canonical-launch-" + operationID)),
		),
	}
}

func markProviderExecutionSubmittedForTest(
	t testing.TB,
	repository *ProviderExecutionRepository,
	record *domainappdev.ProviderExecution,
	ownerHash domainappdev.ProviderExecutionOwnerHash,
	operationID string,
) *domainappdev.ProviderExecution {
	t.Helper()
	cas := providerExecutionCAS(record, ownerHash, time.Time{})
	operationHash, err := domainappdev.HashProviderExecutionLaunchOperationID(operationID, cas)
	if err != nil {
		t.Fatalf("hash provider launch operation: %v", err)
	}
	submitted, err := repository.MarkSubmissionSubmitted(
		context.Background(),
		domainappdev.MarkProviderLaunchSubmittedInput{
			OwnerCAS: cas, OperationHash: operationHash,
			DispatchLeaseDuration: time.Minute,
		},
	)
	if err != nil {
		t.Fatalf("mark provider launch submitted: %v", err)
	}
	return submitted
}

func providerExecutionSaveSubmissionInput(
	t testing.TB,
	cas domainappdev.ProviderExecutionOwnerCAS,
	operationID string,
	providerExecutionID string,
	leaseDuration time.Duration,
	previewRoute string,
) domainappdev.SaveProviderSubmissionInput {
	t.Helper()
	operationHash, err := domainappdev.HashProviderExecutionLaunchOperationID(operationID, cas)
	if err != nil {
		t.Fatalf("hash provider launch operation: %v", err)
	}
	return domainappdev.SaveProviderSubmissionInput{
		OwnerCAS: cas, OperationHash: operationHash,
		ProviderExecutionID:   providerExecutionID,
		ObservedState:         domainappdev.ProviderExecutionObservedRunning,
		ProviderLeaseDuration: leaseDuration, PreviewRoute: previewRoute,
	}
}

func completeProviderExecutionLaunchForTest(
	t testing.TB,
	repository *ProviderExecutionRepository,
	record *domainappdev.ProviderExecution,
	ownerHash domainappdev.ProviderExecutionOwnerHash,
	launchOperationID string,
) *domainappdev.ProviderExecution {
	t.Helper()
	cas := providerExecutionCAS(record, ownerHash, time.Time{})
	launchHash, err := domainappdev.HashProviderExecutionLaunchOperationID(launchOperationID, cas)
	if err != nil {
		t.Fatalf("hash provider launch operation: %v", err)
	}
	checkpointHash, err := domainappdev.HashProviderExecutionOperationID("fixture-checkpoint-" + launchOperationID)
	if err != nil {
		t.Fatalf("hash fixture checkpoint operation: %v", err)
	}
	reservation, err := repository.ReserveCheckpointWrite(
		context.Background(),
		domainappdev.ReserveProviderCheckpointWriteInput{
			OwnerCAS: cas, OperationHash: checkpointHash, ReservationDuration: time.Minute,
		},
	)
	if err != nil {
		t.Fatalf("reserve fixture checkpoint: %v", err)
	}
	cas.ExpectedVersion = reservation.ReservedVersion
	completed, err := repository.SaveCheckpoint(
		context.Background(),
		domainappdev.SaveProviderCheckpointInput{
			OwnerCAS: cas, OperationHash: checkpointHash, LaunchOperationHash: launchHash,
			CheckpointWriteRevision: reservation.Revision, CheckpointEnvelope: "fixture-checkpoint-envelope",
		},
	)
	if err != nil {
		t.Fatalf("complete fixture launch: %v", err)
	}
	return completed
}
